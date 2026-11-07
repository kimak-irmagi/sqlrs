package prepare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sqlrs/engine-local/internal/dbms"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/effectiveconfig"
	"github.com/sqlrs/engine-local/internal/prepare/finalizationcodec"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store"
)

// normalLiquibaseJournal is the local worker's consumer-owned capability set.
// Reads never grant replay authority. Installation is separate and psql keeps
// its previous executor. See issue89-normal-path-storage-extraction.md §2.
type normalLiquibaseJournal interface {
	queue.Store
	liquibaseBindingQueue
	reusableLiquibaseJournal
	capturePruneJournal
	planningCloneJournal
	planningCaptureJournal
	trackedAttemptJournal
	projectionAuthority
	LoadPlanningAttempt(context.Context, string, int) (queue.PlanningRecoverySnapshot, bool, error)
	RecordPlanningCloneQuiescence(context.Context, queue.AttemptRecord, int, string, string, string) (int64, error)
	SavePrivatePlanningCaptureDisposal(context.Context, queue.AttemptRecord, int, string, string, string) (queue.PlanningCaptureDisposal, error)
	RetirePrivatePlanningCapture(context.Context, queue.AttemptRecord, int, queue.PlanningArtifactAbsenceVerifier, string) error
	RetirePlanningArtifact(context.Context, queue.AttemptRecord, int, string, queue.PlanningArtifactAbsenceVerifier, string) error
	ReleasePlanningClone(context.Context, queue.AttemptRecord, int, queue.CloneAbsenceVerifier) error
	AppendPlanningSuffix(context.Context, string, []queue.TaskRecord) error
	SavePlanningProjection(context.Context, string, string, string) error
	LoadPlanningProjection(context.Context, string) (string, bool, error)
	SavePlanningDecision(context.Context, string, planningcodec.Decision, string) (string, string, error)
	BindStep(context.Context, string, string, queue.ExecutionBinding, string) error
	NextCloneGeneration(context.Context, string) (int, error)
	PlanLiquibaseSession(context.Context, queue.LiquibaseSessionLaunch) (queue.LiquibaseSessionRecord, error)
	ActivateLiquibaseSessionClone(context.Context, queue.LiquibaseSessionRecord, string, string) error
	RecordLiquibaseSessionReadiness(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseSessionReadinessVerifier, string) (queue.LiquibaseSessionReadiness, error)
	RecordLiquibaseSessionDBStopped(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, string, string, string) (int64, error)
	PublishLiquibaseSessionState(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, store.StateCreate, string, string, int64, int64, string) error
	RequestLiquibaseSessionDBRestart(context.Context, queue.LiquibaseSessionRecord) error
	MarkLiquibaseSessionDBReady(context.Context, queue.LiquibaseSessionRecord, string, string) error
	SaveLiquibaseSessionPublishedDisposal(context.Context, queue.LiquibaseSessionRecord, string) error
	RecordLiquibaseSessionDisposalDBAbsent(context.Context, queue.LiquibaseSessionRecord, queue.CloneAbsenceVerifier, string) error
	RetireLiquibaseSessionPrivateCapture(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseSessionAbsenceVerifier, string) error
	RetirePublishedLiquibaseSession(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseSessionAbsenceVerifier, string) error
	SelectLiquibaseJobFailure(context.Context, string, finalizationcodec.Outcome, string) (queue.FinalizationSnapshot, error)
	SaveLiquibaseSessionFailureDisposal(context.Context, queue.FinalizationSnapshot, queue.LiquibaseSessionRecord, string) error
	DiscardSelectedLiquibaseFailureSession(context.Context, queue.FinalizationSnapshot, queue.LiquibaseSessionRecord, queue.LiquibaseSessionAbsenceVerifier, string) error
	RequestLiquibaseSessionCancellation(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, string) error
	SaveLiquibaseSessionCancellationDisposal(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, string) error
	DiscardCancelledLiquibaseSession(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, queue.LiquibaseSessionAbsenceVerifier, string) error
	BeginFinalization(context.Context, string, finalizationcodec.Outcome, string) (queue.FinalizationSnapshot, error)
	PublishInstanceAndBeginFinalization(context.Context, string, store.InstanceCreate, managedidentity.RuntimeBinding, instanceaccess.Operation) (queue.FinalizationSnapshot, error)
	ConfirmProjectionsRemoved(context.Context, queue.FinalizationSnapshot, queue.LiquibaseProjectionAbsenceVerifier, string) (queue.FinalizationSnapshot, error)
	CommitFinalization(context.Context, queue.FinalizationSnapshot, string, string) error
}

type normalLiquibaseRuntime interface {
	reusableLiquibaseRuntime
	capturePruneRuntime
	trackedContainerRuntime
	planningCaptureReader
	ReadLiquibaseSessionReadiness(context.Context, string, engineRuntime.SessionReadinessIdentity) (engineRuntime.SessionReadiness, error)
	VerifyContainerAbsent(context.Context, string) error
}

type normalLiquibaseWorking struct {
	session     queue.LiquibaseSessionRecord
	clone       queue.ClonePlan
	rt          *jobRuntime
	coordinator *liquibaseSessionCoordinator
	protected   *effectiveconfig.ProtectedConfig
	args        []string
	helper      string
	invocation  queue.LiquibaseInvocationRecord
}

func (m *PrepareService) normalTimestamp() string { return m.now().UTC().Format(time.RFC3339Nano) }

// runContainerLiquibase owns only a fresh normal chain. On an unresolved effect
// it retains the exact journal/resources and stops; it never falls back to host
// execution, replans from mutable files, or automatically repeats SQL.
func (c *jobCoordinator) runContainerLiquibase(p preparedRequest, jobID string) {
	m := c.m
	ctx, cancel := context.WithCancel(context.Background())
	runner := m.registerRunner(jobID, cancel)
	defer func() { cancel(); close(runner.done); m.unregisterRunner(jobID) }()
	q, ok := m.queue.(normalLiquibaseJournal)
	runtime, runtimeOK := m.runtime.(normalLiquibaseRuntime)
	if !ok || !runtimeOK || m.access == nil || p.managed.Validate() != nil {
		m.pauseContainerLiquibase(jobID, fmt.Errorf("normal container capabilities unavailable"))
		return
	}
	if err := m.validateStore(m.stateStoreRoot); err != nil {
		m.pauseContainerLiquibase(jobID, err)
		return
	}
	if err := q.UpdateJob(ctx, jobID, queue.JobUpdate{Status: strPtr(StatusRunning), StartedAt: strPtr(m.normalTimestamp())}); err != nil {
		m.pauseContainerLiquibase(jobID, err)
		return
	}
	_ = m.appendEvent(jobID, Event{Type: "status", Status: StatusRunning, Ts: m.normalTimestamp()})
	bound, _, err := m.bindContainerLiquibase(ctx, jobID, p)
	if err != nil {
		m.pauseContainerLiquibase(jobID, err)
		return
	}
	p = bound
	plan, err := m.planNormalContainerLiquibase(ctx, q, runtime, jobID, p)
	if err != nil {
		m.pauseContainerLiquibase(jobID, err)
		return
	}
	p.liquibasePlan = make([]LiquibaseChangeset, len(plan.Changesets))
	for i, item := range plan.Changesets {
		p.liquibasePlan[i] = LiquibaseChangeset{ID: item.ID, Author: item.Author, Path: item.Path, SQLHash: item.SQLHash, Checksum: item.Checksum}
	}
	tasks, stateID, problem := c.buildPlanLiquibase(ctx, jobID, p)
	if problem != nil {
		m.pauseContainerLiquibase(jobID, fmt.Errorf("normal plan projection unavailable: %s", problem.Code))
		return
	}
	signature, signatureError := m.computeJobSignatureFromPlan(p, tasks)
	if signatureError != nil {
		m.pauseContainerLiquibase(jobID, fmt.Errorf("job signature unavailable: %s", signatureError.Code))
		return
	}
	if err := q.UpdateJob(ctx, jobID, queue.JobUpdate{Signature: &signature}); err != nil {
		m.pauseContainerLiquibase(jobID, err)
		return
	}
	view, err := m.containerJobView(jobID)
	if err != nil {
		m.pauseContainerLiquibase(jobID, err)
		return
	}
	if view.Purpose == "cache_explain" {
		if err := m.finishNormalLiquibaseExplanation(ctx, q, jobID, p, plan, tasks, stateID); err != nil {
			m.pauseContainerLiquibase(jobID, err)
		}
		return
	}
	if p.request.PlanOnly {
		encoded, _ := json.Marshal(tasks)
		if err := q.SavePlanningProjection(ctx, jobID, string(encoded), m.normalTimestamp()); err != nil {
			m.pauseContainerLiquibase(jobID, err)
			return
		}
		projection, found, err := q.LoadPlanningProjection(ctx, jobID)
		if err != nil || !found {
			m.pauseContainerLiquibase(jobID, fmt.Errorf("saved plan projection unavailable"))
			return
		}
		intent, err := q.BeginFinalization(ctx, jobID, finalizationcodec.Outcome{Kind: finalizationcodec.PlanSuccess, EventProfile: finalizationcodec.Status, Plan: &finalizationcodec.PlanReference{PlanDigest: plan.PlanDigest, ProjectionDigest: digestPlanningProjection(projection)}}, m.normalTimestamp())
		if err == nil {
			err = m.finishNormalLiquibase(ctx, q, intent)
		}
		if err != nil {
			m.pauseContainerLiquibase(jobID, err)
		}
		return
	}
	if err := q.AppendPlanningSuffix(ctx, jobID, taskRecordsFromPlan(jobID, tasks)); err != nil {
		m.pauseContainerLiquibase(jobID, err)
		return
	}
	record, found, err := q.GetLiquibaseExecutionForJob(ctx, jobID)
	if err != nil || !found {
		m.pauseContainerLiquibase(jobID, fmt.Errorf("pinned execution disappeared"))
		return
	}
	var working *normalLiquibaseWorking
	for position, task := range taskStatesFromPlan(tasks) {
		if task.Type == "plan" {
			continue
		}
		if ctx.Err() != nil {
			m.pauseContainerLiquibase(jobID, ctx.Err())
			return
		}
		if task.Type == "resolve_image" {
			if err := m.updateTaskStatus(ctx, jobID, task.TaskID, StatusSucceeded, nil, strPtr(m.normalTimestamp()), nil); err != nil {
				m.pauseContainerLiquibase(jobID, err)
				return
			}
			continue
		}
		if task.Type != "state_execute" {
			continue
		}
		binding, err := queue.ProjectLiquibaseStepBinding(record, p.argsNormalized, task.TaskHash, task.ChangesetID, task.ChangesetAuthor, task.ChangesetPath, task.OutputStateID)
		if err == nil {
			err = q.BindStep(ctx, jobID, task.TaskID, binding, m.normalTimestamp())
		}
		if err != nil {
			m.pauseContainerLiquibase(jobID, err)
			return
		}
		if working == nil && task.Cached != nil && *task.Cached {
			if err := m.updateTaskStatus(ctx, jobID, task.TaskID, StatusSucceeded, nil, strPtr(m.normalTimestamp()), nil); err != nil {
				m.pauseContainerLiquibase(jobID, err)
				return
			}
			continue
		}
		if err := m.updateTaskStatus(ctx, jobID, task.TaskID, StatusRunning, strPtr(m.normalTimestamp()), nil, nil); err != nil {
			m.pauseContainerLiquibase(jobID, err)
			return
		}
		if working == nil {
			working, err = m.startNormalLiquibaseSession(ctx, q, runtime, jobID, p, task, position, binding)
			if err != nil {
				m.pauseContainerLiquibase(jobID, err)
				return
			}
			runner.setRuntime(working.rt)
		}
		if err := m.executeNormalLiquibaseStep(ctx, q, runtime, p, task, binding, working); err != nil {
			if ctx.Err() != nil && working.invocation.Attempt.JobID == jobID {
				if cleanupErr := m.finishNormalLiquibaseCancellation(ctx, q, runtime, working); cleanupErr != nil {
					m.pauseContainerLiquibase(jobID, cleanupErr)
				}
				return
			}
			var known normalLiquibaseChildFailure
			if errors.As(err, &known) {
				if cleanupErr := m.finishNormalLiquibaseFailure(ctx, q, runtime, working); cleanupErr != nil {
					m.pauseContainerLiquibase(jobID, cleanupErr)
				}
				return
			}
			m.pauseContainerLiquibase(jobID, err)
			return
		}
	}
	if working != nil {
		if err := m.retireNormalLiquibaseSession(ctx, q, runtime, working); err != nil {
			m.pauseContainerLiquibase(jobID, err)
			return
		}
		runner.setRuntime(nil)
	}
	if err := m.publishNormalLiquibaseInstance(ctx, q, runtime, runner, jobID, p, stateID); err != nil {
		m.pauseContainerLiquibase(jobID, err)
	}
}

// finishNormalLiquibaseCancellation does not interpret the interrupted exec
// response as a child result. Accepted intent, exact parent stop and fresh
// absence observations are independent prerequisites for discarding the copy.
func (m *PrepareService) finishNormalLiquibaseCancellation(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, w *normalLiquibaseWorking) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	if err := q.RequestLiquibaseSessionCancellation(cleanup, w.session, w.invocation, m.normalTimestamp()); err != nil {
		return err
	}
	if err := w.coordinator.stopRunner(cleanup, w.session); err != nil {
		return err
	}
	stopped, found, err := q.ReadLiquibaseSession(cleanup, w.session.SessionID)
	if err != nil || !found {
		return queue.ErrCommonAttempt
	}
	w.session = stopped
	if err := q.SaveLiquibaseSessionCancellationDisposal(cleanup, w.session, w.invocation, m.normalTimestamp()); err != nil {
		return err
	}
	absence, err := m.disposeNormalLiquibaseSessionResources(cleanup, q, runtime, w)
	if err != nil {
		return err
	}
	if err := q.DiscardCancelledLiquibaseSession(cleanup, w.session, w.invocation, absence, m.normalTimestamp()); err != nil {
		return err
	}
	intent, err := q.BeginFinalization(cleanup, w.session.JobID, finalizationcodec.Outcome{Kind: finalizationcodec.Cancelled, EventProfile: finalizationcodec.StatusError, Problem: &finalizationcodec.PublicError{Code: "cancelled", Message: "job cancelled"}}, m.normalTimestamp())
	if err != nil {
		return err
	}
	return m.finishNormalLiquibase(cleanup, q, intent)
}

// A completed private manifest proves this unsuccessful child is not an
// ambiguous launch. Never include its captured SQL/credentials in diagnostics.
type normalLiquibaseChildFailure struct{}

func (normalLiquibaseChildFailure) Error() string { return "Liquibase child exited unsuccessfully" }

func (m *PrepareService) finishNormalLiquibaseFailure(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, w *normalLiquibaseWorking) error {
	intent, err := q.SelectLiquibaseJobFailure(ctx, w.session.JobID, finalizationcodec.Outcome{Kind: finalizationcodec.Failure, EventProfile: finalizationcodec.StatusError, Problem: &finalizationcodec.PublicError{Code: "liquibase_failed", Message: "Liquibase exited unsuccessfully"}}, m.normalTimestamp())
	if err != nil {
		return fmt.Errorf("select known failure: %w", err)
	}
	if err := w.coordinator.stopRunner(ctx, w.session); err != nil {
		return err
	}
	stopped, found, err := q.ReadLiquibaseSession(ctx, w.session.SessionID)
	if err != nil || !found {
		return queue.ErrCommonAttempt
	}
	w.session = stopped
	if err := q.SaveLiquibaseSessionFailureDisposal(ctx, intent, w.session, m.normalTimestamp()); err != nil {
		return fmt.Errorf("save known-failure disposal: %w", err)
	}
	absence, err := m.disposeNormalLiquibaseSessionResources(ctx, q, runtime, w)
	if err != nil {
		return fmt.Errorf("dispose known-failure resources: %w", err)
	}
	if err := q.DiscardSelectedLiquibaseFailureSession(ctx, intent, w.session, absence, m.normalTimestamp()); err != nil {
		return fmt.Errorf("retire known-failure journal: %w", err)
	}
	if err := m.finishNormalLiquibase(ctx, q, intent); err != nil {
		return fmt.Errorf("finalize known failure: %w", err)
	}
	return nil
}

// finishNormalLiquibaseExplanation freezes a point-in-time decision after its
// independent preview has retired. It never appends executable apply tasks.
func (m *PrepareService) finishNormalLiquibaseExplanation(ctx context.Context, q normalLiquibaseJournal, job string, p preparedRequest, plan queue.PlanningResult, tasks []PlanTask, state string) error {
	signature, problem := m.computeJobSignatureFromPlan(p, tasks)
	if problem != nil {
		return fmt.Errorf("cache signature unavailable: %s", problem.Code)
	}
	cached, err := m.isManagedStateCached(state, p)
	if err != nil {
		return err
	}
	record, found, err := q.GetLiquibaseExecutionForJob(ctx, job)
	if err != nil || !found {
		return queue.ErrLiquibaseBinding
	}
	decision := planningcodec.Decision{Decision: "miss", ReasonCode: "no_matching_state", Signature: signature, ResolvedImageID: p.effectiveImageID(), LiquibaseImageDigest: record.ImageManifestDigest}
	if cached {
		decision.Decision = "hit"
		decision.ReasonCode = "exact_state_match"
		decision.MatchedStateID = state
	}
	_, digest, err := q.SavePlanningDecision(ctx, job, decision, m.normalTimestamp())
	if err != nil {
		return err
	}
	intent, err := q.BeginFinalization(ctx, job, finalizationcodec.Outcome{Kind: finalizationcodec.CacheExplainSuccess, EventProfile: finalizationcodec.Result, Explanation: &finalizationcodec.ExplanationReference{PlanDigest: plan.PlanDigest, DecisionDigest: digest}}, m.normalTimestamp())
	if err != nil {
		return err
	}
	return m.finishNormalLiquibase(ctx, q, intent)
}

// pauseContainerLiquibase is an honest nonterminal result: it retains durable
// ownership and does not delete an uncertain writer or authorize a retry.
func (m *PrepareService) pauseContainerLiquibase(jobID string, cause error) {
	m.logJob(jobID, "normal container path paused: %v", cause)
	problem := ErrorResponse{Code: "recovery_required", Message: "Liquibase work requires reconciliation"}
	encoded, _ := json.Marshal(problem)
	_ = m.queue.UpdateJob(context.Background(), jobID, queue.JobUpdate{Status: strPtr("recovery_required"), ErrorJSON: strPtr(string(encoded))})
	_ = m.appendEvent(jobID, Event{Type: "error", Status: "recovery_required", Ts: m.normalTimestamp(), Error: &problem})
}

func (m *PrepareService) planNormalContainerLiquibase(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, jobID string, p preparedRequest) (queue.PlanningResult, error) {
	record, found, err := q.GetLiquibaseExecutionForJob(ctx, jobID)
	if err != nil || !found {
		return queue.PlanningResult{}, queue.ErrLiquibaseBinding
	}
	launch, err := m.buildPlanningLaunch(ctx, jobID, p, record, m.containerLiquibase.Sources)
	if err != nil {
		return queue.PlanningResult{}, err
	}
	var database *jobRuntime
	attempt, err := launchBoundPlanningClone(ctx, q, boundCloneMaterializerFunc(func(ctx context.Context, clone queue.ClonePlan) (string, error) {
		var problem *ErrorResponse
		database, problem = (&taskExecutor{m: m}).startManagedRuntimeWithPlan(ctx, jobID+"-plan", p, &TaskInput{Kind: clone.SourceKind, ID: clone.SourceImageID}, &clone)
		if problem != nil {
			return "", fmt.Errorf("preview DB: %s (%s)", problem.Message, problem.Details)
		}
		return database.instance.ID, nil
	}), launch)
	if err != nil {
		return queue.PlanningResult{}, err
	}
	saved, found, err := q.LoadPlanningAttempt(ctx, jobID, 1)
	if err != nil || !found {
		return queue.PlanningResult{}, queue.ErrPlanningSchema
	}
	connection, err := m.managedConnection(ctx, database)
	if err != nil {
		return queue.PlanningResult{}, err
	}
	request, protected, captureDir, err := m.buildPlanningExecutorRequest(ctx, p, saved, database.instance, connection.Password, m.containerLiquibase.Sources)
	if err != nil {
		return queue.PlanningResult{}, err
	}
	tracked, err := runTrackedExecutor(ctx, q, runtime, attempt, request, m.normalTimestamp)
	if err != nil {
		return queue.PlanningResult{}, err
	}
	attempt.ExecutorID = tracked.ExecutorID
	// The preview DB is independent of the later working session. Stop and
	// retire its exact assignment before recording absence, not merely pg_ctl.
	preview := &jobRunner{}
	preview.setRuntime(database)
	if err := m.cleanupRuntime(ctx, preview); err != nil {
		return queue.PlanningResult{}, err
	}
	if _, err := q.RecordPlanningCloneQuiescence(ctx, attempt, 1, launch.Clone.PlannedRuntimeName, database.instance.ID, m.normalTimestamp()); err != nil {
		return queue.PlanningResult{}, err
	}
	saved, found, err = q.LoadPlanningAttempt(ctx, jobID, 1)
	if err != nil || !found {
		return queue.PlanningResult{}, queue.ErrPlanningSchema
	}
	result, err := reconcilePlanningCapture(ctx, q, runtime, saved, captureDir, planningCaptureMaxBytes, m.normalTimestamp())
	if err != nil {
		return queue.PlanningResult{}, err
	}
	if _, err := q.SavePrivatePlanningCaptureDisposal(ctx, saved.Attempt, 1, "plan_saved", result.PlanDigest, m.normalTimestamp()); err != nil {
		return queue.PlanningResult{}, err
	}
	if err := runtime.RemoveStoppedContainer(ctx, tracked.ExecutorID); err != nil {
		return queue.PlanningResult{}, err
	}
	observation := normalLiquibaseAbsence{runtime: runtime, planning: saved, protected: protected.HostPath}
	if err := q.RetirePrivatePlanningCapture(ctx, saved.Attempt, 1, observation, m.normalTimestamp()); err != nil {
		return queue.PlanningResult{}, err
	}
	if err := protected.Close(); err != nil {
		return queue.PlanningResult{}, err
	}
	if err := q.RetirePlanningArtifact(ctx, saved.Attempt, 1, "input", observation, m.normalTimestamp()); err != nil {
		return queue.PlanningResult{}, err
	}
	if err := q.ReleasePlanningClone(ctx, saved.Attempt, 1, observation); err != nil {
		return queue.PlanningResult{}, err
	}
	helper := filepath.Join(m.stateStoreRoot, "jobs", jobID, "planning-helpers", launch.Clone.ID, "capture.sh")
	if err := os.Remove(helper); err != nil && !os.IsNotExist(err) {
		return queue.PlanningResult{}, err
	}
	return result, nil
}

// normalLiquibaseAbsence is a real read-only physical observation, bound to
// saved exact IDs. Docker work runs before the journal's write transaction.
type normalLiquibaseAbsence struct {
	runtime   normalLiquibaseRuntime
	planning  queue.PlanningRecoverySnapshot
	protected string
	session   queue.LiquibaseSessionRecord
	clone     queue.ClonePlan
	database  string
}

func (v normalLiquibaseAbsence) VerifyPlanningArtifactAbsent(ctx context.Context, kind, ref, id string) error {
	if id != v.planning.Attempt.ExecutorID {
		return queue.ErrPlanningSchema
	}
	if kind == "executor" && ref == v.planning.Attempt.PlannedExecutorName {
		return v.runtime.VerifyContainerAbsent(ctx, id)
	}
	if kind == "input" && ref == v.planning.Capture.ProtectedInput {
		if _, err := os.Lstat(v.protected); os.IsNotExist(err) {
			return nil
		}
	}
	return queue.ErrPlanningSchema
}
func (v normalLiquibaseAbsence) VerifyReleasedClone(ctx context.Context, clone queue.ClonePlan, id string) error {
	if v.planning.Attempt.JobID != "" {
		if clone != v.planning.Clone || id != v.planning.CloneRuntimeID {
			return queue.ErrPlanningSchema
		}
	} else if clone != v.clone || id != v.database {
		return queue.ErrCommonAttempt
	}
	return v.runtime.VerifyContainerAbsent(ctx, id)
}
func (v normalLiquibaseAbsence) VerifyReleasedLiquibaseSession(ctx context.Context, s queue.LiquibaseSessionRecord, clone queue.ClonePlan, id string) error {
	if !sameLiquibaseSessionIdentity(s, v.session) || s.RunnerID != v.session.RunnerID || clone != v.clone || id != v.database {
		return queue.ErrCommonAttempt
	}
	if err := v.runtime.VerifyContainerAbsent(ctx, id); err != nil {
		return err
	}
	return v.runtime.VerifyContainerAbsent(ctx, s.RunnerID)
}

// A deterministic public-plan digest uses the same canonical bytes as the
// persisted projection. It is not permission to run the projected tasks.
func digestPlanningProjection(encoded string) string { return "sha256:" + sha256Hex(encoded) }

func (m *PrepareService) finishNormalLiquibase(ctx context.Context, q normalLiquibaseJournal, intent queue.FinalizationSnapshot) error {
	return m.withLiquibaseProjectionGate(ctx, intent.JobID, func() error {
		owner, err := m.localLiquibaseProjectionOwner(false)
		if err != nil {
			return err
		}
		if err := owner.CleanupJob(ctx, intent); err != nil {
			return err
		}
		confirmed, err := q.ConfirmProjectionsRemoved(ctx, intent, owner, m.normalTimestamp())
		if err != nil {
			return err
		}
		at := m.now().UTC()
		if err := q.CommitFinalization(ctx, confirmed, at.Format(time.RFC3339Nano), at.Add(24*time.Hour).Format(time.RFC3339Nano)); err != nil {
			return err
		}
		m.events.notify(intent.JobID)
		return nil
	})
}

func (m *PrepareService) startNormalLiquibaseSession(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, jobID string, p preparedRequest, task taskState, position int, binding queue.ExecutionBinding) (*normalLiquibaseWorking, error) {
	id, err := randomHex(12)
	if err != nil {
		return nil, err
	}
	cloneID, err := randomHex(12)
	if err != nil {
		return nil, err
	}
	generation, err := q.NextCloneGeneration(ctx, jobID)
	if err != nil {
		return nil, err
	}
	helperBytes, helperDigest := engineRuntime.LiquibaseStepCaptureHelper()
	stored, err := m.containerLiquibase.Sources.PublishBlob(ctx, helperBytes)
	if err != nil || stored != helperDigest {
		return nil, fmt.Errorf("bound step helper unavailable")
	}
	protectedRef, err := p.liquibaseProjection.protectedRef(id)
	if err != nil {
		return nil, err
	}
	clone := queue.ClonePlan{ID: cloneID, Generation: generation, SourceKind: task.Input.Kind, PlannedRuntimeName: "sqlrs-db-" + cloneID,
		Ref: filepath.ToSlash(filepath.Join("jobs", jobID, "runtime_"+managedOperationRef(m.stateStoreRoot, jobID, task.Input.Kind, task.Input.ID)))}
	if task.Input.Kind == "image" {
		clone.SourceImageID = task.Input.ID
	} else {
		clone.SourceStateID = task.Input.ID
	}
	session := queue.LiquibaseSessionRecord{SessionID: id, JobID: jobID, CloneID: cloneID, SessionNo: 1, WorkerGeneration: 1, ExecutionFingerprint: p.liquibaseFingerprint,
		StartKind: task.Input.Kind, StartID: task.Input.ID, StartPosition: position, PlannedRunnerName: "sqlrs-lb-session-" + id,
		RunnerConfigDigest:  digestLiquibaseValues("lb-session-config-v1", p.liquibaseFingerprint, p.effectiveImageID(), cloneID, helperDigest, protectedRef, p.argsNormalized),
		CaptureHelperDigest: helperDigest, ProtectedConfigRef: protectedRef, CreatedAt: m.normalTimestamp()}
	session, err = q.PlanLiquibaseSession(ctx, queue.LiquibaseSessionLaunch{Session: session, Clone: clone, FirstTaskID: task.TaskID, ExecutionDigest: binding.Digest, HelperLeaseID: "session-helper-" + id})
	if err != nil {
		return nil, err
	}
	rt, problem := (&taskExecutor{m: m}).startManagedRuntimeWithPlan(ctx, jobID, p, task.Input, &clone)
	if problem != nil {
		return nil, fmt.Errorf("working database unavailable: %s (%s)", problem.Message, problem.Details)
	}
	working := &normalLiquibaseWorking{session: session, clone: clone, rt: rt, coordinator: &liquibaseSessionCoordinator{journal: q, runtime: runtime, now: m.normalTimestamp}}
	if err := q.ActivateLiquibaseSessionClone(ctx, session, clone.PlannedRuntimeName, rt.instance.ID); err != nil {
		return nil, err
	}
	working.helper = filepath.Join(m.stateStoreRoot, "jobs", jobID, "liquibase-sessions", id, "step-capture.sh")
	if err := m.containerLiquibase.Sources.MaterializeBlob(ctx, helperDigest, working.helper, maxPlanningHelperBytes); err != nil {
		return nil, err
	}
	connection, err := m.managedConnection(ctx, rt)
	if err != nil {
		return nil, err
	}
	request, protected, err := m.liquibaseContainerRequestWithConfig(p, rt.instance, applyLiquibaseTaskArgs(p.normalizedArgs, task), connection.Password, func(options effectiveconfig.ProtectedOptions) (*effectiveconfig.ProtectedConfig, error) {
		options.Root = p.liquibaseProjection.Settings
		return effectiveconfig.CreatePlannedProtected(options, protectedRef)
	})
	if err != nil {
		return nil, err
	}
	working.protected, working.args = protected, request.Args
	mounts := append([]engineRuntime.Mount(nil), request.Mounts...)
	mounts = append(mounts, engineRuntime.Mount{HostPath: working.helper, ContainerPath: "/sqlrs/helper/step-capture.sh", ReadOnly: true})
	working.session, err = working.coordinator.launchRunner(ctx, session, engineRuntime.RunRequest{ImageID: request.ImageID, Env: request.Env, Dir: request.WorkDir, Network: request.Network, Mounts: mounts})
	if err != nil {
		return nil, err
	}
	if _, err := q.RecordLiquibaseSessionReadiness(ctx, working.session, normalReadinessObserver{runtime}, m.normalTimestamp()); err != nil {
		return nil, err
	}
	return working, nil
}

type normalReadinessObserver struct{ runtime normalLiquibaseRuntime }

func (v normalReadinessObserver) VerifyLiquibaseSessionReady(ctx context.Context, s queue.LiquibaseSessionRecord) (string, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	identity := engineRuntime.SessionReadinessIdentity{SessionID: s.SessionID, RunnerID: s.RunnerID, RunnerConfigDigest: s.RunnerConfigDigest, HelperDigest: s.CaptureHelperDigest, WorkerGeneration: s.WorkerGeneration}
	for {
		ready, err := v.runtime.ReadLiquibaseSessionReadiness(ctx, s.PlannedRunnerName, identity)
		if err == nil {
			return ready.ReadyDigest, ready.ReadyBytes, nil
		}
		select {
		case <-ctx.Done():
			return "", 0, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *PrepareService) executeNormalLiquibaseStep(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, p preparedRequest, task taskState, binding queue.ExecutionBinding, w *normalLiquibaseWorking) error {
	var err error
	w.session, _, err = q.ReadLiquibaseSession(ctx, w.session.SessionID)
	if err != nil {
		return err
	}
	if w.session.DBPhase == "stopped" {
		if err := q.RequestLiquibaseSessionDBRestart(ctx, w.session); err != nil {
			return err
		}
		if err := m.dbms.ResumeSnapshot(ctx, w.rt.instance); err != nil {
			return err
		}
		if err := m.verifyManagedRuntime(ctx, w.rt); err != nil {
			return err
		}
		if err := q.MarkLiquibaseSessionDBReady(ctx, w.session, w.clone.PlannedRuntimeName, w.rt.instance.ID); err != nil {
			return err
		}
		w.session, _, err = q.ReadLiquibaseSession(ctx, w.session.SessionID)
		if err != nil {
			return err
		}
	}
	id, err := randomHex(12)
	if err != nil {
		return err
	}
	ref := "step-" + id
	plan := queue.LiquibaseInvocationPlan{InvocationRef: ref, StdoutRef: ref + "-stdout", StderrRef: ref + "-stderr", ManifestRef: ref + "-manifest", ProtocolRevision: engineRuntime.LiquibaseStepCaptureProtocol, CreatedAt: m.normalTimestamp()}
	invocation, capture, complete, err := w.coordinator.invokeStep(ctx, w.session, task.TaskID, binding.Digest, plan, "", w.args)
	w.invocation = invocation
	if err != nil {
		return err
	}
	if capture.ExitCode != 0 {
		return normalLiquibaseChildFailure{}
	}
	if err := m.verifyManagedRuntime(ctx, w.rt); err != nil {
		return err
	}
	if err := m.dbms.PrepareSnapshot(ctx, w.rt.instance); err != nil {
		return err
	}
	quiet, err := q.RecordLiquibaseSessionDBStopped(ctx, w.session, invocation, w.clone.PlannedRuntimeName, w.rt.instance.ID, m.normalTimestamp())
	if err != nil {
		return err
	}
	if err := m.access.RecordSeal(ctx, task.OutputStateID, w.rt.operation, w.rt.instance.Binding); err != nil {
		return err
	}
	paths, err := resolveStatePaths(m.stateStoreRoot, p.effectiveImageID(), task.OutputStateID, m.statefs)
	if err != nil {
		return err
	}
	if problem := m.ensureCacheCapacity(ctx, w.session.JobID, "snapshot", task.OutputStateID); problem != nil {
		return fmt.Errorf("snapshot capacity unavailable: %s", problem.Code)
	}
	if err := os.MkdirAll(paths.statesDir, 0700); err != nil {
		return err
	}
	if err := m.statefs.EnsureStateDir(ctx, paths.stateDir); err != nil {
		return err
	}
	kind := snapshotKind(m.statefs)
	return withStateBuildLock(ctx, paths.stateDir, stateBuildLockPath(paths.stateDir, kind), kind, func() error {
		if stateBuildMarkerExists(paths.stateDir, kind) {
			return fmt.Errorf("another writer published the same state; reconciliation required")
		}
		if err := m.statefs.Snapshot(ctx, w.rt.dataDir, paths.stateDir); err != nil {
			return err
		}
		if err := writeStateBuildMarker(paths.stateDir, kind); err != nil {
			return err
		}
		size, err := storeUsageFn(paths.stateDir)
		if err != nil {
			return err
		}
		entry := store.StateCreate{StateID: task.OutputStateID, StateFingerprint: task.OutputStateID, ParentStateID: parentStateID(task.Input), ImageID: p.effectiveImageID(), PrepareKind: "lb", PrepareArgsNormalized: p.argsNormalized, CreatedAt: m.normalTimestamp(), SizeBytes: &size, LineageRef: p.managed.LineageRef, IdentityDigest: p.managed.IdentityDigest, LiquibaseExecutionFingerprint: p.liquibaseFingerprint}
		if err := q.PublishLiquibaseSessionState(ctx, w.session, invocation, entry, paths.stateDir, kind, complete, quiet, m.normalTimestamp()); err != nil {
			return err
		}
		w.rt.stateID = task.OutputStateID
		w.session, _, err = q.ReadLiquibaseSession(ctx, w.session.SessionID)
		if err != nil {
			return err
		}
		return m.appendEvent(w.session.JobID, Event{Type: "task", Status: StatusSucceeded, TaskID: task.TaskID, Ts: m.normalTimestamp()})
	})
}

func (m *PrepareService) retireNormalLiquibaseSession(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, w *normalLiquibaseWorking) error {
	if err := w.coordinator.stopRunner(ctx, w.session); err != nil {
		return err
	}
	stopped, found, err := q.ReadLiquibaseSession(ctx, w.session.SessionID)
	if err != nil || !found {
		return queue.ErrCommonAttempt
	}
	w.session = stopped
	if err := q.SaveLiquibaseSessionPublishedDisposal(ctx, w.session, m.normalTimestamp()); err != nil {
		return err
	}
	absence, err := m.disposeNormalLiquibaseSessionResources(ctx, q, runtime, w)
	if err != nil {
		return err
	}
	return q.RetirePublishedLiquibaseSession(ctx, w.session, absence, m.normalTimestamp())
}

// disposeNormalLiquibaseSessionResources consumes an already saved disposal
// basis. Its exact native absence probes run outside SQLite transactions.
func (m *PrepareService) disposeNormalLiquibaseSessionResources(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, w *normalLiquibaseWorking) (normalLiquibaseAbsence, error) {
	absence := normalLiquibaseAbsence{runtime: runtime, session: w.session, clone: w.clone, database: w.rt.instance.ID}
	if err := m.access.RetireOperation(ctx, w.rt.operation); err != nil {
		return absence, err
	}
	if err := m.runtime.Stop(ctx, w.rt.instance.ID); err != nil {
		return absence, err
	}
	if err := q.RecordLiquibaseSessionDisposalDBAbsent(ctx, w.session, absence, m.normalTimestamp()); err != nil {
		return absence, err
	}
	if err := runtime.RemoveStoppedContainer(ctx, w.session.RunnerID); err != nil {
		return absence, err
	}
	if err := q.RetireLiquibaseSessionPrivateCapture(ctx, w.session, absence, m.normalTimestamp()); err != nil {
		return absence, err
	}
	// Fresh exact absence precedes local file deletion, not only a saved time.
	if err := absence.VerifyReleasedLiquibaseSession(ctx, w.session, w.clone, w.rt.instance.ID); err != nil {
		return absence, err
	}
	if w.rt.cleanup != nil {
		if err := w.rt.cleanup(); err != nil {
			return absence, err
		}
	}
	if err := w.protected.Close(); err != nil {
		return absence, err
	}
	if err := os.Remove(w.helper); err != nil && !os.IsNotExist(err) {
		return absence, err
	}
	return absence, nil
}

func (m *PrepareService) publishNormalLiquibaseInstance(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, runner *jobRunner, jobID string, p preparedRequest, stateID string) error {
	if err := m.updateTaskStatus(ctx, jobID, "prepare-instance", StatusRunning, strPtr(m.normalTimestamp()), nil, nil); err != nil {
		return err
	}
	rt, problem := (&taskExecutor{m: m}).startManagedRuntime(ctx, jobID, p, &TaskInput{Kind: "state", ID: stateID})
	if problem != nil {
		return fmt.Errorf("result instance unavailable: %s", problem.Message)
	}
	runner.setRuntime(rt)
	connection, err := m.managedConnection(ctx, rt)
	if err != nil {
		return err
	}
	instanceID := managedOperationRef("instance", jobID)[:32]
	var intent queue.FinalizationSnapshot
	var publicationError error
	err = m.access.Activate(ctx, instanceID, rt.instance.Binding, func(secret instanceaccess.Secret) error {
		return dbms.EnsureManagedPassword(ctx, connection, secret.Password)
	}, func() error {
		active := store.InstanceStatusActive
		var err error
		intent, err = q.PublishInstanceAndBeginFinalization(ctx, jobID, store.InstanceCreate{InstanceID: instanceID, StateID: stateID, ImageID: p.effectiveImageID(), CreatedAt: m.normalTimestamp(), RuntimeID: &rt.instance.ID, RuntimeDir: &rt.runtimeDir, Status: &active}, rt.instance.Binding, rt.operation)
		publicationError = err
		return err
	})
	if err != nil {
		if publicationError != nil {
			return publicationError
		}
		return err
	}
	return m.finishNormalLiquibase(ctx, q, intent)
}
