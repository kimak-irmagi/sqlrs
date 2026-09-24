//go:build !windows

package resolver

import "os"

func nativeContinuityEvidence(*os.File, os.FileInfo) continuityEvidence {
	return continuityEvidence{Class: "unsupported"}
}
