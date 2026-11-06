package runtime

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// PC04/05/08/16 native helper checks: the real untrusted child writes its own
// independent counter. A replay must not overwrite output or increment it.
// This qualifies the script, not journal admission, SQL recovery or pruning.
func TestLiquibasePrivateChildDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("native private child suite not selected")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		t.Fatal("ordinary Linux engine UID required")
	}
	helper, err := os.ReadFile("assets/liquibase-step-capture-v2.sh")
	if err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(t.TempDir(), "step-capture.sh")
	if err := os.WriteFile(helperPath, helper, 0444); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	if err != nil {
		t.Fatal("required native image unavailable")
	}
	image := strings.TrimSpace(string(imageBytes))
	if !validCaptureDigest(image) {
		t.Fatal("unresolved image")
	}
	for _, variant := range []string{"binary", "empty", "failure", "exact_limit", "over_limit", "stderr_over_limit", "no_ready", "wrong_config", "wrong_generation", "untrusted_uid"} {
		t.Run(variant, func(t *testing.T) {
			readyWant := SessionReadinessIdentity{SessionID: "private-child-session", WorkerGeneration: 2,
				RunnerConfigDigest: "sha256:" + strings.Repeat("b", 64), HelperDigest: captureDigest(helper)}
			args := []string{"run", "-d", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
				"-v", dockerBindSpec(helperPath, "/private-helper/step-capture.sh", true),
				"-e", "SQLRS_SESSION_ID=" + readyWant.SessionID, "-e", "SQLRS_WORKER_GENERATION=2",
				"-e", "SQLRS_RUNNER_CONFIG_DIGEST=" + readyWant.RunnerConfigDigest, "-e", "SQLRS_HELPER_DIGEST=" + readyWant.HelperDigest,
				image, "/private-helper/step-capture.sh", "serve"}
			_, runErr, name, id := runRetainedCaptureTestContainer(t, ctx, args)
			if runErr != nil || !createdContainerID.MatchString(id) {
				t.Fatal("private parent failed to launch")
			}
			readyWant.RunnerID = id
			rt := NewDocker(Options{Binary: "docker"})
			readyCtx, readyCancel := context.WithTimeout(ctx, 15*time.Second)
			defer readyCancel()
			for {
				if _, err := rt.ReadLiquibaseSessionReadiness(readyCtx, name, readyWant); err == nil {
					break
				}
				if readyCtx.Err() != nil {
					t.Fatal("private parent never became ready")
				}
				time.Sleep(25 * time.Millisecond)
			}
			identity := StepCaptureIdentity{JobID: "private-child-job", TaskID: "step-1", SessionID: readyWant.SessionID,
				InvocationRef: "private-call-1", AttemptNo: 1, WorkerGeneration: 2, RunnerID: id,
				ExecutionDigest: "sha256:" + strings.Repeat("d", 64), HelperDigest: readyWant.HelperDigest}
			command := `printf x >> /tmp/sqlrs-private-child-count; printf '\000\377\200\012'; printf '\376\000e' >&2`
			out, diagnostic, childExit := []byte{0, 255, 128, 10}, []byte{254, 0, 'e'}, 0
			if variant == "empty" {
				command = `printf x >> /tmp/sqlrs-private-child-count`
				out, diagnostic = nil, nil
			}
			if variant == "failure" {
				command += "; exit 7"
				childExit = 7
			}
			if variant == "exact_limit" || variant == "over_limit" || variant == "stderr_over_limit" {
				command = `printf x >> /tmp/sqlrs-private-child-count; head -c 1024 /dev/zero`
				out, diagnostic = make([]byte, 1024), nil
				if variant != "exact_limit" {
					command = strings.Replace(command, "1024", "1025", 1)
				}
				if variant == "stderr_over_limit" {
					command += " >&2"
				}
			}
			user, generation, config := "0", "2", readyWant.RunnerConfigDigest
			switch variant {
			case "no_ready":
				if err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", id, "/bin/rm", "/sqlrs/capture/ready.complete").Run(); err != nil {
					t.Fatal("failed to remove owned fixture marker")
				}
			case "wrong_config":
				config = "sha256:" + strings.Repeat("e", 64)
			case "wrong_generation":
				generation = "3"
			case "untrusted_uid":
				user = "1001"
			}
			execArgs := []string{"exec", "-u", user, "-e", "SQLRS_CAPTURE_DIR=/sqlrs/capture/" + identity.InvocationRef,
				"-e", "SQLRS_JOB_ID=" + identity.JobID, "-e", "SQLRS_TASK_ID=" + identity.TaskID, "-e", "SQLRS_INVOCATION_REF=" + identity.InvocationRef,
				"-e", "SQLRS_ATTEMPT_NO=1", "-e", "SQLRS_WORKER_GENERATION=" + generation, "-e", "SQLRS_RUNNER_CONFIG_DIGEST=" + config,
				"-e", "SQLRS_RUNNER_ID=" + id, "-e", "SQLRS_EXECUTION_DIGEST=" + identity.ExecutionDigest, "-e", "SQLRS_CAPTURE_MAX_BYTES=1024",
				id, "/bin/sh", "/private-helper/step-capture.sh", "/bin/sh", "-c", command}
			wire, runErr := exec.CommandContext(ctx, "docker", execArgs...).CombinedOutput()
			success := variant == "binary" || variant == "empty" || variant == "failure" || variant == "exact_limit"
			if len(wire) != 0 {
				t.Fatal("private child output escaped into Docker diagnostics")
			}
			if variant == "over_limit" || variant == "stderr_over_limit" {
				exit, ok := runErr.(*exec.ExitError)
				if !ok || exit.ExitCode() != 90 {
					t.Fatal("oversize stream accepted")
				}
				check := `test "$(cat /tmp/sqlrs-private-child-count)" = x && test ! -e /sqlrs/capture/private-call-1/manifest.complete && test "$(stat -c %s /sqlrs/capture/private-call-1/stdout.partial)" -le 1024 && test "$(stat -c %s /sqlrs/capture/private-call-1/stderr.partial)" -le 1024`
				if err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", id, "/bin/sh", "-c", check).Run(); err != nil {
					t.Fatal("quota failure exceeded raw storage bound or fabricated EOF")
				}
				if proof, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1024); err == nil || proof.ManifestDigest != "" {
					t.Fatal("oversize stream returned completion evidence")
				}
				return
			}
			if !success {
				exit, ok := runErr.(*exec.ExitError)
				if !ok || exit.ExitCode() != 90 {
					t.Fatal("invalid invocation not rejected before child")
				}
				if err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", id, "/bin/sh", "-c", "test ! -e /tmp/sqlrs-private-child-count").Run(); err != nil {
					t.Fatal("rejected invocation started child")
				}
				return
			}
			if childExit == 0 && runErr != nil {
				t.Fatalf("valid private child failed: %T", runErr)
			}
			if childExit != 0 {
				exit, ok := runErr.(*exec.ExitError)
				if !ok || exit.ExitCode() != childExit {
					t.Fatal("natural child exit lost")
				}
			}
			capture, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1024)
			if err != nil || capture.ExitCode != childExit || !bytes.Equal(capture.Stdout, out) || !bytes.Equal(capture.Stderr, diagnostic) {
				t.Fatalf("real child capture differs: failed=%t", err != nil)
			}
			if err := exec.CommandContext(ctx, "docker", execArgs...).Run(); err == nil {
				t.Fatal("replayed invocation accepted")
			}
			count, err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", id, "/bin/cat", "/tmp/sqlrs-private-child-count").Output()
			if err != nil || string(count) != "x" {
				t.Fatal("child count is not exactly one")
			}
			again, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1024)
			if err != nil || again.ManifestDigest != capture.ManifestDigest || !bytes.Equal(again.Stdout, capture.Stdout) || !bytes.Equal(again.Stderr, capture.Stderr) {
				t.Fatal("replay changed saved capture")
			}
		})
	}
}
