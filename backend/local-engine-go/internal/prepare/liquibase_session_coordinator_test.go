package prepare

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

type sessionCoordinatorJournal struct {
	session         queue.LiquibaseSessionRecord
	pending         bool
	requested       bool
	completed       int
	priorInvocation string
	prune           *queue.CapturePrunePlan
}

func (j *sessionCoordinatorJournal) ReadLiquibaseSession(context.Context, string) (queue.LiquibaseSessionRecord, bool, error) {
	return j.session, true, nil
}
func (j *sessionCoordinatorJournal) RequestLiquibaseSessionRunnerCreate(context.Context, queue.LiquibaseSessionRecord) error {
	j.session.RunnerPhase = "create_requested"
	return nil
}
func (j *sessionCoordinatorJournal) AttachLiquibaseSessionRunner(_ context.Context, _ queue.LiquibaseSessionRecord, id string) error {
	j.session.RunnerID, j.session.RunnerPhase = id, "created"
	return nil
}
func (j *sessionCoordinatorJournal) RequestLiquibaseSessionRunnerStart(context.Context, queue.LiquibaseSessionRecord) error {
	j.session.RunnerPhase = "start_requested"
	return nil
}
func (j *sessionCoordinatorJournal) MarkLiquibaseSessionRunnerRunning(context.Context, queue.LiquibaseSessionRecord) error {
	j.session.RunnerPhase = "running"
	return nil
}
func (j *sessionCoordinatorJournal) RequestLiquibaseSessionRunnerStop(context.Context, queue.LiquibaseSessionRecord) error {
	j.session.RunnerPhase = "stop_requested"
	return nil
}
func (j *sessionCoordinatorJournal) AcknowledgeLiquibaseSessionRunnerStop(context.Context, queue.LiquibaseSessionRecord) error {
	j.session.RunnerPhase = "stop_acknowledged"
	return nil
}
func (j *sessionCoordinatorJournal) MarkLiquibaseSessionRunnerStopped(context.Context, queue.LiquibaseSessionRecord) error {
	j.session.RunnerPhase = "stopped"
	return nil
}
func (j *sessionCoordinatorJournal) PlanLiquibaseSessionInvocation(_ context.Context, session queue.LiquibaseSessionRecord,
	taskID, digest string, plan queue.LiquibaseInvocationPlan) (queue.LiquibaseInvocationRecord, error) {
	if j.pending {
		return queue.LiquibaseInvocationRecord{}, queue.ErrCommonAttempt
	}
	j.pending, j.requested = true, false
	invocation := queue.LiquibaseInvocationRecord{LiquibaseInvocationPlan: plan, SessionID: session.SessionID,
		HelperDigest: session.CaptureHelperDigest, Attempt: queue.AttemptRecord{AttemptPlan: queue.AttemptPlan{
			JobID: session.JobID, TaskID: taskID, CloneID: session.CloneID, ExecutionDigest: digest,
			WorkerGeneration: session.WorkerGeneration}, AttemptNo: 1, Phase: "planned"}, CaptureSlot: 1}
	j.prune = nil
	// These coordinator-only tests explicitly model the prior published capture;
	// real publication/slot authority is qualified by the SQLite MP01/MP04 tests.
	if j.priorInvocation != "" {
		ref := "retire-" + j.priorInvocation
		invocation.PriorRetirementRef, invocation.CaptureSlot = &ref, 2
		j.prune = &queue.CapturePrunePlan{RetirementRef: ref, SessionID: session.SessionID,
			WorkerGeneration: session.WorkerGeneration, RunnerID: session.RunnerID, HelperDigest: session.CaptureHelperDigest,
			PriorInvocationRef: j.priorInvocation, PriorManifestDigest: "sha256:" + strings.Repeat("e", 64), NextInvocationRef: plan.InvocationRef}
	}
	return invocation, nil
}

func (j *sessionCoordinatorJournal) ReadLiquibaseCapturePrunePlan(_ context.Context, session queue.LiquibaseSessionRecord, invocation queue.LiquibaseInvocationRecord) (*queue.CapturePrunePlan, error) {
	if !j.pending || session.SessionID != j.session.SessionID || invocation.SessionID != session.SessionID {
		return nil, queue.ErrCommonAttempt
	}
	if j.prune == nil {
		return nil, nil
	}
	copy := *j.prune
	return &copy, nil
}

func (j *sessionCoordinatorJournal) ConfirmLiquibaseCapturePruned(ctx context.Context, session queue.LiquibaseSessionRecord, invocation queue.LiquibaseInvocationRecord, verifier queue.LiquibaseCapturePruneVerifier, _ string) error {
	if !j.requested || j.prune == nil || verifier == nil || invocation.InvocationRef != j.prune.NextInvocationRef || session.SessionID != j.prune.SessionID {
		return queue.ErrCommonAttempt
	}
	return verifier.VerifyLiquibaseCapturePruned(ctx, *j.prune)
}
func (j *sessionCoordinatorJournal) RequestLiquibaseSessionExec(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, string) error {
	j.requested = true
	return nil
}
func (j *sessionCoordinatorJournal) CompleteLiquibaseSessionInvocation(_ context.Context, _ queue.LiquibaseSessionRecord,
	invocation queue.LiquibaseInvocationRecord, _ queue.LiquibaseInvocationCompletion, _ string) (int64, error) {
	j.completed++
	j.priorInvocation = invocation.InvocationRef
	j.pending = false
	return int64(j.completed), nil
}

type sessionCoordinatorRuntime struct {
	journal                         *sessionCoordinatorJournal
	id, status                      string
	creates, starts, calls, removes int
	createErr, invokeErr, signalErr error
	labels                          map[string]string
}

func (r *sessionCoordinatorRuntime) CreateContainer(_ context.Context, request engineRuntime.RunRequest) (string, error) {
	if r.journal.session.RunnerPhase != "create_requested" || request.Name != r.journal.session.PlannedRunnerName || request.EntryPoint != "/bin/sh" || request.User != "0" {
		return "", errors.New("create preceded durable intent or used an unbound request")
	}
	r.creates++
	r.labels = request.Labels
	r.status = "created"
	return r.id, r.createErr
}
func (r *sessionCoordinatorRuntime) StartContainer(context.Context, string) error {
	if r.journal.session.RunnerPhase != "start_requested" {
		return errors.New("start preceded durable intent")
	}
	r.starts++
	r.status = "running"
	return nil
}
func (r *sessionCoordinatorRuntime) InspectNamedContainer(context.Context, string) (engineRuntime.ContainerInspection, bool, error) {
	return engineRuntime.ContainerInspection{ID: r.id, Status: r.status, SessionID: r.labels["sqlrs.liquibase.session"], RunnerConfigDigest: r.labels["sqlrs.liquibase.config"],
		WorkerGeneration: r.labels["sqlrs.liquibase.generation"], CaptureHelperDigest: r.labels["sqlrs.liquibase.helper"]}, r.status != "", nil
}

func TestLiquibaseSessionCoordinatorReconcilesAttributedLaunch(t *testing.T) {
	for _, tc := range []struct {
		name, phase, status string
		wrong               bool
		wantErr             bool
		starts              int
	}{
		{"lost create response", "create_requested", "created", false, false, 1},
		{"created before start intent", "created", "created", false, false, 1},
		{"lost start response", "start_requested", "running", false, false, 0},
		{"ambiguous unstarted request", "start_requested", "created", false, true, 0},
		{"wrong generation", "create_requested", "created", true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, j, r := sessionCoordinatorFixture()
			j.session.RunnerPhase = tc.phase
			if tc.phase != "create_requested" {
				j.session.RunnerID = r.id
			}
			r.status = tc.status
			r.labels = map[string]string{"sqlrs.liquibase.session": j.session.SessionID, "sqlrs.liquibase.config": j.session.RunnerConfigDigest,
				"sqlrs.liquibase.generation": "1", "sqlrs.liquibase.helper": j.session.CaptureHelperDigest}
			if tc.wrong {
				r.labels["sqlrs.liquibase.generation"] = "2"
			}
			res, err := c.reconcileRunnerLaunch(context.Background(), j.session)
			if (err != nil) != tc.wantErr || r.creates != 0 || r.starts != tc.starts || (!tc.wantErr && (res.RunnerID != r.id || j.session.RunnerPhase != "running")) {
				t.Fatalf("result=%+v err=%v creates=%d starts=%d", res, err, r.creates, r.starts)
			}
		})
	}
}
func (r *sessionCoordinatorRuntime) SignalContainer(context.Context, string) error {
	if r.journal.session.RunnerPhase != "stop_requested" {
		return errors.New("signal preceded durable intent")
	}
	if r.signalErr != nil {
		return r.signalErr
	}
	r.status = "exited"
	return nil
}
func (r *sessionCoordinatorRuntime) RemoveStoppedContainer(context.Context, string) error {
	if r.journal.session.RunnerPhase != "stopped" || r.status != "exited" {
		return errors.New("removal preceded acknowledged and inspected stop")
	}
	r.removes++
	r.status = ""
	return nil
}
func (r *sessionCoordinatorRuntime) InvokeTrackedLiquibaseStep(_ context.Context, _, _ string,
	identity engineRuntime.StepCaptureIdentity, _ []string, _ int64) (engineRuntime.CompleteStepCapture, error) {
	if !r.journal.pending || !r.journal.requested || identity.RunnerID != r.id || identity.SessionID != r.journal.session.SessionID {
		return engineRuntime.CompleteStepCapture{}, errors.New("exec preceded durable bound intent")
	}
	r.calls++
	return engineRuntime.CompleteStepCapture{ManifestDigest: "sha256:" + strings.Repeat("e", 64), ManifestBytes: 500}, r.invokeErr
}

func (r *sessionCoordinatorRuntime) VerifyLiquibaseCapturePruned(_ context.Context, name string, identity engineRuntime.StepCaptureIdentity) error {
	if r.status != "running" || r.id != identity.RunnerID || name != r.journal.session.PlannedRunnerName ||
		r.journal.prune == nil || identity.Prune == nil || identity.Prune.RetirementRef != r.journal.prune.RetirementRef ||
		identity.Prune.PriorInvocationRef != r.journal.prune.PriorInvocationRef || identity.Prune.NextInvocationRef != identity.InvocationRef {
		return errors.New("unbound coordinator prune observation")
	}
	return nil
}

func sessionCoordinatorFixture() (*liquibaseSessionCoordinator, *sessionCoordinatorJournal, *sessionCoordinatorRuntime) {
	j := &sessionCoordinatorJournal{session: queue.LiquibaseSessionRecord{SessionID: "session", JobID: "job", CloneID: "clone", WorkerGeneration: 1,
		PlannedRunnerName: "sqlrs-lb-session", RunnerPhase: "planned", CaptureHelperDigest: "sha256:" + strings.Repeat("c", 64)}}
	r := &sessionCoordinatorRuntime{journal: j, id: strings.Repeat("a", 64)}
	return &liquibaseSessionCoordinator{journal: j, runtime: r, now: func() string { return "now" }}, j, r
}

// N02/R01/R02: ordinary steps share one runner, and every external boundary
// follows its durable intent. Private/v2 retains the exact stopped parent;
// removal additionally needs saved disposal and DB absence (PC12).
func TestLiquibaseSessionCoordinatorReusesRunner(t *testing.T) {
	c, j, rt := sessionCoordinatorFixture()
	ctx := context.Background()
	session, err := c.launchRunner(ctx, j.session, engineRuntime.RunRequest{ImageID: "liquibase/liquibase@sha256:" + strings.Repeat("d", 64)})
	if err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{"execute-1", "execute-2", "execute-3"} {
		plan := queue.LiquibaseInvocationPlan{InvocationRef: taskID, StdoutRef: taskID + "-stdout", StderrRef: taskID + "-stderr", ManifestRef: taskID + "-manifest", ProtocolRevision: engineRuntime.LiquibaseStepCaptureProtocol, CreatedAt: "now"}
		if _, _, _, err := c.invokeStep(ctx, session, taskID, "digest", plan, "/protected/capture", []string{"update-count", "--count", "1"}); err != nil {
			t.Fatal(err)
		}
	}
	if rt.creates != 1 || rt.starts != 1 || rt.calls != 3 || j.completed != 3 || rt.removes != 0 {
		t.Fatalf("creates=%d starts=%d calls=%d completed=%d removes=%d", rt.creates, rt.starts, rt.calls, j.completed, rt.removes)
	}
	if err := c.stopRunner(ctx, session); err != nil || rt.removes != 0 || j.session.RunnerPhase != "stopped" || rt.status != "exited" || rt.id != session.RunnerID {
		t.Fatalf("cleanup err=%v removes=%d", err, rt.removes)
	}
}

func TestLiquibaseSessionCoordinatorRetainsAmbiguousBoundaries(t *testing.T) {
	t.Run("lost create", func(t *testing.T) {
		c, j, rt := sessionCoordinatorFixture()
		rt.createErr = errors.New("lost create reply")
		request := engineRuntime.RunRequest{ImageID: "pinned-image"}
		if _, err := c.launchRunner(context.Background(), j.session, request); err == nil {
			t.Fatal("lost create reply was accepted")
		}
		if _, err := c.launchRunner(context.Background(), j.session, request); err == nil || rt.creates != 1 {
			t.Fatalf("ambiguous create repeated: err=%v creates=%d", err, rt.creates)
		}
	})
	t.Run("lost exec", func(t *testing.T) {
		c, j, rt := sessionCoordinatorFixture()
		session, err := c.launchRunner(context.Background(), j.session, engineRuntime.RunRequest{ImageID: "pinned-image"})
		if err != nil {
			t.Fatal(err)
		}
		rt.invokeErr = errors.New("lost exec reply")
		plan := queue.LiquibaseInvocationPlan{InvocationRef: "step", CreatedAt: "now"}
		if _, _, _, err := c.invokeStep(context.Background(), session, "execute", "digest", plan, "/protected", []string{"update-count"}); err == nil || j.completed != 0 || !j.pending {
			t.Fatalf("unknown exec resolved: err=%v completed=%d pending=%t", err, j.completed, j.pending)
		}
		rt.signalErr = errors.New("lost stop reply")
		if err := c.stopRunner(context.Background(), session); err == nil || rt.removes != 0 || j.session.RunnerPhase != "stop_requested" {
			t.Fatalf("missing acknowledgement fabricated: err=%v removes=%d phase=%s", err, rt.removes, j.session.RunnerPhase)
		}
	})
}
