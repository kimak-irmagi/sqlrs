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
	record     *cacheRecord
	resolution Resolution
}

// NewCacheRecord constructs a cache record after validating the complete key
// and resolution. All caller-owned byte slices are copied.
func NewCacheRecord(key CacheKey, schema runtimev2.ExtensionIdentitySchema, resolution Resolution) (CacheRecord, error) {
	if !schema.Valid() || schema.Provider() != key.descriptor.Owner || schema.SemanticKind() != key.descriptor.Kind {
		return CacheRecord{}, ErrInvalidDeclaration
	}
	if rejectDuplicateJSON(resolution.Evidence) != nil {
		return CacheRecord{}, ErrInvalidDeclaration
	}
	identity, err := runtimev2.MarshalCanonicalExtensionIdentityJSON(resolution.Identity, schema)
	if err != nil {
		return CacheRecord{}, ErrInvalidDeclaration
	}
	wire := cacheRecord{
		SchemaVersion:  cacheSchema,
		Key:            key.digest,
		WorkspaceScope: key.workspaceScope,
		Descriptor:     key.descriptor,
		Declaration:    append(json.RawMessage(nil), key.declaration...),
		Resolution:     cacheResolution{Identity: identity, Evidence: append(json.RawMessage(nil), resolution.Evidence...)},
	}
	if err := validateCacheRecord(wire, true); err != nil {
		return CacheRecord{}, ErrInvalidDeclaration
	}
	checksum, _ := cacheChecksum(wire)
	wire.Checksum = checksum
	raw, err := json.Marshal(wire)
	if err != nil || len(raw) > runtimeCacheMaxBytes || rejectDuplicateJSON(raw) != nil {
		return CacheRecord{}, ErrInvalidDeclaration
	}
	return CacheRecord{record: &wire, resolution: cloneResolution(resolution)}, nil
}

// DecodeCacheRecordJSON strictly decodes and validates an untrusted cache
// envelope. Unsupported format versions are distinguished from corruption.
func DecodeCacheRecordJSON(raw []byte, schema runtimev2.ExtensionIdentitySchema) (CacheRecord, error) {
	decoded, err := decodeCacheEnvelope(raw)
	if err != nil {
		return CacheRecord{}, err
	}
	if !schema.Valid() || schema.Provider() != decoded.record.Descriptor.Owner || schema.SemanticKind() != decoded.record.Descriptor.Kind {
		return CacheRecord{}, ErrCorruptCache
	}
	var identityVersion struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(decoded.record.Resolution.Identity, &identityVersion); err != nil || identityVersion.SchemaVersion == "" {
		return CacheRecord{}, ErrCorruptCache
	}
	if identityVersion.SchemaVersion != runtimev2.CanonicalSchemaVersion {
		return CacheRecord{}, ErrIncompatibleCache
	}
	identity, err := runtimev2.DecodeCanonicalExtensionIdentityJSON(decoded.record.Resolution.Identity, schema)
	if err != nil {
		return CacheRecord{}, ErrCorruptCache
	}
	decoded.resolution = Resolution{Identity: identity, Evidence: append(json.RawMessage(nil), decoded.record.Resolution.Evidence...)}
	return decoded, nil
}

// decodeCacheEnvelope validates the provider-independent boundary for loading
// and pruning; it does not claim to validate schema-bound identity fields.
func decodeCacheEnvelope(raw []byte) (CacheRecord, error) {
	if len(raw) > runtimeCacheMaxBytes || rejectDuplicateJSON(raw) != nil {
		return CacheRecord{}, ErrCorruptCache
	}
	root, err := cacheObjectMembers(raw, "schema_version", "key", "workspace_scope", "resolver", "normalized_declaration", "resolution", "checksum")
	if err != nil {
		return CacheRecord{}, err
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
	if err := validateCacheMemberSpelling(root); err != nil {
		return CacheRecord{}, err
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

// validateCacheMemberSpelling enforces the closed envelope's exact JSON keys.
// Provider evidence and schema-bound identity are checked at their own layers.
func validateCacheMemberSpelling(root map[string]json.RawMessage) error {
	if _, err := cacheObjectMembers(root["resolver"], "role", "owner", "kind", "specification_schema", "semantic_version"); err != nil {
		return err
	}
	if _, err := cacheObjectMembers(root["resolution"], "identity", "evidence"); err != nil {
		return err
	}
	declaration, err := cacheObjectMembers(root["normalized_declaration"], "schema_version", "owner", "kind", "specification_schema", "fields")
	if err != nil {
		return err
	}
	var fields []json.RawMessage
	if err := json.Unmarshal(declaration["fields"], &fields); err != nil {
		return ErrCorruptCache
	}
	for _, field := range fields {
		if _, err := cacheObjectMembers(field, "name", "value"); err != nil {
			return err
		}
	}
	return nil
}

func cacheObjectMembers(raw []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil || members == nil {
		return nil, ErrCorruptCache
	}
	permitted := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		permitted[name] = true
	}
	for name := range members {
		if !permitted[name] {
			return nil, ErrCorruptCache
		}
	}
	return members, nil
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
	return cloneResolution(r.resolution)
}

// MarshalJSON emits the canonical v0.5.0 cache wire representation.
func (r CacheRecord) MarshalJSON() ([]byte, error) {
	if r.record == nil {
		return nil, ErrInvalidDeclaration
	}
	return json.Marshal(*r.record)
}

func cloneCacheRecord(value cacheRecord) cacheRecord {
	value.Declaration = append(json.RawMessage(nil), value.Declaration...)
	value.Resolution.Identity = append(json.RawMessage(nil), value.Resolution.Identity...)
	value.Resolution.Evidence = append(json.RawMessage(nil), value.Resolution.Evidence...)
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
	if len(value.Resolution.Identity) == 0 || !json.Valid(value.Resolution.Identity) ||
		bytes.Equal(bytes.TrimSpace(value.Resolution.Identity), []byte("null")) {
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
