package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// CA07/CA09: nonpositive caps and unavailable executables return no payload.
// These checks do not qualify Docker transport or prove a child was never run.
func TestCaptureAccessBoundedBinaryRunnerInvalidInputs(t *testing.T) {
	t.Setenv("SQLRS_CAPTURE_BINARY_HELPER", "1")
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runner, ok := any(execRunner{}).(captureBoundedTestRunner)
	if !ok {
		t.Fatal("approved bounded binary transport is unavailable; input cases not reached")
	}
	args := []string{"-test.run=^TestCaptureAccessBinaryProcess$", "--", "binary"}
	for _, tc := range []struct {
		name, program  string
		stdout, stderr int64
	}{
		{"zero_stdout_cap", program, 0, 4096},
		{"negative_stdout_cap", program, -1, 4096},
		{"zero_stderr_cap", program, 1024, 0},
		{"negative_stderr_cap", program, 1024, -1},
		{"empty_executable", "", 1024, 4096},
		{"missing_executable", filepath.Join(t.TempDir(), "missing-capture-program"), 1024, 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			out, err := runner.RunBounded(ctx, tc.program, args, tc.stdout, tc.stderr)
			if err == nil || len(out) != 0 {
				t.Fatal("invalid binary command/limits returned success or partial payload")
			}
		})
	}
}
