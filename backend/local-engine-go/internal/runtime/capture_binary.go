package runtime

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// boundedBinaryRunner is the private byte-preserving command boundary shared by
// capture readers and the same-exec result transport. The caller supplies its
// shared read or original SQL-work deadline respectively;
// a command must not start a fresh per-file timeout. See
// docs/architecture/container-capture-access-amendment.md section 3.
type boundedBinaryRunner interface {
	RunBounded(context.Context, string, []string, int64, int64) ([]byte, error)
}

var (
	errCaptureBinaryLimits  = errors.New("invalid capture command limits")
	errCaptureBinaryQuota   = errors.New("capture command exceeded output limit")
	errCaptureBinaryCommand = errors.New("capture binary command failed")
)

// captureCommandBuffer belongs to exactly one os/exec copy goroutine. stdout is
// retained as bytes; stderr is counted and discarded, never retained in an error.
// Its cancellation function terminates only the local command, not Liquibase or
// any container. Read fields only after Cmd.Run has joined its I/O goroutines.
type captureCommandBuffer struct {
	limit    int64
	count    int64
	retain   bool
	overflow bool
	cancel   context.CancelFunc
	data     bytes.Buffer
}

// Write enforces the quota during acquisition. Check remaining capacity before
// adding lengths, so even very large configured int64 bounds cannot overflow.
func (b *captureCommandBuffer) Write(p []byte) (int, error) {
	if int64(len(p)) > b.limit-b.count {
		b.overflow = true
		b.cancel()
		return 0, errCaptureBinaryQuota
	}
	b.count += int64(len(p))
	if b.retain {
		return b.data.Write(p)
	}
	return len(p), nil
}

// RunBounded acquires binary stdout and bounded diagnostic stderr separately.
// Both caps must be positive. Any quota, cancellation, start, exit or drain error
// invalidates the entire payload; no partial stdout or diagnostic body escapes.
// Cmd.Run reaps the local producer and WaitDelay limits inherited-pipe draining
// to five additional seconds. Independent readers own a shared 30-second capture
// deadline; same-exec SQL uses its original work deadline, never a new 30-second
// timeout. This method never resets either. Requirements: capture-access CA07–CA09
// and liquibase-private-capture-ownership.md bounded-transport rules.
func (execRunner) RunBounded(ctx context.Context, name string, args []string, stdoutCap, stderrCap int64) ([]byte, error) {
	if stdoutCap <= 0 || stderrCap <= 0 {
		return nil, errCaptureBinaryLimits
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout := captureCommandBuffer{limit: stdoutCap, retain: true, cancel: cancel}
	stderr := captureCommandBuffer{limit: stderrCap, cancel: cancel}
	cmd := exec.CommandContext(commandCtx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 5 * time.Second
	hideWindow(cmd)
	err := cmd.Run()
	if stdout.overflow || stderr.overflow {
		return nil, errCaptureBinaryQuota
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err != nil {
		// Do not wrap exec errors: diagnostic bodies or command arguments
		// must not become public text via a later caller's error formatting.
		return nil, errCaptureBinaryCommand
	}
	return stdout.data.Bytes(), nil
}
