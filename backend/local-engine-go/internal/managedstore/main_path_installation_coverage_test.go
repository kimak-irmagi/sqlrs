package managedstore

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/store/sqlite"
)

// IC01 exercises the public installer boundary directly; Initialize's own
// preflight is deliberately not allowed to mask a missing verifier refusal.
// Requirements: issue89-normal-path-storage-tests.md, IC01 and NI01/NI05/NI07.
func TestMainPathInstallationDirectBoundaries(t *testing.T) {
	operations := map[string]func(context.Context, *sql.Tx) error{
		"classify": func(ctx context.Context, tx *sql.Tx) error {
			_, err := queue.ClassifyLiquibaseMainPathSchema(ctx, tx)
			return err
		},
		"install": queue.InstallLiquibaseMainPathSchema,
		"verify":  queue.VerifyLiquibaseMainPathSchema,
	}
	for name, operation := range operations {
		for _, boundary := range []string{"nil", "closed", "cancelled"} {
			t.Run(name+"/"+boundary, func(t *testing.T) {
				db := niOpen(t, filepath.Join(t.TempDir(), "state.db"))
				niExec(t, db, "CREATE TABLE settings(value TEXT); INSERT INTO settings VALUES('retained')")
				before := niSnapshotAll(t, db)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var tx *sql.Tx
				if boundary != "nil" {
					var err error
					tx, err = db.BeginTx(t.Context(), nil)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback()
				}
				if boundary == "closed" {
					if err := tx.Rollback(); err != nil {
						t.Fatal(err)
					}
				}
				if boundary == "cancelled" {
					cancel()
				}
				err := operation(ctx, tx)
				if err == nil {
					t.Fatal("invalid transaction admitted")
				}
				if boundary == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatalf("lost cancellation: %v", err)
				}
				if boundary == "closed" && !errors.Is(err, sql.ErrTxDone) {
					t.Fatalf("lost closed transaction: %v", err)
				}
				if boundary == "nil" && !errors.Is(err, queue.ErrLiquibaseMainPathSchema) {
					t.Fatalf("unsafe nil classification: %v", err)
				}
				if tx != nil {
					_ = tx.Rollback()
				}
				niAssertUnchanged(t, db, before)
			})
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if definitions, err := queue.MainPathSchemaDefinitions(ctx); !errors.Is(err, context.Canceled) || definitions != nil {
		t.Fatal("reference loader lost cancellation", err)
	}
}

func TestMainPathInstallationDirectCatalogue(t *testing.T) {
	for _, variant := range []string{"incomplete-base", "released-fk-on", "reserved-without-jobs", "altered", "marker-row", "foreign-key", "current"} {
		t.Run(variant, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db := niOpen(t, path)
			if variant == "released-fk-on" || variant == "reserved-without-jobs" {
				db.Close()
				db = niBaseline(t, path)
				if variant == "reserved-without-jobs" {
					niExec(t, db, "PRAGMA foreign_keys=OFF; DROP TABLE prepare_events; DROP TABLE prepare_tasks; DROP TABLE prepare_jobs; CREATE TABLE prepare_unknown(value TEXT); PRAGMA foreign_keys=ON")
				}
			} else if variant != "incomplete-base" {
				if _, err := Initialize(t.Context(), db, func(context.Context) error { return nil }); err != nil {
					t.Fatal(err)
				}
				niAssertTarget(t, db)
			}
			switch variant {
			case "incomplete-base":
				niExec(t, db, "CREATE TABLE settings(value TEXT); INSERT INTO settings VALUES('retained')")
			case "altered":
				niExec(t, db, "DROP INDEX idx_liquibase_job_fingerprint")
			case "marker-row":
				niExec(t, db, "DELETE FROM prepare_liquibase_session_store_format")
			case "foreign-key":
				niExec(t, db, "PRAGMA foreign_keys=OFF; CREATE TABLE installation_bad_fk(value INTEGER REFERENCES managed_store_format(slot)); INSERT INTO installation_bad_fk VALUES(999); PRAGMA foreign_keys=ON")
			}
			before := niSnapshotAll(t, db)
			db.Close()
			for _, operation := range []string{"install", "verify"} {
				trace := &niSQLTrace{}
				db = openNIAudited(t, path, trace)
				tx, err := db.BeginTx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				if operation == "install" {
					err = queue.InstallLiquibaseMainPathSchema(t.Context(), tx)
				} else {
					err = queue.VerifyLiquibaseMainPathSchema(t.Context(), tx)
				}
				if variant == "current" {
					if err != nil {
						t.Fatalf("exact target rejected: %v", err)
					}
				} else if !errors.Is(err, queue.ErrLiquibaseMainPathSchema) {
					t.Fatalf("bad target not safely refused: %v", err)
				}
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				if trace.mutations() != 0 {
					t.Fatalf("verification/failed direct installation attempted %d writes", trace.mutations())
				}
				niAssertUnchanged(t, db, before)
				db.Close()
			}
		})
	}
}

// IC02/IC03 use one observed refusal per case. All earlier SQL is real and
// persisted-state observations occur before caller cleanup can conceal a leak.
func TestMainPathInstallationObservedRefusals(t *testing.T) {
	cases := []struct {
		name                         string
		fault                        niOrderedFault
		current, committed, safeOnly bool
	}{
		{name: "unfinished-read", fault: niOrderedFault{kind: "query", needle: "FROM prepare_jobs WHERE prepare_kind", when: "before"}},
		{name: "suspend-trigger", fault: niOrderedFault{kind: "exec", needle: "DROP TRIGGER ", when: "before"}},
		{name: "create-temporary", fault: niOrderedFault{kind: "exec", needle: "CREATE TABLE prepare_jobs__main (", when: "before"}},
		{name: "old-count", fault: niOrderedFault{kind: "query", needle: "SELECT count(*) FROM prepare_jobs", exact: true, when: "before"}},
		{name: "copied-count", fault: niOrderedFault{kind: "query", needle: "SELECT count(*) FROM prepare_jobs__main", exact: true, when: "before"}},
		{name: "copy-difference", fault: niOrderedFault{kind: "query", needle: " FROM prepare_jobs EXCEPT SELECT ", when: "before"}},
		{name: "create-replacement", fault: niOrderedFault{kind: "exec", needle: "CREATE TABLE prepare_jobs (", when: "before"}},
		{name: "fill-replacement", fault: niOrderedFault{kind: "exec", needle: "INSERT INTO prepare_jobs(job_id,", when: "before"}},
		{name: "drop-temporary", fault: niOrderedFault{kind: "exec", needle: "DROP TABLE prepare_jobs__main", exact: true, when: "before"}},
		{name: "restore-trigger", fault: niOrderedFault{kind: "exec", needle: "CREATE TRIGGER managed_job_insert ", when: "before"}},
		{name: "write-marker", fault: niOrderedFault{kind: "exec", needle: "INSERT INTO liquibase_store_format", when: "before"}},
		{name: "read-marker", fault: niOrderedFault{kind: "query", needle: " FROM prepare_liquibase_session_store_format", when: "before", requireWrite: true}},
		{name: "foreign-key-query", fault: niOrderedFault{kind: "query", needle: "PRAGMA foreign_key_check", when: "before", requireWrite: true}},
		{name: "foreign-key-stream", fault: niOrderedFault{kind: "rows", needle: "PRAGMA foreign_key_check", when: "before", requireWrite: true}},
		{name: "initial-pragma", fault: niOrderedFault{kind: "exec", needle: "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=0", when: "before"}, safeOnly: true},
		{name: "preflight-rollback", fault: niOrderedFault{kind: "rollback", when: "before"}},
		{name: "bridge-begin", fault: niOrderedFault{kind: "begin", when: "before", minBegins: 2}},
		{name: "repeated-preflight", fault: niOrderedFault{kind: "query", needle: "SELECT name,type,tbl_name,sql FROM sqlite_master", when: "before", minBegins: 2}},
		{name: "current-runtime-marker", fault: niOrderedFault{kind: "query", needle: "SELECT count(*) FROM runtime_v2_store_format WHERE", when: "before"}, current: true, safeOnly: true},
		{name: "restore-read", fault: niOrderedFault{kind: "query", needle: "PRAGMA foreign_keys", exact: true, when: "before", requireCommit: true}, committed: true},
		{name: "readback-begin", fault: niOrderedFault{kind: "begin", when: "before", minBegins: 3}, committed: true},
		{name: "readback-domain", fault: niOrderedFault{kind: "query", needle: "FROM main.managed_store_format", when: "before", requireCommit: true}, committed: true, safeOnly: true},
		{name: "readback-rollback", fault: niOrderedFault{kind: "rollback", when: "after", requireCommit: true}, committed: true},
		{name: "current-rollback", fault: niOrderedFault{kind: "rollback", when: "before"}, current: true},
		{name: "catalogue-stream", fault: niOrderedFault{kind: "rows", needle: "SELECT name,coalesce(sql,'') FROM sqlite_master", when: "before", skip: 1, requireWrite: true}, safeOnly: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			base := niBaseline(t, path)
			niSeedHistory(t, base, "succeeded")
			if tc.current {
				if _, err := Initialize(t.Context(), base, func(context.Context) error { t.Fatal("managed inventory"); return nil }); err != nil {
					t.Fatal(err)
				}
			}
			before := niSnapshotAll(t, base)
			base.Close()
			trace := &niSQLTrace{faults: []niOrderedFault{tc.fault}}
			db := openNIAudited(t, path, trace)
			format, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("inventory during observed refusal"); return nil })
			if err == nil || format.Version != "" || trace.hits() != 1 {
				t.Fatalf("intended refusal not reached: format=%+v err=%v hits=%d", format, err, trace.hits())
			}
			if !errors.Is(err, sqlite.ErrManagedPreflightUnavailable) {
				t.Fatalf("unsafe failure classification: %v", err)
			}
			if !tc.safeOnly && !errors.Is(err, errNIInjected) {
				t.Fatalf("primary refusal lost: %v", err)
			}
			if tc.name == "initial-pragma" || tc.name == "restore-read" {
				trace.mu.Lock()
				closed := append([]int(nil), trace.closed...)
				trace.mu.Unlock()
				if len(closed) != 1 || closed[0] != 1 {
					t.Fatalf("unproved startup connection was not excluded: %v", closed)
				}
			}
			niAssertConnection(t, db)
			peer := niOpen(t, path)
			if tc.committed {
				niAssertTarget(t, db)
				niAssertTarget(t, peer)
				niAssertOldRows(t, peer, before)
			} else {
				niAssertUnchanged(t, db, before)
				niAssertUnchanged(t, peer, before)
			}
			if tc.current || tc.name == "unfinished-read" || tc.name == "initial-pragma" || tc.name == "preflight-rollback" || tc.name == "bridge-begin" || tc.name == "repeated-preflight" {
				if trace.mutations() != 0 {
					t.Fatal("read-only preflight/current no-op attempted mutation")
				}
			}
			peer.Close()
			db.Close()
			for reopen := 0; reopen < 2; reopen++ {
				clean := niOpen(t, path)
				if _, err := Initialize(t.Context(), clean, func(context.Context) error { t.Fatal("retry inventory"); return nil }); err != nil {
					t.Fatal("clean retry", err)
				}
				niAssertTarget(t, clean)
				niAssertOldRows(t, clean, before)
				clean.Close()
			}
		})
	}
}

// IC04 retains the primary error while a separate physical cleanup proof
// succeeds or fails. Close diagnostics and late callbacks remain outside #89.
func TestMainPathInstallationCleanupProof(t *testing.T) {
	workErr := errors.New("IC work refused")
	rollbackErr := errors.New("IC rollback reply refused")
	cleanupErr := errors.New("IC cleanup refused")
	for _, variant := range []string{"proved-idle", "active-after-rollback", "proof-rollback", "commit-active", "committed-proof-failure", "deferred-restore"} {
		t.Run(variant, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			base := niBaseline(t, path)
			niSeedHistory(t, base, "succeeded")
			before := niSnapshotAll(t, base)
			base.Close()
			trace := &niSQLTrace{}
			work := niOrderedFault{kind: "exec", needle: "INSERT INTO prepare_jobs__main(", when: "after", err: workErr}
			rawRollback := niOrderedFault{kind: "exec", needle: "ROLLBACK", exact: true, when: "before", err: cleanupErr}
			switch variant {
			case "proved-idle":
				trace.faults = []niOrderedFault{work, {kind: "rollback", when: "after", err: rollbackErr}}
			case "active-after-rollback":
				trace.faults = []niOrderedFault{work, {kind: "rollback", when: "before", err: rollbackErr}, rawRollback}
			case "proof-rollback":
				rawRollback.skip = 1
				trace.faults = []niOrderedFault{work, rawRollback}
			case "commit-active":
				trace.faults = []niOrderedFault{{kind: "commit", when: "before", err: workErr}, rawRollback}
			case "committed-proof-failure":
				trace.lostCommitReply = true
				rawRollback.skip = 1
				trace.faults = []niOrderedFault{rawRollback}
			case "deferred-restore":
				trace.faults = []niOrderedFault{work, {kind: "exec", needle: "PRAGMA foreign_keys=ON", exact: true, when: "before", err: cleanupErr}}
			}
			db := openNIAudited(t, path, trace)
			format, err := Initialize(t.Context(), db, func(context.Context) error { t.Fatal("managed inventory"); return nil })
			if err == nil || format.Version != "" {
				t.Fatalf("unsafe cleanup published readiness: %+v %v", format, err)
			}
			wantPrimary := workErr
			if variant == "committed-proof-failure" {
				wantPrimary = errNIInjected
			}
			if !errors.Is(err, wantPrimary) {
				t.Fatalf("lost primary failure: %v", err)
			}
			wantCleanup := cleanupErr
			if variant == "proved-idle" {
				wantCleanup = rollbackErr
			}
			if !errors.Is(err, wantCleanup) {
				t.Fatalf("cleanup failure lost its independent identity: %v", err)
			}
			if variant == "active-after-rollback" && !errors.Is(err, rollbackErr) {
				t.Fatalf("lost the intermediate rollback failure: %v", err)
			}
			trace.mu.Lock()
			hits, next, closed, committed := trace.hitCount, trace.nextFault, append([]int(nil), trace.closed...), trace.committed
			trace.mu.Unlock()
			wantHits := len(trace.faults)
			if trace.lostCommitReply {
				wantHits++
			}
			if next != len(trace.faults) || hits != wantHits {
				t.Fatalf("ordered cleanup not reached: next=%d hits=%d want=%d err=%v", next, hits, wantHits, err)
			}
			if variant == "proved-idle" {
				if len(closed) != 0 {
					t.Fatal("proved idle connection was unnecessarily excluded", closed)
				}
			} else if len(closed) != 1 || closed[0] != 1 {
				t.Fatal("exact unproved connection not excluded", closed)
			}
			if committed != (variant == "committed-proof-failure") {
				t.Fatal("commit outcome confused with cleanup outcome")
			}
			peer := niOpen(t, path)
			if committed {
				niAssertTarget(t, peer)
				niAssertOldRows(t, peer, before)
			} else {
				niAssertUnchanged(t, peer, before)
				niAssertUnchanged(t, db, before)
			}
			niAssertConnection(t, db)
			trace.mu.Lock()
			connections := trace.nextID
			trace.mu.Unlock()
			if variant == "proved-idle" && connections != 1 || variant != "proved-idle" && connections != 2 {
				t.Fatalf("unexpected physical reuse/replacement: %d", connections)
			}
			peer.Close()
			db.Close()
			for reopen := 0; reopen < 2; reopen++ {
				clean := niOpen(t, path)
				if _, err := Initialize(t.Context(), clean, func(context.Context) error { t.Fatal("retry inventory"); return nil }); err != nil {
					t.Fatal(err)
				}
				niAssertTarget(t, clean)
				niAssertOldRows(t, clean, before)
				clean.Close()
			}
		})
	}
}
