package prepare

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedstore"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/sqliteconn"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
)

var errPlanningTransferBoundary = errors.New("PG08 exact transfer boundary")

// Observe the real queue, not a substitute persistence implementation. Methods
// inherited from SQLiteStore keep all ordinary admission/disposal guards active.
type planningTransferJournal struct {
	*queue.SQLiteStore
	f             *mainPathNativeFixture
	runtime       *planningDBNativeRuntime
	fault         string
	events        []string
	transferCalls int
}

func (q *planningTransferJournal) TransferPlanningDBToFirstSession(ctx context.Context, launch queue.LiquibaseSessionLaunch) (queue.LiquibaseSessionRecord, error) {
	q.transferCalls++
	if len(q.runtime.starts) != 1 || len(q.f.rt.databases) != 0 {
		return queue.LiquibaseSessionRecord{}, errors.New("SQL or second DB preceded planning transfer")
	}
	var task, clone, capture int
	if err := q.f.rt.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_tasks WHERE job_id=? AND task_id=? AND status='running' AND input_kind=? AND input_id=?", launch.Session.JobID, launch.FirstTaskID, launch.Session.StartKind, launch.Session.StartID).Scan(&task); err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	if err := q.f.rt.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_clones WHERE job_id=? AND clone_id=? AND phase='active' AND db_runtime_id=? AND clone_generation=1", launch.Session.JobID, launch.Clone.ID, q.runtime.starts[0].ID).Scan(&clone); err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	if err := q.f.rt.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_planning_attempt_artifacts WHERE job_id=? AND executor_retired_at IS NOT NULL AND protected_input_retired_at IS NOT NULL AND output_retired_at IS NOT NULL AND stderr_retired_at IS NOT NULL AND manifest_retired_at IS NOT NULL", launch.Session.JobID).Scan(&capture); err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	if task != 1 || clone != 1 || capture != 1 {
		return queue.LiquibaseSessionRecord{}, errors.New("transfer preceded exact suffix/preview/private cleanup")
	}
	q.events = append(q.events, "transfer-request")
	if q.fault == "before-transfer" {
		return queue.LiquibaseSessionRecord{}, errPlanningTransferBoundary
	}
	if q.fault == "disposal-pending" {
		at := q.f.m.normalTimestamp()
		if _, err := q.SQLiteStore.RequestJobCancellation(ctx, launch.Session.JobID, at); err != nil {
			return queue.LiquibaseSessionRecord{}, err
		}
		preview, found, err := q.SQLiteStore.LoadPlanningAttempt(ctx, launch.Session.JobID, 1)
		if err != nil || !found {
			return queue.LiquibaseSessionRecord{}, errors.New("pending-disposal fixture lost preview")
		}
		w, ok := any(q.SQLiteStore).(interface {
			RequestPlanningDBDisposal(context.Context, queue.AttemptRecord, int, string, string) (int64, error)
		})
		if !ok {
			return queue.LiquibaseSessionRecord{}, errors.New("approved disposal request writer missing")
		}
		if _, err := w.RequestPlanningDBDisposal(ctx, preview.Attempt, 1, "accepted_cancel", at); err != nil {
			return queue.LiquibaseSessionRecord{}, err
		}
		q.events = append(q.events, "disposal-requested")
		return queue.LiquibaseSessionRecord{}, errPlanningTransferBoundary
	}
	w, ok := any(q.SQLiteStore).(interface {
		TransferPlanningDBToFirstSession(context.Context, queue.LiquibaseSessionLaunch) (queue.LiquibaseSessionRecord, error)
	})
	if !ok {
		return queue.LiquibaseSessionRecord{}, errors.New("approved transfer writer missing")
	}
	session, err := w.TransferPlanningDBToFirstSession(ctx, launch)
	if err != nil {
		return queue.LiquibaseSessionRecord{}, err
	}
	var n int
	if err := q.f.rt.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_liquibase_planning_db_ownership WHERE job_id=? AND session_id=? AND transferred_at IS NOT NULL", session.JobID, session.SessionID).Scan(&n); err != nil || n != 1 {
		return queue.LiquibaseSessionRecord{}, errors.New("transfer returned without committed exact ownership")
	}
	q.events = append(q.events, "transfer-committed")
	if q.fault == "lost-transfer-reply" {
		return queue.LiquibaseSessionRecord{}, errors.Join(queue.ErrFinalizationIndeterminate, errPlanningTransferBoundary)
	}
	return session, nil
}

func (q *planningTransferJournal) RecordLiquibaseSessionReadiness(ctx context.Context, session queue.LiquibaseSessionRecord, verifier queue.LiquibaseSessionReadinessVerifier, at string) (queue.LiquibaseSessionReadiness, error) {
	if len(q.events) < 2 || q.events[1] != "transfer-committed" {
		return queue.LiquibaseSessionReadiness{}, errors.New("readiness preceded committed transfer")
	}
	result, err := q.SQLiteStore.RecordLiquibaseSessionReadiness(ctx, session, verifier, at)
	if err == nil {
		q.events = append(q.events, "runner-ready")
	}
	return result, err
}

func (q *planningTransferJournal) PlanLiquibaseSessionInvocation(ctx context.Context, session queue.LiquibaseSessionRecord, task, execution string, plan queue.LiquibaseInvocationPlan) (queue.LiquibaseInvocationRecord, error) {
	if len(q.events) < 3 || q.events[2] != "runner-ready" {
		return queue.LiquibaseInvocationRecord{}, errors.New("SQL admission preceded verified runner readiness")
	}
	result, err := q.SQLiteStore.PlanLiquibaseSessionInvocation(ctx, session, task, execution, plan)
	if err == nil {
		q.events = append(q.events, "sql-admitted")
	}
	return result, err
}

// PG08: observations wrap real stock planning and its actual saved suffix. The
// only injected refusals concern the new transfer boundary, not native SQL.
func TestNormalPlanningDBHandoffOrder(t *testing.T) {
	for _, fault := range []string{"none", "before-transfer", "lost-transfer-reply", "disposal-pending"} {
		t.Run(fault, func(t *testing.T) {
			f := newMainPathNativeFixture(t)
			runtime := &planningDBNativeRuntime{mainPathNativeRuntime: f.rt}
			q := &planningTransferJournal{SQLiteStore: f.q, f: &f, runtime: runtime, fault: fault}
			f.m.runtime, f.m.queue = runtime, q
			accepted, err := f.m.Submit(f.ctx, f.req)
			if err != nil {
				t.Fatal(err)
			}
			status, found := f.m.Get(accepted.JobID)
			if !found || q.transferCalls != 1 || len(runtime.starts) != 1 {
				t.Fatal("did not reach the exact one-DB transfer boundary", q.transferCalls, len(runtime.starts))
			}
			if fault == "none" {
				want := []string{"transfer-request", "transfer-committed", "runner-ready", "sql-admitted", "sql-admitted", "sql-admitted"}
				if status.Status != StatusSucceeded || status.Result == nil || !reflect.DeepEqual(q.events, want) {
					t.Fatal("normal transfer order differs", q.events, status.Error)
				}
				mainPathAssertRows(t, f.ctx, status.Result.DSN, 3, 2)
				return
			}
			if status.Status != "recovery_required" || status.Result != nil || len(f.rt.databases) != 0 || len(f.rt.runners) != 0 {
				t.Fatal("uncertain/refused transfer admitted SQL or publication", status.Error)
			}
			var retained, transferred, pending int
			if err := f.rt.db.QueryRowContext(f.ctx, `SELECT count(*),coalesce(sum(session_id IS NOT NULL),0),coalesce(sum(disposal_requested_evidence_id IS NOT NULL),0)
FROM prepare_liquibase_planning_db_ownership WHERE job_id=? AND disposed_at IS NULL`, accepted.JobID).Scan(&retained, &transferred, &pending); err != nil {
				t.Fatal(err)
			}
			wantTransferred := 0
			if fault == "lost-transfer-reply" {
				wantTransferred = 1
			}
			wantPending := 0
			if fault == "disposal-pending" {
				wantPending = 1
			}
			if retained != 1 || transferred != wantTransferred || pending != wantPending {
				t.Fatal("refusal lost the original exact retained/session owner")
			}
			live, err := f.rt.InspectManaged(f.ctx, runtime.starts[0].Binding)
			if err != nil || live.ID != runtime.starts[0].ID {
				t.Fatal("refusal lost the exact live DB or its identity could not be verified", err)
			}
			var file string
			if err := f.rt.db.QueryRowContext(f.ctx, "SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file); err != nil {
				t.Fatal(err)
			}
			for reopen := 0; reopen < 2; reopen++ {
				db, err := sqliteconn.Open(file)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := managedstore.Initialize(f.ctx, db, func(context.Context) error {
					t.Fatal("current retained store unexpectedly reached inventory")
					return nil
				}); err != nil {
					_ = db.Close()
					t.Fatal(err)
				}
				reopenedQueue, err := queue.New(db)
				if err != nil {
					_ = db.Close()
					t.Fatal(err)
				}
				reopenedStore, err := storesqlite.New(db)
				if err != nil {
					_ = db.Close()
					t.Fatal(err)
				}
				// Model the persisted running status at process failure, then call
				// the actual existing classifier with the newly opened stores.
				running := "running"
				if err := reopenedQueue.UpdateJob(f.ctx, accepted.JobID, queue.JobUpdate{Status: &running}); err != nil {
					_ = db.Close()
					t.Fatal(err)
				}
				f.m.queue, f.m.store = reopenedQueue, reopenedStore
				err = f.m.Recover(f.ctx)
				f.m.queue, f.m.store = q, f.st
				if err != nil {
					_ = db.Close()
					t.Fatal(err)
				}
				job, found, err := reopenedQueue.GetJob(f.ctx, accepted.JobID)
				if err != nil || !found || job.Status != "recovery_required" || len(runtime.starts) != 1 || len(f.rt.databases) != 0 || q.transferCalls != 1 {
					_ = db.Close()
					t.Fatal("reopen broadened first-session recovery or replayed SQL", err)
				}
				var owners, applied int
				if err := db.QueryRowContext(f.ctx, `SELECT
(SELECT count(*) FROM prepare_liquibase_planning_db_ownership WHERE job_id=?1 AND disposed_at IS NULL),
(SELECT count(*) FROM prepare_step_attempts WHERE job_id=?1 AND task_id<>'plan')`, accepted.JobID).Scan(&owners, &applied); err != nil || owners != 1 || applied != 0 {
					_ = db.Close()
					t.Fatal("recovery changed exact ownership/application history", err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
