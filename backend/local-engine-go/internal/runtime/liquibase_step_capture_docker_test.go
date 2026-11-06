package runtime

import (
	"bytes"
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
			name := fmt.Sprintf("sqlrs-lb-step-capture-%d", time.Now().UnixNano())
			rt := NewDocker(Options{Binary: "docker"})
			ready := SessionReadinessIdentity{SessionID: "session", WorkerGeneration: 1,
				RunnerConfigDigest: "sha256:" + strings.Repeat("c", 64), HelperDigest: digest}
			id, err := rt.CreateContainer(ctx, RunRequest{ImageID: image, Name: name,
				EntryPoint: "/bin/sh", User: "0", Network: "none",
				Args: []string{"/sqlrs/helper/step-capture.sh", "serve"},
				Env: map[string]string{"SQLRS_SESSION_ID": ready.SessionID, "SQLRS_WORKER_GENERATION": "1",
					"SQLRS_RUNNER_CONFIG_DIGEST": ready.RunnerConfigDigest, "SQLRS_HELPER_DIGEST": digest},
				Mounts: []Mount{{HostPath: helperPath, ContainerPath: "/sqlrs/helper/step-capture.sh", ReadOnly: true}}})
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
				if err := rt.VerifyContainerAbsent(cleanupCtx, id); err != nil {
					t.Errorf("exact test runner remains: %v", err)
				}
			})
			if err := rt.StartContainer(ctx, id); err != nil {
				t.Fatal(err)
			}
			// OT-MP02: the real private/v2 initializer, not test-written files,
			// establishes the parent's exact readiness before any child command.
			ready.RunnerID = id
			readyCtx, readyCancel := context.WithTimeout(ctx, 15*time.Second)
			defer readyCancel()
			for {
				if _, err := rt.ReadLiquibaseSessionReadiness(readyCtx, name, ready); err == nil {
					break
				}
				if readyCtx.Err() != nil {
					t.Fatal("private session readiness unavailable")
				}
				time.Sleep(25 * time.Millisecond)
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
					"SQLRS_CAPTURE_MAX_BYTES": "1024", "SQLRS_CAPTURE_PRUNE_AUTHORIZATION": ""}
				output, err := rt.Exec(ctx, id, ExecRequest{User: "0", Env: env,
					Args: []string{"/bin/sh", "/sqlrs/helper/step-capture.sh", "/bin/sh", "-c",
						`sh -c 'while :; do sleep 1; done' >/dev/null 2>&1 & printf 'protected-sql:%s' "$!"`}})
				if err != nil || strings.Contains(output, "protected-sql") {
					t.Fatalf("capture helper output=%q err=%v", output, err)
				}
				// Prove complete output and the exact live untrusted PID first:
				// an absent marker or failed initialization is not this control.
				complete, err := readCompleteLiquibaseStepCapture(containerCaptureFiles{
					ctx: ctx, runtime: rt, id: id, invocation: identity.InvocationRef}, identity, 1024)
				if err != nil || complete.ExitCode != 0 || !strings.HasPrefix(string(complete.Stdout), "protected-sql:") {
					t.Fatal("surviving-child fixture lacks complete private output")
				}
				pid, err := strconv.Atoi(strings.TrimPrefix(string(complete.Stdout), "protected-sql:"))
				if err != nil || pid <= 0 {
					t.Fatal("surviving-child fixture lacks an exact PID")
				}
				const liveChild = `set -eu; status="/proc/$1/status"; test -r "$status"; grep -Eq '^Uid:[[:space:]]+1001([[:space:]]|$)' "$status"; ! grep -Eq '^State:[[:space:]]+Z' "$status"`
				if _, err := rt.Exec(ctx, id, ExecRequest{User: "0", Args: []string{
					"/bin/sh", "-c", liveChild, "sqlrs-test-live-child", strconv.Itoa(pid)}}); err != nil {
					t.Fatal("exact untrusted child is not alive")
				}
				if _, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1024); err == nil ||
					err.Error() != "Liquibase session child-process state is not proved idle" {
					t.Fatalf("surviving child did not reach idle refusal: %v", err)
				}
				return
			}
			count := 1
			if variant == "three JVMs one runner" {
				count = 3
			}
			var previous StepCaptureIdentity
			var previousCapture CompleteStepCapture
			for step := 1; step <= count; step++ {
				identity.TaskID = "execute-" + strconv.Itoa(step)
				identity.InvocationRef = "invocation-" + strconv.Itoa(step)
				if step > 1 {
					// Explicit lower-level publication setup only: durable SQLite
					// admission is qualified separately by the queue/manager tests.
					ref := "retirement-" + strconv.Itoa(step-1)
					identity.PriorRetirementRef = &ref
					identity.Prune = &CapturePruneAuthorization{RetirementRef: ref,
						SessionID: identity.SessionID, WorkerGeneration: identity.WorkerGeneration,
						RunnerID: id, PriorInvocationRef: previous.InvocationRef,
						PriorManifestDigest: previousCapture.ManifestDigest,
						NextInvocationRef:   identity.InvocationRef, HelperDigest: digest}
				}
				capture, err := rt.InvokeTrackedLiquibaseStep(ctx, name, "", identity, []string{"--version"}, 1<<20)
				if err != nil || capture.ExitCode != 0 || !strings.Contains(string(capture.Stdout)+string(capture.Stderr), "5.0.4") {
					t.Fatalf("step %d: exit=%d err=%v", step, capture.ExitCode, err)
				}
				view, found, err := rt.InspectNamedContainer(ctx, name)
				if err != nil || !found || view.ID != id || view.Status != "running" {
					t.Fatalf("runner changed after step %d: view=%+v found=%t err=%v", step, view, found, err)
				}
				if step > 1 {
					if err := rt.VerifyLiquibaseCapturePruned(ctx, name, identity); err != nil {
						t.Fatal("exact previous private output was not pruned")
					}
					again, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1<<20)
					if err != nil || again.ManifestDigest != capture.ManifestDigest ||
						!bytes.Equal(again.Stdout, capture.Stdout) || !bytes.Equal(again.Stderr, capture.Stderr) {
						t.Fatal("read-only prune proof changed current private output")
					}
				}
				if variant == "incomplete manifest" {
					if _, err := rt.Exec(ctx, id, ExecRequest{User: "0", Args: []string{
						"/bin/rm", "/sqlrs/capture/" + identity.InvocationRef + "/manifest.complete"}}); err != nil {
						t.Fatal(err)
					}
					if _, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1<<20); err == nil ||
						err.Error() != "complete step manifest unavailable" {
						t.Fatalf("partial capture did not reach manifest refusal: %v", err)
					}
				}
				previous, previousCapture = identity, capture
			}
		})
	}
}
