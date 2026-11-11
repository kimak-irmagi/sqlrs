package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func combinedNativeContext(t *testing.T) context.Context {
	t.Helper()
	if os.Getenv("SQLRS_COST_NATIVE_PROBE") != "1" {
		t.Skip("combined native probe not selected")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || runtime.GOOS != "linux" || os.Geteuid() == 0 {
		t.Fatal("hosted ordinary Linux engine required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func combinedNativeImage(t *testing.T, ctx context.Context) string {
	t.Helper()
	value, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	image := strings.TrimSpace(string(value))
	if err != nil || !validCaptureDigest(image) {
		t.Fatal("required native image unavailable")
	}
	return image
}

func combinedNativeCleanup(t *testing.T, rt *DockerRuntime, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := rt.Stop(ctx, id); err != nil {
		t.Fatal("native combined cleanup failed before fallback")
	}
	if err := rt.VerifyContainerAbsent(ctx, id); err != nil {
		t.Fatal("native combined container leaked before fallback")
	}
}

// CPR08: real unchanged pinned helper, no stand-in. Tiny shell children qualify
// capture/proof behavior; the separate connected prepare test runs real JVM/SQL.
func TestCombinedCapturePruneNativeDocker(t *testing.T) {
	for _, variant := range []string{"control", "prior_recreated", "survivor", "child_7", "stopped"} {
		t.Run(variant, func(t *testing.T) {
			ctx := combinedNativeContext(t)
			helper, digest := LiquibaseStepCaptureHelper()
			file := filepath.Join(t.TempDir(), "step-capture.sh")
			if err := os.WriteFile(file, helper, 0444); err != nil {
				t.Fatal(err)
			}
			ready := SessionReadinessIdentity{SessionID: "combined-native", WorkerGeneration: 1, RunnerConfigDigest: "sha256:" + strings.Repeat("b", 64), HelperDigest: digest}
			_, err, name, id := runRetainedCaptureTestContainer(t, ctx, []string{"run", "-d", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
				"-v", dockerBindSpec(file, "/sqlrs/helper/step-capture.sh", true), "-e", "SQLRS_SESSION_ID=" + ready.SessionID, "-e", "SQLRS_WORKER_GENERATION=1",
				"-e", "SQLRS_RUNNER_CONFIG_DIGEST=" + ready.RunnerConfigDigest, "-e", "SQLRS_HELPER_DIGEST=" + digest, combinedNativeImage(t, ctx), "/sqlrs/helper/step-capture.sh", "serve"})
			if err != nil || !createdContainerID.MatchString(id) {
				t.Fatal("native combined parent failed to start")
			}
			rt := NewDocker(Options{})
			ready.RunnerID = id
			readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			for {
				if _, err := rt.ReadLiquibaseSessionReadiness(readyCtx, name, ready); err == nil {
					break
				}
				if readyCtx.Err() != nil {
					cancel()
					t.Fatal("native combined readiness unavailable")
				}
				time.Sleep(25 * time.Millisecond)
			}
			cancel()
			i := StepCaptureIdentity{JobID: "combined-job", TaskID: "step-1", SessionID: ready.SessionID, InvocationRef: "combined-first", AttemptNo: 1, WorkerGeneration: 1, RunnerID: id, ExecutionDigest: "sha256:" + strings.Repeat("d", 64), HelperDigest: digest}
			first, err := rt.InvokeTrackedLiquibaseStep(ctx, name, "", i, []string{"/bin/sh", "-c", "printf first"}, 1024)
			if err != nil || first.ExitCode != 0 || string(first.Stdout) != "first" {
				t.Fatal("native first capture failed")
			}
			ref := "combined-retirement"
			i.TaskID, i.InvocationRef = "step-2", "combined-second"
			i.PriorRetirementRef = &ref
			i.Prune = &CapturePruneAuthorization{RetirementRef: ref, SessionID: i.SessionID, WorkerGeneration: i.WorkerGeneration, RunnerID: id, PriorInvocationRef: "combined-first", PriorManifestDigest: first.ManifestDigest, NextInvocationRef: i.InvocationRef, HelperDigest: digest}
			child := "printf second; printf private-error >&2"
			wantExit := 0
			if variant == "child_7" {
				child += "; exit 7"
				wantExit = 7
			}
			second, err := rt.InvokeTrackedLiquibaseStep(ctx, name, "", i, []string{"/bin/sh", "-c", child}, 1024)
			if err != nil || second.ExitCode != wantExit || string(second.Stdout) != "second" || string(second.Stderr) != "private-error" || !second.MatchesPruneObservation(i, *i.Prune) {
				t.Fatal("native combined proof or child outcome differs")
			}
			if variant == "prior_recreated" {
				if _, err := rt.Exec(ctx, id, ExecRequest{User: "0", Args: []string{"mkdir", "/sqlrs/capture/combined-first"}}); err != nil {
					t.Fatal("cannot establish recreated-prior negative")
				}
			}
			if variant == "survivor" {
				_, err := rt.Exec(ctx, id, ExecRequest{User: "0", Args: []string{"/bin/sh", "-c", `setpriv --reuid=1001 --regid=1001 --clear-groups /bin/sh -c 'touch /tmp/combined-child-ready; exec sleep 3600' </dev/null >/dev/null 2>&1 &
while [ ! -f /tmp/combined-child-ready ]; do sleep 0.01; done`}})
				if err != nil {
					t.Fatal("cannot establish live-child negative")
				}
			}
			if variant == "stopped" {
				if err := exec.CommandContext(ctx, "docker", "stop", "--time", "1", id).Run(); err != nil {
					t.Fatal("cannot establish stopped-parent control")
				}
			}
			again, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", i, 1024)
			negative := variant == "prior_recreated" || variant == "survivor"
			if negative {
				if err == nil || again.ManifestDigest != "" {
					t.Fatal("valid manifest bypassed actual absence/idle proof")
				}
			} else {
				if err != nil || again.ExitCode != wantExit || again.ManifestDigest != second.ManifestDigest || string(again.Stdout) != "second" || again.MatchesPruneObservation(i, *i.Prune) != (variant != "stopped") {
					t.Fatal("native read/stopped outcome changed")
				}
			}
			combinedNativeCleanup(t, rt, id)
		})
	}
}

// CPR08: actual wrapper execution with a separately labelled probe stand-in.
// This qualifies internal ordering only, never the actual helper's protocol.
func TestCombinedCapturePruneWrapperNativeDocker(t *testing.T) {
	for _, variant := range []string{"control", "idle_before", "absence", "probe_exit", "extra_reply", "idle_after"} {
		t.Run(variant, func(t *testing.T) {
			ctx := combinedNativeContext(t)
			standin := `#!/bin/sh
set -eu
[ "$#" = 1 ] && [ "$1" = probe-prune ] || exit 91
touch /tmp/combined-probe-reached
case "$SQLRS_TEST_PROBE" in
  absence) printf 'wrong-proof\n'; exit 0 ;;
  probe_exit) printf 'sqlrs-capture-prune-absent-v1\n'; exit 7 ;;
  extra_reply) printf 'sqlrs-capture-prune-absent-v1\n\n'; exit 0 ;;
  idle_after)
    setpriv --reuid=1001 --regid=1001 --clear-groups /bin/sh -c 'touch /tmp/combined-child-ready; exec sleep 3600' </dev/null >/dev/null 2>&1 &
    while [ ! -f /tmp/combined-child-ready ]; do sleep 0.01; done ;;
esac
printf 'sqlrs-capture-prune-absent-v1\n'
`
			file := filepath.Join(t.TempDir(), "test-probe.sh")
			if err := os.WriteFile(file, []byte(standin), 0444); err != nil {
				t.Fatal(err)
			}
			_, err, _, id := runRetainedCaptureTestContainer(t, ctx, []string{"run", "-d", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh", "-v", dockerBindSpec(file, "/sqlrs/helper/step-capture.sh", true), combinedNativeImage(t, ctx), "-c", "while :; do sleep 3600; done"})
			if err != nil || !createdContainerID.MatchString(id) {
				t.Fatal("wrapper-only parent unavailable")
			}
			rt := NewDocker(Options{})
			if variant == "idle_before" {
				if _, err := rt.Exec(ctx, id, ExecRequest{User: "0", Args: []string{"/bin/sh", "-c", `setpriv --reuid=1001 --regid=1001 --clear-groups /bin/sh -c 'touch /tmp/combined-child-ready; exec sleep 3600' </dev/null >/dev/null 2>&1 &
while [ ! -f /tmp/combined-child-ready ]; do sleep 0.01; done`}}); err != nil {
					t.Fatal("first idle negative unavailable")
				}
			}
			output, err := rt.runner.(boundedBinaryRunner).RunBounded(ctx, "docker", []string{"exec", "-u", "0", "-e", "SQLRS_CAPTURE_COST=0", "-e", "SQLRS_TEST_PROBE=" + variant, id, "/bin/sh", "-c", combinedStepPruneProbe}, 64, 4096)
			if variant == "control" {
				if err != nil || string(output) != "sqlrs-step-idle-pruned-v1\n" {
					t.Fatal("wrapper positive did not reach complete proof")
				}
			} else if err == nil || len(output) != 0 {
				t.Fatal("wrapper accepted wrong inner-stage evidence")
			}
			probe, err := rt.Exec(ctx, id, ExecRequest{User: "0", Args: []string{"/bin/sh", "-c", `if [ -f /tmp/combined-probe-reached ]; then printf reached; else printf absent; fi`}})
			want := "reached"
			if variant == "idle_before" {
				want = "absent"
			}
			if err != nil || probe != want {
				t.Fatal("wrapper failed before its selected stage")
			}
			combinedNativeCleanup(t, rt, id)
		})
	}
}
