package queue_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// PG03 distinguishes identity/input rejection from a failure in planning setup.
// Each bad launch is followed by the exact valid launch at the same boundary.
func TestPlanningDBFirstSessionTransferRejectsChangedIdentity(t *testing.T) {
	for _, variant := range []string{"job", "clone", "generation", "clone-ref", "runtime-name", "source", "fingerprint", "execution", "task", "position", "session-number", "config", "helper", "recovery-proof", "invalid-time"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			f.retireCapture(t)
			launch := f.firstLaunch(t)
			w, ok := any(f.q).(planningDBTransferWriter)
			if !ok {
				t.Fatal("approved first-session transfer writer is not implemented")
			}
			bad := launch
			switch variant {
			case "job":
				bad.Session.JobID = "another-job"
			case "clone":
				bad.Clone.ID, bad.Session.CloneID = "other-preview", "other-preview"
			case "generation":
				bad.Clone.Generation = 2
			case "clone-ref":
				bad.Clone.Ref += "-other"
			case "runtime-name":
				bad.Clone.PlannedRuntimeName += "-other"
			case "source":
				bad.Clone.SourceImageID, bad.Session.StartID = pgDigest("f"), pgDigest("f")
			case "fingerprint":
				bad.Session.ExecutionFingerprint = pgDigest("f")
			case "execution":
				bad.ExecutionDigest = pgDigest("f")
			case "task":
				bad.FirstTaskID = "two"
			case "position":
				bad.Session.StartPosition = 2
			case "session-number":
				bad.Session.SessionNo = 2
			case "config":
				bad.Session.RunnerConfigDigest = "not-a-digest"
			case "helper":
				bad.Session.CaptureHelperDigest = "not-a-digest"
			case "recovery-proof":
				bad.RequireFirstSessionProof = true
			case "invalid-time":
				bad.Session.CreatedAt = "2026-11-13T12:00:00+03:00"
			}
			before := mainPathJournalRows(t, f.db)
			if _, err := w.TransferPlanningDBToFirstSession(t.Context(), bad); err == nil {
				t.Fatal("changed transfer identity accepted")
			}
			f.unchanged(t, before)
			_, err := w.TransferPlanningDBToFirstSession(t.Context(), launch)
			handoffCheck(t, err)
		})
	}
}

// PG03: the full session INSERT must be refused solely because its otherwise
// matching planning clone has not been transferred. The queue transfer below
// supplies the same session as a positive control, without creating a clone.
func TestPlanningDBDirectSessionAdoptionRejected(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	f.retireCapture(t)
	l := f.firstLaunch(t)
	s := l.Session
	before := mainPathJournalRows(t, f.db)
	_, err := f.db.ExecContext(t.Context(), `INSERT INTO prepare_liquibase_sessions(
session_id,job_id,session_no,clone_id,worker_generation,execution_fingerprint,
start_kind,start_id,start_position,planned_runner_name,runner_config_digest,capture_helper_digest,
protected_config_ref,runner_phase,db_phase,cursor_image_id,cursor_state_id,cursor_position,created_at,
capture_location,capture_protocol_revision)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'planned','running',?,NULL,?,?,'container_private','lb-step-capture-v2')`,
		s.SessionID, s.JobID, s.SessionNo, s.CloneID, s.WorkerGeneration, s.ExecutionFingerprint,
		s.StartKind, s.StartID, s.StartPosition, s.PlannedRunnerName, s.RunnerConfigDigest,
		s.CaptureHelperDigest, s.ProtectedConfigRef, s.StartID, s.StartPosition, s.CreatedAt)
	var constraint interface{ Code() int }
	if !errors.As(err, &constraint) || constraint.Code() != 1811 {
		t.Fatal("direct planning-clone adoption was not refused by its SQL guard", err)
	}
	f.unchanged(t, before)
	w, ok := any(f.q).(planningDBTransferWriter)
	if !ok {
		t.Fatal("approved first-session transfer writer is not implemented")
	}
	_, err = w.TransferPlanningDBToFirstSession(t.Context(), l)
	handoffCheck(t, err)
}

// PG06: a string alone is not destructive authority. A real accepted cancellation
// is saved before its disposal request, and absence is checked only afterwards.
func TestRetainedPlanningDBNonReuse(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	f.retireCapture(t)
	w, ok := any(f.q).(planningDBDisposalWriter)
	if !ok {
		t.Fatal("approved retained DB disposal writers are not implemented")
	}
	before := mainPathJournalRows(t, f.db)
	for _, reason := range []string{"accepted_cancel", "selected_failure", "source_mismatch", "full_cache", "anything"} {
		if id, err := w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, reason, mainPathJournalUTC); err == nil || id != 0 {
			t.Fatal("unproved destructive reason accepted", reason, id, err)
		}
		f.unchanged(t, before)
	}
	_, err := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
	handoffCheck(t, err)
	request, err := w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
	handoffCheck(t, err)
	if request <= f.retained || f.count(t, `SELECT count(*) FROM prepare_liquibase_planning_db_ownership
WHERE job_id=? AND disposal_requested_evidence_id=? AND disposed_at IS NULL AND disposal_evidence_id IS NULL AND session_id IS NULL`, f.attempt.JobID, request) != 1 {
		t.Fatal("request invented absence or lost its retention link")
	}
	probe := &planningDBAbsence{f: f, request: request, err: errors.New("full DB ID still present")}
	before = mainPathJournalRows(t, f.db)
	if err := w.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, probe, mainPathJournalUTC); err == nil {
		t.Fatal("live DB or unretired operation accepted as disposed")
	}
	f.unchanged(t, before)
	// Matching committed disposal intent, not the caller's reason string, permits
	// the real managed-operation writer to cross its retirement boundary.
	handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
	retired := f.op
	retired.Stage = "retired"
	before = mainPathJournalRows(t, f.db)
	if err := w.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, retired, probe, mainPathJournalUTC); err == nil || probe.calls == 0 {
		t.Fatal("failed absence observation accepted or not reached", err)
	}
	f.unchanged(t, before)
	probe.err = nil
	handoffCheck(t, w.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, retired, probe, mainPathJournalUTC))
	if f.count(t, `SELECT count(*) FROM prepare_liquibase_planning_db_ownership
WHERE job_id=? AND disposal_requested_evidence_id=? AND disposal_evidence_id>? AND disposed_at=? AND session_id IS NULL`, f.attempt.JobID, request, request, mainPathJournalUTC) != 1 || f.count(t, "SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase='released'", f.clone.ID) != 1 {
		t.Fatal("disposal did not atomically record exact absence and release")
	}
	if f.count(t, "SELECT count(*) FROM prepare_liquibase_sessions WHERE job_id=?", f.attempt.JobID) != 0 || f.count(t, "SELECT count(*) FROM prepare_step_attempts WHERE job_id=? AND task_id<>'plan'", f.attempt.JobID) != 0 {
		t.Fatal("cancellation cleanup invented session or SQL")
	}
}

// The existing interface promises full exact runtime AND private clone-byte
// absence. Its double checks saved intent and retired operation through the
// single-connection handle, independently of the new writer's own SQL.
type planningDBAbsence struct {
	f       *planningDBFixture
	request int64
	err     error
	calls   int
}

func (p *planningDBAbsence) VerifyReleasedClone(ctx context.Context, clone queue.ClonePlan, runtimeID string) error {
	p.calls++
	if clone != p.f.clone || runtimeID != p.f.op.RuntimeRef {
		return queue.ErrPlanningSchema
	}
	var n int
	err := p.f.db.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_planning_db_ownership o
JOIN managed_runtime_operations op ON op.operation_ref=?
WHERE o.job_id=? AND o.clone_id=? AND o.disposal_requested_evidence_id=? AND o.session_id IS NULL
AND op.stage='retired' AND op.runtime_ref=? AND op.target_path=?`, p.f.op.Ref, p.f.attempt.JobID, clone.ID, p.request, runtimeID, p.f.op.Target).Scan(&n)
	if err != nil {
		return err
	}
	if n != 1 {
		return queue.ErrPlanningSchema
	}
	return p.err
}

// PG07: a live retained owner protects all original identities; historical
// ownership is not permission to delete or reroute them directly.
func TestPlanningDBOwnershipProtection(t *testing.T) {
	for _, variant := range []string{"owner-delete", "fact-delete", "decision-link-delete", "plan-digest", "clear-retention", "partial-transfer", "source", "runtime", "target", "retire-operation", "terminal-job", "delete-job"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			before := mainPathJournalRows(t, f.db)
			var err error
			switch variant {
			case "owner-delete":
				_, err = f.db.Exec("DELETE FROM prepare_liquibase_planning_db_ownership WHERE job_id=?", f.attempt.JobID)
			case "fact-delete":
				_, err = f.db.Exec("DELETE FROM prepare_attempt_evidence WHERE evidence_id=?", f.retained)
			case "decision-link-delete":
				_, err = f.db.Exec("DELETE FROM prepare_decision_evidence WHERE evidence_id=?", f.retained)
			case "plan-digest":
				_, err = f.db.Exec("UPDATE prepare_liquibase_planning_db_ownership SET plan_digest=? WHERE job_id=?", pgDigest("f"), f.attempt.JobID)
			case "clear-retention":
				_, err = f.db.Exec("UPDATE prepare_liquibase_planning_db_ownership SET retained_evidence_id=NULL WHERE job_id=?", f.attempt.JobID)
			case "partial-transfer":
				_, err = f.db.Exec("UPDATE prepare_liquibase_planning_db_ownership SET transferred_at=? WHERE job_id=?", mainPathJournalUTC, f.attempt.JobID)
			case "source":
				_, err = f.db.Exec("UPDATE managed_runtime_operations SET source_ref=? WHERE operation_ref=?", pgDigest("f"), f.op.Ref)
			case "runtime":
				_, err = f.db.Exec("UPDATE managed_runtime_operations SET runtime_ref=? WHERE operation_ref=?", strings.Repeat("f", 64), f.op.Ref)
			case "target":
				_, err = f.db.Exec("UPDATE managed_runtime_operations SET target_path=? WHERE operation_ref=?", f.op.Target+"-other", f.op.Ref)
			case "retire-operation":
				err = f.access.RetireOperation(t.Context(), f.op)
			case "terminal-job":
				status := "failed"
				err = f.q.UpdateJob(t.Context(), f.attempt.JobID, queue.JobUpdate{Status: &status})
			case "delete-job":
				_, err = f.db.Exec("DELETE FROM prepare_jobs WHERE job_id=?", f.attempt.JobID)
			}
			if err == nil {
				t.Fatal("live retained owner lost protection", variant)
			}
			f.unchanged(t, before)
		})
	}
}
