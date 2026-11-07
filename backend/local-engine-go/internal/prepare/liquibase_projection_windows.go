package prepare

import (
	"os"

	"golang.org/x/sys/windows"
)

func projectionPlainPlatform(path string, _ os.FileInfo) bool {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attributes, err := windows.GetFileAttributes(name)
	return err == nil && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0
}

// Sync the file before publishing with a write-through same-volume move.
// Windows does not support FlushFileBuffers on directory handles.
func publishProjectionOwner(source, target string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}

func syncProjectionDirectory(string) error { return nil }

func prepareProjectionRemoval(path string, info os.FileInfo) error {
	if info.Mode().IsRegular() && info.Mode().Perm()&0200 == 0 {
		return os.Chmod(path, 0600)
	}
	return nil
}
