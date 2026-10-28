//go:build !linux

package snapshot

func zfsSupported(path string) bool {
	return false
}

func newZfsManager() Manager {
	return CopyManager{}
}
