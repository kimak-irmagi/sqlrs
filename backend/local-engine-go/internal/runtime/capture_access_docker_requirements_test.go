package runtime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// CA13: actual helper-produced root-owned files must be readable by the runtime
// without granting the ordinary Linux engine UID filesystem access. Windows
// exercises the working local route, not a fabricated Unix permission failure.
// See docs/architecture/container-capture-access-tests.md. Selecting the Docker
// suite requires its prerequisites; root Linux execution is a failure, not a skip.
func TestCaptureAccessPlannerDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("Docker suite not selected; set SQLRS_LIQUIBASE_DOCKER_E2E=1")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		t.Fatal("CA13 requires an ordinary Linux engine user")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	if err != nil {
		t.Fatal("required Liquibase image unavailable")
	}
	image := strings.TrimSpace(string(imageBytes))
	if !validCaptureDigest(image) {
		t.Fatal("unresolved Liquibase image identity")
	}
	helper, helperDigest := LiquibaseCaptureHelper()
	helperPath := filepath.Join(t.TempDir(), "capture.sh")
	if err := os.WriteFile(helperPath, helper, 0444); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, command  string
		stdout, stderr []byte
		exit           int
	}{
		{name: "empty", command: "true"},
		{name: "binary", command: `printf '\000\377\200\012'; printf '\376\000e' >&2`, stdout: []byte{0, 255, 128, 10}, stderr: []byte{254, 0, 'e'}},
		{name: "natural_failure", command: "printf protected-capture-error >&2; exit 7", stderr: []byte("protected-capture-error"), exit: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			identity := CaptureIdentity{JobID: "capture-access-job", AttemptNo: 1, ExecutionDigest: "sha256:" + strings.Repeat("a", 64), HelperDigest: helperDigest, ExitCode: tc.exit}
			args := []string{"run", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
				"-v", dockerBindSpec(helperPath, "/sqlrs/helper/capture.sh", true),
				"-v", dockerBindSpec(dir, "/sqlrs/capture", false),
				"-e", "SQLRS_CAPTURE_DIR=/sqlrs/capture", "-e", "SQLRS_JOB_ID=" + identity.JobID,
				"-e", "SQLRS_TASK_ID=plan", "-e", "SQLRS_ATTEMPT_NO=1",
				"-e", "SQLRS_CAPTURE_MAX_BYTES=1024", "-e", "SQLRS_EXECUTION_DIGEST=" + identity.ExecutionDigest,
				"-e", "SQLRS_HELPER_DIGEST=" + helperDigest,
				image, "/sqlrs/helper/capture.sh", "/bin/sh", "-c", tc.command}
			output, runErr, name, id := runRetainedCaptureTestContainer(t, ctx, args)
			if len(output) != 0 {
				t.Fatal("protected capture leaked into Docker diagnostic output")
			}
			if tc.exit == 0 {
				if runErr != nil {
					t.Fatal("capture helper failed")
				}
			} else if exit, ok := runErr.(*exec.ExitError); !ok || exit.ExitCode() != tc.exit {
				t.Fatal("natural child exit not preserved")
			}
			if runtime.GOOS == "linux" {
				for _, file := range []string{"manifest.complete", "stdout.complete", "stderr.complete"} {
					info, err := os.Stat(filepath.Join(dir, file))
					if err != nil || info.Mode().Perm() != 0600 {
						t.Fatal("private helper file mode changed")
					}
					if _, err := os.ReadFile(filepath.Join(dir, file)); !errors.Is(err, os.ErrPermission) {
						t.Fatal("real local permission denial not established")
					}
				}
			}
			got, err := NewDocker(Options{}).ReadCompleteLiquibaseOutput(ctx, name, id, dir, identity, 1024)
			if err != nil {
				t.Fatalf("exact stopped helper capture unavailable: permission=%t error_type=%T", errors.Is(err, os.ErrPermission), err)
			}
			if !reflect.DeepEqual(got.Stdout, nonNilBytes(tc.stdout)) || !reflect.DeepEqual(got.Stderr, nonNilBytes(tc.stderr)) ||
				got.StdoutDigest != captureDigest(tc.stdout) || got.StderrDigest != captureDigest(tc.stderr) {
				t.Fatal("binary capture bytes or hashes changed")
			}
			// Reading is not permitted to remove or replace the retained executor.
			view, found, err := NewDocker(Options{}).InspectNamedContainer(ctx, name)
			if err != nil || !found || view.ID != id || view.Status != "exited" || view.ExitCode != tc.exit {
				t.Fatal("capture read changed executor lifecycle")
			}
		})
	}
}
