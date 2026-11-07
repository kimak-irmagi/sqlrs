package queue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NI01 compares actual SQLite definitions against the frozen composed target,
// not a map returned by the production verifier. No prototype is run in CI.
func TestMainPathInstallationArtifact(t *testing.T) {
	fixtureRoot := filepath.Join("..", "..", "managedstore", "testdata")
	// Locks were obtained at extraction, not calculated by the verifier.
	// Regenerating both schema and oracle must not silently approve history.
	for path, want := range map[string]string{
		filepath.Join("assets", "liquibase-main-schema.sql"):        "d06c7a45ee3c125f7d01bf3435c590668710c70aef7343b6322a602285d0cc14",
		filepath.Join("assets", "liquibase-main-catalog.json"):      "5408e2b6f0bfde18bfaa7d1cec6027da87ab97f7db54a7deb875d25f1082ac41",
		filepath.Join(fixtureRoot, "issue89-target-catalog.json"):   "d9b373979af157eecded08f34405bd1025e78137bca48ce082ba1acb6ec6d288",
		filepath.Join(fixtureRoot, "issue89-released.sql"):          "3959fbec836ba13386a3a2fb3024f111c55a52a56b369d0537ff02795037674e",
		filepath.Join(fixtureRoot, "issue89-released-catalog.json"): "2e2821f68340c57bb0cfaa0063a60edd4d9d78cd1c42e765c2950976809d53f5",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("unreviewed extraction change: %s", path)
		}
	}
	provenanceBytes, err := os.ReadFile(filepath.Join(fixtureRoot, "issue89-schema-provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var provenance struct {
		Released struct {
			Tag     string `json:"tag"`
			Commit  string `json:"commit"`
			Objects int    `json:"catalog_objects"`
		} `json:"released"`
		Composed struct {
			Head    string `json:"source_head"`
			Dirty   bool   `json:"source_worktree_has_uncommitted_changes"`
			Objects int    `json:"catalog_objects"`
		} `json:"composed"`
		Artifacts map[string]string `json:"artifact_sha256"`
		Sources   map[string]string `json:"source_files_sha256"`
	}
	if err := json.Unmarshal(provenanceBytes, &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance.Released.Tag != "v0.2.0-rc.1" || provenance.Released.Commit != "36e164a" || provenance.Released.Objects != 57 ||
		provenance.Composed.Head != "c1104770f8752a8d41b8bb0a01061b77dc97933a" || !provenance.Composed.Dirty || provenance.Composed.Objects != 282 || len(provenance.Sources) != 144 {
		t.Fatal("extraction provenance no longer identifies the reviewed sources")
	}
	for name, digest := range provenance.Sources {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || !strings.HasPrefix(name, "internal/") || strings.Contains(name, "..") || strings.Contains(name, `\`) {
			t.Fatalf("invalid recorded source identity: %s", name)
		}
	}
	for name, want := range provenance.Artifacts {
		file := filepath.Join(fixtureRoot, name)
		if strings.HasPrefix(name, "liquibase-main-") {
			file = filepath.Join("assets", name)
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("artifact differs from recorded provenance: %s", name)
		}
	}
	expectedBytes, err := os.ReadFile(filepath.Join(fixtureRoot, "issue89-target-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]struct{ Type, Table, SHA256 string }
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		t.Fatal(err)
	}
	// OT-WH02: three handoff guards extend the target; the composed historical
	// 282-object source above remains frozen and is not rebased.
	if len(expected) != 285 {
		t.Fatalf("source catalog changed without review: %d", len(expected))
	}
	baseline, err := os.ReadFile(filepath.Join(fixtureRoot, "issue89-released.sql"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(string(baseline)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := InstallLiquibaseMainPathSchema(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("SELECT name,type,tbl_name,sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		var name, kind, table, definition string
		if err := rows.Scan(&name, &kind, &table, &definition); err != nil {
			t.Fatal(err)
		}
		want, ok := expected[name]
		digest := sha256.Sum256([]byte(strings.TrimSpace(strings.ReplaceAll(definition, "\r\n", "\n"))))
		if !ok || want.Type != kind || want.Table != table || want.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("independent catalog differs at %s", name)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if count != len(expected) {
		t.Fatalf("incomplete artifact: %d/%d", count, len(expected))
	}
	// The independent private/v2 column contract is intentionally not read from
	// the generated artifact or installer, even when their hashes agree.
	for table, want := range map[string]string{
		"prepare_liquibase_session_readiness":   "session_id runner_id ready_digest ready_bytes verified_at",
		"prepare_liquibase_capture_slots":       "session_id slot_no job_id task_id attempt_no reserved_at prior_retirement_ref",
		"prepare_liquibase_capture_retirements": "retirement_ref job_id task_id attempt_no session_id reason requested_at verified_at absence_kind",
	} {
		columns, err := db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for columns.Next() {
			var cid, notnull, pk int
			var name, kind string
			var dflt any
			if err := columns.Scan(&cid, &name, &kind, &notnull, &dflt, &pk); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
		if err := columns.Err(); err != nil {
			t.Fatal(err)
		}
		columns.Close()
		if strings.Join(names, " ") != want {
			t.Fatalf("private columns differ for %s: %v", table, names)
		}
	}
	check, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if check.Next() {
		t.Fatal("artifact has invalid FK dependencies")
	}
	if err := check.Err(); err != nil {
		t.Fatal(err)
	}
	check.Close()
}
