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
)

const cacheSchema = "sqlrs.resolution-cache.v1"

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
	Resolution     Resolution      `json:"resolution"`
	Checksum       string          `json:"checksum,omitempty"`
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

func (c *DirectoryCache) Load(ctx context.Context, key CacheKey) (CacheLoad, error) {
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
	var record cacheRecord
	if err := parseCacheRecord(raw, &record); err != nil {
		return CacheLoad{}, err
	}
	if record.Key != key.String() ||
		record.WorkspaceScope != key.workspaceScope || record.Descriptor != key.descriptor ||
		!bytes.Equal(record.Declaration, key.declaration) {
		return CacheLoad{}, ErrCorruptCache
	}
	return CacheLoad{Hit: true, Resolution: cloneResolution(record.Resolution)}, nil
}

func parseCacheRecord(raw []byte, record *cacheRecord) error {
	if rejectDuplicateJSON(raw) != nil {
		return ErrCorruptCache
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(record); err != nil {
		return ErrCorruptCache
	}
	if record.SchemaVersion != cacheSchema {
		return ErrIncompatibleCache
	}
	want := record.Checksum
	record.Checksum = ""
	digest, _ := cacheChecksum(*record)
	record.Checksum = want
	if want != digest {
		return ErrCorruptCache
	}
	return nil
}

func (c *DirectoryCache) Store(ctx context.Context, key CacheKey, resolution Resolution) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	record := cacheRecord{SchemaVersion: cacheSchema, Key: key.String(), WorkspaceScope: key.workspaceScope,
		Descriptor: key.descriptor, Declaration: append(json.RawMessage(nil), key.declaration...), Resolution: cloneResolution(resolution)}
	checksum, err := cacheChecksum(record)
	if err != nil {
		return err
	}
	record.Checksum = checksum
	// cacheChecksum has just marshalled the same immutable record successfully;
	// adding a fixed-format checksum string cannot introduce a marshal failure.
	raw, _ := json.Marshal(record)
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
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(name, c.path(key)); err != nil {
		return err
	}
	committed = true
	return syncDirectory(c.root)
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
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var scan func() error
	scan = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
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
				if err := scan(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := scan(); err != nil {
					return err
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
		return ErrCorruptCache
	}
	return nil
}
