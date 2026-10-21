package composition

import (
	"encoding/json"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

type expandedRecipeData struct {
	declaration runtimev2.RecipeDeclaration
	trace       ExpansionTrace
}

type expandedTransformData struct {
	declaration runtimev2.TransformDeclarationDocument
	trace       ExpansionTrace
}

// ExpandedRecipe contains an immutable semantic declaration and separate trace.
type ExpandedRecipe struct{ data *expandedRecipeData }

// ExpandedTransform contains an immutable standalone declaration and trace.
type ExpandedTransform struct{ data *expandedTransformData }

// Declaration returns a defensive unresolved recipe value.
func (e ExpandedRecipe) Declaration() runtimev2.RecipeDeclaration {
	if e.data == nil {
		return runtimev2.RecipeDeclaration{}
	}
	// RecipeDeclaration itself is immutable; its accessors copy all mutable DTOs.
	// Returning it directly avoids a redundant, potentially fallible JSON clone.
	return e.data.declaration
}

// Trace returns a defensive diagnostic trace value.
func (e ExpandedRecipe) Trace() ExpansionTrace {
	if e.data == nil {
		return ExpansionTrace{}
	}
	return ExpansionTrace{data: cloneTraceData(e.data.trace.data)}
}

// Declaration returns a defensive standalone transform document.
func (e ExpandedTransform) Declaration() runtimev2.TransformDeclarationDocument {
	if e.data == nil {
		return runtimev2.TransformDeclarationDocument{}
	}
	value, _ := runtimev2.NewTransformDeclarationDocument(e.data.declaration.Declaration())
	return value
}

// Trace returns a defensive diagnostic trace value.
func (e ExpandedTransform) Trace() ExpansionTrace {
	if e.data == nil {
		return ExpansionTrace{}
	}
	return ExpansionTrace{data: cloneTraceData(e.data.trace.data)}
}

// ExpandTransform resolves one exact standalone transform alias.
func (c Catalog) ExpandTransform(name string) (ExpandedTransform, error) {
	if c.data == nil {
		return ExpandedTransform{}, invalid(CodeInvalidDocument, "", "", nil)
	}
	if !validAliasName(name) {
		return ExpandedTransform{}, invalid(CodeInvalidDocument, "", "", nil)
	}
	entry, exists := c.data.aliases[name]
	if !exists {
		return ExpandedTransform{}, invalid(CodeMissingReference, "", "", nil)
	}
	if entry.definition.kind != AliasKindTransform {
		return ExpandedTransform{}, invalid(CodeWrongReferenceKind, entry.sourceID, definitionPointer(name), nil)
	}
	// Catalog entries passed this exact standalone constructor on admission.
	// The immutable snapshot therefore already includes its wrapper's budget.
	declaration, _ := runtimev2.NewTransformDeclarationDocument(entry.definition.transform)
	node := traceNodeData{id: 0, kind: AliasKindTransform, alias: name, sourceID: entry.sourceID, pointer: definitionPointer(name)}
	trace, err := newTrace(traceData{
		nodes:      []traceNodeData{node},
		transforms: []originData{{outputIndex: 0, nodeID: 0, pointer: node.pointer + "/declaration"}},
	})
	if err != nil {
		return ExpandedTransform{}, err
	}
	return ExpandedTransform{data: &expandedTransformData{declaration: declaration, trace: trace}}, nil
}

// ExpandRecipe deterministically flattens one recipe prefix chain and its steps.
func (c Catalog) ExpandRecipe(name string) (ExpandedRecipe, error) {
	if c.data == nil {
		return ExpandedRecipe{}, invalid(CodeInvalidDocument, "", "", nil)
	}
	if !validAliasName(name) {
		return ExpandedRecipe{}, invalid(CodeInvalidDocument, "", "", nil)
	}
	target, exists := c.data.aliases[name]
	if !exists {
		return ExpandedRecipe{}, invalid(CodeMissingReference, "", "", nil)
	}
	if target.definition.kind != AliasKindRecipe {
		return ExpandedRecipe{}, invalid(CodeWrongReferenceKind, target.sourceID, definitionPointer(name), nil)
	}

	chain := make([]catalogEntry, 0, 8)
	active := make(map[string]int)
	current := target
	for {
		active[current.name] = len(chain)
		chain = append(chain, current)
		recipe := current.definition.recipe
		if recipe.factory != nil {
			break
		}
		pointer := definitionPointer(current.name) + "/base/recipe"
		child, found := c.data.aliases[recipe.baseRecipe]
		if !found {
			return ExpandedRecipe{}, invalid(CodeMissingReference, current.sourceID, pointer, nil)
		}
		if child.definition.kind != AliasKindRecipe {
			return ExpandedRecipe{}, invalid(CodeWrongReferenceKind, current.sourceID, pointer, nil)
		}
		if first, cycling := active[child.name]; cycling {
			cycle := make([]DiagnosticReference, 0, len(chain)-first+1)
			for _, entry := range chain[first:] {
				cycle = append(cycle, DiagnosticReference{Alias: entry.name, SourceID: entry.sourceID, Pointer: definitionPointer(entry.name)})
			}
			cycle = append(cycle, DiagnosticReference{Alias: child.name, SourceID: child.sourceID, Pointer: definitionPointer(child.name)})
			return ExpandedRecipe{}, invalidWithCycle(current.sourceID, pointer, cycle)
		}
		current = child
	}

	nodes := make([]traceNodeData, len(chain))
	for index, entry := range chain {
		nodes[index] = traceNodeData{id: index, kind: AliasKindRecipe, alias: entry.name, sourceID: entry.sourceID, pointer: definitionPointer(entry.name)}
		if index > 0 {
			parent := chain[index-1]
			nodes[index].hasParent = true
			nodes[index].parentID = index - 1
			nodes[index].referencePointer = definitionPointer(parent.name) + "/base/recipe"
		}
	}
	factory := copyFactory(*chain[len(chain)-1].definition.recipe.factory)
	factoryOrigin := originData{nodeID: len(chain) - 1, pointer: definitionPointer(chain[len(chain)-1].name) + "/base/factory"}
	transforms := make([]runtimev2.TransformDeclaration, 0)
	origins := make([]originData, 0)
	// Charge the exact recipe envelope/factory and each bounded public child
	// BEFORE copying the next step or appending its output trace. Do not encode
	// a prospective oversized recipe. Requirements: declaration-limits.md.
	empty, err := runtimev2.NewRecipeDeclaration(factory, nil)
	if err != nil {
		return ExpandedRecipe{}, invalid(CodeExpansionTooLarge, target.sourceID, definitionPointer(target.name), nil)
	}
	baseRaw, err := json.Marshal(empty)
	if err != nil {
		return ExpandedRecipe{}, invalid(CodeExpansionTooLarge, target.sourceID, definitionPointer(target.name), nil)
	}
	used := len(baseRaw)
	charge := func(value runtimev2.TransformDeclaration) bool {
		raw, err := json.Marshal(value)
		if err != nil {
			return false
		}
		n := len(raw)
		if len(transforms) > 0 {
			n++
		}
		next, ok := checkedAdd(used, n, runtimev2.MaxJSONBytes)
		if ok {
			used = next
		}
		return ok
	}
	for recipeIndex := len(chain) - 1; recipeIndex >= 0; recipeIndex-- {
		entry := chain[recipeIndex]
		for stepIndex, step := range entry.definition.recipe.steps {
			if step.transform != nil {
				if len(transforms) >= runtimev2.MaxTransforms {
					return ExpandedRecipe{}, invalid(CodeExpansionTooLarge, target.sourceID, definitionPointer(target.name), nil)
				}
				if !charge(*step.transform) {
					return ExpandedRecipe{}, invalid(CodeExpansionTooLarge, target.sourceID, definitionPointer(target.name), nil)
				}
				transforms = append(transforms, copyTransform(*step.transform))
				origins = append(origins, originData{outputIndex: len(origins), nodeID: recipeIndex, pointer: stepPointer(entry.name, stepIndex, "transform")})
				continue
			}
			pointer := stepPointer(entry.name, stepIndex, "use")
			child, found := c.data.aliases[step.use]
			if !found {
				return ExpandedRecipe{}, invalid(CodeMissingReference, entry.sourceID, pointer, nil)
			}
			if child.definition.kind != AliasKindTransform {
				return ExpandedRecipe{}, invalid(CodeWrongReferenceKind, entry.sourceID, pointer, nil)
			}
			if len(transforms) >= runtimev2.MaxTransforms || len(nodes) >= MaxTraceNodes {
				return ExpandedRecipe{}, invalid(CodeExpansionTooLarge, target.sourceID, definitionPointer(target.name), nil)
			}
			if !charge(child.definition.transform) {
				return ExpandedRecipe{}, invalid(CodeExpansionTooLarge, target.sourceID, definitionPointer(target.name), nil)
			}
			transforms = append(transforms, copyTransform(child.definition.transform))
			nodeID := len(nodes)
			nodes = append(nodes, traceNodeData{
				id: nodeID, parentID: recipeIndex, hasParent: true, referencePointer: pointer,
				kind: AliasKindTransform, alias: child.name, sourceID: child.sourceID, pointer: definitionPointer(child.name),
			})
			origins = append(origins, originData{outputIndex: len(origins), nodeID: nodeID, pointer: definitionPointer(child.name) + "/declaration"})
		}
	}

	// Keep the public constructor authoritative, even after incremental charging.
	declaration, err := runtimev2.NewRecipeDeclaration(factory, transforms)
	if err != nil {
		return ExpandedRecipe{}, invalid(CodeExpansionTooLarge, target.sourceID, definitionPointer(target.name), nil)
	}
	trace, err := newTrace(traceData{nodes: nodes, factory: &factoryOrigin, transforms: origins})
	if err != nil {
		// Generated nodes satisfy the trace grammar by construction, leaving only
		// the independent serialized trace-size limit as a possible failure.
		return ExpandedRecipe{}, invalid(CodeExpansionTooLarge, target.sourceID, definitionPointer(target.name), nil)
	}
	return ExpandedRecipe{data: &expandedRecipeData{declaration: declaration, trace: trace}}, nil
}
