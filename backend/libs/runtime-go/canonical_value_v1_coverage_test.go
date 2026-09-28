package runtimev2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCanonicalValueConstructorBoundaryMatrix(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	if _, err := CanonicalString(invalidUTF8); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	if _, err := CanonicalString(strings.Repeat("x", MaxCanonicalStringBytes+1)); err == nil {
		t.Fatal("oversized string accepted")
	}
	if _, err := CanonicalList(make([]CanonicalValue, MaxCanonicalCollectionMembers+1)); err == nil {
		t.Fatal("oversized list accepted")
	}
	if _, err := CanonicalList([]CanonicalValue{{}}); err == nil {
		t.Fatal("zero list member accepted")
	}
	if _, err := CanonicalMap(make([]CanonicalMapEntry, MaxCanonicalCollectionMembers+1)); err == nil {
		t.Fatal("oversized map accepted")
	}
	valid := CanonicalNull()
	mapCases := []CanonicalMapEntry{{Key: invalidUTF8, Value: valid}, {Key: strings.Repeat("k", MaxCanonicalMapKeyBytes+1), Value: valid}, {Key: "ok", Value: CanonicalValue{}}}
	for index, entry := range mapCases {
		if _, err := CanonicalMap([]CanonicalMapEntry{entry}); err == nil {
			t.Fatalf("invalid map case %d accepted", index)
		}
	}
	if _, err := CanonicalMap([]CanonicalMapEntry{{Key: "a", Value: valid}, {Key: "a", Value: valid}}); err == nil {
		t.Fatal("duplicate map key accepted")
	}
	emptyKey, err := CanonicalMap([]CanonicalMapEntry{{Key: "", Value: valid}})
	if err != nil {
		t.Fatalf("empty UTF-8 map key rejected: %v", err)
	}
	if decoded, err := ParseCanonicalValueEnvelope(emptyKey.CanonicalBytes()); err != nil || decoded.Token() != emptyKey.Token() {
		t.Fatalf("empty-key map round trip failed: %v", err)
	}
	if _, err := CanonicalSet(make([]CanonicalValue, MaxCanonicalCollectionMembers+1)); err == nil {
		t.Fatal("oversized set accepted")
	}
	if _, err := CanonicalSet([]CanonicalValue{{}}); err == nil {
		t.Fatal("zero set member accepted")
	}
	if _, err := CanonicalSet([]CanonicalValue{valid, valid}); err == nil {
		t.Fatal("duplicate set member accepted")
	}

	deep := valid
	for depth := 1; depth < MaxCanonicalValueDepth; depth++ {
		deep, _ = CanonicalList([]CanonicalValue{deep})
	}
	if _, err := CanonicalList([]CanonicalValue{deep}); err == nil {
		t.Fatal("depth limit not enforced")
	}
}

// TestCanonicalValueExactBudgetBoundaries covers LM01 at limit-1, limit, and
// limit+1 for the composite budgets that cannot be represented by small cases.
func TestCanonicalValueExactBudgetBoundaries(t *testing.T) {
	values := make([]CanonicalValue, MaxCanonicalCollectionMembers+1)
	entries := make([]CanonicalMapEntry, MaxCanonicalCollectionMembers+1)
	for index := range values {
		values[index], _ = CanonicalString(fmt.Sprintf("v-%03d", index))
		entries[index] = CanonicalMapEntry{Key: fmt.Sprintf("k-%03d", index), Value: CanonicalNull()}
	}
	for _, count := range []int{MaxCanonicalCollectionMembers - 1, MaxCanonicalCollectionMembers} {
		if _, err := CanonicalList(values[:count]); err != nil {
			t.Fatalf("list member boundary %d rejected: %v", count, err)
		}
		if _, err := CanonicalSet(values[:count]); err != nil {
			t.Fatalf("set member boundary %d rejected: %v", count, err)
		}
		if _, err := CanonicalMap(entries[:count]); err != nil {
			t.Fatalf("map member boundary %d rejected: %v", count, err)
		}
	}
	if _, err := CanonicalList(values); err == nil {
		t.Fatal("list member limit+1 accepted")
	}
	if _, err := CanonicalSet(values); err == nil {
		t.Fatal("set member limit+1 accepted")
	}
	if _, err := CanonicalMap(entries); err == nil {
		t.Fatal("map member limit+1 accepted")
	}

	for _, size := range []int{MaxCanonicalMapKeyBytes - 1, MaxCanonicalMapKeyBytes} {
		if _, err := CanonicalMap([]CanonicalMapEntry{{Key: strings.Repeat("k", size), Value: CanonicalNull()}}); err != nil {
			t.Fatalf("map key boundary %d rejected: %v", size, err)
		}
	}
	if _, err := CanonicalMap([]CanonicalMapEntry{{Key: strings.Repeat("k", MaxCanonicalMapKeyBytes+1), Value: CanonicalNull()}}); err == nil {
		t.Fatal("map key limit+1 accepted")
	}

	depthValue := CanonicalNull()
	for depth := 2; depth <= MaxCanonicalValueDepth; depth++ {
		var err error
		depthValue, err = CanonicalList([]CanonicalValue{depthValue})
		if err != nil {
			t.Fatalf("depth %d rejected: %v", depth, err)
		}
	}
	if _, err := CanonicalList([]CanonicalValue{depthValue}); err == nil {
		t.Fatal("depth limit+1 accepted")
	}

	groups := make([]CanonicalValue, 16)
	for group := 0; group < 15; group++ {
		members := make([]CanonicalValue, 256)
		for index := range members {
			members[index] = CanonicalNull()
		}
		groups[group], _ = CanonicalList(members)
	}
	last := make([]CanonicalValue, 239)
	for index := range last {
		last[index] = CanonicalNull()
	}
	groups[15], _ = CanonicalList(last)
	exactNodes, err := CanonicalList(groups)
	if err != nil || exactNodes.NodeCount() != MaxCanonicalValueNodes {
		t.Fatalf("exact node budget = %d, %v", exactNodes.NodeCount(), err)
	}
	last = append(last, CanonicalNull())
	groups[15], _ = CanonicalList(last)
	if _, err := CanonicalList(groups); err == nil {
		t.Fatal("node limit+1 accepted")
	}

	makeByteBoundary := func(lastSize int) CanonicalValue {
		members := make([]CanonicalValue, 255)
		for index := 0; index < 254; index++ {
			members[index], _ = CanonicalString(strings.Repeat("x", MaxCanonicalStringBytes))
		}
		members[254], _ = CanonicalString(strings.Repeat("x", lastSize))
		value, err := CanonicalList(members)
		if err != nil {
			t.Fatalf("byte boundary %d rejected: %v", lastSize, err)
		}
		return value
	}
	belowBytes := makeByteBoundary(3587)
	exactBytes := makeByteBoundary(3588)
	if len(belowBytes.CanonicalBytes()) != MaxCanonicalValueBytes-1 || len(exactBytes.CanonicalBytes()) != MaxCanonicalValueBytes {
		t.Fatalf("byte boundaries = %d/%d", len(belowBytes.CanonicalBytes()), len(exactBytes.CanonicalBytes()))
	}
	members := make([]CanonicalValue, 255)
	for index := 0; index < 254; index++ {
		members[index], _ = CanonicalString(strings.Repeat("x", MaxCanonicalStringBytes))
	}
	members[254], _ = CanonicalString(strings.Repeat("x", 3589))
	if _, err := CanonicalList(members); err == nil {
		t.Fatal("canonical byte limit+1 accepted")
	}
	if _, err := ParseCanonicalValueEnvelope(exactBytes.CanonicalBytes()); err != nil {
		t.Fatalf("decoded envelope exact byte limit rejected: %v", err)
	}
	if _, err := ParseCanonicalValueEnvelope(append(exactBytes.CanonicalBytes(), 0)); err == nil {
		t.Fatal("decoded envelope byte limit+1 accepted")
	}
}

func TestCanonicalValueZeroAndTokenBoundaryMatrix(t *testing.T) {
	var value CanonicalValue
	if value.CanonicalBytes() != nil || value.Depth() != 0 || value.NodeCount() != 0 || value.Token().Valid() {
		t.Fatal("invalid zero value accessors")
	}
	var token CanonicalValueToken
	if token.String() != "" {
		t.Fatal("zero token rendered")
	}
	valid := CanonicalNull().Token().String()
	invalid := []string{"", " " + valid, strings.ToUpper(valid), valid + "00", "civ1:md5:" + strings.Repeat("0", 64), "civ1:sha256:" + strings.Repeat("g", 64)}
	for _, candidate := range invalid {
		if _, err := ParseCanonicalValueToken(candidate); err == nil {
			t.Fatalf("invalid token accepted: %q", candidate)
		}
	}
}

func TestCanonicalBinaryDecoderCompositeAndMalformedMatrix(t *testing.T) {
	a, _ := CanonicalString("a")
	b, _ := CanonicalString("b")
	list, _ := CanonicalList([]CanonicalValue{a, b})
	mapValue, _ := CanonicalMap([]CanonicalMapEntry{{Key: "b", Value: b}, {Key: "a", Value: a}})
	set, _ := CanonicalSet([]CanonicalValue{b, a})
	for _, value := range []CanonicalValue{list, mapValue, set} {
		decoded, err := ParseCanonicalValueEnvelope(value.CanonicalBytes())
		if err != nil {
			t.Fatal(err)
		}
		if string(decoded.CanonicalBytes()) != string(value.CanonicalBytes()) {
			t.Fatal("decode/encode mismatch")
		}
	}

	null := CanonicalNull().CanonicalBytes()
	unknown := append([]byte(nil), null...)
	binary.BigEndian.PutUint16(unknown[:2], 99)
	badNull := append([]byte(nil), null...)
	binary.BigEndian.PutUint64(badNull[2:10], 1)
	badNull = append(badNull, 0)
	badLength := append([]byte(nil), null...)
	binary.BigEndian.PutUint64(badLength[2:10], uint64(MaxCanonicalValueBytes+1))
	trailing := append(append([]byte(nil), null...), 0)
	badString := canonicalNode(canonicalStringTag, []byte{0xff}, 1, 1).CanonicalBytes()
	tooLongString := canonicalNode(canonicalStringTag, []byte(strings.Repeat("x", MaxCanonicalStringBytes+1)), 1, 1).CanonicalBytes()
	shortCollection := canonicalNode(canonicalListTag, []byte{0}, 1, 1).CanonicalBytes()
	widePayload := append(encodeUint32(MaxCanonicalCollectionMembers+1), make([]byte, 8*(MaxCanonicalCollectionMembers+1))...)
	wide := canonicalNode(canonicalListTag, widePayload, 1, 1).CanonicalBytes()
	truncatedMember := canonicalNode(canonicalListTag, append(encodeUint32(1), encodeUint64(100)...), 1, 1).CanonicalBytes()
	for index, raw := range [][]byte{nil, unknown, badNull, badLength, trailing, badString, tooLongString, shortCollection, wide, truncatedMember} {
		if _, err := ParseCanonicalValueEnvelope(raw); err == nil {
			t.Fatalf("malformed binary %d accepted", index)
		}
	}

	unsortedSetPayload := append(encodeUint32(2), encodeBytes(b.CanonicalBytes())...)
	unsortedSetPayload = append(unsortedSetPayload, encodeBytes(a.CanonicalBytes())...)
	unsortedSet := canonicalNode(canonicalSetTag, unsortedSetPayload, 1, 1).CanonicalBytes()
	if _, err := ParseCanonicalValueEnvelope(unsortedSet); err == nil {
		t.Fatal("unsorted set accepted")
	}
	duplicateSetPayload := append(encodeUint32(2), encodeBytes(a.CanonicalBytes())...)
	duplicateSetPayload = append(duplicateSetPayload, encodeBytes(a.CanonicalBytes())...)
	if _, err := ParseCanonicalValueEnvelope(canonicalNode(canonicalSetTag, duplicateSetPayload, 1, 1).CanonicalBytes()); err == nil {
		t.Fatal("duplicate set accepted")
	}

	keyB, _ := CanonicalString("b")
	keyA, _ := CanonicalString("a")
	badMapPayload := append(encodeUint32(2), encodeBytes(keyB.CanonicalBytes())...)
	badMapPayload = append(badMapPayload, encodeBytes(null)...)
	badMapPayload = append(badMapPayload, encodeBytes(keyA.CanonicalBytes())...)
	badMapPayload = append(badMapPayload, encodeBytes(null)...)
	if _, err := ParseCanonicalValueEnvelope(canonicalNode(canonicalMapTag, badMapPayload, 1, 1).CanonicalBytes()); err == nil {
		t.Fatal("unsorted map accepted")
	}
}

func TestAllocationPlanRejectsBeforeMaterialization(t *testing.T) {
	plan, err := allocationPlan(2, 2, 32, "entries")
	if err != nil || plan.count != 2 || plan.minimumSize != 32 {
		t.Fatalf("valid plan: %+v, %v", plan, err)
	}
	cases := []struct {
		count, frames uint64
		available     int
	}{{MaxCanonicalCollectionMembers + 1, 1, 1 << 20}, {^uint64(0), 2, 1 << 20}, {2, 2, 31}}
	for index, tc := range cases {
		if _, err := allocationPlan(tc.count, tc.frames, tc.available, "members"); err == nil {
			t.Fatalf("hostile plan %d accepted", index)
		}
	}
	_, err = allocationPlan(MaxCanonicalCollectionMembers+1, 1, 0, "members")
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != CodeLimitExceeded {
		t.Fatalf("unexpected plan error: %v", err)
	}
}

func TestIdentityFieldAndSecretReferenceBoundaryMatrix(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	referenceCases := [][3]string{{"Bad", "id", "v1"}, {"vault", "", "v1"}, {"vault", invalidUTF8, "v1"}, {"vault", "id", ""}, {"vault", "id", invalidUTF8}, {"vault", strings.Repeat("i", MaxCanonicalStringBytes+1), "v1"}, {"vault", "id", strings.Repeat("v", MaxCanonicalStringBytes+1)}}
	for index, value := range referenceCases {
		if _, err := NewSecretReference(value[0], value[1], value[2]); err == nil {
			t.Fatalf("invalid reference %d accepted", index)
		}
	}
	if _, err := NewTextIdentityField("value", invalidUTF8); err == nil {
		t.Fatal("invalid text accepted")
	}
	if _, err := NewTextIdentityField("value", strings.Repeat("x", MaxCanonicalStringBytes+1)); err == nil {
		t.Fatal("oversized text accepted")
	}
	if _, err := NewTextIdentityField("Bad", "x"); err == nil {
		t.Fatal("invalid field name accepted")
	}
	if _, err := NewCanonicalValueIdentityField("value", CanonicalValue{}); err == nil {
		t.Fatal("zero canonical value field accepted")
	}
	if _, err := NewSecretReferenceIdentityField("value", SecretReference{}); err == nil {
		t.Fatal("zero secret reference field accepted")
	}
	var reference SecretReference
	if reference.Valid() || reference.Provider() != "" || reference.Identifier() != "" || reference.Version() != "" {
		t.Fatal("zero reference accessors")
	}
	var field IdentityField
	if field.Valid() || field.Name() != "" || field.Kind() != "" || field.Commitment() != "" || field.Text() != "" || field.CanonicalValue().Valid() || field.SecretReference().Valid() {
		t.Fatal("zero field accessors")
	}
	text, _ := NewTextIdentityField("text", "x")
	if text.CanonicalValue().Valid() || text.SecretReference().Valid() {
		t.Fatal("wrong-kind accessors returned values")
	}
}

func TestCanonicalIdentityZeroAndCorruptInternalMatrix(t *testing.T) {
	var factory CanonicalFactoryIdentity
	if factory.valid() || factory.Provider() != "" || factory.Kind() != "" || factory.IdentitySchema() != "" {
		t.Fatal("zero factory identity accessors")
	}
	if _, err := CanonicalFactoryIdentityBytes(factory); err == nil {
		t.Fatal("zero factory bytes accepted")
	}
	if _, err := CanonicalFactoryFingerprint(factory); err == nil {
		t.Fatal("zero factory fingerprint accepted")
	}
	var transform CanonicalTransformIdentity
	if transform.valid() || transform.Provider() != "" || transform.Kind() != "" || transform.IdentitySchema() != "" {
		t.Fatal("zero transform identity accessors")
	}
	if _, err := CanonicalTransformIdentityBytes(transform); err == nil {
		t.Fatal("zero transform bytes accepted")
	}
	if _, err := CanonicalTransformFingerprint(transform); err == nil {
		t.Fatal("zero transform fingerprint accepted")
	}
	if copyCanonicalIdentityData(nil) != nil {
		t.Fatal("nil identity copy became non-nil")
	}
	if identityFieldKindTag("unknown") != 0 {
		t.Fatal("unknown field kind gained tag")
	}
	badField := IdentityField{data: &identityFieldData{name: "x", kind: IdentityFieldText, commitment: "bad"}}
	if _, err := canonicalTypedFieldSet([]canonicalIdentityField{{field: badField}}); err == nil {
		t.Fatal("malformed commitment accepted")
	}
	if _, err := canonicalTypedFieldSet([]canonicalIdentityField{{}}); err == nil {
		t.Fatal("zero field accepted")
	}
}

func TestCanonicalMapDecoderMalformedParts(t *testing.T) {
	null := CanonicalNull().CanonicalBytes()
	text, _ := CanonicalString("key")
	makeMap := func(payload []byte) []byte { return canonicalNode(canonicalMapTag, payload, 1, 1).CanonicalBytes() }
	nonString := append(encodeUint32(1), encodeBytes(null)...)
	nonString = append(nonString, encodeBytes(null)...)
	badValue := append(encodeUint32(1), encodeBytes(text.CanonicalBytes())...)
	badValue = append(badValue, encodeBytes([]byte{0})...)
	trailing := append(encodeUint32(1), encodeBytes(text.CanonicalBytes())...)
	trailing = append(trailing, encodeBytes(null)...)
	trailing = append(trailing, 0)
	missingValue := append(encodeUint32(1), encodeBytes(text.CanonicalBytes())...)
	for index, raw := range [][]byte{makeMap(nonString), makeMap(badValue), makeMap(trailing), makeMap(missingValue)} {
		if _, err := ParseCanonicalValueEnvelope(raw); err == nil {
			t.Fatalf("malformed map %d accepted", index)
		}
	}
	listPayload := append(encodeUint32(1), encodeBytes(null)...)
	listPayload = append(listPayload, 0)
	if _, err := ParseCanonicalValueEnvelope(canonicalNode(canonicalListTag, listPayload, 1, 1).CanonicalBytes()); err == nil {
		t.Fatal("list trailing payload accepted")
	}
}

func TestCanonicalCombinedNodeAndByteBudgets(t *testing.T) {
	large, _ := CanonicalString(strings.Repeat("x", MaxCanonicalStringBytes))
	members := make([]CanonicalValue, MaxCanonicalCollectionMembers)
	for index := range members {
		members[index] = large
	}
	if _, err := CanonicalList(members); err == nil {
		t.Fatal("combined canonical byte budget not enforced")
	}
	childMembers := make([]CanonicalValue, 16)
	for index := range childMembers {
		childMembers[index] = CanonicalNull()
	}
	child, _ := CanonicalList(childMembers)
	for index := range members {
		members[index] = child
	}
	if _, err := CanonicalList(members); err == nil {
		t.Fatal("combined node budget not enforced")
	}
	if _, err := ParseCanonicalValueEnvelope(make([]byte, MaxCanonicalValueBytes+1)); err == nil {
		t.Fatal("oversized binary envelope accepted")
	}
}

func TestDecodeIdentityFieldShapeMatrix(t *testing.T) {
	text := "x"
	cases := []identityFieldWire{
		{Name: "x", Kind: IdentityFieldText, Disclosure: DisclosurePublic, Text: &text, CanonicalHex: "00", Commitment: "bad"},
		{Name: "x", Kind: IdentityFieldCanonicalValue, Disclosure: DisclosurePublic, Text: &text, CanonicalHex: "00", Token: "bad", Commitment: "bad"},
		{Name: "x", Kind: IdentityFieldCanonicalValue, Disclosure: DisclosurePublic, CanonicalHex: "zz", Token: "bad", Commitment: "bad"},
		{Name: "x", Kind: IdentityFieldCanonicalValue, Disclosure: DisclosurePublic, CanonicalHex: "00", Token: "bad", Commitment: "bad"},
		{Name: "x", Kind: IdentityFieldSecretReference, Disclosure: DisclosurePublic, Text: &text, Secret: &secretReferenceWire{Provider: "vault", Identifier: "id", Version: "v1"}, Commitment: "bad"},
		{Name: "x", Kind: IdentityFieldSecretReference, Disclosure: DisclosurePublic, Secret: &secretReferenceWire{Provider: "Bad", Identifier: "id", Version: "v1"}, Commitment: "bad"},
		{Name: "x", Kind: "unknown", Disclosure: DisclosurePublic, Commitment: "bad"},
	}
	for index, wire := range cases {
		if _, err := decodeIdentityField(wire, index); err == nil {
			t.Fatalf("invalid field wire %d accepted", index)
		}
	}
	value, _ := CanonicalString("x")
	field, _ := NewCanonicalValueIdentityField("x", value)
	wire := fieldToWire(canonicalIdentityField{field: field, disclosure: DisclosurePublic})
	wire.Commitment = "sha256:" + strings.Repeat("0", 64)
	if _, err := decodeIdentityField(wire, 0); err == nil {
		t.Fatal("bad canonical field commitment accepted")
	}
	if expectedDomain("unknown") != "" {
		t.Fatal("unknown descriptor kind gained domain")
	}
	var target *FingerprintEnvelope
	if err := target.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("nil envelope receiver accepted")
	}
}

func TestDerivedStateRejectsCorruptVerifiedTransformDigest(t *testing.T) {
	transform := FingerprintEnvelope{data: &fingerprintEnvelopeData{descriptor: fingerprintDescriptorData{kind: FingerprintKindTransform, domain: CanonicalTransformDomain, digest: "bad"}}}
	if _, err := CanonicalDerivedStateBytes(CanonicalStateID("sha256:"+strings.Repeat("1", 64)), transform); err == nil {
		t.Fatal("corrupt transform digest accepted")
	}
}
