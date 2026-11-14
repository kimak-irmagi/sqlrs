// Package managedstore coordinates the owned store startup before admission.
package managedstore

import (
	"context"
	"database/sql"
	"strings"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/store/sqlite"
)

// Initialize runs under Lock and returns only a completely verified store.
// Final Liquibase definitions are installed directly; only the exact released
// job-table bridge is supported. See issue89-normal-path-storage-extraction.md.
func Initialize(ctx context.Context, db *sql.DB, inventory func(context.Context) error) (sqlite.ManagedStoreFormat, error) {
	return initializeMainPath(ctx, db, inventory)
}

func install(ctx context.Context, tx *sql.Tx) error {
	for _, fn := range []func(context.Context, *sql.Tx) error{sqlite.InstallManagedStateSchema, instanceaccess.InstallSchema, sqlite.InstallRuntimeV2Schema} {
		if err := fn(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "CREATE TRIGGER managed_state_seal_eviction AFTER DELETE ON states BEGIN DELETE FROM managed_state_seals WHERE state_id=OLD.state_id; END"); err != nil {
		return failure(ctx)
	}
	return queue.InstallLiquibaseMainPathSchema(ctx, tx)
}

// The reference is the mechanically frozen final catalog, not a replay of
// historical migrations. Each caller owns its map; real-store checks are fresh.
func expectedSchema(ctx context.Context) (map[string]string, error) {
	return queue.MainPathSchemaDefinitions(ctx)
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
