package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// C01a/C02a/C05a: the real Liquibase image runs the verified helper as root
// while its child is unprivileged and cannot inspect or alter capture files.
// This focused Docker test needs no database or JDBC driver.
func TestLiquibaseCaptureHelperDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("set SQLRS_LIQUIBASE_DOCKER_E2E=1 for Docker integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	imageCmd := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}")
	imageBytes, err := imageCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	image := strings.TrimSpace(string(imageBytes))
	if !strings.HasPrefix(image, "sha256:") {
		t.Fatalf("unresolved image ID %q", image)
	}
	helper, helperDigest := LiquibaseCaptureHelper()
	helperPath := filepath.Join(t.TempDir(), "capture.sh")
	if err := os.WriteFile(helperPath, helper, 0444); err != nil {
		t.Fatal(err)
	}
	executionDigest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, child string
		want        string
	}{
		{"nonempty isolated", "printf secret-sql; printf secret-error >&2; id -u; [ ! -r /sqlrs/capture ] && [ ! -w /sqlrs/capture ]", "secret-sql1001\n"},
		{"empty", "true", ""},
		{"wait for stream EOF", "(sleep 0.2; printf late) &", "late"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captureDir := t.TempDir()
			args := []string{"run", "--rm", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
				"-v", dockerBindSpec(helperPath, "/sqlrs/helper/capture.sh", true),
				"-e", "SQLRS_CAPTURE_DIR=/sqlrs/capture", "-e", "SQLRS_JOB_ID=job-1",
				"-e", "SQLRS_TASK_ID=plan", "-e", "SQLRS_ATTEMPT_NO=1",
				"-e", "SQLRS_CAPTURE_MAX_BYTES=1024",
				"-e", "SQLRS_EXECUTION_DIGEST=" + executionDigest, "-e", "SQLRS_HELPER_DIGEST=" + helperDigest,
				image, "/sqlrs/helper/capture.sh", "/bin/sh", "-c", tc.child}
			output, err, name, id := runRetainedCaptureTestContainer(t, ctx, args)
			if err != nil {
				t.Fatalf("capture helper failed: %v (%s)", err, output)
			}
			if strings.Contains(string(output), "secret-sql") || strings.Contains(string(output), "secret-error") {
				t.Fatal("protected stream reached Docker logs")
			}
			capture, err := NewDocker(Options{}).ReadCompleteLiquibaseOutput(ctx, name, id, captureDir, CaptureIdentity{JobID: "job-1", AttemptNo: 1, ExecutionDigest: executionDigest, HelperDigest: helperDigest, ExitCode: 0}, 1024)
			if err != nil {
				t.Fatal(err)
			}
			if string(capture.Stdout) != tc.want {
				t.Fatalf("stdout=%q want=%q", capture.Stdout, tc.want)
			}
			if tc.name == "nonempty isolated" && string(capture.Stderr) != "secret-error" {
				t.Fatalf("stderr=%q", capture.Stderr)
			}
		})
	}
	t.Run("mounted inputs stay readable and immutable", func(t *testing.T) {
		// C05a: a Liquibase child can consume bound inputs, but cannot alter
		// source, driver, or settings bytes or address another job's capture.
		inputDir := t.TempDir()
		captureDir := t.TempDir()
		for _, name := range []string{"source.sql", "driver.jar", "liquibase.properties"} {
			if err := os.WriteFile(filepath.Join(inputDir, name), []byte("original"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		child := `for path in /sqlrs/source/source.sql /sqlrs/driver/driver.jar /sqlrs/settings/liquibase.properties; do
  test -r "$path" || exit 13
  if (printf changed >> "$path") 2>/dev/null; then exit 14; fi
done
test ! -e /sqlrs/other-job-capture || exit 15
printf inputs-safe`
		args := []string{"run", "--rm", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
			"-v", dockerBindSpec(helperPath, "/sqlrs/helper/capture.sh", true),
			"-v", dockerBindSpec(filepath.Join(inputDir, "source.sql"), "/sqlrs/source/source.sql", true),
			"-v", dockerBindSpec(filepath.Join(inputDir, "driver.jar"), "/sqlrs/driver/driver.jar", true),
			"-v", dockerBindSpec(filepath.Join(inputDir, "liquibase.properties"), "/sqlrs/settings/liquibase.properties", true),
			"-e", "SQLRS_CAPTURE_DIR=/sqlrs/capture", "-e", "SQLRS_JOB_ID=job-1",
			"-e", "SQLRS_TASK_ID=plan", "-e", "SQLRS_ATTEMPT_NO=1",
			"-e", "SQLRS_CAPTURE_MAX_BYTES=1024",
			"-e", "SQLRS_EXECUTION_DIGEST=" + executionDigest, "-e", "SQLRS_HELPER_DIGEST=" + helperDigest,
			image, "/sqlrs/helper/capture.sh", "/bin/sh", "-c", child}
		output, err, name, id := runRetainedCaptureTestContainer(t, ctx, args)
		if err != nil {
			t.Fatalf("read-only input check failed: %v (%s)", err, output)
		}
		capture, err := NewDocker(Options{}).ReadCompleteLiquibaseOutput(ctx, name, id, captureDir, CaptureIdentity{JobID: "job-1", AttemptNo: 1, ExecutionDigest: executionDigest, HelperDigest: helperDigest, ExitCode: 0}, 1024)
		if err != nil || string(capture.Stdout) != "inputs-safe" {
			t.Fatalf("capture=%+v err=%v", capture, err)
		}
		for _, name := range []string{"source.sql", "driver.jar", "liquibase.properties"} {
			got, err := os.ReadFile(filepath.Join(inputDir, name))
			if err != nil || string(got) != "original" {
				t.Fatalf("%s changed: %q (%v)", name, got, err)
			}
		}
	})
	t.Run("nonzero child keeps protected diagnostics", func(t *testing.T) {
		captureDir := t.TempDir()
		args := []string{"run", "--rm", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
			"-v", dockerBindSpec(helperPath, "/sqlrs/helper/capture.sh", true),
			"-e", "SQLRS_CAPTURE_DIR=/sqlrs/capture", "-e", "SQLRS_JOB_ID=job-1",
			"-e", "SQLRS_TASK_ID=plan", "-e", "SQLRS_ATTEMPT_NO=1",
			"-e", "SQLRS_CAPTURE_MAX_BYTES=1024",
			"-e", "SQLRS_EXECUTION_DIGEST=" + executionDigest, "-e", "SQLRS_HELPER_DIGEST=" + helperDigest,
			image, "/sqlrs/helper/capture.sh", "/bin/sh", "-c", "printf protected-error >&2; exit 7"}
		output, err, name, id := runRetainedCaptureTestContainer(t, ctx, args)
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
			t.Fatalf("exit=%v logs=%q", err, output)
		}
		if strings.Contains(string(output), "protected-error") {
			t.Fatal("failed child leaked diagnostics to Docker logs")
		}
		capture, err := NewDocker(Options{}).ReadCompleteLiquibaseOutput(ctx, name, id, captureDir, CaptureIdentity{JobID: "job-1", AttemptNo: 1, ExecutionDigest: executionDigest, HelperDigest: helperDigest, ExitCode: 7}, 1024)
		if err != nil || string(capture.Stderr) != "protected-error" {
			t.Fatalf("failed capture=%+v err=%v", capture, err)
		}
	})
	t.Run("quota prevents completion marker", func(t *testing.T) {
		for _, child := range []string{"printf 123456", "printf 123456 >&2"} {
			args := []string{"run", "--rm", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
				"-v", dockerBindSpec(helperPath, "/sqlrs/helper/capture.sh", true),
				"-e", "SQLRS_CAPTURE_DIR=/sqlrs/capture", "-e", "SQLRS_JOB_ID=job-1",
				"-e", "SQLRS_TASK_ID=plan", "-e", "SQLRS_ATTEMPT_NO=1",
				"-e", "SQLRS_CAPTURE_MAX_BYTES=4",
				"-e", "SQLRS_EXECUTION_DIGEST=" + executionDigest, "-e", "SQLRS_HELPER_DIGEST=" + helperDigest,
				image, "/sqlrs/helper/capture.sh", "/bin/sh", "-c", child}
			output, err, _, id := runRetainedCaptureTestContainer(t, ctx, args)
			if err == nil || strings.Contains(string(output), "123456") {
				t.Fatalf("over-quota capture exit=%v logs=%q", err, output)
			}
			assertPrivatePlanningOverflow(t, ctx, NewDocker(Options{}), id, 4)
		}
	})
}

// runRetainedCaptureTestContainer implements the approved OT-CA01 fixture
// adjustment. Only the exact ID with this invocation's random ownership label
// may be removed. Retention permits ordinary Linux runtime verification of
// root-owned captures; it does not change capture permissions or child checks.
func runRetainedCaptureTestContainer(t *testing.T, ctx context.Context, args []string) ([]byte, error, string, string) {
	t.Helper()
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	owner := hex.EncodeToString(nonce[:])
	name := "sqlrs-lb-capture-access-" + owner
	const label = "sqlrs.capture-access-test"
	if len(args) == 0 || args[0] != "run" {
		t.Fatal("retained fixture requires docker run")
	}
	retained := []string{"run", "--name", name, "--label", label + "=" + owner}
	for _, arg := range args[1:] {
		if arg != "--rm" {
			retained = append(retained, arg)
		}
	}
	var exactID string
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		ids, err := exec.CommandContext(cleanupCtx, "docker", "container", "ls", "-aq", "--no-trunc",
			"--filter", "name=^/"+name+"$", "--filter", "label="+label+"="+owner).Output()
		if err != nil {
			t.Error("cannot inventory exact owned capture fixture")
			return
		}
		id := strings.TrimSpace(string(ids))
		if id == "" {
			return
		}
		if !createdContainerID.MatchString(id) || (exactID != "" && id != exactID) {
			t.Error("capture fixture ownership/identity changed; refusing removal")
			return
		}
		if err := exec.CommandContext(cleanupCtx, "docker", "rm", "-f", "-v", id).Run(); err != nil {
			t.Error("cannot remove exact owned capture fixture")
			return
		}
		left, err := exec.CommandContext(cleanupCtx, "docker", "container", "ls", "-aq", "--no-trunc", "--filter", "id="+id).Output()
		if err != nil || len(strings.TrimSpace(string(left))) != 0 {
			t.Error("capture fixture absence not confirmed")
		}
	})
	out, runErr := exec.CommandContext(ctx, "docker", retained...).CombinedOutput()
	data, err := exec.CommandContext(ctx, "docker", "container", "inspect", "--format", "{{json .}}", name).Output()
	if err != nil {
		t.Fatal("retained capture fixture was not created")
	}
	var details struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(data, &details); err != nil || !createdContainerID.MatchString(details.ID) ||
		details.Name != "/"+name || details.Config.Labels[label] != owner {
		t.Fatal("retained capture fixture identity not verified")
	}
	exactID = details.ID
	return out, runErr, name, exactID
}
