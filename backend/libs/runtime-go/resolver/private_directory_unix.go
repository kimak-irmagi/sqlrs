//go:build !windows

package resolver

import "os"

func preparePrivateDirectoryPlatform(root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	return os.Chmod(root, 0o700)
}
