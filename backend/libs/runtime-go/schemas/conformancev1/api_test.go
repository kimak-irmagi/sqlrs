package conformancev1_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestExactExportedAPIAndDocumentation(t *testing.T) {
	want := []string{
		"DeclarationReference", "ExtensionIdentitySchema", "ExtensionKind",
		"ExtensionObservationSchema", "ExtensionSpecificationSchema",
		"FactoryIdentitySchema", "FactoryKind", "FactoryObservationSchema",
		"FieldCredential", "FieldLocator", "FieldPlan", "NewDeploymentDeclaration",
		"NewExecutionEnvironmentDeclaration", "NewExtensionBuilder",
		"NewExtensionObservation", "NewFactoryBuilder", "NewFactoryObservation",
		"NewInputDeclaration", "NewTransformBuilder", "NewTransformObservation",
		"ObservationCheckpointBackend", "ObservationContainerID", "ObservationJobID",
		"ObservationMaterializationPath", "ObservationPhysicalSize",
		"ObservationTimestamp", "Provider", "TransformIdentitySchema",
		"TransformKind", "TransformObservationSchema",
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	var got []string
	packageDocumented := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(set, entry.Name(), nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if file.Doc != nil && strings.Contains(file.Doc.Text(), "contract verification") && strings.Contains(file.Doc.Text(), "not a production provider") {
			packageDocumented = true
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Name.IsExported() {
					got = append(got, value.Name.Name)
					if value.Doc == nil {
						t.Errorf("%s lacks a documentation comment", value.Name.Name)
					}
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					switch item := spec.(type) {
					case *ast.TypeSpec:
						if item.Name.IsExported() {
							got = append(got, item.Name.Name)
							if item.Doc == nil && value.Doc == nil {
								t.Errorf("%s lacks a documentation comment", item.Name.Name)
							}
						}
					case *ast.ValueSpec:
						for _, name := range item.Names {
							if name.IsExported() {
								got = append(got, name.Name)
								if item.Doc == nil && item.Comment == nil && value.Doc == nil {
									t.Errorf("%s lacks a documentation comment", name.Name)
								}
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("exported API:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !packageDocumented {
		t.Fatal("package documentation must classify the facade as contract verification, not a production provider")
	}
}
