package composition

import (
	"sort"
)

// SourceDocument supplies one explicit logical diagnostic source to a catalog.
type SourceDocument struct {
	SourceID string
	Document AliasDocument
}

type catalogEntry struct {
	name       string
	sourceID   string
	definition aliasDefinition
}

type catalogData struct{ aliases map[string]catalogEntry }

// Catalog is an immutable exact-name namespace built from explicit documents.
type Catalog struct{ data *catalogData }

// NewCatalog validates all sources in canonical source/name order.
func NewCatalog(sources []SourceDocument) (Catalog, error) {
	if len(sources) > MaxSourceDocuments {
		return Catalog{}, invalid(CodeExpansionTooLarge, "", "", nil)
	}
	ordered := append([]SourceDocument(nil), sources...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].SourceID < ordered[right].SourceID })
	for index := range ordered {
		if !validateSourceID(ordered[index].SourceID) {
			return Catalog{}, invalid(CodeInvalidDocument, ordered[index].SourceID, "", nil)
		}
		if index > 0 && ordered[index-1].SourceID == ordered[index].SourceID {
			return Catalog{}, invalid(CodeInvalidDocument, ordered[index].SourceID, "", nil)
		}
	}
	for _, source := range ordered {
		if source.Document.data == nil {
			return Catalog{}, invalid(CodeInvalidDocument, source.SourceID, "", nil)
		}
	}
	totalBytes, totalDefinitions := 0, 0
	for _, source := range ordered {
		// MaxSourceDocuments and MaxSourceIDBytes bound this addition well below
		// MaxCatalogBytes; the document addition below is the effective limit.
		totalBytes += len(source.SourceID)
		var ok bool
		totalBytes, ok = checkedAdd(totalBytes, source.Document.data.canonicalBytes, MaxCatalogBytes)
		if !ok {
			return Catalog{}, invalid(CodeExpansionTooLarge, "", "", nil)
		}
		totalDefinitions, ok = checkedAdd(totalDefinitions, len(source.Document.data.aliases), MaxAliases)
		if !ok {
			return Catalog{}, invalid(CodeExpansionTooLarge, "", "", nil)
		}
	}

	candidates := make(map[string][]catalogEntry, totalDefinitions)
	for _, source := range ordered {
		names := make([]string, 0, len(source.Document.data.aliases))
		for name := range source.Document.data.aliases {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			definition := cloneDefinition(source.Document.data.aliases[name])
			candidates[name] = append(candidates[name], catalogEntry{name: name, sourceID: source.SourceID, definition: definition})
		}
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entries := candidates[name]
		if len(entries) <= 1 {
			continue
		}
		diagnostics := make([]DiagnosticReference, len(entries))
		for index, entry := range entries {
			diagnostics[index] = DiagnosticReference{Alias: name, SourceID: entry.sourceID, Pointer: definitionPointer(name)}
		}
		return Catalog{}, invalidWithCandidates(diagnostics)
	}
	data := &catalogData{aliases: make(map[string]catalogEntry, len(candidates))}
	for name, entries := range candidates {
		data.aliases[name] = entries[0]
	}
	return Catalog{data: data}, nil
}

func cloneDefinition(source aliasDefinition) aliasDefinition {
	result := aliasDefinition{kind: source.kind}
	switch source.kind {
	case AliasKindTransform:
		result.transform = copyTransform(source.transform)
	case AliasKindRecipe:
		result.recipe.baseRecipe = source.recipe.baseRecipe
		if source.recipe.factory != nil {
			factory := copyFactory(*source.recipe.factory)
			result.recipe.factory = &factory
		}
		result.recipe.steps = make([]recipeStep, len(source.recipe.steps))
		for index, step := range source.recipe.steps {
			result.recipe.steps[index].use = step.use
			if step.transform != nil {
				transform := copyTransform(*step.transform)
				result.recipe.steps[index].transform = &transform
			}
		}
	}
	return result
}
