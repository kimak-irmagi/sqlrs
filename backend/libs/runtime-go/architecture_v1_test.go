package runtimev2

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSchemaAuthorImportBoundary(t *testing.T) {
	const target = "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		clean := filepath.ToSlash(path)
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasPrefix(clean, "schemaauthor/") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			value, _ := strconv.Unquote(spec.Path.Value)
			if value == target && !strings.HasPrefix(clean, "schemas/") {
				t.Errorf("%s bypasses approved schema facade", clean)
			}
			if value == target && spec.Name != nil {
				t.Errorf("%s aliases schemaauthor import", clean)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRepositorySchemaAuthorImportBoundary covers BU04 outside the nested
// module when the tests run from a repository checkout. A downloaded module has
// no repository siblings, so the module-local boundary above remains its gate.
func TestRepositorySchemaAuthorImportBoundary(t *testing.T) {
	const target = "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(os.DirFS(repositoryRoot), ".github"); err != nil {
		t.Skip("not running from a complete repository checkout")
	}
	err = filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		clean := filepath.ToSlash(relative)
		if entry.IsDir() {
			base := entry.Name()
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(clean, ".go") || strings.HasSuffix(clean, "_test.go") {
			return nil
		}
		allowed := strings.HasPrefix(clean, "backend/libs/runtime-go/schemaauthor/") || strings.HasPrefix(clean, "backend/libs/runtime-go/schemas/")
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			value, _ := strconv.Unquote(spec.Path.Value)
			if value == target && !allowed {
				t.Errorf("%s bypasses the approved schema facade", clean)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSchemaFacadesDoNotExposeGenericSchemaTypes(t *testing.T) {
	err := filepath.WalkDir("schemas", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		aliases := map[string]string{}
		for _, spec := range parsed.Imports {
			importPath, _ := strconv.Unquote(spec.Path.Value)
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			aliases[name] = importPath
		}
		forbidden := func(root ast.Node) bool {
			found := false
			ast.Inspect(root, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				identifier, ok := selector.X.(*ast.Ident)
				if !ok {
					return true
				}
				importPath := aliases[identifier.Name]
				if importPath == "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor" {
					found = true
					return false
				}
				if importPath == "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go" && strings.HasSuffix(selector.Sel.Name, "IdentitySchema") {
					found = true
					return false
				}
				return true
			})
			return found
		}
		for _, declaration := range parsed.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Name.IsExported() && forbidden(value.Type) {
					// The two approved fixed capabilities let the registry/cache
					// validate their providers without schema-authoring access.
					if fixedSchemaCapabilityExport(filepath.ToSlash(path), value, aliases) {
						continue
					}
					t.Errorf("%s exports generic schema type from %s", path, value.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					if typed, ok := spec.(*ast.TypeSpec); ok && typed.Name.IsExported() && forbidden(typed.Type) {
						t.Errorf("%s re-exports generic schema type as %s", path, typed.Name.Name)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// fixedSchemaCapabilityExport limits ES25 to the two approved no-argument
// accessors. Neither arbitrary authoring parameters nor other result types pass.
func fixedSchemaCapabilityExport(path string, fn *ast.FuncDecl, imports map[string]string) bool {
	approved := path == "schemas/sqlrs/schema.go" && fn.Name.Name == "WorkspaceFileExtensionSchema" ||
		path == "schemas/sqlrs/execution.go" && fn.Name.Name == "ExecutionEnvironmentSchema"
	if !approved || fn.Recv != nil || fn.Type.Params.NumFields() != 0 || fn.Type.Results.NumFields() != 1 {
		return false
	}
	result, ok := fn.Type.Results.List[0].Type.(*ast.SelectorExpr)
	if !ok || result.Sel.Name != "ExtensionIdentitySchema" {
		return false
	}
	pkg, ok := result.X.(*ast.Ident)
	return ok && imports[pkg.Name] == "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
}

func TestFixedSchemaCapabilityExceptionIsNarrow(t *testing.T) {
	for _, tc := range []struct {
		name, path, signature string
		allowed               bool
	}{
		{"workspace", "schemas/sqlrs/schema.go", "WorkspaceFileExtensionSchema() runtimev2.ExtensionIdentitySchema", true},
		{"execution", "schemas/sqlrs/execution.go", "ExecutionEnvironmentSchema() runtimev2.ExtensionIdentitySchema", true},
		{"other file", "schemas/sqlrs/other.go", "ExecutionEnvironmentSchema() runtimev2.ExtensionIdentitySchema", false},
		{"other name", "schemas/sqlrs/execution.go", "ArbitrarySchema() runtimev2.ExtensionIdentitySchema", false},
		{"parameters", "schemas/sqlrs/execution.go", "ExecutionEnvironmentSchema(v string) runtimev2.ExtensionIdentitySchema", false},
		{"definition", "schemas/sqlrs/execution.go", "ExecutionEnvironmentSchema(v schemaauthor.SchemaInput) runtimev2.ExtensionIdentitySchema", false},
		{"other kind", "schemas/sqlrs/execution.go", "ExecutionEnvironmentSchema() runtimev2.FactoryIdentitySchema", false},
		{"generic result", "schemas/sqlrs/execution.go", "ExecutionEnvironmentSchema() schemaauthor.SchemaInput", false},
		{"pointer result", "schemas/sqlrs/execution.go", "ExecutionEnvironmentSchema() *runtimev2.ExtensionIdentitySchema", false},
		{"multiple results", "schemas/sqlrs/execution.go", "ExecutionEnvironmentSchema() (runtimev2.ExtensionIdentitySchema, error)", false},
		{"no result", "schemas/sqlrs/execution.go", "ExecutionEnvironmentSchema()", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture\nfunc "+tc.signature+" {}", 0)
			if err != nil {
				t.Fatal(err)
			}
			fn := file.Decls[0].(*ast.FuncDecl)
			imports := map[string]string{"runtimev2": "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go", "schemaauthor": "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"}
			if got := fixedSchemaCapabilityExport(tc.path, fn, imports); got != tc.allowed {
				t.Fatalf("allowed=%v, want %v", got, tc.allowed)
			}
		})
	}
}

func TestConformanceFacadeImportBoundary(t *testing.T) {
	const target = "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/conformancev1"
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(os.DirFS(repositoryRoot), ".github"); err != nil {
		t.Skip("not running from a complete repository checkout")
	}
	err = filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		clean := filepath.ToSlash(relative)
		if entry.IsDir() {
			base := entry.Name()
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(clean, ".go") || strings.HasSuffix(clean, "_test.go") {
			return nil
		}
		allowed := strings.HasPrefix(clean, "backend/libs/runtime-go/schemas/conformancev1/") ||
			strings.HasPrefix(clean, "test/runtime-v2-consumer/")
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			value, _ := strconv.Unquote(spec.Path.Value)
			if value == target && !allowed {
				t.Errorf("%s imports the contract-verification-only facade", clean)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("schemas", "conformance")); !os.IsNotExist(err) {
		t.Fatal("unversioned conformance facade must not exist")
	}
}
