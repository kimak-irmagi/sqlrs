package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
)

// The caller owns one transaction for preflight, format reservation and all
// schema installation, so a failed migration cannot publish a new-format marker.
func TestManagedFormatDurableDomainAndRollback(t *testing.T) {
	ctx := context.Background()
	db, path := openManagedPreflightDB(t)
	checks := 0
	inventory := func(context.Context) error { checks++; return nil }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := EnsureManagedStoreFormat(ctx, tx, inventory)
	if err != nil || first.Version != ManagedStoreFormatVersion || first.DomainRef == "" {
		t.Fatalf("format=%+v err=%v", first, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback left schema: count=%d err=%v", count, err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := EnsureManagedStoreFormat(ctx, tx, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("CREATE TABLE states(value TEXT); INSERT INTO states VALUES ('new-format-state')"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	reopened, err := EnsureManagedStoreFormat(ctx, tx, func(context.Context) error { t.Fatal("new format must not run empty-legacy inventory"); return nil })
	if err != nil || reopened != committed || checks != 2 {
		t.Fatalf("reopen=%+v committed=%+v checks=%d err=%v", reopened, committed, checks, err)
	}
}

func TestManagedFormatFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := EnsureManagedStoreFormat(ctx, nil, nil); err != context.Canceled {
		t.Fatalf("canceled: %v", err)
	}
	if _, err := EnsureManagedStoreFormat(context.Background(), nil, nil); err != ErrManagedPreflightUnavailable {
		t.Fatalf("nil tx: %v", err)
	}
	for _, fixture := range []string{"closed-tx", "query-only", "inventory-error", "inventory-canceled", "entropy"} {
		t.Run(fixture, func(t *testing.T) {
			db, _ := openManagedPreflightDB(t)
			if fixture == "query-only" {
				execManagedPreflightSQL(t, db, "PRAGMA query_only=ON")
			}
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			inventory := func(context.Context) error { return nil }
			want := ErrManagedPreflightUnavailable
			if fixture == "closed-tx" {
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			}
			if fixture == "inventory-error" {
				inventory = func(context.Context) error { return errors.New("private-canary") }
			}
			if fixture == "inventory-canceled" {
				inventory = func(context.Context) error { return context.Canceled }
				want = context.Canceled
			}
			var got ManagedStoreFormat
			if fixture == "entropy" {
				got, err = ensureManagedStoreFormat(context.Background(), tx, inventory, bytes.NewReader(nil))
			} else {
				got, err = EnsureManagedStoreFormat(context.Background(), tx, inventory)
			}
			if err != want || got != (ManagedStoreFormat{}) {
				t.Fatalf("format=%+v err=%v, want %v", got, err, want)
			}
		})
	}
}

func TestManagedFormatRejectsLegacyOrUncertainInventory(t *testing.T) {
	for _, fixture := range []string{"legacy", "runtime", "missing-inventory", "partial-lineage", "view"} {
		t.Run(fixture, func(t *testing.T) {
			db, _ := openManagedPreflightDB(t)
			want := ErrManagedPreflightUnavailable
			inventory := func(context.Context) error { return nil }
			switch fixture {
			case "legacy":
				execManagedPreflightSQL(t, db, "CREATE TABLE states(value TEXT); INSERT INTO states VALUES ('legacy')")
				want = ErrLegacyManagedData
			case "runtime":
				inventory = func(context.Context) error { return ErrLegacyManagedData }
				want = ErrLegacyManagedData
			case "missing-inventory":
				inventory = nil
			case "partial-lineage":
				execManagedPreflightSQL(t, db, ManagedLineageSchemaSQL())
			case "view":
				execManagedPreflightSQL(t, db, "CREATE VIEW managed_store_format AS SELECT 1")
			}
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			got, err := EnsureManagedStoreFormat(context.Background(), tx, inventory)
			if !errors.Is(err, want) || got != (ManagedStoreFormat{}) {
				t.Fatalf("format=%+v err=%v, want=%v", got, err, want)
			}
		})
	}
}

func TestManagedFormatPreservesSettingsAndRejectsCorruption(t *testing.T) {
	for _, mutation := range []string{
		"UPDATE managed_store_format SET format_version='unsupported'",
		"UPDATE managed_store_format SET domain_ref='../foreign'",
		"DELETE FROM managed_store_format",
		"DROP TABLE managed_store_format; CREATE TABLE managed_store_format(domain_ref TEXT)",
	} {
		t.Run(mutation, func(t *testing.T) {
			db, _ := openManagedPreflightDB(t)
			execManagedPreflightSQL(t, db, "CREATE TABLE settings(value TEXT); INSERT INTO settings VALUES ('preserve')")
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := EnsureManagedStoreFormat(context.Background(), tx, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			execManagedPreflightSQL(t, db, mutation)
			tx, err = db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := EnsureManagedStoreFormat(context.Background(), tx, func(context.Context) error { t.Fatal("corruption must not regenerate domain"); return nil }); err != ErrManagedPreflightUnavailable {
				t.Fatalf("corrupt format: %v", err)
			}
			var setting string
			if err := tx.QueryRow("SELECT value FROM settings").Scan(&setting); err != nil || setting != "preserve" {
				t.Fatalf("settings changed: %q %v", setting, err)
			}
		})
	}
}
