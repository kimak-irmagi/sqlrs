package queue_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/finalizationcodec"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	sqlitedriver "modernc.org/sqlite"
)

// WH02/WH06 exercise durable winner selection, rather than merely passing a
// mismatching session argument. A rejection must preserve every journal row.
func TestWorkingInstanceHandoffAuthorityBoundaries(t *testing.T) {
	for _, variant := range []string{"access-not-activated", "ordinary-cache-publisher", "failure-selected-first"} {
		t.Run(variant, func(t *testing.T) {
			f := newWorkingHandoffFixture(t)
			if variant == "access-not-activated" {
				handoffCheck(t, f.q.RequestLiquibaseSessionDBRestart(t.Context(), f.session))
				handoffCheck(t, f.q.MarkLiquibaseSessionDBReady(t.Context(), f.session, "sqlrs-db-handoff", f.binding.RuntimeRef))
				f.session.DBPhase = "running"
			} else {
				f.ready(t)
			}
			if variant == "failure-selected-first" {
				_, err := f.q.SelectLiquibaseJobFailure(t.Context(), f.session.JobID, handoffFailure(), mainPathJournalUTC)
				handoffCheck(t, err)
			}
			before := mainPathJournalRows(t, f.db)
			var err error
			if variant == "ordinary-cache-publisher" {
				// Only the new working-owner route may preserve an operation whose
				// source differs from the final state; the cache route stays strict.
				if f.op.Source == f.entry.StateID {
					t.Fatal("control lacks the distinct source identity")
				}
				_, err = f.q.PublishInstanceAndBeginFinalization(t.Context(), f.session.JobID, f.entry, f.binding, f.op)
			} else {
				_, err = f.q.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
			}
			refused := errors.Is(err, queue.ErrLiquibaseFinalization)
			if variant == "access-not-activated" {
				var coded interface{ Code() int }
				refused = errors.As(err, &coded) && coded.Code() == 1811 && strings.Contains(err.Error(), "invalid exact Liquibase session disposal authority")
			}
			if !refused || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
				t.Fatal("publication crossed an unproved authority boundary", err)
			}
		})
	}
}

func handoffFailure() finalizationcodec.Outcome {
	return finalizationcodec.Outcome{Kind: finalizationcodec.Failure, EventProfile: finalizationcodec.StatusError,
		Problem: &finalizationcodec.PublicError{Code: "prepare_failed", Message: "controlled publication refusal"}}
}

// WH06: a committed success cannot be replaced while private cleanup is still
// pending. Neither late operation may drop the publication pin or private owner.
func TestWorkingInstanceHandoffSuccessWinsLateOutcomes(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	_, err := f.q.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
	handoffCheck(t, err)
	before := mainPathJournalRows(t, f.db)
	_, err = f.q.RequestJobCancellation(t.Context(), f.session.JobID, mainPathJournalUTC)
	if !errors.Is(err, queue.ErrJobCancellationRejected) || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("late cancellation replaced success", err)
	}
	_, err = f.q.SelectLiquibaseJobFailure(t.Context(), f.session.JobID, handoffFailure(), mainPathJournalUTC)
	if !errors.Is(err, queue.ErrLiquibaseFinalization) || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("late failure replaced success", err)
	}
}

// WH05: a caller cannot invoke a physical verifier without a matching committed
// transfer. The existing successful-retirement test is the positive control.
func TestWorkingInstanceHandoffCleanupRequiresExactOwner(t *testing.T) {
	for _, variant := range []string{"no-receipt", "foreign-session", "foreign-generation", "foreign-runner", "foreign-cursor"} {
		t.Run(variant, func(t *testing.T) {
			f := newWorkingHandoffFixture(t)
			f.ready(t)
			if variant != "no-receipt" {
				_, err := f.q.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
				handoffCheck(t, err)
			}
			session := f.session
			switch variant {
			case "foreign-session":
				session.SessionID = "foreign"
			case "foreign-generation":
				session.WorkerGeneration++
			case "foreign-runner":
				session.RunnerID = strings.Repeat("1", 64)
			case "foreign-cursor":
				session.CursorID = strings.Repeat("1", 64)
			}
			before := mainPathJournalRows(t, f.db)
			probe := &handoffAbsence{db: f.db, session: f.session, database: f.binding.RuntimeRef}
			err := f.q.RetireTransferredLiquibaseSession(t.Context(), session, probe, mainPathJournalUTC)
			if err == nil || probe.calls != 0 || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
				t.Fatal("unproved owner reached physical cleanup or changed rows", err)
			}
		})
	}
}

// WH03: cancel only after SQLite actually updates prepare_instance, not before
// its statement. Both original and independent handles must observe a rollback.
func TestWorkingInstanceHandoffCancellationAfterTaskWrite(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	var file string
	handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
	control := &handoffReplyConnector{path: file, native: &sqlitedriver.Driver{}}
	db := sql.OpenDB(control)
	db.SetMaxOpenConns(1)
	defer db.Close()
	q, err := queue.New(db)
	handoffCheck(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	control.cancel = cancel
	control.cancelAfter = "UPDATE prepare_tasks SET status='succeeded'"
	before := mainPathJournalRows(t, f.db)
	_, err = q.PublishWorkingInstanceAndBeginFinalization(ctx, f.session, f.entry, f.binding, f.op)
	if !errors.Is(err, context.Canceled) || control.cancellations != 1 {
		t.Fatal("did not cancel after actual task UPDATE", err)
	}
	if !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) || !reflect.DeepEqual(before, mainPathJournalRows(t, db)) {
		t.Fatal("task-boundary cancellation exposed a partial transfer")
	}
}

// WH05 limits failures to the new private-retirement transaction. Even after
// physical absence is observed, a rollback must keep the exact live DB pinned;
// a lost reply requires complete durable evidence, not a partial success.
func TestWorkingInstanceHandoffPrivateRetirementCommit(t *testing.T) {
	for _, fault := range []string{"after-logical-release", "commit-busy", "lost-reply", "lost-reply-unreadable"} {
		t.Run(fault, func(t *testing.T) {
			f := newWorkingHandoffFixture(t)
			f.ready(t)
			_, err := f.q.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
			handoffCheck(t, err)
			var file string
			handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
			control := &handoffReplyConnector{path: file, native: &sqlitedriver.Driver{}}
			db := sql.OpenDB(control)
			db.SetMaxOpenConns(1)
			defer db.Close()
			q, err := queue.New(db)
			handoffCheck(t, err)
			var release func()
			switch fault {
			case "after-logical-release":
				_, err := f.db.Exec(`CREATE TRIGGER wh_retirement_refuse AFTER UPDATE OF runner_phase ON prepare_liquibase_sessions
WHEN NEW.session_id='handoff-session' AND NEW.runner_phase='retired'
BEGIN SELECT RAISE(ABORT,'WH exact private retirement boundary'); END`)
				handoffCheck(t, err)
				release = func() { _, err := f.db.Exec("DROP TRIGGER wh_retirement_refuse"); handoffCheck(t, err) }
			case "commit-busy":
				reader, err := f.db.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
				handoffCheck(t, err)
				defer reader.Rollback()
				var count int
				handoffCheck(t, reader.QueryRow("SELECT count(*) FROM instances").Scan(&count))
				release = func() { handoffCheck(t, reader.Rollback()) }
			case "lost-reply", "lost-reply-unreadable":
				control.armed = true
				if fault == "lost-reply-unreadable" {
					control.field = "unavailable"
				}
			}
			// Read through the writer handle before acquiring the peer's lock;
			// the peer pool intentionally has just one connection.
			before := mainPathJournalRows(t, db)
			probe := &handoffAbsence{db: db, session: f.session, database: f.binding.RuntimeRef}
			err = q.RetireTransferredLiquibaseSession(t.Context(), f.session, probe, mainPathJournalUTC)
			if probe.calls != 1 {
				t.Fatal("retirement did not reach the exact physical-absence boundary")
			}
			if release != nil {
				if fault == "after-logical-release" {
					if err == nil || !strings.Contains(err.Error(), "WH exact private retirement boundary") {
						t.Fatal("retirement failed before its intended logical boundary", err)
					}
				} else {
					var coded interface{ Code() int }
					if !errors.As(err, &coded) || coded.Code() != 5 {
						t.Fatal("retirement did not reach real COMMIT BUSY", err)
					}
				}
				if !reflect.DeepEqual(before, mainPathJournalRows(t, db)) {
					t.Fatal("failed retirement exposed partial rows on its original handle")
				}
				release()
				if !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
					t.Fatal("peer observed partial retirement")
				}
				handoffCheck(t, q.RetireTransferredLiquibaseSession(t.Context(), f.session, probe, mainPathJournalUTC))
			} else {
				if control.commits != 1 {
					t.Fatal("lost reply did not follow exactly one real COMMIT")
				}
				if fault == "lost-reply" {
					handoffCheck(t, err)
				} else if err == nil || !errors.Is(err, errHandoffLostReply) || control.reads == 0 {
					t.Fatal("unavailable retirement evidence became success", err)
				}
			}
			var pin, phase string
			var slots, instances int
			handoffCheck(t, f.db.QueryRow("SELECT instance_pin FROM prepare_liquibase_job_finalizations WHERE job_id=?", f.session.JobID).Scan(&pin))
			handoffCheck(t, f.db.QueryRow("SELECT runner_phase FROM prepare_liquibase_sessions WHERE session_id=?", f.session.SessionID).Scan(&phase))
			handoffCheck(t, f.db.QueryRow("SELECT count(*) FROM prepare_liquibase_capture_slots WHERE session_id=?", f.session.SessionID).Scan(&slots))
			handoffCheck(t, f.db.QueryRow("SELECT count(*) FROM instances WHERE instance_id=? AND runtime_id=?", f.entry.InstanceID, f.binding.RuntimeRef).Scan(&instances))
			if pin != f.entry.InstanceID || phase != "retired" || slots != 0 || instances != 1 {
				t.Fatal("private retirement lost the live instance or its bounded publication protection")
			}
		})
	}
}
