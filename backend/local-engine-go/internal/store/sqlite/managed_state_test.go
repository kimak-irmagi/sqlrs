package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/store"
)

// Managed state identity is committed with the state row, not in a PGDATA marker
// or a separate best-effort write. Existing public JSON remains unchanged.
func TestManagedStateBindingPersistenceAndIsolation(t *testing.T) {
	ctx := context.Background()
	db, _ := openManagedPreflightDB(t)
	execManagedPreflightSQL(t, db, "PRAGMA foreign_keys=ON")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	format, err := EnsureManagedStoreFormat(ctx, tx, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := InstallManagedStateSchema(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	st := &Store{db: db}
	record := lineageCandidate(t, 1)
	record.Selector.DomainRef = format.DomainRef
	if _, err := st.Reserve(ctx, record); err != nil {
		t.Fatal(err)
	}
	state := store.StateCreate{StateID: "state-1", ImageID: "postgres@" + record.Selector.ImageDigest, PrepareKind: "psql", PrepareArgsNormalized: "[]", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), LineageRef: record.Binding.LineageRef, IdentityDigest: record.Binding.IdentityDigest}
	if err := st.CreateState(ctx, state); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.GetState(ctx, state.StateID)
	if err != nil || !ok || got.LineageRef != state.LineageRef || got.IdentityDigest != state.IdentityDigest {
		t.Fatalf("state binding not retained: %+v %v", got, err)
	}
	items, err := st.ListStates(ctx, store.StateFilters{})
	if err != nil || len(items) != 1 || items[0].LineageRef != state.LineageRef {
		t.Fatalf("state list binding: %+v %v", items, err)
	}
	public, err := json.Marshal(got)
	if err != nil || strings.Contains(string(public), state.LineageRef) || strings.Contains(string(public), state.IdentityDigest) {
		t.Fatal("internal identity widened public JSON")
	}
	if err := st.CreateState(ctx, state); err != nil {
		t.Fatal("exact duplicate is not idempotent", err)
	}
	for _, change := range []func(*store.StateCreate){
		func(s *store.StateCreate) { s.LineageRef = "" },
		func(s *store.StateCreate) { s.IdentityDigest = strings.Repeat("0", 64) },
		func(s *store.StateCreate) { s.LineageRef = "missing-lineage" },
		func(s *store.StateCreate) { s.ImageID = "postgres@sha256:" + strings.Repeat("f", 64) },
		func(s *store.StateCreate) { s.ImageID = "invalid-prefix" + record.Selector.ImageDigest },
	} {
		bad := state
		bad.StateID = "rejected"
		change(&bad)
		if err := st.CreateState(ctx, bad); err == nil {
			t.Fatal("invalid state binding accepted")
		}
		if _, ok, err := st.GetState(ctx, bad.StateID); err != nil || ok {
			t.Fatal("partial state publication")
		}
	}
	child := state
	child.StateID = "child"
	child.StateFingerprint = "child"
	child.ParentStateID = &state.StateID
	if err := st.CreateState(ctx, child); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := st.GetState(ctx, child.StateID); err != nil || !ok || got.LineageRef != state.LineageRef {
		t.Fatal("child state was not persisted", err)
	}
	collision := child
	collision.StateID = "fingerprint-collision"
	if err := st.CreateState(ctx, collision); err == nil {
		t.Fatal("fingerprint conflict was silently reported as successful publication")
	}
	other := lineageCandidate(t, 2)
	other.Selector.DomainRef = format.DomainRef
	other.Selector.InitSpecDigest = strings.Repeat("c", 64)
	if _, err := st.Reserve(ctx, other); err != nil {
		t.Fatal(err)
	}
	child.StateID = "foreign-child"
	child.LineageRef = other.Binding.LineageRef
	child.IdentityDigest = other.Binding.IdentityDigest
	if err := st.CreateState(ctx, child); err == nil {
		t.Fatal("child adopted a different lineage")
	}
	child.StateID = state.StateID
	child.ParentStateID = nil
	if err := st.CreateState(ctx, child); err == nil {
		t.Fatal("conflicting state ID silently reused")
	}
	if _, err := db.Exec("UPDATE states SET identity_digest=? WHERE state_id=?", other.Binding.IdentityDigest, state.StateID); err == nil {
		t.Fatal("state binding mutated")
	}
	if _, err := db.Exec("UPDATE managed_base_lineages SET username=?", other.Binding.Username); err == nil {
		t.Fatal("lineage identity mutated")
	}
	if _, err := db.Exec("DELETE FROM managed_base_lineages"); err == nil {
		t.Fatal("referenced lineage removed")
	}
	if err := st.DeleteState(ctx, "child"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteState(ctx, state.StateID); err != nil {
		t.Fatal(err)
	}
	if retained, err := st.Get(ctx, format.DomainRef, record.Binding.LineageRef); err != nil || retained.Binding != record.Binding {
		t.Fatal("state deletion removed durable lineage", err)
	}
}

// Schema errors must roll back the format reservation along with all DDL.
func TestManagedStateSchemaFailureRollback(t *testing.T) {
	ctx := context.Background()
	if err := InstallManagedStateSchema(ctx, nil); err != ErrManagedPreflightUnavailable {
		t.Fatalf("nil transaction: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := InstallManagedStateSchema(canceled, nil); err != context.Canceled {
		t.Fatalf("canceled installation: %v", err)
	}
	db, _ := openManagedPreflightDB(t)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := EnsureManagedStoreFormat(ctx, tx, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// A conflicting object makes installation fail after format reservation.
	if _, err := tx.Exec("CREATE VIEW states AS SELECT 'ambiguous' AS state_id"); err != nil {
		t.Fatal(err)
	}
	if err := InstallManagedStateSchema(ctx, tx); err != ErrManagedPreflightUnavailable {
		t.Fatalf("schema conflict: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial format/schema: %d %v", count, err)
	}
}
