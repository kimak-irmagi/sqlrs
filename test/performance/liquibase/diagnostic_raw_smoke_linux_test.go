package benchmark

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// UR06 runs only in explicitly opted-in Actions. It proves raw output under
// the actual unchanged trace filter, not synthetic parser compatibility alone.
func TestDiagnosticRawSyscallSmoke(t *testing.T) {
	if os.Getenv("SQLRS_DIAGNOSTIC_RAW_SMOKE") != "1" {
		t.Skip("explicit Actions raw syscall probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	root := t.TempDir()
	binary := filepath.Join(root, "raw-syscall")
	compile := exec.Command("cc", "-O2", "-Wall", "-Werror", "testdata/raw-syscall.c", "-o", binary)
	compile.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := compile.Start(); err != nil {
		t.Fatal("native fixture compilation failed")
	}
	compiled := make(chan struct{})
	var compileErr error
	go func() { compileErr = compile.Wait(); close(compiled) }()
	select {
	case <-ctx.Done():
	case <-compiled:
	}
	compileCleanup, compileCancel := context.WithTimeout(context.Background(), 9*time.Second)
	compileAbsent := stopDiagnosticGroup(compileCleanup, compile.Process.Pid, compiled) == nil
	compileCancel()
	if !compileAbsent || ctx.Err() != nil || compileErr != nil {
		t.Fatal("native compiler completion/cleanup failed")
	}
	trace, err := newDiagnosticTrace(root)
	if err != nil {
		t.Fatal("private trace unavailable")
	}
	ready, release := filepath.Join(root, "ready"), filepath.Join(root, "release")
	cmd := exec.Command("strace", diagnosticTraceArgs(binary, []string{ready, release})...)
	cmd.ExtraFiles = []*os.File{trace.writer}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		trace.abort()
		t.Fatal("native tracer start failed")
	}
	trace.begin(cmd.Process.Pid)
	waited := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(waited) }()
	finished := false
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cleanupCancel()
		if stopDiagnosticGroup(cleanupCtx, cmd.Process.Pid, waited) != nil {
			t.Error("native probe tree cleanup failed")
		}
		if !finished {
			trace.abort()
		}
		if err := os.Remove(filepath.Join(root, "diagnostic.trace")); err != nil {
			t.Error("native private trace disposal failed")
		}
		if _, err := os.Stat(filepath.Join(root, "diagnostic.trace")); !errors.Is(err, os.ErrNotExist) {
			t.Error("native private trace retained")
		}
	}()
	var childPID int
	for childPID == 0 {
		data, readErr := os.ReadFile(ready)
		if readErr == nil && len(data) > 0 {
			pid, parseErr := strconv.Atoi(string(data))
			if parseErr != nil || verifyDiagnosticEndpoint("/proc", pid, cmd.Process.Pid, binary) != nil {
				t.Fatal("native child identity failed")
			}
			childPID = pid
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("native child readiness timed out")
		case <-waited:
			t.Fatal("native child exited before readiness")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal("native release handshake failed")
	}
	select {
	case <-ctx.Done():
		t.Fatal("native syscall probe timed out")
	case <-waited:
	}
	if waitErr != nil {
		t.Fatal("native syscall did not independently confirm ENOSYS")
	}
	if !errors.Is(syscall.Kill(-cmd.Process.Pid, 0), syscall.ESRCH) {
		t.Fatal("native probe process tree still present")
	}
	rows, err := trace.finishMeasured(-1)
	finished = true
	if err != nil {
		t.Fatal("native raw trace decoding failed")
	}
	data, err := os.ReadFile(filepath.Join(root, "diagnostic.trace"))
	if err != nil {
		t.Fatal("native private evidence unavailable")
	}
	found := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := diagnosticTraceLine.FindStringSubmatch(line)
		if fields == nil || fields[1] != strconv.Itoa(childPID) {
			continue
		}
		call, parseErr := parseDiagnosticRaw(fields[3])
		if parseErr == nil && call.name == "syscall_0x3fffffff" && call.args == [6]uint64{0, 1, ^uint64(0), 2, 0, 3} && !call.pending && !call.unresolved {
			found++
		}
	}
	unattributed := false
	for _, row := range rows {
		if row.Category == "process.unattributed" && row.EndNS > row.StartNS {
			unattributed = true
		}
	}
	if found != 1 || !unattributed {
		t.Fatal("native raw call was not actually observed")
	}
}
