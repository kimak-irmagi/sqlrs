//go:build !windows

package instanceaccess

import (
	"os"
	"syscall"
)

func createPrivateDirectory(path string) error { return os.Mkdir(path, 0700) }
func ownPrivateFile(string) error              { return nil }
func checkPrivatePath(_ string, info os.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0077 != 0 {
		return ErrInvalid
	}
	return nil
}
func syncPrivateDirectory(root secretRoot, _ string) error {
	f, err := root.Open(".")
	if err != nil {
		return ErrUnavailable
	}
	defer f.Close()
	if f.Sync() != nil {
		return ErrUnavailable
	}
	return nil
}
