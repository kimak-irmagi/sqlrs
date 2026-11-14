//go:build !windows

package prepare

import "os"

func projectionPlainPlatform(_ string, info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == 0
}

func publishProjectionOwner(source, target string) error { return os.Rename(source, target) }

func syncProjectionDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func prepareProjectionRemoval(string, os.FileInfo) error { return nil }
