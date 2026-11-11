package prepare

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// reusableLiquibaseJournal is the consumer-owned durable boundary for one
// parent runner and its serialized child commands. Database publication is
// owned by the wider session journal, never by an in-memory handle.
// planningCaptureMaxBytes preserves the approved bounded private stream limit.
const planningCaptureMaxBytes int64 = 64 << 20

type reusableLiquibaseJournal interface {
	ReadLiquibaseSession(context.Context, string) (queue.LiquibaseSessionRecord, bool, error)
	RequestLiquibaseSessionRunnerCreate(context.Context, queue.LiquibaseSessionRecord) error
	AttachLiquibaseSessionRunner(context.Context, queue.LiquibaseSessionRecord, string) error
	RequestLiquibaseSessionRunnerStart(context.Context, queue.LiquibaseSessionRecord) error
	MarkLiquibaseSessionRunnerRunning(context.Context, queue.LiquibaseSessionRecord) error
	RequestLiquibaseSessionRunnerStop(context.Context, queue.LiquibaseSessionRecord) error
	AcknowledgeLiquibaseSessionRunnerStop(context.Context, queue.LiquibaseSessionRecord) error
	MarkLiquibaseSessionRunnerStopped(context.Context, queue.LiquibaseSessionRecord) error
	PlanLiquibaseSessionInvocation(context.Context, queue.LiquibaseSessionRecord, string, string, queue.LiquibaseInvocationPlan) (queue.LiquibaseInvocationRecord, error)
	RequestLiquibaseSessionExec(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, string) error
	CompleteLiquibaseSessionInvocation(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, queue.LiquibaseInvocationCompletion, string) (int64, error)
}

type reusableLiquibaseRuntime interface {
	CreateContainer(context.Context, engineRuntime.RunRequest) (string, error)
	StartContainer(context.Context, string) error
	InspectNamedContainer(context.Context, string) (engineRuntime.ContainerInspection, bool, error)
	SignalContainer(context.Context, string) error
	RemoveStoppedContainer(context.Context, string) error
	InvokeTrackedLiquibaseStep(context.Context, string, string, engineRuntime.StepCaptureIdentity, []string, int64) (engineRuntime.CompleteStepCapture, error)
}

// liquibaseSessionCoordinator serializes the external actions around the
// durable session journal. It never decides that an ambiguous call is safe to
// repeat. See docs/architecture/liquibase-container-component-structure.md.
type liquibaseSessionCoordinator struct {
	journal reusableLiquibaseJournal
	runtime reusableLiquibaseRuntime
	now     func() string
}

func (c *liquibaseSessionCoordinator) timestamp() string {
	if c.now != nil {
		return c.now()
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func sameLiquibaseSessionIdentity(saved, want queue.LiquibaseSessionRecord) bool {
	return saved.SessionID == want.SessionID && saved.JobID == want.JobID && saved.CloneID == want.CloneID &&
		saved.WorkerGeneration == want.WorkerGeneration && saved.ExecutionFingerprint == want.ExecutionFingerprint &&
		saved.PlannedRunnerName == want.PlannedRunnerName && saved.RunnerConfigDigest == want.RunnerConfigDigest &&
		saved.CaptureHelperDigest == want.CaptureHelperDigest && saved.ProtectedConfigRef == want.ProtectedConfigRef &&
		(want.RunnerID == "" || saved.RunnerID == want.RunnerID)
}

func (c *liquibaseSessionCoordinator) readSession(ctx context.Context, want queue.LiquibaseSessionRecord) (queue.LiquibaseSessionRecord, error) {
	if c == nil || c.journal == nil || c.runtime == nil {
		return queue.LiquibaseSessionRecord{}, fmt.Errorf("Liquibase session capabilities are unavailable")
	}
	saved, found, err := c.journal.ReadLiquibaseSession(ctx, want.SessionID)
	if err != nil || !found || saved.RetiredAt != nil || !sameLiquibaseSessionIdentity(saved, want) {
		return queue.LiquibaseSessionRecord{}, fmt.Errorf("saved Liquibase session identity is unavailable")
	}
	return saved, nil
}

// launchRunner starts only an untouched planned journal. A repeated call can
// reuse a proved running parent; create_requested/start_requested are recovery
// boundaries and cannot silently authorize another create or start request.
func (c *liquibaseSessionCoordinator) launchRunner(ctx context.Context, want queue.LiquibaseSessionRecord,
	request engineRuntime.RunRequest) (queue.LiquibaseSessionRecord, error) {
	session, err := c.readSession(ctx, want)
	if err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	if session.RunnerPhase == "running" {
		view, found, err := c.runtime.InspectNamedContainer(ctx, session.PlannedRunnerName)
		if err != nil || !found || view.ID != session.RunnerID || view.Status != "running" {
			return queue.LiquibaseSessionRecord{}, fmt.Errorf("saved Liquibase runner is not running")
		}
		return session, nil
	}
	if session.RunnerPhase != "planned" || session.RunnerID != "" {
		return queue.LiquibaseSessionRecord{}, fmt.Errorf("Liquibase runner launch requires reconciliation")
	}
	// PC04: private output belongs to the writable container layer. Refuse
	// covering mounts before create intent, not after the helper starts.
	for _, mount := range request.Mounts {
		target := path.Clean(mount.ContainerPath)
		if target == "/" || target == "/sqlrs" || target == "/sqlrs/capture" || strings.HasPrefix(target, "/sqlrs/capture/") {
			return queue.LiquibaseSessionRecord{}, fmt.Errorf("Liquibase private capture mount is forbidden")
		}
	}
	if err := c.journal.RequestLiquibaseSessionRunnerCreate(ctx, session); err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	request.Name, request.EntryPoint, request.User = session.PlannedRunnerName, "/bin/sh", "0"
	request.Labels = map[string]string{"sqlrs.liquibase.session": session.SessionID, "sqlrs.liquibase.config": session.RunnerConfigDigest,
		"sqlrs.liquibase.generation": strconv.Itoa(session.WorkerGeneration), "sqlrs.liquibase.helper": session.CaptureHelperDigest}
	// The immutable helper checks itself and atomically publishes ready.complete
	// before waiting for children. Copy Env: request maps remain caller-owned.
	env := make(map[string]string, len(request.Env)+4)
	for key, value := range request.Env {
		env[key] = value
	}
	env["SQLRS_SESSION_ID"] = session.SessionID
	env["SQLRS_WORKER_GENERATION"] = strconv.Itoa(session.WorkerGeneration)
	env["SQLRS_RUNNER_CONFIG_DIGEST"] = session.RunnerConfigDigest
	env["SQLRS_HELPER_DIGEST"] = session.CaptureHelperDigest
	env["SQLRS_CAPTURE_COST"] = "0"
	request.Env = env
	request.Args = []string{"/sqlrs/helper/step-capture.sh", "serve"}
	id, err := c.runtime.CreateContainer(ctx, request)
	if err != nil {
		return queue.LiquibaseSessionRecord{}, fmt.Errorf("Liquibase runner create outcome requires reconciliation")
	}
	if err := c.journal.AttachLiquibaseSessionRunner(ctx, session, id); err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	session.RunnerID = id
	if err := c.journal.RequestLiquibaseSessionRunnerStart(ctx, session); err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	if err := c.runtime.StartContainer(ctx, id); err != nil {
		return queue.LiquibaseSessionRecord{}, fmt.Errorf("Liquibase runner start outcome requires reconciliation")
	}
	view, found, err := c.runtime.InspectNamedContainer(ctx, session.PlannedRunnerName)
	if err != nil || !found || view.ID != id || view.Status != "running" {
		return queue.LiquibaseSessionRecord{}, fmt.Errorf("started Liquibase runner identity is not proved")
	}
	if err := c.journal.MarkLiquibaseSessionRunnerRunning(ctx, session); err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	session.RunnerPhase = "running"
	return session, nil
}

// reconcileRunnerLaunch adopts only an exact engine-attributed idle parent.
// It never repeats create, nor repeats a start whose response was ambiguous.
// A created container after start_requested remains a non-start recovery
// boundary; running observation can resolve a lost start response. See R01.
func (c *liquibaseSessionCoordinator) reconcileRunnerLaunch(ctx context.Context, want queue.LiquibaseSessionRecord) (queue.LiquibaseSessionRecord, error) {
	session, err := c.readSession(ctx, want)
	if err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	view, found, err := c.runtime.InspectNamedContainer(ctx, session.PlannedRunnerName)
	if err != nil || !found || view.SessionID != session.SessionID || view.RunnerConfigDigest != session.RunnerConfigDigest ||
		view.WorkerGeneration != strconv.Itoa(session.WorkerGeneration) || view.CaptureHelperDigest != session.CaptureHelperDigest ||
		(session.RunnerID != "" && session.RunnerID != view.ID) {
		return queue.LiquibaseSessionRecord{}, fmt.Errorf("saved runner ownership is unproved")
	}
	if session.RunnerPhase == "create_requested" && session.RunnerID == "" && view.Status == "created" {
		if err := c.journal.AttachLiquibaseSessionRunner(ctx, session, view.ID); err != nil {
			return queue.LiquibaseSessionRecord{}, err
		}
		session.RunnerID, session.RunnerPhase = view.ID, "created"
	}
	if session.RunnerPhase == "created" && view.Status == "created" {
		if err := c.journal.RequestLiquibaseSessionRunnerStart(ctx, session); err != nil {
			return queue.LiquibaseSessionRecord{}, err
		}
		session.RunnerPhase = "start_requested"
		if err := c.runtime.StartContainer(ctx, session.RunnerID); err != nil {
			return queue.LiquibaseSessionRecord{}, fmt.Errorf("saved runner start outcome is unproved")
		}
		view, found, err = c.runtime.InspectNamedContainer(ctx, session.PlannedRunnerName)
		if err != nil || !found || view.ID != session.RunnerID || view.Status != "running" {
			return queue.LiquibaseSessionRecord{}, fmt.Errorf("saved runner start observation differs")
		}
	}
	if session.RunnerPhase != "start_requested" || view.Status != "running" {
		return queue.LiquibaseSessionRecord{}, fmt.Errorf("runner launch remains ambiguous")
	}
	if err := c.journal.MarkLiquibaseSessionRunnerRunning(ctx, session); err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	session.RunnerPhase = "running"
	return session, nil
}

// invokeStep retains its manifest for recovery if the response or the final
// SQL write is lost. A natural nonzero child exit remains a known result;
// only the caller's separate failure cleanup decides the job's terminal state.
func (c *liquibaseSessionCoordinator) invokeStep(ctx context.Context, want queue.LiquibaseSessionRecord,
	taskID, executionDigest string, plan queue.LiquibaseInvocationPlan, captureDir string, args []string) (
	queue.LiquibaseInvocationRecord, engineRuntime.CompleteStepCapture, int64, error) {
	session, err := c.readSession(ctx, want)
	if err != nil {
		return queue.LiquibaseInvocationRecord{}, engineRuntime.CompleteStepCapture{}, 0, err
	}
	if session.RunnerPhase != "running" {
		return queue.LiquibaseInvocationRecord{}, engineRuntime.CompleteStepCapture{}, 0, fmt.Errorf("Liquibase session is not runnable")
	}
	pruneJournal, hasJournal := c.journal.(capturePruneJournal)
	_, hasRuntime := c.runtime.(capturePruneRuntime)
	if !hasJournal || !hasRuntime {
		return queue.LiquibaseInvocationRecord{}, engineRuntime.CompleteStepCapture{}, 0, fmt.Errorf("saved Liquibase capture pruning is unavailable")
	}
	invocation, err := preparecost.Observe(ctx, preparecost.InvocationPlan, func(child context.Context) (queue.LiquibaseInvocationRecord, error) {
		return c.journal.PlanLiquibaseSessionInvocation(child, session, taskID, executionDigest, plan)
	})
	if err != nil {
		return queue.LiquibaseInvocationRecord{}, engineRuntime.CompleteStepCapture{}, 0, err
	}
	prune, err := pruneJournal.ReadLiquibaseCapturePrunePlan(ctx, session, invocation)
	if err != nil {
		return invocation, engineRuntime.CompleteStepCapture{}, 0, err
	}
	identity := engineRuntime.StepCaptureIdentity{JobID: session.JobID, TaskID: taskID, SessionID: session.SessionID,
		InvocationRef: plan.InvocationRef, AttemptNo: invocation.Attempt.AttemptNo, WorkerGeneration: session.WorkerGeneration,
		RunnerID: session.RunnerID, ExecutionDigest: executionDigest, HelperDigest: session.CaptureHelperDigest,
		PriorRetirementRef: invocation.PriorRetirementRef}
	if prune != nil {
		authority := projectCapturePrune(*prune)
		identity.Prune = &authority
	}
	if err := preparecost.ObserveCall(ctx, preparecost.InvocationAdmit, func(child context.Context) error {
		return c.journal.RequestLiquibaseSessionExec(child, session, invocation, c.timestamp())
	}); err != nil {
		return invocation, engineRuntime.CompleteStepCapture{}, 0, err
	}
	capture, err := c.runtime.InvokeTrackedLiquibaseStep(ctx, session.PlannedRunnerName, captureDir, identity, args, planningCaptureMaxBytes)
	if err != nil {
		return invocation, engineRuntime.CompleteStepCapture{}, 0, fmt.Errorf("Liquibase child outcome requires reconciliation")
	}
	if prune != nil {
		observation := sessionCapturePruneObservation{identity: identity, capture: capture}
		if err := pruneJournal.ConfirmLiquibaseCapturePruned(ctx, session, invocation, observation, c.timestamp()); err != nil {
			return invocation, capture, 0, err
		}
	}
	proof, err := preparecost.Observe(ctx, preparecost.InvocationComplete, func(child context.Context) (int64, error) {
		return c.journal.CompleteLiquibaseSessionInvocation(child, session, invocation,
			queue.LiquibaseInvocationCompletion{ExitCode: capture.ExitCode, ManifestDigest: capture.ManifestDigest,
				ManifestBytes: capture.ManifestBytes}, c.timestamp())
	})
	return invocation, capture, proof, err
}

// stopRunner preserves the independent stop intent, acknowledgement and
// stopped inspection boundaries. It retains the parent's private output: parent
// removal additionally requires saved disposal and durable DB absence (PC12).
func (c *liquibaseSessionCoordinator) stopRunner(ctx context.Context, want queue.LiquibaseSessionRecord) error {
	session, err := c.readSession(ctx, want)
	if err != nil {
		return err
	}
	if session.RunnerPhase != "stop_requested" && session.RunnerPhase != "stop_acknowledged" && session.RunnerPhase != "stopped" {
		if err := c.journal.RequestLiquibaseSessionRunnerStop(ctx, session); err != nil {
			return err
		}
		session.RunnerPhase = "stop_requested"
	}
	if session.RunnerPhase == "stop_requested" {
		view, found, err := c.runtime.InspectNamedContainer(ctx, session.PlannedRunnerName)
		if err != nil || !found || view.ID != session.RunnerID || view.Status != "running" {
			return fmt.Errorf("exact Liquibase runner stop target is not proved")
		}
		if err := c.runtime.SignalContainer(ctx, session.RunnerID); err != nil {
			return fmt.Errorf("Liquibase runner stop acknowledgement is missing")
		}
		if err := c.journal.AcknowledgeLiquibaseSessionRunnerStop(ctx, session); err != nil {
			return err
		}
		session.RunnerPhase = "stop_acknowledged"
	}
	if session.RunnerPhase == "stop_acknowledged" {
		view, found, err := c.runtime.InspectNamedContainer(ctx, session.PlannedRunnerName)
		if err != nil || !found || view.ID != session.RunnerID || view.Status != "exited" {
			return fmt.Errorf("exact Liquibase runner stop is not proved")
		}
		if err := c.journal.MarkLiquibaseSessionRunnerStopped(ctx, session); err != nil {
			return err
		}
	}
	view, found, err := c.runtime.InspectNamedContainer(ctx, session.PlannedRunnerName)
	if err != nil {
		return fmt.Errorf("Liquibase stopped runner inspection is unavailable")
	}
	if found {
		if view.ID != session.RunnerID || view.Status != "exited" {
			return fmt.Errorf("Liquibase stopped runner identity changed")
		}
	}
	return nil
}
