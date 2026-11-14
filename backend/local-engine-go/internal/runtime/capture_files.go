package runtime

import (
	"context"
	"path/filepath"
)

// captureFileReader separates byte acquisition from the one manifest/hash
// verifier. Only engine-controlled fixed basenames reach it; it is not supplied
// by recipes/plugins. See container-capture-access-amendment.md section 3.
type captureFileReader interface {
	readCapture(string, int64) ([]byte, error)
}

// filesystemCaptureFiles keeps the exported filesystem-only verification
// helpers usable without Docker authority. New private executions do not select
// this source, even when old host files happen to exist or remain readable.
type filesystemCaptureFiles struct{ dir string }

func (f filesystemCaptureFiles) readCapture(file string, limit int64) ([]byte, error) {
	return readRegularBounded(filepath.Join(f.dir, file), limit)
}

// containerCaptureFiles binds every file to one inspected physical allocation
// and one shared deadline. There is no local-first or mixed-source fallback.
type containerCaptureFiles struct {
	ctx        context.Context
	runtime    *DockerRuntime
	id         string
	invocation string
}

func (f containerCaptureFiles) readCapture(file string, limit int64) ([]byte, error) {
	return f.runtime.readCaptureArtifact(f.ctx, f.id, f.invocation, file, limit)
}
