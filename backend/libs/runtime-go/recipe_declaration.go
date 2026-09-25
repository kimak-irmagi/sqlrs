package runtimev2

import "encoding/json"

type recipeDeclarationData struct {
	factory    FactoryDeclaration
	transforms []TransformDeclaration
}

// RecipeDeclaration is an immutable versioned unresolved recipe. It cannot be
// passed to Build, which accepts only resolved Recipe values.
type RecipeDeclaration struct{ data *recipeDeclarationData }

// NewRecipeDeclaration validates and snapshots an unresolved recipe.
func NewRecipeDeclaration(factory FactoryDeclaration, transforms []TransformDeclaration) (RecipeDeclaration, error) {
	if err := validateFactoryDeclaration(&factory); err != nil {
		return RecipeDeclaration{}, prefixError(err, "factory")
	}
	if len(transforms) > MaxTransforms {
		return RecipeDeclaration{}, invalid(CodeTooLarge, "transforms")
	}
	copied := make([]TransformDeclaration, len(transforms))
	for index := range transforms {
		if err := validateTransformDeclaration(&transforms[index]); err != nil {
			return RecipeDeclaration{}, prefixError(err, "transforms["+itoa(index)+"]")
		}
		copied[index] = *copyTransformDeclaration(&transforms[index])
	}
	return RecipeDeclaration{data: &recipeDeclarationData{factory: *copyFactoryDeclaration(&factory), transforms: copied}}, nil
}

// Factory returns a defensive copy of the unresolved factory.
func (r RecipeDeclaration) Factory() FactoryDeclaration {
	if r.data == nil {
		return FactoryDeclaration{}
	}
	return *copyFactoryDeclaration(&r.data.factory)
}

// Transforms returns defensive copies in recipe order.
func (r RecipeDeclaration) Transforms() []TransformDeclaration {
	if r.data == nil {
		return nil
	}
	result := make([]TransformDeclaration, len(r.data.transforms))
	for index := range r.data.transforms {
		result[index] = *copyTransformDeclaration(&r.data.transforms[index])
	}
	return result
}

type recipeDeclarationWire struct {
	SchemaVersion *string                 `json:"schema_version"`
	Factory       *FactoryDeclaration     `json:"factory"`
	Transforms    *[]TransformDeclaration `json:"transforms"`
}

func (r RecipeDeclaration) MarshalJSON() ([]byte, error) {
	if r.data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	version, factory, transforms := SchemaVersion, r.Factory(), r.Transforms()
	return json.Marshal(recipeDeclarationWire{&version, &factory, &transforms})
}

func (r *RecipeDeclaration) UnmarshalJSON(raw []byte) error {
	if r == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire recipeDeclarationWire
	if err := decodeStrict(raw, &wire); err != nil {
		return err
	}
	if wire.SchemaVersion == nil {
		return invalid(CodeInvalidShape, "schema_version")
	}
	if *wire.SchemaVersion != SchemaVersion {
		return invalid(CodeInvalidVersion, "schema_version")
	}
	if wire.Factory == nil {
		return invalid(CodeInvalidShape, "factory")
	}
	if wire.Transforms == nil {
		return invalid(CodeInvalidShape, "transforms")
	}
	value, err := NewRecipeDeclaration(*wire.Factory, *wire.Transforms)
	if err != nil {
		return err
	}
	r.data = value.data
	return nil
}
