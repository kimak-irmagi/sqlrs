package statefs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PublishedMarkerPath is the one marker layout used by snapshot publication
// and legacy-state migration. The btrfs backend places the marker outside the
// read-only state subvolume; other backends keep it inside the state folder.
// See docs/architecture/transformation-attempt-recovery-storage-schema.md, M03.
func PublishedMarkerPath(stateDir, kind string) string {
	if strings.TrimSpace(kind) == "btrfs" {
		return filepath.Join(filepath.Dir(stateDir), ".build", filepath.Base(stateDir)+".ok")
	}
	return filepath.Join(stateDir, ".build.ok")
}

// VerifyPublishedMarker is a read-only preflight. It rejects absence,
// symlinks, directories, or content other than the exact publication marker;
// a caller must separately verify state metadata, seal, and step binding.
func VerifyPublishedMarker(stateDir, kind string) error {
	if !filepath.IsAbs(stateDir) || strings.TrimSpace(stateDir) == "" {
		return fmt.Errorf("published state directory is not absolute")
	}
	path := PublishedMarkerPath(stateDir, kind)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("published state marker unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("published state marker is not a regular file")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("published state marker unreadable: %w", err)
	}
	if string(value) != "ok" {
		return fmt.Errorf("published state marker content differs")
	}
	return nil
}
