package resolver

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// CacheRecord is the closed, checksummed resolution-cache envelope shared by
// persistent adapters. Requirements:
// docs/architecture/runtime-v2-persistence-structure.md.
type CacheRecord struct {
	record *cacheRecord
}

// NewCacheRecord constructs a cache record after validating the complete key
// and resolution. All caller-owned byte slices are copied.
func NewCacheRecord(key CacheKey, resolution Resolution) (CacheRecord, error) {
	wire := cacheRecord{
		SchemaVersion:  cacheSchema,
		Key:            key.digest,
		WorkspaceScope: key.workspaceScope,
		Descriptor:     key.descriptor,
		Declaration:    append(json.RawMessage(nil), key.declaration...),
		Resolution:     cloneResolution(resolution),
	}
	if err := validateCacheRecord(wire, true); err != nil {
		return CacheRecord{}, ErrInvalidDeclaration
	}
	checksum, _ := cacheChecksum(wire)
	wire.Checksum = checksum
	return CacheRecord{record: &wire}, nil
}

// DecodeCacheRecordJSON strictly decodes and validates an untrusted cache
// envelope. Unsupported format versions are distinguished from corruption.
func DecodeCacheRecordJSON(raw []byte) (CacheRecord, error) {
	if len(raw) > runtimeCacheMaxBytes || rejectDuplicateJSON(raw) != nil {
		return CacheRecord{}, ErrCorruptCache
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire cacheRecord
	if err := decoder.Decode(&wire); err != nil {
		return CacheRecord{}, ErrCorruptCache
	}
	if wire.SchemaVersion == "" {
		return CacheRecord{}, ErrCorruptCache
	}
	if wire.SchemaVersion != cacheSchema {
		return CacheRecord{}, ErrIncompatibleCache
	}
	want := wire.Checksum
	wire.Checksum = ""
	digest, err := cacheChecksum(wire)
	wire.Checksum = want
	if err != nil || want != digest {
		return CacheRecord{}, ErrCorruptCache
	}
	if err := validateCacheRecord(wire, false); err != nil {
		return CacheRecord{}, ErrCorruptCache
	}
	copy := cloneCacheRecord(wire)
	return CacheRecord{record: &copy}, nil
}

// Matches reports whether every identity-bearing cache-key constituent is the
// same as the one captured by this record.
func (r CacheRecord) Matches(key CacheKey) bool {
	if r.record == nil {
		return false
	}
	return r.record.Key == key.digest &&
		r.record.WorkspaceScope == key.workspaceScope &&
		r.record.Descriptor == key.descriptor &&
		bytes.Equal(r.record.Declaration, key.declaration)
}

// Resolution returns a defensive copy of the cached resolution.
func (r CacheRecord) Resolution() Resolution {
	if r.record == nil {
		return Resolution{}
	}
	return cloneResolution(r.record.Resolution)
}

// MarshalJSON emits the existing v0.2.0 wire representation byte-for-byte.
func (r CacheRecord) MarshalJSON() ([]byte, error) {
	if r.record == nil {
		return nil, ErrInvalidDeclaration
	}
	return json.Marshal(*r.record)
}

func cloneCacheRecord(value cacheRecord) cacheRecord {
	value.Declaration = append(json.RawMessage(nil), value.Declaration...)
	value.Resolution = cloneResolution(value.Resolution)
	return value
}

func validateCacheRecord(value cacheRecord, withoutChecksum bool) error {
	if value.SchemaVersion != cacheSchema || !validCacheDigest(value.Key) ||
		!validCacheDigest(value.WorkspaceScope) || !validDescriptor(value.Descriptor) ||
		len(value.Declaration) == 0 || !json.Valid(value.Declaration) ||
		bytes.Equal(bytes.TrimSpace(value.Declaration), []byte("null")) {
		return ErrCorruptCache
	}
	declaration, err := decodeCacheDeclaration(value.Declaration, value.Descriptor.Role)
	if err != nil || declaration.Owner() != value.Descriptor.Owner ||
		declaration.Kind() != value.Descriptor.Kind ||
		declaration.SpecificationSchema() != value.Descriptor.SpecificationSchema {
		return ErrCorruptCache
	}
	identityJSON, err := json.Marshal(value.Resolution.Identity)
	if err != nil {
		return ErrCorruptCache
	}
	var identity runtimev2.ResolvedExtensionIdentity
	if err := json.Unmarshal(identityJSON, &identity); err != nil {
		return ErrCorruptCache
	}
	if len(value.Resolution.Evidence) == 0 || !json.Valid(value.Resolution.Evidence) ||
		bytes.Equal(bytes.TrimSpace(value.Resolution.Evidence), []byte("null")) {
		return ErrCorruptCache
	}
	if !withoutChecksum && value.Checksum == "" {
		return ErrCorruptCache
	}
	return validateCacheKeyDigest(value)
}

func decodeCacheDeclaration(raw []byte, role string) (runtimev2.ExtensionDeclaration, error) {
	switch role {
	case "input":
		var value runtimev2.InputDeclaration
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		return value, nil
	case "execution_environment":
		var value runtimev2.ExecutionEnvironmentDeclaration
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		return value, nil
	case "deployment":
		var value runtimev2.DeploymentDeclaration
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, ErrCorruptCache
	}
}

func validCacheDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validateCacheKeyDigest(value cacheRecord) error {
	scope, err := hex.DecodeString(value.WorkspaceScope)
	if err != nil || len(scope) != sha256.Size {
		return ErrCorruptCache
	}
	parts := [][]byte{
		[]byte(cacheSchema), scope, []byte(value.Descriptor.Role),
		[]byte(value.Descriptor.Owner), []byte(value.Descriptor.Kind),
		[]byte(value.Descriptor.SpecificationSchema),
		[]byte(value.Descriptor.SemanticVersion), value.Declaration,
	}
	var preimage bytes.Buffer
	for _, part := range parts {
		_ = binary.Write(&preimage, binary.BigEndian, uint64(len(part)))
		_, _ = preimage.Write(part)
	}
	digest := sha256.Sum256(preimage.Bytes())
	if value.Key != hex.EncodeToString(digest[:]) {
		return ErrCorruptCache
	}
	return nil
}
