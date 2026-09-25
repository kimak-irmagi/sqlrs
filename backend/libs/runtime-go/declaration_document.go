package runtimev2

import "encoding/json"

// FactoryDeclarationDocument is the versioned standalone transport for an
// unresolved factory declaration. Requirements: runtime-v2-declaration-structure.md.
type FactoryDeclarationDocument struct{ declaration *FactoryDeclaration }

// TransformDeclarationDocument is the versioned standalone transport for an
// unresolved transform declaration.
type TransformDeclarationDocument struct{ declaration *TransformDeclaration }

// NewFactoryDeclarationDocument validates and snapshots a declaration.
func NewFactoryDeclarationDocument(value FactoryDeclaration) (FactoryDeclarationDocument, error) {
	if err := validateFactoryDeclaration(&value); err != nil {
		return FactoryDeclarationDocument{}, err
	}
	return FactoryDeclarationDocument{declaration: copyFactoryDeclaration(&value)}, nil
}

// NewTransformDeclarationDocument validates and snapshots a declaration.
func NewTransformDeclarationDocument(value TransformDeclaration) (TransformDeclarationDocument, error) {
	if err := validateTransformDeclaration(&value); err != nil {
		return TransformDeclarationDocument{}, err
	}
	return TransformDeclarationDocument{declaration: copyTransformDeclaration(&value)}, nil
}

// Declaration returns a defensive copy.
func (d FactoryDeclarationDocument) Declaration() FactoryDeclaration {
	if d.declaration == nil {
		return FactoryDeclaration{}
	}
	return *copyFactoryDeclaration(d.declaration)
}

// Declaration returns a defensive copy.
func (d TransformDeclarationDocument) Declaration() TransformDeclaration {
	if d.declaration == nil {
		return TransformDeclaration{}
	}
	return *copyTransformDeclaration(d.declaration)
}

type factoryDeclarationDocumentWire struct {
	SchemaVersion *string             `json:"schema_version"`
	Declaration   *FactoryDeclaration `json:"declaration"`
}
type transformDeclarationDocumentWire struct {
	SchemaVersion *string               `json:"schema_version"`
	Declaration   *TransformDeclaration `json:"declaration"`
}

func (d FactoryDeclarationDocument) MarshalJSON() ([]byte, error) {
	if d.declaration == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	version := SchemaVersion
	return json.Marshal(factoryDeclarationDocumentWire{&version, copyFactoryDeclaration(d.declaration)})
}
func (d *FactoryDeclarationDocument) UnmarshalJSON(raw []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire factoryDeclarationDocumentWire
	if err := decodeStrict(raw, &wire); err != nil {
		return err
	}
	if wire.SchemaVersion == nil {
		return invalid(CodeInvalidShape, "schema_version")
	}
	if *wire.SchemaVersion != SchemaVersion {
		return invalid(CodeInvalidVersion, "schema_version")
	}
	if wire.Declaration == nil {
		return invalid(CodeInvalidShape, "declaration")
	}
	value, err := NewFactoryDeclarationDocument(*wire.Declaration)
	if err != nil {
		return prefixError(err, "declaration")
	}
	d.declaration = value.declaration
	return nil
}
func (d TransformDeclarationDocument) MarshalJSON() ([]byte, error) {
	if d.declaration == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	version := SchemaVersion
	return json.Marshal(transformDeclarationDocumentWire{&version, copyTransformDeclaration(d.declaration)})
}
func (d *TransformDeclarationDocument) UnmarshalJSON(raw []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire transformDeclarationDocumentWire
	if err := decodeStrict(raw, &wire); err != nil {
		return err
	}
	if wire.SchemaVersion == nil {
		return invalid(CodeInvalidShape, "schema_version")
	}
	if *wire.SchemaVersion != SchemaVersion {
		return invalid(CodeInvalidVersion, "schema_version")
	}
	if wire.Declaration == nil {
		return invalid(CodeInvalidShape, "declaration")
	}
	value, err := NewTransformDeclarationDocument(*wire.Declaration)
	if err != nil {
		return prefixError(err, "declaration")
	}
	d.declaration = value.declaration
	return nil
}
