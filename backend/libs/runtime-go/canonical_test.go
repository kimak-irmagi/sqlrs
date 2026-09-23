package runtimev2

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestCanonicalPrimitiveBytes(t *testing.T) {
	if got := hex.EncodeToString(encodeString("A")); got != "000000000000000141" {
		t.Fatalf("string encoding = %s", got)
	}
	got := hex.EncodeToString(encodeRecord("d", []canonicalField{{tag: 1, payload: []byte("x")}}))
	want := "000000000000000164000000010001000000000000000178"
	if got != want {
		t.Fatalf("record encoding = %s, want %s", got, want)
	}
}

func TestCanonicalFieldOrderingAndFraming(t *testing.T) {
	a := encodeRecord("d", []canonicalField{{tag: 2, payload: []byte("c")}, {tag: 1, payload: []byte("ab")}})
	b := encodeRecord("d", []canonicalField{{tag: 1, payload: []byte("a")}, {tag: 2, payload: []byte("bc")}})
	if string(a) == string(b) {
		t.Fatal("length framing allowed an ambiguous record")
	}
	ordered := encodeRecord("d", []canonicalField{{tag: 2, payload: []byte("b")}, {tag: 1, payload: []byte("a")}})
	if strings.Index(hex.EncodeToString(ordered), "0001") > strings.LastIndex(hex.EncodeToString(ordered), "0002") {
		t.Fatal("record tags were not sorted")
	}
}

func TestCanonicalIdentityIgnoresInputFieldOrder(t *testing.T) {
	left, err := NewTransformIdentity(TransformIdentityInput{
		SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "schema.v1",
		Fields: []ResolvedField{{Name: "z", Value: "last"}, {Name: "a", Value: "first"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewTransformIdentity(TransformIdentityInput{
		SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "schema.v1",
		Fields: []ResolvedField{{Name: "a", Value: "first"}, {Name: "z", Value: "last"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := TransformFingerprint(left)
	b, _ := TransformFingerprint(right)
	if a != b || string(canonicalTransform(left)) != string(canonicalTransform(right)) {
		t.Fatal("field order affected canonical identity")
	}
}

func TestCanonicalDomainsAreSeparated(t *testing.T) {
	payload := []canonicalField{{tag: 1, payload: []byte("same")}}
	factory := hashRecord(factoryDomain, payload)
	transform := hashRecord(transformDomain, payload)
	state := hashRecord(stateDomain, payload)
	if factory == transform || transform == state || factory == state {
		t.Fatal("canonical domains collided")
	}
}
