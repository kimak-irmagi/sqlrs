package sqliteconn

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"modernc.org/sqlite"
)

var errConnectSetting = errors.New("observed configuration failure")
var errConnectClose = errors.New("observed close failure")

// connectObservationDriver observes FC01/FC02 using the pinned real SQLite.
// Faults select semantic configuration boundaries, never arbitrary SQL counts.
// Requirements: liquibase-finalization-connection-ownership.md §6.
type connectObservationDriver struct {
	mu                      sync.Mutex
	point, target           string
	closeFault, closeBefore bool
	cancel                  context.CancelFunc
	opened                  int
	connections             []*connectObservationConn
}

func (d *connectObservationDriver) Open(path string) (driver.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.opened++
	if d.point == "open" {
		return nil, errConnectSetting
	}
	real, err := (&sqlite.Driver{}).Open(path)
	if err != nil {
		return nil, err
	}
	c := &connectObservationConn{Conn: real, owner: d}
	d.connections = append(d.connections, c)
	if d.point == "cancel-open" {
		d.cancel()
	}
	if d.point == "missing-exec" {
		return &connectQueryOnly{Conn: c}, nil
	}
	if d.point == "missing-query" {
		return &connectExecOnly{Conn: c}, nil
	}
	return c, nil
}

// cleanup releases only test-owned native resources after a deliberately failed
// Close. It is never counted as product cleanup or used before assertions.
func (d *connectObservationDriver) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range d.connections {
		if !c.nativeClosed {
			_ = c.Conn.Close()
			c.nativeClosed = true
		}
	}
}

type connectObservationConn struct {
	driver.Conn
	owner          *connectObservationDriver
	closes         int
	nativeClosed   bool
	verified       []string
	openRows       int
	closedWithRows bool
}

func (c *connectObservationConn) Close() error {
	d := c.owner
	d.mu.Lock()
	defer d.mu.Unlock()
	c.closes++
	if c.openRows != 0 {
		c.closedWithRows = true
	}
	if d.closeFault && d.closeBefore {
		return errConnectClose
	}
	err := c.Conn.Close()
	c.nativeClosed = true
	if d.closeFault {
		return errors.Join(err, errConnectClose)
	}
	return err
}

func (c *connectObservationConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	d := c.owner
	d.mu.Lock()
	point, target, cancel := d.point, d.target, d.cancel
	d.mu.Unlock()
	match := query == "PRAGMA "+target
	if match && point == "exec-before" {
		return nil, errConnectSetting
	}
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	if match && point == "cancel-exec" {
		cancel()
	}
	if err == nil && match && point == "exec-after" {
		return nil, errConnectSetting
	}
	return result, err
}

func (c *connectObservationConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	d := c.owner
	d.mu.Lock()
	point, target, cancel := d.point, d.target, d.cancel
	if strings.HasPrefix(query, "PRAGMA ") {
		c.verified = append(c.verified, query)
	}
	d.mu.Unlock()
	match := query == "PRAGMA "+target
	if match && point == "query" {
		return nil, errConnectSetting
	}
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	c.openRows++
	d.mu.Unlock()
	rowPoint := ""
	if match && strings.HasPrefix(point, "rows-") {
		rowPoint = point
	}
	if match && point == "cancel-query" {
		cancel()
	}
	return &connectObservationRows{Rows: rows, point: rowPoint, owner: c}, nil
}

func (c *connectObservationConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
func (c *connectObservationConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
}
func (c *connectObservationConn) Ping(ctx context.Context) error {
	return c.Conn.(driver.Pinger).Ping(ctx)
}

// Capability-negative wrappers intentionally omit exactly one configuration
// interface, while still forwarding native ownership/Close to the observation.
type connectQueryOnly struct{ driver.Conn }

func (c *connectQueryOnly) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

type connectExecOnly struct{ driver.Conn }

func (c *connectExecOnly) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

type connectObservationRows struct {
	driver.Rows
	point  string
	seen   bool
	owner  *connectObservationConn
	closed bool
}

func (r *connectObservationRows) Next(values []driver.Value) error {
	if r.point == "rows-late-error" && r.seen {
		return errConnectSetting
	}
	if r.point == "rows-error" {
		return errConnectSetting
	}
	if r.point == "rows-empty" {
		return io.EOF
	}
	if r.point == "rows-extra" && r.seen {
		values[0] = int64(0)
		return nil
	}
	err := r.Rows.Next(values)
	if err == nil && r.point == "rows-value" {
		values[0] = int64(9)
	}
	if err == nil && r.point == "rows-type" {
		values[0] = "not an integer"
	}
	r.seen = true
	return err
}
func (r *connectObservationRows) Close() error {
	err := r.Rows.Close()
	r.owner.owner.mu.Lock()
	if !r.closed {
		r.owner.openRows--
		r.closed = true
	}
	r.owner.owner.mu.Unlock()
	if r.point == "rows-close" {
		return errors.Join(err, errConnectSetting)
	}
	return err
}

// TestConnectorConfigurationRefusal covers approved FC02 plus §6's explicit
// capability/cancellation ownership rules. Each cut retains real opening and
// configuration effects outside that cut; a later independent Connect retries.
func TestConnectorConfigurationRefusal(t *testing.T) {
	cases := []struct{ point, target string }{{"open", ""}, {"missing-exec", ""}, {"missing-query", ""}}
	for _, p := range []struct{ name, setting string }{{"foreign_keys", "foreign_keys=ON"}, {"busy_timeout", "busy_timeout=0"}, {"read_uncommitted", "read_uncommitted=0"}} {
		for _, point := range []string{"exec-before", "exec-after", "cancel-exec"} {
			cases = append(cases, struct{ point, target string }{point, p.setting})
		}
		for _, point := range []string{"query", "rows-value", "rows-type", "rows-empty", "rows-extra", "rows-error", "rows-late-error", "rows-close", "cancel-query"} {
			cases = append(cases, struct{ point, target string }{point, p.name})
		}
	}
	cases = append(cases, struct{ point, target string }{"cancel-open", ""})
	for _, tc := range cases {
		t.Run(tc.point+"/"+tc.target, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			d := &connectObservationDriver{point: tc.point, target: tc.target, cancel: cancel}
			path := filepath.Join(t.TempDir(), "db.sqlite")
			t.Cleanup(d.cleanup)
			connector := newConnector(path, d)
			if connector.Driver() != d {
				t.Fatal("different source returned by Driver")
			}
			got, err := connector.Connect(ctx)
			if got != nil || err == nil {
				t.Fatalf("FC02 connection escaped refusal: conn=%T err=%v", got, err)
			}
			if strings.HasPrefix(tc.point, "cancel-") && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation cause lost", err)
			}
			if tc.point == "open" || tc.point == "query" || tc.point == "rows-error" || tc.point == "rows-late-error" || tc.point == "rows-close" || strings.HasPrefix(tc.point, "exec-") {
				if !errors.Is(err, errConnectSetting) {
					t.Fatal("original cause lost", err)
				}
			}
			d.mu.Lock()
			if d.opened != 1 {
				t.Error("FC02 opening retried inside Connect", d.opened)
			}
			for _, c := range d.connections {
				if c.closes != 1 || !c.nativeClosed || c.closedWithRows || c.openRows != 0 {
					t.Error("FC02 refused connection not closed exactly once", c.closes, c.nativeClosed)
				}
			}
			d.point, d.target = "", ""
			d.mu.Unlock()
			next, err := connector.Connect(t.Context())
			if err != nil {
				t.Fatal("independent subsequent Connect failed", err)
			}
			assertConnectSettings(t, next)
			if err := next.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConnectorRefusalPreservesCloseError(t *testing.T) {
	for _, before := range []bool{false, true} {
		t.Run(map[bool]string{false: "after-close", true: "before-close"}[before], func(t *testing.T) {
			d := &connectObservationDriver{point: "exec-before", target: "foreign_keys=ON", closeFault: true, closeBefore: before}
			path := filepath.Join(t.TempDir(), "db.sqlite")
			t.Cleanup(d.cleanup)
			got, err := newConnector(path, d).Connect(t.Context())
			if got != nil || !errors.Is(err, errConnectSetting) || !errors.Is(err, errConnectClose) {
				t.Fatal("FC02 setting/close cause lost or connection handed out", err)
			}
			if d.opened != 1 || len(d.connections) != 1 || d.connections[0].closes != 1 {
				t.Fatal("FC02 duplicate open/close on refusal")
			}
			if d.connections[0].nativeClosed == before {
				t.Fatal("test did not observe selected native close effect")
			}
		})
	}
}

func TestConnectorCancelledBeforeOpen(t *testing.T) {
	d := &connectObservationDriver{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := newConnector(filepath.Join(t.TempDir(), "db.sqlite"), d).Connect(ctx)
	if got != nil || !errors.Is(err, context.Canceled) || d.opened != 0 {
		t.Fatal("FC02 cancelled acquisition opened a connection", err, d.opened)
	}
}

// assertConnectSettings reads real pragma results only; response-corruption
// faults are disabled before calling it. It does not set or repair any setting.
func assertConnectSettings(t *testing.T, c driver.Conn) {
	t.Helper()
	for _, p := range []struct {
		name string
		want int64
	}{{"foreign_keys", 1}, {"busy_timeout", 0}, {"read_uncommitted", 0}} {
		rows, err := c.(driver.QueryerContext).QueryContext(t.Context(), "PRAGMA "+p.name, nil)
		if err != nil {
			t.Fatal(err)
		}
		values := make([]driver.Value, 1)
		err = rows.Next(values)
		closeErr := rows.Close()
		if err != nil || closeErr != nil || values[0] != p.want {
			t.Fatalf("FC01 %s=%v err=%v close=%v", p.name, values[0], err, closeErr)
		}
	}
}

func TestConnectorFirstAndReplacement(t *testing.T) {
	d := &connectObservationDriver{}
	path := filepath.Join(t.TempDir(), "db.sqlite")
	t.Cleanup(d.cleanup)
	db := sql.OpenDB(newConnector(path, d))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	for generation := 0; generation < 2; generation++ {
		c, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Raw(func(v any) error { assertConnectSettings(t, v.(driver.Conn)); return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err := c.ExecContext(t.Context(), "CREATE TABLE IF NOT EXISTS parent(id INTEGER PRIMARY KEY); CREATE TABLE IF NOT EXISTS child(id INTEGER REFERENCES parent(id))"); err != nil {
			t.Fatal(err)
		}
		_, err = c.ExecContext(t.Context(), "INSERT INTO child VALUES(42)")
		var coded interface{ Code() int }
		if !errors.As(err, &coded) || coded.Code() != 787 {
			t.Fatal("FC01 real FK not enforced", err)
		}
		if generation == 0 {
			if err := c.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(err, driver.ErrBadConn) {
				t.Fatal(err)
			}
		} else {
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if d.opened != 2 || d.connections[0].closes != 1 || d.connections[0] == d.connections[1] {
		t.Fatal("FC01 real replacement not observed")
	}
}

func TestConnectorConcurrentConnects(t *testing.T) {
	d := &connectObservationDriver{}
	path := filepath.Join(t.TempDir(), "db.sqlite")
	t.Cleanup(d.cleanup)
	connector := newConnector(path, d)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := connector.Connect(t.Context())
			if err != nil {
				t.Error(err)
				return
			}
			assertConnectSettings(t, c)
			if err := c.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if d.opened != 8 {
		t.Fatal("FC01 connections were shared", d.opened)
	}
	for _, c := range d.connections {
		if c.closes != 1 || !c.nativeClosed {
			t.Fatal("FC01 ownership crossed connections")
		}
	}
}

func TestConfiguredOpenReturnsUsablePool(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if db.Stats().MaxOpenConnections != 1 {
		t.Fatal("FC03 pool is not bounded to one")
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), "CREATE TABLE control(id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	stmt, err := db.PrepareContext(t.Context(), "SELECT count(*) FROM control")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := stmt.QueryRowContext(t.Context()).Scan(&count); err != nil || count != 0 {
		t.Fatal("context statement control failed", err, count)
	}
	if err := stmt.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestConnectorWrapperContextTransparency is §6.4's positive control: injected
// drivers must retain the real context-aware transaction/statement/Ping path.
func TestConnectorWrapperContextTransparency(t *testing.T) {
	d := &connectObservationDriver{}
	path := filepath.Join(t.TempDir(), "db.sqlite")
	t.Cleanup(d.cleanup)
	db := sql.OpenDB(newConnector(path, d))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), "CREATE TABLE control(value INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	stmt, err := db.PrepareContext(t.Context(), "SELECT count(*) FROM control")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	err = stmt.QueryRowContext(t.Context()).Scan(&count)
	closeErr := stmt.Close()
	if err != nil || closeErr != nil || count != 0 {
		t.Fatal("wrapper changed context statement", err, closeErr, count)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.opened != 1 || len(d.connections) != 1 || d.connections[0].closes != 0 || d.connections[0].openRows != 0 {
		t.Fatal("wrapper changed ownership or leaked configuration rows")
	}
}
