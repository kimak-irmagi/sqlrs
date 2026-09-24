//go:build windows

package resolver

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func replaceFile(source, target string) error {
	from, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	var callErr error
	for attempt := 0; attempt < 20; attempt++ {
		result, _, err := moveFileEx.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), 0x1|0x8)
		if result != 0 {
			return nil
		}
		callErr = err
		time.Sleep(5 * time.Millisecond)
	}
	return callErr
}
func syncDirectory(string) error { return nil }

var _ = os.ErrNotExist
