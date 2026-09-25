//go:build !windows

package resolver

import "os"

func isLinkLike(info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }
