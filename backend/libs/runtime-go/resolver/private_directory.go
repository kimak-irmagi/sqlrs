package resolver

import (
	"os"
	"path/filepath"
)

// preparePrivateDirectory creates a storage root and rejects aliases that
// could redirect cache or artifact writes outside the configured directory.
func preparePrivateDirectory(root string) (string, error) {
	if err := preparePrivateDirectoryPlatform(root); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || isLinkLike(info) {
		return "", ErrUnsafePath
	}
	return filepath.Abs(root)
}
