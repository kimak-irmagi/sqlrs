package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

var (
	// ErrLegacyManagedData requires an operator-approved disposition or migration;
	// startup must preserve the existing store and its physical resources.
	ErrLegacyManagedData = errors.New("legacy managed database data prevents upgrade; retain the store and arrange an explicit migration")
	// ErrManagedPreflightUnavailable deliberately omits driver diagnostics, SQL,
	// paths and stored values. Uncertain inventory must never authorize migration.
	ErrManagedPreflightUnavailable = errors.New("managed database metadata preflight unavailable; store must remain unchanged")
)

// CheckEmptyManagedMetadata reads a consistent inventory of the fixed managed
// tables without migrations, PRAGMA changes or data writes. The caller must run
// it before Store.New, with admission closed and the legacy format established.
// Success proves only metadata emptiness: physical/runtime inventory and durable
// format selection are separate gates. See
// docs/architecture/managed-database-identity-internals.md, upgrade preflight.
func CheckEmptyManagedMetadata(ctx context.Context, db *sql.DB) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil {
		return ErrManagedPreflightUnavailable
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return managedPreflightError(ctx, err)
	}
	defer tx.Rollback()
	return checkEmptyManagedTables(ctx, tx)
}

// checkEmptyManagedTables shares the same read snapshot with the startup
// transaction that installs the format and schema after physical inventory.
func checkEmptyManagedTables(ctx context.Context, tx *sql.Tx) error {
	for _, table := range []string{"states", "instances", "names", "prepare_jobs", "prepare_tasks", "prepare_events", "instance_access"} {
		var count int
		var kind string
		err := tx.QueryRowContext(ctx, `SELECT count(*), coalesce(min(type), '')
 FROM main.sqlite_master WHERE name = ? COLLATE NOCASE`, table).Scan(&count, &kind)
		if err != nil {
			return managedPreflightError(ctx, err)
		}
		if count == 0 {
			continue
		}
		if count != 1 || kind != "table" {
			return ErrManagedPreflightUnavailable
		}
		var populated bool
		// Identifiers come only from the fixed list above. Explicit main schema
		// qualification prevents temporary objects from shadowing the inventory.
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main."`+table+`" LIMIT 1)`).Scan(&populated); err != nil {
			return managedPreflightError(ctx, err)
		}
		if populated {
			return ErrLegacyManagedData
		}
	}
	return ctx.Err()
}

// managedPreflightError preserves cancellation while bounding all storage errors.
func managedPreflightError(ctx context.Context, err error) error {
	if canceled := ctx.Err(); canceled != nil {
		return canceled
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return ErrManagedPreflightUnavailable
}
