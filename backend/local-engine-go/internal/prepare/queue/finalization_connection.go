package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/sqlrs/engine-local/internal/sqliteconn"
	"log"
	"time"
)

// finalizationConnection owns one borrower through work, settlement and full
// reconciliation. It never cleans through the pool or retries business SQL.
// Requirements: liquibase-finalization-connection-ownership.md §1–4.
type finalizationConnection struct {
	conn               *sql.Conn
	tx                 *sql.Tx
	observer           sqliteconn.CloseObserver
	caller             context.Context
	settlement         context.Context
	cancel             context.CancelFunc
	finished, released bool
}

// withFinalization explicitly finishes every normal return before reporting
// success. The fallback handles exceptional exits without replacing a panic.
func withFinalization[T any](ctx context.Context, s *SQLiteStore, work func(*finalizationConnection) (T, error)) (value T, err error) {
	f, err := acquireFinalization(ctx, s)
	if err != nil {
		return value, err
	}
	defer f.fallback()
	value, err = work(f)
	if err != nil {
		err = errors.Join(err, ctx.Err())
	}
	err = f.finish(err)
	if err != nil {
		var zero T
		return zero, err
	}
	return value, err
}

func acquireFinalization(ctx context.Context, s *SQLiteStore) (f *finalizationConnection, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	f = &finalizationConnection{conn: conn, caller: ctx}
	// Acquisition itself can panic in a driver; the same exceptional ownership
	// rule applies before the caller receives the helper.
	ready := false
	owner := f
	defer func() {
		if !ready {
			owner.cleanupPanic(recover())
		}
	}()
	f.observer, err = sqliteconn.ObserveClose(conn)
	if err != nil && (s.requireCloseObservation || !errors.Is(err, sqliteconn.ErrCloseObservationUnsupported)) {
		return nil, errors.Join(err, f.retire())
	}
	for _, setting := range []struct {
		name string
		want int
	}{{"foreign_keys", 1}, {"busy_timeout", 0}, {"read_uncommitted", 0}} {
		var value int
		if err = conn.QueryRowContext(ctx, "PRAGMA "+setting.name).Scan(&value); err != nil || value != setting.want {
			if err == nil {
				err = errors.New("invalid finalization SQLite connection settings")
			}
			return nil, errors.Join(err, f.retire())
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, errors.Join(err, f.retire())
	}
	f.tx, err = conn.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return nil, errors.Join(err, f.retire())
	}
	ready = true
	return f, nil
}

// budget is created once; rollback/probe/readback share this independent limit.
func (f *finalizationConnection) budget() context.Context {
	if f.settlement == nil {
		f.settlement, f.cancel = context.WithTimeout(context.WithoutCancel(f.caller), 5*time.Second)
	}
	return f.settlement
}

// settleNative establishes only cleanliness, never whether COMMIT took effect.
// A failed rollback is followed by a real empty BEGIN/ROLLBACK, not text parsing.
func (f *finalizationConnection) settleNative() error {
	ctx := f.budget()
	_, rollbackErr := f.conn.ExecContext(ctx, "ROLLBACK")
	if rollbackErr == nil {
		return nil
	}
	return f.probeClean(rollbackErr)
}

// probeClean is used after ANY failed rollback, including sql.Tx.Rollback.
// Do not silently retry that rollback and then claim the empty-probe boundary.
func (f *finalizationConnection) probeClean(rollbackErr error) error {
	ctx := f.budget()
	if errors.Is(rollbackErr, driver.ErrBadConn) || errors.Is(rollbackErr, sql.ErrConnDone) {
		return rollbackErr
	}
	if _, err := f.conn.ExecContext(ctx, "BEGIN"); err != nil {
		return errors.Join(rollbackErr, err)
	}
	if _, err := f.conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		return errors.Join(rollbackErr, err)
	}
	return nil
}

// commit never retries COMMIT. Failed replies are reconciled through a fresh
// read transaction on the STILL OWNED physical connection after settlement.
func (f *finalizationConnection) commit(check func(context.Context, *sql.Tx) (bool, error)) error {
	if err := f.caller.Err(); err != nil {
		return err
	}
	err := f.tx.Commit()
	f.tx = nil
	if err == nil {
		return nil
	}
	commitErr := errors.Join(err, f.caller.Err())
	if err := f.settleNative(); err != nil {
		return errors.Join(ErrFinalizationIndeterminate, commitErr, err, f.retire())
	}
	ctx := f.budget()
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrFinalizationIndeterminate, commitErr, err, f.retire())
	}
	readTx, err := f.conn.BeginTx(context.WithoutCancel(ctx), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return errors.Join(ErrFinalizationIndeterminate, commitErr, err, f.retire())
	}
	f.tx = readTx
	matched, readErr := check(ctx, readTx)
	rollbackErr := readTx.Rollback()
	f.tx = nil
	if rollbackErr != nil {
		if cleanErr := f.probeClean(rollbackErr); cleanErr != nil {
			if readErr == nil && matched {
				_ = f.retire()
				return nil
			}
			return errors.Join(ErrFinalizationIndeterminate, commitErr, readErr, rollbackErr, cleanErr, f.retire())
		}
	}
	if readErr != nil {
		return errors.Join(ErrFinalizationIndeterminate, commitErr, readErr, f.retire())
	}
	if matched {
		return nil
	}
	return commitErr
}

// retire targets only this borrower. Raw's ErrBadConn is a discard signal, not
// a retry wrapper around an already-effectful operation. Missing observation in
// caller_owned mode remains explicitly unavailable, not successful native Close.
func (f *finalizationConnection) retire() error {
	if f.released {
		return nil
	}
	f.released = true
	err := f.conn.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) {
		err = nil
	}
	if f.observer == nil {
		return errors.Join(err, sqliteconn.ErrCloseObservationUnsupported)
	}
	snapshot := f.observer.Snapshot()
	if !snapshot.Completed {
		return errors.Join(err, sqliteconn.ErrCloseUnconfirmed)
	}
	return errors.Join(err, snapshot.Err)
}

// finish discharges a live transaction before returning the borrower. Cleanup
// diagnostics cannot erase an established committed effect (§4).
func (f *finalizationConnection) finish(cause error) error {
	defer func() {
		if f.cancel != nil {
			f.cancel()
		}
	}()
	if f.tx != nil {
		err := f.tx.Rollback()
		f.tx = nil
		if err != nil {
			if cause != nil {
				cause = errors.Join(cause, err)
			} else {
				log.Print("sqlite_finalization_rollback diagnostic=failed")
			}
			if cleanErr := f.probeClean(err); cleanErr != nil {
				cause = errors.Join(ErrFinalizationIndeterminate, cause, err, cleanErr, f.retire())
			}
		}
	}
	f.finished = true
	if !f.released {
		f.released = true
		err := f.conn.Close()
		if f.observer != nil {
			snapshot := f.observer.Snapshot()
			err = errors.Join(err, snapshot.Err)
		}
		if err != nil && !errors.Is(err, sql.ErrConnDone) {
			if cause != nil {
				cause = errors.Join(cause, err)
			} else {
				log.Print("sqlite_finalization_release diagnostic=failed")
			}
		}
	}
	return cause
}

// fallback preserves an original panic even if rollback/retirement also panics.
// It performs only lifecycle cleanup, never business writes or a new outcome.
func (f *finalizationConnection) fallback() {
	f.cleanupPanic(recover())
}

func (f *finalizationConnection) cleanupPanic(primary any) {
	var secondary any
	if !f.finished {
		func() {
			defer func() { secondary = recover() }()
			if f.tx != nil {
				_ = f.tx.Rollback()
				f.tx = nil
			}
			_ = f.retire()
		}()
		if f.cancel != nil {
			f.cancel()
		}
	}
	if primary != nil {
		if secondary != nil {
			log.Print("sqlite_finalization_cleanup diagnostic=secondary_panic")
		}
		panic(primary)
	}
	if secondary != nil {
		panic(secondary)
	}
}
