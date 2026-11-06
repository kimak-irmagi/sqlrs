package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// N02/N03/C02: real helper/JVM invocations share one live runner. Independent
// private manifests prove each child; incomplete capture and surviving
// untrusted processes reject completion. Every exact test-owned container is
// removed and its absence is checked even when a subtest fails.
func TestLiquibaseStepCaptureReusableRunnerDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("set SQLRS_LIQUIBASE_DOCKER_E2E=1 for Docker integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	if err != nil {
		t.Fatal(err)
	}
	image := strings.TrimSpace(string(imageBytes))
	if !validCaptureDigest(image) {
		t.Fatalf("invalid pinned image ID %q", image)
	}
	for _, variant := range []string{"three JVMs one runner", "incomplete manifest", "surviving child"} {
		t.Run(variant, func(t *testing.T) {
			helper, digest := LiquibaseStepCaptureHelper()
			if len(helper) == 0 || digest != captureDigest(helper) {
				t.Fatal("step helper digest mismatch")
			}
			helperPath := filepath.Join(t.TempDir(), "step-capture.sh")
			if err := os.WriteFile(helperPath, helper, 0444); err != nil {
				t.Fatal(err)
			}
			captureRoot := t.TempDir()
			name := fmt.Sprintf("sqlrs-lb-step-capture-%d", time.Now().UnixNano())
			rt := NewDocker(Options{Binary: "docker"})
			id, err := rt.CreateContainer(ctx, RunRequest{ImageID: image, Name: name,
				EntryPoint: "/bin/sh", User: "0", Network: "none",
				Args: []string{"-c", "umask 077; chmod 700 /sqlrs/capture; while :; do sleep 3600; done"},
				Mounts: []Mount{{HostPath: helperPath, ContainerPath: "/sqlrs/helper/step-capture.sh", ReadOnly: true},
					{HostPath: captureRoot, ContainerPath: "/sqlrs/capture"}}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cleanupCancel()
				if err := rt.Stop(cleanupCtx, id); err != nil {
					t.Errorf("remove exact test runner: %v", err)
				}
				if _, found, err := rt.InspectNamedContainer(cleanupCtx, name); err != nil || found {
					t.Errorf("test runner remains: found=%t err=%v", found, err)
				}
			})
			if err := rt.StartContainer(ctx, id); err != nil {
				t.Fatal(err)
			}
			identity := StepCaptureIdentity{JobID: "job", TaskID: "execute-1", SessionID: "session",
				InvocationRef: "invocation-1", AttemptNo: 1, WorkerGeneration: 1, RunnerID: id,
				ExecutionDigest: "sha256:" + strings.Repeat("a", 64), HelperDigest: digest}
			if variant == "surviving child" {
				env := map[string]string{"SQLRS_CAPTURE_DIR": "/sqlrs/capture/" + identity.InvocationRef,
					"SQLRS_JOB_ID": identity.JobID, "SQLRS_TASK_ID": identity.TaskID,
					"SQLRS_SESSION_ID": identity.SessionID, "SQLRS_INVOCATION_REF": identity.InvocationRef,
					"SQLRS_ATTEMPT_NO": "1", "SQLRS_WORKER_GENERATION": "1", "SQLRS_RUNNER_ID": id,
					"SQLRS_EXECUTION_DIGEST": identity.ExecutionDigest, "SQLRS_HELPER_DIGEST": digest,
					"SQLRS_CAPTURE_MAX_BYTES": "1024"}
				output, err := rt.Exec(ctx, id, ExecRequest{User: "0", Env: env,
					Args: []string{"/bin/sh", "/sqlrs/helper/step-capture.sh", "/bin/sh", "-c", "sleep 30 >/dev/null 2>&1 & printf protected-sql"}})
				if err != nil || strings.Contains(output, "protected-sql") {
					t.Fatalf("capture helper output=%q err=%v", output, err)
				}
				if _, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id,
					filepath.Join(captureRoot, identity.InvocationRef), identity, 1024); err == nil {
					t.Fatal("surviving child was classified as idle")
				}
				return
			}
			count := 1
			if variant == "three JVMs one runner" {
				count = 3
			}
			for step := 1; step <= count; step++ {
				identity.TaskID = "execute-" + strconv.Itoa(step)
				identity.InvocationRef = "invocation-" + strconv.Itoa(step)
				dir := filepath.Join(captureRoot, identity.InvocationRef)
				capture, err := rt.InvokeTrackedLiquibaseStep(ctx, name, dir, identity, []string{"--version"}, 1<<20)
				if err != nil || capture.ExitCode != 0 || !strings.Contains(string(capture.Stdout)+string(capture.Stderr), "5.0.4") {
					t.Fatalf("step %d: exit=%d err=%v", step, capture.ExitCode, err)
				}
				view, found, err := rt.InspectNamedContainer(ctx, name)
				if err != nil || !found || view.ID != id || view.Status != "running" {
					t.Fatalf("runner changed after step %d: view=%+v found=%t err=%v", step, view, found, err)
				}
				if variant == "incomplete manifest" {
					if err := os.Remove(filepath.Join(dir, "manifest.complete")); err != nil {
						t.Fatal(err)
					}
					if _, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, dir, identity, 1<<20); err == nil {
						t.Fatal("partial capture was accepted")
					}
				}
			}
		})
	}
}
