package consumertest

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ET08 is mandatory under the existing CI/release whole-consumer step. Selecting
// only the self-contained file excludes this wrapper and prevents recursion.
func TestExecutionTransformConsumerIsolation(t *testing.T) {
	const selected = "execution_transform_test.go"
	parsed, err := parser.ParseFile(token.NewFileSet(), selected, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(path, "/schemaauthor") || strings.Contains(path, "/schemas/conformancev1") || strings.Contains(path, "/internal/") {
			t.Fatalf("forbidden consumer capability: %s", path)
		}
	}
	control := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":            "module example.invalid/isolation-control\n\ngo 1.25.0\n",
		"positive_test.go":  "package control\nimport \"testing\"\nfunc TestPositive(t *testing.T) {}\n",
		"dependent_test.go": "package control\nimport \"testing\"\nfunc TestDependent(t *testing.T) { helper() }\n",
		"helper_test.go":    "package control\nfunc helper() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(control, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := executionTransformSelectedTest(control, "positive_test.go"); err != nil {
		t.Fatalf("positive control: %v\n%s", err, output)
	}
	if output, err := executionTransformGoTest(control, "./..."); err != nil {
		t.Fatalf("dependent control is not otherwise valid: %v\n%s", err, output)
	}
	if output, err := executionTransformSelectedTest(control, "dependent_test.go"); err == nil || !strings.Contains(string(output), "undefined: helper") {
		t.Fatalf("file isolation not proved: %v\n%s", err, output)
	}
	if output, err := executionTransformSelectedTest(".", selected); err != nil {
		t.Fatalf("standalone consumer: %v\n%s", err, output)
	}
}

func executionTransformSelectedTest(directory, file string) ([]byte, error) {
	return executionTransformGoTest(directory, file)
}

func executionTransformGoTest(directory, target string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", target)
	cmd.Dir = directory
	return cmd.CombinedOutput()
}
