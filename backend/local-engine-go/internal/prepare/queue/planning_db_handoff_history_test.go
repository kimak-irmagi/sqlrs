package queue_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/statefs"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
)

func planningDBStateSource(t *testing.T) *planningDBFixture {
	t.Helper()
	exit := 0
	return newPlanningDBFixtureForSource(t, &exit, func(f *planningDBFixture) string { return planningDBPublishCache(t, f)[2] })
}

// PG07's physical control must precede any deletion of source bytes. A later
// SQLite trigger refusal is insufficient once a real directory is already gone.
// Run the pre-retention case independently while the new writers are absent.
func TestPlanningDBSourceDeletionPreservesBytes(t *testing.T) {
	for _, phase := range []string{"preview", "retained", "disposal-pending"} {
		t.Run(phase, func(t *testing.T) {
			f := planningDBStateSource(t)
			if phase != "preview" {
				f.complete(t)
			}
			if phase == "disposal-pending" {
				_, err := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
				handoffCheck(t, err)
				w, ok := any(f.q).(planningDBDisposalWriter)
				if !ok {
					t.Fatal("approved disposal writer missing")
				}
				_, err = w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
				handoffCheck(t, err)
			}
			root := t.TempDir()
			fs := statefs.NewManager(statefs.Options{Backend: "copy", StateStoreRoot: root})
			// State paths use the database image, not the Liquibase runner image.
			job, found, err := f.q.GetJob(t.Context(), f.attempt.JobID)
			if err != nil || !found {
				t.Fatal("source job unavailable", err)
			}
			path, err := fs.StateDir(root, job.ResolvedImageID, f.clone.SourceStateID)
			handoffCheck(t, err)
			handoffCheck(t, os.MkdirAll(path, 0700))
			marker := filepath.Join(path, "source-sentinel")
			handoffCheck(t, os.WriteFile(marker, []byte("exact retained source bytes"), 0600))
			st, err := storesqlite.New(f.db)
			handoffCheck(t, err)
			manager, err := deletion.NewManager(deletion.Options{Store: st, StateFS: fs, StateStoreRoot: root})
			handoffCheck(t, err)
			before := mainPathJournalRows(t, f.db)
			result, found, deleteErr := manager.DeleteState(t.Context(), f.clone.SourceStateID, deletion.DeleteOptions{Recurse: true, Force: true})
			if deleteErr == nil && (!found || result.Outcome != deletion.OutcomeBlocked) {
				t.Fatal("source deletion was not blocked", result)
			}
			body, readErr := os.ReadFile(marker)
			if readErr != nil || string(body) != "exact retained source bytes" {
				t.Fatal("source bytes removed before ownership refusal", deleteErr, readErr)
			}
			f.unchanged(t, before)
		})
	}
}

// PG07's state source has real published provenance and an immutable operation
// Source, not a task's mutable claimed input. This control is executable before
// any O5 writer exists.
func TestPlanningDBStateSourceFixture(t *testing.T) {
	f := planningDBStateSource(t)
	saved, found, err := f.q.LoadPlanningAttempt(t.Context(), f.attempt.JobID, 1)
	if err != nil || !found || saved.Clone.SourceKind != "state" || saved.Clone.SourceStateID != f.op.Source {
		t.Fatal("state-source preview not bound", err)
	}
	handoffCheck(t, f.observe(t.Context(), saved, f.op))
}

func TestPlanningDBOwnershipProtectsStateSource(t *testing.T) {
	for _, phase := range []string{"retained", "disposal-pending"} {
		t.Run(phase, func(t *testing.T) {
			f := planningDBStateSource(t)
			f.complete(t)
			if phase == "disposal-pending" {
				_, err := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
				handoffCheck(t, err)
				w, ok := any(f.q).(planningDBDisposalWriter)
				if !ok {
					t.Fatal("approved disposal writer missing")
				}
				_, err = w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
				handoffCheck(t, err)
			}
			before := mainPathJournalRows(t, f.db)
			if _, err := f.db.Exec("DELETE FROM states WHERE state_id=?", f.clone.SourceStateID); err == nil {
				t.Fatal("live planning owner lost source protection")
			}
			f.unchanged(t, before)
			if _, err := f.db.Exec("UPDATE prepare_clones SET source_state_id=? WHERE clone_id=?", strings.Repeat("3", 64), f.clone.ID); err == nil {
				t.Fatal("pending owner changed its source")
			}
			f.unchanged(t, before)
		})
	}
}

// PG02/PG03: a previous real working session and then an admitted application
// attempt must never authorise a new retention fact. Known prior completion
// reconciliation remains a separate metadata-only operation.
func TestPlanningDBRetentionRejectsWorkingHistory(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	f.retireCapture(t)
	l := f.firstLaunch(t)
	w, ok := any(f.q).(planningDBTransferWriter)
	if !ok {
		t.Fatal("approved transfer writer missing")
	}
	s, err := w.TransferPlanningDBToFirstSession(t.Context(), l)
	handoffCheck(t, err)
	refuse := func() {
		t.Helper()
		before := mainPathJournalRows(t, f.db)
		if id, err := f.writer(t).RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC); err == nil || id != 0 {
			t.Fatal("working history allowed a new retention fact")
		}
		f.unchanged(t, before)
	}
	refuse()
	handoffCheck(t, f.q.RequestLiquibaseSessionRunnerCreate(t.Context(), s))
	s.RunnerID = strings.Repeat("c", 64)
	handoffCheck(t, f.q.AttachLiquibaseSessionRunner(t.Context(), s, s.RunnerID))
	handoffCheck(t, f.q.RequestLiquibaseSessionRunnerStart(t.Context(), s))
	handoffCheck(t, f.q.MarkLiquibaseSessionRunnerRunning(t.Context(), s))
	s.RunnerPhase = "running"
	_, err = f.q.RecordLiquibaseSessionReadiness(t.Context(), s, mainPathJournalReady{db: f.db, want: s}, mainPathJournalUTC)
	handoffCheck(t, err)
	_, err = f.q.PlanLiquibaseSessionInvocation(t.Context(), s, l.FirstTaskID, l.ExecutionDigest, mainPathInvocationPlan("retained-history-invocation"))
	handoffCheck(t, err)
	refuse()
}

// PG02 uses the existing guarded journal representation of a second planning
// attempt. This is metadata preparation, not a new recovery dispatcher. Every
// INSERT/UPDATE runs with the real schema/FKs and all product guards enabled.
func planningDBSecondAttempt(t *testing.T, f *planningDBFixture) queue.AttemptRecord {
	t.Helper()
	quiet, err := f.q.RecordPlanningCloneQuiescence(t.Context(), f.attempt, 1, f.clone.PlannedRuntimeName, f.op.RuntimeRef, mainPathJournalUTC)
	handoffCheck(t, err)
	const nextClone = "second-planning-preview"
	const decision = "planning-replay-fixture"
	tx, err := f.db.BeginTx(t.Context(), nil)
	handoffCheck(t, err)
	defer tx.Rollback()
	_, err = tx.Exec("PRAGMA defer_foreign_keys=ON")
	handoffCheck(t, err)
	_, err = tx.Exec(`INSERT INTO prepare_attempt_decisions(decision_id,job_id,task_id,attempt_no,decision_no,choice,target_clone_id,next_attempt_no,worker_generation,created_at) VALUES(?,?,'plan',1,1,'replay_fresh',?,2,1,?)`, decision, f.attempt.JobID, nextClone, mainPathJournalUTC)
	handoffCheck(t, err)
	for _, id := range []int64{f.exit, quiet} {
		_, err = tx.Exec("INSERT INTO prepare_decision_evidence(decision_id,evidence_id) VALUES(?,?)", decision, id)
		handoffCheck(t, err)
	}
	_, err = tx.Exec("UPDATE prepare_step_attempts SET phase='closed',resolution='discarded_clone' WHERE job_id=? AND task_id='plan' AND attempt_no=1", f.attempt.JobID)
	handoffCheck(t, err)
	_, err = tx.Exec("UPDATE prepare_clones SET phase='retained' WHERE clone_id=?", f.clone.ID)
	handoffCheck(t, err)
	_, err = tx.Exec(`INSERT INTO prepare_events(job_id,type,message,ts) VALUES(?,'log','WARNING [liquibase_external_effect_replay] controlled fixture',?)`, f.attempt.JobID, mainPathJournalUTC)
	handoffCheck(t, err)
	_, err = tx.Exec(`INSERT INTO prepare_clones(clone_id,job_id,clone_generation,source_kind,source_image_id,clone_ref,planned_db_runtime_name,phase,created_at) VALUES(?,?,2,'image',?,?,'sqlrs-db-second-planning','allocated',?)`, nextClone, f.attempt.JobID, f.clone.SourceImageID, "jobs/planning-handoff/second", mainPathJournalUTC)
	handoffCheck(t, err)
	a := f.attempt
	a.CloneID, a.AttemptNo, a.ExecutorID, a.Phase = nextClone, 2, "", "planned"
	sum := sha256.Sum256([]byte(fmt.Sprintf("common-attempt-v1\n%s\nplan\n2", a.JobID)))
	a.PlannedExecutorName = "sqlrs-step-" + hex.EncodeToString(sum[:20])
	_, err = tx.Exec(`INSERT INTO prepare_step_attempts(job_id,task_id,attempt_no,clone_id,execution_digest,worker_generation,planned_executor_name,phase,legacy_unbound,created_at) VALUES(?,'plan',2,?,?,1,?,'planned',0,?)`, a.JobID, a.CloneID, a.ExecutionDigest, a.PlannedExecutorName, mainPathJournalUTC)
	handoffCheck(t, err)
	_, err = tx.Exec(`INSERT INTO prepare_planning_attempt_artifacts(job_id,task_id,attempt_no,protected_input_ref,complete_output_ref,complete_stderr_ref,capture_manifest_ref,capture_helper_digest,capture_protocol_revision,capture_location) VALUES(?,'plan',2,'second-input','second-output','second-stderr','second-manifest',?,'lb-capture-v2','container_private')`, a.JobID, f.capture.HelperDigest)
	handoffCheck(t, err)
	handoffCheck(t, tx.Commit())
	handoffCheck(t, f.q.ActivatePlanningClone(t.Context(), a, 1, "sqlrs-db-second-planning", strings.Repeat("a", 64)))
	handoffCheck(t, f.q.RequestAttemptCreate(t.Context(), a, 1))
	a.ExecutorID = strings.Repeat("b", 64)
	handoffCheck(t, f.q.AttachAttemptExecutor(t.Context(), a, 1, a.ExecutorID))
	handoffCheck(t, f.q.RequestAttemptStart(t.Context(), a, 1, mainPathJournalUTC))
	_, err = f.q.ObserveAttemptExit(t.Context(), a, 1, a.ExecutorID, 0, mainPathJournalUTC)
	handoffCheck(t, err)
	saved, found, err := f.q.LoadPlanningAttempt(t.Context(), a.JobID, 2)
	if err != nil || !found || saved.Attempt.AttemptNo != 2 || saved.Attempt.CloneID != a.CloneID || saved.Attempt.Phase != "observed_stopped" {
		t.Fatal("actual second planning attempt not reached", err)
	}
	return a
}

func TestPlanningDBSecondAttemptFixture(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	planningDBSecondAttempt(t, f)
}

// Attempt 2 remains ineligible even when it is a real stopped successful
// planner with a valid replay chain, rather than an invented attempt number.
func TestPlanningDBRetentionRejectsSecondAttempt(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	a := planningDBSecondAttempt(t, f)
	saved, found, err := f.q.LoadPlanningAttempt(t.Context(), a.JobID, 2)
	if err != nil || !found {
		t.Fatal("second attempt missing", err)
	}
	op, err := f.access.Assign(t.Context(), "second_planning_operation", filepath.Join(filepath.Dir(f.op.Target), "second"), f.clone.SourceImageID, f.op.Identity)
	handoffCheck(t, err)
	handoffCheck(t, f.access.Attach(t.Context(), op, managedidentity.RuntimeBinding{RuntimeRef: saved.CloneRuntimeID, PhysicalIdentity: op.PhysicalIdentity, IdentityBinding: op.Identity}))
	op.RuntimeRef, op.Stage = saved.CloneRuntimeID, "running"
	observe := func(_ context.Context, got queue.PlanningRecoverySnapshot, actual instanceaccess.Operation) error {
		if got.Attempt.AttemptNo != 2 || got.Clone != saved.Clone || actual != op {
			return queue.ErrPlanningSchema
		}
		return nil
	}
	before := mainPathJournalRows(t, f.db)
	if id, err := f.writer(t).RecordPlanningDBRetention(t.Context(), a, 1, op, observe, mainPathJournalUTC); err == nil || id != 0 {
		t.Fatal("second attempt admitted normal-path retention", id, err)
	}
	f.unchanged(t, before)
}
