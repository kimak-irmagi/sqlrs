package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// IR08 deliberately invokes the actual pinned helper with a tiny shell child;
// real stock-CLI/SQL prepare is independently checked by IR09 in prepare.
func TestStepResultEnvelopeNativeDocker(t *testing.T) {
	for _, variant := range []string{"binary-child7", "surviving-child"} {
		t.Run(variant, func(t *testing.T) {
			ctx := combinedNativeContext(t)
			helper, digest := LiquibaseStepCaptureHelper()
			file := filepath.Join(t.TempDir(), "step-result.sh")
			if err := os.WriteFile(file, helper, 0444); err != nil {
				t.Fatal(err)
			}
			ready := SessionReadinessIdentity{SessionID: "inline-native", WorkerGeneration: 1, RunnerConfigDigest: "sha256:" + strings.Repeat("b", 64), HelperDigest: digest}
			_, err, name, id := runRetainedCaptureTestContainer(t, ctx, []string{"run", "-d", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
				"-v", dockerBindSpec(file, "/sqlrs/helper/step-capture.sh", true), "-e", "SQLRS_SESSION_ID=" + ready.SessionID, "-e", "SQLRS_WORKER_GENERATION=1",
				"-e", "SQLRS_RUNNER_CONFIG_DIGEST=" + ready.RunnerConfigDigest, "-e", "SQLRS_HELPER_DIGEST=" + digest, combinedNativeImage(t, ctx), "/sqlrs/helper/step-capture.sh", "serve"})
			if err != nil || !createdContainerID.MatchString(id) {
				t.Fatal("inline parent not created")
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
					t.Fatal("inline parent not ready")
				}
				time.Sleep(25 * time.Millisecond)
			}
			cancel()
			identity := StepCaptureIdentity{JobID: "inline-job", TaskID: "execute-1", SessionID: ready.SessionID, InvocationRef: "inline-first", AttemptNo: 1, WorkerGeneration: 1, RunnerID: id, ExecutionDigest: "sha256:" + strings.Repeat("d", 64), HelperDigest: digest}
			args := stepCaptureExecEnvironment(identity, 1024, "")
			args = append(args, "-e", "SQLRS_CAPTURE_COST=0", "-e", "SQLRS_CAPTURE_RESULT=same-exec-v1", id, "/bin/sh", "/sqlrs/helper/step-capture.sh", "/bin/sh", "-c")
			child := `printf '\000\377\200\012'; printf 'native-stderr' >&2; exit 7`
			if variant == "surviving-child" {
				child = `sh -c 'while :; do sleep 1; done' </dev/null >/dev/null 2>&1 & printf 'surviving-marker'; exit 0`
			}
			args = append(args, child)
			wire, runErr := rt.runner.(boundedBinaryRunner).RunBounded(ctx, rt.binary, args, 2*1024+8192+4096+256, 4096)
			if variant == "surviving-child" {
				if runErr == nil || len(wire) != 0 {
					t.Fatal("live child completed a transport")
				}
				files := containerCaptureFiles{ctx: ctx, runtime: rt, id: id, invocation: identity.InvocationRef}
				capture, err := readCompleteLiquibaseStepCapture(files, identity, 1024)
				if err != nil || string(capture.Stdout) != "surviving-marker" {
					t.Fatal("live-child case failed before durable capture", err)
				}
				if _, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1024); err == nil {
					t.Fatal("independent reader ignored survivor")
				}
			} else {
				if runErr != nil {
					t.Fatal("child7 became transport failure", runErr)
				}
				frame, err := decodeStepResultEnvelope(wire, 1024)
				if err != nil || frame.pruned || len(frame.detail) != 0 {
					t.Fatal("native binary frame invalid", err)
				}
				capture, err := readCompleteLiquibaseStepCapture(frame.files, identity, 1024)
				if err != nil || capture.ExitCode != 7 || !bytes.Equal(capture.Stdout, []byte{0, 255, 128, '\n'}) || string(capture.Stderr) != "native-stderr" {
					t.Fatal("native result bytes/code changed", err)
				}
				// Treat the acquired reply as lost: no CLI rerun, just a separately
				// requested actual read of the durable same-invocation files.
				again, err := rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", identity, 1024)
				if err != nil || again.ManifestDigest != capture.ManifestDigest || !bytes.Equal(again.Stdout, capture.Stdout) || !bytes.Equal(again.Stderr, capture.Stderr) {
					t.Fatal("durable result unavailable after losing transport reply", err)
				}
			}
			combinedNativeCleanup(t, rt, id)
		})
	}
}
