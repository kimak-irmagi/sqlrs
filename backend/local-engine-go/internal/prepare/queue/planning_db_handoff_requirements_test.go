package queue_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// PG02 preparation control: prove the old, supported writers actually reach a
// stopped successful planner over a live managed DB, before testing new writers.
func TestPlanningDBFixture(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	saved, found, err := f.q.LoadPlanningAttempt(t.Context(), f.attempt.JobID, 1)
	if err != nil || !found {
		t.Fatal("actual planning fixture", err)
	}
	handoffCheck(t, f.observe(t.Context(), saved, f.op))
	if f.count(t, "SELECT count(*) FROM prepare_attempt_evidence WHERE job_id=? AND kind='db_quiescence'", f.attempt.JobID) != 0 || f.count(t, "SELECT count(*) FROM prepare_liquibase_sessions WHERE job_id=?", f.attempt.JobID) != 0 {
		t.Fatal("fixture invented DB absence or a working session")
	}
}

// PG02: success is one exact durable plan/ownership/decision, not a released
// clone or an in-memory permission to launch SQL. Schema §§2–3.
func TestCompleteRetainedPlanning(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	if f.observationCalls != 1 || !(f.exit < f.retained && f.retained < f.output) {
		t.Fatal("missing exact live DB observation or wrong evidence order")
	}
	saved, found, err := f.q.LoadPlanningAttempt(t.Context(), f.attempt.JobID, 1)
	if err != nil || !found || saved.Attempt.Phase != "closed" || saved.Attempt.Resolution != "planning_result_saved" || !saved.PlanSaved || !saved.OutputComplete || saved.DecisionSaved || saved.ClonePhase != "active" || saved.CloneRuntimeID != f.op.RuntimeRef {
		t.Fatalf("retained planning snapshot: %+v, %v", saved, err)
	}
	plan, found, err := f.q.LoadPlanningResult(t.Context(), f.attempt.JobID)
	if err != nil || !found || plan.PlanDigest != f.planDigest || !reflect.DeepEqual(plan.Changesets, f.changes) {
		t.Fatal("saved plan changed", err)
	}
	if f.count(t, `SELECT count(*) FROM prepare_liquibase_planning_db_ownership
WHERE job_id=? AND task_id='plan' AND attempt_no=1 AND clone_id=? AND retained_evidence_id=? AND plan_digest=?
AND session_id IS NULL AND first_task_id IS NULL AND first_execution_digest IS NULL AND transferred_at IS NULL
AND disposal_requested_evidence_id IS NULL AND disposal_evidence_id IS NULL AND disposed_at IS NULL`, f.attempt.JobID, f.clone.ID, f.retained, f.planDigest) != 1 {
		t.Fatal("retained ownership is not exact/all-untransferred")
	}
	if f.count(t, `SELECT count(*) FROM prepare_decision_evidence link JOIN prepare_attempt_decisions d USING(decision_id)
WHERE d.job_id=? AND d.choice='complete_planning' AND link.evidence_id IN (?,?,?)`, f.attempt.JobID, f.exit, f.retained, f.output) != 3 || f.count(t, `SELECT count(*) FROM prepare_decision_evidence link JOIN prepare_attempt_decisions d USING(decision_id) WHERE d.job_id=?`, f.attempt.JobID) != 3 {
		t.Fatal("completion does not use exactly exit/retention/output")
	}
	if f.count(t, "SELECT count(*) FROM prepare_tasks WHERE job_id=? AND task_id='plan' AND status='succeeded'", f.attempt.JobID) != 1 || f.count(t, "SELECT count(*) FROM prepare_attempt_evidence WHERE job_id=? AND kind='db_quiescence'", f.attempt.JobID) != 0 || f.count(t, "SELECT count(*) FROM prepare_step_attempts WHERE job_id=? AND task_id<>'plan'", f.attempt.JobID) != 0 {
		t.Fatal("completion invented DB absence, SQL or omitted the stable task")
	}
	var raw, factsDigest, scopeDigest string
	handoffCheck(t, f.db.QueryRow("SELECT facts_json,facts_digest,scope_digest FROM prepare_attempt_evidence WHERE evidence_id=? AND job_id=? AND kind='planning_db_retained' AND worker_generation=1", f.retained, f.attempt.JobID).Scan(&raw, &factsDigest, &scopeDigest))
	hash := func(text string) string {
		sum := sha256.Sum256([]byte(text))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	if factsDigest != hash(raw) || scopeDigest != hash(fmt.Sprintf("attempt-scope-v1\n%s\nplan\n1\n%s\n%s", f.attempt.JobID, f.clone.ID, f.attempt.ExecutionDigest)) {
		t.Fatal("retention fact is not independently hash/scope bound")
	}
	var facts struct {
		CloneID     string                     `json:"clone_id"`
		Execution   string                     `json:"execution_digest"`
		Observation map[string]json.RawMessage `json:"observation"`
	}
	handoffCheck(t, json.Unmarshal([]byte(raw), &facts))
	if facts.CloneID != f.clone.ID || facts.Execution != f.attempt.ExecutionDigest || len(facts.Observation) == 0 {
		t.Fatal("retention lacks its exact attempt scope")
	}
	// Immutable metadata reconciliation must not append a second owner/decision.
	before := mainPathJournalRows(t, f.db)
	handoffCheck(t, f.writer(t).CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
	f.unchanged(t, before)
}

// PG02 validates each caller identity independently; a same-phase valid control
// uses the unchanged persisted tuple after every refusal.
func TestPlanningDBRetentionRejectsWrongObservation(t *testing.T) {
	for _, variant := range []string{"job", "clone", "generation", "execution", "executor", "runtime", "target", "source", "physical", "identity", "operation", "stage", "nil-observer", "observer-refusal", "invalid-time"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			w := f.writer(t)
			a, generation, op, at := f.attempt, 1, f.op, mainPathJournalUTC
			observe := f.observe
			denied := errors.New("trusted DB observation refused")
			switch variant {
			case "job":
				a.JobID = "other-job"
			case "clone":
				a.CloneID = "other-clone"
			case "generation":
				generation = 2
			case "execution":
				a.ExecutionDigest = pgDigest("f")
			case "executor":
				a.ExecutorID = strings.Repeat("f", 64)
			case "runtime":
				op.RuntimeRef = strings.Repeat("f", 64)
			case "target":
				op.Target += "-other"
			case "source":
				op.Source = pgDigest("f")
			case "physical":
				op.PhysicalIdentity = "physical_other"
			case "identity":
				op.Identity.IdentityDigest = pgDigest("f")
			case "operation":
				op.Ref = "runtime_other"
			case "stage":
				op.Stage = "retired"
			case "nil-observer":
				observe = nil
			case "observer-refusal":
				observe = func(context.Context, queue.PlanningRecoverySnapshot, instanceaccess.Operation) error {
					f.observationCalls++
					return denied
				}
			case "invalid-time":
				at = "2026-11-13T12:00:00+03:00"
			}
			before := mainPathJournalRows(t, f.db)
			id, err := w.RecordPlanningDBRetention(t.Context(), a, generation, op, observe, at)
			if err == nil || id != 0 {
				t.Fatal("wrong retention observation accepted", id, err)
			}
			if variant == "observer-refusal" && f.observationCalls != 1 {
				t.Fatal("negative case did not reach its exact observer")
			}
			f.unchanged(t, before)
			_, err = w.RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC)
			handoffCheck(t, err)
		})
	}
}

// PG02: exact evidence identity/order and eligibility, not merely three positive
// integers. Rejection must preserve every table, including foreign-job rows.
func TestCompleteRetainedPlanningRejectsWrongProof(t *testing.T) {
	for _, variant := range []string{"missing-exit", "missing-retention", "missing-output", "retention-is-exit", "output-before-retention", "wrong-clone", "wrong-generation", "zero-plan", "recovery", "accepted-cancel", "invalid-time"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			if variant == "zero-plan" {
				f.changes = nil
			}
			f.saveRetainedPlan(t)
			a, generation, exit, retained, output, at := f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC
			switch variant {
			case "missing-exit":
				exit = 0
			case "missing-retention":
				retained = 0
			case "missing-output":
				output = 0
			case "retention-is-exit":
				retained = exit
			case "output-before-retention":
				retained, output = output, retained
			case "wrong-clone":
				a.CloneID = "other-clone"
			case "wrong-generation":
				generation = 2
			case "recovery":
				status := "recovery_required"
				handoffCheck(t, f.q.UpdateJob(t.Context(), a.JobID, queue.JobUpdate{Status: &status}))
			case "accepted-cancel":
				_, err := f.q.RequestJobCancellation(t.Context(), a.JobID, mainPathJournalUTC)
				handoffCheck(t, err)
			case "invalid-time":
				at = "2026-02-31T12:00:00Z"
			}
			before := mainPathJournalRows(t, f.db)
			if err := f.writer(t).CompleteRetainedPlanning(t.Context(), a, generation, exit, retained, output, at); err == nil {
				t.Fatal("invalid retained completion accepted")
			}
			f.unchanged(t, before)
			if f.count(t, "SELECT count(*) FROM prepare_liquibase_planning_db_ownership WHERE job_id=?", f.attempt.JobID) != 0 {
				t.Fatal("refusal installed an owner")
			}
		})
	}
}

func TestPlanningDBRetentionRejectsFailedPlanner(t *testing.T) {
	f := newPlanningDBFixture(t, 1)
	before := mainPathJournalRows(t, f.db)
	if id, err := f.writer(t).RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC); err == nil || id != 0 {
		t.Fatal("failed planner authorised DB reuse", id, err)
	}
	f.unchanged(t, before)
}

// PG02 keeps plan-only and cache explanation on the original absence-based
// cleanup route. They must not accidentally acquire the new destructive owner.
func TestPlanningDBRetentionRejectsNonPrepare(t *testing.T) {
	for _, purpose := range []string{"plan-only", "cache_explain"} {
		t.Run(purpose, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0, func(job *queue.JobRecord) {
				job.PlanOnly = true
				if purpose == "cache_explain" {
					job.Purpose = purpose
				}
			})
			before := mainPathJournalRows(t, f.db)
			if id, err := f.writer(t).RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC); err == nil || id != 0 {
				t.Fatal("non-prepare acquired retained ownership", id, err)
			}
			f.unchanged(t, before)
		})
	}
}

// PG05: private bytes and the live DB have deliberately different owners.
func TestRetainedPlanningCaptureDisposal(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	probe := f.retireCapture(t)
	if probe.executor != 1 || probe.inputCalls != 1 || probe.cloneCalls != 0 {
		t.Fatal("private capture cleanup used DB deletion proof", probe)
	}
	before := mainPathJournalRows(t, f.db)
	if err := f.q.ReleasePlanningClone(t.Context(), f.attempt, 1, probe); err == nil {
		t.Fatal("planning cleanup released the retained live DB")
	}
	f.unchanged(t, before)
	if probe.cloneCalls != 0 {
		t.Fatal("forbidden DB release reached physical cleanup observation")
	}
	if f.count(t, "SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase='active' AND db_runtime_id=?", f.clone.ID, f.op.RuntimeRef) != 1 || f.count(t, "SELECT count(*) FROM managed_runtime_operations WHERE operation_ref=? AND stage='running' AND runtime_ref=?", f.op.Ref, f.op.RuntimeRef) != 1 {
		t.Fatal("private output cleanup destroyed or retired the live DB")
	}
}

// PG03: one physical allocation with exactly the same persisted identity.
func TestPlanningDBFirstSessionTransfer(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	probe := f.retireCapture(t)
	launch := f.firstLaunch(t)
	w, ok := any(f.q).(planningDBTransferWriter)
	if !ok {
		t.Fatal("approved first-session transfer writer is not implemented")
	}
	var opBefore string
	handoffCheck(t, f.db.QueryRow("SELECT json_array(operation_ref,target_path,source_ref,identity_json,physical_identity,runtime_ref,stage) FROM managed_runtime_operations WHERE operation_ref=?", f.op.Ref).Scan(&opBefore))
	session, err := w.TransferPlanningDBToFirstSession(t.Context(), launch)
	handoffCheck(t, err)
	if session.CloneID != f.clone.ID || session.WorkerGeneration != 1 || session.RunnerPhase != "planned" || session.DBPhase != "running" || session.CursorID != f.clone.SourceImageID || session.CursorPosition != 1 {
		t.Fatalf("transfer created the wrong first session: %+v", session)
	}
	if f.count(t, "SELECT count(*) FROM prepare_clones WHERE job_id=?", f.attempt.JobID) != 1 || f.count(t, "SELECT count(*) FROM prepare_clones WHERE job_id=? AND clone_id=? AND clone_generation=1 AND clone_ref=? AND db_runtime_id=? AND phase='active'", f.attempt.JobID, f.clone.ID, f.clone.Ref, f.op.RuntimeRef) != 1 {
		t.Fatal("transfer allocated another clone/generation or changed the live DB")
	}
	if f.count(t, `SELECT count(*) FROM prepare_liquibase_planning_db_ownership WHERE job_id=? AND session_id=? AND first_task_id=? AND first_execution_digest=? AND transferred_at=? AND disposal_requested_evidence_id IS NULL`, f.attempt.JobID, session.SessionID, launch.FirstTaskID, launch.ExecutionDigest, mainPathJournalUTC) != 1 {
		t.Fatal("missing exact durable transfer")
	}
	var opAfter string
	handoffCheck(t, f.db.QueryRow("SELECT json_array(operation_ref,target_path,source_ref,identity_json,physical_identity,runtime_ref,stage) FROM managed_runtime_operations WHERE operation_ref=?", f.op.Ref).Scan(&opAfter))
	if opBefore != opAfter {
		t.Fatal("transfer rewrote managed operation identity or Source")
	}
	before := mainPathJournalRows(t, f.db)
	if _, err := f.q.PlanLiquibaseSessionInvocation(t.Context(), session, launch.FirstTaskID, launch.ExecutionDigest, mainPathInvocationPlan("premature-retained-sql")); err == nil {
		t.Fatal("transfer bypassed runner readiness and admitted SQL")
	}
	f.unchanged(t, before)
	if err := f.q.ReleasePlanningClone(t.Context(), f.attempt, 1, probe); err == nil {
		t.Fatal("historical planner reclaimed the transferred DB")
	}
	f.unchanged(t, before)
	if probe.cloneCalls != 0 {
		t.Fatal("old planner reached DB cleanup after transfer")
	}
	launch.Session.SessionID = "another-retained-session"
	launch.Session.PlannedRunnerName = "sqlrs-lb-another-retained-session"
	if _, err := w.TransferPlanningDBToFirstSession(t.Context(), launch); err == nil {
		t.Fatal("second transfer succeeded")
	}
	f.unchanged(t, before)
}

// PG01/PG03: the future-session FK really fails at COMMIT, rather than an earlier
// unrelated trigger. Explicit SQL transaction ownership lets the test roll back
// the failed COMMIT on the same physical connection before checking the journal.
func TestPlanningDBTransferDeferredFK(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	f.retireCapture(t)
	launch := f.firstLaunch(t)
	before := mainPathJournalRows(t, f.db)
	conn, err := f.db.Conn(t.Context())
	handoffCheck(t, err)
	_, err = conn.ExecContext(t.Context(), "BEGIN IMMEDIATE")
	handoffCheck(t, err)
	_, err = conn.ExecContext(t.Context(), `UPDATE prepare_liquibase_planning_db_ownership SET session_id=?,first_task_id=?,first_execution_digest=?,transferred_at=? WHERE job_id=?`, launch.Session.SessionID, launch.FirstTaskID, launch.ExecutionDigest, mainPathJournalUTC, f.attempt.JobID)
	if err != nil {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		_ = conn.Close()
		t.Fatal("valid future link was rejected before deferred FK COMMIT", err)
	}
	_, commitErr := conn.ExecContext(t.Context(), "COMMIT")
	var code interface{ Code() int }
	if !errors.As(commitErr, &code) || code.Code() != 787 {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		_ = conn.Close()
		t.Fatal("missing future session did not fail with SQLITE_CONSTRAINT_FOREIGNKEY at COMMIT", commitErr)
	}
	_, err = conn.ExecContext(t.Context(), "ROLLBACK")
	handoffCheck(t, err)
	handoffCheck(t, conn.Close())
	f.unchanged(t, before)
	w, ok := any(f.q).(planningDBTransferWriter)
	if !ok {
		t.Fatal("approved first-session transfer writer is not implemented")
	}
	_, err = w.TransferPlanningDBToFirstSession(t.Context(), launch)
	handoffCheck(t, err)
	if f.count(t, "SELECT count(*) FROM prepare_liquibase_sessions WHERE job_id=?", f.attempt.JobID) != 1 {
		t.Fatal("same-phase matching future session did not commit")
	}
}
