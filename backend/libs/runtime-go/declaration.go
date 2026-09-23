package runtimev2

import (
	"encoding/json"
	"sort"
)

// FactoryDeclaration records unresolved, non-identity-bearing factory input.
type FactoryDeclaration struct {
	Kind       string            `json:"kind"`
	Reference  string            `json:"reference"`
	Arguments  []string          `json:"arguments"`
	Attributes map[string]string `json:"attributes"`
}

// TransformDeclaration records unresolved, non-identity-bearing transform input.
type TransformDeclaration struct {
	Kind       string            `json:"kind"`
	Reference  string            `json:"reference"`
	Arguments  []string          `json:"arguments"`
	Attributes map[string]string `json:"attributes"`
}

// ResolverObservation records non-identity-bearing resolver diagnostics.
type ResolverObservation struct {
	Implementation string `json:"implementation"`
	Version        string `json:"version"`
}

func validateDeclaration(kind, reference string, arguments []string, attributes map[string]string) error {
	if err := validateIdentifier(kind, "kind"); err != nil {
		return err
	}
	if err := validateUTF8(reference, "reference", false, MaxResolvedValueBytes); err != nil {
		return err
	}
	if len(arguments) > MaxArguments {
		return invalid(CodeTooLarge, "arguments")
	}
	for index, argument := range arguments {
		if err := validateUTF8(argument, "arguments["+itoa(index)+"]", false, MaxResolvedValueBytes); err != nil {
			return err
		}
	}
	if len(attributes) > MaxAttributes {
		return invalid(CodeTooLarge, "attributes")
	}
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := validateIdentifier(key, "attributes."+key); err != nil {
			return err
		}
		if err := validateUTF8(attributes[key], "attributes."+key, true, MaxResolvedValueBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateResolver(value *ResolverObservation) error {
	if value == nil {
		return nil
	}
	if err := validateIdentifier(value.Implementation, "implementation"); err != nil {
		return err
	}
	return validateUTF8(value.Version, "version", false, MaxIdentifierBytes)
}

func copyAttributes(source map[string]string) map[string]string {
	if source == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func copyStrings(source []string) []string {
	result := make([]string, len(source))
	copy(result, source)
	return result
}

func copyFactoryDeclaration(source *FactoryDeclaration) *FactoryDeclaration {
	if source == nil {
		return nil
	}
	return &FactoryDeclaration{Kind: source.Kind, Reference: source.Reference, Arguments: copyStrings(source.Arguments), Attributes: copyAttributes(source.Attributes)}
}

func copyTransformDeclaration(source *TransformDeclaration) *TransformDeclaration {
	if source == nil {
		return nil
	}
	return &TransformDeclaration{Kind: source.Kind, Reference: source.Reference, Arguments: copyStrings(source.Arguments), Attributes: copyAttributes(source.Attributes)}
}

func copyResolver(source *ResolverObservation) *ResolverObservation {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

type declarationWire struct {
	Kind       *string            `json:"kind"`
	Reference  *string            `json:"reference"`
	Arguments  *[]string          `json:"arguments"`
	Attributes *map[string]string `json:"attributes"`
}

func declarationFromWire(w declarationWire) (string, string, []string, map[string]string, error) {
	if w.Kind == nil {
		return "", "", nil, nil, invalid(CodeInvalidShape, "kind")
	}
	if w.Reference == nil {
		return "", "", nil, nil, invalid(CodeInvalidShape, "reference")
	}
	if w.Arguments == nil {
		return "", "", nil, nil, invalid(CodeInvalidShape, "arguments")
	}
	if w.Attributes == nil {
		return "", "", nil, nil, invalid(CodeInvalidShape, "attributes")
	}
	if err := validateDeclaration(*w.Kind, *w.Reference, *w.Arguments, *w.Attributes); err != nil {
		return "", "", nil, nil, err
	}
	return *w.Kind, *w.Reference, copyStrings(*w.Arguments), copyAttributes(*w.Attributes), nil
}

func (d FactoryDeclaration) MarshalJSON() ([]byte, error) {
	if err := validateDeclaration(d.Kind, d.Reference, d.Arguments, d.Attributes); err != nil {
		return nil, err
	}
	args, attrs := copyStrings(d.Arguments), copyAttributes(d.Attributes)
	return json.Marshal(declarationWire{Kind: &d.Kind, Reference: &d.Reference, Arguments: &args, Attributes: &attrs})
}
func (d *FactoryDeclaration) UnmarshalJSON(data []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire declarationWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	kind, ref, args, attrs, err := declarationFromWire(wire)
	if err != nil {
		return err
	}
	*d = FactoryDeclaration{Kind: kind, Reference: ref, Arguments: args, Attributes: attrs}
	return nil
}
func (d TransformDeclaration) MarshalJSON() ([]byte, error) {
	if err := validateDeclaration(d.Kind, d.Reference, d.Arguments, d.Attributes); err != nil {
		return nil, err
	}
	args, attrs := copyStrings(d.Arguments), copyAttributes(d.Attributes)
	return json.Marshal(declarationWire{Kind: &d.Kind, Reference: &d.Reference, Arguments: &args, Attributes: &attrs})
}
func (d *TransformDeclaration) UnmarshalJSON(data []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire declarationWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	kind, ref, args, attrs, err := declarationFromWire(wire)
	if err != nil {
		return err
	}
	*d = TransformDeclaration{Kind: kind, Reference: ref, Arguments: args, Attributes: attrs}
	return nil
}
