package run

import (
	"os"
	"path/filepath"
	"testing"
)

// macOS exposes TMPDIR through /var -> /private/var. These fixtures require a
// canonical trust root; explicit symlink-escape tests still create their links.
func TestMain(m *testing.M) {
	root, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("TMPDIR", root); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
