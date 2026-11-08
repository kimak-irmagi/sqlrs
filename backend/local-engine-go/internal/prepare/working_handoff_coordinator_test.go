package prepare

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/sqlrs/engine-local/internal/dbms"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store"
)

// These test-only observers wrap the existing runtime/journal boundaries. They
// never substitute SQLite effects or grant replay/cleanup authority (WH07).
type handoffSequence struct {
	f           mainPathNativeFixture
	events      []string
	failAt      string
	faults      int
	slotsBefore []handoffHeldSlot
}

// handoffHeldSlot preserves the entire slot row, not merely a count that could
// conceal replacing one retained invocation with a different one after failure.
type handoffHeldSlot struct {
	session, job, task, reserved string
	slot, attempt                int
	prior                        sql.NullString
}

func readHandoffHeldSlots(ctx context.Context, db *sql.DB, runner string) ([]handoffHeldSlot, error) {
	rows, err := db.QueryContext(ctx, `SELECT session_id,job_id,task_id,reserved_at,slot_no,attempt_no,prior_retirement_ref
FROM prepare_liquibase_capture_slots WHERE session_id=(SELECT session_id FROM prepare_liquibase_sessions WHERE runner_id=?) ORDER BY slot_no`, runner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var slots []handoffHeldSlot
	for rows.Next() {
		var slot handoffHeldSlot
		if err := rows.Scan(&slot.session, &slot.job, &slot.task, &slot.reserved, &slot.slot, &slot.attempt, &slot.prior); err != nil {
			return nil, err
		}
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

var errHandoffSequenceFault = errors.New("WH exact coordinator boundary")

func (s *handoffSequence) inject(at string) error {
	if s.failAt == at {
		s.faults++
		return errHandoffSequenceFault
	}
	return nil
}

func (s *handoffSequence) require(last string) error {
	if len(s.events) == 0 || s.events[len(s.events)-1] != last {
		return fmt.Errorf("WH coordinator reached a phase out of order: expected %s", last)
	}
	return nil
}

type handoffSequenceJournal struct {
	*queue.SQLiteStore
	sequence *handoffSequence
}

func (q *handoffSequenceJournal) MarkLiquibaseSessionRunnerStopped(ctx context.Context, session queue.LiquibaseSessionRecord) error {
	var completed int
	if err := q.sequence.f.rt.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_tasks WHERE job_id=? AND type='state_execute' AND status='succeeded'", session.JobID).Scan(&completed); err != nil {
		return err
	}
	if completed != 3 || len(q.sequence.events) != 0 {
		return fmt.Errorf("WH parent stopped before the exact final published snapshot")
	}
	if err := q.SQLiteStore.MarkLiquibaseSessionRunnerStopped(ctx, session); err != nil {
		return err
	}
	q.sequence.events = append(q.sequence.events, "parent-stopped")
	return nil
}

func (q *handoffSequenceJournal) RequestLiquibaseSessionDBRestart(ctx context.Context, session queue.LiquibaseSessionRecord) error {
	if len(q.sequence.events) == 0 {
		return q.SQLiteStore.RequestLiquibaseSessionDBRestart(ctx, session)
	}
	if err := q.sequence.require("parent-stopped"); err != nil {
		return err
	}
	if err := q.SQLiteStore.RequestLiquibaseSessionDBRestart(ctx, session); err != nil {
		return err
	}
	q.sequence.events = append(q.sequence.events, "resume-requested")
	return nil
}

func (q *handoffSequenceJournal) MarkLiquibaseSessionDBReady(ctx context.Context, session queue.LiquibaseSessionRecord, name, database string) error {
	if len(q.sequence.events) == 0 {
		return q.SQLiteStore.MarkLiquibaseSessionDBReady(ctx, session, name, database)
	}
	if err := q.sequence.require("database-resumed"); err != nil {
		return err
	}
	if err := q.SQLiteStore.MarkLiquibaseSessionDBReady(ctx, session, name, database); err != nil {
		return err
	}
	q.sequence.events = append(q.sequence.events, "database-ready")
	return nil
}

func (q *handoffSequenceJournal) PublishWorkingInstanceAndBeginFinalization(ctx context.Context, session queue.LiquibaseSessionRecord, entry store.InstanceCreate, binding managedidentity.RuntimeBinding, operation instanceaccess.Operation) (queue.FinalizationSnapshot, error) {
	if err := q.sequence.require("database-ready"); err != nil {
		return queue.FinalizationSnapshot{}, err
	}
	if len(q.sequence.f.rt.databases) != 3 || binding.RuntimeRef != q.sequence.f.rt.databases[2] {
		return queue.FinalizationSnapshot{}, fmt.Errorf("WH publication replaced the working runtime")
	}
	if err := q.sequence.inject("publication"); err != nil {
		return queue.FinalizationSnapshot{}, err
	}
	intent, err := q.SQLiteStore.PublishWorkingInstanceAndBeginFinalization(ctx, session, entry, binding, operation)
	if err == nil {
		q.sequence.events = append(q.sequence.events, "published")
	}
	return intent, err
}

func (q *handoffSequenceJournal) RetireTransferredLiquibaseSession(ctx context.Context, session queue.LiquibaseSessionRecord, verifier queue.LiquibaseTransferredSessionVerifier, at string) error {
	if err := q.sequence.require("parent-removed"); err != nil {
		return err
	}
	if err := q.SQLiteStore.RetireTransferredLiquibaseSession(ctx, session, verifier, at); err != nil {
		return err
	}
	q.sequence.events = append(q.sequence.events, "private-retired")
	return nil
}

func (q *handoffSequenceJournal) CommitFinalization(ctx context.Context, intent queue.FinalizationSnapshot, at, expires string) error {
	if err := q.sequence.require("private-retired"); err != nil {
		return err
	}
	if err := q.SQLiteStore.CommitFinalization(ctx, intent, at, expires); err != nil {
		return err
	}
	q.sequence.events = append(q.sequence.events, "terminal")
	return nil
}

type handoffSequenceRuntime struct {
	*mainPathNativeRuntime
	sequence *handoffSequence
}

func (r *handoffSequenceRuntime) SignalContainer(ctx context.Context, id string) error {
	if len(r.runners) == 3 && id == r.runners[2] {
		var err error
		r.sequence.slotsBefore, err = readHandoffHeldSlots(ctx, r.db, id)
		if err != nil {
			return err
		}
		if err := r.sequence.inject("stop"); err != nil {
			return err
		}
	}
	return r.mainPathNativeRuntime.SignalContainer(ctx, id)
}

func (r *handoffSequenceRuntime) StartManaged(ctx context.Context, request engineRuntime.ManagedStartRequest) (engineRuntime.Instance, error) {
	if len(r.runners) >= 3 {
		return engineRuntime.Instance{}, fmt.Errorf("WH coordinator allocated a replacement after its last SQL")
	}
	return r.mainPathNativeRuntime.StartManaged(ctx, request)
}

func (r *handoffSequenceRuntime) Exec(ctx context.Context, id string, request engineRuntime.ExecRequest) (string, error) {
	finalResume := len(r.sequence.events) != 0 && len(request.Args) != 0 && request.Args[0] == "pg_ctl" && request.Args[len(request.Args)-1] == "start"
	if finalResume {
		if err := r.sequence.require("resume-requested"); err != nil {
			return "", err
		}
		if len(r.databases) != 3 || id != r.databases[2] {
			return "", fmt.Errorf("WH coordinator resumed a foreign runtime")
		}
		if err := r.sequence.inject("resume"); err != nil {
			return "", err
		}
	}
	out, err := r.mainPathNativeRuntime.Exec(ctx, id, request)
	if finalResume && err == nil {
		r.sequence.events = append(r.sequence.events, "database-resumed")
	}
	return out, err
}

func (r *handoffSequenceRuntime) RemoveStoppedContainer(ctx context.Context, id string) error {
	finalParent := len(r.runners) == 3 && id == r.runners[2]
	if finalParent {
		if err := r.sequence.require("published"); err != nil {
			return err
		}
		var exact int
		if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_session_disposals d
JOIN prepare_liquibase_job_finalizations f ON f.job_id=d.handoff_job_id
AND f.instance_pin=d.handoff_instance_id AND f.outcome_digest=d.handoff_outcome_digest
JOIN instances i ON i.instance_id=f.instance_pin AND i.runtime_id=d.db_runtime_id
JOIN instance_access a ON a.instance_ref=i.instance_id AND a.stage='verified'
WHERE d.reason='instance_transferred' AND d.runner_id=? AND d.db_absent_at IS NULL`, id).Scan(&exact); err != nil || exact != 1 {
			return fmt.Errorf("WH parent disposal preceded committed activated ownership")
		}
		if err := r.sequence.inject("private-removal"); err != nil {
			return err
		}
	}
	if err := r.mainPathNativeRuntime.RemoveStoppedContainer(ctx, id); err != nil {
		return err
	}
	if finalParent {
		r.sequence.events = append(r.sequence.events, "parent-removed")
	}
	return nil
}

func (r *handoffSequenceRuntime) Stop(ctx context.Context, id string) error {
	if len(r.sequence.events) != 0 && len(r.databases) == 3 && id == r.databases[2] {
		return fmt.Errorf("WH coordinator attempted destructive disposal of the transferred database")
	}
	return r.mainPathNativeRuntime.Stop(ctx, id)
}

// WH07 uses real three-change SQL, real metadata writers and real managed access;
// the observers assert ordering without a new production injection mechanism.
func TestWorkingInstanceHandoffCoordinator(t *testing.T) {
	f := newMainPathNativeFixture(t)
	sequence := &handoffSequence{f: f}
	runtime := &handoffSequenceRuntime{mainPathNativeRuntime: f.rt, sequence: sequence}
	f.m.runtime = runtime
	f.m.dbms = dbms.NewPostgres(runtime)
	f.m.queue = &handoffSequenceJournal{SQLiteStore: f.q, sequence: sequence}
	accepted, err := f.m.Submit(f.ctx, f.req)
	if err != nil {
		t.Fatal(err)
	}
	status, found := f.m.Get(accepted.JobID)
	if !found || status.Status != StatusSucceeded || status.Result == nil {
		t.Fatal("observed coordinator did not complete", status.Error)
	}
	want := []string{"parent-stopped", "resume-requested", "database-resumed", "database-ready", "published", "parent-removed", "private-retired", "terminal"}
	if !reflect.DeepEqual(want, sequence.events) {
		t.Fatalf("handoff sequence differs: %v", sequence.events)
	}
	mainPathAssertRows(t, f.ctx, status.Result.DSN, 3, 2)
}

// WH04/WH05/WH07 exercise only the new handoff boundaries. Refusal retains the
// already published SQL prefix and exact native owners; there is no recovery
// dispatcher or fallback publication. Native cleanup runs later through the
// original fixture, not the refusing observer.
func TestWorkingInstanceHandoffCoordinatorRefusal(t *testing.T) {
	for _, at := range []string{"stop", "resume", "publication", "private-removal"} {
		t.Run(at, func(t *testing.T) {
			f := newMainPathNativeFixture(t)
			sequence := &handoffSequence{f: f, failAt: at}
			runtime := &handoffSequenceRuntime{mainPathNativeRuntime: f.rt, sequence: sequence}
			f.m.runtime = runtime
			f.m.dbms = dbms.NewPostgres(runtime)
			f.m.queue = &handoffSequenceJournal{SQLiteStore: f.q, sequence: sequence}
			accepted, err := f.m.Submit(f.ctx, f.req)
			if err != nil {
				t.Fatal(err)
			}
			status, found := f.m.Get(accepted.JobID)
			if !found || status.Status != "recovery_required" || status.Result != nil || sequence.faults != 1 {
				t.Fatal("handoff refusal did not reach its exact boundary and retain work", status.Status, sequence.faults)
			}
			var completed, sessions, active, slots, lastSlot, publications, pins int
			queries := []struct {
				query string
				into  *int
			}{
				{"SELECT count(*) FROM prepare_tasks WHERE job_id=? AND type='state_execute' AND status='succeeded'", &completed},
				{"SELECT count(*) FROM prepare_liquibase_sessions WHERE job_id=?", &sessions},
				{"SELECT count(*) FROM prepare_liquibase_sessions WHERE job_id=? AND retired_at IS NULL", &active},
				{"SELECT count(*) FROM prepare_liquibase_capture_slots WHERE job_id=?", &slots},
				{"SELECT count(*) FROM prepare_liquibase_capture_slots WHERE job_id=? AND task_id='execute-2'", &lastSlot},
				{"SELECT count(*) FROM prepare_liquibase_job_finalizations WHERE job_id=? AND outcome_kind='prepare_success'", &publications},
				{"SELECT count(*) FROM prepare_liquibase_job_finalizations WHERE job_id=? AND instance_pin IS NOT NULL", &pins},
			}
			for _, check := range queries {
				if err := f.rt.db.QueryRowContext(f.ctx, check.query, accepted.JobID).Scan(check.into); err != nil {
					t.Fatal(err)
				}
			}
			wantPublications := 0
			if at == "private-removal" {
				wantPublications = 1
			}
			if completed != 3 || sessions != 1 || active != 1 || slots < 1 || slots > 2 || lastSlot != 1 || publications != wantPublications || pins != wantPublications || len(f.rt.runners) != 3 {
				t.Fatalf("handoff refusal lost exact retained ownership: completed=%d sessions=%d active=%d slots=%d publications=%d pins=%d", completed, sessions, active, slots, publications, pins)
			}
			slotsAfter, err := readHandoffHeldSlots(f.ctx, f.rt.db, f.rt.runners[2])
			if err != nil || !reflect.DeepEqual(sequence.slotsBefore, slotsAfter) {
				t.Fatal("handoff refusal changed the exact retained capture slots", err)
			}
			// Saved DB and parent IDs must still exist before fallback, whether
			// the database process is stopped or resumed in its same container.
			var parentName, databaseName string
			if err := f.rt.db.QueryRowContext(f.ctx, `SELECT s.planned_runner_name,c.planned_db_runtime_name FROM prepare_liquibase_sessions s JOIN prepare_clones c ON c.clone_id=s.clone_id WHERE s.job_id=?`, accepted.JobID).Scan(&parentName, &databaseName); err != nil {
				t.Fatal(err)
			}
			parent, found, err := f.rt.InspectNamedContainer(f.ctx, parentName)
			if err != nil || !found || parent.ID != f.rt.runners[2] {
				t.Fatal("refusal lost its exact retained parent", err)
			}
			database, found, err := f.rt.InspectNamedDatabaseContainer(f.ctx, databaseName)
			if err != nil || !found || database.ID != f.rt.databases[2] {
				t.Fatal("refusal lost its exact retained database", err)
			}
		})
	}
}
