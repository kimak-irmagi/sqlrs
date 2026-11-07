package managedstore

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// NI06 faults the real SQLite effects, not only their reply values. A second
// connection and same-handle checks precede cleanup so Close cannot hide a leak.
func TestMainPathInstallationAtomicity(t *testing.T) {
	for _, variant := range []string{"begin", "created-table", "copied-jobs", "dropped-jobs", "verification", "cancelled-before", "cancelled-after-write", "commit-busy", "committed-lost-reply", "committed-readback-unavailable"} {
		t.Run(variant, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			baseline := variant == "copied-jobs" || variant == "dropped-jobs"
			seed := niOpen(t, path)
			if baseline {
				seed.Close()
				seed = niBaseline(t, path)
				niSeedHistory(t, seed, "succeeded")
			} else {
				niExec(t, seed, "CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT); INSERT INTO settings VALUES('keep','unchanged')")
			}
			before := niSnapshotAll(t, seed)
			seed.Close()
			trace := &niSQLTrace{}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch variant {
			case "begin":
				trace.faultKind, trace.when = "begin", "before"
			case "created-table":
				trace.faultKind, trace.needle, trace.when = "exec", "CREATE TABLE liquibase_execution_bindings", "after"
			case "copied-jobs":
				trace.faultKind, trace.needle, trace.when = "exec", "INSERT INTO prepare_jobs__main", "after"
			case "dropped-jobs":
				trace.faultKind, trace.needle, trace.when = "exec", "DROP TABLE prepare_jobs", "after"
			case "verification":
				trace.faultKind, trace.needle, trace.when, trace.requireWrite = "query", "PRAGMA foreign_key_check", "before", true
			case "cancelled-before":
				cancel()
			case "cancelled-after-write":
				trace.cancelAfterWrite = cancel
			case "committed-lost-reply":
				trace.lostCommitReply = true
			case "committed-readback-unavailable":
				trace.lostCommitReply = true
				trace.faultKind, trace.needle, trace.when, trace.requireCommit = "query", "sqlite_master", "before", true
			}
			// Hold a real read transaction over pre-existing data before the writer.
			// A rollback-journal writer may mutate, but its physical COMMIT must be BUSY.
			var releaseReader func()
			if variant == "commit-busy" {
				peer := niOpen(t, path)
				niExec(t, peer, "PRAGMA journal_mode=DELETE")
				reader, err := peer.BeginTx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				var value string
				if err := reader.QueryRow("SELECT value FROM settings WHERE key='keep'").Scan(&value); err != nil {
					t.Fatal(err)
				}
				releaseReader = func() { reader.Rollback(); peer.Close() }
				defer func() {
					if releaseReader != nil {
						releaseReader()
					}
				}()
			}
			db := openNIAudited(t, path, trace)
			format, runErr := Initialize(ctx, db, func(context.Context) error { return nil })
			if releaseReader != nil {
				releaseReader()
				releaseReader = nil
			}
			committed := variant == "committed-lost-reply" || variant == "committed-readback-unavailable"
			if !committed {
				if runErr == nil {
					t.Fatal("failed installation reported success")
				}
				if variant == "cancelled-before" || variant == "cancelled-after-write" {
					if !errors.Is(runErr, context.Canceled) {
						t.Fatal("lost cancellation", runErr)
					}
				}
				if variant == "commit-busy" {
					trace.mu.Lock()
					codes := append([]int(nil), trace.commitCodes...)
					trace.mu.Unlock()
					if !reflect.DeepEqual(codes, []int{5}) || trace.mutations() == 0 {
						t.Fatalf("real COMMIT BUSY was not reached: %v writes=%d", codes, trace.mutations())
					}
				} else if variant != "cancelled-before" && trace.hits() != 1 {
					t.Fatalf("intended refusal not reached: hits=%d err=%v", trace.hits(), runErr)
				}
				// Check the same handle before closing it: pending uncommitted DDL is not
				// an acceptable rollback, even if a later physical Close would erase it.
				niAssertUnchanged(t, db, before)
				niAssertConnection(t, db)
				peer := niOpen(t, path)
				niAssertUnchanged(t, peer, before)
				peer.Close()
			} else {
				trace.mu.Lock()
				codes := append([]int(nil), trace.commitCodes...)
				trace.mu.Unlock()
				if !reflect.DeepEqual(codes, []int{0}) {
					t.Fatalf("lost-reply case did not really commit once: %v", codes)
				}
				if variant == "committed-readback-unavailable" {
					if runErr == nil || trace.hits() != 2 {
						t.Fatalf("unavailable postcommit proof not reached: err=%v hits=%d", runErr, trace.hits())
					}
				} else if trace.hits() != 1 {
					t.Fatal("lost reply was not observed")
				}
				if runErr != nil && format.Version != "" {
					t.Fatal("uncertain installation returned a ready format")
				}
				peer := niOpen(t, path)
				niAssertTarget(t, peer)
				niAssertOldRows(t, peer, before)
				peer.Close()
			}
			// Remove the observer, not the database or its data, for two honest opens.
			db.Close()
			for open := 0; open < 2; open++ {
				clean := niOpen(t, path)
				if _, err := Initialize(t.Context(), clean, func(context.Context) error { return nil }); err != nil {
					clean.Close()
					t.Fatalf("retry %d after boundary: %v", open, err)
				}
				niAssertTarget(t, clean)
				niAssertOldRows(t, clean, before)
				clean.Close()
			}
		})
	}
}

// NI07: setup/restore failures cannot release a ready store. The real wrapper
// configures replacements so exclusion can be checked without changing product.
func TestMainPathInstallationConnectionFailure(t *testing.T) {
	for _, phase := range []string{"setup", "setup-after-effect", "restore", "restore-after-effect"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			base := niBaseline(t, path)
			before := niSnapshotAll(t, base)
			base.Close()
			trace := &niSQLTrace{faultKind: "exec", when: "before"}
			if phase == "setup" || phase == "setup-after-effect" {
				trace.needle = "PRAGMA foreign_keys=OFF"
			} else {
				trace.needle = "PRAGMA foreign_keys=ON"
				trace.requireCommit = true
			}
			if phase == "setup-after-effect" || phase == "restore-after-effect" {
				trace.when = "after"
			}
			db := openNIAudited(t, path, trace)
			format, err := Initialize(t.Context(), db, func(context.Context) error { return nil })
			if err == nil || format.Version != "" || trace.hits() != 1 {
				t.Fatalf("unsafe startup escaped %s: format=%+v err=%v hits=%d", phase, format, err, trace.hits())
			}
			if phase == "setup" || phase == "setup-after-effect" {
				niAssertUnchanged(t, db, before)
			}
			if phase == "restore" || phase == "restore-after-effect" {
				trace.mu.Lock()
				closed := len(trace.closed)
				trace.mu.Unlock()
				if closed == 0 {
					t.Fatal("unclean physical connection was returned to the pool")
				}
			}
			niFKProof(t, db)
			db.Close()
		})
	}
}
