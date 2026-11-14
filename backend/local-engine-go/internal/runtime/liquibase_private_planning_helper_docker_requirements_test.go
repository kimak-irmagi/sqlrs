package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// PC12/PC16/PC18: the default pinned planner produces the v2 wire record in
// its container layer, with no host capture mount. No database/JVM is claimed.
// Exact-ID, randomly labelled fixture cleanup is shared with OT-CA01.
func TestLiquibasePrivatePlanningHelperDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("set SQLRS_LIQUIBASE_DOCKER_E2E=1 for Docker integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	if err != nil {
		t.Fatal("pinned fixture image unavailable", err)
	}
	image := strings.TrimSpace(string(imageBytes))
	if !validCaptureDigest(image) {
		t.Fatal("invalid fixture image identity")
	}
	helper, helperDigest := LiquibaseCaptureHelper()
	helperPath := filepath.Join(t.TempDir(), "capture.sh")
	if err := os.WriteFile(helperPath, helper, 0444); err != nil {
		t.Fatal(err)
	}
	executionDigest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, child, stdout, stderr string
		limit, exit                 int
		overflow                    bool
	}{
		{"private_child", `printf protected-sql; printf protected-error >&2; test "$(id -u)" = 1001; test ! -r /sqlrs/capture; test ! -w /sqlrs/capture`, "protected-sql", "protected-error", 1024, 0, false},
		{"empty", "true", "", "", 1024, 0, false},
		{"nonzero", "printf protected-error >&2; exit 7", "", "protected-error", 1024, 7, false},
		{"wait_eof", "(sleep 0.2; printf late) &", "late", "", 1024, 0, false},
		{"exact_limit", "printf 1234; printf 4321 >&2", "1234", "4321", 4, 0, false},
		{"stdout_overflow", "printf 123456", "", "", 4, 90, true},
		{"stderr_overflow", "printf 123456 >&2", "", "", 4, 90, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"run", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
				"-v", dockerBindSpec(helperPath, "/sqlrs/helper/capture.sh", true),
				"-e", "SQLRS_CAPTURE_DIR=/sqlrs/capture", "-e", "SQLRS_JOB_ID=job-1", "-e", "SQLRS_TASK_ID=plan", "-e", "SQLRS_ATTEMPT_NO=1",
				"-e", "SQLRS_CAPTURE_MAX_BYTES=" + strconv.Itoa(tc.limit), "-e", "SQLRS_EXECUTION_DIGEST=" + executionDigest,
				"-e", "SQLRS_HELPER_DIGEST=" + helperDigest, image, "/sqlrs/helper/capture.sh", "/bin/sh", "-c", tc.child}
			logs, runErr, name, id := runRetainedCaptureTestContainer(t, ctx, args)
			gotExit := 0
			if runErr != nil {
				exit, ok := runErr.(*exec.ExitError)
				if !ok {
					t.Fatal("helper execution failed without observed child status", runErr)
				}
				gotExit = exit.ExitCode()
			}
			if gotExit != tc.exit || len(logs) != 0 {
				t.Fatal("unexpected helper exit or protected output in Docker logs", gotExit, tc.exit)
			}
			runtime := NewDocker(Options{})
			if tc.overflow {
				assertPrivatePlanningOverflow(t, ctx, runtime, id, tc.limit)
				return
			}
			capture, err := runtime.ReadCompleteLiquibaseOutput(ctx, name, id, "unused-host-locator", CaptureIdentity{
				JobID: "job-1", AttemptNo: 1, ExecutionDigest: executionDigest, HelperDigest: helperDigest, ExitCode: tc.exit}, int64(tc.limit))
			if err != nil || string(capture.Stdout) != tc.stdout || string(capture.Stderr) != tc.stderr {
				t.Fatal("private planning capture differs from expected complete streams", err)
			}
		})
	}
}

// assertPrivatePlanningOverflow observes the exact retained test container.
// Only a successful bounded directory read proves marker absence; Docker failure
// is not absence. It never prints protected archive bytes or partial contents.
func assertPrivatePlanningOverflow(t *testing.T, ctx context.Context, runtime *DockerRuntime, id string, limit int) {
	t.Helper()
	reader := runtime.runner.(boundedBinaryRunner)
	archive, err := reader.RunBounded(ctx, runtime.binary, []string{"cp", id + ":/sqlrs/capture/.", "-"}, 65536, 4096)
	if err != nil {
		t.Fatal("private overflow directory inspection unavailable")
	}
	entries := tar.NewReader(bytes.NewReader(archive))
	partials := 0
	for {
		header, err := entries.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal("invalid private overflow archive")
		}
		base := path.Base(header.Name)
		if base == "manifest.complete" {
			t.Fatal("overflow published a completion marker")
		}
		if base == "stdout.partial" || base == "stderr.partial" {
			partials++
			if header.Size > int64(limit) {
				t.Fatal("partial stream exceeded exact byte quota")
			}
		}
	}
	if partials != 2 {
		t.Fatal("did not inspect both real private partial streams", partials)
	}
}
