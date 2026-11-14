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
	sqlitedriver "modernc.org/sqlite"
)

var errHandoffLostReply = errors.New("WH lost real commit reply")
var errHandoffReadback = errors.New("WH settlement read unavailable")

// Test-only transparent wrappers retain actual SQLite transactions and native
// outcomes. They inject a lost response AFTER real Commit, never product hooks.
type handoffReplyConnector struct {
	path           string
	native         driver.Driver
	armed, lost    bool
	field          string
	commits, reads int
	cancel         context.CancelFunc
	cancelAfter    string
	cancellations  int
}

func (c *handoffReplyConnector) Driver() driver.Driver { return c.native }
func (c *handoffReplyConnector) Connect(context.Context) (driver.Conn, error) {
	raw, err := c.native.Open(c.path)
	if err != nil {
		return nil, err
	}
	if _, err = raw.(driver.ExecerContext).ExecContext(context.Background(), "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=0; PRAGMA read_uncommitted=0", nil); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return &handoffReplyConn{Conn: raw, owner: c}, nil
}

type handoffReplyConn struct {
	driver.Conn
	owner *handoffReplyConnector
}

func (c *handoffReplyConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
	if err == nil && c.owner.cancel != nil && strings.Contains(q, c.owner.cancelAfter) {
		c.owner.cancellations++
		c.owner.cancel()
		c.owner.cancel = nil
	}
	return result, err
}
func (c *handoffReplyConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if c.owner.lost && c.owner.field == "unavailable" && strings.Contains(q, "FROM prepare_liquibase_job_finalizations") {
		c.owner.reads++
		return nil, errHandoffReadback
	}
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
	if err != nil {
		return nil, err
	}
	return &handoffReplyRows{Rows: rows, owner: c.owner}, nil
}
func (c *handoffReplyConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *handoffReplyConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &handoffReplyTx{Tx: tx, owner: c.owner}, nil
}

type handoffReplyTx struct {
	driver.Tx
	owner *handoffReplyConnector
}

func (t *handoffReplyTx) Commit() error {
	err := t.Tx.Commit()
	if t.owner.armed && err == nil {
		t.owner.armed = false
		t.owner.lost = true
		t.owner.commits++
		return errHandoffLostReply
	}
	return err
}

type handoffReplyRows struct {
	driver.Rows
	owner *handoffReplyConnector
}

func (r *handoffReplyRows) Next(values []driver.Value) error {
	err := r.Rows.Next(values)
	if err != nil || !r.owner.lost || r.owner.field == "" || r.owner.field == "unavailable" {
		return err
	}
	for i, name := range r.Rows.Columns() {
		if name == r.owner.field {
			values[i] = "foreign"
			r.owner.reads++
		}
	}
	return nil
}

// WH03: success requires complete committed evidence, not a matching outcome
// while one transfer identity is missing/foreign. A clean peer remains honest.
func TestWorkingInstanceHandoffCommitReply(t *testing.T) {
	for _, field := range []string{"", "unavailable", "handoff_job_id", "handoff_instance_id", "handoff_state_id", "handoff_operation_ref", "handoff_runtime_dir", "handoff_outcome_digest", "runner_id", "db_runtime_id"} {
		name := field
		if name == "" {
			name = "known_commit"
		}
		t.Run(name, func(t *testing.T) {
			f := newWorkingHandoffFixture(t)
			f.ready(t)
			var file string
			handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
			control := &handoffReplyConnector{path: file, native: &sqlitedriver.Driver{}, field: field}
			db := sql.OpenDB(control)
			db.SetMaxOpenConns(1)
			defer db.Close()
			q, err := queue.New(db)
			handoffCheck(t, err)
			control.armed = true
			_, err = q.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
			if control.commits != 1 {
				t.Fatal("lost-reply boundary was not reached exactly once")
			}
			if field == "" {
				handoffCheck(t, err)
			} else if err == nil || control.reads == 0 || !errors.Is(err, errHandoffLostReply) {
				t.Fatal("incomplete settlement became success or wrong boundary", err)
			}
			var instances, uses int
			handoffCheck(t, f.db.QueryRow("SELECT count(*) FROM instances WHERE instance_id=?", f.entry.InstanceID).Scan(&instances))
			handoffCheck(t, f.db.QueryRow("SELECT use_count FROM states WHERE state_id=?", f.entry.StateID).Scan(&uses))
			if instances != 1 || uses != 1 {
				t.Fatal("independent committed publication differs")
			}
			// A deliberate metadata retry through an honest queue proves this is
			// not permission for another native SQL application or instance.
			_, err = any(f.q).(handoffPublisher).PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
			handoffCheck(t, err)
			handoffCheck(t, f.db.QueryRow("SELECT use_count FROM states WHERE state_id=?", f.entry.StateID).Scan(&uses))
			if uses != 1 {
				t.Fatal("lost response doubled state use")
			}
		})
	}
}

var _ driver.Connector = (*handoffReplyConnector)(nil)
var _ driver.QueryerContext = (*handoffReplyConn)(nil)
var _ driver.ExecerContext = (*handoffReplyConn)(nil)
var _ driver.ConnBeginTx = (*handoffReplyConn)(nil)

func TestWorkingInstanceHandoffCancellationAfterWrite(t *testing.T) {
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
	control.cancelAfter = "INSERT INTO instances"
	before := mainPathJournalRows(t, f.db)
	_, err = q.PublishWorkingInstanceAndBeginFinalization(ctx, f.session, f.entry, f.binding, f.op)
	if !errors.Is(err, context.Canceled) || control.cancellations != 1 {
		t.Fatal("did not cancel after actual instance INSERT", err)
	}
	if !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) || !reflect.DeepEqual(before, mainPathJournalRows(t, db)) {
		t.Fatal("cancelled publication exposed partial transfer")
	}
}
