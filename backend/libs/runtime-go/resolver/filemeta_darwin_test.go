//go:build darwin

package resolver

import (
	"os"
	"testing"
)

type darwinFileInfoWithoutStat struct{ os.FileInfo }

func (darwinFileInfoWithoutStat) Sys() any { return nil }

func TestDarwinFilesystemName(t *testing.T) {
	raw := []int8{'a', 'p', 'f', 's', 0, 'x'}
	if got := darwinFilesystemName(raw); got != "apfs" {
		t.Fatalf("darwinFilesystemName = %q", got)
	}
	if cheapRevalidationEnabled("darwin-unknown", "darwin-stat-v1") {
		t.Fatal("unknown Darwin filesystem unexpectedly enabled")
	}
}

func TestDarwinContinuityEvidenceFailsClosed(t *testing.T) {
	temporary, err := os.CreateTemp(t.TempDir(), "evidence-")
	if err != nil {
		t.Fatal(err)
	}
	info, err := temporary.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(temporary, darwinFileInfoWithoutStat{info}); got.Class != "darwin-unknown" || got.Revision != "" {
		t.Fatalf("missing native stat evidence = %+v", got)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(temporary, info); got.Class != "darwin-unknown" || got.Revision != "" {
		t.Fatalf("closed file evidence = %+v", got)
	}

	device, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("devfs unavailable: %v", err)
	}
	defer device.Close()
	deviceInfo, err := device.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(device, deviceInfo); got.Class != "darwin-unknown" || got.Revision != "" {
		t.Fatalf("unapproved filesystem evidence = %+v", got)
	}
}
