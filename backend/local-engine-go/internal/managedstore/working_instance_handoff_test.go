package managedstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// WH01 independently specifies the approved transfer shape, rather than
// accepting a regenerated catalog as its own oracle. See
// docs/architecture/issue89-working-instance-handoff-tests.md.
func TestWorkingInstanceHandoffSchema(t *testing.T) {
	for _, released := range []bool{false, true} {
		name := "fresh"
		if released {
			name = "released"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niOpen(t, path)
			if released {
				_ = db.Close()
				db = niBaseline(t, path)
			}
			if _, err := Initialize(t.Context(), db, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			for reopen := 0; reopen < 3; reopen++ {
				if reopen > 0 {
					_ = db.Close()
					db = niOpen(t, path)
					if _, err := Initialize(t.Context(), db, func(context.Context) error {
						t.Fatal("current store must not inventory on reopen")
						return nil
					}); err != nil {
						t.Fatal(err)
					}
				}
				var version int
				if err := db.QueryRow("SELECT format_version FROM prepare_liquibase_session_store_format WHERE slot=1").Scan(&version); err != nil || version != 3 {
					t.Fatalf("handoff requires session format 3: version=%d err=%v", version, err)
				}
				rows, err := db.Query("PRAGMA table_info(prepare_liquibase_session_disposals)")
				if err != nil {
					t.Fatal(err)
				}
				got := map[string]string{}
				for rows.Next() {
					var index, required, primary int
					var column, kind string
					var defaultValue sql.NullString
					if err := rows.Scan(&index, &column, &kind, &required, &defaultValue, &primary); err != nil {
						t.Fatal(err)
					}
					if strings.HasPrefix(column, "handoff_") {
						if required != 0 || defaultValue.Valid || primary != 0 {
							t.Fatalf("historical handoff field must be nullable without default: %s", column)
						}
						got[column] = kind
					}
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				_ = rows.Close()
				want := map[string]string{"handoff_job_id": "TEXT", "handoff_instance_id": "TEXT", "handoff_state_id": "TEXT", "handoff_operation_ref": "TEXT", "handoff_runtime_dir": "TEXT", "handoff_outcome_digest": "TEXT"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("handoff column contract differs: got=%v", got)
				}
				rows, err = db.Query("PRAGMA foreign_key_list(prepare_liquibase_session_disposals)")
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for rows.Next() {
					var id, seq int
					var table, from, to, update, deletion, match string
					if err := rows.Scan(&id, &seq, &table, &from, &to, &update, &deletion, &match); err != nil {
						t.Fatal(err)
					}
					if strings.HasPrefix(from, "handoff_") {
						if found || from != "handoff_job_id" || table != "prepare_liquibase_job_finalizations" || to != "job_id" || deletion != "CASCADE" {
							t.Fatalf("unexpected historical FK: %s -> %s.%s", from, table, to)
						}
						found = true
					}
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				_ = rows.Close()
				if !found {
					t.Fatal("missing deferred outcome linkage")
				}
				niAssertTarget(t, db)
			}
		})
	}
}

// WH01 fail-before-write checks target the new component itself. They retain
// all old damage cases, rather than changing their historical expectations.
func TestWorkingInstanceHandoffSchemaDamage(t *testing.T) {
	for _, variant := range []string{"prototype-2", "missing-column", "missing-guard", "altered-guard"} {
		t.Run(variant, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niOpen(t, path)
			_, err := Initialize(t.Context(), db, func(context.Context) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			niAssertTarget(t, db)
			switch variant {
			case "prototype-2":
				niExec(t, db, "DROP TABLE prepare_liquibase_session_store_format; CREATE TABLE prepare_liquibase_session_store_format(slot INTEGER PRIMARY KEY CHECK(slot=1),format_version INTEGER NOT NULL CHECK(format_version=2)); INSERT INTO prepare_liquibase_session_store_format VALUES(1,2)")
			case "missing-column":
				niExec(t, db, "ALTER TABLE prepare_liquibase_session_disposals RENAME COLUMN handoff_runtime_dir TO unsupported_handoff_runtime_dir")
			case "missing-guard":
				niExec(t, db, "DROP TRIGGER prepare_liquibase_handoff_resume")
			case "altered-guard":
				niExec(t, db, "DROP TRIGGER prepare_liquibase_handoff_resume; CREATE TRIGGER prepare_liquibase_handoff_resume BEFORE UPDATE ON prepare_liquibase_sessions BEGIN SELECT 1; END")
			}
			before := niSnapshotAll(t, db)
			_ = db.Close()
			for reopen := 0; reopen < 2; reopen++ {
				trace := &niSQLTrace{}
				db = openNIAudited(t, path, trace)
				_, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("damaged target reached inventory"); return nil })
				if err == nil || trace.mutations() != 0 {
					t.Fatal("damaged handoff schema admitted or mutated", err)
				}
				niAssertUnchanged(t, db, before)
				_ = db.Close()
			}
		})
	}
}
