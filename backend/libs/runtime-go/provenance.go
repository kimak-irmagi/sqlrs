package runtimev2

import (
	"encoding/json"
)

// FactoryProvenanceInput combines immutable identity with optional diagnostics.
type FactoryProvenanceInput struct {
	Identity    ResolvedFactoryIdentity
	Declaration *FactoryDeclaration
	Resolver    *ResolverObservation
}

// TransformProvenanceInput combines immutable identity with optional diagnostics.
type TransformProvenanceInput struct {
	Identity    ResolvedTransformIdentity
	Declaration *TransformDeclaration
	Resolver    *ResolverObservation
}

type factoryProvenanceData struct {
	identity    ResolvedFactoryIdentity
	declaration *FactoryDeclaration
	resolver    *ResolverObservation
}
type transformProvenanceData struct {
	identity    ResolvedTransformIdentity
	declaration *TransformDeclaration
	resolver    *ResolverObservation
}

// FactoryProvenance separates resolved identity from factory diagnostics.
type FactoryProvenance struct{ data *factoryProvenanceData }

// TransformProvenance separates resolved identity from transform diagnostics.
type TransformProvenance struct{ data *transformProvenanceData }

// NewFactoryProvenance validates diagnostics and takes defensive copies.
func NewFactoryProvenance(input FactoryProvenanceInput) (FactoryProvenance, error) {
	if !input.Identity.valid() {
		return FactoryProvenance{}, invalid(CodeInvalidShape, "identity")
	}
	if input.Declaration != nil {
		if err := validateFactoryDeclaration(input.Declaration); err != nil {
			return FactoryProvenance{}, prefixError(err, "declaration")
		}
	}
	if err := validateResolver(input.Resolver); err != nil {
		return FactoryProvenance{}, prefixError(err, "resolver")
	}
	return FactoryProvenance{data: &factoryProvenanceData{identity: input.Identity, declaration: copyFactoryDeclaration(input.Declaration), resolver: copyResolver(input.Resolver)}}, nil
}

// NewTransformProvenance validates diagnostics and takes defensive copies.
func NewTransformProvenance(input TransformProvenanceInput) (TransformProvenance, error) {
	if !input.Identity.valid() {
		return TransformProvenance{}, invalid(CodeInvalidShape, "identity")
	}
	if input.Declaration != nil {
		if err := validateTransformDeclaration(input.Declaration); err != nil {
			return TransformProvenance{}, prefixError(err, "declaration")
		}
	}
	if err := validateResolver(input.Resolver); err != nil {
		return TransformProvenance{}, prefixError(err, "resolver")
	}
	return TransformProvenance{data: &transformProvenanceData{identity: input.Identity, declaration: copyTransformDeclaration(input.Declaration), resolver: copyResolver(input.Resolver)}}, nil
}

// Identity returns the immutable resolved factory identity.
func (p FactoryProvenance) Identity() ResolvedFactoryIdentity {
	if p.data == nil {
		return ResolvedFactoryIdentity{}
	}
	return p.data.identity
}

// Identity returns the immutable resolved transform identity.
func (p TransformProvenance) Identity() ResolvedTransformIdentity {
	if p.data == nil {
		return ResolvedTransformIdentity{}
	}
	return p.data.identity
}

// Declaration returns a defensive copy of optional factory diagnostics.
func (p FactoryProvenance) Declaration() *FactoryDeclaration {
	if p.data == nil {
		return nil
	}
	return copyFactoryDeclaration(p.data.declaration)
}

// Declaration returns a defensive copy of optional transform diagnostics.
func (p TransformProvenance) Declaration() *TransformDeclaration {
	if p.data == nil {
		return nil
	}
	return copyTransformDeclaration(p.data.declaration)
}

// Resolver returns a defensive copy of optional factory-resolution diagnostics.
func (p FactoryProvenance) Resolver() *ResolverObservation {
	if p.data == nil {
		return nil
	}
	return copyResolver(p.data.resolver)
}

// Resolver returns a defensive copy of optional transform-resolution diagnostics.
func (p TransformProvenance) Resolver() *ResolverObservation {
	if p.data == nil {
		return nil
	}
	return copyResolver(p.data.resolver)
}

type provenanceWire struct {
	Identity    json.RawMessage `json:"identity"`
	Declaration json.RawMessage `json:"declaration,omitempty"`
	Resolver    json.RawMessage `json:"resolver,omitempty"`
}

func (p FactoryProvenance) MarshalJSON() ([]byte, error) {
	if p.data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	identity, _ := json.Marshal(p.data.identity)
	wire := provenanceWire{Identity: identity}
	if p.data.declaration != nil {
		wire.Declaration, _ = json.Marshal(p.data.declaration)
	}
	if p.data.resolver != nil {
		wire.Resolver, _ = json.Marshal(p.data.resolver)
	}
	return json.Marshal(wire)
}

func (p *FactoryProvenance) UnmarshalJSON(data []byte) error {
	if p == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire provenanceWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	if len(wire.Identity) == 0 || isJSONNull(wire.Identity) {
		return invalid(CodeInvalidShape, "identity")
	}
	var identity ResolvedFactoryIdentity
	if err := json.Unmarshal(wire.Identity, &identity); err != nil {
		return prefixError(err, "identity")
	}
	var declaration *FactoryDeclaration
	if len(wire.Declaration) > 0 {
		if isJSONNull(wire.Declaration) {
			return invalid(CodeInvalidShape, "declaration")
		}
		declaration = &FactoryDeclaration{}
		if err := json.Unmarshal(wire.Declaration, declaration); err != nil {
			return prefixError(err, "declaration")
		}
	}
	resolver, err := decodeResolver(wire.Resolver)
	if err != nil {
		return prefixError(err, "resolver")
	}
	value, err := NewFactoryProvenance(FactoryProvenanceInput{Identity: identity, Declaration: declaration, Resolver: resolver})
	if err != nil {
		return err
	}
	*p = value
	return nil
}

func (p TransformProvenance) MarshalJSON() ([]byte, error) {
	if p.data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	identity, _ := json.Marshal(p.data.identity)
	wire := provenanceWire{Identity: identity}
	if p.data.declaration != nil {
		wire.Declaration, _ = json.Marshal(p.data.declaration)
	}
	if p.data.resolver != nil {
		wire.Resolver, _ = json.Marshal(p.data.resolver)
	}
	return json.Marshal(wire)
}

func (p *TransformProvenance) UnmarshalJSON(data []byte) error {
	if p == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire provenanceWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	if len(wire.Identity) == 0 || isJSONNull(wire.Identity) {
		return invalid(CodeInvalidShape, "identity")
	}
	var identity ResolvedTransformIdentity
	if err := json.Unmarshal(wire.Identity, &identity); err != nil {
		return prefixError(err, "identity")
	}
	var declaration *TransformDeclaration
	if len(wire.Declaration) > 0 {
		if isJSONNull(wire.Declaration) {
			return invalid(CodeInvalidShape, "declaration")
		}
		declaration = &TransformDeclaration{}
		if err := json.Unmarshal(wire.Declaration, declaration); err != nil {
			return prefixError(err, "declaration")
		}
	}
	resolver, err := decodeResolver(wire.Resolver)
	if err != nil {
		return prefixError(err, "resolver")
	}
	value, err := NewTransformProvenance(TransformProvenanceInput{Identity: identity, Declaration: declaration, Resolver: resolver})
	if err != nil {
		return err
	}
	*p = value
	return nil
}

func decodeResolver(raw json.RawMessage) (*ResolverObservation, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, invalid(CodeInvalidShape, "$")
	}
	type resolverWire struct {
		Implementation *string `json:"implementation"`
		Version        *string `json:"version"`
	}
	var wire resolverWire
	if err := decodeStrict(raw, &wire); err != nil {
		return nil, err
	}
	if wire.Implementation == nil {
		return nil, invalid(CodeInvalidShape, "implementation")
	}
	if wire.Version == nil {
		return nil, invalid(CodeInvalidShape, "version")
	}
	value := &ResolverObservation{Implementation: *wire.Implementation, Version: *wire.Version}
	if err := validateResolver(value); err != nil {
		return nil, err
	}
	return value, nil
}
