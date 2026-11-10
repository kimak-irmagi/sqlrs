package preparecost

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// CT01: source keys are a closed catalog, not strings obtained from requests,
// Docker errors or SQL. Current symbols must really exist in the source tree.
func TestCostCatalogOwnsDefinitionsAndCurrentSymbols(t *testing.T) {
	definitions := Definitions()
	seen := map[Operation]bool{}
	for _, definition := range definitions {
		if seen[definition.Operation] {
			t.Fatal("duplicate operation key")
		}
		seen[definition.Operation] = true
		got, ok := Describe(definition.Operation)
		if !ok || got != definition {
			t.Fatal("catalog lookup differs from enumeration")
		}
		if definition.Relation == Additional {
			if definition.Released != (Location{}) {
				t.Fatal("additional action invented a released counterpart")
			}
		} else if definition.Released.File == "" || definition.Released.Symbol == "" {
			t.Fatal("shared/changed/shifted action lacks released source correspondence")
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", filepath.FromSlash(definition.Current.File)), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			symbol := function.Name.Name
			if function.Recv != nil {
				receiver := function.Recv.List[0].Type
				if pointer, ok := receiver.(*ast.StarExpr); ok {
					receiver = pointer.X
				}
				if named, ok := receiver.(*ast.Ident); ok {
					symbol = named.Name + "." + symbol
				}
			}
			found = found || symbol == definition.Current.Symbol
		}
		if !found {
			t.Fatalf("operation %s points at a missing current symbol", definition.Operation)
		}
	}
	if len(seen) < 4 {
		t.Fatal("initial worker/capture/inventory catalog missing")
	}
	before := Definitions()
	definitions[0].Current.Symbol = "private-mutated-symbol"
	if !reflect.DeepEqual(before, Definitions()) {
		t.Fatal("caller mutated the process catalog")
	}
	for _, op := range []Operation{"", "private-unknown-key", "capture.read\nprivate-data"} {
		definition, ok := Describe(op)
		if ok || definition != (Definition{}) || strings.Contains(string(definition.Operation), "private-") {
			t.Fatal("unknown input acquired a source identity")
		}
	}
}
