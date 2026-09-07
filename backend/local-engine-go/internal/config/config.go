package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidPath  = errors.New("config path is invalid")
	ErrPathNotFound = errors.New("config path not found")
	ErrInvalidValue = errors.New("config value is invalid")
)

var (
	osMkdirAll   = os.MkdirAll
	osCreateTemp = os.CreateTemp
	osRename     = os.Rename
	osOpen       = os.Open
	fileWrite    = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
	fileSync     = func(file *os.File) error { return file.Sync() }
	fileClose    = func(file *os.File) error { return file.Close() }
)

type Value struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

type Options struct {
	StateStoreRoot string
	Defaults       map[string]any
	Schema         map[string]any
	WriteFile      func(path string, data []byte) error
}

type Store interface {
	Get(path string, effective bool) (any, error)
	Set(path string, value any) (any, error)
	Remove(path string) (any, error)
	Schema() any
}

type Manager struct {
	mu        sync.Mutex
	path      string
	defaults  map[string]any
	overrides map[string]any
	schema    map[string]any
	writeFile func(path string, data []byte) error
}

func NewManager(opts Options) (*Manager, error) {
	root := strings.TrimSpace(opts.StateStoreRoot)
	if root == "" {
		return nil, fmt.Errorf("state store root is required")
	}
	defaults := opts.Defaults
	if defaults == nil {
		defaults = DefaultConfig()
	}
	schema := opts.Schema
	if schema == nil {
		schema = DefaultSchema()
	}
	writer := opts.WriteFile
	if writer == nil {
		writer = atomicWriteFile
	}
	path := filepath.Join(root, "config.json")
	overrides, err := loadOverrides(path)
	if err != nil {
		return nil, err
	}
	return &Manager{
		path:      path,
		defaults:  defaults,
		overrides: overrides,
		schema:    schema,
		writeFile: writer,
	}, nil
}

func DefaultConfig() map[string]any {
	return map[string]any{
		"log": map[string]any{
			"level": "debug",
		},
		"cache": map[string]any{
			"capacity": map[string]any{
				"maxBytes":      0,
				"reserveBytes":  nil,
				"highWatermark": 0.90,
				"lowWatermark":  0.80,
				"minStateAge":   "10m",
			},
    },
		"container": map[string]any{
			"runtime": "auto",
		},
		"snapshot": map[string]any{
			"backend": "auto",
		},
		"orchestrator": map[string]any{
			"jobs": map[string]any{
				"maxIdentical": 2,
			},
		},
	}
}

func DefaultSchema() map[string]any {
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
		"properties": map[string]any{
			"log": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"level": map[string]any{
						"type": []any{"string", "null"},
						"enum": []any{"debug", "info", "warn", "error", nil},
					},
				},
				"additionalProperties": true,
			},
			"cache": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"capacity": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"maxBytes": map[string]any{
								"type":    []any{"integer", "null"},
								"minimum": 0,
							},
							"reserveBytes": map[string]any{
								"type":    []any{"integer", "null"},
								"minimum": 0,
							},
							"highWatermark": map[string]any{
								"type":    []any{"number", "null"},
								"minimum": 0,
								"maximum": 1,
							},
							"lowWatermark": map[string]any{
								"type":    []any{"number", "null"},
								"minimum": 0,
								"maximum": 1,
							},
							"minStateAge": map[string]any{
								"type": []any{"string", "null"},
							},
						},
						"additionalProperties": true,
          },
        },
      },
			"container": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"runtime": map[string]any{
						"type": []any{"string", "null"},
						"enum": []any{"auto", "docker", "podman", nil},
					},
				},
				"additionalProperties": true,
			},
			"snapshot": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"backend": map[string]any{
						"type": []any{"string", "null"},
						"enum": []any{"auto", "overlay", "btrfs", "zfs", "copy", nil},
					},
				},
				"additionalProperties": true,
			},
			"orchestrator": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"jobs": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"maxIdentical": map[string]any{
								"type":    []any{"integer", "null"},
								"minimum": 0,
							},
						},
						"additionalProperties": true,
					},
				},
				"additionalProperties": true,
			},
		},
		"additionalProperties": true,
	}
}

func (m *Manager) Get(path string, effective bool) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	segments, err := parsePath(path)
	if err != nil {
		return nil, ErrInvalidPath
	}
	var root any
	if effective {
		root = mergeMaps(cloneMap(m.defaults), m.overrides)
	} else {
		root = cloneMap(m.overrides)
	}
	if len(segments) == 0 {
		if root == nil {
			return map[string]any{}, nil
		}
		return root, nil
	}
	value, ok := getPathValue(root, segments)
	if !ok {
		return nil, ErrPathNotFound
	}
	return value, nil
}

func (m *Manager) Set(path string, value any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	segments, err := parsePath(path)
	if err != nil || len(segments) == 0 {
		return nil, ErrInvalidPath
	}
	if err := validateValue(path, value); err != nil {
		return nil, err
	}
	next := cloneMap(m.overrides)
	updated, err := setPathValue(next, segments, value)
	if err != nil {
		return nil, err
	}
	if err := validateCacheConstraintsForPath(path, m.defaults, updated); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := m.writeFile(m.path, data); err != nil {
		return nil, err
	}
	m.overrides = updated
	return value, nil
}

func (m *Manager) Remove(path string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	segments, err := parsePath(path)
	if err != nil || len(segments) == 0 {
		return nil, ErrInvalidPath
	}
	next := cloneMap(m.overrides)
	updated, removed := removePathValue(next, segments)
	if !removed {
		return nil, ErrPathNotFound
	}
	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := m.writeFile(m.path, data); err != nil {
		return nil, err
	}
	m.overrides = updated
	effective := mergeMaps(cloneMap(m.defaults), updated)
	value, ok := getPathValue(effective, segments)
	if !ok {
		return nil, nil
	}
	return value, nil
}

func (m *Manager) Schema() any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneValue(m.schema)
}

func loadOverrides(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	normalized, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config file must contain a JSON object")
	}
	return normalized, nil
}

func validateValue(path string, value any) error {
	if path == "orchestrator.jobs.maxIdentical" {
		if value == nil {
			return nil
		}
		if num, ok := asInt(value); ok {
			if num < 0 {
				return ErrInvalidValue
			}
			return nil
		}
		return ErrInvalidValue
	}
	if path == "cache.capacity.maxBytes" || path == "cache.capacity.reserveBytes" {
		if value == nil {
			return nil
		}
		if num, ok := asInt(value); ok {
			if num < 0 {
				return ErrInvalidValue
			}
			return nil
		}
		return ErrInvalidValue
	}
	if path == "cache.capacity.highWatermark" || path == "cache.capacity.lowWatermark" {
		if value == nil {
			return nil
		}
		next, ok := asFloat(value)
		if !ok {
			return ErrInvalidValue
		}
		if next <= 0 || next > 1 {
			return ErrInvalidValue
		}
		return nil
	}
	if path == "cache.capacity.minStateAge" {
		if value == nil {
			return nil
		}
		str, ok := value.(string)
		if !ok {
			return ErrInvalidValue
		}
		if _, err := time.ParseDuration(strings.TrimSpace(str)); err != nil {
			return ErrInvalidValue
		}
		return nil
	}
	if path == "snapshot.backend" {
		if value == nil {
			return nil
		}
		str, ok := value.(string)
		if !ok {
			return ErrInvalidValue
		}
		switch str {
		case "auto", "overlay", "btrfs", "zfs", "copy":
			return nil
		default:
			return ErrInvalidValue
		}
	}
	if path == "container.runtime" {
		if value == nil {
			return nil
		}
		str, ok := value.(string)
		if !ok {
			return ErrInvalidValue
		}
		switch str {
		case "auto", "docker", "podman":
			return nil
		default:
			return ErrInvalidValue
		}
	}
	if path == "log.level" {
		if value == nil {
			return nil
		}
		str, ok := value.(string)
		if !ok {
			return ErrInvalidValue
		}
		switch strings.ToLower(strings.TrimSpace(str)) {
		case "debug", "info", "warn", "error":
			return nil
		default:
			return ErrInvalidValue
		}
	}
	return nil
}

func asInt(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case float32:
		if math.Trunc(float64(v)) != float64(v) {
			return 0, false
		}
		return int64(v), true
	case float64:
		if math.Trunc(v) != v {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			return 0, false
		}
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func validateCacheConstraintsForPath(path string, defaults map[string]any, overrides map[string]any) error {
	if path != "cache.capacity.lowWatermark" && path != "cache.capacity.highWatermark" {
		return nil
	}
	const (
		defaultCacheHighWatermark = 0.90
	)
	effective := mergeMaps(cloneMap(defaults), overrides)
	high := defaultCacheHighWatermark
	if value, ok := getPathValue(effective, []pathSegment{{key: "cache"}, {key: "capacity"}, {key: "highWatermark"}}); ok && value != nil {
		parsed, ok := asFloat(value)
		if !ok || parsed <= 0 || parsed > 1 {
			return ErrInvalidValue
		}
		high = parsed
	}
	if value, ok := getPathValue(effective, []pathSegment{{key: "cache"}, {key: "capacity"}, {key: "lowWatermark"}}); ok && value != nil {
		parsed, ok := asFloat(value)
		if !ok || parsed <= 0 || parsed > 1 {
			return ErrInvalidValue
		}
		if parsed >= high {
			return ErrInvalidValue
		}
	}
	return nil
}

type pathSegment struct {
	key     string
	index   int
	isIndex bool
}

func parsePath(path string) ([]pathSegment, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	var segments []pathSegment
	var key strings.Builder
	flushKey := func() error {
		segments = append(segments, pathSegment{key: key.String()})
		key.Reset()
		return nil
	}
	for i := 0; i < len(path); i++ {
		ch := path[i]
		switch ch {
		case '.':
			if key.Len() == 0 {
				if len(segments) == 0 || !segments[len(segments)-1].isIndex {
					return nil, ErrInvalidPath
				}
				continue
			}
			if err := flushKey(); err != nil {
				return nil, err
			}
		case '[':
			if key.Len() > 0 {
				if err := flushKey(); err != nil {
					return nil, err
				}
			}
			j := i + 1
			if j >= len(path) {
				return nil, ErrInvalidPath
			}
			start := j
			for j < len(path) && path[j] >= '0' && path[j] <= '9' {
				j++
			}
			if start == j || j >= len(path) || path[j] != ']' {
				return nil, ErrInvalidPath
			}
			index, err := strconv.Atoi(path[start:j])
			if err != nil || index < 0 {
				return nil, ErrInvalidPath
			}
			segments = append(segments, pathSegment{index: index, isIndex: true})
			i = j
		default:
			key.WriteByte(ch)
		}
	}
	if key.Len() > 0 {
		if err := flushKey(); err != nil {
			return nil, err
		}
	}
	return segments, nil
}

func getPathValue(root any, segments []pathSegment) (any, bool) {
	current := root
	for _, seg := range segments {
		if seg.isIndex {
			list, ok := current.([]any)
			if !ok || seg.index < 0 || seg.index >= len(list) {
				return nil, false
			}
			current = list[seg.index]
			continue
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := m[seg.key]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func setPathValue(root map[string]any, segments []pathSegment, value any) (map[string]any, error) {
	updated := setValue(root, segments, value)
	mapped, ok := updated.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config root must be object")
	}
	return mapped, nil
}

func setValue(current any, segments []pathSegment, value any) any {
	seg := segments[0]
	if seg.isIndex {
		list, ok := current.([]any)
		if !ok || list == nil {
			list = []any{}
		}
		if seg.index >= len(list) {
			padding := make([]any, seg.index-len(list)+1)
			list = append(list, padding...)
		}
		if len(segments) == 1 {
			list[seg.index] = value
			return list
		}
		next := setValue(list[seg.index], segments[1:], value)
		list[seg.index] = next
		return list
	}
	m, ok := current.(map[string]any)
	if !ok || m == nil {
		m = map[string]any{}
	}
	if len(segments) == 1 {
		m[seg.key] = value
		return m
	}
	next := setValue(m[seg.key], segments[1:], value)
	m[seg.key] = next
	return m
}

func removePathValue(root map[string]any, segments []pathSegment) (map[string]any, bool) {
	updated, removed := removeValue(root, segments)
	if !removed {
		return root, false
	}
	return updated.(map[string]any), true
}

func removeValue(current any, segments []pathSegment) (any, bool) {
	seg := segments[0]
	if seg.isIndex {
		list, ok := current.([]any)
		if !ok || seg.index < 0 || seg.index >= len(list) {
			return current, false
		}
		if len(segments) == 1 {
			list = append(list[:seg.index], list[seg.index+1:]...)
			return list, true
		}
		next, removed := removeValue(list[seg.index], segments[1:])
		if !removed {
			return current, false
		}
		list[seg.index] = next
		return list, true
	}
	m, ok := current.(map[string]any)
	if !ok {
		return current, false
	}
	if len(segments) == 1 {
		if _, ok := m[seg.key]; !ok {
			return current, false
		}
		delete(m, seg.key)
		return m, true
	}
	next, removed := removeValue(m[seg.key], segments[1:])
	if !removed {
		return current, false
	}
	m[seg.key] = next
	return m, true
}

func mergeMaps(base map[string]any, overrides map[string]any) map[string]any {
	if overrides == nil {
		return base
	}
	for key, override := range overrides {
		if overrideMap, ok := override.(map[string]any); ok {
			if baseMap, ok := base[key].(map[string]any); ok {
				base[key] = mergeMaps(baseMap, overrideMap)
				continue
			}
			base[key] = cloneValue(overrideMap)
			continue
		}
		base[key] = cloneValue(override)
	}
	return base
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, val := range value {
		out[key] = cloneValue(val)
	}
	return out
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[key] = cloneValue(val)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = cloneValue(val)
		}
		return out
	default:
		return v
	}
}

func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := osMkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := osCreateTemp(dir, "config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = fileClose(tmp)
		_ = os.Remove(tmpName)
	}()
	if _, err := fileWrite(tmp, data); err != nil {
		return err
	}
	if err := fileSync(tmp); err != nil {
		return err
	}
	if err := fileClose(tmp); err != nil {
		return err
	}
	if err := osRename(tmpName, path); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	handle, err := osOpen(dir)
	if err != nil {
		return err
	}
	defer fileClose(handle)
	if err := fileSync(handle); err != nil {
		return nil
	}
	return nil
}
