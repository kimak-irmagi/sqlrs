// Package managedstore coordinates the empty-store format cutover before
// migrations, recovery or admission. See managed-database-identity-internals.md.
package managedstore

import (
	"context"
	"database/sql"
	"strings"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/store/sqlite"
)

// Initialize must run under Lock. All metadata and physical inventory checks,
// format reservation and schema installation share one transaction. Existing
// format is checked against the full expected DDL, never silently repaired.
func Initialize(ctx context.Context, db *sql.DB, inventory func(context.Context) error) (sqlite.ManagedStoreFormat, error) {
	if db == nil {
		return sqlite.ManagedStoreFormat{}, sqlite.ErrManagedPreflightUnavailable
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000"); err != nil {
		return sqlite.ManagedStoreFormat{}, failure(ctx)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return sqlite.ManagedStoreFormat{}, failure(ctx)
	}
	defer tx.Rollback()
	var existing int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name='managed_store_format' COLLATE NOCASE").Scan(&existing); err != nil {
		return sqlite.ManagedStoreFormat{}, failure(ctx)
	}
	format, err := sqlite.EnsureManagedStoreFormat(ctx, tx, inventory)
	if err != nil {
		return sqlite.ManagedStoreFormat{}, err
	}
	if existing == 0 {
		if err := install(ctx, tx); err != nil {
			return sqlite.ManagedStoreFormat{}, err
		}
	}
	expected, err := expectedSchema(ctx)
	if err != nil {
		return sqlite.ManagedStoreFormat{}, err
	}
	actual, err := schema(ctx, tx)
	if err != nil {
		return sqlite.ManagedStoreFormat{}, err
	}
	for name, definition := range expected {
		if actual[name] != definition {
			return sqlite.ManagedStoreFormat{}, sqlite.ErrManagedPreflightUnavailable
		}
	}
	if err := tx.Commit(); err != nil {
		return sqlite.ManagedStoreFormat{}, failure(ctx)
	}
	return format, nil
}

func install(ctx context.Context, tx *sql.Tx) error {
	for _, fn := range []func(context.Context, *sql.Tx) error{sqlite.InstallManagedStateSchema, queue.InstallManagedSchema, instanceaccess.InstallSchema} {
		if err := fn(ctx, tx); err != nil {
			return err
		}
	}
	// Cache eviction retires the capture certificate, never the lineage. A later
	// rebuild of the same logical key must record its own exact physical proof.
	if _, err := tx.ExecContext(ctx, `CREATE TRIGGER managed_state_seal_eviction AFTER DELETE ON states BEGIN DELETE FROM managed_state_seals WHERE state_id=OLD.state_id; END`); err != nil {
		return failure(ctx)
	}
	return nil
}

// Build the reference in a disposable in-memory database. This avoids a second,
// hand-maintained schema catalogue and performs no write on the user's store.
func expectedSchema(ctx context.Context) (map[string]string, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, failure(ctx)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, failure(ctx)
	}
	defer tx.Rollback()
	if _, err := sqlite.EnsureManagedStoreFormat(ctx, tx, func(context.Context) error { return nil }); err != nil {
		return nil, err
	}
	if err := install(ctx, tx); err != nil {
		return nil, err
	}
	return schema(ctx, tx)
}

func schema(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT name,coalesce(sql,'') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, failure(ctx)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, definition string
		if rows.Scan(&name, &definition) != nil {
			return nil, failure(ctx)
		}
		out[name] = strings.Join(strings.Fields(definition), " ")
	}
	if rows.Err() != nil {
		return nil, failure(ctx)
	}
	return out, nil
}

func failure(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return sqlite.ErrManagedPreflightUnavailable
}
