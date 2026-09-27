package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runtimeV2Manifest(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT name, type FROM sqlite_master WHERE lower(name) LIKE 'runtime_v2_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			t.Fatal(err)
		}
		result[name] = kind
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRuntimeV2SchemaUpgradePreservesLegacyAndUnrelatedData(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "rc6.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(SchemaSQL()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
INSERT INTO states(state_id,state_fingerprint,image_id,prepare_kind,prepare_args_normalized,created_at) VALUES('legacy-state','legacy-fingerprint','image','psql','{}','2026-01-01');
INSERT INTO instances(instance_id,state_id,image_id,created_at) VALUES('legacy-instance','legacy-state','image','2026-01-01');
INSERT INTO names(name,instance_id,state_id,state_fingerprint,image_id,is_primary) VALUES('legacy','legacy-instance','legacy-state','legacy-fingerprint','image',1);
CREATE TABLE unrelated(value BLOB); INSERT INTO unrelated VALUES(x'000102ff');`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRuntimeV2Schema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var state, instance, name string
	var blob []byte
	if err := db.QueryRow(`SELECT state_id FROM states`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT instance_id FROM instances`).Scan(&instance); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT name FROM names`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT value FROM unrelated`).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	if state != "legacy-state" || instance != "legacy-instance" || name != "legacy" || string(blob) != string([]byte{0, 1, 2, 255}) {
		t.Fatalf("legacy/unrelated data changed: %q %q %q %v", state, instance, name, blob)
	}
}

func TestRuntimeV2SchemaInstallationRollsBackAtomically(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := InstallRuntimeV2Schema(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := runtimeV2Manifest(t, db); len(got) != 0 {
		t.Fatalf("rollback left partial schema: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx, err = db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := InstallRuntimeV2Schema(ctx, tx); err == nil {
		t.Fatal("cancelled install succeeded")
	}
}

func TestSQLiteConstructionFailsFastUnderExternalWriteLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.db")
	locker, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close()
	if _, err := locker.Exec(`PRAGMA busy_timeout=0; CREATE TABLE unrelated(value TEXT); BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	defer locker.Exec(`ROLLBACK`)
	started := time.Now()
	blocked, err := Open(path)
	if blocked != nil {
		blocked.Close()
	}
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("constructor succeeded under external write lock")
	}
	if elapsed > time.Second {
		t.Fatalf("constructor waited instead of failing fast: %s", elapsed)
	}
}

func TestRuntimeV2SchemaFreshInstallAndIdempotence(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := EnsureRuntimeV2Schema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"runtime_v2_store_format": "table", "runtime_v2_states": "table", "runtime_v2_states_parent": "index",
		"runtime_v2_provenance_observations": "table", "runtime_v2_provenance_state_page": "index",
		"runtime_v2_resolutions": "table", "runtime_v2_materializations": "table",
		"runtime_v2_materializations_state_page": "index", "runtime_v2_materialization_components": "table",
		"runtime_v2_states_immutable": "trigger", "runtime_v2_provenance_immutable": "trigger",
		"runtime_v2_materializations_immutable": "trigger", "runtime_v2_materialization_components_immutable": "trigger",
		"runtime_v2_states_no_delete": "trigger", "runtime_v2_provenance_no_delete": "trigger",
		"runtime_v2_materializations_no_delete": "trigger", "runtime_v2_materialization_components_no_delete": "trigger",
	}
	got := runtimeV2Manifest(t, db)
	if len(got) != len(want) {
		t.Fatalf("manifest size = %d, want %d: %+v", len(got), len(want), got)
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Fatalf("%s = %q, want %q", name, got[name], kind)
		}
	}
	var format, semantic string
	if err := db.QueryRow(`SELECT format_version, semantic_version FROM runtime_v2_store_format WHERE slot=1`).Scan(&format, &semantic); err != nil {
		t.Fatal(err)
	}
	if format != RuntimeV2RecordVersion || semantic != "sqlrs.runtime.v2" {
		t.Fatalf("marker = %q %q", format, semantic)
	}
	before := runtimeV2Manifest(t, db)
	if err := EnsureRuntimeV2Schema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	after := runtimeV2Manifest(t, db)
	if len(before) != len(after) {
		t.Fatalf("idempotent manifest changed")
	}
}

func TestRuntimeV2SchemaRefusesReservedCollisionWithoutMutation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE RUNTIME_V2_STATES (wrong TEXT); CREATE TABLE unrelated (value TEXT); INSERT INTO unrelated VALUES ('kept')`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRuntimeV2Schema(context.Background(), db); err == nil {
		t.Fatal("reserved collision accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM unrelated WHERE value='kept'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unrelated row changed: %d %v", count, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='runtime_v2_store_format'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial schema installed: %d %v", count, err)
	}
}

func TestRuntimeV2SchemaRefusesPartialAndAlteredShapes(t *testing.T) {
	t.Run("wrong object kind", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec(`CREATE VIEW runtime_v2_states AS SELECT 1 AS wrong`); err != nil {
			t.Fatal(err)
		}
		if err := EnsureRuntimeV2Schema(context.Background(), db); err == nil {
			t.Fatal("wrong object kind accepted")
		}
	})
	t.Run("partial namespace", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec(`CREATE TABLE runtime_v2_store_format(slot INTEGER)`); err != nil {
			t.Fatal(err)
		}
		if err := EnsureRuntimeV2Schema(context.Background(), db); err == nil {
			t.Fatal("partial namespace repaired")
		}
	})
	mutations := map[string]string{
		"missing object":   `DROP INDEX runtime_v2_states_parent`,
		"extra column":     `ALTER TABLE runtime_v2_resolutions ADD COLUMN unexpected TEXT`,
		"altered index":    `DROP INDEX runtime_v2_materializations_state_page; CREATE INDEX runtime_v2_materializations_state_page ON runtime_v2_materializations(state_id ASC)`,
		"altered trigger":  `DROP TRIGGER runtime_v2_states_immutable; CREATE TRIGGER runtime_v2_states_immutable BEFORE UPDATE ON runtime_v2_states BEGIN SELECT RAISE(ABORT,'changed'); END`,
		"malformed marker": `DELETE FROM runtime_v2_store_format`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := EnsureRuntimeV2Schema(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if err := EnsureRuntimeV2Schema(context.Background(), db); err == nil {
				t.Fatal("altered schema accepted")
			}
		})
	}
	t.Run("unknown marker version", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := EnsureRuntimeV2Schema(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON; UPDATE runtime_v2_store_format SET format_version='future'`); err != nil {
			t.Fatal(err)
		}
		if err := EnsureRuntimeV2Schema(context.Background(), db); err == nil {
			t.Fatal("unknown marker accepted")
		}
	})
}

func TestRuntimeV2SchemaPragmasAndImmutability(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var foreignKeys, busyTimeout int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 || busyTimeout != 0 {
		t.Fatalf("pragmas foreign_keys=%d busy_timeout=%d", foreignKeys, busyTimeout)
	}
	stateID := "sha256:" + strings.Repeat("a", 64)
	if _, err := db.Exec(`INSERT INTO runtime_v2_states(state_id,record_version,state_kind,parent_state_id,state_json,resolved_identity_json,stored_at) VALUES(?,?,?,?,?,?,?)`, stateID, RuntimeV2RecordVersion, "factory", nil, `{}`, `{}`, "2026-01-01T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE runtime_v2_states SET stored_at=stored_at WHERE state_id=?`, stateID); err == nil {
		t.Fatal("immutable state updated")
	}
	if _, err := db.Exec(`DELETE FROM runtime_v2_states WHERE state_id=?`, stateID); err == nil {
		t.Fatal("immutable state deleted")
	}
}

func TestRuntimeV2DDLComesFromDelimitedCanonicalSchema(t *testing.T) {
	if !strings.Contains(schemaSQL, runtimeV2SchemaBegin) || !strings.Contains(schemaSQL, runtimeV2SchemaEnd) {
		t.Fatal("canonical schema lacks Runtime v2 delimiters")
	}
	if strings.Contains(SchemaSQL(), "runtime_v2_store_format") {
		t.Fatal("legacy schema accessor leaked Runtime v2 DDL")
	}
	if !strings.Contains(RuntimeV2SchemaSQL(), "CREATE TABLE runtime_v2_store_format") {
		t.Fatal("Runtime v2 extractor missed marker table")
	}
}
