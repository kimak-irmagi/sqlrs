package runtimev2

import (
	"encoding/hex"
	"math"
	"sort"
	"strings"
	"testing"
)

func TestCanonicalPrimitiveBytes(t *testing.T) {
	integerCases := []struct {
		name string
		got  []byte
		want string
	}{
		{"u16 zero", encodeUint16(0), "0000"},
		{"u16 max", encodeUint16(math.MaxUint16), "ffff"},
		{"u32 zero", encodeUint32(0), "00000000"},
		{"u32 max", encodeUint32(math.MaxUint32), "ffffffff"},
		{"u64 zero", encodeUint64(0), "0000000000000000"},
		{"u64 max", encodeUint64(math.MaxUint64), "ffffffffffffffff"},
	}
	for _, test := range integerCases {
		t.Run(test.name, func(t *testing.T) {
			if got := hex.EncodeToString(test.got); got != test.want {
				t.Fatalf("encoding = %s, want %s", got, test.want)
			}
		})
	}
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

func TestEveryThreeFieldPermutationIsCanonical(t *testing.T) {
	fields := []ResolvedField{{Name: "a", Value: "first"}, {Name: "m", Value: "middle"}, {Name: "z", Value: "last"}}
	var fingerprints []Fingerprint
	for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		input := TransformIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "schema.v1"}
		for _, index := range order {
			input.Fields = append(input.Fields, fields[index])
		}
		identity, err := NewTransformIdentity(input)
		if err != nil {
			t.Fatal(err)
		}
		fingerprint, _ := TransformFingerprint(identity)
		fingerprints = append(fingerprints, fingerprint)
	}
	for _, fingerprint := range fingerprints[1:] {
		if fingerprint != fingerprints[0] {
			t.Fatal("field permutation changed fingerprint")
		}
	}
}

func TestCanonicalStringsAreNotUnicodeNormalized(t *testing.T) {
	nfc, nfd := "é", "e\u0301"
	if nfc == nfd || string(encodeString(nfc)) == string(encodeString(nfd)) {
		t.Fatal("distinct NFC and NFD bytes collapsed")
	}
	inputs := []string{nfc, nfd}
	fingerprints := make([]Fingerprint, 0, len(inputs))
	for _, value := range inputs {
		identity, err := NewTransformIdentity(TransformIdentityInput{
			SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "schema.v1",
			Fields: []ResolvedField{{Name: "sql", Value: value}},
		})
		if err != nil {
			t.Fatal(err)
		}
		fingerprint, _ := TransformFingerprint(identity)
		fingerprints = append(fingerprints, fingerprint)
	}
	if fingerprints[0] == fingerprints[1] {
		t.Fatal("Unicode normalization changed identity semantics")
	}
}

func TestCanonicalDomainBytesAreFixed(t *testing.T) {
	domains := []string{factoryDomain, transformDomain, stateDomain}
	want := []string{"sqlrs.runtime.v2/factory-state", "sqlrs.runtime.v2/transform", "sqlrs.runtime.v2/state"}
	if !sort.StringsAreSorted([]string{factoryDomain, stateDomain, transformDomain}) {
		t.Fatal("test invariant: expected lexical domain order")
	}
	for index := range domains {
		if domains[index] != want[index] {
			t.Fatalf("domain %d = %q, want %q", index, domains[index], want[index])
		}
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
