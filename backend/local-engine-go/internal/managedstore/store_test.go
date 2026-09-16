package managedstore

import (
	"context"
	"database/sql"
	"path/filepath"
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
