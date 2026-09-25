//go:build darwin

package resolver

import (
	"os"
	"strconv"
	"syscall"
)

// nativeContinuityEvidence returns strong evidence only for APFS, whose native
// CI mutation test proves the ctime-based revision. Requirements:
// docs/architecture/runtime-v2-resolver-structure.md.
func nativeContinuityEvidence(file *os.File, info os.FileInfo) continuityEvidence {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return continuityEvidence{Class: "darwin-unknown"}
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Fstatfs(int(file.Fd()), &filesystem); err != nil {
		return continuityEvidence{Class: "darwin-unknown"}
	}
	class := "darwin-unknown"
	if darwinFilesystemName(filesystem.Fstypename[:]) == "apfs" {
		class = "darwin-apfs"
	}
	if !cheapRevalidationEnabled(class, "darwin-stat-v1") {
		return continuityEvidence{Class: class}
	}
	return continuityEvidence{
		Class: class, Revision: "darwin-stat-v1",
		VolumeID:    strconv.FormatInt(int64(stat.Dev), 10),
		FileID:      strconv.FormatUint(stat.Ino, 10),
		ChangeToken: strconv.FormatInt(stat.Ctimespec.Sec, 10) + ":" + strconv.FormatInt(stat.Ctimespec.Nsec, 10),
	}
}

func darwinFilesystemName(raw []int8) string {
	bytes := make([]byte, 0, len(raw))
	for _, value := range raw {
		if value == 0 {
			break
		}
		bytes = append(bytes, byte(value))
	}
	return string(bytes)
}
