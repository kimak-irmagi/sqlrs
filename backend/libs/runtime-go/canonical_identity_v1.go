package runtimev2

import (
	"bytes"
	"crypto/sha256"
	"sort"
)

// CanonicalFingerprint is a canonical-v1 digest and is intentionally not
// assignable to the legacy Fingerprint type.
type CanonicalFingerprint string

// CanonicalStateID is a canonical-v1 state identifier and is intentionally not
// assignable to the legacy StateID type.
type CanonicalStateID string

type canonicalIdentityField struct {
	field      IdentityField
	disclosure DisclosureClass
}

type canonicalIdentityData struct {
	provider, semanticKind, identitySchema string
	fields                                 []canonicalIdentityField
}

// CanonicalFactoryIdentity is an immutable schema-bound factory identity.
type CanonicalFactoryIdentity struct{ data *canonicalIdentityData }

// CanonicalTransformIdentity is an immutable schema-bound transform identity.
type CanonicalTransformIdentity struct{ data *canonicalIdentityData }

// CanonicalResolvedExtensionIdentity is an immutable schema-bound extension.
type CanonicalResolvedExtensionIdentity struct{ data *canonicalIdentityData }

func copyCanonicalIdentityData(source *canonicalIdentityData) *canonicalIdentityData {
	if source == nil {
		return nil
	}
	result := *source
	result.fields = append([]canonicalIdentityField(nil), source.fields...)
	return &result
}

func canonicalTypedFieldSet(fields []canonicalIdentityField) ([]byte, error) {
	ordered := append([]canonicalIdentityField(nil), fields...)
	sort.Slice(ordered, func(i, j int) bool {
		return bytes.Compare([]byte(ordered[i].field.Name()), []byte(ordered[j].field.Name())) < 0
	})
	result := encodeUint32(uint32(len(ordered)))
	for index, item := range ordered {
		if !item.field.Valid() {
			return nil, canonicalInvalid(CodeShapeInvalid, "fields["+itoa(index)+"]")
		}
		raw, err := parseDigest(string(item.field.Commitment()), "fields["+itoa(index)+"].commitment")
		if err != nil {
			return nil, canonicalizeValidationError(err)
		}
		result = append(result, encodeString(item.field.Name())...)
		result = append(result, encodeUint16(identityFieldKindTag(item.field.Kind()))...)
		result = append(result, raw...)
	}
	return result, nil
}

func identityFieldKindTag(kind IdentityFieldKind) uint16 {
	switch kind {
	case IdentityFieldText:
		return identityFieldTextTag
	case IdentityFieldCanonicalValue:
		return identityFieldCanonicalValueTag
	case IdentityFieldSecretReference:
		return identityFieldSecretReferenceTag
	default:
		return 0
	}
}

func canonicalIdentityBytes(domain string, data *canonicalIdentityData) ([]byte, error) {
	if data == nil {
		return nil, canonicalInvalid(CodeShapeInvalid, "identity")
	}
	fields, err := canonicalTypedFieldSet(data.fields)
	if err != nil {
		return nil, err
	}
	return encodeRecord(domain, []canonicalField{
		{tag: 1, payload: []byte(CanonicalSchemaVersion)},
		{tag: 2, payload: []byte(data.provider)},
		{tag: 3, payload: []byte(data.semanticKind)},
		{tag: 4, payload: []byte(data.identitySchema)},
		{tag: 5, payload: fields},
	}), nil
}

// CanonicalFactoryIdentityBytes returns a defensive copy of the exact preimage.
func CanonicalFactoryIdentityBytes(identity CanonicalFactoryIdentity) ([]byte, error) {
	return canonicalIdentityBytes(CanonicalFactoryStateDomain, identity.data)
}

// CanonicalTransformIdentityBytes returns a defensive copy of the exact preimage.
func CanonicalTransformIdentityBytes(identity CanonicalTransformIdentity) ([]byte, error) {
	return canonicalIdentityBytes(CanonicalTransformDomain, identity.data)
}

// CanonicalResolvedExtensionBytes returns a defensive copy of the exact preimage.
func CanonicalResolvedExtensionBytes(identity CanonicalResolvedExtensionIdentity) ([]byte, error) {
	return canonicalIdentityBytes(CanonicalResolvedExtensionDomain, identity.data)
}

func canonicalFingerprint(domain string, data *canonicalIdentityData) (CanonicalFingerprint, error) {
	encoded, err := canonicalIdentityBytes(domain, data)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return CanonicalFingerprint("sha256:" + hexLower(digest[:])), nil
}

// CanonicalFactoryFingerprint hashes a verified factory identity.
func CanonicalFactoryFingerprint(identity CanonicalFactoryIdentity) (CanonicalFingerprint, error) {
	return canonicalFingerprint(CanonicalFactoryStateDomain, identity.data)
}

// CanonicalTransformFingerprint hashes a verified transform identity.
func CanonicalTransformFingerprint(identity CanonicalTransformIdentity) (CanonicalFingerprint, error) {
	return canonicalFingerprint(CanonicalTransformDomain, identity.data)
}

// CanonicalExtensionFingerprint hashes a verified resolved extension identity.
func CanonicalExtensionFingerprint(identity CanonicalResolvedExtensionIdentity) (CanonicalFingerprint, error) {
	return canonicalFingerprint(CanonicalResolvedExtensionDomain, identity.data)
}

func (i CanonicalFactoryIdentity) valid() bool   { return i.data != nil }
func (i CanonicalTransformIdentity) valid() bool { return i.data != nil }

func (i CanonicalFactoryIdentity) Provider() string {
	if i.data == nil {
		return ""
	}
	return i.data.provider
}
func (i CanonicalFactoryIdentity) Kind() string {
	if i.data == nil {
		return ""
	}
	return i.data.semanticKind
}
func (i CanonicalFactoryIdentity) IdentitySchema() string {
	if i.data == nil {
		return ""
	}
	return i.data.identitySchema
}
func (i CanonicalTransformIdentity) Provider() string {
	if i.data == nil {
		return ""
	}
	return i.data.provider
}
func (i CanonicalTransformIdentity) Kind() string {
	if i.data == nil {
		return ""
	}
	return i.data.semanticKind
}
func (i CanonicalTransformIdentity) IdentitySchema() string {
	if i.data == nil {
		return ""
	}
	return i.data.identitySchema
}
