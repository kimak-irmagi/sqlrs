package resolver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// These tests specify the public cache envelope described by
// docs/architecture/runtime-v2-persistence-structure.md.
func TestCacheRecordLegacyWireCompatibility(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "resolution-cache-v0.2.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.TrimSpace(raw)
	key := CacheKey{
		digest:         "54f09db3aa36879ab19fa6b7e8aa3844929ff88859a1e104e10b4cbad64c0fdb",
		workspaceScope: "9400f1b21cb527d7fa3d3eabba93557d81f3918c6eabd22fc6bd8ef8bedc2722",
		descriptor: Descriptor{
			Role: "input", Owner: "owner", Kind: "kind",
			SpecificationSchema: "owner.kind.v1", SemanticVersion: "1",
		},
		declaration: json.RawMessage(`{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1","fields":[]}`),
	}
	record, err := DecodeCacheRecordJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !record.Matches(key) {
		t.Fatal("legacy record did not match its complete cache key")
	}
	encoded, err := record.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, raw) {
		t.Fatalf("legacy record changed on re-encode\n got: %s\nwant: %s", encoded, raw)
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
			record, err := NewCacheRecord(key, coverageResolution(t))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := record.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeCacheRecordJSON(raw)
			if err != nil || !decoded.Matches(key) {
				t.Fatalf("round trip = %v, matches=%v", err, decoded.Matches(key))
			}
		})
	}
}

func TestCacheRecordConstructionRoundTripAndDefensiveCopies(t *testing.T) {
	key, resolution := coverageKey(t, "1")
	inputEvidence := resolution.Evidence
	record, err := NewCacheRecord(key, resolution)
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
	decoded, err := DecodeCacheRecordJSON(raw)
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
	record, err := NewCacheRecord(key, resolution)
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
	for name, change := range map[string]func(*CacheKey, *Resolution){
		"zero key":         func(key *CacheKey, _ *Resolution) { *key = CacheKey{} },
		"zero resolution":  func(_ *CacheKey, resolution *Resolution) { *resolution = Resolution{} },
		"invalid evidence": func(_ *CacheKey, resolution *Resolution) { resolution.Evidence = []byte(`{`) },
	} {
		t.Run(name, func(t *testing.T) {
			candidateKey, candidateResolution := key, resolution
			change(&candidateKey, &candidateResolution)
			if _, err := NewCacheRecord(candidateKey, candidateResolution); !errors.Is(err, ErrInvalidDeclaration) {
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
	if zero.Resolution().Identity.SchemaVersion() != "" {
		t.Fatal("zero record returned resolution")
	}
	if _, err := zero.MarshalJSON(); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("zero marshal=%v", err)
	}
	key, resolution := coverageKey(t, "1")
	key.declaration = json.RawMessage(`{}`)
	if _, err := NewCacheRecord(key, resolution); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("malformed declaration=%v", err)
	}
	key, _ = coverageKey(t, "1")
	key.descriptor.Owner = "other"
	if _, err := NewCacheRecord(key, resolution); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("descriptor mismatch=%v", err)
	}
}

func TestDecodeCacheRecordJSONTrustBoundary(t *testing.T) {
	key, resolution := coverageKey(t, "1")
	record, err := NewCacheRecord(key, resolution)
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
		candidate.Resolution = bytes.Replace(candidate.Resolution, []byte(`"owner":"owner"`), []byte(`"owner":""`), 1)
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
		candidate.Resolution = cloneResolution(envelope.Resolution)
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
		{"duplicate member", append([]byte(`{"schema_version":"sqlrs.resolution-cache.v1",`), valid[1:]...), ErrCorruptCache},
		{"null required member", bytes.Replace(valid, []byte(`"key":"`+key.String()+`"`), []byte(`"key":null`), 1), ErrCorruptCache},
		{"missing required member", reencode(func(candidate *cacheRecord) { candidate.Key = "" }), ErrCorruptCache},
		{"trailing token", append(append([]byte(nil), valid...), []byte(` {}`)...), ErrCorruptCache},
		{"oversized", bytes.Repeat([]byte(" "), runtimeCacheMaxBytes+1), ErrCorruptCache},
		{"checksum", bytes.Replace(valid, []byte(`"checksum":"sha256:`), []byte(`"checksum":"sha256:x`), 1), ErrCorruptCache},
		{"unsupported schema", reencode(func(candidate *cacheRecord) { candidate.SchemaVersion = "future" }), ErrIncompatibleCache},
		{"malformed identity", malformedIdentity(), ErrCorruptCache},
		{"zero value", []byte(`{}`), ErrCorruptCache},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeCacheRecordJSON(test.raw); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
