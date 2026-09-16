package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedstore"

	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store/sqlite"
)

// Unit startup tests use an empty physical inventory. Real Docker inventory has
// its own adapter tests and the explicitly selected managedintegration suite.
func TestMain(m *testing.M) {
	if runtime.GOOS == "darwin" {
		root, err := filepath.EvalSymlinks(os.TempDir())
		if err != nil {
			panic(err)
		}
		if err := os.Setenv("TMPDIR", root); err != nil {
			panic(err)
		}
	}
	checkManagedInventoryFn = func(context.Context, *engineRuntime.DockerRuntime, string) error { return nil }
	os.Exit(m.Run())
}

func TestManagedStartupRefusesUnsafeOwnershipAndStorage(t *testing.T) {
	for _, scenario := range []string{"store-owned", "database-owned", "secret-file", "database-parent"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "state.db")
			t.Setenv("SQLRS_STATE_STORE", root)
			t.Setenv("SQLRS_STATE_DB", path)
			switch scenario {
			case "store-owned", "database-owned":
				lockPath := filepath.Join(root, "engine.lock")
				if scenario == "database-owned" {
					lockPath = path + ".engine-lock"
				}
				lock, err := managedstore.Lock(lockPath)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
			case "secret-file":
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				_, err = managedstore.Initialize(context.Background(), db, func(context.Context) error { return nil })
				db.Close()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "managed-secrets"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "database-parent":
				if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("SQLRS_STATE_DB", filepath.Join(path, "child.db"))
			}
			stateFile := filepath.Join(root, "engine.json")
			code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + stateFile})
			if code != 1 || err == nil {
				t.Fatal("unsafe startup admitted", code, err)
			}
			if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
				t.Fatal("failed startup advertised endpoint", err)
			}
		})
	}
}

func TestRunManagedLegacyRefusalPrecedesStoreConstructor(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE states(id TEXT); INSERT INTO states VALUES('retained')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	t.Setenv("SQLRS_STATE_STORE", root)
	t.Setenv("SQLRS_STATE_DB", path)
	previous := newStoreFn
	newStoreFn = func(*sql.DB) (*sqlite.Store, error) {
		t.Fatal("legacy store constructor mutated schema")
		return nil, nil
	}
	defer func() { newStoreFn = previous }()
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + filepath.Join(t.TempDir(), "engine.json")})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "upgrade refused") {
		t.Fatal(code, err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow("SELECT id FROM states").Scan(&value); err != nil || value != "retained" {
		t.Fatal("legacy data changed", err)
	}
	var count int
	db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='managed_store_format'").Scan(&count)
	if count != 0 {
		t.Fatal("failed startup committed format")
	}
}
