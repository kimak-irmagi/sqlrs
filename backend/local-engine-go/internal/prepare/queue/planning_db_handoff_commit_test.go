package queue_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/sqliteconn"
	sqlitedriver "modernc.org/sqlite"
)

var errPlanningDBWrite = errors.New("PG04 refused after real metadata effect")
var errPlanningDBReply = errors.New("PG04 lost real COMMIT reply")
var errPlanningDBRead = errors.New("PG04 settlement unavailable")

// Test-only transparent driver; every Begin/Commit/row comes from actual SQLite.
// Faults are armed after all fixture preparation. There is no product hook or
// changed behavior in any old boundary wrapper. PG04, storage schema §§3–5.
type planningDBCommitConnector struct {
	path, mode              string
	native                  driver.Driver
	armed, lost             bool
	effects, commits, reads int
	commitCode              int
}

func (c *planningDBCommitConnector) Driver() driver.Driver { return c.native }
func (c *planningDBCommitConnector) Connect(context.Context) (driver.Conn, error) {
	raw, err := c.native.Open(c.path)
	if err != nil {
		return nil, err
	}
	if _, err := raw.(driver.ExecerContext).ExecContext(context.Background(), "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=0; PRAGMA read_uncommitted=0", nil); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return &planningDBCommitConn{Conn: raw, owner: c}, nil
}

type planningDBCommitConn struct {
	driver.Conn
	owner *planningDBCommitConnector
}

func (c *planningDBCommitConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *planningDBCommitConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &planningDBCommitTx{Tx: tx, owner: c.owner}, nil
}
func (c *planningDBCommitConn) ExecContext(ctx context.Context, text string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, text, args)
	upper := strings.ToUpper(strings.TrimSpace(text))
	if err == nil && c.owner.armed && c.owner.mode == "after-first-effect" && (strings.HasPrefix(upper, "INSERT ") || strings.HasPrefix(upper, "UPDATE ")) {
		c.owner.armed = false
		c.owner.effects++
		return nil, errPlanningDBWrite
	}
	return result, err
}
func (c *planningDBCommitConn) QueryContext(ctx context.Context, text string, args []driver.NamedValue) (driver.Rows, error) {
	if c.owner.lost && c.owner.mode == "unavailable" && strings.Contains(text, "prepare_liquibase_planning_db_ownership") {
		c.owner.reads++
		return nil, errPlanningDBRead
	}
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, text, args)
	if err != nil {
		return nil, err
	}
	return &planningDBCommitRows{Rows: rows, owner: c.owner}, nil
}

type planningDBCommitTx struct {
	driver.Tx
	owner *planningDBCommitConnector
}

func (tx *planningDBCommitTx) Commit() error {
	err := tx.Tx.Commit()
	if tx.owner.armed {
		tx.owner.commits++
		var code interface{ Code() int }
		if errors.As(err, &code) {
			tx.owner.commitCode = code.Code()
		}
		if err == nil && tx.owner.mode != "busy" {
			tx.owner.armed, tx.owner.lost = false, true
			return errPlanningDBReply
		}
	}
	return err
}

type planningDBCommitRows struct {
	driver.Rows
	owner *planningDBCommitConnector
}

func (r *planningDBCommitRows) Next(values []driver.Value) error {
	err := r.Rows.Next(values)
	if err != nil || !r.owner.lost || r.owner.mode != "partial" {
		return err
	}
	for i, name := range r.Columns() {
		if name == "plan_digest" {
			values[i] = pgDigest("f")
			r.owner.reads++
		}
	}
	return nil
}

// Only the three new metadata writers are multiplied by these five boundaries.
// A reader really holds a rollback-journal SHARED lock for COMMIT BUSY; no fake
// error code substitutes for that condition. Both handles are observed before
// Close, followed by two independently opened managed connections.
func TestPlanningDBHandoffCommit(t *testing.T) {
	for _, writer := range []string{"retained-completion", "first-session-transfer", "disposal-completion"} {
		for _, mode := range []string{"after-first-effect", "busy", "lost-reply", "unavailable", "partial"} {
			t.Run(writer+"/"+mode, func(t *testing.T) {
				f := newPlanningDBFixture(t, 0)
				var launch queue.LiquibaseSessionLaunch
				var request int64
				if writer == "retained-completion" {
					f.saveRetainedPlan(t)
				} else {
					f.complete(t)
					f.retireCapture(t)
					if writer == "first-session-transfer" {
						launch = f.firstLaunch(t)
					} else {
						_, err := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
						handoffCheck(t, err)
						w, ok := any(f.q).(planningDBDisposalWriter)
						if !ok {
							t.Fatal("approved retained DB disposal writers are not implemented")
						}
						request, err = w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
						handoffCheck(t, err)
						handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
						f.op.Stage = "retired"
					}
				}
				invoke := func(q *queue.SQLiteStore) error {
					switch writer {
					case "retained-completion":
						w, ok := any(q).(retainedPlanningWriter)
						if !ok {
							t.Fatal("approved retained-planning writers are not implemented")
						}
						return w.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC)
					case "first-session-transfer":
						w, ok := any(q).(planningDBTransferWriter)
						if !ok {
							t.Fatal("approved first-session transfer writer is not implemented")
						}
						_, err := w.TransferPlanningDBToFirstSession(t.Context(), launch)
						return err
					default:
						w, ok := any(q).(planningDBDisposalWriter)
						if !ok {
							t.Fatal("approved retained DB disposal writers are not implemented")
						}
						return w.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, &planningDBAbsence{f: f, request: request}, mainPathJournalUTC)
					}
				}
				var file, journal string
				handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
				handoffCheck(t, f.db.QueryRow("PRAGMA journal_mode=DELETE").Scan(&journal))
				if strings.ToLower(journal) != "delete" {
					t.Fatal("BUSY control requires a real rollback journal")
				}
				control := &planningDBCommitConnector{path: file, mode: mode, native: &sqlitedriver.Driver{}}
				db := sql.OpenDB(control)
				db.SetMaxOpenConns(1)
				defer db.Close()
				q, err := queue.New(db)
				handoffCheck(t, err)
				before := mainPathJournalRows(t, f.db)
				var reader *sql.Tx
				if mode == "busy" {
					lockDB, openErr := sqliteconn.Open(file)
					handoffCheck(t, openErr)
					defer lockDB.Close()
					reader, err = lockDB.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
					handoffCheck(t, err)
					var n int
					handoffCheck(t, reader.QueryRow("SELECT count(*) FROM prepare_jobs").Scan(&n))
					defer reader.Rollback()
				}
				control.armed = true
				err = invoke(q)
				if reader != nil {
					handoffCheck(t, reader.Rollback())
				}
				rolledBack := mode == "busy" || mode == "after-first-effect"
				if mode == "after-first-effect" && (control.effects != 1 || !errors.Is(err, errPlanningDBWrite)) {
					t.Fatal("did not reach the first real effect", control.effects, err)
				}
				if mode == "busy" && (control.commits != 1 || control.commitCode != 5 || err == nil) {
					t.Fatal("not a real COMMIT SQLITE_BUSY refusal", control.commits, control.commitCode, err)
				}
				if !rolledBack && (control.commits != 1 || !control.lost) {
					t.Fatal("did not lose a real committed response")
				}
				if mode == "lost-reply" {
					handoffCheck(t, err)
				}
				if (mode == "unavailable" || mode == "partial") && (err == nil || control.reads == 0 || !errors.Is(err, errPlanningDBReply)) {
					t.Fatal("partial settlement became success or did not reach its boundary", control.reads, err)
				}
				// Stop corrupting observations before comparing original and peer.
				control.armed, control.lost = false, false
				after := mainPathJournalRows(t, f.db)
				if !reflect.DeepEqual(after, mainPathJournalRows(t, db)) {
					t.Fatal("owned connection and peer disagree before Close")
				}
				if rolledBack && !reflect.DeepEqual(before, after) {
					t.Fatal("failed write retained a partial metadata effect")
				}
				if !rolledBack {
					assertPlanningDBCommitEffect(t, f, writer, launch, request)
					if reflect.DeepEqual(before, after) {
						t.Fatal("real commit had no complete metadata effect")
					}
					handoffCheck(t, invoke(f.q))
					f.unchanged(t, after)
				}
				handoffCheck(t, db.Close())
				for reopen := 0; reopen < 2; reopen++ {
					reopened, err := sqliteconn.Open(file)
					handoffCheck(t, err)
					if !reflect.DeepEqual(after, mainPathJournalRows(t, reopened)) {
						t.Fatal("reopen changed known durable outcome")
					}
					handoffCheck(t, reopened.Close())
				}
			})
		}
	}
}

var _ driver.Connector = (*planningDBCommitConnector)(nil)
var _ driver.ExecerContext = (*planningDBCommitConn)(nil)
var _ driver.QueryerContext = (*planningDBCommitConn)(nil)
var _ driver.ConnBeginTx = (*planningDBCommitConn)(nil)
var _ driver.Rows = (*planningDBCommitRows)(nil)

// PG06 has one additional admission check, not another failure matrix: an
// unclassifiable disposal-request COMMIT must return no destructive authority.
// The request really commits, then only its response/readback are unavailable.
func TestPlanningDBDisposalRequestIndeterminate(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	f.retireCapture(t)
	_, err := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
	handoffCheck(t, err)
	var file string
	handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
	control := &planningDBCommitConnector{path: file, mode: "unavailable", native: &sqlitedriver.Driver{}}
	db := sql.OpenDB(control)
	db.SetMaxOpenConns(1)
	defer db.Close()
	q, err := queue.New(db)
	handoffCheck(t, err)
	w, ok := any(q).(planningDBDisposalWriter)
	if !ok {
		t.Fatal("approved disposal request writer missing")
	}
	control.armed = true
	id, err := w.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
	if id != 0 || err == nil || !errors.Is(err, errPlanningDBReply) || control.commits != 1 || !control.lost || control.reads == 0 {
		t.Fatal("uncertain request granted cleanup authority or missed real boundary", id, err)
	}
	control.armed, control.lost = false, false
	if f.count(t, "SELECT count(*) FROM managed_runtime_operations WHERE operation_ref=? AND stage='running' AND runtime_ref=?", f.op.Ref, f.op.RuntimeRef) != 1 || f.count(t, "SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase='active'", f.clone.ID) != 1 || f.count(t, "SELECT count(*) FROM prepare_liquibase_sessions WHERE job_id=?", f.attempt.JobID) != 0 {
		t.Fatal("uncertain request retired/replaced the live DB")
	}
	if f.count(t, `SELECT count(*) FROM prepare_liquibase_planning_db_ownership WHERE job_id=? AND session_id IS NULL AND disposal_requested_evidence_id IS NOT NULL AND disposal_evidence_id IS NULL AND disposed_at IS NULL`, f.attempt.JobID) != 1 {
		t.Fatal("lost reply did not leave the exact pending owner")
	}
}
