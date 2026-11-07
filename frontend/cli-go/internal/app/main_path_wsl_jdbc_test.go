package app

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/sqlrs/cli/internal/enginebin"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// MP07/JDBC delivery: installation transfers verified bundle bytes, verifies
// them in WSL before replacing the executable, and fails before transport when
// the release artifact is missing or invalid. No actual WSL/device is modified.
func TestMainPathWSLInstallerRequiresAndTransfersDriver(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "sqlrs-engine")
			if err := os.WriteFile(source, []byte("engine fixture"), 0700); err != nil {
				t.Fatal(err)
			}
			previousRead, previousRun := readBundledJDBCFn, runWSLCommandWithInputFn
			t.Cleanup(func() { readBundledJDBCFn = previousRead; runWSLCommandWithInputFn = previousRun })
			driver := []byte("already verified test driver")
			readBundledJDBCFn = func(path string) ([]byte, error) {
				if path != source {
					t.Fatal("wrong release bundle")
				}
				if mode == "missing" {
					return nil, nil
				}
				if mode == "invalid" {
					return nil, errors.New("digest mismatch")
				}
				return driver, nil
			}
			calls := 0
			runWSLCommandWithInputFn = func(_ context.Context, distro string, _ bool, desc, input, command string, args ...string) (string, error) {
				calls++
				if distro != "Ubuntu" || command != "sh" || len(args) != 1 || args[0] != "-s" {
					t.Fatal("unexpected transport")
				}
				for _, expected := range []string{base64.StdEncoding.EncodeToString(driver), "sha256sum", strings.TrimPrefix(enginebin.JDBCDriverDigest, "sha256:"), enginebin.JDBCDriverFileName} {
					if !strings.Contains(input, expected) {
						t.Fatal("driver transfer/verification missing")
					}
				}
				if strings.Index(input, "driver_hash=") > strings.Index(input, `mv -f "$tmp" "$dest"`) {
					t.Fatal("engine published before driver verification")
				}
				return "/tmp/sqlrs-fixture/sqlrs-engine\n", nil
			}
			_, err := installWSLEngine(t.Context(), "Ubuntu", source, "/tmp/sqlrs-fixture/sqlrs-engine", false)
			if mode == "valid" {
				if err != nil || calls != 1 {
					t.Fatal("valid bundle refused", err)
				}
			} else if err == nil || calls != 0 {
				t.Fatal("invalid bundle reached WSL transport")
			}
		})
	}
}
