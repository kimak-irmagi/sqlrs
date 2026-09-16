package instanceaccess

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedidentity"
)

func TestPhysicalOperationsAndSeals(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := InstallSchema(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	secrets, err := OpenSecrets(filepath.Join(t.TempDir(), "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	defer secrets.Close()
	s, err := NewService(db, secrets, "domain")
	if err != nil {
		t.Fatal(err)
	}
	id, err := managedidentity.Generate(strings.NewReader(strings.Repeat("a", 16)))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "physical")
	op, err := s.Assign(ctx, "operation", target, "base", id)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Assign(ctx, "operation", target, "base", id)
	if err != nil || op != again {
		t.Fatal("unstable operation", err)
	}
	if _, err := s.Assign(ctx, "operation", target, "other-source", id); err != ErrConflict {
		t.Fatal("operation adopted other source", err)
	}
	binding := managedidentity.RuntimeBinding{RuntimeRef: "container", PhysicalIdentity: op.PhysicalIdentity, IdentityBinding: id}
	if err := s.Attach(ctx, op, binding); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSeal(ctx, "state", op, binding); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckSeal(ctx, "state", id); err != nil {
		t.Fatal(err)
	}
	bad := binding
	bad.RuntimeRef = "other"
	if err := s.RecordSeal(ctx, "other-state", op, bad); err == nil {
		t.Fatal("stale proof accepted")
	}
	if err := s.RetireOperation(ctx, op); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSeal(ctx, "late-state", op, binding); err == nil {
		t.Fatal("retired operation published")
	}
}

func TestBaseEvictionCreatesNewPhysicalOperationWithSameIdentity(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	target := filepath.Join(t.TempDir(), "base")
	first, err := s.BaseOperation(ctx, "base-key", target, b.IdentityBinding, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BaseReady(ctx, first); err != nil {
		t.Fatal(err)
	}
	reused, err := s.BaseOperation(ctx, "base-key", target, b.IdentityBinding, true)
	if err != nil || reused.PhysicalIdentity != first.PhysicalIdentity {
		t.Fatal(err)
	}
	rebuilt, err := s.BaseOperation(ctx, "base-key", target, b.IdentityBinding, false)
	if err != nil || rebuilt.PhysicalIdentity == first.PhysicalIdentity || rebuilt.Identity != first.Identity || rebuilt.Stage != "assigned" {
		t.Fatal("eviction changed identity or reused physical generation", err)
	}
	old, err := s.operation(ctx, first.Ref)
	if err != nil || old.Stage != "retired" {
		t.Fatal("old physical operation still active", err)
	}
	if _, err := s.BaseOperation(ctx, "unowned", target, b.IdentityBinding, true); err != ErrConflict {
		t.Fatal("adopted existing target", err)
	}
}
