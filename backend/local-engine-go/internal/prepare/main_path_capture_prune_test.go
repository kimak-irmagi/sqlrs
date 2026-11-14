package prepare

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// MP03 uses explicit saved queue authority while keeping the old coordinator
// test's child/start counters independent from read-only absence observations.
type mainPathDispatchJournal struct {
	*sessionCoordinatorJournal
	readErr, requestErr, confirmationErr error
	readCalls, confirmationCalls         int
}

func (j *mainPathDispatchJournal) ReadLiquibaseCapturePrunePlan(ctx context.Context, session queue.LiquibaseSessionRecord, invocation queue.LiquibaseInvocationRecord) (*queue.CapturePrunePlan, error) {
	j.readCalls++
	if j.readErr != nil {
		return nil, j.readErr
	}
	return j.sessionCoordinatorJournal.ReadLiquibaseCapturePrunePlan(ctx, session, invocation)
}

func (j *mainPathDispatchJournal) RequestLiquibaseSessionExec(ctx context.Context, session queue.LiquibaseSessionRecord, invocation queue.LiquibaseInvocationRecord, at string) error {
	if j.requestErr != nil {
		return j.requestErr
	}
	return j.sessionCoordinatorJournal.RequestLiquibaseSessionExec(ctx, session, invocation, at)
}

func (j *mainPathDispatchJournal) ConfirmLiquibaseCapturePruned(ctx context.Context, session queue.LiquibaseSessionRecord, invocation queue.LiquibaseInvocationRecord, verifier queue.LiquibaseCapturePruneVerifier, at string) error {
	j.confirmationCalls++
	if j.confirmationErr != nil {
		return j.confirmationErr
	}
	return j.sessionCoordinatorJournal.ConfirmLiquibaseCapturePruned(ctx, session, invocation, verifier, at)
}

type mainPathDispatchRuntime struct {
	*sessionCoordinatorRuntime
	identity engineRuntime.StepCaptureIdentity
}

func (r *mainPathDispatchRuntime) InvokeTrackedLiquibaseStep(ctx context.Context, name, dir string, identity engineRuntime.StepCaptureIdentity, args []string, limit int64) (engineRuntime.CompleteStepCapture, error) {
	r.identity = identity
	return r.sessionCoordinatorRuntime.InvokeTrackedLiquibaseStep(ctx, name, dir, identity, args, limit)
}

// MP02/MP03: missing capabilities/read refusal/admission refusal block the
// command; a lost response never supplies permission to invoke B again.
func TestMainPathCapturePruneDispatch(t *testing.T) {
	for _, variant := range []string{"control", "read-refused", "request-refused", "missing-reader", "missing-probe", "lost-response"} {
		t.Run(variant, func(t *testing.T) {
			c, baseJournal, baseRuntime := sessionCoordinatorFixture()
			baseJournal.session.RunnerID, baseJournal.session.RunnerPhase = baseRuntime.id, "running"
			baseRuntime.status = "running"
			baseJournal.completed = 1
			baseJournal.priorInvocation = "published-A"
			j := &mainPathDispatchJournal{sessionCoordinatorJournal: baseJournal}
			r := &mainPathDispatchRuntime{sessionCoordinatorRuntime: baseRuntime}
			c.journal, c.runtime = j, r
			switch variant {
			case "read-refused":
				j.readErr = errors.New("saved linkage unavailable")
			case "request-refused":
				j.requestErr = errors.New("exec admission refused")
			case "missing-reader":
				c.journal = struct{ reusableLiquibaseJournal }{j}
			case "missing-probe":
				c.runtime = struct{ reusableLiquibaseRuntime }{r}
			case "lost-response":
				baseRuntime.invokeErr = errors.New("lost response")
			}
			plan := queue.LiquibaseInvocationPlan{InvocationRef: "next-B", StdoutRef: "next-B-stdout", StderrRef: "next-B-stderr", ManifestRef: "next-B-manifest", ProtocolRevision: "lb-step-capture-v2"}
			_, _, _, err := c.invokeStep(t.Context(), baseJournal.session, "execute-B", "sha256:"+strings.Repeat("d", 64), plan, "", []string{"update-count", "--count", "1"})
			if variant != "control" {
				wantCalls := 0
				if variant == "lost-response" {
					wantCalls = 1
				}
				if err == nil || baseRuntime.calls != wantCalls || baseJournal.completed != 1 || j.confirmationCalls != 0 {
					t.Fatal("refusal crossed command/result boundary", baseRuntime.calls, baseJournal.completed, j.confirmationCalls, err)
				}
				wantReads := 1
				if variant == "missing-reader" || variant == "missing-probe" {
					wantReads = 0
				}
				if j.readCalls != wantReads || variant == "request-refused" && !errors.Is(err, j.requestErr) || variant == "read-refused" && !errors.Is(err, j.readErr) {
					t.Fatal("refusal did not reach its intended journal boundary", j.readCalls, err)
				}
				if variant == "lost-response" {
					_, _, _, repeatErr := c.invokeStep(t.Context(), baseJournal.session, "execute-B", "binding", plan, "", []string{"update-count", "--count", "1"})
					if repeatErr == nil || baseRuntime.calls != 1 {
						t.Fatal("lost response caused duplicate SQL")
					}
				}
				return
			}
			if err != nil || baseRuntime.calls != 1 || baseJournal.completed != 2 || j.readCalls != 1 || j.confirmationCalls != 1 {
				t.Fatal("normal dispatch did not use saved authority", err)
			}
			identity := r.identity
			p := baseJournal.prune
			want := &engineRuntime.CapturePruneAuthorization{RetirementRef: p.RetirementRef, SessionID: p.SessionID,
				WorkerGeneration: p.WorkerGeneration, RunnerID: p.RunnerID, HelperDigest: p.HelperDigest,
				PriorInvocationRef: p.PriorInvocationRef, PriorManifestDigest: p.PriorManifestDigest, NextInvocationRef: "next-B"}
			if identity.PriorRetirementRef == nil || *identity.PriorRetirementRef != p.RetirementRef || !reflect.DeepEqual(identity.Prune, want) {
				t.Fatal("saved authority was not projected exactly", identity.Prune)
			}
		})
	}
}
