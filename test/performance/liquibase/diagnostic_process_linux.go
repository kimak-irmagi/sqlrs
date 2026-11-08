package benchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// diagnosticTrace owns only the external test's private pipe/file. The engine
// executable is unchanged. See docs/architecture/issue89-performance-diagnostic-design.md
// and issue89-performance-diagnostic-tests.md DG06–DG07.
type diagnosticTrace struct {
	writer, reader, file *os.File
	done                 chan error
}

func newDiagnosticTrace(root string) (*diagnosticTrace, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || !filepath.IsAbs(root) || resolved != root || filepath.Dir(root) == root {
		return nil, errDiagnosticTrace
	}
	f, err := os.OpenFile(filepath.Join(root, "diagnostic.trace"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errDiagnosticTrace
	}
	r, w, err := os.Pipe()
	if err != nil {
		f.Close()
		return nil, errDiagnosticTrace
	}
	return &diagnosticTrace{writer: w, reader: r, file: f}, nil
}

// diagnosticTraceArgs records exec records plus strace's terminal markers only;
// raw argument bytes go to the private FD, never stdout/stderr or artifacts.
func diagnosticTraceArgs(engine string, args []string) []string {
	prefix := []string{"-f", "-ttt", "-T", "-s", "4096", "-e", "trace=execve,execveat", "-e", "signal=none", "-o", "/proc/self/fd/3", "--", engine}
	return append(prefix, args...)
}

func (t *diagnosticTrace) begin(group int) {
	t.writer.Close()
	t.done = make(chan error, 1)
	go func() {
		err := copyDiagnosticTrace(t.file, t.reader, 32<<20)
		if err != nil && group > 1 && group != syscall.Getpgrp() {
			_ = syscall.Kill(-group, syscall.SIGTERM)
		}
		t.reader.Close()
		t.done <- err
	}()
}

// finish is called only after the owned process tree is absent. Closing a timed
// out pipe unblocks and joins its exact reader; no detached goroutine survives.
func (t *diagnosticTrace) finish(enginePID int) ([]diagnosticInterval, error) {
	if t.done == nil {
		t.abort()
		return nil, errDiagnosticTrace
	}
	var err error
	select {
	case err = <-t.done:
	case <-time.After(10 * time.Second):
		t.reader.Close()
		<-t.done
		err = errDiagnosticTrace
	}
	defer t.file.Close()
	if err != nil {
		return nil, errDiagnosticTrace
	}
	if _, err := t.file.Seek(0, 0); err != nil {
		return nil, errDiagnosticTrace
	}
	return decodeDiagnosticTrace(t.file, enginePID)
}

func (t *diagnosticTrace) abort() {
	t.writer.Close()
	t.reader.Close()
	if t.done != nil {
		<-t.done
	}
	t.file.Close()
}

// verifyDiagnosticEndpoint binds strace's real child, process group and exact
// executable before its endpoint/token can be trusted. Untraced equality stays
// in the old collector. procRoot is /proc in the native observer (DG07).
func verifyDiagnosticEndpoint(procRoot string, pid, launcher int, executable string) error {
	if pid < 2 || launcher < 2 || pid == launcher || !filepath.IsAbs(executable) {
		return errDiagnosticTrace
	}
	dir := filepath.Join(procRoot, strconv.Itoa(pid))
	b, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return errDiagnosticTrace
	}
	close := strings.LastIndex(string(b), ")")
	if close < 0 {
		return errDiagnosticTrace
	}
	fields := strings.Fields(string(b[close+1:]))
	if len(fields) < 3 || fields[0] == "Z" || fields[0] == "X" {
		return errDiagnosticTrace
	}
	parent, e1 := strconv.Atoi(fields[1])
	group, e2 := strconv.Atoi(fields[2])
	if e1 != nil || e2 != nil || parent != launcher || group != launcher {
		return errDiagnosticTrace
	}
	target, err := os.Readlink(filepath.Join(dir, "exe"))
	if err != nil || filepath.Clean(target) != filepath.Clean(executable) {
		return errDiagnosticTrace
	}
	return nil
}

// stopDiagnosticGroup never targets this test's own group or a broad PID. A
// tracer exit does not substitute for actual group absence. DG07/OT-DG03.
func stopDiagnosticGroup(ctx context.Context, group int, wait <-chan struct{}) error {
	if group < 2 || group == syscall.Getpgrp() {
		return errDiagnosticTrace
	}
	_ = syscall.Kill(-group, syscall.SIGTERM)
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	killed := false
	for {
		gone := errors.Is(syscall.Kill(-group, 0), syscall.ESRCH)
		reaped := false
		select {
		case <-wait:
			reaped = true
		default:
		}
		if gone && reaped {
			return nil
		}
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-group, syscall.SIGKILL)
			return errDiagnosticTrace
		case <-timer.C:
			if killed {
				return errDiagnosticTrace
			}
			_ = syscall.Kill(-group, syscall.SIGKILL)
			killed = true
			timer.Reset(5 * time.Second)
		case <-time.After(20 * time.Millisecond):
		}
	}
}
