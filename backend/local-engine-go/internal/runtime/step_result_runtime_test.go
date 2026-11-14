package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// This local adapter is independent of the legacy archive/combined fixture;
// those paths remain available and retain their original negative controls.
type stepResultRunner struct {
	*combinedCaptureRunner
	result       []byte
	transportErr error
}

// inlineFixtureRunner is used only by the specifically approved OT-IR fixtures;
// independent archive/direct-read fixtures retain their original transport.
type inlineFixtureRunner struct {
	*captureFilesRequirementsRunner
}

func (r *inlineFixtureRunner) RunBounded(ctx context.Context, binary string, args []string, out, diagnostic int64) ([]byte, error) {
	if len(args) == 0 || args[0] != "exec" || out != stepResultWireLimit(1024) || diagnostic != 4096 || !containsArg(args, "-e", "SQLRS_CAPTURE_RESULT=same-exec-v1") {
		return nil, errCaptureBinaryLimits
	}
	value, err := r.Run(ctx, binary, args, nil)
	return []byte(value), err
}

func (r *stepResultRunner) Run(ctx context.Context, binary string, args []string, input *string) (string, error) {
	if len(args) == 0 || args[0] != "container" {
		r.t.Fatal("inline path issued an ordinary non-inspect command")
	}
	return r.combinedCaptureRunner.Run(ctx, binary, args, input)
}
func (r *stepResultRunner) RunBounded(ctx context.Context, binary string, args []string, out, diagnostic int64) ([]byte, error) {
	if binary != "configured-docker" || out != 2*1024+8192+4096+256 || diagnostic != 4096 || !containsArg(args, "-e", "SQLRS_CAPTURE_RESULT=same-exec-v1") || !strings.Contains(strings.Join(args, " "), r.id+" /bin/sh /sqlrs/helper/step-capture.sh liquibase update-count --count 1") {
		r.t.Fatal("wrong bounded invocation identity/quotas/mode")
	}
	if err := r.record(ctx, "apply-result", args); err != nil {
		return r.result, err
	}
	return r.result, r.transportErr
}
func stepResultRuntimeFixture(t *testing.T, prior bool, exit int) (*stepResultRunner, StepCaptureIdentity, string) {
	t.Helper()
	base, identity, dir := combinedCaptureFixture(t, prior, "running", exit)
	manifest, stdout, stderr := stepResultFiles(t, dir)
	return &stepResultRunner{combinedCaptureRunner: base, result: literalStepResultFrame(manifest, stdout, stderr, nil, prior)}, identity, dir
}

func TestStepResultRuntimeExactPathAndReceipt(t *testing.T) {
	for _, prior := range []bool{false, true} {
		for _, exit := range []int{0, 7} {
			r, identity, dir := stepResultRuntimeFixture(t, prior, exit)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			capture, err := NewDocker(Options{Binary: "configured-docker", Runner: r}).InvokeTrackedLiquibaseStep(ctx, r.name, "", identity, []string{"update-count", "--count", "1"}, 1024)
			if err != nil || capture.ExitCode != exit || !reflect.DeepEqual(r.operations, []string{"inspect-before", "apply-result", "inspect-after"}) {
				t.Fatal("not the exact three-command path", r.operations, err)
			}
			if prior && !capture.MatchesPruneObservation(identity, *identity.Prune) || !prior && capture.pruneObservation != nil {
				t.Fatal("wrong prune proof authority")
			}
			deadline, _ := ctx.Deadline()
			for _, observed := range r.deadlines {
				if !observed.Equal(deadline) {
					t.Fatal("whole invocation got a new/30-second deadline")
				}
			}
			original, err := ReadCompleteLiquibaseStepCapture(dir, identity, 1024)
			if err != nil || capture.ManifestDigest != original.ManifestDigest || !reflect.DeepEqual(capture.Stdout, original.Stdout) || !reflect.DeepEqual(capture.Stderr, original.Stderr) {
				t.Fatal("inline differs from durable private files", err)
			}
		}
	}
}

func TestStepResultRuntimeRejectsUnknownTransportOrParent(t *testing.T) {
	for _, variant := range []string{"transport-error", "partial", "foreign-parent", "renamed-parent", "stopped-parent", "stopped-before", "cancelled", "cancel-during", "cancel-after", "missing-bounded", "invalid-identity", "wrong-footer"} {
		t.Run(variant, func(t *testing.T) {
			r, identity, _ := stepResultRuntimeFixture(t, true, 7)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var runner commandRunner = r
			switch variant {
			case "transport-error":
				r.transportErr = errors.New("private-transport-marker")
			case "partial":
				r.result = r.result[:len(r.result)-1]
			case "foreign-parent":
				r.postID = strings.Repeat("e", 64)
			case "renamed-parent":
				r.postName = "foreign"
			case "stopped-parent":
				r.postPhase = "exited"
			case "stopped-before":
				r.phase = "exited"
			case "cancelled":
				cancel()
			case "cancel-during":
				r.after = func(kind string) {
					if kind == "apply-result" {
						cancel()
					}
				}
			case "cancel-after":
				r.after = func(kind string) {
					if kind == "inspect-after" {
						cancel()
					}
				}
			case "missing-bounded":
				runner = struct{ commandRunner }{r}
			case "invalid-identity":
				identity.HelperDigest = "invalid"
			case "wrong-footer":
				r.result = []byte(strings.Replace(string(r.result), "idle absent\n", "idle none\n", 1))
			}
			got, err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).InvokeTrackedLiquibaseStep(ctx, r.name, "", identity, []string{"update-count", "--count", "1"}, 1024)
			if err == nil || !reflect.DeepEqual(got, CompleteStepCapture{}) || strings.Contains(err.Error(), "private-") {
				t.Fatal("unknown result accepted or private error exposed", err)
			}
			if (variant == "cancelled" || variant == "missing-bounded" || variant == "invalid-identity") && len(r.operations) != 0 {
				t.Fatal("missing prerequisite reached Docker", r.operations)
			}
			if strings.HasPrefix(variant, "cancel") && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation identity lost", err)
			}
			for _, operation := range r.operations {
				if operation != "inspect-before" && operation != "apply-result" && operation != "inspect-after" {
					t.Fatal("hidden fallback or retry", r.operations)
				}
			}
		})
	}
}

func TestStepResultRuntimeFreezesAuthorityAndOwnsCosts(t *testing.T) {
	r, identity, _ := stepResultRuntimeFixture(t, true, 0)
	expected := identity
	authority, prior := *identity.Prune, *identity.PriorRetirementRef
	expected.Prune, expected.PriorRetirementRef = &authority, &prior
	r.after = func(kind string) {
		if kind == "inspect-before" {
			identity.Prune.PriorInvocationRef = "caller-mutation"
			*identity.PriorRetirementRef = "caller-mutation"
		}
	}
	recorder := preparecost.NewRecorder(true, 1)
	ctx := preparecost.WithPhase(preparecost.WithRecorder(t.Context(), recorder), preparecost.PhaseStep, 1)
	got, err := NewDocker(Options{Binary: "configured-docker", Runner: r}).InvokeTrackedLiquibaseStep(ctx, r.name, "", identity, []string{"update-count", "--count", "1"}, 1024)
	if err != nil || !got.MatchesPruneObservation(expected, authority) {
		t.Fatal("caller redirected saved authority", err)
	}
	report := recorder.Finish()
	counts := map[preparecost.Operation]int{}
	for _, span := range report.Spans {
		counts[span.Operation]++
	}
	if !report.Complete || counts[preparecost.Invocation] != 1 || counts[preparecost.ResultDecode] != 1 || counts[preparecost.CaptureRead] != 0 || counts[preparecost.CaptureCopy] != 0 || counts[preparecost.PruneObserve] != 0 || counts[preparecost.DockerCommand] != 3 {
		t.Fatal("inline work missing or counted as removed reads", counts)
	}
}
