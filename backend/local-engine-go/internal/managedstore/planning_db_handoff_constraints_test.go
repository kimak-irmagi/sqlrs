package managedstore

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// PG01 verifies key grouping and actions, which cannot be established by a
// flattened list of FK columns or by hashes generated from the same DDL.
// Requirements: issue89-planning-db-handoff-schema.md §1.
func TestPlanningDBHandoffRelationalKeys(t *testing.T) {
	db := niOpen(t, filepath.Join(t.TempDir(), "state.db"))
	_, err := Initialize(t.Context(), db, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	const table = "prepare_liquibase_planning_db_ownership"
	rows, err := db.Query("SELECT name,type,\"notnull\",pk FROM pragma_table_info(?) ORDER BY cid", table)
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	for rows.Next() {
		var name, kind string
		var required, primary int
		if err := rows.Scan(&name, &kind, &required, &primary); err != nil {
			t.Fatal(err)
		}
		// SQLite may report either nullability flag for a TEXT primary key
		// depending on an explicit NOT NULL spelling. The approved contract
		// specifies its PK, not that incidental spelling.
		if name == "job_id" {
			required = 0
		}
		actual = append(actual, fmt.Sprintf("%s:%s:%d:%d", name, kind, required, primary))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	want := []string{"job_id:TEXT:0:1", "task_id:TEXT:1:0", "attempt_no:INTEGER:1:0", "clone_id:TEXT:1:0", "retained_evidence_id:INTEGER:1:0", "plan_digest:TEXT:1:0", "session_id:TEXT:0:0", "first_task_id:TEXT:0:0", "first_execution_digest:TEXT:0:0", "transferred_at:TEXT:0:0", "disposal_requested_evidence_id:INTEGER:0:0", "disposal_evidence_id:INTEGER:0:0", "disposed_at:TEXT:0:0"}
	if !reflect.DeepEqual(actual, want) {
		t.Fatal("ownership column types/nullability differ", actual)
	}
	rows, err = db.Query("SELECT id,seq,\"table\",\"from\",\"to\",on_update,on_delete FROM pragma_foreign_key_list(?) ORDER BY id,seq", table)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[int][]string{}
	for rows.Next() {
		var id, seq int
		var target, from, to, update, deletion string
		if err := rows.Scan(&id, &seq, &target, &from, &to, &update, &deletion); err != nil {
			t.Fatal(err)
		}
		groups[id] = append(groups[id], target+":"+from+"->"+to+":"+update+":"+deletion)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	actual = nil
	for _, group := range groups {
		actual = append(actual, strings.Join(group, ","))
	}
	want = []string{
		"prepare_jobs:job_id->job_id:NO ACTION:CASCADE",
		"prepare_step_attempts:job_id->job_id:NO ACTION:CASCADE,prepare_step_attempts:task_id->task_id:NO ACTION:CASCADE,prepare_step_attempts:attempt_no->attempt_no:NO ACTION:CASCADE",
		"prepare_clones:clone_id->clone_id:NO ACTION:CASCADE",
		"prepare_attempt_evidence:retained_evidence_id->evidence_id:NO ACTION:CASCADE",
		"prepare_liquibase_sessions:session_id->session_id:NO ACTION:CASCADE",
		"prepare_tasks:job_id->job_id:NO ACTION:NO ACTION,prepare_tasks:first_task_id->task_id:NO ACTION:NO ACTION",
		"prepare_execution_bindings:first_execution_digest->execution_digest:NO ACTION:NO ACTION",
		"prepare_attempt_evidence:disposal_requested_evidence_id->evidence_id:NO ACTION:CASCADE",
		"prepare_attempt_evidence:disposal_evidence_id->evidence_id:NO ACTION:CASCADE",
	}
	sort.Strings(actual)
	sort.Strings(want)
	if !reflect.DeepEqual(actual, want) {
		t.Fatal("composite keys or FK actions differ", actual)
	}
	rows, err = db.Query(`SELECT group_concat(ii.name,',') FROM pragma_index_list(?) il JOIN pragma_index_info(il.name) ii WHERE il."unique"=1 GROUP BY il.name ORDER BY il.name`, table)
	if err != nil {
		t.Fatal(err)
	}
	actual = nil
	for rows.Next() {
		var columns string
		if err := rows.Scan(&columns); err != nil {
			t.Fatal(err)
		}
		actual = append(actual, columns)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	want = []string{"job_id", "clone_id", "retained_evidence_id", "session_id", "disposal_requested_evidence_id", "disposal_evidence_id"}
	sort.Strings(actual)
	sort.Strings(want)
	if !reflect.DeepEqual(actual, want) {
		t.Fatal("ownership uniqueness differs", actual)
	}
}

// PG01 rejects damage specifically to the newly introduced namespace before
// inventory or any startup mutation, across two physical reopenings.
func TestPlanningDBHandoffRejectsCatalogDamage(t *testing.T) {
	for _, kind := range []string{"index", "trigger", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niOpen(t, path)
			if _, err := Initialize(t.Context(), db, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='prepare_liquibase_planning_db_ownership'").Scan(&count); err != nil || count != 1 {
				t.Fatal("positive ownership schema missing", err)
			}
			if kind == "unknown" {
				niExec(t, db, "CREATE TABLE prepare_planning_db_unknown(id TEXT)")
			} else {
				var name string
				if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type=? AND tbl_name='prepare_liquibase_planning_db_ownership' AND sql IS NOT NULL ORDER BY name LIMIT 1", kind).Scan(&name); err != nil {
					t.Fatal("positive schema object missing", err)
				}
				niExec(t, db, "DROP "+strings.ToUpper(kind)+" \""+strings.ReplaceAll(name, "\"", "\"\"")+"\"")
			}
			before := niSnapshotAll(t, db)
			db.Close()
			for reopen := 0; reopen < 2; reopen++ {
				trace := &niSQLTrace{}
				current := openNIAudited(t, path, trace)
				_, err := Initialize(t.Context(), current, func(context.Context) error { t.Fatal("damaged ownership catalog reached inventory"); return nil })
				if err == nil || trace.mutations() != 0 {
					t.Fatal("damaged ownership catalog accepted or repaired", err)
				}
				niAssertUnchanged(t, current, before)
				current.Close()
			}
		})
	}
}
