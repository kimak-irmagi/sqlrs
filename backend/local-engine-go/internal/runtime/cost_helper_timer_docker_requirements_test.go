package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// DT02/DT08: execute the pinned helpers, with their real builtin clock, only in
// Actions. A tiny child tests capture/exit equality; the full prepare test proves
// actual Liquibase/JVM counts separately. No benchmark numbers come from here.
func TestCostHelperTimersDocker(t *testing.T) {
	if os.Getenv("SQLRS_COST_NATIVE_PROBE") != "1" {
		t.Skip("explicit Actions-only timer qualification")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("native timer qualification requires Actions")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	image := strings.TrimSpace(string(imageBytes))
	if err != nil || !validCaptureDigest(image) {
		t.Fatal("qualified image required")
	}
	for _, kind := range []string{"planning", "session"} {
		for _, flag := range []string{"0", "1"} {
			for _, childExit := range []int{0, 7} {
				t.Run(fmt.Sprintf("%s/%s/exit-%d", kind, flag, childExit), func(t *testing.T) {
					helper, digest := LiquibaseCaptureHelper()
					if kind == "session" {
						helper, digest = LiquibaseStepCaptureHelper()
					}
					helperPath := filepath.Join(t.TempDir(), "capture.sh")
					if err := os.WriteFile(helperPath, helper, 0444); err != nil {
						t.Fatal(err)
					}
					env := map[string]string{"SQLRS_CAPTURE_COST": flag, "SQLRS_CAPTURE_DIR": "/sqlrs/capture", "SQLRS_JOB_ID": "timer-job",
						"SQLRS_TASK_ID": "plan", "SQLRS_ATTEMPT_NO": "1", "SQLRS_CAPTURE_MAX_BYTES": "1024",
						"SQLRS_EXECUTION_DIGEST": "sha256:" + strings.Repeat("a", 64), "SQLRS_HELPER_DIGEST": digest,
						"SQLRS_SESSION_ID": "timer-session", "SQLRS_WORKER_GENERATION": "1",
						"SQLRS_RUNNER_CONFIG_DIGEST": "sha256:" + strings.Repeat("b", 64), "SQLRS_INVOCATION_REF": "timer-call"}
					child := []string{"/sqlrs/helper/capture.sh", "/bin/sh", "-c", "printf 'one-cli-start\\n'; printf 'private-stderr\\n' >&2; exit " + strconv.Itoa(childExit)}
					args := child
					if kind == "session" {
						args = []string{"/sqlrs/helper/capture.sh", "serve"}
						env["SQLRS_CAPTURE_DIR"] += "/timer-call"
					}
					rt := NewDocker(Options{Binary: "docker"})
					name := fmt.Sprintf("sqlrs-step-cost-timer-%d", time.Now().UnixNano())
					id, err := rt.CreateContainer(ctx, RunRequest{ImageID: image, Name: name, EntryPoint: "/bin/sh", User: "0", Network: "none", Env: env, Args: args,
						Mounts: []Mount{{HostPath: helperPath, ContainerPath: "/sqlrs/helper/capture.sh", ReadOnly: true}}})
					if err != nil {
						t.Fatal("timer fixture create failed")
					}
					t.Cleanup(func() {
						cleanup, finish := context.WithTimeout(context.Background(), 20*time.Second)
						defer finish()
						if err := rt.Stop(cleanup, id); err != nil {
							t.Error("timer fixture cleanup failed")
						}
						if err := rt.VerifyContainerAbsent(cleanup, id); err != nil {
							t.Error("timer fixture retained container")
						}
					})
					started := time.Now()
					if err := rt.StartContainer(ctx, id); err != nil {
						t.Fatal("timer fixture start failed")
					}
					output, exit := "", 0
					var capture CompleteCapture
					if kind == "session" {
						ready := SessionReadinessIdentity{SessionID: "timer-session", WorkerGeneration: 1, RunnerID: id, RunnerConfigDigest: env["SQLRS_RUNNER_CONFIG_DIGEST"], HelperDigest: digest}
						readyCtx, stop := context.WithTimeout(ctx, 10*time.Second)
						defer stop()
						for {
							if _, err := rt.ReadLiquibaseSessionReadiness(readyCtx, name, ready); err == nil {
								break
							}
							if readyCtx.Err() != nil {
								t.Fatal("timer parent readiness missing")
							}
							time.Sleep(20 * time.Millisecond)
						}
						env["SQLRS_RUNNER_ID"] = id
						started = time.Now()
						output, err = rt.Exec(ctx, id, ExecRequest{User: "0", Env: env, Args: append([]string{"/bin/sh"}, child...)})
						if err != nil {
							var process *exec.ExitError
							if !errors.As(err, &process) {
								t.Fatal("timer child exit unavailable")
							}
							exit = process.ExitCode()
						}
					} else {
						output, exit, err = rt.WaitContainer(ctx, id)
						if err != nil {
							t.Fatal("timer planner exit unavailable")
						}
					}
					elapsed := time.Since(started).Nanoseconds()
					if exit != childExit {
						t.Fatal("timer changed original child exit")
					}
					if flag == "0" {
						if output != "" {
							t.Fatal("disabled helper wrote diagnostics")
						}
					} else if detail, err := preparecost.ParseHelperDetail([]byte(output), elapsed); err != nil || len(detail.Regions) != 8 {
						t.Fatal("actual uptime/timer phases unavailable")
					}
					if kind == "session" {
						identity := StepCaptureIdentity{JobID: "timer-job", TaskID: "plan", SessionID: "timer-session", InvocationRef: "timer-call", AttemptNo: 1, WorkerGeneration: 1, RunnerID: id, ExecutionDigest: env["SQLRS_EXECUTION_DIGEST"], HelperDigest: digest}
						result, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1024)
						if err != nil || result.ExitCode != childExit {
							t.Fatal("timer session capture changed")
						}
						capture = result.CompleteCapture
					} else {
						capture, err = rt.ReadCompleteLiquibaseOutput(ctx, name, id, "", CaptureIdentity{JobID: "timer-job", AttemptNo: 1, ExecutionDigest: env["SQLRS_EXECUTION_DIGEST"], HelperDigest: digest, ExitCode: childExit}, 1024)
						if err != nil {
							t.Fatal("timer planner capture changed")
						}
					}
					if string(capture.Stdout) != "one-cli-start\n" || string(capture.Stderr) != "private-stderr\n" || capture.StdoutDigest != captureDigest(capture.Stdout) || capture.StderrDigest != captureDigest(capture.Stderr) {
						t.Fatal("timer changed CLI count or protected bytes")
					}
					if err := rt.Stop(ctx, id); err != nil {
						t.Fatal("timer fixture removal failed before fallback")
					}
					if err := rt.VerifyContainerAbsent(ctx, id); err != nil {
						t.Fatal("timer fixture leaked before fallback")
					}
				})
			}
		}
	}
}
