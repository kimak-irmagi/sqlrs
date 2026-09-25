package resolver

import "testing"

func TestFilesystemClassesDefaultToUnknown(t *testing.T) {
	for _, class := range []string{"ntfs", "apfs", "ext4", "xfs", "btrfs", "overlayfs", "network", "unknown"} {
		if cheapRevalidationEnabled(class, "v1") {
			t.Fatalf("%s enabled without native evidence", class)
		}
	}
	if !cheapRevalidationEnabled("ntfs-usn", "ntfs-usn-v1") {
		t.Fatal("native-gated NTFS evidence disabled")
	}
	if cheapRevalidationEnabled("ntfs-usn", "other") {
		t.Fatal("unknown NTFS evidence revision enabled")
	}
}
