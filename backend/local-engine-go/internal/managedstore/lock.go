package managedstore

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrStoreOwned = errors.New("managed store is already owned by another engine or its lock is unavailable")

// Lock uses an OS-held exclusive lock. Process termination releases ownership;
// the sentinel is never removed, avoiding unlink/recreate ownership races.
func Lock(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrStoreOwned
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, ErrStoreOwned
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, ErrStoreOwned
	}
	if lockFile(f) != nil {
		f.Close()
		return nil, ErrStoreOwned
	}
	return f, nil
}
