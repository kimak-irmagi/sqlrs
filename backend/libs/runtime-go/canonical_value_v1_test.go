package runtimev2

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestCanonicalV1NullKnownAnswer covers CV01 and OR01 from
// runtime-v2-canonical-contract-tests.md.
func TestCanonicalV1NullKnownAnswer(t *testing.T) {
	value := CanonicalNull()
	if got := hex.EncodeToString(value.CanonicalBytes()); got != "00000000000000000000" {
		t.Fatalf("canonical null = %s", got)
	}
	if got := value.Token().String(); got != "civ1:sha256:2ac65b7225ef0570f257d700065ac35986ff1877faec596c7eb3f0023691cc63" {
		t.Fatalf("canonical null token = %q", got)
	}
	if value.Depth() != 1 || value.NodeCount() != 1 {
		t.Fatalf("canonical null budget = depth %d nodes %d", value.Depth(), value.NodeCount())
	}
}

// TestCanonicalV1EveryFiveMemberPermutation covers CV03 exhaustively for the
// approved five-member constructor boundary.
func TestCanonicalV1EveryFiveMemberPermutation(t *testing.T) {
	values := make([]CanonicalValue, 5)
	entries := make([]CanonicalMapEntry, 5)
	for index := range values {
		values[index], _ = CanonicalString(fmt.Sprintf("value-%d", index))
		entries[index] = CanonicalMapEntry{Key: fmt.Sprintf("key-%d", index), Value: values[index]}
	}
	wantMap, _ := CanonicalMap(entries)
	wantSet, _ := CanonicalSet(values)
	permutation := []int{0, 1, 2, 3, 4}
	count := 0
	var visit func(int)
	visit = func(offset int) {
		if offset == len(permutation) {
			mapInput := make([]CanonicalMapEntry, len(permutation))
			setInput := make([]CanonicalValue, len(permutation))
			for index, source := range permutation {
				mapInput[index], setInput[index] = entries[source], values[source]
			}
			gotMap, err := CanonicalMap(mapInput)
			if err != nil || !bytes.Equal(gotMap.CanonicalBytes(), wantMap.CanonicalBytes()) {
				t.Fatalf("map permutation %v changed canonical bytes: %v", permutation, err)
			}
			gotSet, err := CanonicalSet(setInput)
			if err != nil || !bytes.Equal(gotSet.CanonicalBytes(), wantSet.CanonicalBytes()) {
				t.Fatalf("set permutation %v changed canonical bytes: %v", permutation, err)
			}
			count++
			return
		}
		for index := offset; index < len(permutation); index++ {
			permutation[offset], permutation[index] = permutation[index], permutation[offset]
			visit(offset + 1)
			permutation[offset], permutation[index] = permutation[index], permutation[offset]
		}
	}
	visit(0)
	if count != 120 {
		t.Fatalf("visited %d permutations, want 120", count)
	}
}

// TestCanonicalV1Collections covers CV02, CV03, CV04, and CV05.
func TestCanonicalV1Collections(t *testing.T) {
	nfc, err := CanonicalString("é")
	if err != nil {
		t.Fatal(err)
	}
	nfd, err := CanonicalString("e\u0301")
	if err != nil {
		t.Fatal(err)
	}
	if nfc.Token() == nfd.Token() {
		t.Fatal("canonical values normalized Unicode")
	}

	one, _ := CanonicalString("one")
	two, _ := CanonicalString("two")
	left, err := CanonicalMap([]CanonicalMapEntry{{Key: "b", Value: two}, {Key: "a", Value: one}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := CanonicalMap([]CanonicalMapEntry{{Key: "a", Value: one}, {Key: "b", Value: two}})
	if err != nil {
		t.Fatal(err)
	}
	if left.Token() != right.Token() || string(left.CanonicalBytes()) != string(right.CanonicalBytes()) {
		t.Fatal("map source order changed canonical value")
	}
	if _, err := CanonicalMap([]CanonicalMapEntry{{Key: "a", Value: one}, {Key: "a", Value: two}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate source key accepted: %v", err)
	}
	if _, err := CanonicalSet([]CanonicalValue{one, one}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate set member accepted: %v", err)
	}

	list, err := CanonicalList([]CanonicalValue{one, two})
	if err != nil {
		t.Fatal(err)
	}
	reversed, _ := CanonicalList([]CanonicalValue{two, one})
	if list.Token() == reversed.Token() {
		t.Fatal("ordered list lost member order")
	}

	emptyString, _ := CanonicalString("")
	emptyList, _ := CanonicalList(nil)
	emptyMap, _ := CanonicalMap(nil)
	emptySet, _ := CanonicalSet(nil)
	seen := map[CanonicalValueToken]bool{}
	for _, value := range []CanonicalValue{CanonicalNull(), emptyString, emptyList, emptyMap, emptySet} {
		if seen[value.Token()] {
			t.Fatal("null and empty canonical values are not distinct")
		}
		seen[value.Token()] = true
	}
}

// TestCanonicalV1LimitsAndParsing covers CV06-CV08 and LM01.
func TestCanonicalV1LimitsAndParsing(t *testing.T) {
	for _, size := range []int{MaxCanonicalStringBytes - 1, MaxCanonicalStringBytes, MaxCanonicalStringBytes + 1} {
		value, err := CanonicalString(strings.Repeat("x", size))
		if size <= MaxCanonicalStringBytes && err != nil {
			t.Fatalf("size %d rejected: %v", size, err)
		}
		if size > MaxCanonicalStringBytes && (!errors.Is(err, ErrInvalid) || value.Valid()) {
			t.Fatalf("size %d accepted: %+v %v", size, value, err)
		}
	}

	token, err := ParseCanonicalValueToken("civ1:sha256:2ac65b7225ef0570f257d700065ac35986ff1877faec596c7eb3f0023691cc63")
	if err != nil || token.String() == "" {
		t.Fatalf("valid token rejected: %q %v", token, err)
	}
	for _, raw := range []string{
		"", "civ1:sha256:ABC65b7225ef0570f257d700065ac35986ff1877faec596c7eb3f0023691cc63",
		"civ1:sha256:2ac65b72", "civ1:sha512:" + strings.Repeat("a", 64),
		" civ1:sha256:" + strings.Repeat("a", 64), "civ1:sha256:" + strings.Repeat("a", 64) + ":x",
	} {
		if value, err := ParseCanonicalValueToken(raw); !errors.Is(err, ErrInvalid) || value.Valid() {
			t.Fatalf("invalid token %q accepted: %+v %v", raw, value, err)
		}
	}

	if (CanonicalValue{}).Valid() || (CanonicalValueToken{}).Valid() {
		t.Fatal("zero canonical values are valid")
	}
	if _, err := ParseCanonicalValueEnvelope([]byte{0, 0, 0}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("truncated value accepted: %v", err)
	}
	decoded, err := ParseCanonicalValueEnvelope(CanonicalNull().CanonicalBytes())
	if err != nil || decoded.Token() != CanonicalNull().Token() {
		t.Fatalf("round trip = %+v %v", decoded, err)
	}
}

// TestCanonicalV1TypedFields covers ID01, ID02, ID03, and ID07.
func TestCanonicalV1TypedFields(t *testing.T) {
	text, err := NewTextIdentityField("source", "civ1:sha256:"+strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	value, _ := CanonicalString("payload")
	canonical, err := NewCanonicalValueIdentityField("source", value)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := NewSecretReference("vault", "opaque-7f4a", "rotation-3")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := NewSecretReferenceIdentityField("source", ref)
	if err != nil {
		t.Fatal(err)
	}
	if text.Kind() != IdentityFieldText || canonical.Kind() != IdentityFieldCanonicalValue || secret.Kind() != IdentityFieldSecretReference {
		t.Fatalf("field kinds = %q %q %q", text.Kind(), canonical.Kind(), secret.Kind())
	}
	if !strings.HasPrefix(text.Text(), "civ1:") {
		t.Fatal("text token was sniffed")
	}
	if text.Commitment() == canonical.Commitment() || canonical.Commitment() == secret.Commitment() {
		t.Fatal("field kind did not separate commitments")
	}
	if ref.Provider() != "vault" || ref.Identifier() != "opaque-7f4a" || ref.Version() != "rotation-3" {
		t.Fatalf("secret reference lost data: %+v", ref)
	}
	changed, _ := NewSecretReference("vault", "opaque-7f4a", "rotation-4")
	changedField, _ := NewSecretReferenceIdentityField("source", changed)
	if changedField.Commitment() == secret.Commitment() {
		t.Fatal("secret revision did not change contribution")
	}
	if (IdentityField{}).Valid() || (SecretReference{}).Valid() {
		t.Fatal("zero typed field values are valid")
	}
}
