//go:build linux

package resolver

import (
	"os"
	"testing"
)

type linuxFileInfoWithoutStat struct{ os.FileInfo }

func (linuxFileInfoWithoutStat) Sys() any { return nil }

func TestLinuxFilesystemClassAllowlist(t *testing.T) {
	tests := map[uint64]string{
		ext4SuperMagic:    "linux-ext4",
		xfsSuperMagic:     "linux-xfs",
		btrfsSuperMagic:   "linux-btrfs",
		overlaySuperMagic: "linux-overlay",
		0:                 "linux-unknown",
	}
	for magic, want := range tests {
		if got := linuxFilesystemClass(magic); got != want {
			t.Fatalf("linuxFilesystemClass(%x) = %q, want %q", magic, got, want)
		}
	}
	if cheapRevalidationEnabled("linux-overlay", "linux-stat-v1") {
		t.Fatal("overlay filesystem unexpectedly enabled")
	}
}

func TestLinuxContinuityEvidenceFailsClosed(t *testing.T) {
	temporary, err := os.CreateTemp(t.TempDir(), "evidence-")
	if err != nil {
		t.Fatal(err)
	}
	info, err := temporary.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(temporary, linuxFileInfoWithoutStat{info}); got.Class != "linux-unknown" || got.Revision != "" {
		t.Fatalf("missing native stat evidence = %+v", got)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(temporary, info); got.Class != "linux-unknown" || got.Revision != "" {
		t.Fatalf("closed file evidence = %+v", got)
	}

	proc, err := os.Open("/proc/version")
	if err != nil {
		t.Skipf("procfs unavailable: %v", err)
	}
	defer proc.Close()
	procInfo, err := proc.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(proc, procInfo); got.Class != "linux-unknown" || got.Revision != "" {
		t.Fatalf("unapproved filesystem evidence = %+v", got)
	}
}
