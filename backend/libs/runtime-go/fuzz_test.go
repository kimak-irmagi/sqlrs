package runtimev2

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
)

func FuzzDigestAndCanonicalScalars(f *testing.F) {
	f.Add("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", []byte("value"))
	f.Add("bad", []byte{0, 1, 2})
	f.Fuzz(func(t *testing.T, digest string, payload []byte) {
		if parsed, err := parseDigest(digest, "digest"); err == nil && len(parsed) != 32 {
			t.Fatalf("accepted digest decoded to %d bytes", len(parsed))
		}
		encoded := encodeBytes(payload)
		if len(encoded) != len(payload)+8 || binary.BigEndian.Uint64(encoded[:8]) != uint64(len(payload)) || !bytes.Equal(encoded[8:], payload) {
			t.Fatal("byte scalar encoding is not length-prefixed and lossless")
		}
		if !bytes.Equal(encoded, encodeBytes(payload)) || !bytes.Equal(encodeString(string(payload)), encodeString(string(payload))) {
			t.Fatal("canonical scalar encoding is not deterministic")
		}
	})
}

func FuzzFieldPermutations(f *testing.F) {
	f.Add([]byte{0, 1, 2})
	f.Add([]byte{2, 1, 0})
	base := []ResolvedField{{Name: "alpha", Value: "1"}, {Name: "beta", Value: "2"}, {Name: "gamma", Value: "3"}}
	wantIdentity, _ := NewTransformIdentity(TransformIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "v1", Fields: base})
	want, _ := TransformFingerprint(wantIdentity)
	f.Fuzz(func(t *testing.T, order []byte) {
		fields := append([]ResolvedField(nil), base...)
		for index, value := range order {
			left := index % len(fields)
			right := int(value) % len(fields)
			fields[left], fields[right] = fields[right], fields[left]
		}
		identity, err := NewTransformIdentity(TransformIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "v1", Fields: fields})
		if err != nil {
			t.Fatal(err)
		}
		got, _ := TransformFingerprint(identity)
		if got != want {
			t.Fatalf("field permutation changed fingerprint: %s != %s", got, want)
		}
	})
}

func FuzzLineageJSON(f *testing.F) {
	factory, transform := requirementProvenanceForFuzz(f)
	recipe, _ := NewRecipe(factory, []TransformProvenance{transform})
	lineage, _ := Build(recipe)
	relative, _ := Extend(lineage.Root().ID(), []TransformProvenance{transform})
	lineageJSON, _ := json.Marshal(lineage)
	relativeJSON, _ := json.Marshal(relative)
	f.Add(lineageJSON, false)
	f.Add(relativeJSON, true)
	f.Add([]byte(`{}`), false)
	f.Fuzz(func(t *testing.T, raw []byte, isRelative bool) {
		if isRelative {
			var value RelativeLineage
			err := DecodeJSON(raw, &value)
			if err != nil {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("unstructured relative-lineage error: %v", err)
				}
				if value.Anchor() != "" || value.Steps() != nil || value.EndpointID() != "" {
					t.Fatal("failed relative-lineage decode returned a partial value")
				}
				return
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var decoded RelativeLineage
			if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.EndpointID() != value.EndpointID() {
				t.Fatalf("accepted relative lineage is not stable: %v", err)
			}
			return
		}
		var value RecipeLineage
		err := DecodeJSON(raw, &value)
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("unstructured recipe-lineage error: %v", err)
			}
			if value.Factory().data != nil || value.Root().data != nil || value.Steps() != nil || value.Endpoint().data != nil {
				t.Fatal("failed recipe-lineage decode returned a partial value")
			}
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded RecipeLineage
		if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Endpoint().ID() != value.Endpoint().ID() {
			t.Fatalf("accepted recipe lineage is not stable: %v", err)
		}
	})
}

func requirementProvenanceForFuzz(tb testing.TB) (FactoryProvenance, TransformProvenance) {
	tb.Helper()
	factoryIdentity, err := NewFactoryIdentity(FactoryIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "postgres", IdentitySchema: "factory.v1"})
	if err != nil {
		tb.Fatal(err)
	}
	transformIdentity, err := NewTransformIdentity(TransformIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "transform.v1"})
	if err != nil {
		tb.Fatal(err)
	}
	factory, err := NewFactoryProvenance(FactoryProvenanceInput{Identity: factoryIdentity})
	if err != nil {
		tb.Fatal(err)
	}
	transform, err := NewTransformProvenance(TransformProvenanceInput{Identity: transformIdentity})
	if err != nil {
		tb.Fatal(err)
	}
	return factory, transform
}
