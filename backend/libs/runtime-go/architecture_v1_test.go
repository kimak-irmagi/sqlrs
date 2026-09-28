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
