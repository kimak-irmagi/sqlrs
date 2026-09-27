// Package runtimev2store defines the engine-owned Runtime v2 persistence
// boundary. Requirements: docs/architecture/runtime-v2-persistence-structure.md.
package runtimev2store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

const (
	RecordVersion    = "sqlrs.runtime-persistence.v1"
	MaxMetadataBytes = 64 << 10
	MaxComponents    = 256
	MaxLocatorBytes  = 4096
)

var (
	ErrInvalid       = errors.New("runtime v2 store value invalid")
	identifierRegexp = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
)

type MaterializationInput struct {
	StateID           runtimev2.StateID
	MaterializationID string
	Backend           string
	RuntimeID         *string
	JobID             *string
	CreatedAt         time.Time
	SizeBytes         *int64
	Metadata          json.RawMessage
	Components        []MaterializationComponentInput
}

type MaterializationComponentInput struct {
	Name, Kind, Locator string
	SizeBytes           *int64
	Metadata            json.RawMessage
}

type materializationData struct {
	stateID          runtimev2.StateID
	id, backend      string
	runtimeID, jobID *string
	createdAt        string
	sizeBytes        *int64
	metadata         json.RawMessage
	components       []MaterializationComponent
}

type Materialization struct{ data *materializationData }

type MaterializationComponent struct {
	name, kind, locator string
	sizeBytes           *int64
	metadata            json.RawMessage
}

func NewMaterialization(input MaterializationInput) (Materialization, error) {
	if !validStateID(input.StateID) || !validOpaque(input.MaterializationID, 256) ||
		!identifierRegexp.MatchString(input.Backend) || input.CreatedAt.IsZero() ||
		!validOptional(input.RuntimeID, 256) || !validOptional(input.JobID, 256) ||
		!validSize(input.SizeBytes) || len(input.Components) > MaxComponents {
		return Materialization{}, ErrInvalid
	}
	metadata, err := canonicalMetadata(input.Metadata)
	if err != nil {
		return Materialization{}, err
	}
	components := make([]MaterializationComponent, len(input.Components))
	seen := make(map[string]struct{}, len(input.Components))
	for index, component := range input.Components {
		if !identifierRegexp.MatchString(component.Name) || !identifierRegexp.MatchString(component.Kind) ||
			!validOpaque(component.Locator, MaxLocatorBytes) || !validSize(component.SizeBytes) {
			return Materialization{}, ErrInvalid
		}
		if _, exists := seen[component.Name]; exists {
			return Materialization{}, ErrInvalid
		}
		seen[component.Name] = struct{}{}
		componentMetadata, metadataErr := canonicalMetadata(component.Metadata)
		if metadataErr != nil {
			return Materialization{}, metadataErr
		}
		components[index] = MaterializationComponent{
			name: component.Name, kind: component.Kind, locator: component.Locator,
			sizeBytes: copyInt64(component.SizeBytes), metadata: componentMetadata,
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i].name < components[j].name })
	return Materialization{data: &materializationData{
		stateID: input.StateID, id: input.MaterializationID, backend: input.Backend,
		runtimeID: copyString(input.RuntimeID), jobID: copyString(input.JobID),
		createdAt: input.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		sizeBytes: copyInt64(input.SizeBytes), metadata: metadata, components: components,
	}}, nil
}

func (m Materialization) RecordVersion() string {
	if m.data == nil {
		return ""
	}
	return RecordVersion
}
func (m Materialization) StateID() runtimev2.StateID {
	if m.data == nil {
		return ""
	}
	return m.data.stateID
}
func (m Materialization) MaterializationID() string {
	if m.data == nil {
		return ""
	}
	return m.data.id
}
func (m Materialization) Backend() string {
	if m.data == nil {
		return ""
	}
	return m.data.backend
}
func (m Materialization) RuntimeID() *string {
	if m.data == nil {
		return nil
	}
	return copyString(m.data.runtimeID)
}
func (m Materialization) JobID() *string {
	if m.data == nil {
		return nil
	}
	return copyString(m.data.jobID)
}
func (m Materialization) CreatedAt() string {
	if m.data == nil {
		return ""
	}
	return m.data.createdAt
}
func (m Materialization) SizeBytes() *int64 {
	if m.data == nil {
		return nil
	}
	return copyInt64(m.data.sizeBytes)
}
func (m Materialization) Metadata() json.RawMessage {
	if m.data == nil {
		return nil
	}
	return append(json.RawMessage(nil), m.data.metadata...)
}
func (m Materialization) Components() []MaterializationComponent {
	if m.data == nil {
		return nil
	}
	out := make([]MaterializationComponent, len(m.data.components))
	for i, value := range m.data.components {
		out[i] = value.clone()
	}
	return out
}
func (m Materialization) Equal(other Materialization) bool {
	if m.data == nil || other.data == nil {
		return m.data == nil && other.data == nil
	}
	a, _ := json.Marshal(materializationComparable(m))
	b, _ := json.Marshal(materializationComparable(other))
	return bytes.Equal(a, b)
}

func (c MaterializationComponent) Name() string      { return c.name }
func (c MaterializationComponent) Kind() string      { return c.kind }
func (c MaterializationComponent) Locator() string   { return c.locator }
func (c MaterializationComponent) SizeBytes() *int64 { return copyInt64(c.sizeBytes) }
func (c MaterializationComponent) Metadata() json.RawMessage {
	return append(json.RawMessage(nil), c.metadata...)
}
func (c MaterializationComponent) clone() MaterializationComponent {
	c.sizeBytes = copyInt64(c.sizeBytes)
	c.metadata = append(json.RawMessage(nil), c.metadata...)
	return c
}

func validStateID(value runtimev2.StateID) bool {
	s := string(value)
	if len(s) != 71 || !strings.HasPrefix(s, "sha256:") {
		return false
	}
	for _, char := range s[7:] {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}
func validOpaque(value string, maximum int) bool {
	if value == "" || !utf8.ValidString(value) || len(value) > maximum {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}
func validOptional(value *string, maximum int) bool {
	return value == nil || validOpaque(*value, maximum)
}
func validSize(value *int64) bool { return value == nil || *value >= 0 }
func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func canonicalMetadata(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if len(raw) > MaxMetadataBytes || rejectDuplicateObjectKeys(raw) != nil {
		return nil, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > MaxMetadataBytes {
		return nil, ErrInvalid
	}
	return canonical, nil
}

func rejectDuplicateObjectKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var scan func() error
	scan = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, e := decoder.Token()
				if e != nil {
					return e
				}
				key := keyToken.(string)
				if _, exists := seen[key]; exists {
					return ErrInvalid
				}
				seen[key] = struct{}{}
				if e = scan(); e != nil {
					return e
				}
			}
		case '[':
			for decoder.More() {
				if e := scan(); e != nil {
					return e
				}
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := scan(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

func materializationComparable(value Materialization) any {
	type component struct {
		Name, Kind, Locator string
		SizeBytes           *int64
		Metadata            json.RawMessage
	}
	type materialization struct {
		StateID          runtimev2.StateID
		ID, Backend      string
		RuntimeID, JobID *string
		CreatedAt        string
		SizeBytes        *int64
		Metadata         json.RawMessage
		Components       []component
	}
	components := value.Components()
	out := make([]component, len(components))
	for i, item := range components {
		out[i] = component{item.Name(), item.Kind(), item.Locator(), item.SizeBytes(), item.Metadata()}
	}
	return materialization{value.StateID(), value.MaterializationID(), value.Backend(), value.RuntimeID(), value.JobID(), value.CreatedAt(), value.SizeBytes(), value.Metadata(), out}
}
