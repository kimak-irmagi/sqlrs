package runtimev2

import (
	"crypto/sha256"
	"unicode/utf8"
)

// IdentityFieldKind makes canonical-v1 field interpretation explicit.
type IdentityFieldKind string

const (
	IdentityFieldText            IdentityFieldKind = "text"
	IdentityFieldCanonicalValue  IdentityFieldKind = "canonical-value"
	IdentityFieldSecretReference IdentityFieldKind = "secret-reference"
)

const (
	identityFieldTextTag uint16 = 1 + iota
	identityFieldCanonicalValueTag
	identityFieldSecretReferenceTag
)

type secretReferenceData struct{ provider, identifier, version string }

// SecretReference identifies a credential revision without containing a secret.
type SecretReference struct{ data *secretReferenceData }

// NewSecretReference constructs the only credential-related identity value.
func NewSecretReference(provider, identifier, version string) (SecretReference, error) {
	if err := validateIdentifier(provider, "provider"); err != nil {
		return SecretReference{}, canonicalizeValidationError(err)
	}
	if !utf8.ValidString(identifier) || identifier == "" {
		return SecretReference{}, canonicalInvalid(CodeValueInvalid, "identifier")
	}
	if !utf8.ValidString(version) || version == "" {
		return SecretReference{}, canonicalInvalid(CodeValueInvalid, "version")
	}
	if len(identifier) > MaxCanonicalStringBytes {
		return SecretReference{}, canonicalInvalid(CodeLimitExceeded, "identifier")
	}
	if len(version) > MaxCanonicalStringBytes {
		return SecretReference{}, canonicalInvalid(CodeLimitExceeded, "version")
	}
	return SecretReference{data: &secretReferenceData{provider, identifier, version}}, nil
}

func (r SecretReference) Valid() bool { return r.data != nil }
func (r SecretReference) Provider() string {
	if r.data == nil {
		return ""
	}
	return r.data.provider
}
func (r SecretReference) Identifier() string {
	if r.data == nil {
		return ""
	}
	return r.data.identifier
}
func (r SecretReference) Version() string {
	if r.data == nil {
		return ""
	}
	return r.data.version
}

type identityFieldData struct {
	name       string
	kind       IdentityFieldKind
	payload    []byte
	text       string
	value      CanonicalValue
	reference  SecretReference
	commitment CanonicalFingerprint
}

// IdentityField is one immutable typed canonical-v1 identity contribution.
type IdentityField struct{ data *identityFieldData }

// NewTextIdentityField constructs an explicitly textual identity field.
func NewTextIdentityField(name, value string) (IdentityField, error) {
	if !utf8.ValidString(value) {
		return IdentityField{}, canonicalInvalid(CodeValueInvalid, "value")
	}
	if len(value) > MaxCanonicalStringBytes {
		return IdentityField{}, canonicalInvalid(CodeLimitExceeded, "value")
	}
	return newIdentityField(name, IdentityFieldText, identityFieldTextTag, []byte(value), value, CanonicalValue{}, SecretReference{})
}

// NewCanonicalValueIdentityField requires the complete tree, never a bare token.
func NewCanonicalValueIdentityField(name string, value CanonicalValue) (IdentityField, error) {
	if !value.Valid() {
		return IdentityField{}, canonicalInvalid(CodeShapeInvalid, "value")
	}
	token := value.Token()
	return newIdentityField(name, IdentityFieldCanonicalValue, identityFieldCanonicalValueTag, token.digest[:], "", value, SecretReference{})
}

// NewSecretReferenceIdentityField stores only the non-secret stable reference.
func NewSecretReferenceIdentityField(name string, value SecretReference) (IdentityField, error) {
	if !value.Valid() {
		return IdentityField{}, canonicalInvalid(CodeShapeInvalid, "reference")
	}
	payload := append(encodeString(value.Provider()), encodeString(value.Identifier())...)
	payload = append(payload, encodeString(value.Version())...)
	return newIdentityField(name, IdentityFieldSecretReference, identityFieldSecretReferenceTag, payload, "", CanonicalValue{}, value)
}

func newIdentityField(name string, kind IdentityFieldKind, tag uint16, payload []byte, text string, value CanonicalValue, reference SecretReference) (IdentityField, error) {
	if err := validateIdentifier(name, "name"); err != nil {
		return IdentityField{}, canonicalizeValidationError(err)
	}
	preimage := append(encodeString(name), encodeUint16(tag)...)
	preimage = append(preimage, encodeBytes(payload)...)
	digest := sha256.Sum256(preimage)
	data := &identityFieldData{name: name, kind: kind, payload: append([]byte(nil), payload...), text: text, value: value, reference: reference, commitment: CanonicalFingerprint("sha256:" + hexLower(digest[:]))}
	return IdentityField{data: data}, nil
}

func hexLower(value []byte) string {
	const digits = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for index, b := range value {
		result[index*2], result[index*2+1] = digits[b>>4], digits[b&15]
	}
	return string(result)
}

func (f IdentityField) Valid() bool { return f.data != nil }
func (f IdentityField) Name() string {
	if f.data == nil {
		return ""
	}
	return f.data.name
}
func (f IdentityField) Kind() IdentityFieldKind {
	if f.data == nil {
		return ""
	}
	return f.data.kind
}
func (f IdentityField) Commitment() CanonicalFingerprint {
	if f.data == nil {
		return ""
	}
	return f.data.commitment
}
func (f IdentityField) Text() string {
	if f.data == nil || f.data.kind != IdentityFieldText {
		return ""
	}
	return f.data.text
}
func (f IdentityField) CanonicalValue() CanonicalValue {
	if f.data == nil || f.data.kind != IdentityFieldCanonicalValue {
		return CanonicalValue{}
	}
	return f.data.value
}
func (f IdentityField) SecretReference() SecretReference {
	if f.data == nil || f.data.kind != IdentityFieldSecretReference {
		return SecretReference{}
	}
	return f.data.reference
}
