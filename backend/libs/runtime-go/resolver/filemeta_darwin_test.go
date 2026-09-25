//go:build darwin

package resolver

import "testing"

func TestDarwinFilesystemName(t *testing.T) {
	raw := []int8{'a', 'p', 'f', 's', 0, 'x'}
	if got := darwinFilesystemName(raw); got != "apfs" {
		t.Fatalf("darwinFilesystemName = %q", got)
	}
	if cheapRevalidationEnabled("darwin-unknown", "darwin-stat-v1") {
		t.Fatal("unknown Darwin filesystem unexpectedly enabled")
	}
}
