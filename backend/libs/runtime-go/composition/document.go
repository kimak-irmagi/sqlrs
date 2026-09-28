package composition

import (
	"encoding/json"
	"sort"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// AliasKind identifies one closed alias variant.
type AliasKind string

const (
	AliasKindTransform AliasKind = "transform"
	AliasKindRecipe    AliasKind = "recipe"
)

// DocumentInput is the duplicate-preserving authoring input for an alias document.
type DocumentInput struct{ Aliases []NamedAliasInput }

// NamedAliasInput binds one validated lower-case name to exactly one variant.
type NamedAliasInput struct {
	Name      string
	Transform *runtimev2.TransformDeclaration
	Recipe    *RecipeAliasInput
}

// RecipeAliasInput describes one factory/recipe prefix plus ordered transforms.
type RecipeAliasInput struct {
	Base  RecipeBaseInput
	Steps []RecipeStepInput
}

// RecipeBaseInput contains exactly one inline factory or recipe reference.
type RecipeBaseInput struct {
	Factory *runtimev2.FactoryDeclaration
	Recipe  string
}

// RecipeStepInput contains exactly one named or inline transform.
type RecipeStepInput struct {
	Use       string
	Transform *runtimev2.TransformDeclaration
}

type aliasDefinition struct {
	kind      AliasKind
	transform runtimev2.TransformDeclaration
	recipe    recipeAlias
}

type recipeAlias struct {
	factory    *runtimev2.FactoryDeclaration
	baseRecipe string
	steps      []recipeStep
}

type recipeStep struct {
	use       string
	transform *runtimev2.TransformDeclaration
}

type aliasDocumentData struct {
	aliases        map[string]aliasDefinition
	canonicalBytes int
}

// AliasDocument is an immutable versioned composition document.
type AliasDocument struct{ data *aliasDocumentData }

// NewAliasDocument validates and snapshots constructor input.
func NewAliasDocument(input DocumentInput) (AliasDocument, error) {
	if len(input.Aliases) > MaxAliases {
		return AliasDocument{}, invalid(CodeExpansionTooLarge, "", "/aliases", nil)
	}
	data := &aliasDocumentData{aliases: make(map[string]aliasDefinition, len(input.Aliases))}
	for _, named := range input.Aliases {
		pointer := definitionPointer(named.Name)
		if !validAliasName(named.Name) {
			return AliasDocument{}, invalid(CodeInvalidDocument, "", pointer, nil)
		}
		if _, exists := data.aliases[named.Name]; exists {
			return AliasDocument{}, invalid(CodeInvalidDocument, "", pointer, nil)
		}
		definition, err := buildDefinition(named)
		if err != nil {
			return AliasDocument{}, err
		}
		data.aliases[named.Name] = definition
	}
	document := AliasDocument{data: data}
	// All values stored above have already passed the runtime-v2 constructors,
	// so this in-memory wire form contains no JSON values that can fail to encode.
	raw, _ := document.marshalUnchecked()
	if len(raw) > runtimev2.MaxJSONBytes {
		return AliasDocument{}, invalid(CodeExpansionTooLarge, "", "", nil)
	}
	data.canonicalBytes = len(raw)
	return document, nil
}

func buildDefinition(named NamedAliasInput) (aliasDefinition, error) {
	pointer := definitionPointer(named.Name)
	switch {
	case named.Transform != nil && named.Recipe == nil:
		document, err := runtimev2.NewTransformDeclarationDocument(*named.Transform)
		if err != nil {
			return aliasDefinition{}, invalid(CodeInvalidDocument, "", pointer+"/declaration", err)
		}
		return aliasDefinition{kind: AliasKindTransform, transform: document.Declaration()}, nil
	case named.Transform == nil && named.Recipe != nil:
		recipe, err := buildRecipeAlias(named.Name, *named.Recipe)
		if err != nil {
			return aliasDefinition{}, err
		}
		return aliasDefinition{kind: AliasKindRecipe, recipe: recipe}, nil
	default:
		return aliasDefinition{}, invalid(CodeInvalidDocument, "", pointer, nil)
	}
}

func buildRecipeAlias(name string, input RecipeAliasInput) (recipeAlias, error) {
	basePointer := definitionPointer(name) + "/base"
	result := recipeAlias{}
	switch {
	case input.Base.Factory != nil && input.Base.Recipe == "":
		document, err := runtimev2.NewFactoryDeclarationDocument(*input.Base.Factory)
		if err != nil {
			return recipeAlias{}, invalid(CodeInvalidDocument, "", basePointer+"/factory", err)
		}
		factory := document.Declaration()
		result.factory = &factory
	case input.Base.Factory == nil && validAliasName(input.Base.Recipe):
		result.baseRecipe = input.Base.Recipe
	default:
		return recipeAlias{}, invalid(CodeInvalidDocument, "", basePointer, nil)
	}
	if len(input.Steps) > runtimev2.MaxTransforms {
		return recipeAlias{}, invalid(CodeExpansionTooLarge, "", definitionPointer(name)+"/steps", nil)
	}
	result.steps = make([]recipeStep, len(input.Steps))
	for index, step := range input.Steps {
		pointer := stepPointer(name, index, "")
		switch {
		case step.Transform == nil && validAliasName(step.Use):
			result.steps[index].use = step.Use
		case step.Transform != nil && step.Use == "":
			document, err := runtimev2.NewTransformDeclarationDocument(*step.Transform)
			if err != nil {
				return recipeAlias{}, invalid(CodeInvalidDocument, "", stepPointer(name, index, "transform"), err)
			}
			transform := document.Declaration()
			result.steps[index].transform = &transform
		default:
			return recipeAlias{}, invalid(CodeInvalidDocument, "", pointer[:len(pointer)-1], nil)
		}
	}
	return result, nil
}

// DecodeAliasDocumentJSON strictly decodes one complete bounded document and
// leaves target unchanged on failure.
func DecodeAliasDocumentJSON(raw []byte, target *AliasDocument) error {
	if target == nil {
		return invalid(CodeInvalidDocument, "", "", nil)
	}
	value, err := decodeAliasDocument(raw)
	if err != nil {
		return err
	}
	*target = value
	return nil
}

// UnmarshalJSON implements strict atomic JSON decoding.
func (d *AliasDocument) UnmarshalJSON(raw []byte) error { return DecodeAliasDocumentJSON(raw, d) }

// MarshalJSON emits the canonical versioned document form.
func (d AliasDocument) MarshalJSON() ([]byte, error) {
	if d.data == nil {
		return nil, invalid(CodeInvalidDocument, "", "", nil)
	}
	return d.marshalUnchecked()
}

type documentWire struct {
	SchemaVersion *string                     `json:"schema_version"`
	Aliases       *map[string]json.RawMessage `json:"aliases"`
}

type aliasTypeWire struct {
	Type *AliasKind `json:"type"`
}

type transformAliasWire struct {
	Type        *AliasKind                      `json:"type"`
	Declaration *runtimev2.TransformDeclaration `json:"declaration"`
}

type recipeAliasWire struct {
	Type  *AliasKind      `json:"type"`
	Base  *recipeBaseWire `json:"base"`
	Steps *[]stepWire     `json:"steps"`
}

type recipeBaseWire struct {
	Factory *runtimev2.FactoryDeclaration `json:"factory,omitempty"`
	Recipe  *string                       `json:"recipe,omitempty"`
}

type stepWire struct {
	Use       *string                         `json:"use,omitempty"`
	Transform *runtimev2.TransformDeclaration `json:"transform,omitempty"`
}

func decodeAliasDocument(raw []byte) (AliasDocument, error) {
	var wire documentWire
	if err := decodeStrictJSON(raw, &wire, runtimev2.MaxJSONBytes, "", ""); err != nil {
		return AliasDocument{}, err
	}
	if wire.SchemaVersion == nil || *wire.SchemaVersion != AliasSchemaVersion || wire.Aliases == nil {
		return AliasDocument{}, invalid(CodeInvalidDocument, "", "", nil)
	}
	if len(*wire.Aliases) > MaxAliases {
		return AliasDocument{}, invalid(CodeExpansionTooLarge, "", "/aliases", nil)
	}
	names := make([]string, 0, len(*wire.Aliases))
	for name := range *wire.Aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	input := DocumentInput{Aliases: make([]NamedAliasInput, 0, len(names))}
	for _, name := range names {
		if !validAliasName(name) {
			return AliasDocument{}, invalid(CodeInvalidDocument, "", definitionPointer(name), nil)
		}
		aliasRaw := (*wire.Aliases)[name]
		if isJSONNull(aliasRaw) {
			return AliasDocument{}, invalid(CodeInvalidDocument, "", definitionPointer(name), nil)
		}
		var discriminator aliasTypeWire
		if err := json.Unmarshal(aliasRaw, &discriminator); err != nil || discriminator.Type == nil {
			return AliasDocument{}, invalid(CodeInvalidDocument, "", definitionPointer(name), nil)
		}
		switch *discriminator.Type {
		case AliasKindTransform:
			var alias transformAliasWire
			if err := decodeStrictJSON(aliasRaw, &alias, runtimev2.MaxJSONBytes, "", definitionPointer(name)); err != nil {
				return AliasDocument{}, err
			}
			if alias.Type == nil || *alias.Type != AliasKindTransform || alias.Declaration == nil {
				return AliasDocument{}, invalid(CodeInvalidDocument, "", definitionPointer(name), nil)
			}
			input.Aliases = append(input.Aliases, NamedAliasInput{Name: name, Transform: alias.Declaration})
		case AliasKindRecipe:
			var alias recipeAliasWire
			if err := decodeStrictJSON(aliasRaw, &alias, runtimev2.MaxJSONBytes, "", definitionPointer(name)); err != nil {
				return AliasDocument{}, err
			}
			if alias.Type == nil || *alias.Type != AliasKindRecipe || alias.Base == nil || alias.Steps == nil {
				return AliasDocument{}, invalid(CodeInvalidDocument, "", definitionPointer(name), nil)
			}
			recipe := RecipeAliasInput{Steps: make([]RecipeStepInput, len(*alias.Steps))}
			if alias.Base.Factory != nil {
				recipe.Base.Factory = alias.Base.Factory
			}
			if alias.Base.Recipe != nil {
				recipe.Base.Recipe = *alias.Base.Recipe
			}
			for index, step := range *alias.Steps {
				if step.Use != nil {
					recipe.Steps[index].Use = *step.Use
				}
				recipe.Steps[index].Transform = step.Transform
			}
			input.Aliases = append(input.Aliases, NamedAliasInput{Name: name, Recipe: &recipe})
		default:
			return AliasDocument{}, invalid(CodeInvalidDocument, "", definitionPointer(name)+"/type", nil)
		}
	}
	value, err := NewAliasDocument(input)
	if err != nil {
		return AliasDocument{}, err
	}
	value.data.canonicalBytes = len(rawCanonical(value))
	return value, nil
}

func (d AliasDocument) marshalUnchecked() ([]byte, error) {
	aliases := make(map[string]any, len(d.data.aliases))
	for name, definition := range d.data.aliases {
		typeValue := definition.kind
		switch definition.kind {
		case AliasKindTransform:
			declaration := copyTransform(definition.transform)
			aliases[name] = transformAliasWire{Type: &typeValue, Declaration: &declaration}
		case AliasKindRecipe:
			base := recipeBaseWire{}
			if definition.recipe.factory != nil {
				factory := copyFactory(*definition.recipe.factory)
				base.Factory = &factory
			} else {
				reference := definition.recipe.baseRecipe
				base.Recipe = &reference
			}
			steps := make([]stepWire, len(definition.recipe.steps))
			for index, step := range definition.recipe.steps {
				if step.transform != nil {
					value := copyTransform(*step.transform)
					steps[index].Transform = &value
				} else {
					use := step.use
					steps[index].Use = &use
				}
			}
			aliases[name] = recipeAliasWire{Type: &typeValue, Base: &base, Steps: &steps}
		}
	}
	version := AliasSchemaVersion
	return json.Marshal(struct {
		SchemaVersion *string        `json:"schema_version"`
		Aliases       map[string]any `json:"aliases"`
	}{SchemaVersion: &version, Aliases: aliases})
}

func rawCanonical(document AliasDocument) []byte {
	raw, _ := document.marshalUnchecked()
	return raw
}

func copyFactory(value runtimev2.FactoryDeclaration) runtimev2.FactoryDeclaration {
	document, _ := runtimev2.NewFactoryDeclarationDocument(value)
	return document.Declaration()
}

func copyTransform(value runtimev2.TransformDeclaration) runtimev2.TransformDeclaration {
	document, _ := runtimev2.NewTransformDeclarationDocument(value)
	return document.Declaration()
}
