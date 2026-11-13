package queue_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	sqlitedriver "modernc.org/sqlite"
)

// CP-PG03 instruments real SQLite: an unreadable proof or refusal of any
// transactional metadata effect grants no authority and rolls back all rows.
type planningDBFault struct {
	query, exec, rows, column string
	value                     driver.Value
	hit                       bool
	path                      string
	values                    map[string]driver.Value
	whenID                    int64
	begin                     bool
	skip                      int
	indexes                   map[int]driver.Value
	cancel                    context.CancelFunc
	rollback                  bool
	changed                   int
}

func (f *planningDBFault) Driver() driver.Driver { return &sqlitedriver.Driver{} }
func (f *planningDBFault) Connect(ctx context.Context) (driver.Conn, error) {
	c, e := f.Driver().Open(f.path)
	if e != nil {
		return nil, e
	}
	_, e = c.(driver.ExecerContext).ExecContext(ctx, "PRAGMA foreign_keys=ON", nil)
	if e != nil {
		c.Close()
		return nil, e
	}
	return &planningDBFaultConn{Conn: c, f: f}, nil
}

type planningDBFaultConn struct {
	driver.Conn
	f *planningDBFault
}

func (c *planningDBFaultConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	if c.f.begin && !c.f.hit {
		c.f.hit = true
		return nil, errPlanningDBRead
	}
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, o)
	if err != nil {
		return nil, err
	}
	return &planningDBFaultTx{Tx: tx, f: c.f}, nil
}

type planningDBFaultTx struct {
	driver.Tx
	f *planningDBFault
}

func (tx *planningDBFaultTx) Rollback() error {
	err := tx.Tx.Rollback()
	if tx.f.rollback && !tx.f.hit {
		tx.f.hit = true
		return errPlanningDBRead
	}
	return err
}

func TestPlanningDBSuffixReadRefusal(t *testing.T) {
	for _, variant := range []string{"query", "stream", "scan", "input", "metadata", "empty"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			f.retireCapture(t)
			launch := f.firstLaunch(t)
			q, c := planningDBFaultQueue(t, f)
			c.query = "SELECT job_id,task_id,position,type,status,planner_kind"
			switch variant {
			case "stream":
				c.rows = "next"
			case "scan":
				c.column = "position"
				c.value = "invalid-position"
			case "input":
				c.column = "input_id"
				c.value = "foreign"
			case "metadata":
				c.column = "changeset_id"
				c.value = "foreign"
			case "empty":
				c.column = "type"
				c.value = "foreign"
			}
			before := mainPathJournalRows(t, f.db)
			_, e := q.TransferPlanningDBToFirstSession(t.Context(), launch)
			if e == nil || !c.hit {
				t.Fatal("unreadable suffix authorised transfer", e, c.hit)
			}
			f.unchanged(t, before)
			_, e = q.TransferPlanningDBToFirstSession(t.Context(), launch)
			handoffCheck(t, e)
		})
	}
}

type planningDBVerifyFunc func(context.Context, queue.ClonePlan, string) error

func (v planningDBVerifyFunc) VerifyReleasedClone(ctx context.Context, c queue.ClonePlan, id string) error {
	return v(ctx, c, id)
}

func TestPlanningDBDisposalReadRefusal(t *testing.T) {
	for _, variant := range []string{"begin", "owner", "operation", "request-fact", "second-read"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			_, e := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
			handoffCheck(t, e)
			id, e := f.q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
			handoffCheck(t, e)
			handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
			f.op.Stage = "retired"
			q, c := planningDBFaultQueue(t, f)
			switch variant {
			case "begin":
				c.begin = true
			case "owner":
				c.query = "SELECT job_id,task_id,attempt_no,clone_id,retained_evidence_id"
			case "operation":
				c.query = "SELECT operation_ref,target_path,source_ref"
			case "request-fact":
				c.query = "SELECT facts_json,scope_digest,facts_digest"
				c.whenID = id
			}
			absence := &planningDBAbsence{f: f, request: id}
			v := planningDBVerifyFunc(func(ctx context.Context, clone queue.ClonePlan, runtime string) error {
				if e := absence.VerifyReleasedClone(ctx, clone, runtime); e != nil {
					return e
				}
				if variant == "second-read" && !c.hit {
					c.query = "SELECT job_id,task_id,attempt_no,clone_id,retained_evidence_id"
				}
				return nil
			})
			before := mainPathJournalRows(t, f.db)
			e = q.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, v, mainPathJournalUTC)
			if e == nil || !c.hit {
				t.Fatal("unreadable disposal proof authorised completion", e, c.hit)
			}
			f.unchanged(t, before)
			handoffCheck(t, q.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, absence, mainPathJournalUTC))
		})
	}
}
func (c *planningDBFaultConn) ExecContext(ctx context.Context, q string, v []driver.NamedValue) (driver.Result, error) {
	if c.f.exec != "" && !c.f.hit && strings.Contains(q, c.f.exec) {
		c.f.hit = true
		return nil, errPlanningDBRead
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, v)
}
func (c *planningDBFaultConn) QueryContext(ctx context.Context, q string, v []driver.NamedValue) (driver.Rows, error) {
	idMatch := c.f.whenID == 0 || len(v) > 0 && v[0].Value == c.f.whenID
	if c.f.query != "" && !c.f.hit && idMatch && strings.Contains(q, c.f.query) {
		if c.f.skip > 0 {
			c.f.skip--
			return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, v)
		}
		c.f.hit = true
		if c.f.rows == "" && c.f.column == "" && c.f.values == nil && c.f.indexes == nil {
			return nil, errPlanningDBRead
		}
		rows, e := c.Conn.(driver.QueryerContext).QueryContext(ctx, q, v)
		if e != nil {
			return nil, e
		}
		return &planningDBFaultRows{Rows: rows, f: c.f}, nil
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, v)
}

type planningDBFaultRows struct {
	driver.Rows
	f *planningDBFault
}

func (r *planningDBFaultRows) Next(v []driver.Value) error {
	if r.f.rows == "cancel" {
		r.f.cancel()
		return context.Canceled
	}
	if r.f.rows == "next" {
		return errPlanningDBRead
	}
	e := r.Rows.Next(v)
	if e == nil {
		for i, name := range r.Columns() {
			if value, ok := r.f.indexes[i]; ok {
				v[i] = value
				r.f.changed++
			}
			if value, ok := r.f.values[name]; ok {
				v[i] = value
			}
			if name == r.f.column {
				v[i] = r.f.value
			}
		}
	}
	return e
}
func (r *planningDBFaultRows) Close() error {
	e := r.Rows.Close()
	if r.f.rows == "close" {
		return errPlanningDBRead
	}
	return e
}

func planningDBFaultQueue(t *testing.T, f *planningDBFixture) (*queue.SQLiteStore, *planningDBFault) {
	t.Helper()
	control := &planningDBFault{}
	handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&control.path))
	db := sql.OpenDB(control)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	q, e := queue.New(db)
	handoffCheck(t, e)
	return q, control
}

func TestPlanningDBMetadataEffectRefusal(t *testing.T) {
	cases := map[string][]string{
		"completion": {"INSERT INTO prepare_attempt_decisions", "INSERT INTO prepare_decision_evidence", "UPDATE prepare_step_attempts SET phase='closed'", "UPDATE prepare_tasks SET status='succeeded'"},
		"retention":  {"INSERT INTO prepare_attempt_evidence"},
		"transfer":   {"INSERT INTO prepare_artifact_leases", "INSERT INTO prepare_liquibase_sessions"},
		"request":    {"INSERT INTO prepare_attempt_evidence", "UPDATE prepare_liquibase_planning_db_ownership SET disposal_requested"},
		"disposal":   {"INSERT INTO prepare_attempt_evidence", "UPDATE prepare_liquibase_planning_db_ownership SET disposal_evidence", "UPDATE prepare_clones SET phase='released'"},
	}
	for writer, effects := range cases {
		for _, effect := range effects {
			t.Run(writer+"/"+effect, func(t *testing.T) {
				f := newPlanningDBFixture(t, 0)
				var launch queue.LiquibaseSessionLaunch
				var request int64
				if writer == "completion" {
					f.saveRetainedPlan(t)
				} else if writer != "retention" {
					f.complete(t)
					f.retireCapture(t)
					if writer == "transfer" {
						launch = f.firstLaunch(t)
					} else {
						_, e := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
						handoffCheck(t, e)
						if writer == "disposal" {
							request, e = f.q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
							handoffCheck(t, e)
							handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
							f.op.Stage = "retired"
						}
					}
				}
				q, control := planningDBFaultQueue(t, f)
				control.exec = effect
				invoke := func() error {
					switch writer {
					case "retention":
						_, e := q.RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC)
						return e
					case "completion":
						return q.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC)
					case "transfer":
						_, e := q.TransferPlanningDBToFirstSession(t.Context(), launch)
						return e
					case "request":
						_, e := q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
						return e
					default:
						return q.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, &planningDBAbsence{f: f, request: request}, mainPathJournalUTC)
					}
				}
				before := mainPathJournalRows(t, f.db)
				if e := invoke(); e == nil || !control.hit {
					t.Fatal("did not refuse the exact metadata boundary", e, control.hit)
				}
				f.unchanged(t, before)
				handoffCheck(t, invoke())
			})
		}
	}
}

func TestPlanningDBUnreadableSnapshot(t *testing.T) {
	cases := []struct{ query, rows string }{
		{"SELECT * FROM prepare_liquibase_planning_db_ownership", ""},
		{"SELECT * FROM prepare_jobs", "next"},
		{"SELECT * FROM prepare_tasks", "close"},
		{"SELECT count(*) FROM prepare_jobs j JOIN liquibase_job_bindings", ""},
		{"SELECT operation_ref,target_path,source_ref", ""},
		{"SELECT facts_json,scope_digest,facts_digest", ""},
	}
	for _, tc := range cases {
		t.Run(tc.query+tc.rows, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.saveRetainedPlan(t)
			q, control := planningDBFaultQueue(t, f)
			control.query, control.rows = tc.query, tc.rows
			before := mainPathJournalRows(t, f.db)
			e := q.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC)
			if e == nil || !control.hit {
				t.Fatal("unreadable complete proof accepted", e, control.hit)
			}
			f.unchanged(t, before)
			handoffCheck(t, q.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
		})
	}
}

func TestPlanningDBStoredIdentityReadRefusal(t *testing.T) {
	for _, column := range []string{"identity_json", "owner-clone", "plan_digest", "source_ref", "runtime_ref"} {
		t.Run(column, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			f.retireCapture(t)
			launch := f.firstLaunch(t)
			q, c := planningDBFaultQueue(t, f)
			c.query = "SELECT operation_ref,target_path,source_ref"
			c.column = column
			c.value = "foreign"
			if column == "owner-clone" || column == "plan_digest" {
				c.query = "SELECT job_id,task_id,attempt_no,clone_id,retained_evidence_id"
				if column == "owner-clone" {
					c.column = "clone_id"
				}
			} // first saved owner must match the original clone
			before := mainPathJournalRows(t, f.db)
			_, e := q.TransferPlanningDBToFirstSession(t.Context(), launch)
			if e == nil || !c.hit {
				t.Fatal("changed stored identity granted transfer", e, c.hit)
			}
			f.unchanged(t, before)
			_, e = q.TransferPlanningDBToFirstSession(t.Context(), launch)
			handoffCheck(t, e)
		})
	}
}

// Scope-valid but malformed/noncanonical observed identity must fail before
// completion; an unreadable exit proof is not replaced by the retention fact.
func TestPlanningDBUnreadableCompletionEvidence(t *testing.T) {
	for _, variant := range []string{"typed-retention", "extra-retention", "exit-scope", "output-scope"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.saveRetainedPlan(t)
			q, c := planningDBFaultQueue(t, f)
			c.query = "SELECT facts_json,scope_digest,facts_digest"
			c.whenID = f.retained
			if variant == "exit-scope" || variant == "output-scope" {
				c.whenID = f.exit
				if variant == "output-scope" {
					c.whenID = f.output
				}
				c.column = "scope_digest"
				c.value = pgDigest("f")
			} else {
				var raw string
				handoffCheck(t, f.db.QueryRow("SELECT facts_json FROM prepare_attempt_evidence WHERE evidence_id=?", f.retained).Scan(&raw))
				var fact map[string]any
				handoffCheck(t, json.Unmarshal([]byte(raw), &fact))
				obs := fact["observation"].(map[string]any)
				if variant == "typed-retention" {
					obs["clone_generation"] = "wrong-type"
				} else {
					obs["unproved-extra"] = true
				}
				encoded, e := json.Marshal(fact)
				handoffCheck(t, e)
				c.values = map[string]driver.Value{"facts_json": string(encoded), "facts_digest": planningDBFactDigest(string(encoded))}
			}
			before := mainPathJournalRows(t, f.db)
			e := q.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC)
			if e == nil || !c.hit {
				t.Fatal("unproved evidence granted completion", e, c.hit)
			}
			f.unchanged(t, before)
			handoffCheck(t, q.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
		})
	}
}

func TestPlanningDBRequestIdempotenceAndValidation(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	_, e := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
	handoffCheck(t, e)
	id, e := f.q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
	handoffCheck(t, e)
	before := mainPathJournalRows(t, f.db)
	again, e := f.q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
	handoffCheck(t, e)
	if again != id {
		t.Fatal("duplicate authority")
	}
	f.unchanged(t, before)
	var nilQueue *queue.SQLiteStore
	for _, q := range []*queue.SQLiteStore{nilQueue, f.q} {
		if _, e = q.RecordPlanningDBRetention(t.Context(), f.attempt, 2, f.op, f.observe, mainPathJournalUTC); e == nil {
			t.Fatal("invalid retention")
		}
		if e = q.CompleteRetainedPlanning(t.Context(), f.attempt, 2, f.exit, f.retained, f.output, mainPathJournalUTC); e == nil {
			t.Fatal("invalid completion")
		}
		if _, e = q.RequestPlanningDBDisposal(t.Context(), f.attempt, 2, "accepted_cancel", mainPathJournalUTC); e == nil {
			t.Fatal("invalid request")
		}
		if e = q.CompletePlanningDBDisposal(t.Context(), f.attempt, 2, f.op, nil, mainPathJournalUTC); e == nil {
			t.Fatal("invalid disposal")
		}
		if _, e = q.TransferPlanningDBToFirstSession(t.Context(), queue.LiquibaseSessionLaunch{}); e == nil {
			t.Fatal("invalid transfer")
		}
	}
	f.unchanged(t, before)
}
