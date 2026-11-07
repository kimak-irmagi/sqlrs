package queue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// These artifacts are mechanically frozen from the approved composed catalog.
// They contain final DDL, not executable historical migrations. See
// docs/architecture/issue89-normal-path-storage-extraction.md.
//
//go:embed assets/liquibase-main-schema.sql
var mainPathDDL string

//go:embed assets/liquibase-main-catalog.json
var mainPathCatalogJSON []byte

// ErrLiquibaseMainPathSchema refuses unsupported, incomplete or altered metadata;
// it never authorizes implicit repair or recovery of an old unfinished job.
var ErrLiquibaseMainPathSchema = errors.New("Liquibase main-path schema unavailable")

type mainPathObject struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Table          string `json:"table"`
	SHA256         string `json:"sha256"`
	Offset         int    `json:"offset"`
	Length         int    `json:"length"`
	ReleasedSHA256 string `json:"released_sha256"`
}
type mainPathMarker struct {
	Table string `json:"table"`
	SQL   string `json:"sql"`
}
type mainPathArtifact struct {
	Objects []mainPathObject `json:"objects"`
	Markers []mainPathMarker `json:"markers"`
}
type mainPathActual struct{ kind, table, definition, digest string }

func mainPathDigest(s string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))))
	return hex.EncodeToString(sum[:])
}
func loadMainPathArtifact(ctx context.Context) (mainPathArtifact, error) {
	if err := ctx.Err(); err != nil {
		return mainPathArtifact{}, err
	}
	var artifact mainPathArtifact
	if err := json.Unmarshal(mainPathCatalogJSON, &artifact); err != nil {
		return artifact, ErrLiquibaseMainPathSchema
	}
	return artifact, nil
}
func mainPathStatement(object mainPathObject) string {
	return mainPathDDL[object.Offset : object.Offset+object.Length]
}
func readMainPathCatalog(ctx context.Context, tx *sql.Tx) (map[string]mainPathActual, error) {
	rows, err := tx.QueryContext(ctx, "SELECT name,type,tbl_name,sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]mainPathActual{}
	for rows.Next() {
		var name string
		var o mainPathActual
		if err := rows.Scan(&name, &o.kind, &o.table, &o.definition); err != nil {
			return nil, err
		}
		o.digest = mainPathDigest(o.definition)
		result[name] = o
	}
	return result, rows.Err()
}
func mainPathReserved(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "prepare_") || strings.HasPrefix(n, "liquibase_") ||
		strings.HasPrefix(n, "managed_") || strings.HasPrefix(n, "runtime_v2_") ||
		strings.HasPrefix(n, "instance_access") || n == "bootstrap_access"
}
func mainPathMatches(actual map[string]mainPathActual, artifact mainPathArtifact, released bool) bool {
	expected := map[string]mainPathObject{}
	for _, o := range artifact.Objects {
		if released && o.ReleasedSHA256 == "" {
			continue
		}
		expected[o.Name] = o
		hash := o.SHA256
		if released {
			hash = o.ReleasedSHA256
		}
		got, found := actual[o.Name]
		if !found || got.kind != o.Type || got.table != o.Table || got.digest != hash {
			return false
		}
	}
	for name := range actual {
		if _, ok := expected[name]; !ok && mainPathReserved(name) {
			return false
		}
	}
	return true
}

// ClassifyLiquibaseMainPathSchema is read-only and must precede all startup
// DDL/inventory. It accepts only an unmarked initial store, the exact released
// managed baseline, or the exact current target. No partial repair is allowed.
func ClassifyLiquibaseMainPathSchema(ctx context.Context, tx *sql.Tx) (string, error) {
	if tx == nil {
		return "", ErrLiquibaseMainPathSchema
	}
	artifact, err := loadMainPathArtifact(ctx)
	if err != nil {
		return "", err
	}
	actual, err := readMainPathCatalog(ctx, tx)
	if err != nil {
		return "", err
	}
	if _, marked := actual["managed_store_format"]; !marked {
		for name := range actual {
			// An absent top-level marker is not permission to inventory or repair
			// another partial owned namespace, including case-varied names.
			if mainPathReserved(name) {
				return "", ErrLiquibaseMainPathSchema
			}
		}
		return "fresh", nil
	}
	if mainPathMatches(actual, artifact, false) {
		return "current", nil
	}
	if !mainPathMatches(actual, artifact, true) {
		return "", ErrLiquibaseMainPathSchema
	}
	var unfinished int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM prepare_jobs WHERE prepare_kind='lb' AND status IN ('queued','running','recovery_required')").Scan(&unfinished); err != nil {
		return "", err
	}
	if unfinished != 0 {
		return "", ErrLiquibaseMainPathSchema
	}
	return "released", nil
}

// MainPathSchemaDefinitions returns caller-owned immutable reference values;
// it never caches validation of a real database or opens a reference SQLite.
func MainPathSchemaDefinitions(ctx context.Context) (map[string]string, error) {
	artifact, err := loadMainPathArtifact(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(artifact.Objects))
	for _, o := range artifact.Objects {
		result[o.Name] = strings.Join(strings.Fields(strings.TrimSuffix(mainPathStatement(o), ";")), " ")
	}
	return result, nil
}

// InstallLiquibaseMainPathSchema runs in the caller's exclusive startup
// transaction. Fresh base prerequisites are installed by their existing owners.
// Only the known released job table is converted; prototype upgrades are absent.
func InstallLiquibaseMainPathSchema(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return ErrLiquibaseMainPathSchema
	}
	artifact, err := loadMainPathArtifact(ctx)
	if err != nil {
		return err
	}
	actual, err := readMainPathCatalog(ctx, tx)
	if err != nil {
		return err
	}
	if mainPathMatches(actual, artifact, false) {
		return VerifyLiquibaseMainPathSchema(ctx, tx)
	}
	if _, hasJobs := actual["prepare_jobs"]; hasJobs {
		mode, err := ClassifyLiquibaseMainPathSchema(ctx, tx)
		if err != nil || mode != "released" {
			return errors.Join(ErrLiquibaseMainPathSchema, err)
		}
		if err := bridgeMainPathJobs(ctx, tx, actual, artifact); err != nil {
			return err
		}
	} else {
		// A fresh coordinated install has all old base objects except those owned
		// by queue. It is not permission to fill an arbitrary partial namespace.
		for _, o := range artifact.Objects {
			queueOwned := o.Table == "prepare_jobs" || o.Table == "prepare_tasks" || o.Table == "prepare_events"
			if o.ReleasedSHA256 == "" || queueOwned {
				continue
			}
			got, found := actual[o.Name]
			if !found || got.digest != o.ReleasedSHA256 {
				return ErrLiquibaseMainPathSchema
			}
		}
		for name := range actual {
			if strings.HasPrefix(strings.ToLower(name), "prepare_") || strings.HasPrefix(strings.ToLower(name), "liquibase_") {
				return ErrLiquibaseMainPathSchema
			}
		}
	}
	actual, err = readMainPathCatalog(ctx, tx)
	if err != nil {
		return err
	}
	for _, o := range artifact.Objects {
		if got, found := actual[o.Name]; found {
			if got.digest != o.SHA256 || got.kind != o.Type || got.table != o.Table {
				return ErrLiquibaseMainPathSchema
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, mainPathStatement(o)); err != nil {
			return err
		}
	}
	for _, marker := range artifact.Markers {
		if _, err := tx.ExecContext(ctx, marker.SQL); err != nil {
			return err
		}
	}
	return VerifyLiquibaseMainPathSchema(ctx, tx)
}

func quoteMainPathName(name string) string {
	return "\"" + strings.ReplaceAll(name, "\"", "\"\"") + "\""
}

// bridgeMainPathJobs preserves all released columns and temporarily suspends
// triggers only within the same transaction. Two changed admission guards use
// their approved final definitions; every unrelated trigger is restored exactly.
func bridgeMainPathJobs(ctx context.Context, tx *sql.Tx, actual map[string]mainPathActual, artifact mainPathArtifact) error {
	var fk int
	if err := tx.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 0 {
		return ErrLiquibaseMainPathSchema
	}
	target := map[string]mainPathObject{}
	for _, o := range artifact.Objects {
		target[o.Name] = o
	}
	names := make([]string, 0, len(actual))
	for name := range actual {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if actual[name].kind == "trigger" {
			if _, err := tx.ExecContext(ctx, "DROP TRIGGER "+quoteMainPathName(name)); err != nil {
				return err
			}
		}
	}
	statement := mainPathStatement(target["prepare_jobs"])
	temporary := strings.Replace(statement, "CREATE TABLE prepare_jobs (", "CREATE TABLE prepare_jobs__main (", 1)
	if _, err := tx.ExecContext(ctx, temporary); err != nil {
		return err
	}
	const oldColumns = "job_id,status,prepare_kind,image_id,plan_only,snapshot_mode,prepare_args_normalized,signature,request_json,created_at,started_at,finished_at,result_json,error_json,resolved_image_id,lineage_ref,identity_digest"
	if _, err := tx.ExecContext(ctx, "INSERT INTO prepare_jobs__main("+oldColumns+") SELECT "+oldColumns+" FROM prepare_jobs"); err != nil {
		return err
	}
	var oldCount, newCount, difference int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM prepare_jobs").Scan(&oldCount); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM prepare_jobs__main").Scan(&newCount); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM (SELECT "+oldColumns+" FROM prepare_jobs EXCEPT SELECT "+oldColumns+" FROM prepare_jobs__main)").Scan(&difference); err != nil {
		return err
	}
	if oldCount != newCount || difference != 0 {
		return ErrLiquibaseMainPathSchema
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE prepare_jobs"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, statement); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO prepare_jobs("+oldColumns+") SELECT "+oldColumns+" FROM prepare_jobs__main"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE prepare_jobs__main"); err != nil {
		return err
	}
	for _, name := range names {
		previous := actual[name]
		if previous.kind != "trigger" && !(previous.kind == "index" && previous.table == "prepare_jobs") {
			continue
		}
		definition := previous.definition
		if name == "managed_job_insert" || name == "managed_job_immutable" {
			definition = mainPathStatement(target[name])
		}
		if _, err := tx.ExecContext(ctx, definition); err != nil {
			return err
		}
	}
	return nil
}

// VerifyLiquibaseMainPathSchema never installs or repairs. It rechecks the
// actual catalog, singleton component versions and actual FK relationships.
func VerifyLiquibaseMainPathSchema(ctx context.Context, tx *sql.Tx) error {
	artifact, err := loadMainPathArtifact(ctx)
	if err != nil {
		return err
	}
	actual, err := readMainPathCatalog(ctx, tx)
	if err != nil {
		return err
	}
	if !mainPathMatches(actual, artifact, false) {
		return ErrLiquibaseMainPathSchema
	}
	for table, want := range map[string]int{
		"liquibase_store_format": 2, "prepare_attempt_store_format": 3,
		"prepare_liquibase_session_store_format":         2,
		"prepare_liquibase_finalization_store_format":    2,
		"prepare_liquibase_cache_selection_store_format": 1,
	} {
		var count, version, slots int
		if err := tx.QueryRowContext(ctx, "SELECT count(*),coalesce(max(format_version),0),coalesce(sum(CASE WHEN slot=1 THEN 1 ELSE 0 END),0) FROM "+table).Scan(&count, &version, &slots); err != nil {
			return err
		}
		if count != 1 || version != want || slots != 1 {
			return ErrLiquibaseMainPathSchema
		}
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return ErrLiquibaseMainPathSchema
	}
	return rows.Err()
}
