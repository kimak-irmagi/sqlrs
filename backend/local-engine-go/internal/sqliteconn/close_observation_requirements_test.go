package sqliteconn

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
)

var errObservedNativeClose = errors.New("native close marker")
var errObservedReporter = errors.New("reporter marker")

// closeTestNative retains real SQLite effects and cuts only the selected Close
// boundary. Cleanup of deliberately retained resources is never product evidence.
// Requirements: close-observation-tests.md OC03–OC06, OC10, §3.4.
type closeTestNative struct {
	driver.Conn
	mode       string
	calls      atomic.Int32
	entered    chan struct{}
	release    chan struct{}
	closed     bool
	marker     any
	failure    error
	blockAfter bool
}

func (c *closeTestNative) Close() error {
	c.calls.Add(1)
	if c.entered != nil && !c.blockAfter {
		close(c.entered)
		<-c.release
	}
	if c.mode == "before" {
		return c.failure
	}
	if c.mode == "panic" {
		panic(c.marker)
	}
	err := c.Conn.Close()
	c.closed = true
	if c.entered != nil && c.blockAfter {
		close(c.entered)
		<-c.release
	}
	if c.mode == "after" {
		return errors.Join(err, c.failure)
	}
	return err
}

func newCloseTestNative(t *testing.T, mode string) *closeTestNative {
	t.Helper()
	native, err := (&sqlite.Driver{}).Open(filepath.Join(t.TempDir(), "capture.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	c := &closeTestNative{Conn: native, mode: mode, marker: &struct{ name string }{"native"}, failure: errObservedNativeClose}
	t.Cleanup(func() {
		if !c.closed {
			_ = c.Conn.Close()
		}
	})
	return c
}

func (c *closeTestNative) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
func (c *closeTestNative) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
}
func (c *closeTestNative) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}
func (c *closeTestNative) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (c *closeTestNative) Ping(ctx context.Context) error { return c.Conn.(driver.Pinger).Ping(ctx) }

// capturedClose converts a panic only at the test boundary, retaining its exact
// value. The product is required to propagate the native panic itself.
func capturedClose(c driver.Conn) (err error, value any) {
	defer func() { value = recover() }()
	err = c.Close()
	return
}

func TestCloseObservationRepeatMatrix(t *testing.T) {
	for _, mode := range []string{"success", "before", "after", "panic"} {
		for _, reporting := range []string{"success", "error", "panic"} {
			if mode == "success" && reporting != "success" {
				continue
			}
			t.Run(mode+"/"+reporting, func(t *testing.T) {
				native := newCloseTestNative(t, mode)
				var reports atomic.Int32
				var event closeEvent
				c := newObservedConn(native, func(e closeEvent) error {
					reports.Add(1)
					event = e
					if reporting == "error" {
						return errObservedReporter
					}
					if reporting == "panic" {
						panic("receiver payload must not escape")
					}
					return nil
				})
				before := c.observation.Snapshot()
				if before.ID == 0 || before.Attempted || before.Completed || before.Err != nil {
					t.Fatalf("invalid initial observation: %+v", before)
				}
				first, firstPanic := capturedClose(c)
				if mode == "panic" {
					if firstPanic != native.marker {
						t.Fatal("native panic replaced", firstPanic)
					}
				} else if firstPanic != nil {
					t.Fatal("unexpected panic", firstPanic)
				}
				snapshot := c.observation.Snapshot()
				if !snapshot.Attempted || snapshot.Completed != (mode != "panic") || snapshot.ID != before.ID {
					t.Fatalf("invalid completed observation: %+v", snapshot)
				}
				if mode == "success" {
					if first != nil || snapshot.Err != nil || snapshot.ReportState != "not_needed" || reports.Load() != 0 {
						t.Fatal("success reported failure", first, snapshot)
					}
				} else {
					if reports.Load() != 1 || event.ID != snapshot.ID || snapshot.ReportState != "finished" {
						t.Fatal("missing/duplicate reporting", snapshot, reports.Load())
					}
					if mode != "panic" && (!errors.Is(first, errObservedNativeClose) || !errors.Is(snapshot.Err, errObservedNativeClose) || !errors.Is(event.Err, errObservedNativeClose)) {
						t.Fatal("native cause lost", first, snapshot)
					}
					if mode == "panic" && (event.Category != "close_interrupted" || event.Err != nil) {
						t.Fatal("panic payload retained", event)
					}
					if reporting == "success" && snapshot.ReportErr != nil {
						t.Fatal(snapshot.ReportErr)
					}
					if reporting == "error" && !errors.Is(snapshot.ReportErr, errObservedReporter) {
						t.Fatal("receiver error lost", snapshot.ReportErr)
					}
					if reporting == "panic" && !errors.Is(snapshot.ReportErr, ErrCloseReporterInterrupted) {
						t.Fatal("receiver panic not classified", snapshot.ReportErr)
					}
				}
				checkRepeat := func() {
					err, p := capturedClose(c)
					if p != nil {
						t.Error("repeated panic", p)
					}
					switch mode {
					case "success":
						if err != nil {
							t.Error(err)
						}
					case "panic":
						if !errors.Is(err, ErrCloseUnconfirmed) {
							t.Error("false confirmed repeat", err)
						}
					default:
						if !errors.Is(err, errObservedNativeClose) {
							t.Error("repeated cause lost", err)
						}
					}
				}
				checkRepeat()
				var wg sync.WaitGroup
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() { defer wg.Done(); checkRepeat() }()
				}
				wg.Wait()
				if native.calls.Load() != 1 || reports.Load() > 1 {
					t.Fatal("native/report repeated")
				}
			})
		}
	}
}

func TestCloseObservationInProgress(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-native-effect", true: "after-native-effect"}[after], func(t *testing.T) {
			testCloseObservationInProgress(t, after)
		})
	}
}

func testCloseObservationInProgress(t *testing.T, after bool) {
	native := newCloseTestNative(t, "after")
	native.blockAfter = after
	native.entered, native.release = make(chan struct{}), make(chan struct{})
	reportEntered, reportRelease := make(chan struct{}), make(chan struct{})
	var releaseNative, releaseReport sync.Once
	defer func() {
		releaseNative.Do(func() { close(native.release) })
		releaseReport.Do(func() { close(reportRelease) })
	}()
	c := newObservedConn(native, func(closeEvent) error { close(reportEntered); <-reportRelease; return errObservedReporter })
	done := make(chan error, 1)
	exited := make(chan struct{})
	defer func() {
		releaseNative.Do(func() { close(native.release) })
		releaseReport.Do(func() { close(reportRelease) })
		<-exited
	}()
	go func() { defer close(exited); done <- c.Close() }()
	<-native.entered
	if s := c.observation.Snapshot(); !s.Attempted || s.Completed {
		t.Fatal("blocked native close falsely complete", s)
	}
	releaseNative.Do(func() { close(native.release) })
	<-reportEntered
	if s := c.observation.Snapshot(); !s.Completed || !errors.Is(s.Err, errObservedNativeClose) || s.ReportState != "in_progress" {
		t.Fatal("native result not published before receiver", s)
	}
	releaseReport.Do(func() { close(reportRelease) })
	if err := <-done; !errors.Is(err, errObservedNativeClose) {
		t.Fatal(err)
	}
	if s := c.observation.Snapshot(); s.ReportState != "finished" || !errors.Is(s.ReportErr, errObservedReporter) {
		t.Fatal(s)
	}
}

// OC09's automatic discard is distinct from the planned Raw discard above.
// Only the selected business call returns ErrBadConn, without executing SQL.
type closeBadConnDriver struct {
	source driver.Driver
	calls  *atomic.Int32
}
type closeBadConnNative struct {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.QueryerContext
	driver.Pinger
	exec  driver.ExecerContext
	calls *atomic.Int32
}

func (d closeBadConnDriver) Open(path string) (driver.Conn, error) {
	c, err := d.source.Open(path)
	if err != nil {
		return nil, err
	}
	return &closeBadConnNative{c, c.(driver.ConnBeginTx), c.(driver.ConnPrepareContext), c.(driver.QueryerContext), c.(driver.Pinger), c.(driver.ExecerContext), d.calls}, nil
}
func (c *closeBadConnNative) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if query == "SELECT 'OC09 pre-effect refusal'" {
		c.calls.Add(1)
		return nil, driver.ErrBadConn
	}
	return c.exec.ExecContext(ctx, query, args)
}

func TestCloseObservationAutomaticOwnershipLoss(t *testing.T) {
	source := &connectObservationDriver{closeFault: true}
	t.Cleanup(source.cleanup)
	var calls atomic.Int32
	var events []closeEvent
	db := sql.OpenDB(newConnectorWithReporter(filepath.Join(t.TempDir(), "bad.sqlite"), closeBadConnDriver{source, &calls}, func(event closeEvent) error { events = append(events, event); return reportClose(event) }))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	c, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	old, err := ObserveClose(c)
	if err != nil {
		t.Fatal(err)
	}
	id := old.Snapshot().ID
	_, err = c.ExecContext(t.Context(), "SELECT 'OC09 pre-effect refusal'")
	if !errors.Is(err, driver.ErrBadConn) || calls.Load() != 1 {
		t.Fatal("primary error lost or replayed", err, calls.Load())
	}
	if len(events) != 1 || events[0].ID != id || !errors.Is(events[0].Err, errConnectClose) {
		t.Fatal("exact native close cause lost", events)
	}
	if lost, err := ObserveClose(c); lost != nil || !errors.Is(err, sql.ErrConnDone) || !errors.Is(err, ErrCloseObservationLost) {
		t.Fatal("lost target reacquired", lost, err)
	}
	next, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	observer, err := ObserveClose(next)
	if err != nil {
		t.Fatal(err)
	}
	if observer.Snapshot().ID == id || old.Snapshot().ID != id || source.opened != 2 {
		t.Fatal("replacement identity lost")
	}
	if err := next.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("business SQL replayed on replacement")
	}
	source.closeFault = false
}

// OC08/OC09: use actual sql.Conn ownership, never retain the Raw driver object.
func TestObserveCloseOwnershipAndCapabilities(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "managed.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	c, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	observer, err := ObserveClose(c)
	if err != nil || observer == nil {
		t.Fatal("managed observer unavailable", err)
	}
	if _, ok := observer.(driver.Conn); ok {
		t.Fatal("observer exposes driver")
	}
	if _, ok := observer.(io.Closer); ok {
		t.Fatal("observer exposes close")
	}
	observerType := reflect.TypeOf(observer)
	if observerType.NumMethod() != 1 || observerType.Method(0).Name != "Snapshot" {
		t.Fatal("observer exposes control methods", observerType)
	}
	snapshotType := reflect.TypeOf(observer.Snapshot())
	for i := 0; i < snapshotType.NumField(); i++ {
		field := snapshotType.Field(i)
		if field.IsExported() && (field.Type.Kind() == reflect.Func || field.Type == reflect.TypeFor[*sql.Conn]() || field.Type == reflect.TypeFor[*sql.DB]() || field.Type.Implements(reflect.TypeFor[driver.Conn]()) || field.Type.Implements(reflect.TypeFor[io.Closer]())) {
			t.Fatal("snapshot exposes resource capability", field.Name)
		}
	}
	oldID := observer.Snapshot().ID
	if err := c.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(err, driver.ErrBadConn) {
		t.Fatal(err)
	}
	if lost, err := ObserveClose(c); lost != nil || !errors.Is(err, ErrCloseObservationLost) || !errors.Is(err, sql.ErrConnDone) {
		t.Fatal("lost observation fabricated", lost, err)
	}
	next, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	newObserver, err := ObserveClose(next)
	if err != nil || newObserver == nil {
		t.Fatal(err)
	}
	if newObserver.Snapshot().ID == oldID || observer.Snapshot().ID != oldID || !observer.Snapshot().Completed {
		t.Fatal("replacement changed observer identity")
	}
	var one int
	if err := next.QueryRowContext(t.Context(), "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatal("replacement unusable", err)
	}
	if o, err := ObserveClose(nil); o != nil || err == nil {
		t.Fatal("nil accepted")
	}
	foreign, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "foreign.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	fc, err := foreign.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer fc.Close()
	if o, err := ObserveClose(fc); o != nil || !errors.Is(err, ErrCloseObservationUnsupported) {
		t.Fatal("foreign observation fabricated", o, err)
	}
	if err := fc.PingContext(t.Context()); err != nil {
		t.Fatal("ObserveClose closed foreign connection", err)
	}
}

// OC01 tests each missing interface independently; all other capabilities and
// native opening/Close still reach the real driver. No existing fixture changes.
type closeCapabilityDriver struct {
	source driver.Driver
	omit   string
}

func (d closeCapabilityDriver) Open(path string) (driver.Conn, error) {
	c, err := d.source.Open(path)
	if err != nil {
		return nil, err
	}
	b := c.(driver.ConnBeginTx)
	p := c.(driver.ConnPrepareContext)
	e := c.(driver.ExecerContext)
	q := c.(driver.QueryerContext)
	g := c.(driver.Pinger)
	switch d.omit {
	case "begin":
		return struct {
			driver.Conn
			driver.ConnPrepareContext
			driver.ExecerContext
			driver.QueryerContext
			driver.Pinger
		}{c, p, e, q, g}, nil
	case "prepare":
		return struct {
			driver.Conn
			driver.ConnBeginTx
			driver.ExecerContext
			driver.QueryerContext
			driver.Pinger
		}{c, b, e, q, g}, nil
	case "exec":
		return struct {
			driver.Conn
			driver.ConnBeginTx
			driver.ConnPrepareContext
			driver.QueryerContext
			driver.Pinger
		}{c, b, p, q, g}, nil
	case "query":
		return struct {
			driver.Conn
			driver.ConnBeginTx
			driver.ConnPrepareContext
			driver.ExecerContext
			driver.Pinger
		}{c, b, p, e, g}, nil
	case "ping":
		return struct {
			driver.Conn
			driver.ConnBeginTx
			driver.ConnPrepareContext
			driver.ExecerContext
			driver.QueryerContext
		}{c, b, p, e, q}, nil
	default:
		return c, nil
	}
}

func TestCloseObservationCapabilityRefusal(t *testing.T) {
	for _, omit := range []string{"begin", "prepare", "exec", "query", "ping"} {
		t.Run(omit, func(t *testing.T) {
			source := &connectObservationDriver{}
			t.Cleanup(source.cleanup)
			c, err := newConnector(filepath.Join(t.TempDir(), "caps.sqlite"), closeCapabilityDriver{source, omit}).Connect(t.Context())
			if c != nil || err == nil {
				t.Fatal("incompatible source handed off", c, err)
			}
			if source.opened != 1 || len(source.connections) != 1 || source.connections[0].closes != 1 || !source.connections[0].nativeClosed {
				t.Fatal("refusal did not close exact connection once")
			}
		})
	}
}

func TestCloseObservationLegacyInterfaceTransparency(t *testing.T) {
	source := &connectObservationDriver{}
	t.Cleanup(source.cleanup)
	c, err := newConnector(filepath.Join(t.TempDir(), "legacy.sqlite"), source).Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, ok := c.(driver.SessionResetter); ok {
		t.Fatal("invented session reset")
	}
	if _, ok := c.(driver.Validator); ok {
		t.Fatal("invented validation")
	}
	tx, err := c.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := c.Prepare("CREATE TABLE control(value INTEGER)")
	if err != nil {
		t.Fatal(err)
	}
	_, execErr := stmt.Exec(nil)
	closeErr := stmt.Close()
	if execErr != nil || closeErr != nil {
		t.Fatal(execErr, closeErr)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	stmt, err = c.Prepare("INSERT INTO control VALUES (?)")
	if err != nil {
		t.Fatal(err)
	}
	result, execErr := stmt.Exec([]driver.Value{int64(17)})
	closeErr = stmt.Close()
	if execErr != nil || closeErr != nil {
		t.Fatal(execErr, closeErr)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		t.Fatal("result changed", count, err)
	}
	rows, err := c.(driver.QueryerContext).QueryContext(t.Context(), "SELECT value FROM control", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	values := make([]driver.Value, 1)
	if err := rows.Next(values); err != nil || values[0] != int64(17) {
		t.Fatal("statement arguments/effect changed", values, err)
	}
	if err := rows.Next(values); err != io.EOF {
		t.Fatal("extra effect", err)
	}
}

func TestCloseObservationConcurrentFirstCall(t *testing.T) {
	for _, mode := range []string{"success", "before", "after", "panic"} {
		for _, reporting := range []string{"success", "error", "panic"} {
			if mode == "success" && reporting != "success" {
				continue
			}
			t.Run(mode+"/"+reporting, func(t *testing.T) {
				native := newCloseTestNative(t, mode)
				entered, release := make(chan struct{}), make(chan struct{})
				native.entered, native.release = entered, release
				var unblock sync.Once
				var reports atomic.Int32
				c := newObservedConn(native, func(closeEvent) error {
					reports.Add(1)
					if reporting == "error" {
						return errObservedReporter
					}
					if reporting == "panic" {
						panic("receiver")
					}
					return nil
				})
				type result struct {
					err   error
					value any
				}
				results := make(chan result, 8)
				var wg sync.WaitGroup
				defer func() { unblock.Do(func() { close(release) }); wg.Wait() }()
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() { defer wg.Done(); err, p := capturedClose(c); results <- result{err, p} }()
				}
				<-entered
				if snapshot := c.observation.Snapshot(); !snapshot.Attempted || snapshot.Completed {
					t.Fatal("premature completion", snapshot)
				}
				unblock.Do(func() { close(release) })
				wg.Wait()
				panics := 0
				for i := 0; i < 8; i++ {
					r := <-results
					if r.value != nil {
						panics++
						if mode != "panic" || r.value != native.marker {
							t.Fatal("wrong panic", r.value)
						}
						continue
					}
					switch mode {
					case "success":
						if r.err != nil {
							t.Fatal(r.err)
						}
					case "panic":
						if !errors.Is(r.err, ErrCloseUnconfirmed) {
							t.Fatal("unconfirmed close lost", r.err)
						}
					default:
						if !errors.Is(r.err, errObservedNativeClose) {
							t.Fatal("cause lost", r.err)
						}
					}
				}
				if (mode == "panic" && panics != 1) || (mode != "panic" && panics != 0) || native.calls.Load() != 1 {
					t.Fatal("duplicate/missing native call or panic", panics, native.calls.Load())
				}
				wantReports := int32(1)
				if mode == "success" {
					wantReports = 0
				}
				if reports.Load() != wantReports {
					t.Fatal("wrong report count", reports.Load())
				}
			})
		}
	}
}

func TestCloseObservationLateReceiver(t *testing.T) {
	for _, retain := range []bool{false, true} {
		t.Run(map[bool]string{false: "unused-observer", true: "retained-observer"}[retain], func(t *testing.T) {
			source := &connectObservationDriver{closeFault: true}
			t.Cleanup(source.cleanup)
			var event closeEvent
			var reports int
			db := sql.OpenDB(newConnectorWithReporter(filepath.Join(t.TempDir(), "late.sqlite"), source, func(e closeEvent) error { event = e; reports++; return nil }))
			t.Cleanup(func() { _ = db.Close() })
			db.SetMaxOpenConns(1)
			var retained CloseObserver
			id := func() uint64 {
				c, err := db.Conn(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				o, err := ObserveClose(c)
				if err != nil {
					t.Fatal(err)
				}
				if retain {
					retained = o
				}
				return o.Snapshot().ID
			}()
			if reports != 0 || source.connections[0].closes != 0 {
				t.Fatal("borrower Close mistaken for physical Close")
			}
			_ = db.Close() // native Close may fail; receiver must work without helper.
			if reports != 1 || event.ID != id || !errors.Is(event.Err, errConnectClose) || source.connections[0].closes != 1 {
				t.Fatal("late failure not delivered exactly once", event, reports)
			}
			if retain {
				s := retained.Snapshot()
				if !s.Completed || !errors.Is(s.Err, errConnectClose) {
					t.Fatal(s)
				}
			}
		})
	}
}

// OC11 observes the production receiver in a child, never mutating the parent
// logger. Deliberately sensitive driver error/panic payloads cannot reach logs.
type privateCloseError struct{ payload string }

func (e privateCloseError) Error() string { return e.payload }
func (e privateCloseError) Code() int     { return 5 }

type singleCloseNativeDriver struct{ native driver.Conn }

func (d singleCloseNativeDriver) Open(string) (driver.Conn, error) { return d.native, nil }

func TestCloseProductionDiagnosticPrivacy(t *testing.T) {
	const childKey = "SQLRS_OC_LOG_CHILD"
	const secret = "private-DSN-SQL-stdout-panic-marker"
	if mode := os.Getenv(childKey); mode != "" {
		native := newCloseTestNative(t, mode)
		native.marker = secret
		native.failure = privateCloseError{secret}
		c, err := newConnector(secret, singleCloseNativeDriver{native}).Connect(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		_, _ = capturedClose(c)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "after", "panic"} {
		command := exec.CommandContext(t.Context(), executable, "-test.run=^TestCloseProductionDiagnosticPrivacy$", "-test.count=1")
		command.Env = append(os.Environ(), childKey+"="+mode)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatal("child failed", err, string(output))
		}
		text := string(output)
		if strings.Contains(text, secret) || strings.Contains(text, errObservedNativeClose.Error()) {
			t.Fatal("private diagnostic leaked", text)
		}
		want := 0
		if mode != "success" {
			want = 1
		}
		if strings.Count(text, "sqlite_connection_close ") != want {
			t.Fatal("wrong production report count", mode, text)
		}
		code := "code=0"
		if mode == "after" {
			code = "code=5"
		}
		if want == 1 && (!strings.Contains(text, "observation=") || !strings.Contains(text, code)) {
			t.Fatal("safe metadata missing", text)
		}
	}
}
