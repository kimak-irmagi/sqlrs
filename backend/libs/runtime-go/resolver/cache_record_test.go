package resolver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

// These tests specify the canonical cache envelope described by
// docs/architecture/runtime-v2-canonical-resolver-structure.md.
func TestCacheRecordRejectsLegacyWire(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "resolution-cache-v0.2.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.TrimSpace(raw)
	if _, err := DecodeCacheRecordJSON(raw, coverageSchema()); !errors.Is(err, ErrIncompatibleCache) {
		t.Fatalf("legacy record = %v, want incompatible", err)
	}
}

// CR07: fixed cache bytes, key, and checksum come from a separate Node
// crypto/Buffer calculation of the normative wire grammar. The synthetic
// workspace scope is SHA-256 of "/workspace", independent of the host OS.
func TestCanonicalCacheRecordIndependentWire(t *testing.T) {
	const expected = `{"schema_version":"sqlrs.resolution-cache.canonical.v1","key":"2d9dbd344b61347b74cc4b4ad98dae7ce61f046373c1eb7a2c6763f4222ebb08","workspace_scope":"c52ddf65534b7b46035084358ab7902be4bfef220bdb503ac7039cc861905b05","resolver":{"role":"input","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1","semantic_version":"1"},"normalized_declaration":{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1","fields":[]},"resolution":{"identity":{"schema_version":"sqlrs.runtime.v2.canonical.v1","provider":"owner","kind":"kind","identity_schema":"owner.kind.v1","fields":[],"canonical_bytes":"AAAAAAAAADBzcWxycy5ydW50aW1lLnYyLmNhbm9uaWNhbC52MS9yZXNvbHZlZC1leHRlbnNpb24AAAAFAAEAAAAAAAAAHXNxbHJzLnJ1bnRpbWUudjIuY2Fub25pY2FsLnYxAAIAAAAAAAAABW93bmVyAAMAAAAAAAAABGtpbmQABAAAAAAAAAANb3duZXIua2luZC52MQAFAAAAAAAAAAQAAAAA","fingerprint":"sha256:403df2fd592b9ce0fd62769eac1a7ebb96648a8c05197c57c8a1ae295bdd1dfb"},"evidence":{}},"checksum":"sha256:c793bcd55a9597ad784d3f6f3a05c60d1d5cce032df4496ae95e65f932e3fa58"}`
	key := CacheKey{
		digest:         "2d9dbd344b61347b74cc4b4ad98dae7ce61f046373c1eb7a2c6763f4222ebb08",
		workspaceScope: "c52ddf65534b7b46035084358ab7902be4bfef220bdb503ac7039cc861905b05",
		descriptor:     Descriptor{Role: "input", Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", SemanticVersion: "1"},
		declaration:    json.RawMessage(`{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1","fields":[]}`),
	}
	record, err := NewCacheRecord(key, coverageSchema(), coverageResolution(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := record.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != expected {
		t.Fatalf("cache wire disagrees with independent fixture\nactual: %s\nexpected: %s", raw, expected)
	}
	decoded, err := DecodeCacheRecordJSON([]byte(expected), coverageSchema())
	if err != nil || !decoded.Matches(key) {
		t.Fatalf("independent fixture rejected: %v", err)
	}
}

func TestCacheRecordSupportsEveryDeclarationRole(t *testing.T) {
	input := runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", Fields: []runtimev2.DeclarationField{}}
	inputDeclaration, err := runtimev2.NewInputDeclaration(input)
	if err != nil {
		t.Fatal(err)
	}
	executionDeclaration, err := runtimev2.NewExecutionEnvironmentDeclaration(input)
	if err != nil {
		t.Fatal(err)
	}
	deploymentDeclaration, err := runtimev2.NewDeploymentDeclaration(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range []runtimev2.ExtensionDeclaration{inputDeclaration, executionDeclaration, deploymentDeclaration} {
		t.Run(declaration.Role(), func(t *testing.T) {
			key, err := NewCacheKey(Workspace{Root: t.TempDir()}, Descriptor{Role: declaration.Role(), Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", SemanticVersion: "1"}, NormalizedDeclaration{Declaration: declaration})
			if err != nil {
				t.Fatal(err)
			}
			record, err := NewCacheRecord(key, coverageSchema(), coverageResolution(t))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := record.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeCacheRecordJSON(raw, coverageSchema())
			if err != nil || !decoded.Matches(key) {
				t.Fatalf("round trip = %v, matches=%v", err, decoded.Matches(key))
			}
		})
	}
}

func TestCacheRecordConstructionRoundTripAndDefensiveCopies(t *testing.T) {
	key, resolution := coverageKey(t, "1")
	inputEvidence := resolution.Evidence
	record, err := NewCacheRecord(key, coverageSchema(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	inputEvidence[0] = '['
	if !record.Matches(key) {
		t.Fatal("new record did not match its key")
	}

	raw, err := record.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCacheRecordJSON(raw, coverageSchema())
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Matches(key) {
		t.Fatal("decoded record did not match its key")
	}
	first := decoded.Resolution()
	if !bytes.Equal(first.Evidence, []byte(`{}`)) {
		t.Fatalf("evidence = %s", first.Evidence)
	}
	first.Evidence[0] = '['
	if got := decoded.Resolution().Evidence; !bytes.Equal(got, []byte(`{}`)) {
		t.Fatalf("record was mutated through Resolution: %s", got)
	}
	again, err := decoded.MarshalJSON()
	if err != nil || !bytes.Equal(again, raw) {
		t.Fatalf("serialization changed after accessor mutation: %v\n%s\n%s", err, again, raw)
	}
}

func TestCacheRecordMatchesEveryKeyConstituent(t *testing.T) {
	key, resolution := coverageKey(t, "1")
	record, err := NewCacheRecord(key, coverageSchema(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(CacheKey) CacheKey{
		"digest":               func(k CacheKey) CacheKey { k.digest = "different"; return k },
		"workspace":            func(k CacheKey) CacheKey { k.workspaceScope = "different"; return k },
		"role":                 func(k CacheKey) CacheKey { k.descriptor.Role = "deployment"; return k },
		"owner":                func(k CacheKey) CacheKey { k.descriptor.Owner = "other"; return k },
		"kind":                 func(k CacheKey) CacheKey { k.descriptor.Kind = "other"; return k },
		"specification schema": func(k CacheKey) CacheKey { k.descriptor.SpecificationSchema = "other.v1"; return k },
		"resolver version":     func(k CacheKey) CacheKey { k.descriptor.SemanticVersion = "2"; return k },
		"declaration":          func(k CacheKey) CacheKey { k.declaration = json.RawMessage(`{}`); return k },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if record.Matches(mutate(key)) {
				t.Fatal("record matched a changed key")
			}
		})
	}
}

func TestCacheRecordRejectsInvalidConstruction(t *testing.T) {
	key, resolution := coverageKey(t, "1")
	wrongSchema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: "other", SemanticKind: "kind", IdentitySchema: "owner.kind.v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCacheRecord(key, wrongSchema, resolution); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("mismatched schema = %v", err)
	}
	otherDeclaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "other", Kind: "kind", SpecificationSchema: "other.kind.v1", Fields: []runtimev2.DeclarationField{}})
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := NewCacheKey(Workspace{Root: t.TempDir()}, Descriptor{Role: "input", Owner: "other", Kind: "kind", SpecificationSchema: "other.kind.v1", SemanticVersion: "1"}, NormalizedDeclaration{Declaration: otherDeclaration})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCacheRecord(otherKey, coverageSchema(), resolution); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("schema disagrees with descriptor = %v", err)
	}
	for name, change := range map[string]func(*CacheKey, *Resolution){
		"zero key":         func(key *CacheKey, _ *Resolution) { *key = CacheKey{} },
		"zero resolution":  func(_ *CacheKey, resolution *Resolution) { *resolution = Resolution{} },
		"invalid evidence": func(_ *CacheKey, resolution *Resolution) { resolution.Evidence = []byte(`{`) },
	} {
		t.Run(name, func(t *testing.T) {
			candidateKey, candidateResolution := key, resolution
			change(&candidateKey, &candidateResolution)
			if _, err := NewCacheRecord(candidateKey, coverageSchema(), candidateResolution); !errors.Is(err, ErrInvalidDeclaration) {
				t.Fatalf("error = %v, want %v", err, ErrInvalidDeclaration)
			}
		})
	}
}

func TestCacheRecordZeroValueAndMalformedDeclaration(t *testing.T) {
	var zero CacheRecord
	if zero.Matches(CacheKey{}) {
		t.Fatal("zero record matched")
	}
	if zero.Resolution().Identity.Provider() != "" {
		t.Fatal("zero record returned resolution")
	}
	if _, err := zero.MarshalJSON(); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("zero marshal=%v", err)
	}
	key, resolution := coverageKey(t, "1")
	key.declaration = json.RawMessage(`{}`)
	if _, err := NewCacheRecord(key, coverageSchema(), resolution); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("malformed declaration=%v", err)
	}
	key, _ = coverageKey(t, "1")
	key.descriptor.Owner = "other"
	if _, err := NewCacheRecord(key, coverageSchema(), resolution); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("descriptor mismatch=%v", err)
	}
}

func TestDecodeCacheRecordJSONTrustBoundary(t *testing.T) {
	key, resolution := coverageKey(t, "1")
	record, err := NewCacheRecord(key, coverageSchema(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := record.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var envelope cacheRecord
	if err := json.Unmarshal(valid, &envelope); err != nil {
		t.Fatal(err)
	}
	type rawEnvelope struct {
		SchemaVersion  string          `json:"schema_version"`
		Key            string          `json:"key"`
		WorkspaceScope string          `json:"workspace_scope"`
		Descriptor     Descriptor      `json:"resolver"`
		Declaration    json.RawMessage `json:"normalized_declaration"`
		Resolution     json.RawMessage `json:"resolution"`
		Checksum       string          `json:"checksum,omitempty"`
	}
	malformedIdentity := func() []byte {
		var candidate rawEnvelope
		if unmarshalErr := json.Unmarshal(valid, &candidate); unmarshalErr != nil {
			t.Fatalf("decode valid envelope: %v: %q", unmarshalErr, valid)
		}
		candidate.Resolution = bytes.Replace(candidate.Resolution, []byte(`"provider":"owner"`), []byte(`"provider":""`), 1)
		candidate.Checksum = ""
		withoutChecksum, marshalErr := json.Marshal(candidate)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		digest := sha256.Sum256(withoutChecksum)
		candidate.Checksum = "sha256:" + hex.EncodeToString(digest[:])
		raw, marshalErr := json.Marshal(candidate)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return raw
	}
	reencode := func(change func(*cacheRecord)) []byte {
		candidate := envelope
		candidate.Declaration = append(json.RawMessage(nil), envelope.Declaration...)
		candidate.Resolution.Identity = append(json.RawMessage(nil), envelope.Resolution.Identity...)
		candidate.Resolution.Evidence = append(json.RawMessage(nil), envelope.Resolution.Evidence...)
		change(&candidate)
		candidate.Checksum = ""
		checksum, checksumErr := cacheChecksum(candidate)
		if checksumErr != nil {
			t.Fatal(checksumErr)
		}
		candidate.Checksum = checksum
		raw, marshalErr := json.Marshal(candidate)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return raw
	}

	cases := []struct {
		name string
		raw  []byte
		want error
	}{
		{"unknown member", append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"unknown":true}`)...), ErrCorruptCache},
		{"duplicate member", append([]byte(`{"schema_version":"sqlrs.resolution-cache.canonical.v1",`), valid[1:]...), ErrCorruptCache},
		{"case-variant member", bytes.Replace(valid, []byte(`"key":"`), []byte(`"Key":"`), 1), ErrCorruptCache},
		{"case-variant resolver member", bytes.Replace(valid, []byte(`"role":"input"`), []byte(`"Role":"input"`), 1), ErrCorruptCache},
		{"case-variant resolution member", bytes.Replace(valid, []byte(`"identity":{`), []byte(`"Identity":{`), 1), ErrCorruptCache},
		{"case-variant declaration member", reencode(func(candidate *cacheRecord) {
			candidate.Declaration = bytes.Replace(candidate.Declaration, []byte(`"owner":"owner"`), []byte(`"Owner":"owner"`), 1)
		}), ErrCorruptCache},
		{"case-variant identity member", reencode(func(candidate *cacheRecord) {
			candidate.Resolution.Identity = bytes.Replace(candidate.Resolution.Identity, []byte(`"provider":"owner"`), []byte(`"Provider":"owner"`), 1)
		}), ErrCorruptCache},
		{"null required member", bytes.Replace(valid, []byte(`"key":"`+key.String()+`"`), []byte(`"key":null`), 1), ErrCorruptCache},
		{"missing required member", reencode(func(candidate *cacheRecord) { candidate.Key = "" }), ErrCorruptCache},
		{"trailing token", append(append([]byte(nil), valid...), []byte(` {}`)...), ErrCorruptCache},
		{"oversized", bytes.Repeat([]byte(" "), runtimeCacheMaxBytes+1), ErrCorruptCache},
		{"checksum", bytes.Replace(valid, []byte(`"checksum":"sha256:`), []byte(`"checksum":"sha256:x`), 1), ErrCorruptCache},
		{"unsupported schema", reencode(func(candidate *cacheRecord) { candidate.SchemaVersion = "future" }), ErrIncompatibleCache},
		{"malformed identity", malformedIdentity(), ErrCorruptCache},
		{"invalid utf8", append(append([]byte(nil), valid[:len(valid)-1]...), []byte{',', '"', 'x', '"', ':', '"', 0xff, '"', '}'}...), ErrCorruptCache},
		{"unpaired surrogate", bytes.Replace(valid, []byte(`"schema_version":"`+cacheSchema+`"`), []byte(`"schema_version":"\ud800"`), 1), ErrCorruptCache},
		{"identity version", reencode(func(candidate *cacheRecord) {
			candidate.Resolution.Identity = bytes.Replace(candidate.Resolution.Identity, []byte(`"schema_version":"`+runtimev2.CanonicalSchemaVersion+`"`), []byte(`"schema_version":"future"`), 1)
		}), ErrIncompatibleCache},
		{"zero value", []byte(`{}`), ErrCorruptCache},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeCacheRecordJSON(test.raw, coverageSchema()); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestCacheRecordInternalValidationBranches(t *testing.T) {
	key, resolution := coverageKey(t, "1")
	record, err := NewCacheRecord(key, coverageSchema(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	envelope := cloneCacheRecord(*record.record)
	envelope.Checksum = ""
	if err := validateCacheRecord(envelope, false); !errors.Is(err, ErrCorruptCache) {
		t.Fatalf("missing checksum = %v", err)
	}

	for _, role := range []string{"execution_environment", "deployment"} {
		if _, err := decodeCacheDeclaration([]byte(`{`), role); err == nil {
			t.Fatalf("malformed %s declaration accepted", role)
		}
	}
	if _, err := decodeCacheDeclaration([]byte(`{}`), "future"); !errors.Is(err, ErrCorruptCache) {
		t.Fatalf("unknown role = %v", err)
	}

	malformedScope := envelope
	malformedScope.WorkspaceScope = "not-hex"
	if err := validateCacheKeyDigest(malformedScope); !errors.Is(err, ErrCorruptCache) {
		t.Fatalf("malformed workspace scope = %v", err)
	}
	mismatchedKey := envelope
	mismatchedKey.Key = strings.Repeat("0", sha256.Size*2)
	if err := validateCacheKeyDigest(mismatchedKey); !errors.Is(err, ErrCorruptCache) {
		t.Fatalf("mismatched key = %v", err)
	}
}

func TestDirectoryCacheLoadRejectsValidRecordForAnotherKeyAndOversize(t *testing.T) {
	cache, err := NewDirectoryCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wantedKey, _ := coverageKey(t, "1")
	otherKey, otherResolution := coverageKey(t, "2")
	otherRecord, err := NewCacheRecord(otherKey, coverageSchema(), otherResolution)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := otherRecord.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache.path(wantedKey), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(context.Background(), wantedKey, coverageSchema()); !errors.Is(err, ErrCorruptCache) {
		t.Fatalf("mismatched record = %v", err)
	}
	if err := os.WriteFile(cache.path(wantedKey), bytes.Repeat([]byte("x"), runtimeCacheMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(context.Background(), wantedKey, coverageSchema()); !errors.Is(err, ErrCorruptCache) {
		t.Fatalf("oversized record = %v", err)
	}
}
