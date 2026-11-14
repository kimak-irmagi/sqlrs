package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// captureBoundedTestRunner describes the approved binary-command boundary by
// observable inputs/outputs without requiring a production type declaration.
// The initial red run records missing capability, not qualified overflow or
// cancellation behavior. CA07/08/09 subcases become reached after implementation.
type captureBoundedTestRunner interface {
	RunBounded(context.Context, string, []string, int64, int64) ([]byte, error)
}

// TestCaptureAccessBinaryProcess is a controlled executable fixture. Its output
// is independent of the Docker transport/parser, including invalid UTF-8 and
// simultaneous stdout/stderr. The parent always supplies cancellation.
func TestCaptureAccessBinaryProcess(t *testing.T) {
	if os.Getenv("SQLRS_CAPTURE_BINARY_HELPER") != "1" {
		return
	}
	args := os.Args
	mode := args[len(args)-1]
	switch mode {
	case "binary":
		_, _ = os.Stdout.Write([]byte{0, 255, 128, '\n'})
		_, _ = os.Stderr.Write([]byte("private-diagnostic-marker"))
	case "stdout_at_cap":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, 1024))
	case "stderr_at_cap":
		_, _ = os.Stderr.Write(bytes.Repeat([]byte{'x'}, 4096))
	case "stdout_overflow", "stderr_overflow":
		file, count := os.Stdout, 1025
		if mode == "stderr_overflow" {
			file, count = os.Stderr, 4097
		}
		_, _ = file.Write(bytes.Repeat([]byte{'x'}, count))
	case "simultaneous":
		done := make(chan struct{})
		go func() { _, _ = os.Stderr.Write(bytes.Repeat([]byte{'e'}, 4096)); close(done) }()
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'o'}, 1024))
		<-done
	case "nonzero":
		_, _ = os.Stdout.Write([]byte("complete-looking-content"))
		_, _ = os.Stderr.Write([]byte("private-diagnostic-marker"))
		os.Exit(7)
	case "stall":
		time.Sleep(time.Minute)
	case "continuous_stdout", "continuous_stderr":
		file := os.Stdout
		if mode == "continuous_stderr" {
			file = os.Stderr
		}
		for {
			if _, err := file.Write(bytes.Repeat([]byte{'x'}, 8192)); err != nil {
				os.Exit(0)
			}
		}
	default:
		_, _ = fmt.Fprint(os.Stderr, "unknown controlled process mode")
		os.Exit(9)
	}
	os.Exit(0)
}

// CA07/08/09: exercise the real command runner, not a fake that pretends to
// enforce caps. Caps apply during acquisition, diagnostics never contaminate
// binary stdout, and failed commands never return a partial successful payload.
func TestCaptureAccessBoundedBinaryRunner(t *testing.T) {
	t.Setenv("SQLRS_CAPTURE_BINARY_HELPER", "1")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode      string
		want      []byte
		wantErr   bool
		cancelled bool
	}{
		{mode: "binary", want: []byte{0, 255, 128, '\n'}},
		{mode: "stdout_at_cap", want: bytes.Repeat([]byte{'x'}, 1024)},
		{mode: "stderr_at_cap"},
		{mode: "simultaneous", want: bytes.Repeat([]byte{'o'}, 1024)},
		{mode: "stdout_overflow", wantErr: true},
		{mode: "stderr_overflow", wantErr: true},
		{mode: "nonzero", wantErr: true},
		{mode: "stall", wantErr: true},
		{mode: "continuous_stdout", wantErr: true},
		{mode: "continuous_stderr", wantErr: true},
		{mode: "binary", wantErr: true, cancelled: true},
	} {
		name := tc.mode
		if tc.cancelled {
			name = "already_cancelled"
		}
		t.Run(name, func(t *testing.T) {
			runner, ok := any(execRunner{}).(captureBoundedTestRunner)
			if !ok {
				t.Fatal("approved bounded binary transport is not implemented; subprocess case not reached")
			}
			// OT-MP03 separates cold process startup from the targeted cancellation
			// case; quotas and the seven-second cleanup bound remain unchanged.
			budget := 5 * time.Second
			if tc.mode == "stall" || tc.cancelled {
				budget = time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			started := time.Now()
			out, err := runner.RunBounded(ctx, binary, []string{"-test.run=^TestCaptureAccessBinaryProcess$", "--", tc.mode}, 1024, 4096)
			if elapsed := time.Since(started); elapsed > 7*time.Second {
				t.Fatalf("process/pipe cleanup exceeded caller deadline plus drain allowance: %s", elapsed)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected command outcome: mode=%s failed=%t", tc.mode, err != nil)
			}
			if tc.wantErr {
				if len(out) != 0 {
					t.Fatal("failed binary command returned partial payload")
				}
				if strings.Contains(err.Error(), "private-diagnostic-marker") {
					t.Fatal("private Docker diagnostic body exposed")
				}
			} else if !bytes.Equal(out, tc.want) {
				t.Fatal("stdout changed or stderr entered binary output")
			}
		})
	}
}
