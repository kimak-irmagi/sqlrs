package runtimev2

import "encoding/json"

// declarationJSONBudget counts the existing normalized wire projections before
// aggregate copying/encoding. All inputs are structurally validated, counts are
// nonnegative, and 0 <= used <= limit. Requirements:
// docs/architecture/runtime-go-declaration-limits.md.
type declarationJSONBudget struct{ used, limit int }

// add rejects before addition so even a large positive count cannot overflow.
func (b *declarationJSONBudget) add(n int) error {
	if n > b.limit-b.used {
		return invalid(CodeTooLarge, "$")
	}
	b.used += n
	return nil
}

// string quotes only one bounded validated scalar, never an enclosing value.
// Reject its allocation-free lower bound before asking encoding/json to escape.
func (b *declarationJSONBudget) string(value string) error {
	if len(value) > b.limit-b.used || b.limit-b.used-len(value) < 2 {
		return invalid(CodeTooLarge, "$")
	}
	raw, _ := json.Marshal(value) // encoding/json cannot fail on a string.
	return b.add(len(raw))
}

// collection charges brackets/braces and commas, stopping before any subsequent
// child visitor once a separator or child has exhausted the budget.
func (b *declarationJSONBudget) collection(count int, visit func(int) error) error {
	if err := b.add(2); err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		if i > 0 {
			if err := b.add(1); err != nil {
				return err
			}
		}
		if err := visit(i); err != nil {
			return err
		}
	}
	return nil
}

// declarationJSONMember borrows a wire member and its local sizing visitor.
// It stores no source copies or mutable global instrumentation.
type declarationJSONMember struct {
	name  string
	value func() error
}

func (b *declarationJSONBudget) object(members []declarationJSONMember) error {
	return b.collection(len(members), func(i int) error {
		if err := b.string(members[i].name); err != nil {
			return err
		}
		if err := b.add(1); err != nil {
			return err
		}
		return members[i].value()
	})
}

func (b *declarationJSONBudget) text(name, value string) declarationJSONMember {
	return declarationJSONMember{name, func() error { return b.string(value) }}
}

// extension shares the existing specification/observation wire projections;
// fields are charged individually, including all standard JSON escaping.
func (b *declarationJSONBudget) extension(version, owner, kind, schemaKey, schema string, fields []DeclarationField) error {
	return b.object([]declarationJSONMember{
		b.text("schema_version", version), b.text("owner", owner), b.text("kind", kind), b.text(schemaKey, schema),
		{"fields", func() error {
			return b.collection(len(fields), func(i int) error {
				return b.object([]declarationJSONMember{b.text("name", fields[i].Name), b.text("value", fields[i].Value)})
			})
		}},
	})
}

func (b *declarationJSONBudget) specification(w extensionSpecificationWire) error {
	return b.extension(*w.SchemaVersion, *w.Owner, *w.Kind, "specification_schema", *w.SpecificationSchema, *w.Fields)
}

func (b *declarationJSONBudget) observation(w diagnosticObservationWire) error {
	return b.extension(*w.SchemaVersion, *w.Owner, *w.Kind, "observation_schema", *w.ObservationSchema, *w.Fields)
}

// declaration walks the same normalized projection used by the codecs, with
// identical omitted optional collections. No public MarshalJSON is called.
func (b *declarationJSONBudget) declaration(w declarationWire) error {
	members := []declarationJSONMember{
		b.text("kind", *w.Kind), b.text("reference", *w.Reference),
		{"arguments", func() error {
			return b.collection(len(*w.Arguments), func(i int) error { return b.string((*w.Arguments)[i]) })
		}},
		{"attributes", func() error {
			// Only the sum matters; key order does not affect encoded byte length.
			attrs := make([]declarationJSONMember, 0, len(*w.Attributes))
			for key, value := range *w.Attributes {
				attrs = append(attrs, b.text(key, value))
			}
			return b.object(attrs)
		}},
	}
	if len(w.Inputs) > 0 {
		members = append(members, declarationJSONMember{"inputs", func() error {
			return b.collection(len(w.Inputs), func(i int) error {
				wire, _ := specificationWire(w.Inputs[i].data) // Validated opaque child.
				return b.specification(wire)
			})
		}})
	}
	if w.ExecutionEnvironment != nil {
		members = append(members, declarationJSONMember{"execution_environment", func() error {
			wire, _ := specificationWire(w.ExecutionEnvironment.data)
			return b.specification(wire)
		}})
	}
	if w.Deployment != nil {
		members = append(members, declarationJSONMember{"deployment", func() error {
			wire, _ := specificationWire(w.Deployment.data)
			return b.specification(wire)
		}})
	}
	if len(w.Capabilities) > 0 {
		members = append(members, declarationJSONMember{"capabilities", func() error {
			return b.collection(len(w.Capabilities), func(i int) error { return b.observation(diagnosticWire(w.Capabilities[i].data)) })
		}})
	}
	if len(w.Portability) > 0 {
		members = append(members, declarationJSONMember{"portability", func() error {
			return b.collection(len(w.Portability), func(i int) error { return b.observation(diagnosticWire(w.Portability[i].data)) })
		}})
	}
	return b.object(members)
}

func (b *declarationJSONBudget) document(declaration declarationWire) error {
	return b.object([]declarationJSONMember{b.text("schema_version", SchemaVersion), {"declaration", func() error { return b.declaration(declaration) }}})
}

// validateRecipeDeclaration checks the whole shared budget before cloning any
// aggregate. It stops validating later transforms as soon as the budget rejects.
func validateRecipeDeclaration(factory *FactoryDeclaration, transforms []TransformDeclaration) error {
	if err := validateFactoryDeclarationInput(factory); err != nil {
		return prefixError(err, "factory")
	}
	if len(transforms) > MaxTransforms {
		return invalid(CodeTooLarge, "transforms")
	}
	b := declarationJSONBudget{limit: MaxJSONBytes}
	return b.object([]declarationJSONMember{
		b.text("schema_version", SchemaVersion),
		{"factory", func() error { return b.declaration(factoryDeclarationWire(factory)) }},
		{"transforms", func() error {
			return b.collection(len(transforms), func(i int) error {
				if err := validateTransformDeclarationInput(&transforms[i]); err != nil {
					return prefixError(err, "transforms["+itoa(i)+"]")
				}
				return b.declaration(transformDeclarationWire(&transforms[i]))
			})
		}},
	})
}
