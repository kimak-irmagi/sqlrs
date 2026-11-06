package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type mainPathPruneProbeRunner struct {
	captureFilesRequirementsRunner
	outputs  [][]byte
	probeErr error
	bounded  [][]string
}

func (r *mainPathPruneProbeRunner) RunBounded(_ context.Context, _ string, args []string, stdoutCap, stderrCap int64) ([]byte, error) {
	r.bounded = append(r.bounded, append([]string(nil), args...))
	if stdoutCap != 64 || stderrCap != 4096 {
		return nil, errors.New("unexpected probe limits")
	}
	if r.probeErr != nil && len(args) > 0 && args[len(args)-1] == "probe-prune" {
		return nil, r.probeErr
	}
	if len(r.outputs) == 0 {
		return nil, errors.New("unplanned bounded command")
	}
	result := r.outputs[0]
	r.outputs = r.outputs[1:]
	return result, nil
}

// MP03: a read-only absence probe cannot run SQL or mutate a capture. Exact
// transport arguments and independent output/status refusals qualify the new
// runtime capability without replacing native MP05 verification.
func TestMainPathCapturePruneProbe(t *testing.T) {
	for _, variant := range []string{"control", "foreign-parent", "stopped-parent", "missing-authorization", "busy-parent", "probe-refused", "partial-response", "oversized-response", "changed-parent", "busy-after", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			_, identity, _ := stepCaptureFixture(t)
			ref := "saved-retirement"
			identity.PriorRetirementRef = &ref
			identity.Prune = &CapturePruneAuthorization{RetirementRef: ref, SessionID: identity.SessionID,
				WorkerGeneration: identity.WorkerGeneration, RunnerID: identity.RunnerID,
				PriorInvocationRef: "prior-call", PriorManifestDigest: "sha256:" + strings.Repeat("e", 64),
				NextInvocationRef: identity.InvocationRef, HelperDigest: identity.HelperDigest}
			name := "sqlrs-lb-main-path-prune"
			runner := &mainPathPruneProbeRunner{outputs: [][]byte{[]byte("sqlrs-step-idle-v1\n"), []byte("sqlrs-capture-prune-absent-v1\n"), []byte("sqlrs-step-idle-v1\n")}}
			beforeID, afterID, phase := identity.RunnerID, identity.RunnerID, "running"
			ctx := t.Context()
			switch variant {
			case "foreign-parent":
				beforeID = strings.Repeat("f", 64)
			case "stopped-parent":
				phase = "exited"
			case "missing-authorization":
				identity.Prune = nil
			case "busy-parent":
				runner.outputs[0] = []byte("busy")
			case "probe-refused":
				runner.probeErr = errors.New("private diagnostic sentinel")
			case "partial-response":
				runner.outputs[1] = []byte("sqlrs-capture-prune-absent-v1")
			case "oversized-response":
				runner.outputs[1] = []byte(strings.Repeat("x", 65))
			case "changed-parent":
				afterID = strings.Repeat("f", 64)
			case "busy-after":
				runner.outputs[2] = []byte("busy")
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			runner.responses = append(accessInspection(beforeID, name, phase, 0), accessInspection(afterID, name, "running", 0)...)
			err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).VerifyLiquibaseCapturePruned(ctx, name, identity)
			if variant != "control" {
				if err == nil || strings.Contains(err.Error(), "private diagnostic sentinel") {
					t.Fatal("unproved absence accepted or private output exposed", err)
				}
				if (variant == "missing-authorization" || variant == "cancelled") && (len(runner.calls) != 0 || len(runner.bounded) != 0) {
					t.Fatal("invalid input reached Docker")
				}
				wantBounded := map[string]int{"foreign-parent": 0, "stopped-parent": 0, "missing-authorization": 0,
					"busy-parent": 1, "probe-refused": 2, "partial-response": 2, "oversized-response": 2,
					"changed-parent": 3, "busy-after": 3, "cancelled": 0}[variant]
				if len(runner.bounded) != wantBounded {
					t.Fatal("refusal did not reach its intended observation boundary", variant, len(runner.bounded), wantBounded)
				}
				return
			}
			if err != nil || len(runner.bounded) != 3 || len(runner.calls) != 4 {
				t.Fatal("exact bounded probe not reached", len(runner.bounded), len(runner.calls), err)
			}
			probe := runner.bounded[1]
			if len(probe) < 5 || !reflect.DeepEqual(probe[len(probe)-4:], []string{identity.RunnerID, "/bin/sh", "/sqlrs/helper/step-capture.sh", "probe-prune"}) {
				t.Fatal("probe did not use fixed read-only helper and exact full ID", probe)
			}
			for _, call := range runner.bounded {
				for _, arg := range call {
					if arg == "liquibase" || arg == "start" || arg == "rm" {
						t.Fatal("absence observation acquired mutation authority")
					}
				}
			}
		})
	}
}

// MP03: even a genuine first invocation explicitly clears authority inherited
// from an image. No previous capture is selected by caller/image environment.
func TestMainPathCapturePruneEmptyAuthorityOverridesEnvironment(t *testing.T) {
	dir, identity, manifest := stepCaptureFixture(t)
	writeStepCaptureFixture(t, dir, manifest, nil, nil)
	name := "sqlrs-lb-main-path-first"
	runner := &captureFilesRequirementsRunner{archives: privateCaptureArchives(t, dir, identity.RunnerID, identity.InvocationRef)}
	runner.responses = append(accessInspection(identity.RunnerID, name, "running", 0), runResponse{})
	runner.responses = append(runner.responses, accessInspection(identity.RunnerID, name, "running", 0)...)
	runner.responses = append(runner.responses, runResponse{output: "sqlrs-step-idle-v1\n"})
	runner.responses = append(runner.responses, accessInspection(identity.RunnerID, name, "running", 0)...)
	_, err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).InvokeTrackedLiquibaseStep(t.Context(), name, "", identity, []string{"update-count", "--count", "1"}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, arg := range runner.calls[2].args {
		if strings.HasPrefix(arg, "SQLRS_CAPTURE_PRUNE_AUTHORIZATION=") {
			count++
			if arg != "SQLRS_CAPTURE_PRUNE_AUTHORIZATION=" {
				t.Fatal("first invocation inherited authority")
			}
		}
	}
	if count != 1 {
		t.Fatal("first invocation did not explicitly clear inherited authority", count)
	}
}
