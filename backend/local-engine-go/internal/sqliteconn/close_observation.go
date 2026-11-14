package sqliteconn

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"log"
	"sync"
	"sync/atomic"
)

// These classifications describe observation availability, not SQL outcome.
// Requirements: liquibase-connection-close-observation-design.md §3, §5.2.
var (
	ErrCloseObservationUnsupported = errors.New("SQLite close observation unavailable")
	ErrCloseObservationLost        = errors.New("SQLite close observation ownership lost")
	ErrCloseUnconfirmed            = errors.New("SQLite physical close not confirmed")
	ErrCloseReporterInterrupted    = errors.New("SQLite close diagnostic receiver interrupted")
)

// CloseSnapshot separates the physical call's result from its diagnostic delivery.
// Completed means a return, not universal proof of absence of native resources.
// No field grants access to a connection, pool or resource-management callback.
type CloseSnapshot struct {
	ID                   uint64
	Attempted, Completed bool
	Err                  error
	ReportState          string
	ReportErr            error
}

// CloseObserver only reads the state of one physical target, even after retirement.
// It cannot close that target or acquire a replacement (§2 and §5.2).
type CloseObserver interface{ Snapshot() CloseSnapshot }

type closeObservation struct {
	mu    sync.Mutex
	state CloseSnapshot
}

func (o *closeObservation) Snapshot() CloseSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.state
}

// closeEvent contains only process-local diagnostics; interruption never retains
// the panic value. The receiver has no job, connection or cleanup authority.
type closeEvent struct {
	ID       uint64
	Category string
	Err      error
}
type closeReporter func(closeEvent) error

var nextCloseObservationID atomic.Uint64

// observedConn alone owns the physical connection; the separately retained
// observation has no back-reference. Once also serializes concurrent first calls.
type observedConn struct {
	native      driver.Conn
	once        sync.Once
	reporter    closeReporter
	observation *closeObservation
}

func newObservedConn(native driver.Conn, reporter closeReporter) *observedConn {
	return &observedConn{native: native, reporter: reporter, observation: &closeObservation{
		state: CloseSnapshot{ID: nextCloseObservationID.Add(1), ReportState: "not_started"},
	}}
}

// Close attempts native Close once. A panic reaches its initiating caller, while
// later calls return an honest unconfirmed result, never sync.Once's false nil.
func (c *observedConn) Close() error {
	c.once.Do(func() {
		c.observation.mu.Lock()
		c.observation.state.Attempted = true
		c.observation.mu.Unlock()
		returned := false
		defer func() {
			if !returned {
				primary := recover()
				c.report(closeEvent{ID: c.observation.Snapshot().ID, Category: "close_interrupted"})
				panic(primary)
			}
		}()
		err := c.native.Close()
		returned = true
		c.observation.mu.Lock()
		c.observation.state.Completed, c.observation.state.Err = true, err
		if err == nil {
			c.observation.state.ReportState = "not_needed"
		}
		c.observation.mu.Unlock()
		if err != nil {
			c.report(closeEvent{ID: c.observation.Snapshot().ID, Category: "close_failed", Err: err})
		}
	})
	snapshot := c.observation.Snapshot()
	if !snapshot.Completed {
		return ErrCloseUnconfirmed
	}
	return snapshot.Err
}

// report publishes native results before calling outside code, without holding
// the snapshot mutex. A receiver failure is recorded, not recursively reported.
func (c *observedConn) report(event closeEvent) {
	c.observation.mu.Lock()
	c.observation.state.ReportState = "in_progress"
	c.observation.mu.Unlock()
	err := func() (err error) {
		returned := false
		defer func() {
			if !returned {
				_ = recover()
				err = ErrCloseReporterInterrupted
			}
		}()
		err = c.reporter(event)
		returned = true
		return
	}()
	c.observation.mu.Lock()
	c.observation.state.ReportState, c.observation.state.ReportErr = "finished", err
	c.observation.mu.Unlock()
}

// reportClose emits fixed metadata only; arbitrary errors and panic payloads
// must not leak DSNs, SQL or private capture into the process log (§4.2).
func reportClose(event closeEvent) error {
	code := 0
	var coded interface{ Code() int }
	if errors.As(event.Err, &coded) {
		code = coded.Code()
	}
	log.Printf("sqlite_connection_close category=%s observation=%d code=%d", event.Category, event.ID, code)
	return nil
}

// ObserveClose uses Raw only to extract independent state. Its callback neither
// retains the driver object nor performs I/O; it never reacquires after failure.
func ObserveClose(conn *sql.Conn) (CloseObserver, error) {
	if conn == nil {
		return nil, errors.New("nil SQLite connection for close observation")
	}
	var observer CloseObserver
	err := conn.Raw(func(value any) error {
		if wrapped, ok := value.(*observedConn); ok {
			observer = wrapped.observation
		}
		return nil
	})
	if err != nil {
		return nil, errors.Join(ErrCloseObservationLost, err)
	}
	if observer == nil {
		return nil, ErrCloseObservationUnsupported
	}
	return observer, nil
}

// The pinned driver's supported interfaces are forwarded unchanged, without
// inventing ResetSession/Validator or intercepting transactions/statements (§2).
func (c *observedConn) Prepare(query string) (driver.Stmt, error) { return c.native.Prepare(query) }
func (c *observedConn) Begin() (driver.Tx, error)                 { return c.native.Begin() }
func (c *observedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.native.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
func (c *observedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.native.(driver.ConnPrepareContext).PrepareContext(ctx, query)
}
func (c *observedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.native.(driver.ExecerContext).ExecContext(ctx, query, args)
}
func (c *observedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.native.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (c *observedConn) Ping(ctx context.Context) error { return c.native.(driver.Pinger).Ping(ctx) }
