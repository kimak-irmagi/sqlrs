package runtimev2

import (
	"context"
	"encoding/hex"
	"encoding/json"
)

const CanonicalEnvelopeVersion = "sqlrs.runtime.fingerprint-envelope.v1"

// FingerprintKind selects one fixed descriptor/domain mapping.
type FingerprintKind string

const (
	FingerprintKindCanonicalValue    FingerprintKind = "canonical-value"
	FingerprintKindFactoryState      FingerprintKind = "factory-state"
	FingerprintKindTransform         FingerprintKind = "transform"
	FingerprintKindResolvedExtension FingerprintKind = "resolved-extension"
	FingerprintKindDerivedState      FingerprintKind = "derived-state"
)

type fingerprintDescriptorData struct {
	kind                                   FingerprintKind
	domain, digest                         string
	provider, semanticKind, identitySchema string
}

// FingerprintDescriptor is available only from a verified envelope.
type FingerprintDescriptor struct{ data *fingerprintDescriptorData }

// EnvelopeVersion returns the strict transport-envelope revision.
func (d FingerprintDescriptor) EnvelopeVersion() string {
	if d.data == nil {
		return ""
	}
	return CanonicalEnvelopeVersion
}

// SchemaVersion returns the semantic schema governing canonical bytes.
func (d FingerprintDescriptor) SchemaVersion() string {
	if d.data == nil {
		return ""
	}
	return CanonicalSchemaVersion
}

// Algorithm returns the only hash algorithm permitted by canonical-v1.
func (d FingerprintDescriptor) Algorithm() string {
	if d.data == nil {
		return ""
	}
	return "sha256"
}

func (d FingerprintDescriptor) Kind() FingerprintKind {
	if d.data == nil {
		return ""
	}
	return d.data.kind
}
func (d FingerprintDescriptor) Domain() string {
	if d.data == nil {
		return ""
	}
	return d.data.domain
}
func (d FingerprintDescriptor) Digest() CanonicalFingerprint {
	if d.data == nil {
		return ""
	}
	return CanonicalFingerprint(d.data.digest)
}
func (d FingerprintDescriptor) Provider() string {
	if d.data == nil {
		return ""
	}
	return d.data.provider
}
func (d FingerprintDescriptor) SemanticKind() string {
	if d.data == nil {
		return ""
	}
	return d.data.semanticKind
}
func (d FingerprintDescriptor) IdentitySchema() string {
	if d.data == nil {
		return ""
	}
	return d.data.identitySchema
}

type fingerprintEnvelopeData struct {
	descriptor fingerprintDescriptorData
	value      CanonicalValue
	identity   *canonicalIdentityData
	parent     CanonicalStateID
	transform  *fingerprintEnvelopeData
}

// FingerprintEnvelope couples a descriptor to the full recomputable subject.
type FingerprintEnvelope struct{ data *fingerprintEnvelopeData }

func (e FingerprintEnvelope) Valid() bool { return e.data != nil }
func (e FingerprintEnvelope) Descriptor() FingerprintDescriptor {
	if e.data == nil {
		return FingerprintDescriptor{}
	}
	data := e.data.descriptor
	return FingerprintDescriptor{data: &data}
}

func NewCanonicalValueEnvelope(value CanonicalValue) (FingerprintEnvelope, error) {
	if !value.Valid() {
		return FingerprintEnvelope{}, canonicalInvalid(CodeShapeInvalid, "subject")
	}
	token := value.Token()
	digest := "sha256:" + token.String()[len("civ1:sha256:"):]
	return FingerprintEnvelope{data: &fingerprintEnvelopeData{descriptor: fingerprintDescriptorData{kind: FingerprintKindCanonicalValue, domain: CanonicalIdentityValueDomain, digest: digest}, value: value}}, nil
}

func newIdentityEnvelope(kind FingerprintKind, domain string, data *canonicalIdentityData) (FingerprintEnvelope, error) {
	fingerprint, err := canonicalFingerprint(domain, data)
	if err != nil {
		return FingerprintEnvelope{}, err
	}
	copyData := copyCanonicalIdentityData(data)
	return FingerprintEnvelope{data: &fingerprintEnvelopeData{descriptor: fingerprintDescriptorData{
		kind: kind, domain: domain, digest: string(fingerprint), provider: data.provider,
		semanticKind: data.semanticKind, identitySchema: data.identitySchema,
	}, identity: copyData}}, nil
}

func NewFactoryEnvelope(identity CanonicalFactoryIdentity) (FingerprintEnvelope, error) {
	return newIdentityEnvelope(FingerprintKindFactoryState, CanonicalFactoryStateDomain, identity.data)
}
func NewTransformEnvelope(identity CanonicalTransformIdentity) (FingerprintEnvelope, error) {
	return newIdentityEnvelope(FingerprintKindTransform, CanonicalTransformDomain, identity.data)
}
func NewExtensionEnvelope(identity CanonicalResolvedExtensionIdentity) (FingerprintEnvelope, error) {
	return newIdentityEnvelope(FingerprintKindResolvedExtension, CanonicalResolvedExtensionDomain, identity.data)
}

// CanonicalDerivedStateBytes encodes a parent and a verified transform digest.
func CanonicalDerivedStateBytes(parent CanonicalStateID, transform FingerprintEnvelope) ([]byte, error) {
	parentDigest, err := parseDigest(string(parent), "parent")
	if err != nil {
		return nil, canonicalizeValidationError(err)
	}
	if transform.data == nil || transform.data.descriptor.kind != FingerprintKindTransform {
		return nil, canonicalInvalid(CodeDescriptorInvalid, "transform.descriptor.kind")
	}
	transformDigest, err := parseDigest(transform.data.descriptor.digest, "transform.descriptor.digest")
	if err != nil {
		return nil, canonicalizeValidationError(err)
	}
	return encodeRecord(CanonicalDerivedStateDomain, []canonicalField{{tag: 1, payload: parentDigest}, {tag: 2, payload: transformDigest}}), nil
}

// NewDerivedStateEnvelope derives and retains the verified transform subject.
func NewDerivedStateEnvelope(parent CanonicalStateID, transform FingerprintEnvelope) (FingerprintEnvelope, error) {
	encoded, err := CanonicalDerivedStateBytes(parent, transform)
	if err != nil {
		return FingerprintEnvelope{}, err
	}
	digest := hashBytes(encoded)
	copyTransform := *transform.data
	copyTransform.identity = copyCanonicalIdentityData(transform.data.identity)
	return FingerprintEnvelope{data: &fingerprintEnvelopeData{descriptor: fingerprintDescriptorData{kind: FingerprintKindDerivedState, domain: CanonicalDerivedStateDomain, digest: digest}, parent: parent, transform: &copyTransform}}, nil
}

type descriptorWire struct {
	EnvelopeVersion string          `json:"envelope_version"`
	SchemaVersion   string          `json:"schema_version"`
	Kind            FingerprintKind `json:"kind"`
	Domain          string          `json:"domain"`
	Algorithm       string          `json:"algorithm"`
	Digest          string          `json:"digest"`
	Provider        string          `json:"provider,omitempty"`
	SemanticKind    string          `json:"semantic_kind,omitempty"`
	IdentitySchema  string          `json:"identity_schema,omitempty"`
}

type envelopeWire struct {
	Descriptor descriptorWire  `json:"descriptor"`
	Subject    json.RawMessage `json:"subject"`
}

type valueSubjectWire struct {
	CanonicalHex string `json:"canonical_hex"`
	Token        string `json:"token"`
}

type derivedSubjectWire struct {
	Parent    string          `json:"parent"`
	Transform json.RawMessage `json:"transform"`
}

type secretReferenceWire struct {
	Provider   string `json:"provider"`
	Identifier string `json:"identifier"`
	Version    string `json:"version"`
}

type identityFieldWire struct {
	Name         string               `json:"name"`
	Kind         IdentityFieldKind    `json:"kind"`
	Disclosure   DisclosureClass      `json:"disclosure"`
	Text         *string              `json:"text,omitempty"`
	CanonicalHex string               `json:"canonical_hex,omitempty"`
	Token        string               `json:"token,omitempty"`
	Secret       *secretReferenceWire `json:"secret_reference,omitempty"`
	Commitment   string               `json:"commitment"`
}

type identitySubjectWire struct {
	Fields *[]identityFieldWire `json:"fields"`
}

func descriptorToWire(value fingerprintDescriptorData) descriptorWire {
	return descriptorWire{EnvelopeVersion: CanonicalEnvelopeVersion, SchemaVersion: CanonicalSchemaVersion,
		Kind: value.kind, Domain: value.domain, Algorithm: "sha256", Digest: value.digest,
		Provider: value.provider, SemanticKind: value.semanticKind, IdentitySchema: value.identitySchema}
}

func fieldToWire(item canonicalIdentityField) identityFieldWire {
	field := item.field
	wire := identityFieldWire{Name: field.Name(), Kind: field.Kind(), Disclosure: item.disclosure, Commitment: string(field.Commitment())}
	switch field.Kind() {
	case IdentityFieldText:
		value := field.Text()
		wire.Text = &value
	case IdentityFieldCanonicalValue:
		value := field.CanonicalValue()
		wire.CanonicalHex = hex.EncodeToString(value.CanonicalBytes())
		wire.Token = value.Token().String()
	case IdentityFieldSecretReference:
		value := field.SecretReference()
		wire.Secret = &secretReferenceWire{Provider: value.Provider(), Identifier: value.Identifier(), Version: value.Version()}
	}
	return wire
}

func (e FingerprintEnvelope) MarshalJSON() ([]byte, error) {
	if e.data == nil {
		return nil, canonicalInvalid(CodeShapeInvalid, "$")
	}
	var subject []byte
	var err error
	if e.data.descriptor.kind == FingerprintKindCanonicalValue {
		subject, err = json.Marshal(valueSubjectWire{CanonicalHex: hex.EncodeToString(e.data.value.CanonicalBytes()), Token: e.data.value.Token().String()})
	} else if e.data.descriptor.kind == FingerprintKindDerivedState {
		transform, marshalErr := json.Marshal(FingerprintEnvelope{data: e.data.transform})
		if marshalErr != nil {
			return nil, marshalErr
		}
		subject, err = json.Marshal(derivedSubjectWire{Parent: string(e.data.parent), Transform: transform})
	} else {
		fields := make([]identityFieldWire, len(e.data.identity.fields))
		for index, field := range e.data.identity.fields {
			fields[index] = fieldToWire(field)
		}
		subject, err = json.Marshal(identitySubjectWire{Fields: &fields})
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelopeWire{Descriptor: descriptorToWire(e.data.descriptor), Subject: subject})
}

func expectedDomain(kind FingerprintKind) string {
	switch kind {
	case FingerprintKindCanonicalValue:
		return CanonicalIdentityValueDomain
	case FingerprintKindFactoryState:
		return CanonicalFactoryStateDomain
	case FingerprintKindTransform:
		return CanonicalTransformDomain
	case FingerprintKindResolvedExtension:
		return CanonicalResolvedExtensionDomain
	case FingerprintKindDerivedState:
		return CanonicalDerivedStateDomain
	default:
		return ""
	}
}

func validateDescriptor(wire descriptorWire) error {
	if wire.EnvelopeVersion != CanonicalEnvelopeVersion || wire.SchemaVersion != CanonicalSchemaVersion {
		return canonicalInvalid(CodeRevisionMismatch, "descriptor.schema_version")
	}
	if wire.Algorithm != "sha256" || expectedDomain(wire.Kind) == "" || wire.Domain != expectedDomain(wire.Kind) {
		return canonicalInvalid(CodeDescriptorInvalid, "descriptor")
	}
	if _, err := parseDigest(wire.Digest, "descriptor.digest"); err != nil {
		return canonicalInvalid(CodeDescriptorInvalid, "descriptor.digest")
	}
	identityKind := wire.Kind == FingerprintKindFactoryState || wire.Kind == FingerprintKindTransform || wire.Kind == FingerprintKindResolvedExtension
	if identityKind {
		if validateIdentifier(wire.Provider, "descriptor.provider") != nil || validateIdentifier(wire.SemanticKind, "descriptor.semantic_kind") != nil || validateIdentifier(wire.IdentitySchema, "descriptor.identity_schema") != nil {
			return canonicalInvalid(CodeDescriptorInvalid, "descriptor")
		}
	} else if wire.Provider != "" || wire.SemanticKind != "" || wire.IdentitySchema != "" {
		return canonicalInvalid(CodeDescriptorInvalid, "descriptor")
	}
	return nil
}

func decodeIdentityField(wire identityFieldWire, index int) (canonicalIdentityField, error) {
	path := "subject.fields[" + itoa(index) + "]"
	var field IdentityField
	var err error
	switch wire.Kind {
	case IdentityFieldText:
		if wire.Text == nil || wire.CanonicalHex != "" || wire.Token != "" || wire.Secret != nil {
			return canonicalIdentityField{}, canonicalInvalid(CodeShapeInvalid, path)
		}
		field, err = NewTextIdentityField(wire.Name, *wire.Text)
	case IdentityFieldCanonicalValue:
		if wire.Text != nil || wire.CanonicalHex == "" || wire.Token == "" || wire.Secret != nil {
			return canonicalIdentityField{}, canonicalInvalid(CodeShapeInvalid, path)
		}
		raw, decodeErr := hex.DecodeString(wire.CanonicalHex)
		if decodeErr != nil {
			return canonicalIdentityField{}, canonicalInvalid(CodeShapeInvalid, path+".canonical_hex")
		}
		value, decodeErr := ParseCanonicalValueEnvelope(raw)
		if decodeErr != nil {
			return canonicalIdentityField{}, prefixError(decodeErr, path+".canonical_hex")
		}
		if value.Token().String() != wire.Token {
			return canonicalIdentityField{}, canonicalInvalid(CodeCommitmentMismatch, path+".token")
		}
		field, err = NewCanonicalValueIdentityField(wire.Name, value)
	case IdentityFieldSecretReference:
		if wire.Text != nil || wire.CanonicalHex != "" || wire.Token != "" || wire.Secret == nil {
			return canonicalIdentityField{}, canonicalInvalid(CodeShapeInvalid, path)
		}
		reference, referenceErr := NewSecretReference(wire.Secret.Provider, wire.Secret.Identifier, wire.Secret.Version)
		if referenceErr != nil {
			return canonicalIdentityField{}, prefixError(referenceErr, path+".secret_reference")
		}
		field, err = NewSecretReferenceIdentityField(wire.Name, reference)
	default:
		return canonicalIdentityField{}, canonicalInvalid(CodeShapeInvalid, path+".kind")
	}
	if err != nil {
		return canonicalIdentityField{}, prefixError(err, path)
	}
	if string(field.Commitment()) != wire.Commitment {
		return canonicalIdentityField{}, canonicalInvalid(CodeCommitmentMismatch, path+".commitment")
	}
	if wire.Disclosure != DisclosurePublic && wire.Disclosure != DisclosureProtected {
		return canonicalIdentityField{}, canonicalInvalid(CodeShapeInvalid, path+".disclosure")
	}
	return canonicalIdentityField{field: field, disclosure: wire.Disclosure}, nil
}

func (e *FingerprintEnvelope) UnmarshalJSON(raw []byte) error {
	if e == nil {
		return canonicalInvalid(CodeShapeInvalid, "")
	}
	if len(raw) > MaxCanonicalEnvelopeBytes {
		return canonicalInvalid(CodeDocumentTooLarge, "")
	}
	var wire envelopeWire
	if err := decodeCanonicalStrict(raw, &wire); err != nil {
		return err
	}
	if len(wire.Subject) == 0 || isJSONNull(wire.Subject) {
		return canonicalInvalid(CodeShapeInvalid, "subject")
	}
	if err := validateDescriptor(wire.Descriptor); err != nil {
		return err
	}
	data := &fingerprintEnvelopeData{descriptor: fingerprintDescriptorData{kind: wire.Descriptor.Kind, domain: wire.Descriptor.Domain, digest: wire.Descriptor.Digest, provider: wire.Descriptor.Provider, semanticKind: wire.Descriptor.SemanticKind, identitySchema: wire.Descriptor.IdentitySchema}}
	if wire.Descriptor.Kind == FingerprintKindCanonicalValue {
		var subject valueSubjectWire
		if err := decodeCanonicalStrict(wire.Subject, &subject); err != nil {
			return prefixError(err, "subject")
		}
		encoded, err := hex.DecodeString(subject.CanonicalHex)
		if err != nil {
			return canonicalInvalid(CodeShapeInvalid, "subject.canonical_hex")
		}
		value, err := ParseCanonicalValueEnvelope(encoded)
		if err != nil {
			return prefixError(err, "subject.canonical_hex")
		}
		if value.Token().String() != subject.Token {
			return canonicalInvalid(CodeCommitmentMismatch, "subject.token")
		}
		digest := "sha256:" + subject.Token[len("civ1:sha256:"):]
		if digest != wire.Descriptor.Digest {
			return canonicalInvalid(CodeDigestMismatch, "descriptor.digest")
		}
		data.value = value
	} else if wire.Descriptor.Kind == FingerprintKindDerivedState {
		var subject derivedSubjectWire
		if err := decodeCanonicalStrict(wire.Subject, &subject); err != nil {
			return prefixError(err, "subject")
		}
		if len(subject.Transform) == 0 || isJSONNull(subject.Transform) {
			return canonicalInvalid(CodeShapeInvalid, "subject.transform")
		}
		var transform FingerprintEnvelope
		if err := json.Unmarshal(subject.Transform, &transform); err != nil {
			return prefixError(err, "subject.transform")
		}
		parent := CanonicalStateID(subject.Parent)
		encoded, err := CanonicalDerivedStateBytes(parent, transform)
		if err != nil {
			return prefixError(err, "subject")
		}
		if hashBytes(encoded) != wire.Descriptor.Digest {
			return canonicalInvalid(CodeDigestMismatch, "descriptor.digest")
		}
		copyTransform := *transform.data
		data.parent, data.transform = parent, &copyTransform
	} else {
		var subject identitySubjectWire
		if err := decodeCanonicalStrict(wire.Subject, &subject); err != nil {
			return prefixError(err, "subject")
		}
		if subject.Fields == nil {
			return canonicalInvalid(CodeShapeInvalid, "subject.fields")
		}
		fields := make([]canonicalIdentityField, len(*subject.Fields))
		for index, fieldWire := range *subject.Fields {
			field, err := decodeIdentityField(fieldWire, index)
			if err != nil {
				return err
			}
			if index > 0 && fields[index-1].field.Name() >= field.field.Name() {
				return canonicalInvalid(CodeNonCanonical, "subject.fields["+itoa(index)+"].name")
			}
			fields[index] = field
		}
		identity := &canonicalIdentityData{provider: wire.Descriptor.Provider, semanticKind: wire.Descriptor.SemanticKind, identitySchema: wire.Descriptor.IdentitySchema, fields: fields}
		fingerprint, err := canonicalFingerprint(wire.Descriptor.Domain, identity)
		if err != nil {
			return err
		}
		if string(fingerprint) != wire.Descriptor.Digest {
			return canonicalInvalid(CodeDigestMismatch, "descriptor.digest")
		}
		data.identity = identity
	}
	e.data = data
	return nil
}

// InternalDisclosureAuthorizer makes the privileged disclosure decision before
// any explanation value is allocated.
type InternalDisclosureAuthorizer interface{ AuthorizeInternalDisclosure(context.Context) error }

type explanationFieldWire struct {
	Name         string               `json:"name"`
	Kind         IdentityFieldKind    `json:"kind"`
	Redacted     bool                 `json:"redacted"`
	Text         *string              `json:"text,omitempty"`
	CanonicalHex string               `json:"canonical_hex,omitempty"`
	Secret       *secretReferenceWire `json:"secret_reference,omitempty"`
	Commitment   string               `json:"commitment,omitempty"`
}
type explanationWire struct {
	Profile    string                 `json:"profile"`
	Descriptor descriptorWire         `json:"descriptor"`
	Fields     []explanationFieldWire `json:"fields,omitempty"`
}

// FingerprintExplanation is a one-way projection with no trust-producing decoder.
type FingerprintExplanation struct{ data *explanationWire }

func (e FingerprintExplanation) Valid() bool { return e.data != nil }
func (e FingerprintExplanation) MarshalJSON() ([]byte, error) {
	if e.data == nil {
		return nil, canonicalInvalid(CodeShapeInvalid, "$")
	}
	return json.Marshal(e.data)
}

func explain(envelope FingerprintEnvelope, internal bool) (FingerprintExplanation, error) {
	if envelope.data == nil {
		return FingerprintExplanation{}, canonicalInvalid(CodeShapeInvalid, "envelope")
	}
	wire := &explanationWire{Profile: "safe", Descriptor: descriptorToWire(envelope.data.descriptor)}
	if internal {
		wire.Profile = "internal"
	}
	if envelope.data.identity != nil {
		wire.Fields = make([]explanationFieldWire, len(envelope.data.identity.fields))
		for index, item := range envelope.data.identity.fields {
			field := item.field
			out := explanationFieldWire{Name: field.Name(), Kind: field.Kind()}
			if !internal && (item.disclosure == DisclosureProtected || field.Kind() == IdentityFieldSecretReference) {
				out.Redacted = true
			} else {
				out.Commitment = string(field.Commitment())
				switch field.Kind() {
				case IdentityFieldText:
					value := field.Text()
					out.Text = &value
				case IdentityFieldCanonicalValue:
					out.CanonicalHex = hex.EncodeToString(field.CanonicalValue().CanonicalBytes())
				case IdentityFieldSecretReference:
					value := field.SecretReference()
					out.Secret = &secretReferenceWire{Provider: value.Provider(), Identifier: value.Identifier(), Version: value.Version()}
				}
			}
			wire.Fields[index] = out
		}
	}
	return FingerprintExplanation{data: wire}, nil
}

func ExplainSafe(envelope FingerprintEnvelope) (FingerprintExplanation, error) {
	return explain(envelope, false)
}
func ExplainInternal(ctx context.Context, envelope FingerprintEnvelope, authorizer InternalDisclosureAuthorizer) (FingerprintExplanation, error) {
	if ctx == nil || ctx.Err() != nil || authorizer == nil {
		return FingerprintExplanation{}, canonicalInvalid(CodeAuthorizationDenied, "")
	}
	if err := authorizer.AuthorizeInternalDisclosure(ctx); err != nil {
		return FingerprintExplanation{}, canonicalInvalid(CodeAuthorizationDenied, "")
	}
	if ctx.Err() != nil {
		return FingerprintExplanation{}, canonicalInvalid(CodeAuthorizationDenied, "")
	}
	return explain(envelope, true)
}
