package statefs

import (
	"os"
	"path/filepath"
	"testing"
)

// M03/C01: an imported published state must have the exact durable marker;
// absence, corruption, or a directory/symlink must not authorize publication.
func TestVerifyPublishedMarker(t *testing.T) {
	for _, kind := range []string{"copy", "btrfs"} {
		t.Run(kind, func(t *testing.T) {
			stateDir := filepath.Join(t.TempDir(), "states", "state-1")
			marker := PublishedMarkerPath(stateDir, kind)
			if err := VerifyPublishedMarker(stateDir, kind); err == nil {
				t.Fatal("missing marker was accepted")
			}
			if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(marker, []byte("wrong"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := VerifyPublishedMarker(stateDir, kind); err == nil {
				t.Fatal("corrupt marker was accepted")
			}
			if err := os.WriteFile(marker, []byte("ok"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := VerifyPublishedMarker(stateDir, kind); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(marker, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := VerifyPublishedMarker(stateDir, kind); err == nil {
				t.Fatal("directory marker was accepted")
			}
		})
	}
	if err := VerifyPublishedMarker("relative/state", "copy"); err == nil {
		t.Fatal("relative state path was accepted")
	}
}
