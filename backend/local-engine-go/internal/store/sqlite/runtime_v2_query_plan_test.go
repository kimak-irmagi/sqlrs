package sqlite

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeV2RequiredQueriesUseDeclaredIndexes(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cases := []struct {
		name, query, index string
		args               []any
	}{
		{"state", `SELECT state_json FROM runtime_v2_states WHERE state_id=? AND record_version=?`, "sqlite_autoindex_runtime_v2_states_1", []any{"id", RuntimeV2RecordVersion}},
		{"parent", `SELECT state_json FROM runtime_v2_states WHERE parent_state_id=?`, "runtime_v2_states_parent", []any{"id"}},
		{"provenance page", `SELECT insertion_seq FROM runtime_v2_provenance_observations WHERE state_id=? AND insertion_seq>? AND insertion_seq<=? ORDER BY insertion_seq ASC LIMIT ?`, "runtime_v2_provenance_state_page", []any{"id", 0, 10, 2}},
		{"cache", `SELECT record_version,cache_record_json,stored_at FROM runtime_v2_resolutions WHERE cache_key=?`, "sqlite_autoindex_runtime_v2_resolutions_1", []any{strings.Repeat("a", 64)}},
		{"material page", `SELECT insertion_seq FROM runtime_v2_materializations WHERE state_id=? AND insertion_seq<=? ORDER BY insertion_seq DESC LIMIT ?`, "runtime_v2_materializations_state_page", []any{"id", 10, 2}},
		{"components", `SELECT name FROM runtime_v2_materialization_components WHERE materialization_id=? ORDER BY name`, "sqlite_autoindex_runtime_v2_materialization_components_1", []any{"id"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rows, err := store.db.Query(`EXPLAIN QUERY PLAN `+test.query, test.args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			details := []string{}
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				details = append(details, detail)
			}
			joined := strings.Join(details, "\n")
			if !strings.Contains(joined, test.index) {
				t.Fatalf("plan did not use %s:\n%s", test.index, joined)
			}
			if strings.Contains(strings.ToUpper(joined), "SCAN RUNTIME_V2_") {
				t.Fatalf("plan used full scan:\n%s", joined)
			}
		})
	}
}
