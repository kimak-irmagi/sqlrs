package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	_ "modernc.org/sqlite"
)

const RuntimeV2RecordVersion = "sqlrs.runtime-persistence.v1"

var ErrRuntimeV2Schema = errors.New("sqlite: invalid Runtime v2 schema")

type runtimeV2SchemaObject struct {
	kind, table, sql string
}

// InstallRuntimeV2Schema installs or verifies the complete reserved namespace
// in the caller's transaction. It never repairs a partial or altered schema.
// Requirements: docs/architecture/runtime-v2-persistence-structure.md.
func InstallRuntimeV2Schema(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return ErrRuntimeV2Schema
	}
	actual, err := readRuntimeV2Manifest(ctx, tx)
	if err != nil {
		return err
	}
	if len(actual) == 0 {
		ddl := RuntimeV2SchemaSQL()
		if ddl == "" {
			return ErrRuntimeV2Schema
		}
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_v2_store_format(slot,format_version,semantic_version) VALUES(1,?,?)`, RuntimeV2RecordVersion, "sqlrs.runtime.v2"); err != nil {
			return err
		}
		return nil
	}
	expected, err := expectedRuntimeV2Manifest(ctx)
	if err != nil || !sameRuntimeV2Manifest(actual, expected) {
		return ErrRuntimeV2Schema
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_v2_store_format WHERE slot=1 AND format_version=? AND semantic_version=?`, RuntimeV2RecordVersion, "sqlrs.runtime.v2").Scan(&count); err != nil || count != 1 {
		return ErrRuntimeV2Schema
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_v2_store_format`).Scan(&count); err != nil || count != 1 {
		return ErrRuntimeV2Schema
	}
	return nil
}

// EnsureRuntimeV2Schema is the transactional constructor wrapper shared by
// direct SQLite users.
func EnsureRuntimeV2Schema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return ErrRuntimeV2Schema
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON; PRAGMA busy_timeout=0`); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := InstallRuntimeV2Schema(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

type runtimeV2ManifestReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readRuntimeV2Manifest(ctx context.Context, reader runtimeV2ManifestReader) (map[string]runtimeV2SchemaObject, error) {
	rows, err := reader.QueryContext(ctx, `SELECT name,type,tbl_name,coalesce(sql,'') FROM sqlite_master WHERE lower(name) LIKE 'runtime_v2_%' ORDER BY lower(name),name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]runtimeV2SchemaObject{}
	for rows.Next() {
		var name string
		var object runtimeV2SchemaObject
		if err := rows.Scan(&name, &object.kind, &object.table, &object.sql); err != nil {
			return nil, err
		}
		result[name] = runtimeV2SchemaObject{object.kind, object.table, normalizeRuntimeV2SQL(object.sql)}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func expectedRuntimeV2Manifest(ctx context.Context) (map[string]runtimeV2SchemaObject, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, RuntimeV2SchemaSQL()); err != nil {
		return nil, err
	}
	return readRuntimeV2Manifest(ctx, db)
}

func sameRuntimeV2Manifest(actual, expected map[string]runtimeV2SchemaObject) bool {
	if len(actual) != len(expected) {
		return false
	}
	for name, want := range expected {
		got, ok := actual[name]
		if !ok || got != want {
			return false
		}
	}
	return true
}

func normalizeRuntimeV2SQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
