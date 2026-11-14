package managedstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/store/sqlite"
)

// initializeMainPath owns one exact connection through COMMIT reconciliation.
// A failed COMMIT cannot return that physical connection with a live transaction.
// No Docker or filesystem operation is performed under this SQL ownership.
func initializeMainPath(ctx context.Context, db *sql.DB, inventory func(context.Context) error) (format sqlite.ManagedStoreFormat, returnedErr error) {
	if db == nil {
		return format, sqlite.ErrManagedPreflightUnavailable
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		return format, failure(ctx)
	}
	var tx *sql.Tx
	suspended, retired := false, false
	quarantine := func() {
		if retired {
			return
		}
		retired = true
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	restore := func(cleanCtx context.Context) error {
		if !suspended {
			return nil
		}
		if _, err := conn.ExecContext(cleanCtx, "PRAGMA foreign_keys=ON"); err != nil {
			return err
		}
		var enabled int
		if err := conn.QueryRowContext(cleanCtx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			return err
		}
		if enabled != 1 {
			return sqlite.ErrManagedPreflightUnavailable
		}
		suspended = false
		return nil
	}
	defer func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if tx != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				returnedErr = errors.Join(returnedErr, rollbackErr)
			}
			// Context cancellation/failed COMMIT may have consumed sql.Tx without
			// clearing its physical SQLite transaction. Prove it before pool reuse.
			if err := clearMainPathTransaction(cleanCtx, conn); err != nil {
				returnedErr = errors.Join(returnedErr, err)
				quarantine()
			}
		}
		if !retired {
			if err := restore(cleanCtx); err != nil {
				returnedErr = errors.Join(returnedErr, err)
				quarantine()
			}
		}
		if err := conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			returnedErr = errors.Join(returnedErr, err)
		}
		if returnedErr != nil {
			format = sqlite.ManagedStoreFormat{}
		}
	}()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=0"); err != nil {
		quarantine()
		return format, failure(ctx)
	}
	tx, err = conn.BeginTx(ctx, nil)
	if err != nil {
		return format, failure(ctx)
	}
	mode, err := queue.ClassifyLiquibaseMainPathSchema(ctx, tx)
	if err != nil {
		return format, startupMainPathError(ctx, err)
	}
	format, err = sqlite.EnsureManagedStoreFormat(ctx, tx, inventory)
	if err != nil {
		return format, err
	}
	if mode == "released" {
		if err := tx.Rollback(); err != nil {
			return format, startupMainPathError(ctx, err)
		}
		tx = nil
		// Retain restoration ownership before the command: a lost reply can
		// mean that foreign-key suspension already took effect.
		suspended = true
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return format, startupMainPathError(ctx, err)
		}
		tx, err = conn.BeginTx(ctx, nil)
		if err != nil {
			return format, startupMainPathError(ctx, err)
		}
		// Repeat the exact baseline/unfinished-work proof after beginning the
		// conversion transaction. A read failure never means absence.
		again, err := queue.ClassifyLiquibaseMainPathSchema(ctx, tx)
		if err != nil || again != "released" {
			return format, startupMainPathError(ctx, err)
		}
	}
	if mode == "fresh" {
		if err := install(ctx, tx); err != nil {
			return format, startupMainPathError(ctx, err)
		}
	} else if mode == "released" {
		if err := queue.InstallLiquibaseMainPathSchema(ctx, tx); err != nil {
			return format, startupMainPathError(ctx, err)
		}
	} else {
		// The namespace is already complete; its existing owner verifies marker
		// values without filling missing objects.
		if err := sqlite.InstallRuntimeV2Schema(ctx, tx); err != nil {
			return format, startupMainPathError(ctx, err)
		}
	}
	if err := verifyMainPathTransaction(ctx, tx); err != nil {
		return format, startupMainPathError(ctx, err)
	}
	if mode == "current" {
		if err := ctx.Err(); err != nil {
			return format, err
		}
		if err := tx.Rollback(); err != nil {
			return format, startupMainPathError(ctx, err)
		}
		tx = nil
		return format, nil
	}
	commitErr := tx.Commit()
	tx = nil
	cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if commitErr != nil {
		if err := clearMainPathTransaction(cleanCtx, conn); err != nil {
			quarantine()
			return format, startupMainPathError(ctx, errors.Join(commitErr, err))
		}
	}
	if err := restore(cleanCtx); err != nil {
		quarantine()
		return format, startupMainPathError(ctx, errors.Join(commitErr, err))
	}
	// Reconciliation reads only after physical transaction cleanup. A complete
	// committed target and identical domain are mandatory even after a lost reply.
	check, err := conn.BeginTx(cleanCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return format, startupMainPathError(ctx, errors.Join(commitErr, err))
	}
	// Keep this transaction under the same deferred cleanup ownership too.
	// A readback rollback error must not return an unproved connection.
	tx = check
	if err := verifyMainPathTransaction(cleanCtx, check); err != nil {
		return format, startupMainPathError(ctx, errors.Join(commitErr, err))
	}
	saved, err := sqlite.EnsureManagedStoreFormat(cleanCtx, check, nil)
	if err != nil || saved != format {
		return format, startupMainPathError(ctx, errors.Join(commitErr, err))
	}
	if err := check.Rollback(); err != nil {
		return format, startupMainPathError(ctx, errors.Join(commitErr, err))
	}
	tx = nil
	return format, nil
}

func startupMainPathError(ctx context.Context, cause error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.Join(sqlite.ErrManagedPreflightUnavailable, cause)
}

// clearMainPathTransaction uses real BEGIN/ROLLBACK to prove an idle physical
// connection after sql.Tx is done. It does not classify driver message text.
func clearMainPathTransaction(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, "ROLLBACK")
	if err == nil {
		return nil
	}
	if _, beginErr := conn.ExecContext(ctx, "BEGIN"); beginErr != nil {
		return errors.Join(err, beginErr)
	}
	if _, rollbackErr := conn.ExecContext(ctx, "ROLLBACK"); rollbackErr != nil {
		return errors.Join(err, rollbackErr)
	}
	return nil
}

func verifyMainPathTransaction(ctx context.Context, tx *sql.Tx) error {
	expected, err := expectedSchema(ctx)
	if err != nil {
		return err
	}
	actual, err := schema(ctx, tx)
	if err != nil {
		return err
	}
	for name, definition := range expected {
		if actual[name] != definition {
			return sqlite.ErrManagedPreflightUnavailable
		}
	}
	return queue.VerifyLiquibaseMainPathSchema(ctx, tx)
}
