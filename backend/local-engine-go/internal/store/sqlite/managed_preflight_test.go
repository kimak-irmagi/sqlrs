package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These fixtures bypass Store.New: the upgrade gate must precede its migrations.
// See docs/architecture/managed-database-identity-tests.md, upgrade preflight.
func openManagedPreflightDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metadata.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func execManagedPreflightSQL(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

func assertManagedPreflightUnchanged(t *testing.T, db *sql.DB, path string, want error) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// SQLite itself rejects writes, in addition to the byte-for-byte assertion.
	execManagedPreflightSQL(t, db, "PRAGMA query_only = ON")
	got := CheckEmptyManagedMetadata(context.Background(), db)
	if !errors.Is(got, want) {
		t.Fatalf("preflight = %v, want %v", got, want)
	}
	if want != nil && got.Error() != want.Error() {
		t.Fatalf("diagnostic must be bounded: %v", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("preflight modified database schema or data")
	}
}

func TestCheckEmptyManagedMetadataRejectsEachLegacyTable(t *testing.T) {
	for _, table := range []string{"states", "instances", "names", "prepare_jobs", "prepare_tasks", "prepare_events", "instance_access"} {
		t.Run(table, func(t *testing.T) {
			db, path := openManagedPreflightDB(t)
			execManagedPreflightSQL(t, db, "CREATE TABLE "+table+" (value TEXT)")
			execManagedPreflightSQL(t, db, "INSERT INTO "+table+" VALUES ('private-canary')")
			execManagedPreflightSQL(t, db, "CREATE TABLE settings (value TEXT)")
			execManagedPreflightSQL(t, db, "INSERT INTO settings VALUES ('preserve-me')")
			assertManagedPreflightUnchanged(t, db, path, ErrLegacyManagedData)
		})
	}
}

func TestCheckEmptyManagedMetadataAllowsEmptyAndUnrelatedData(t *testing.T) {
	for _, fixture := range []string{"new", "empty-tables", "settings"} {
		t.Run(fixture, func(t *testing.T) {
			db, path := openManagedPreflightDB(t)
			if err := db.Ping(); err != nil {
				t.Fatal(err)
			}
			if fixture != "new" {
				for _, table := range []string{"states", "instances", "names", "prepare_jobs", "prepare_tasks", "prepare_events", "instance_access"} {
					execManagedPreflightSQL(t, db, "CREATE TABLE "+table+" (value TEXT)")
				}
			}
			if fixture == "settings" {
				execManagedPreflightSQL(t, db, "CREATE TABLE settings (value TEXT)")
				execManagedPreflightSQL(t, db, "INSERT INTO settings VALUES ('preserve-me')")
			}
			assertManagedPreflightUnchanged(t, db, path, nil)
		})
	}
}

func TestCheckEmptyManagedMetadataRejectsAmbiguousSchema(t *testing.T) {
	for _, statement := range []string{
		"CREATE VIEW states AS SELECT 'private-canary' AS value WHERE 0",
		"CREATE TABLE settings (value TEXT); CREATE INDEX states ON settings(value)",
	} {
		t.Run(statement, func(t *testing.T) {
			db, path := openManagedPreflightDB(t)
			execManagedPreflightSQL(t, db, statement)
			assertManagedPreflightUnchanged(t, db, path, ErrManagedPreflightUnavailable)
		})
	}
}

func TestCheckEmptyManagedMetadataSQLiteNamesAreCaseInsensitive(t *testing.T) {
	db, path := openManagedPreflightDB(t)
	execManagedPreflightSQL(t, db, "CREATE TABLE STATES (value TEXT); INSERT INTO STATES VALUES ('private-canary')")
	assertManagedPreflightUnchanged(t, db, path, ErrLegacyManagedData)
}

func TestCheckEmptyManagedMetadataUnavailable(t *testing.T) {
	if err := CheckEmptyManagedMetadata(context.Background(), nil); err != ErrManagedPreflightUnavailable {
		t.Fatalf("nil database: %v", err)
	}
	db, _ := openManagedPreflightDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := CheckEmptyManagedMetadata(context.Background(), db); err != ErrManagedPreflightUnavailable {
		t.Fatalf("closed database: %v", err)
	}
}

func TestCheckEmptyManagedMetadataPreservesCancellation(t *testing.T) {
	db, _ := openManagedPreflightDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckEmptyManagedMetadata(ctx, db); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := CheckEmptyManagedMetadata(ctx, db); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired deadline: %v", err)
	}
}

// Corrupt actual b-tree pages, rather than fabricating an impossible scanner.
// Catalog and table-page failures both forbid upgrade without exposing values.
func TestCheckEmptyManagedMetadataCorruptPages(t *testing.T) {
	for _, page := range []string{"catalog", "table"} {
		t.Run(page, func(t *testing.T) {
			db, path := openManagedPreflightDB(t)
			execManagedPreflightSQL(t, db, "PRAGMA page_size = 4096; CREATE TABLE states (value TEXT); INSERT INTO states VALUES ('private-canary')")
			var rootPage int
			if err := db.QueryRow("SELECT rootpage FROM sqlite_master WHERE name = 'states'").Scan(&rootPage); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			offset := 100 // First-page b-tree header follows SQLite's file header.
			if page == "table" {
				offset = (rootPage - 1) * 4096
			}
			data[offset] = 0xff // Invalid b-tree page type, as after physical corruption.
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			db, err = sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if got := CheckEmptyManagedMetadata(context.Background(), db); got != ErrManagedPreflightUnavailable {
				t.Fatalf("corrupt %s diagnostic: %v", page, got)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatalf("corrupt store changed: %v", err)
			}
		})
	}
}

func TestCheckEmptyManagedMetadataCanceledWhileWaitingForConnection(t *testing.T) {
	db, _ := openManagedPreflightDB(t)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- CheckEmptyManagedMetadata(ctx, db) }()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for db.Stats().WaitCount == 0 {
		select {
		case <-deadline:
			t.Fatal("preflight did not enter connection wait")
		case <-tick.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("waiting cancellation: %v", err)
		}
	case <-deadline:
		t.Fatal("preflight did not observe cancellation")
	}
}

func TestManagedPreflightErrorPreservesWrappedCancellation(t *testing.T) {
	for _, known := range []error{context.Canceled, context.DeadlineExceeded} {
		if got := managedPreflightError(context.Background(), fmt.Errorf("private-canary: %w", known)); got != known {
			t.Fatalf("wrapped cancellation: %v", got)
		}
	}
	if got := managedPreflightError(context.Background(), errors.New("private-canary")); got != ErrManagedPreflightUnavailable {
		t.Fatalf("unbounded diagnostic: %v", got)
	}
}
