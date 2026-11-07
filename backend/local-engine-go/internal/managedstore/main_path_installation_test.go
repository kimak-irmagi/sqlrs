package managedstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

type niCatalogObject struct {
	Type   string `json:"type"`
	Table  string `json:"table"`
	SHA256 string `json:"sha256"`
}

type niSnapshot struct {
	Catalog map[string]niCatalogObject
	Columns map[string][]string
	Rows    map[string][]string
}

// NI helpers deliberately use a frozen released fixture, not Initialize or
// current queue DDL. Requirements: issue89-normal-path-storage-tests.md.
func niOpen(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys=ON; PRAGMA busy_timeout=0"); err != nil {
		t.Fatal(err)
	}
	return db
}

func niExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("fixture SQL failed: %v", err)
	}
}

func niBaseline(t *testing.T, path string) *sql.DB {
	t.Helper()
	db := niOpen(t, path)
	body, err := os.ReadFile("testdata/issue89-released.sql")
	if err != nil {
		t.Fatal(err)
	}
	niExec(t, db, string(body))
	return db
}

func niExpected(t *testing.T) map[string]niCatalogObject {
	t.Helper()
	body, err := os.ReadFile("testdata/issue89-target-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]niCatalogObject
	if err := json.Unmarshal(body, &expected); err != nil {
		t.Fatal(err)
	}
	return expected
}

func niCatalog(t *testing.T, db *sql.DB) map[string]niCatalogObject {
	t.Helper()
	rows, err := db.Query("SELECT name,type,tbl_name,sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[string]niCatalogObject{}
	for rows.Next() {
		var name, kind, table, definition string
		if err := rows.Scan(&name, &kind, &table, &definition); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ReplaceAll(definition, "\r\n", "\n"))))
		result[name] = niCatalogObject{kind, table, hex.EncodeToString(sum[:])}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func niIdentifier(s string) string { return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\"" }

func niRows(t *testing.T, db *sql.DB, table string, columns []string) ([]string, []string) {
	t.Helper()
	selection := "*"
	if columns != nil {
		quoted := make([]string, len(columns))
		for i, c := range columns {
			quoted[i] = niIdentifier(c)
		}
		selection = strings.Join(quoted, ",")
	}
	rows, err := db.Query("SELECT " + selection + " FROM " + niIdentifier(table))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	for rows.Next() {
		values := make([]any, len(names))
		dest := make([]any, len(names))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, string(encoded))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(result)
	return names, result
}

func niSnapshotAll(t *testing.T, db *sql.DB) niSnapshot {
	t.Helper()
	result := niSnapshot{Catalog: niCatalog(t, db), Columns: map[string][]string{}, Rows: map[string][]string{}}
	for name, obj := range result.Catalog {
		if obj.Type != "table" {
			continue
		}
		result.Columns[name], result.Rows[name] = niRows(t, db, name, nil)
	}
	return result
}

func niAssertUnchanged(t *testing.T, db *sql.DB, before niSnapshot) {
	t.Helper()
	if after := niSnapshotAll(t, db); !reflect.DeepEqual(before, after) {
		t.Fatal("catalog or persisted rows changed")
	}
}

func niAssertOldRows(t *testing.T, db *sql.DB, before niSnapshot) {
	t.Helper()
	for table, cols := range before.Columns {
		_, got := niRows(t, db, table, cols)
		if !reflect.DeepEqual(before.Rows[table], got) {
			t.Fatalf("old values changed in %s", table)
		}
	}
}

func niAssertTarget(t *testing.T, db *sql.DB) {
	t.Helper()
	actual := niCatalog(t, db)
	for name, want := range niExpected(t) {
		if got, ok := actual[name]; !ok || got != want {
			t.Fatalf("target object %s differs: present=%t", name, ok)
		}
	}
	for table, want := range map[string]int{
		"liquibase_store_format": 2, "prepare_attempt_store_format": 3,
		"prepare_liquibase_session_store_format": 2, "prepare_liquibase_finalization_store_format": 2,
		"prepare_liquibase_cache_selection_store_format": 1,
	} {
		var count, version, slots int
		if err := db.QueryRow("SELECT count(*),max(format_version),sum(CASE WHEN slot=1 THEN 1 ELSE 0 END) FROM "+table).Scan(&count, &version, &slots); err != nil || count != 1 || slots != 1 || version != want {
			t.Fatalf("marker %s count/version/slots=%d/%d/%d err=%v", table, count, version, slots, err)
		}
	}
	niAssertConnection(t, db)
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violations")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func niAssertConnection(t *testing.T, db *sql.DB) {
	t.Helper()
	var fk, busy int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if fk != 1 || busy != 0 {
		t.Fatalf("unsafe connection: fk=%d busy=%d", fk, busy)
	}
}

// niSeedHistory creates SQL-valid, cryptographically consistent managed
// identities and public access references; it does not manufacture passwords.
func niSeedHistory(t *testing.T, db *sql.DB, lbStatus string) {
	t.Helper()
	var domain string
	if err := db.QueryRow("SELECT domain_ref FROM managed_store_format WHERE slot=1").Scan(&domain); err != nil {
		t.Fatal(err)
	}
	id, err := managedidentity.Generate(bytes.NewReader(bytes.Repeat([]byte{0x35}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("a", 64)
	niExec(t, db, "INSERT INTO managed_base_lineages VALUES(?,?,?,?,?,?,?,?,?)", domain, id.LineageRef, "postgres", image, strings.Repeat("b", 64), id.PolicyVersion, id.Username, id.IdentityDigest, "2026-11-07T00:00:00Z")
	niExec(t, db, "INSERT INTO states(state_id,state_fingerprint,image_id,prepare_kind,prepare_args_normalized,created_at,size_bytes,lineage_ref,identity_digest) VALUES(?,?,?,?,?,?,?,?,?)",
		strings.Repeat("c", 64), strings.Repeat("c", 64), image, "psql", "select 17", "2026-11-07T00:00:00Z", 123, id.LineageRef, id.IdentityDigest)
	runtimeBinding := managedidentity.RuntimeBinding{RuntimeRef: "runtime-seed", PhysicalIdentity: "physical-seed", IdentityBinding: id}
	if err := runtimeBinding.Validate(); err != nil {
		t.Fatal(err)
	}
	bindingJSON, err := json.Marshal(runtimeBinding)
	if err != nil {
		t.Fatal(err)
	}
	identityJSON, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	niExec(t, db, "INSERT INTO managed_runtime_operations VALUES(?,?,?,?,?,?,?)", "operation-seed", "/fixture/retained", "source-seed", string(identityJSON), runtimeBinding.PhysicalIdentity, runtimeBinding.RuntimeRef, "running")
	niExec(t, db, "INSERT INTO managed_state_seals VALUES(?,?,?)", strings.Repeat("c", 64), "operation-seed", string(bindingJSON))
	niExec(t, db, "INSERT INTO instances(instance_id,state_id,image_id,created_at,runtime_id,runtime_dir,status) VALUES(?,?,?,?,?,?,?)", "instance-seed", strings.Repeat("c", 64), image, "2026-11-07T00:00:00Z", strings.Repeat("d", 64), "/fixture/retained", "active")
	secret := instanceaccess.SecretBinding{DomainRef: domain, OwnerRef: "instance-seed", IdentityDigest: id.IdentityDigest, Version: "access-v1", Purpose: "instance"}
	if err := secret.Validate(); err != nil {
		t.Fatal(err)
	}
	access := instanceaccess.AccessBinding{RuntimeBinding: runtimeBinding, InstanceRef: "instance-seed", DomainRef: domain, AccessVersion: "access-v1", SecretRef: secret.Ref()}
	accessJSON, err := json.Marshal(access)
	if err != nil {
		t.Fatal(err)
	}
	niExec(t, db, "INSERT INTO instance_access VALUES(?,?,?,?)", access.InstanceRef, string(accessJSON), access.SecretRef, "verified")
	niExec(t, db, "CREATE TABLE settings(key TEXT PRIMARY KEY,value BLOB)")
	niExec(t, db, "INSERT INTO settings VALUES(?,?)", "unrelated", []byte{0, 1, 255, 10, 32})
	jobs := []struct{ kind, status string }{{"psql", "queued"}, {"psql", "running"}, {"psql", "succeeded"}, {"psql", "failed"}}
	if lbStatus != "" {
		jobs = append(jobs, struct{ kind, status string }{"lb", lbStatus})
	}
	for _, job := range jobs {
		jobID := job.kind + "-" + job.status
		niExec(t, db, "INSERT INTO prepare_jobs(job_id,status,prepare_kind,image_id,plan_only,snapshot_mode,prepare_args_normalized,signature,request_json,created_at,started_at,finished_at,result_json,error_json,resolved_image_id,lineage_ref,identity_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
			jobID, job.status, job.kind, "postgres:17", 0, "always", "keep args", nil, "{\"preserve\":\"request bytes\"}", "2026-11-07T00:00:00Z", nil, nil, nil, nil, image, id.LineageRef, id.IdentityDigest)
		niExec(t, db, "INSERT INTO prepare_tasks(job_id,task_id,position,type,status) VALUES(?,?,?,?,?)", jobID, "old-task", 0, "state_execute", job.status)
		niExec(t, db, "INSERT INTO prepare_events(job_id,type,ts,status,message) VALUES(?,?,?,?,?)", jobID, "status", "2026-11-07T00:00:00Z", job.status, "keep event")
	}
}

func TestMainPathInstallationFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db := niOpen(t, path)
	inventory := 0
	format, err := Initialize(t.Context(), db, func(context.Context) error { inventory++; return nil })
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	niAssertTarget(t, db)
	niSeedHistory(t, db, "")
	before := niSnapshotAll(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for reopen := 0; reopen < 2; reopen++ {
		db = niOpen(t, path)
		again, err := Initialize(t.Context(), db, func(context.Context) error { inventory++; return nil })
		if err != nil || again != format || inventory != 1 {
			db.Close()
			t.Fatalf("reopen=%d inventory=%d err=%v", reopen, inventory, err)
		}
		niAssertTarget(t, db)
		niAssertUnchanged(t, db, before)
		db.Close()
	}
}

func TestMainPathInstallationBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db := niBaseline(t, path)
	defer func() { db.Close() }()
	niSeedHistory(t, db, "succeeded")
	before := niSnapshotAll(t, db)
	format, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("inventory during managed bridge"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	niAssertTarget(t, db)
	niAssertOldRows(t, db, before)
	var newPurpose, explained int
	if err := db.QueryRow("SELECT count(*),count(cache_explain_result_json) FROM prepare_jobs WHERE purpose='prepare'").Scan(&newPurpose, &explained); err != nil || newPurpose != 5 || explained != 0 {
		t.Fatalf("invented job data: %d %d %v", newPurpose, explained, err)
	}
	for name, obj := range before.Catalog {
		if name == "prepare_jobs" || name == "managed_job_insert" || name == "managed_job_immutable" {
			continue
		}
		if niCatalog(t, db)[name] != obj {
			t.Fatalf("unrelated definition changed: %s", name)
		}
	}
	after := niSnapshotAll(t, db)
	db.Close()
	for reopen := 0; reopen < 2; reopen++ {
		db = niOpen(t, path)
		again, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("inventory on reopen"); return nil })
		if err != nil || again != format {
			t.Fatalf("reopen %d: %v", reopen, err)
		}
		niAssertUnchanged(t, db, after)
		db.Close()
	}
}

func TestMainPathInstallationRejectsUnfinished(t *testing.T) {
	for _, status := range []string{"queued", "running", "recovery_required"} {
		t.Run(status, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niBaseline(t, path)
			niSeedHistory(t, db, status)
			before := niSnapshotAll(t, db)
			db.Close()
			for reopen := 0; reopen < 3; reopen++ {
				trace := &niSQLTrace{}
				db = openNIAudited(t, path, trace)
				_, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("inventory after unsupported work"); return nil })
				if err == nil {
					db.Close()
					t.Fatal("unfinished old Liquibase admitted")
				}
				if trace.mutations() != 0 {
					db.Close()
					t.Fatalf("preflight attempted %d mutations", trace.mutations())
				}
				niAssertUnchanged(t, db, before)
				db.Close()
			}
		})
	}
}

func TestMainPathInstallationRejectsDamage(t *testing.T) {
	for _, variant := range []string{"table", "trigger", "index", "marker-row", "marker-table", "marker-version", "reserved-table", "reserved-view", "reserved-case", "baseline-job", "half-job", "partial", "prototype", "unmarked-reserved", "unmarked-case"} {
		t.Run(variant, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niOpen(t, path)
			if !strings.HasPrefix(variant, "unmarked-") {
				db.Close()
				db = niBaseline(t, path)
			}
			switch variant {
			case "unmarked-reserved":
				niExec(t, db, "CREATE VIEW managed_runtime_operations AS SELECT 1 AS unsupported")
			case "unmarked-case":
				niExec(t, db, "CREATE TABLE RUNTIME_V2_UNSUPPORTED(value TEXT)")
			case "reserved-table":
				niExec(t, db, "CREATE TABLE prepare_liquibase_capture_slots(fake TEXT)")
			case "reserved-view":
				niExec(t, db, "CREATE VIEW prepare_liquibase_capture_slots AS SELECT 1 AS fake")
			case "reserved-case":
				niExec(t, db, "CREATE TABLE PREPARE_LIQUIBASE_CAPTURE_SLOTS(fake TEXT)")
			case "baseline-job":
				niExec(t, db, "ALTER TABLE prepare_jobs ADD COLUMN unsupported TEXT")
			case "half-job":
				niExec(t, db, "ALTER TABLE prepare_jobs ADD COLUMN purpose TEXT")
			case "partial":
				niExec(t, db, "CREATE TABLE prepare_liquibase_session_store_format(slot INTEGER,format_version INTEGER)")
			case "prototype":
				niExec(t, db, "CREATE TABLE liquibase_store_format(slot INTEGER PRIMARY KEY CHECK(slot=1),format_version INTEGER CHECK(format_version=1)); INSERT INTO liquibase_store_format VALUES(1,1)")
			default:
				if _, err := Initialize(t.Context(), db, func(context.Context) error { return nil }); err != nil {
					t.Fatal("valid damage control", err)
				}
				niAssertTarget(t, db)
				niExec(t, db, "PRAGMA foreign_keys=OFF")
				switch variant {
				case "table":
					niExec(t, db, "DROP TABLE prepare_liquibase_capture_slots")
				case "trigger":
					niExec(t, db, "DROP TRIGGER prepare_liquibase_private_slot_insert")
				case "index":
					niExec(t, db, "DROP INDEX idx_liquibase_job_fingerprint")
				case "marker-row":
					niExec(t, db, "DELETE FROM prepare_liquibase_session_store_format")
				case "marker-table":
					niExec(t, db, "DROP TABLE prepare_liquibase_session_store_format")
				case "marker-version":
					niExec(t, db, "DROP TABLE prepare_liquibase_session_store_format; CREATE TABLE prepare_liquibase_session_store_format(slot INTEGER,format_version INTEGER); INSERT INTO prepare_liquibase_session_store_format VALUES(1,99)")
				}
				niExec(t, db, "PRAGMA foreign_keys=ON")
			}
			before := niSnapshotAll(t, db)
			db.Close()
			for reopen := 0; reopen < 3; reopen++ {
				trace := &niSQLTrace{}
				db = openNIAudited(t, path, trace)
				_, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("inventory on damaged metadata"); return nil })
				if err == nil || trace.mutations() != 0 {
					db.Close()
					t.Fatalf("damaged catalog admitted/mutated: %v mutations=%d", err, trace.mutations())
				}
				niAssertUnchanged(t, db, before)
				db.Close()
			}
		})
	}
}

// NI07 proves enforcement by a real FK violation and a matching valid write.
func niFKProof(t *testing.T, db *sql.DB) {
	t.Helper()
	niAssertConnection(t, db)
	niExec(t, db, "CREATE TABLE installation_fk_probe(value INTEGER REFERENCES managed_store_format(slot))")
	_, err := db.Exec("INSERT INTO installation_fk_probe VALUES(999)")
	var coded interface{ Code() int }
	if !errors.As(err, &coded) || coded.Code() != 787 {
		t.Fatalf("FK enforcement not proved: %v", err)
	}
	niExec(t, db, "INSERT INTO installation_fk_probe VALUES(1)")
	niExec(t, db, "DROP TABLE installation_fk_probe")
}

func TestMainPathInstallationConnectionScope(t *testing.T) {
	for _, baseline := range []bool{false, true} {
		name := "fresh"
		if baseline {
			name = "baseline"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niOpen(t, path)
			if baseline {
				db.Close()
				db = niBaseline(t, path)
			}
			defer db.Close()
			for open := 0; open < 2; open++ {
				if _, err := Initialize(t.Context(), db, func(context.Context) error { return nil }); err != nil {
					t.Fatal(err)
				}
				niFKProof(t, db)
			}
		})
	}
}

func TestMainPathInstallationPsqlUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db := niOpen(t, path)
	defer db.Close()
	if _, err := Initialize(t.Context(), db, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	niAssertTarget(t, db)
	niSeedHistory(t, db, "")
	q, err := queue.New(db)
	if err != nil {
		t.Fatal(err)
	}
	// Ordinary psql changes neither its executor nor its journal authority.
	status := "succeeded"
	if err := q.UpdateTask(t.Context(), "psql-running", "old-task", queue.TaskUpdate{Status: &status}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateJob(t.Context(), "psql-running", queue.JobUpdate{Status: &status}); err != nil {
		t.Fatal(err)
	}
	var saved string
	if err := db.QueryRow("SELECT status FROM prepare_jobs WHERE job_id='psql-running'").Scan(&saved); err != nil || saved != status {
		t.Fatal("psql did not complete", err)
	}
	for _, table := range []string{"prepare_step_attempts", "prepare_liquibase_sessions", "prepare_liquibase_invocations"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("psql was cut over to %s: count=%d err=%v", table, count, err)
		}
	}
	niExec(t, db, "UPDATE states SET size_bytes=124 WHERE state_id=?", strings.Repeat("c", 64))
}
