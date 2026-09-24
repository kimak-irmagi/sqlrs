package resolver

import "testing"

func TestFilesystemClassesDefaultToUnknown(t *testing.T) {
	for _, class := range []string{"ntfs", "apfs", "ext4", "xfs", "btrfs", "overlayfs", "network", "unknown"} {
		if cheapRevalidationEnabled(class, "v1") {
			t.Fatalf("%s enabled without native evidence", class)
		}
	}
}
