package snapshot

import (
	"fmt"
	"strings"
)

// ValidateStore ensures the state store matches the snapshot backend requirements.
func ValidateStore(kind string, root string) error {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "":
		return nil
	case "btrfs":
		root = strings.TrimSpace(root)
		if root == "" {
			return fmt.Errorf("state store root is required")
		}
		if btrfsSupportedFn(root) {
			return nil
		}
		return fmt.Errorf("state store is not mounted as btrfs: %s", root)
	case "zfs":
		root = strings.TrimSpace(root)
		if root == "" {
			return fmt.Errorf("state store root is required")
		}
		if zfsSupportedFn(root) {
			return nil
		}
		return fmt.Errorf("state store is not mounted as zfs: %s", root)
	default:
		return nil
	}
}
