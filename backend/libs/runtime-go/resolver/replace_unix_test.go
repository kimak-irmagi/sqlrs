//go:build !windows

package resolver

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDirectoryRejectsMissingPath(t *testing.T) {
	err := syncDirectory(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sync missing directory = %v", err)
	}
}
