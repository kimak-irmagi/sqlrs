//go:build linux

package resolver

import (
	"os"
	"strconv"
	"syscall"
)

const (
	ext4SuperMagic    = 0xef53
	xfsSuperMagic     = 0x58465342
	btrfsSuperMagic   = 0x9123683e
	overlaySuperMagic = 0x794c7630
)

// nativeContinuityEvidence returns a revisioned proof only for local Linux
// filesystems covered by native mutation tests. Requirements:
// docs/architecture/runtime-v2-resolver-structure.md.
func nativeContinuityEvidence(file *os.File, info os.FileInfo) continuityEvidence {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return continuityEvidence{Class: "linux-unknown"}
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Fstatfs(int(file.Fd()), &filesystem); err != nil {
		return continuityEvidence{Class: "linux-unknown"}
	}
	class := linuxFilesystemClass(uint64(filesystem.Type))
	if !cheapRevalidationEnabled(class, "linux-stat-v1") {
		return continuityEvidence{Class: class}
	}
	return continuityEvidence{
		Class: class, Revision: "linux-stat-v1",
		VolumeID:    strconv.FormatUint(uint64(stat.Dev), 10),
		FileID:      strconv.FormatUint(stat.Ino, 10),
		ChangeToken: strconv.FormatInt(stat.Ctim.Sec, 10) + ":" + strconv.FormatInt(stat.Ctim.Nsec, 10),
	}
}

func linuxFilesystemClass(magic uint64) string {
	switch magic {
	case ext4SuperMagic:
		return "linux-ext4"
	case xfsSuperMagic:
		return "linux-xfs"
	case btrfsSuperMagic:
		return "linux-btrfs"
	case overlaySuperMagic:
		return "linux-overlay"
	default:
		return "linux-unknown"
	}
}
