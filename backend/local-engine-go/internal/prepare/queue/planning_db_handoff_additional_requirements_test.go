package queue_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// PG02 compatibility control distinguishes the attempt's complete_planning
// decision from the cache-explanation projection's DecisionSaved flag. Both
// are existing public meanings used by the recovery classifier.
func TestPlanningDBLegacyCompletionSnapshotControl(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	quiet, err := f.q.RecordPlanningCloneQuiescence(t.Context(), f.attempt, 1, f.clone.PlannedRuntimeName, f.op.RuntimeRef, mainPathJournalUTC)
	handoffCheck(t, err)
	output, err := f.q.RecordVerifiedPlanningCapture(t.Context(), f.attempt, 1, queue.PlanningCaptureProof{StdoutDigest: pgDigest("2"), StderrDigest: pgDigest("3"), StdoutBytes: 100, ManifestRef: f.capture.Manifest}, mainPathJournalUTC)
	handoffCheck(t, err)
	_, err = f.q.SavePlanningResult(t.Context(), f.attempt.JobID, 1, pgDigest("2"), f.changes, mainPathJournalUTC)
	handoffCheck(t, err)
	handoffCheck(t, f.q.CompletePlanning(t.Context(), f.attempt, 1, f.exit, quiet, output, mainPathJournalUTC))
	saved, found, err := f.q.LoadPlanningAttempt(t.Context(), f.attempt.JobID, 1)
	if err != nil || !found || !saved.PlanSaved || saved.DecisionSaved {
		t.Fatal("ordinary completion was confused with a cache-explanation decision", err)
	}
	if f.count(t, "SELECT count(*) FROM prepare_attempt_decisions WHERE job_id=? AND choice='complete_planning'", f.attempt.JobID) != 1 {
		t.Fatal("ordinary completion decision missing")
	}
}

// PG06 uses checkpoints produced by ordinary publication, including their
// execution/adapter provenance, rather than substituting arbitrary cached IDs.
func planningDBPublishCache(t *testing.T, f *planningDBFixture) []string {
	t.Helper()
	s, found, err := f.q.ReadLiquibaseSession(t.Context(), "main-path-session")
	if err != nil || !found {
		t.Fatal("cache producer session missing", err)
	}
	states := []string{strings.Repeat("2", 64), strings.Repeat("3", 64), strings.Repeat("4", 64)}
	for i, task := range []string{"execute", "execute-next", "execute-third"} {
		t.Logf("cache producer: admit %s", task)
		invocation := mainPathNextInvocation(t, f.q, s, pgDigest("9"), task, "cache-producer-"+task)
		if invocation.PriorRetirementRef != nil {
			plan, err := f.q.ReadLiquibaseCapturePrunePlan(t.Context(), s, invocation)
			handoffCheck(t, err)
			if plan == nil {
				t.Fatal("cache producer lost its prior capture authority")
			}
			handoffCheck(t, f.q.ConfirmLiquibaseCapturePruned(t.Context(), s, invocation, &mainPathPruneProbe{db: f.db, want: *plan}, mainPathJournalUTC))
		}
		t.Logf("cache producer: publish %s", task)
		s = mainPathPublishStep(t, f.db, f.q, s, invocation, states[i])
	}
	return states
}

func TestRetainedPlanningDBCachedNonReuse(t *testing.T) {
	for _, reason := range []string{"source_mismatch", "full_cache"} {
		t.Run(reason, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			states := planningDBPublishCache(t, f)
			f.complete(t)
			f.retireCapture(t)
			launch := f.firstLaunchWithStates(t, states)
			cached, succeeded := true, "succeeded"
			handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, "one", queue.TaskUpdate{Status: &succeeded, Cached: &cached}))
			binding := queue.ExecutionBinding{Digest: pgDigest("9"), PrepareKind: "lb", CommandDigest: pgDigest("9"), InputManifestDigest: f.record.BundleManifestDigest, ExecutorImageDigest: f.record.ImageManifestDigest, TargetIdentityDigest: f.record.TargetIdentityDigest, PolicyRevision: "lb-container-v2", EffectScope: "unknown", AdapterBindingDigest: f.record.ExecutionFingerprint}
			for _, task := range []string{"two", "three"} {
				handoffCheck(t, f.q.BindStep(t.Context(), f.attempt.JobID, task, binding, mainPathJournalUTC))
			}
			if reason == "full_cache" {
				for _, task := range []string{"two", "three"} {
					handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, task, queue.TaskUpdate{Status: &succeeded, Cached: &cached}))
				}
			} else {
				running := "running"
				handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, "two", queue.TaskUpdate{Status: &running}))
				launch.FirstTaskID = "two"
				launch.Session.StartKind = "state"
				launch.Session.StartID = states[0]
				launch.Session.StartPosition = 2
				// Keep the actual preview identity. A source-compatible cursor is
				// not permission to rewrite its immutable image source to SQL1.
			}
			transfer, ok := any(f.q).(planningDBTransferWriter)
			if !ok {
				t.Fatal("approved transfer writer missing")
			}
			before := mainPathJournalRows(t, f.db)
			if _, err := transfer.TransferPlanningDBToFirstSession(t.Context(), launch); err == nil {
				t.Fatal("cache path transferred incompatible planning DB")
			}
			f.unchanged(t, before)
			w, ok := any(f.q).(planningDBDisposalWriter)
			if !ok {
				t.Fatal("approved disposal writer missing")
			}
			request, err := w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, reason, mainPathJournalUTC)
			handoffCheck(t, err)
			if request <= f.retained || f.count(t, "SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase='active'", f.clone.ID) != 1 {
				t.Fatal("request invented absence")
			}
			handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
			f.op.Stage = "retired"
			handoffCheck(t, w.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, &planningDBAbsence{f: f, request: request}, mainPathJournalUTC))
			if f.count(t, "SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase='released'", f.clone.ID) != 1 || f.count(t, "SELECT count(*) FROM prepare_liquibase_sessions WHERE job_id=?", f.attempt.JobID) != 0 {
				t.Fatal("cache cleanup lost exact owner or created a session")
			}
		})
	}
}

// The cache producer is independently executable while O5 is still red. This
// catches fixture failures before they can be mistaken for a missing O5 writer.
func TestPlanningDBCacheFixture(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	states := planningDBPublishCache(t, f)
	for _, state := range states {
		fingerprint, found, err := f.q.GetLiquibaseStateExecution(t.Context(), state)
		if err != nil || !found || fingerprint != f.record.ExecutionFingerprint {
			t.Fatal("real cache checkpoint provenance missing", err)
		}
	}
}

// PG02 reaches a genuinely live, persisted planner rather than changing only
// the caller's phase string. Refusal is followed by a real successful exit and
// an otherwise identical positive retention observation.
func TestPlanningDBRetentionRejectsLivePlanner(t *testing.T) {
	f := newPlanningDBFixtureAtExit(t, nil)
	w := f.writer(t)
	before := mainPathJournalRows(t, f.db)
	if id, err := w.RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC); err == nil || id != 0 {
		t.Fatal("live planner allowed retention")
	}
	f.unchanged(t, before)
	var err error
	f.exit, err = f.q.ObserveAttemptExit(t.Context(), f.attempt, 1, f.attempt.ExecutorID, 0, mainPathJournalUTC)
	handoffCheck(t, err)
	_, err = w.RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC)
	handoffCheck(t, err)
}

type planningDBCaptureRefusal struct {
	probe  *mainPathPreviewAbsence
	refuse bool
	calls  int
}

func (v *planningDBCaptureRefusal) VerifyPlanningArtifactAbsent(ctx context.Context, kind, ref, id string) error {
	if err := v.probe.VerifyPlanningArtifactAbsent(ctx, kind, ref, id); err != nil {
		return err
	}
	v.calls++
	if v.refuse {
		return errors.New("exact child remains live or its inspection is unavailable")
	}
	return nil
}

// PG05 proves the private cleanup verifier is still mandatory with a retained
// live DB. Wrong linkage cannot reach it, and inspection failure cannot retire
// any bytes; removing only that refusal permits the same cleanup to complete.
func TestRetainedPlanningCaptureDisposalRejectsUnprovedAbsence(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	_, err := f.q.SavePrivatePlanningCaptureDisposal(t.Context(), f.attempt, 1, "plan_saved", f.planDigest, mainPathJournalUTC)
	handoffCheck(t, err)
	v := &planningDBCaptureRefusal{probe: &mainPathPreviewAbsence{db: f.db, attempt: f.attempt, clone: f.clone, dbID: f.op.RuntimeRef, input: f.capture.ProtectedInput}, refuse: true}
	before := mainPathJournalRows(t, f.db)
	bad := f.attempt
	bad.CloneID = "other-clone"
	if err := f.q.RetirePrivatePlanningCapture(t.Context(), bad, 1, v, mainPathJournalUTC); err == nil || v.calls != 0 {
		t.Fatal("foreign cleanup reached physical observation", err)
	}
	f.unchanged(t, before)
	if err := f.q.RetirePrivatePlanningCapture(t.Context(), f.attempt, 1, v, mainPathJournalUTC); err == nil || v.calls != 1 {
		t.Fatal("unproved child absence was accepted or not reached", err)
	}
	f.unchanged(t, before)
	v.refuse = false
	handoffCheck(t, f.q.RetirePrivatePlanningCapture(t.Context(), f.attempt, 1, v, mainPathJournalUTC))
	handoffCheck(t, f.q.RetirePlanningArtifact(t.Context(), f.attempt, 1, "input", v, mainPathJournalUTC))
	if f.count(t, "SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase='active' AND db_runtime_id=?", f.clone.ID, f.op.RuntimeRef) != 1 || f.count(t, "SELECT count(*) FROM prepare_attempt_evidence WHERE job_id=? AND kind='db_quiescence'", f.attempt.JobID) != 0 {
		t.Fatal("private cleanup invented DB absence")
	}
}

// PG01/PG07 exercise the relational transitions independently of queue writer
// validation. Every refused statement must preserve the complete journal.
// Requirements: issue89-planning-db-handoff-schema.md §§1, 4–5.
func TestPlanningDBOwnershipTransitionConstraints(t *testing.T) {
	for _, phase := range []string{"retained", "transferred", "disposal-pending", "disposed"} {
		t.Run(phase, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			f.retireCapture(t)
			l := f.firstLaunch(t)
			if phase == "transferred" {
				w, ok := any(f.q).(planningDBTransferWriter)
				if !ok {
					t.Fatal("approved transfer writer missing")
				}
				_, err := w.TransferPlanningDBToFirstSession(t.Context(), l)
				handoffCheck(t, err)
			}
			if phase == "disposal-pending" || phase == "disposed" {
				_, err := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
				handoffCheck(t, err)
				w, ok := any(f.q).(planningDBDisposalWriter)
				if !ok {
					t.Fatal("approved disposal writer missing")
				}
				request, err := w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
				handoffCheck(t, err)
				if phase == "disposed" {
					handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
					f.op.Stage = "retired"
					handoffCheck(t, w.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, &planningDBAbsence{f: f, request: request}, mainPathJournalUTC))
				}
			}
			for _, assignment := range []string{
				"job_id='main-path-job'", "task_id='one'", "attempt_no=2",
				"clone_id='main-path-clone'", "retained_evidence_id=1",
				"plan_digest='sha256:BAD'", "session_id='other-session'",
				"first_task_id='two'", "first_execution_digest=NULL",
				"transferred_at='2026-02-31T12:00:00Z'",
				"disposal_requested_evidence_id=1", "disposal_evidence_id=1",
				"disposed_at='2026-11-13T12:00:00+03:00'",
			} {
				t.Run(assignment, func(t *testing.T) {
					// A no-op is not a new transition; test only actual changes.
					if assignment == "first_execution_digest=NULL" && phase != "transferred" {
						return
					}
					before := mainPathJournalRows(t, f.db)
					_, err := f.db.Exec("UPDATE prepare_liquibase_planning_db_ownership SET "+assignment+" WHERE job_id=?", f.attempt.JobID)
					if err == nil {
						t.Fatal("invalid ownership transition committed")
					}
					f.unchanged(t, before)
				})
			}
			before := mainPathJournalRows(t, f.db)
			_, err := f.db.Exec(`UPDATE prepare_liquibase_planning_db_ownership SET
job_id=job_id,task_id=task_id,attempt_no=attempt_no,clone_id=clone_id,
retained_evidence_id=retained_evidence_id,plan_digest=plan_digest,
session_id=session_id,first_task_id=first_task_id,first_execution_digest=first_execution_digest,
transferred_at=transferred_at,disposal_requested_evidence_id=disposal_requested_evidence_id,
disposal_evidence_id=disposal_evidence_id,disposed_at=disposed_at WHERE job_id=?`, f.attempt.JobID)
			handoffCheck(t, err)
			f.unchanged(t, before)
		})
	}
}

// PG03/PG06: either persisted disposal intent or transfer wins. Exercise both
// serializations through independently opened SQLite handles; never fake an
// ownership row or allow physical cleanup before the winning request commits.
func TestPlanningDBTransferDisposalExclusive(t *testing.T) {
	for _, winner := range []string{"transfer", "disposal"} {
		t.Run(winner, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			f.retireCapture(t)
			l := f.firstLaunch(t)
			var file string
			handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
			peer, err := sql.Open("sqlite", file)
			handoffCheck(t, err)
			defer peer.Close()
			handoffCheck(t, execPlanningDBPeerPragmas(t.Context(), peer))
			q, err := queue.New(peer)
			handoffCheck(t, err)
			transfer, ok := any(q).(planningDBTransferWriter)
			if !ok {
				t.Fatal("approved transfer writer missing")
			}
			dispose, ok := any(q).(planningDBDisposalWriter)
			if !ok {
				t.Fatal("approved disposal writer missing")
			}
			if winner == "transfer" {
				_, err = transfer.TransferPlanningDBToFirstSession(t.Context(), l)
				handoffCheck(t, err)
				_, err = f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
				handoffCheck(t, err)
				before := mainPathJournalRows(t, f.db)
				if _, err := dispose.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC); err == nil {
					t.Fatal("planning disposal reclaimed transferred DB")
				}
				f.unchanged(t, before)
			} else {
				_, err = f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
				handoffCheck(t, err)
				_, err = dispose.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
				handoffCheck(t, err)
				before := mainPathJournalRows(t, f.db)
				if _, err := transfer.TransferPlanningDBToFirstSession(t.Context(), l); err == nil {
					t.Fatal("transfer stole disposal-pending DB")
				}
				f.unchanged(t, before)
			}
		})
	}
}

// PG01 checks malformed transfer times with all four transition fields present,
// so all-or-none validation or the future-session FK cannot mask a bad date.
// The existing deferred-FK test is the otherwise identical positive UPDATE.
func TestPlanningDBTransferInvalidUTC(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	f.retireCapture(t)
	l := f.firstLaunch(t)
	for _, at := range []string{"", "2026-02-31T12:00:00Z", "2026-11-13T12:00:00+03:00", "2026-11-13T25:00:00Z"} {
		before := mainPathJournalRows(t, f.db)
		tx, err := f.db.BeginTx(t.Context(), nil)
		handoffCheck(t, err)
		_, err = tx.Exec(`UPDATE prepare_liquibase_planning_db_ownership SET session_id=?,first_task_id=?,first_execution_digest=?,transferred_at=? WHERE job_id=?`, l.Session.SessionID, l.FirstTaskID, l.ExecutionDigest, at, f.attempt.JobID)
		handoffCheck(t, tx.Rollback())
		if err == nil {
			t.Fatal("malformed transfer time passed its UPDATE boundary", at)
		}
		f.unchanged(t, before)
	}
}

func execPlanningDBPeerPragmas(ctx context.Context, db *sql.DB) error {
	db.SetMaxOpenConns(1)
	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=0; PRAGMA read_uncommitted=0")
	return err
}

// PG02/PG03/PG06: a saved outcome blocks new execution but cannot strand a
// previously retained DB or redirect a previously committed cleanup request.
func TestPlanningDBSelectedFailureCleanup(t *testing.T) {
	for _, timing := range []string{"before-retention", "before-transfer", "after-request"} {
		t.Run(timing, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			var l queue.LiquibaseSessionLaunch
			var request int64
			var states []string
			if timing == "after-request" {
				states = planningDBPublishCache(t, f)
			}
			if timing != "before-retention" {
				f.complete(t)
				f.retireCapture(t)
				if timing == "after-request" {
					l = f.firstLaunchWithStates(t, states)
					cached, succeeded := true, "succeeded"
					binding := queue.ExecutionBinding{Digest: pgDigest("9"), PrepareKind: "lb", CommandDigest: pgDigest("9"), InputManifestDigest: f.record.BundleManifestDigest, ExecutorImageDigest: f.record.ImageManifestDigest, TargetIdentityDigest: f.record.TargetIdentityDigest, PolicyRevision: "lb-container-v2", EffectScope: "unknown", AdapterBindingDigest: f.record.ExecutionFingerprint}
					for _, task := range []string{"one", "two", "three"} {
						handoffCheck(t, f.q.BindStep(t.Context(), f.attempt.JobID, task, binding, mainPathJournalUTC))
						handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, task, queue.TaskUpdate{Status: &succeeded, Cached: &cached}))
					}
				} else {
					l = f.firstLaunch(t)
				}
			} else {
				// Earlier selected failure uses the existing completed/absent
				// planning route; O5 does not invent pre-plan outcome admission.
				quiet, err := f.q.RecordPlanningCloneQuiescence(t.Context(), f.attempt, 1, f.clone.PlannedRuntimeName, f.op.RuntimeRef, mainPathJournalUTC)
				handoffCheck(t, err)
				f.output, err = f.q.RecordVerifiedPlanningCapture(t.Context(), f.attempt, 1, queue.PlanningCaptureProof{StdoutDigest: pgDigest("2"), StderrDigest: pgDigest("3"), StdoutBytes: 100, ManifestRef: f.capture.Manifest}, mainPathJournalUTC)
				handoffCheck(t, err)
				f.planDigest, err = f.q.SavePlanningResult(t.Context(), f.attempt.JobID, 1, pgDigest("2"), f.changes, mainPathJournalUTC)
				handoffCheck(t, err)
				handoffCheck(t, f.q.CompletePlanning(t.Context(), f.attempt, 1, f.exit, quiet, f.output, mainPathJournalUTC))
				probe := f.retireCapture(t)
				handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
				f.op.Stage = "retired"
				handoffCheck(t, f.q.ReleasePlanningClone(t.Context(), f.attempt, 1, probe))
			}
			w, ok := any(f.q).(planningDBDisposalWriter)
			if !ok {
				t.Fatal("approved disposal writer missing")
			}
			if timing == "after-request" {
				var err error
				request, err = w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "full_cache", mainPathJournalUTC)
				handoffCheck(t, err)
			}
			_, err := f.q.SelectLiquibaseJobFailure(t.Context(), f.attempt.JobID, handoffFailure(), mainPathJournalUTC)
			handoffCheck(t, err)
			before := mainPathJournalRows(t, f.db)
			if timing == "before-retention" {
				if id, err := f.writer(t).RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC); err == nil || id != 0 {
					t.Fatal("selected failure allowed new retention")
				}
				f.unchanged(t, before)
				return
			}
			transfer, ok := any(f.q).(planningDBTransferWriter)
			if !ok {
				t.Fatal("approved transfer writer missing")
			}
			if _, err := transfer.TransferPlanningDBToFirstSession(t.Context(), l); err == nil {
				t.Fatal("selected failure allowed new session")
			}
			f.unchanged(t, before)
			if timing == "before-transfer" {
				request, err = w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "selected_failure", mainPathJournalUTC)
				handoffCheck(t, err)
			} else {
				// The earlier full-cache request remains the sole authority;
				// cancellation/failure precedence is not changed by this fixture.
				if id, err := w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "selected_failure", mainPathJournalUTC); err == nil || id != 0 {
					t.Fatal("late failure replaced original disposal authority")
				}
				f.unchanged(t, before)
			}
			handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
			f.op.Stage = "retired"
			handoffCheck(t, w.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, &planningDBAbsence{f: f, request: request}, mainPathJournalUTC))
			if f.count(t, "SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase='released'", f.clone.ID) != 1 {
				t.Fatal("selected failure stranded exact old owner")
			}
		})
	}
}

// PG03 also rejects changes to persisted eligibility, not just caller fields.
func TestPlanningDBTransferRejectsPersistedIneligibility(t *testing.T) {
	for _, variant := range []string{"recovery", "accepted-cancel", "selected-failure", "task-queued"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			f.retireCapture(t)
			l := f.firstLaunch(t)
			switch variant {
			case "recovery":
				status := "recovery_required"
				handoffCheck(t, f.q.UpdateJob(t.Context(), f.attempt.JobID, queue.JobUpdate{Status: &status}))
			case "accepted-cancel":
				_, err := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
				handoffCheck(t, err)
			case "selected-failure":
				_, err := f.q.SelectLiquibaseJobFailure(t.Context(), f.attempt.JobID, handoffFailure(), mainPathJournalUTC)
				handoffCheck(t, err)
			case "task-queued":
				status := "queued"
				handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, l.FirstTaskID, queue.TaskUpdate{Status: &status}))
			}
			before := mainPathJournalRows(t, f.db)
			w, ok := any(f.q).(planningDBTransferWriter)
			if !ok {
				t.Fatal("approved transfer writer missing")
			}
			if _, err := w.TransferPlanningDBToFirstSession(t.Context(), l); err == nil {
				t.Fatal("persisted ineligibility allowed transfer")
			}
			f.unchanged(t, before)
		})
	}
}

// PG04's durable oracle validates the whole effect independently of the method
// being reconciled. Comparing two equally partial snapshots is insufficient.
func assertPlanningDBCommitEffect(t *testing.T, f *planningDBFixture, writer string, l queue.LiquibaseSessionLaunch, request int64) {
	t.Helper()
	plan, found, err := f.q.LoadPlanningResult(t.Context(), f.attempt.JobID)
	if err != nil || !found || plan.PlanDigest != f.planDigest || !reflect.DeepEqual(plan.Changesets, f.changes) {
		t.Fatal("committed plan incomplete", err)
	}
	saved, found, err := f.q.LoadPlanningAttempt(t.Context(), f.attempt.JobID, 1)
	if err != nil || !found || saved.Attempt.Phase != "closed" || saved.Attempt.Resolution != "planning_result_saved" || !saved.PlanSaved || !saved.OutputComplete || saved.DecisionSaved || saved.CloneRuntimeID != f.op.RuntimeRef {
		t.Fatal("committed attempt/plan incomplete", err)
	}
	if f.count(t, `SELECT count(*) FROM prepare_tasks WHERE job_id=? AND task_id='plan' AND status='succeeded'`, f.attempt.JobID) != 1 || f.count(t, `SELECT count(*) FROM prepare_decision_evidence l JOIN prepare_attempt_decisions d USING(decision_id) WHERE d.job_id=? AND d.choice='complete_planning'`, f.attempt.JobID) != 3 {
		t.Fatal("committed task/decision links incomplete")
	}
	if f.count(t, `SELECT count(*) FROM prepare_attempt_evidence WHERE job_id=? AND evidence_id IN (?,?,?)`, f.attempt.JobID, f.exit, f.retained, f.output) != 3 {
		t.Fatal("committed evidence incomplete")
	}
	var session, task, execution, transferred, disposed sql.NullString
	var requested, proof sql.NullInt64
	var clone, digest string
	var retained int64
	handoffCheck(t, f.db.QueryRow(`SELECT clone_id,retained_evidence_id,plan_digest,session_id,first_task_id,first_execution_digest,transferred_at,disposal_requested_evidence_id,disposal_evidence_id,disposed_at FROM prepare_liquibase_planning_db_ownership WHERE job_id=?`, f.attempt.JobID).Scan(&clone, &retained, &digest, &session, &task, &execution, &transferred, &requested, &proof, &disposed))
	if clone != f.clone.ID || retained != f.retained || digest != f.planDigest {
		t.Fatal("committed ownership identity incomplete")
	}
	if writer == "first-session-transfer" {
		if session.String != l.Session.SessionID || task.String != l.FirstTaskID || execution.String != l.ExecutionDigest || transferred.String != mainPathJournalUTC || requested.Valid || proof.Valid || disposed.Valid {
			t.Fatal("committed transfer incomplete")
		}
		s, found, err := f.q.ReadLiquibaseSession(t.Context(), l.Session.SessionID)
		if err != nil || !found || s.CloneID != f.clone.ID || s.RunnerPhase != "planned" || s.DBPhase != "running" || s.CursorID != f.clone.SourceImageID || s.CursorPosition != 1 {
			t.Fatal("committed session incomplete", err)
		}
		if f.count(t, `SELECT count(*) FROM prepare_artifact_leases WHERE artifact_kind='capture_helper' AND artifact_digest=? AND owner_kind='job' AND owner_id=?`, s.CaptureHelperDigest, f.attempt.JobID) < 1 {
			t.Fatal("committed transfer lost helper lease")
		}
	} else if session.Valid || task.Valid || execution.Valid || transferred.Valid {
		t.Fatal("non-transfer writer invented session ownership")
	}
	phase, stage := "active", "running"
	if writer == "disposal-completion" {
		phase, stage = "released", "retired"
		if requested.Int64 != request || proof.Int64 <= request || disposed.String != mainPathJournalUTC || f.count(t, `SELECT count(*) FROM prepare_attempt_evidence WHERE evidence_id=? AND job_id=? AND kind='planning_db_disposed'`, proof.Int64, f.attempt.JobID) != 1 {
			t.Fatal("committed disposal incomplete")
		}
	} else if requested.Valid || proof.Valid || disposed.Valid {
		t.Fatal("non-disposal writer invented absence")
	}
	if f.count(t, `SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase=? AND db_runtime_id=? AND clone_ref=? AND clone_generation=1`, f.clone.ID, phase, f.op.RuntimeRef, f.clone.Ref) != 1 || f.count(t, `SELECT count(*) FROM managed_runtime_operations WHERE operation_ref=? AND stage=? AND runtime_ref=? AND target_path=? AND source_ref=? AND physical_identity=?`, f.op.Ref, stage, f.op.RuntimeRef, f.op.Target, f.op.Source, f.op.PhysicalIdentity) != 1 {
		t.Fatal("committed clone/operation identity incomplete")
	}
}
