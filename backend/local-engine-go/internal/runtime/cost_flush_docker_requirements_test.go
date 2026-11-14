package runtime

import (
	"bytes"
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
)

// The shim is exclusively a native functional-test observer. It validates the
// real files at every boundary and delegates to the actual qualified GNU sync;
// it is never present in benchmark samples or product helper bytes.
const costFlushObserver = `#!/bin/sh
set -eu
n=0
[ ! -f /sqlrs/flush-probe/count ] || read -r n < /sqlrs/flush-probe/count
n=$((n+1))
[ "$#" -eq 2 ] && [ "$1" = -f ] || exit 71
case "$n" in
  1) [ "$2" = "$SQLRS_CAPTURE_DIR" ]; test -f "$SQLRS_CAPTURE_DIR/stdout.partial"; test -f "$SQLRS_CAPTURE_DIR/stderr.partial"; test ! -e "$SQLRS_CAPTURE_DIR/stdout.complete" ;;
  2) [ "$2" = "$SQLRS_CAPTURE_DIR" ]; test -f "$SQLRS_CAPTURE_DIR/stdout.complete"; test -f "$SQLRS_CAPTURE_DIR/stderr.complete"; test ! -e "$SQLRS_CAPTURE_DIR/manifest.partial" ;;
  3) [ "$2" = "$SQLRS_CAPTURE_DIR/manifest.partial" ]; test -f "$SQLRS_CAPTURE_DIR/manifest.partial"; test ! -e "$SQLRS_CAPTURE_DIR/manifest.complete" ;;
  4) [ "$2" = "$SQLRS_CAPTURE_DIR" ]; test -f "$SQLRS_CAPTURE_DIR/manifest.complete"; test ! -e "$SQLRS_CAPTURE_DIR/manifest.partial" ;;
  *) exit 72 ;;
esac
printf '%s\n' "$n" > /sqlrs/flush-probe/count
printf '%s\n' "$n" >> /sqlrs/flush-probe/calls
[ "$n" != "$SQLRS_TEST_FAIL_FLUSH" ] || exit 77
exec /usr/bin/sync "$@"
`

// CT05: both embedded helpers execute all four native boundaries, or preserve
// a nonzero helper exit at the independently observed injected phase. In
// particular phase 4 may already have published the complete manifest.
func TestCostHelperFlushBoundariesDocker(t *testing.T) {
	if os.Getenv("SQLRS_COST_NATIVE_PROBE") != "1" {
		t.Skip("explicit Actions-only helper flush test")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("native helper flush tests must run in Actions")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	image := strings.TrimSpace(string(imageBytes))
	if err != nil || !validCaptureDigest(image) {
		t.Fatal("flush tests require the resolved qualified Liquibase image")
	}
	for _, kind := range []string{"planning", "session"} {
		for failure := 0; failure <= 4; failure++ {
			t.Run(fmt.Sprintf("%s/flush-%d", kind, failure), func(t *testing.T) {
				helper, digest := LiquibaseCaptureHelper()
				if kind == "session" {
					helper, digest = LiquibaseStepCaptureHelper()
				}
				root := t.TempDir()
				helperPath, shimPath := filepath.Join(root, "capture.sh"), filepath.Join(root, "sync")
				if err := os.WriteFile(helperPath, helper, 0444); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(shimPath, []byte(costFlushObserver), 0755); err != nil {
					t.Fatal(err)
				}
				capture := "/sqlrs/capture"
				if kind == "session" {
					capture += "/invocation-1"
				}
				env := map[string]string{"SQLRS_CAPTURE_DIR": capture, "SQLRS_JOB_ID": "flush-job", "SQLRS_TASK_ID": "plan",
					"SQLRS_ATTEMPT_NO": "1", "SQLRS_CAPTURE_MAX_BYTES": "1024", "SQLRS_EXECUTION_DIGEST": "sha256:" + strings.Repeat("a", 64),
					"SQLRS_HELPER_DIGEST": digest, "SQLRS_SESSION_ID": "flush-session", "SQLRS_WORKER_GENERATION": "1",
					"SQLRS_RUNNER_CONFIG_DIGEST": "sha256:" + strings.Repeat("b", 64), "SQLRS_TEST_FAIL_FLUSH": strconv.Itoa(failure),
					"SQLRS_INVOCATION_REF": "invocation-1", "SQLRS_CAPTURE_PRUNE_AUTHORIZATION": ""}
				const invoke = `mkdir -m 700 /sqlrs/flush-probe; export PATH="/sqlrs/test-bin:$PATH"; exec /bin/sh /sqlrs/helper/capture.sh /bin/sh -c 'printf stream-one; printf stream-two >&2'`
				args := []string{"-c", invoke}
				if kind == "session" {
					args = []string{"/sqlrs/helper/capture.sh", "serve"}
				}
				rt := NewDocker(Options{Binary: "docker"})
				name := fmt.Sprintf("sqlrs-step-cost-flush-%d", time.Now().UnixNano())
				id, err := rt.CreateContainer(ctx, RunRequest{ImageID: image, Name: name, EntryPoint: "/bin/sh", User: "0", Network: "none", Env: env, Args: args,
					Mounts: []Mount{{HostPath: helperPath, ContainerPath: "/sqlrs/helper/capture.sh", ReadOnly: true}, {HostPath: shimPath, ContainerPath: "/sqlrs/test-bin/sync", ReadOnly: true}}})
				if err != nil {
					t.Fatal("flush fixture create failed")
				}
				t.Cleanup(func() {
					cleanup, finish := context.WithTimeout(context.Background(), 20*time.Second)
					defer finish()
					if err := rt.Stop(cleanup, id); err != nil {
						t.Error("exact flush fixture removal failed")
					}
					if err := rt.VerifyContainerAbsent(cleanup, id); err != nil {
						t.Error("exact flush fixture remains")
					}
				})
				if err := rt.StartContainer(ctx, id); err != nil {
					t.Fatal("flush fixture start failed")
				}
				exit := 0
				if kind == "session" {
					ready := SessionReadinessIdentity{SessionID: env["SQLRS_SESSION_ID"], WorkerGeneration: 1, RunnerID: id, RunnerConfigDigest: env["SQLRS_RUNNER_CONFIG_DIGEST"], HelperDigest: digest}
					readyCtx, stopReady := context.WithTimeout(ctx, 10*time.Second)
					defer stopReady()
					for {
						if _, err := rt.ReadLiquibaseSessionReadiness(readyCtx, name, ready); err == nil {
							break
						}
						if readyCtx.Err() != nil {
							t.Fatal("real session readiness missing")
						}
						time.Sleep(20 * time.Millisecond)
					}
					env["SQLRS_RUNNER_ID"] = id
					_, execErr := rt.Exec(ctx, id, ExecRequest{User: "0", Env: env, Args: []string{"/bin/sh", "-c", invoke}})
					if execErr != nil {
						var process *exec.ExitError
						if !errors.As(execErr, &process) {
							t.Fatal("flush failure lacks a native helper exit")
						}
						exit = process.ExitCode()
					}
				} else {
					_, exit, err = rt.WaitContainer(ctx, id)
					if err != nil {
						t.Fatal("planning flush exit unavailable")
					}
				}
				wantExit, phases := 0, 4
				if failure > 0 {
					wantExit, phases = 90, failure
				}
				if exit != wantExit {
					t.Fatal("helper exit differs at the intended flush boundary", exit, wantExit)
				}
				// Docker cp also reads stopped planning containers. No restart is
				// permitted merely to observe the shim's root-private numeric log.
				probeCtx, stopProbe := context.WithTimeout(ctx, 10*time.Second)
				defer stopProbe()
				wire, copyErr := (execRunner{}).RunBounded(probeCtx, "docker", []string{"cp", id + ":/sqlrs/flush-probe/calls", "-"}, 40000, 4096)
				calls, parseErr := decodeCaptureArchive(wire, "calls", 64)
				var expected strings.Builder
				for phase := 1; phase <= phases; phase++ {
					fmt.Fprintf(&expected, "%d\n", phase)
				}
				if copyErr != nil || parseErr != nil || string(calls) != expected.String() {
					t.Fatal("native shim did not reach the exact flush sequence", copyErr != nil, parseErr != nil)
				}
				if failure == 0 {
					var result CompleteCapture
					if kind == "session" {
						identity := StepCaptureIdentity{JobID: "flush-job", TaskID: "plan", SessionID: "flush-session", InvocationRef: "invocation-1", AttemptNo: 1, WorkerGeneration: 1, RunnerID: id, ExecutionDigest: env["SQLRS_EXECUTION_DIGEST"], HelperDigest: digest}
						step, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1024)
						if err != nil || step.ExitCode != 0 {
							t.Fatal("four-flush session lost complete capture", err)
						}
						result = step.CompleteCapture
					} else {
						result, err = rt.ReadCompleteLiquibaseOutput(ctx, name, id, "", CaptureIdentity{JobID: "flush-job", AttemptNo: 1, ExecutionDigest: env["SQLRS_EXECUTION_DIGEST"], HelperDigest: digest, ExitCode: 0}, 1024)
						if err != nil {
							t.Fatal("four-flush planner lost complete capture", err)
						}
					}
					if !bytes.Equal(result.Stdout, []byte("stream-one")) || !bytes.Equal(result.Stderr, []byte("stream-two")) || result.StdoutDigest != captureDigest(result.Stdout) || result.StderrDigest != captureDigest(result.Stderr) {
						t.Fatal("flush deduplication changed closed stream bytes/hashes")
					}
				}
			})
		}
	}
}
