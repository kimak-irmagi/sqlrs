package managedstore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/sqlrs/engine-local/internal/store/sqlite"
)

func TestCoordinatedCutoverAndReopen(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	calls := 0
	inventory := func(context.Context) error { calls++; return nil }
	format, err := Initialize(ctx, db, inventory)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Initialize(ctx, db, inventory)
	if err != nil || again != format || calls != 1 {
		t.Fatal("restart changed identity or checked empty legacy inventory", err)
	}
	if _, err := db.Exec("DROP TRIGGER instance_access_lifecycle"); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(ctx, db, inventory); err == nil {
		t.Fatal("repaired incomplete schema on reopen")
	}
	var count int
	db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='instance_access_lifecycle'").Scan(&count)
	if count != 0 {
		t.Fatal("startup mutated damaged schema")
	}
}

// Vary cancellation at database/sql context observation boundaries. The real
// SQLite driver still performs every statement and rollback; no SQL errors or
// impossible catalog values are fabricated to reach defensive branches.
type cancellationBoundary struct {
	context.Context
	cancel    context.CancelFunc
	remaining atomic.Int32
}

func (c *cancellationBoundary) Done() <-chan struct{} {
	if c.remaining.Add(-1) == 0 {
		c.cancel()
	}
	return c.Context.Done()
}

func TestCutoverCancellationNeverCommitsPartialSchema(t *testing.T) {
	for boundary := int32(1); boundary <= 512; boundary++ {
		path := filepath.Join(t.TempDir(), "store.db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("CREATE TABLE settings(value TEXT); INSERT INTO settings VALUES('keep')"); err != nil {
			db.Close()
			t.Fatal(err)
		}
		base, cancel := context.WithCancel(context.Background())
		ctx := &cancellationBoundary{Context: base, cancel: cancel}
		ctx.remaining.Store(boundary)
		_, runErr := Initialize(ctx, db, func(context.Context) error { return nil })
		cancel()
		var settings string
		if err := db.QueryRow("SELECT value FROM settings").Scan(&settings); err != nil || settings != "keep" {
			db.Close()
			t.Fatalf("boundary %d lost settings: %v", boundary, err)
		}
		var marker int
		if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='managed_store_format'").Scan(&marker); err != nil {
			db.Close()
			t.Fatal(err)
		}
		if runErr != nil && marker != 0 {
			db.Close()
			t.Fatalf("boundary %d committed partial schema: %v", boundary, runErr)
		}
		if runErr == nil {
			if _, err := Initialize(context.Background(), db, func(context.Context) error { return nil }); err != nil {
				db.Close()
				t.Fatal("successful cutover cannot reopen", err)
			}
		}
		db.Close()
	}
}

func TestCutoverFailureLeavesOriginalStore(t *testing.T) {
	for _, scenario := range []string{"inventory", "read-only", "schema-conflict", "cancelled", "nil", "closed"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "store.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec("CREATE TABLE settings(value TEXT); INSERT INTO settings VALUES('preserve')"); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			inventory := func(context.Context) error { return nil }
			switch scenario {
			case "inventory":
				inventory = func(context.Context) error { return sqlite.ErrManagedPreflightUnavailable }
			case "read-only":
				_, err = db.Exec("PRAGMA query_only=ON")
			case "schema-conflict":
				_, err = db.Exec("CREATE VIEW managed_runtime_operations AS SELECT value FROM settings")
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "closed":
				err = db.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			input := db
			if scenario == "nil" {
				input = nil
			}
			if _, err := Initialize(ctx, input, inventory); err == nil {
				t.Fatal("unsafe store admitted")
			} else if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			if scenario == "closed" {
				return
			}
			var value string
			var markers int
			if err := db.QueryRow("SELECT value FROM settings").Scan(&value); err != nil || value != "preserve" {
				t.Fatal("settings changed", err)
			}
			if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='managed_store_format'").Scan(&markers); err != nil || markers != 0 {
				t.Fatal("partial format committed", err)
			}
		})
	}
}

func TestLockRefusesInvalidTargets(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"relative.lock", root, filepath.Join(root, "missing", "lock")} {
		if f, err := Lock(path); err == nil {
			f.Close()
			t.Fatal("invalid lock admitted", path)
		}
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if f, err := Lock(filepath.Join(file, "lock")); err == nil {
		f.Close()
		t.Fatal("non-directory parent admitted")
	}
}

func TestSchemaReadPreservesCancellationAndClosedTransaction(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	if _, err := schema(context.Background(), tx); err == nil {
		t.Fatal("closed transaction read")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := expectedSchema(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("reference cancellation", err)
	}
}

func TestCoordinatedCutoverRefusesLegacyWithoutMutation(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE states(id TEXT); INSERT INTO states VALUES('legacy'); CREATE TABLE settings(value TEXT); INSERT INTO settings VALUES('keep')"); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(ctx, db, func(context.Context) error { t.Fatal("inventory after metadata refusal"); return nil }); err != sqlite.ErrLegacyManagedData {
		t.Fatal(err)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='managed_store_format'").Scan(&n)
	if n != 0 {
		t.Fatal("legacy marker mutated")
	}
	var value string
	db.QueryRow("SELECT value FROM settings").Scan(&value)
	if value != "keep" {
		t.Fatal("unrelated data changed")
	}
}

func TestExclusiveStoreOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.lock")
	first, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Lock(path); err == nil {
		second.Close()
		t.Fatal("second engine admitted")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Lock(path)
	if err != nil {
		t.Fatal("crash-safe OS lock did not release", err)
	}
	second.Close()
}
