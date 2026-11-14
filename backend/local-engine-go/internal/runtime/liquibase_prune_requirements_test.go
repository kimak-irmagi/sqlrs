package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// OT-CPR01 is local to the binding test: the archive fixture remains unchanged,
// while the combined native observation has its own bounded response/counter.
type pruneBindingRunner struct {
	*captureFilesRequirementsRunner
	probes int
}

func (r *pruneBindingRunner) RunBounded(ctx context.Context, name string, args []string, out, diagnostic int64) ([]byte, error) {
	if len(args) > 0 && args[0] == "exec" {
		r.probes++
		if out != stepResultWireLimit(1024) || diagnostic != 4096 || !containsArg(args, "-e", "SQLRS_CAPTURE_RESULT=same-exec-v1") {
			return nil, fmt.Errorf("unexpected bounded binding probe")
		}
		value, err := r.Run(ctx, name, args, nil)
		return []byte(value), err
	}
	return r.captureFilesRequirementsRunner.RunBounded(ctx, name, args, out, diagnostic)
}

// PC10/PC20-N01: one exact typed authorization is carried on the normal helper
// exec. This H check cannot qualify whether the queue actually pinned/published
// its source; those remain journal admission requirements.
func TestLiquibasePrivatePruneInvocationBinding(t *testing.T) {
	for _, variant := range []string{"control", "missing_authorization", "missing_prior", "retirement", "session", "generation", "runner", "helper", "prior_token", "self", "next", "manifest_digest"} {
		t.Run(variant, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			prior := "saved-retirement"
			identity.PriorRetirementRef = &prior
			identity.Prune = &CapturePruneAuthorization{RetirementRef: prior, SessionID: identity.SessionID, WorkerGeneration: identity.WorkerGeneration,
				RunnerID: identity.RunnerID, PriorInvocationRef: "previous-invocation", PriorManifestDigest: "sha256:" + strings.Repeat("d", 64),
				NextInvocationRef: identity.InvocationRef, HelperDigest: identity.HelperDigest}
			manifest["prior_retirement_ref"] = prior
			writeStepCaptureFixture(t, dir, manifest, nil, nil)
			switch variant {
			case "missing_authorization":
				identity.Prune = nil
			case "missing_prior":
				identity.PriorRetirementRef = nil
			case "retirement":
				identity.Prune.RetirementRef = "other-retirement"
			case "session":
				identity.Prune.SessionID = "other-session"
			case "generation":
				identity.Prune.WorkerGeneration++
			case "runner":
				identity.Prune.RunnerID = strings.Repeat("e", 64)
			case "helper":
				identity.Prune.HelperDigest = "sha256:" + strings.Repeat("e", 64)
			case "prior_token":
				identity.Prune.PriorInvocationRef = "../foreign"
			case "self":
				identity.Prune.PriorInvocationRef = identity.InvocationRef
			case "next":
				identity.Prune.NextInvocationRef = "foreign-next"
			case "manifest_digest":
				identity.Prune.PriorManifestDigest = "private-invalid-value"
			}
			name := "sqlrs-lb-prune-binding"
			manifestBytes, stdout, stderr := stepResultFiles(t, dir)
			runner := &captureFilesRequirementsRunner{}
			runner.responses = append(presentInspection(identity.RunnerID, name, "running", 0), runResponse{output: string(literalStepResultFrame(manifestBytes, stdout, stderr, nil, true))})
			runner.responses = append(runner.responses, presentInspection(identity.RunnerID, name, "running", 0)...)
			transport := &pruneBindingRunner{captureFilesRequirementsRunner: runner}
			capture, err := NewDocker(Options{Binary: "configured-docker", Runner: transport}).InvokeTrackedLiquibaseStep(context.Background(), name, "", identity, []string{"update-count", "--count", "1"}, 1024)
			if variant != "control" {
				if err == nil || !reflect.DeepEqual(capture, CompleteStepCapture{}) || len(runner.calls) != 0 || len(runner.copies) != 0 || transport.probes != 0 {
					t.Fatal("invalid prune binding reached Docker or returned capture")
				}
				return
			}
			if err != nil || capture.ExitCode != 0 || len(runner.calls) != 3 || len(runner.copies) != 0 || transport.probes != 1 {
				t.Fatal("prune added a separate command or failed normal capture")
			}
			args := runner.calls[1].args
			count := 0
			for i, arg := range args {
				if !strings.HasPrefix(arg, "SQLRS_CAPTURE_PRUNE_AUTHORIZATION=") {
					continue
				}
				count++
				if i == 0 || args[i-1] != "-e" {
					t.Fatal("authorization not a single exec environment value")
				}
				var fields map[string]any
				if err := json.Unmarshal([]byte(strings.TrimPrefix(arg, "SQLRS_CAPTURE_PRUNE_AUTHORIZATION=")), &fields); err != nil {
					t.Fatal(err)
				}
				want := map[string]any{"protocol": "lb-capture-prune-v1", "retirement_ref": prior,
					"session_id": identity.SessionID, "worker_generation": float64(identity.WorkerGeneration), "runner_id": identity.RunnerID,
					"prior_invocation_ref": "previous-invocation", "prior_manifest_digest": "sha256:" + strings.Repeat("d", 64),
					"next_invocation_ref": identity.InvocationRef, "helper_digest": identity.HelperDigest}
				if !reflect.DeepEqual(fields, want) {
					t.Fatal("exact typed authorization lost")
				}
			}
			if count != 1 {
				t.Fatal("normal helper exec lacks exactly one prune authorization")
			}
		})
	}
}

func TestLiquibasePrivatePruneAuthorizationBound(t *testing.T) {
	_, identity, _ := stepCaptureFixture(t)
	ref := strings.Repeat("r", 128)
	identity.SessionID = strings.Repeat("s", 128)
	identity.InvocationRef = strings.Repeat("n", 128)
	identity.WorkerGeneration = int(^uint(0) >> 1)
	identity.PriorRetirementRef = &ref
	identity.Prune = &CapturePruneAuthorization{RetirementRef: ref, SessionID: identity.SessionID, WorkerGeneration: identity.WorkerGeneration,
		RunnerID: identity.RunnerID, PriorInvocationRef: strings.Repeat("p", 128), PriorManifestDigest: "sha256:" + strings.Repeat("f", 64),
		NextInvocationRef: identity.InvocationRef, HelperDigest: identity.HelperDigest}
	encoded, err := serializeStepPrune(identity)
	if err != nil || len(encoded) == 0 || len(encoded) > 4096 {
		t.Fatal("maximal admitted authorization exceeds fixed wire budget")
	}
}
