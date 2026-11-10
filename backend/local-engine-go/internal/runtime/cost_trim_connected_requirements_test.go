package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type costTrimAcquisitionRunner struct {
	captureFilesRequirementsRunner
	events []string
}

func (r *costTrimAcquisitionRunner) Run(ctx context.Context, binary string, args []string, input *string) (string, error) {
	r.events = append(r.events, strings.Join(args, " "))
	return r.captureFilesRequirementsRunner.Run(ctx, binary, args, input)
}

func (r *costTrimAcquisitionRunner) RunBounded(ctx context.Context, binary string, args []string, out, diagnostic int64) ([]byte, error) {
	r.events = append(r.events, strings.Join(args, " "))
	return r.captureFilesRequirementsRunner.RunBounded(ctx, binary, args, out, diagnostic)
}

// CT02/CT04: exercise the connected consumer, not only the inspect/archive
// helpers. Exactly one copy remains bracketed by the original identity proofs;
// the live branch retains its idle proof, and every refusal reaches its target.
func TestCostTrimConnectedSessionAcquisition(t *testing.T) {
	for _, variant := range []string{"running", "exited", "foreign-after", "renamed-after", "changed-phase", "idle-refused", "missing-file", "cancel-during-copy"} {
		t.Run(variant, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			stdout, stderr := []byte{0, 255, 128, '\n'}, []byte("private-stderr-marker")
			manifest["stdout_bytes"], manifest["stdout_sha256"] = len(stdout), captureHex(stdout)
			manifest["stderr_bytes"], manifest["stderr_sha256"] = len(stderr), captureHex(stderr)
			writeStepCaptureFixture(t, dir, manifest, stdout, stderr)
			name, phase := "sqlrs-lb-connected-cost", "running"
			if variant == "exited" {
				phase = "exited"
			}
			runner := &costTrimAcquisitionRunner{}
			runner.archives = sessionBundleFixture(t, dir, identity.RunnerID, identity.InvocationRef)
			runner.responses = presentInspection(identity.RunnerID, name, phase, 23)
			if phase == "running" {
				probe := runResponse{output: "sqlrs-step-idle-v1\n"}
				if variant == "idle-refused" {
					probe.err = errors.New("private-probe-marker")
				}
				runner.responses = append(runner.responses, probe)
			}
			postID, postName, postPhase := identity.RunnerID, name, phase
			switch variant {
			case "foreign-after":
				postID = strings.Repeat("f", 64)
			case "renamed-after":
				postName += "-renamed"
			case "changed-phase":
				postPhase = "exited"
			case "missing-file":
				runner.archives[identity.RunnerID+":/sqlrs/capture/"+identity.InvocationRef+"/."] = costTrimBundleTar(t,
					[]costTrimTarEntry{{"stdout.complete", stdout, 0}, {"stderr.complete", stderr, 0}})
			}
			runner.responses = append(runner.responses, presentInspection(postID, postName, postPhase, 23)...)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if variant == "cancel-during-copy" {
				runner.afterCopy = cancel
			}
			got, err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).ReadCompleteLiquibaseStepOutput(ctx, name, identity.RunnerID, "unused-host-directory", identity, 1024)
			positive := variant == "running" || variant == "exited"
			if positive {
				if err != nil || !reflect.DeepEqual(got.Stdout, stdout) || !reflect.DeepEqual(got.Stderr, stderr) ||
					got.StdoutDigest != captureDigest(stdout) || got.StderrDigest != captureDigest(stderr) || got.ExitCode != 0 {
					t.Fatal("connected capture differs from independent bytes", err)
				}
			} else if err == nil || !reflect.DeepEqual(got, CompleteStepCapture{}) || strings.Contains(err.Error(), "private-") {
				t.Fatal("unproved capture returned bytes or private diagnostics", err)
			}
			inspect := "container inspect --format {{json .}} " + identity.RunnerID
			want := []string{inspect, "cp " + identity.RunnerID + ":/sqlrs/capture/" + identity.InvocationRef + "/. -"}
			if variant != "missing-file" && variant != "cancel-during-copy" {
				if phase == "running" {
					want = append(want, "exec -u 0 "+identity.RunnerID+" /bin/sh -c "+stepIdleProbe)
				}
				if variant != "idle-refused" {
					want = append(want, inspect)
				}
			}
			if !reflect.DeepEqual(runner.events, want) || len(runner.copies) != 1 {
				t.Fatal("copy/proof order changed or refusal reached an earlier boundary", variant, runner.events)
			}
		})
	}
}
