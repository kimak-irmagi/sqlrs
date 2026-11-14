package managedstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// PG01 specifies the approved schema independently of regenerated catalog hashes.
// See docs/architecture/issue89-planning-db-handoff-tests.md and its schema §1/6.
func TestPlanningDBHandoffSchema(t *testing.T) {
	for _, released := range []bool{false, true} {
		name := "fresh"
		if released {
			name = "released"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niOpen(t, path)
			if released {
				db.Close()
				db = niBaseline(t, path)
			}
			if _, err := Initialize(t.Context(), db, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			for reopen := 0; reopen < 3; reopen++ {
				if reopen > 0 {
					db.Close()
					db = niOpen(t, path)
					if _, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("current store reached inventory"); return nil }); err != nil {
						t.Fatal(err)
					}
				}
				var version int
				if err := db.QueryRow("SELECT format_version FROM prepare_liquibase_session_store_format WHERE slot=1").Scan(&version); err != nil || version != 4 {
					t.Fatalf("planning DB ownership requires format 4: version=%d err=%v", version, err)
				}
				rows, err := db.Query("PRAGMA table_info(prepare_liquibase_planning_db_ownership)")
				if err != nil {
					t.Fatal(err)
				}
				var names []string
				for rows.Next() {
					var ordinal, required, primary int
					var name, kind string
					var defaultValue sql.NullString
					if err := rows.Scan(&ordinal, &name, &kind, &required, &defaultValue, &primary); err != nil {
						t.Fatal(err)
					}
					names = append(names, name)
					if name == "job_id" && (primary != 1 || kind != "TEXT") {
						t.Fatal("job ownership is not the primary key")
					}
					if defaultValue.Valid {
						t.Fatal("ownership field has an invented default", name)
					}
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				rows.Close()
				want := []string{"job_id", "task_id", "attempt_no", "clone_id", "retained_evidence_id", "plan_digest", "session_id", "first_task_id", "first_execution_digest", "transferred_at", "disposal_requested_evidence_id", "disposal_evidence_id", "disposed_at"}
				if !reflect.DeepEqual(names, want) {
					t.Fatal("ownership column contract differs", names)
				}
				rows, err = db.Query("PRAGMA foreign_key_list(prepare_liquibase_planning_db_ownership)")
				if err != nil {
					t.Fatal(err)
				}
				var foreign []string
				for rows.Next() {
					var id, seq int
					var table, from, to, update, deletion, match string
					if err := rows.Scan(&id, &seq, &table, &from, &to, &update, &deletion, &match); err != nil {
						t.Fatal(err)
					}
					foreign = append(foreign, from+"->"+table+"."+to)
					if table == "managed_runtime_operations" || table == "states" || table == "instances" {
						t.Fatal("historical ownership permanently pins a physical resource")
					}
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				rows.Close()
				expectedForeign := []string{"job_id->prepare_jobs.job_id", "job_id->prepare_step_attempts.job_id", "task_id->prepare_step_attempts.task_id", "attempt_no->prepare_step_attempts.attempt_no", "clone_id->prepare_clones.clone_id", "retained_evidence_id->prepare_attempt_evidence.evidence_id", "session_id->prepare_liquibase_sessions.session_id", "job_id->prepare_tasks.job_id", "first_task_id->prepare_tasks.task_id", "first_execution_digest->prepare_execution_bindings.execution_digest", "disposal_requested_evidence_id->prepare_attempt_evidence.evidence_id", "disposal_evidence_id->prepare_attempt_evidence.evidence_id"}
				sort.Strings(foreign)
				sort.Strings(expectedForeign)
				if !reflect.DeepEqual(foreign, expectedForeign) {
					t.Fatal("ownership foreign keys differ", foreign)
				}
				var definition string
				if err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='prepare_liquibase_planning_db_ownership'").Scan(&definition); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(strings.ToUpper(definition), "DEFERRABLE INITIALLY DEFERRED") {
					t.Fatal("session FK is not deferred; runtime COMMIT proof is covered by PG04")
				}
				rows, err = db.Query("SELECT sql FROM sqlite_master WHERE type='index' AND tbl_name='prepare_liquibase_planning_db_ownership' AND sql IS NOT NULL")
				if err != nil {
					t.Fatal(err)
				}
				liveIndex := false
				for rows.Next() {
					var sqlText string
					if err := rows.Scan(&sqlText); err != nil {
						t.Fatal(err)
					}
					compact := strings.ToLower(strings.Join(strings.Fields(sqlText), " "))
					if strings.Contains(compact, "where session_id is null and disposed_at is null") {
						liveIndex = true
					}
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				rows.Close()
				if !liveIndex {
					t.Fatal("active ownership has no partial index")
				}
				niAssertTarget(t, db)
			}
		})
	}
}

// A prototype version is not an implicit migration source, including a fully
// populated current-looking catalog whose only corruption is its format marker.
func TestPlanningDBHandoffSchemaRejectsPrototype(t *testing.T) {
	for _, version := range []string{"3", "5"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niOpen(t, path)
			if _, err := Initialize(t.Context(), db, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			var marker string
			if err := db.QueryRow("SELECT sql FROM sqlite_master WHERE name='prepare_liquibase_session_store_format' AND type='table'").Scan(&marker); err != nil {
				t.Fatal(err)
			}
			// Start with the exact supported definition, not a differently formatted
			// replacement that might fail catalog validation for the wrong reason.
			if strings.Count(marker, "CHECK(format_version=4)") != 1 {
				t.Fatal("positive format-4 control is missing")
			}
			if _, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("positive control reached inventory"); return nil }); err != nil {
				t.Fatal("positive format-4 control", err)
			}
			marker = strings.Replace(marker, "CHECK(format_version=4)", "CHECK(format_version="+version+")", 1)
			niExec(t, db, "DROP TABLE prepare_liquibase_session_store_format; "+marker+"; INSERT INTO prepare_liquibase_session_store_format VALUES(1,"+version+")")
			before := niSnapshotAll(t, db)
			db.Close()
			for reopen := 0; reopen < 2; reopen++ {
				trace := &niSQLTrace{}
				current := openNIAudited(t, path, trace)
				_, err := Initialize(t.Context(), current, func(context.Context) error { t.Fatal("unsupported schema reached inventory"); return nil })
				if err == nil || trace.mutations() != 0 {
					t.Fatal("unsupported planning ownership format accepted or mutated", version, err)
				}
				niAssertUnchanged(t, current, before)
				current.Close()
			}
		})
	}
}
