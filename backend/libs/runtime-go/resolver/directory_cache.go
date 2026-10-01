package resolver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

const cacheSchema = "sqlrs.resolution-cache.canonical.v1"

// syncCacheDirectory is a narrow durability seam for injected failure tests.
// Production calls the platform-specific directory sync implementation.
var syncCacheDirectory = syncDirectory

// DirectoryCache is a strict restart-safe cache rooted outside the workspace.
type DirectoryCache struct {
	root string
	mu   sync.Mutex
}

type cacheRecord struct {
	SchemaVersion  string          `json:"schema_version"`
	Key            string          `json:"key"`
	WorkspaceScope string          `json:"workspace_scope"`
	Descriptor     Descriptor      `json:"resolver"`
	Declaration    json.RawMessage `json:"normalized_declaration"`
	Resolution     cacheResolution `json:"resolution"`
	Checksum       string          `json:"checksum,omitempty"`
}

type cacheResolution struct {
	Identity json.RawMessage `json:"identity"`
	Evidence json.RawMessage `json:"evidence"`
}

// NewDirectoryCache creates or validates an owner-only cache root.
func NewDirectoryCache(root string) (*DirectoryCache, error) {
	if root == "" {
		return nil, ErrCorruptCache
	}
	abs, err := preparePrivateDirectory(root)
	if err != nil {
		return nil, err
	}
	return &DirectoryCache{root: abs}, nil
}

func (c *DirectoryCache) path(key CacheKey) string {
	return filepath.Join(c.root, key.String()+".json")
}

func (c *DirectoryCache) Load(ctx context.Context, key CacheKey, schema runtimev2.ExtensionIdentitySchema) (CacheLoad, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return CacheLoad{}, err
	}
	path := c.path(key)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return CacheLoad{}, nil
	}
	if err != nil {
		return CacheLoad{}, err
	}
	if isLinkLike(info) || !info.Mode().IsRegular() {
		return CacheLoad{}, ErrUnsafePath
	}
	if info.Size() > runtimeCacheMaxBytes {
		return CacheLoad{}, ErrCorruptCache
	}
	file, err := os.Open(path)
	if err != nil {
		return CacheLoad{}, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, runtimeCacheMaxBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return CacheLoad{}, readErr
	}
	if closeErr != nil {
		return CacheLoad{}, closeErr
	}
	if len(raw) > runtimeCacheMaxBytes {
		return CacheLoad{}, ErrCorruptCache
	}
	record, err := DecodeCacheRecordJSON(raw, schema)
	if err != nil {
		return CacheLoad{}, err
	}
	if !record.Matches(key) {
		return CacheLoad{}, ErrCorruptCache
	}
	return CacheLoad{Hit: true, Resolution: record.Resolution()}, nil
}

func parseCacheRecord(raw []byte, record *cacheRecord) error {
	decoded, err := decodeCacheEnvelope(raw)
	if err != nil {
		return err
	}
	*record = cloneCacheRecord(*decoded.record)
	return nil
}

func (c *DirectoryCache) Store(ctx context.Context, key CacheKey, schema runtimev2.ExtensionIdentitySchema, resolution Resolution) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	record, err := NewCacheRecord(key, schema, resolution)
	if err != nil {
		return err
	}
	raw, _ := record.MarshalJSON()
	if len(raw) > runtimeCacheMaxBytes {
		return ErrInvalidDeclaration
	}
	temporary, err := os.CreateTemp(c.root, ".tmp-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	publicationBarrier("cache", publicationTemporaryCreated)
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	publicationBarrier("cache", publicationContentWritten)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	publicationBarrier("cache", publicationFileSynced)
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(name, c.path(key)); err != nil {
		return err
	}
	committed = true
	publicationBarrier("cache", publicationReplaced)
	if err := syncCacheDirectory(c.root); err != nil {
		return err
	}
	publicationBarrier("cache", publicationDirectorySynced)
	return nil
}

func cacheChecksum(record cacheRecord) (string, error) {
	record.Checksum = ""
	raw, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func cloneResolution(value Resolution) Resolution {
	value.Evidence = append(json.RawMessage(nil), value.Evidence...)
	return value
}

const runtimeCacheMaxBytes = 4 << 20

func rejectDuplicateJSON(raw []byte) error {
	if !utf8.Valid(raw) || rejectUnpairedSurrogates(raw) != nil {
		return ErrCorruptCache
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var scan func(int) error
	scan = func(depth int) error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if depth >= 64 {
			return ErrCorruptCache
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				// encoding/json emits only string tokens for object keys.
				key := keyToken.(string)
				if _, exists := seen[key]; exists {
					return ErrCorruptCache
				}
				seen[key] = struct{}{}
				if err := scan(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := scan(depth + 1); err != nil {
					return err
				}
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := scan(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrCorruptCache
	}
	return nil
}

// rejectUnpairedSurrogates keeps encoding/json's replacement behavior from
// silently changing a cache string before checksum or semantic validation.
func rejectUnpairedSurrogates(raw []byte) error {
	inside, escaped := false, false
	for index := 0; index < len(raw); index++ {
		ch := raw[index]
		if !inside {
			if ch == '"' {
				inside = true
			}
			continue
		}
		if escaped {
			escaped = false
			if ch != 'u' {
				continue
			}
			if index+4 >= len(raw) {
				return ErrCorruptCache
			}
			first, ok := cacheHex4(raw[index+1 : index+5])
			if !ok {
				return ErrCorruptCache
			}
			index += 4
			if first >= 0xdc00 && first <= 0xdfff {
				return ErrCorruptCache
			}
			if first >= 0xd800 && first <= 0xdbff {
				if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
					return ErrCorruptCache
				}
				second, ok := cacheHex4(raw[index+3 : index+7])
				if !ok || second < 0xdc00 || second > 0xdfff {
					return ErrCorruptCache
				}
				index += 6
			}
			continue
		}
		if ch == '\\' {
			escaped = true
		} else if ch == '"' {
			inside = false
		}
	}
	return nil
}

func cacheHex4(raw []byte) (uint16, bool) {
	var result uint16
	for _, ch := range raw {
		result <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			result |= uint16(ch - '0')
		case ch >= 'a' && ch <= 'f':
			result |= uint16(ch - 'a' + 10)
		case ch >= 'A' && ch <= 'F':
			result |= uint16(ch - 'A' + 10)
		default:
			return 0, false
		}
	}
	return result, true
}
