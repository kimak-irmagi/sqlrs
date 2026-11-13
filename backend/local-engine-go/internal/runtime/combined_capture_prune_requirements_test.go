package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// combinedCaptureRunner records every ordinary/bounded operation in one order.
// It never issues Docker; its independently authored bytes qualify CPR01–07,
// not native helper semantics (qualified separately by CPR08 in Actions).
type combinedCaptureRunner struct {
	t                           *testing.T
	id, name, phase             string
	postID, postName, postPhase string
	wire, proof                 []byte
	fail                        string
	operations                  []string
	arguments                   [][]string
	deadlines                   []time.Time
	inspects                    int
	after                       func(string)
}

func (r *combinedCaptureRunner) record(ctx context.Context, kind string, args []string) error {
	r.operations = append(r.operations, kind)
	r.arguments = append(r.arguments, append([]string(nil), args...))
	deadline, _ := ctx.Deadline()
	r.deadlines = append(r.deadlines, deadline)
	if r.after != nil {
		r.after(kind)
	}
	if kind == r.fail {
		return errors.New("private-combined-error")
	}
	return nil
}

func (r *combinedCaptureRunner) Run(ctx context.Context, binary string, args []string, _ *string) (string, error) {
	if binary != "configured-docker" {
		r.t.Fatal("wrong configured transport")
	}
	if len(args) == 5 && args[0] == "container" && args[1] == "inspect" && args[4] == r.id {
		r.inspects++
		kind := "inspect-before"
		id, name, phase := r.id, r.name, r.phase
		if r.inspects > 1 {
			kind = "inspect-after"
			id, name, phase = r.postID, r.postName, r.postPhase
		}
		if err := r.record(ctx, kind, args); err != nil {
			return "", err
		}
		body, err := json.Marshal(map[string]any{"Id": id, "Name": "/" + name, "State": map[string]any{"Status": phase, "ExitCode": 23}})
		return string(body), err
	}
	if len(args) > 0 && args[0] == "exec" {
		kind := "idle"
		if args[len(args)-1] != stepIdleProbe {
			kind = "apply"
		}
		if err := r.record(ctx, kind, args); err != nil {
			return "", err
		}
		if kind == "idle" {
			return "sqlrs-step-idle-v1\n", nil
		}
		return "", nil
	}
	r.t.Fatal("unexpected ordinary command")
	return "", nil
}

func (r *combinedCaptureRunner) RunBounded(ctx context.Context, binary string, args []string, out, diagnostic int64) ([]byte, error) {
	if binary != "configured-docker" || diagnostic != 4096 {
		r.t.Fatal("wrong bounded transport")
	}
	if len(args) == 3 && args[0] == "cp" && args[1] == r.id+":/sqlrs/capture/invocation-3/." && args[2] == "-" {
		if out != 2*1024+8192+captureArchiveAllowance {
			r.t.Fatal("archive quota changed")
		}
		if err := r.record(ctx, "archive", args); err != nil {
			return nil, err
		}
		return r.wire, nil
	}
	if len(args) > 0 && args[0] == "exec" {
		if out != 64 {
			r.t.Fatal("combined response must be tightly bounded")
		}
		if err := r.record(ctx, "combined", args); err != nil {
			return nil, err
		}
		return r.proof, nil
	}
	r.t.Fatal("unexpected bounded command")
	return nil, nil
}

func combinedCaptureFixture(t *testing.T, prior bool, phase string, exit int) (*combinedCaptureRunner, StepCaptureIdentity, string) {
	t.Helper()
	dir, identity, manifest := stepCaptureFixture(t)
	if prior {
		ref := "retire-invocation-2"
		identity.PriorRetirementRef = &ref
		identity.Prune = &CapturePruneAuthorization{RetirementRef: ref, SessionID: identity.SessionID, WorkerGeneration: identity.WorkerGeneration,
			RunnerID: identity.RunnerID, PriorInvocationRef: "invocation-2", PriorManifestDigest: "sha256:" + strings.Repeat("d", 64), NextInvocationRef: identity.InvocationRef, HelperDigest: identity.HelperDigest}
		manifest["prior_retirement_ref"] = ref
	}
	stdout, stderr := []byte{0, 255, 128, '\n'}, []byte("private-stream-marker")
	manifest["child_exit_code"] = exit
	manifest["stdout_bytes"], manifest["stdout_sha256"] = len(stdout), captureHex(stdout)
	manifest["stderr_bytes"], manifest["stderr_sha256"] = len(stderr), captureHex(stderr)
	writeStepCaptureFixture(t, dir, manifest, stdout, stderr)
	name := "sqlrs-lb-combined"
	return &combinedCaptureRunner{t: t, id: identity.RunnerID, name: name, phase: phase, postID: identity.RunnerID, postName: name, postPhase: phase,
		wire:  sessionBundleFixture(t, dir, identity.RunnerID, identity.InvocationRef)[identity.RunnerID+":/sqlrs/capture/"+identity.InvocationRef+"/."],
		proof: []byte("sqlrs-step-idle-pruned-v1\n")}, identity, dir
}

func readCombinedFixture(t *testing.T, r *combinedCaptureRunner, identity StepCaptureIdentity) (CompleteStepCapture, error) {
	t.Helper()
	return NewDocker(Options{Binary: "configured-docker", Runner: r}).ReadCompleteLiquibaseStepOutput(t.Context(), r.name, r.id, "", identity, 1024)
}

// CPR01/04/06: inspect/archive/combined/inspect on a live attributed parent;
// first/stopped readers retain their existing outcome and cannot mint receipts.
func TestCombinedCapturePruneRead(t *testing.T) {
	for _, tc := range []struct {
		name, phase string
		prior       bool
		exit        int
		want        []string
	}{
		{"combined", "running", true, 0, []string{"inspect-before", "archive", "combined", "inspect-after"}},
		{"child_7", "running", true, 7, []string{"inspect-before", "archive", "combined", "inspect-after"}},
		{"first", "running", false, 0, []string{"inspect-before", "archive", "idle", "inspect-after"}},
		{"stopped", "exited", true, 7, []string{"inspect-before", "archive", "inspect-after"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, identity, _ := combinedCaptureFixture(t, tc.prior, tc.phase, tc.exit)
			got, err := readCombinedFixture(t, r, identity)
			if err != nil || got.ExitCode != tc.exit || !reflect.DeepEqual(got.Stdout, []byte{0, 255, 128, '\n'}) || string(got.Stderr) != "private-stream-marker" || !reflect.DeepEqual(r.operations, tc.want) {
				t.Fatal("completion, bytes, or ordered proof differs", err, r.operations)
			}
			if tc.prior {
				if got.MatchesPruneObservation(identity, *identity.Prune) != (tc.phase == "running") {
					t.Fatal("native receipt provenance differs")
				}
			}
			if tc.prior && tc.phase == "running" {
				args := r.arguments[2]
				if !containsArg(args, "-e", "SQLRS_CAPTURE_COST=0") || args[len(args)-4] != identity.RunnerID || args[len(args)-3] != "/bin/sh" || args[len(args)-2] != "-c" {
					t.Fatal("combined command changed identity or diagnostics authority")
				}
				for _, fragment := range []string{"probe-prune", stepIdleProbe} {
					if !strings.Contains(args[len(args)-1], fragment) {
						t.Fatal("combined command omitted native guard")
					}
				}
				var auth CapturePruneAuthorization
				for _, arg := range args {
					if strings.HasPrefix(arg, "SQLRS_CAPTURE_PRUNE_AUTHORIZATION=") {
						var wire map[string]json.RawMessage
						if err := json.Unmarshal([]byte(strings.TrimPrefix(arg, "SQLRS_CAPTURE_PRUNE_AUTHORIZATION=")), &wire); err != nil {
							t.Fatal(err)
						}
						delete(wire, "protocol")
						body, _ := json.Marshal(wire)
						if err := json.Unmarshal(body, &auth); err != nil {
							t.Fatal(err)
						}
					}
				}
				if auth != *identity.Prune {
					t.Fatal("exact saved authorization lost")
				}
				for _, deadline := range r.deadlines[1:] {
					if deadline.IsZero() || !deadline.Equal(r.deadlines[1]) {
						t.Fatal("capture operations lack one shared deadline")
					}
				}
			}
		})
	}
}

// CPR02: only the actual native reader can seal a matching observation. Caller
// mutation and a syntactically valid local manifest confer no absence authority.
func TestCombinedCapturePruneReceipt(t *testing.T) {
	r, identity, dir := combinedCaptureFixture(t, true, "running", 0)
	got, err := readCombinedFixture(t, r, identity)
	if err != nil {
		t.Fatal(err)
	}
	local, err := ReadCompleteLiquibaseStepCapture(dir, identity, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if (CompleteStepCapture{}).MatchesPruneObservation(identity, *identity.Prune) || local.MatchesPruneObservation(identity, *identity.Prune) {
		t.Fatal("manifest-only or default capture fabricated native proof")
	}
	for _, field := range []string{"retirement", "session", "generation", "runner", "prior", "prior_digest", "next", "helper"} {
		t.Run(field, func(t *testing.T) {
			a := *identity.Prune
			switch field {
			case "retirement":
				a.RetirementRef = "other"
			case "session":
				a.SessionID = "other"
			case "generation":
				a.WorkerGeneration++
			case "runner":
				a.RunnerID = strings.Repeat("e", 64)
			case "prior":
				a.PriorInvocationRef = "other"
			case "prior_digest":
				a.PriorManifestDigest = "sha256:" + strings.Repeat("e", 64)
			case "next":
				a.NextInvocationRef = "other"
			case "helper":
				a.HelperDigest = "sha256:" + strings.Repeat("e", 64)
			}
			// The proposed authority and its nullable links agree with each
			// other. Rejection must come from the retained native observation,
			// not merely mismatched caller arguments before that comparison.
			expected := identity
			expected.Prune = &a
			ref := a.RetirementRef
			expected.PriorRetirementRef = &ref
			if got.MatchesPruneObservation(expected, a) {
				t.Fatal("receipt accepted changed saved field")
			}
		})
	}
	for _, field := range []string{"job", "task", "session", "invocation", "attempt", "generation", "runner", "helper", "execution", "prior_link", "no_authority", "digest", "bytes", "exit", "stdout_digest", "stderr_digest"} {
		t.Run(field, func(t *testing.T) {
			i, c := identity, got
			switch field {
			case "job":
				i.JobID = "other"
			case "task":
				i.TaskID = "other"
			case "session":
				i.SessionID = "other"
			case "invocation":
				i.InvocationRef = "other"
			case "attempt":
				i.AttemptNo++
			case "generation":
				i.WorkerGeneration++
			case "runner":
				i.RunnerID = strings.Repeat("f", 64)
			case "helper":
				i.HelperDigest = "sha256:" + strings.Repeat("f", 64)
			case "execution":
				i.ExecutionDigest = "sha256:" + strings.Repeat("e", 64)
			case "prior_link":
				v := "other"
				i.PriorRetirementRef = &v
			case "no_authority":
				i.Prune = nil
			case "digest":
				c.ManifestDigest = "sha256:" + strings.Repeat("e", 64)
			case "bytes":
				c.ManifestBytes++
			case "exit":
				c.ExitCode = 7
			case "stdout_digest":
				c.StdoutDigest = strings.Repeat("f", 64)
			case "stderr_digest":
				c.StderrDigest = strings.Repeat("f", 64)
			}
			if c.MatchesPruneObservation(i, *identity.Prune) {
				t.Fatal("receipt accepted foreign completion")
			}
		})
	}
	oldPrior, oldAuthority := *identity.PriorRetirementRef, *identity.Prune
	original := identity
	original.PriorRetirementRef = &oldPrior
	original.Prune = &oldAuthority
	*identity.PriorRetirementRef = "changed-after-return"
	identity.Prune.NextInvocationRef = "changed-after-return"
	if !got.MatchesPruneObservation(original, oldAuthority) || got.MatchesPruneObservation(identity, *identity.Prune) {
		t.Fatal("caller mutation retargeted retained proof")
	}
}

// CPR03/06: prior responses are valid and every refusal reaches its own native
// boundary. No bytes/receipt or later command survives an inconclusive read.
func TestCombinedCapturePruneRefusals(t *testing.T) {
	for _, variant := range []string{"inspect-before", "archive", "combined", "inspect-after", "partial", "extra", "oversized", "bad_archive", "changed_id", "changed_name", "changed_phase", "cancel_archive", "cancel_probe", "cancel_post", "already_cancelled", "invalid_authority"} {
		t.Run(variant, func(t *testing.T) {
			r, i, _ := combinedCaptureFixture(t, true, "running", 0)
			want := []string{"inspect-before", "archive", "combined", "inspect-after"}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch variant {
			case "inspect-before":
				r.fail = variant
				want = want[:1]
			case "archive":
				r.fail = variant
				want = want[:2]
			case "combined":
				r.fail = variant
				want = want[:3]
			case "inspect-after":
				r.fail = variant
			case "partial":
				r.proof = []byte("sqlrs-step-idle-pruned-v1")
				want = want[:3]
			case "extra":
				r.proof = append(r.proof, []byte("private-marker")...)
				want = want[:3]
			case "oversized":
				r.proof = []byte(strings.Repeat("x", 65))
				want = want[:3]
			case "bad_archive":
				r.wire = []byte("private-marker")
				want = want[:2]
			case "changed_id":
				r.postID = strings.Repeat("e", 64)
			case "changed_name":
				r.postName = "sqlrs-lb-other"
			case "changed_phase":
				r.postPhase = "exited"
			case "cancel_archive":
				r.after = func(kind string) {
					if kind == "archive" {
						cancel()
					}
				}
				want = want[:2]
			case "cancel_probe":
				r.after = func(kind string) {
					if kind == "combined" {
						cancel()
					}
				}
				want = want[:3]
			case "cancel_post":
				r.after = func(kind string) {
					if kind == "inspect-after" {
						cancel()
					}
				}
			case "already_cancelled":
				cancel()
				want = nil
			case "invalid_authority":
				i.Prune.NextInvocationRef = "other"
				want = nil
			}
			got, err := NewDocker(Options{Binary: "configured-docker", Runner: r}).ReadCompleteLiquibaseStepOutput(ctx, r.name, r.id, "", i, 1024)
			if err == nil || !reflect.DeepEqual(got, CompleteStepCapture{}) || strings.Contains(err.Error(), "private-") || !reflect.DeepEqual(r.operations, want) {
				t.Fatal("wrong refusal boundary or usable/private result", variant, r.operations, err)
			}
			if (strings.HasPrefix(variant, "cancel_") || variant == "already_cancelled") && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost its original classification")
			}
		})
	}
}

// CPR07: the real bounded call belongs to capture.read, not a second prune
// parent; recording off changes no native operation or child outcome.
func TestCombinedCapturePruneCostBoundaries(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
			r, i, _ := combinedCaptureFixture(t, true, "running", 7)
			recorder := preparecost.NewRecorder(enabled, 1)
			ctx := preparecost.WithPhase(preparecost.WithRecorder(t.Context(), recorder), preparecost.PhaseStep, 2)
			got, err := NewDocker(Options{Binary: "configured-docker", Runner: r}).ReadCompleteLiquibaseStepOutput(ctx, r.name, r.id, "", i, 1024)
			if err != nil || got.ExitCode != 7 || !got.MatchesPruneObservation(i, *i.Prune) || len(r.operations) != 4 {
				t.Fatal("recording changed native result", err)
			}
			report := recorder.Finish()
			if !enabled {
				if len(report.Spans) != 0 {
					t.Fatal("disabled recorder emitted spans")
				}
				return
			}
			counts := map[preparecost.Operation]int{}
			for _, span := range report.Spans {
				counts[span.Operation]++
			}
			if !report.Complete || counts[preparecost.CaptureRead] != 1 || counts[preparecost.PruneObserve] != 0 || counts[preparecost.CaptureCopy] != 1 || counts[preparecost.DockerCommand] != 3 || len(report.Helpers) != 0 {
				t.Fatal("combined proof was omitted or counted twice")
			}
		})
	}
}

func TestCombinedCapturePruneInvocation(t *testing.T) {
	r, identity, _ := stepResultRuntimeFixture(t, true, 0)
	got, err := NewDocker(Options{Binary: "configured-docker", Runner: r}).InvokeTrackedLiquibaseStep(t.Context(), r.name, "", identity, []string{"update-count", "--count", "1"}, 1024)
	if err != nil || !got.MatchesPruneObservation(identity, *identity.Prune) || !reflect.DeepEqual(r.operations, []string{"inspect-before", "apply-result", "inspect-after"}) {
		t.Fatal("whole invocation did not use its three actual operations", err, r.operations)
	}
}

func TestCombinedCapturePruneCapabilityAndDeadline(t *testing.T) {
	for _, variant := range []string{"missing-bounded", "expired"} {
		t.Run(variant, func(t *testing.T) {
			r, identity, _ := combinedCaptureFixture(t, true, "running", 0)
			var transport commandRunner = r
			ctx := t.Context()
			if variant == "missing-bounded" {
				transport = struct{ commandRunner }{r}
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			}
			capture, err := NewDocker(Options{Binary: "configured-docker", Runner: transport}).ReadCompleteLiquibaseStepOutput(ctx, r.name, r.id, "", identity, 1024)
			if err == nil || !reflect.DeepEqual(capture, CompleteStepCapture{}) || len(r.operations) != 0 {
				t.Fatal("missing prerequisite reached native observation", err, r.operations)
			}
		})
	}
}
