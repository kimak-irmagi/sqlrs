package instanceaccess

import (
	"context"
	"database/sql"
	"os"
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
	other, err := s.Assign(ctx, "other-operation", filepath.Join(t.TempDir(), "other"), "base", id)
	if err != nil {
		t.Fatal(err)
	}
	otherBinding := managedidentity.RuntimeBinding{RuntimeRef: "other-container", PhysicalIdentity: other.PhysicalIdentity, IdentityBinding: id}
	if err := s.Attach(ctx, other, otherBinding); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSeal(ctx, "state", other, otherBinding); err != ErrConflict {
		t.Fatal("seal adopted another physical capture", err)
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

func TestBaseAssignmentFailureKeepsDurableWinner(t *testing.T) {
	for _, scenario := range []string{"invalid", "closed", "missing-operation", "mismatch", "retirement-write", "assignment-write", "head-write"} {
		t.Run(scenario, func(t *testing.T) {
			s, b := accessFixture(t)
			ctx := context.Background()
			target := filepath.Join(t.TempDir(), "base")
			first, err := s.BaseOperation(ctx, "base", target, b.IdentityBinding, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.BaseReady(ctx, first); err != nil {
				t.Fatal(err)
			}
			key := "base"
			switch scenario {
			case "invalid":
				key = "../invalid"
			case "closed":
				s.db.Close()
			case "missing-operation":
				_, err = s.db.Exec("DELETE FROM managed_runtime_operations")
			case "mismatch":
				target = filepath.Join(t.TempDir(), "other")
			case "retirement-write":
				_, err = s.db.Exec(`CREATE TRIGGER refuse BEFORE UPDATE ON managed_runtime_operations BEGIN SELECT RAISE(ABORT,'canary'); END`)
			case "assignment-write":
				_, err = s.db.Exec(`CREATE TRIGGER refuse BEFORE INSERT ON managed_runtime_operations BEGIN SELECT RAISE(ABORT,'canary'); END`)
			case "head-write":
				_, err = s.db.Exec(`CREATE TRIGGER refuse BEFORE INSERT ON managed_base_targets BEGIN SELECT RAISE(ABORT,'canary'); END`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.BaseOperation(ctx, key, target, b.IdentityBinding, false); err == nil || strings.Contains(err.Error(), "canary") {
				t.Fatal("invalid assignment or unbounded failure", err)
			}
			if strings.HasSuffix(scenario, "write") {
				current, err := s.operation(ctx, first.Ref)
				if err != nil || current.Stage != "ready" {
					t.Fatal("failed replacement retired winner", err)
				}
			}
		})
	}
}

func TestPublicationRecoveryRefusesLostEvidence(t *testing.T) {
	for _, scenario := range []string{"missing-intent", "missing-operation", "retired-operation", "corrupt-operation", "missing-secret"} {
		t.Run(scenario, func(t *testing.T) {
			s, b := accessFixture(t)
			ctx := context.Background()
			op, err := s.Assign(ctx, "operation", filepath.Join(t.TempDir(), "target"), "source", b.IdentityBinding)
			if err != nil {
				t.Fatal(err)
			}
			b.PhysicalIdentity = op.PhysicalIdentity
			if err := s.Attach(ctx, op, b); err != nil {
				t.Fatal(err)
			}
			if err := s.Activate(ctx, "instance", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			ref := "instance"
			switch scenario {
			case "missing-intent":
				ref = "missing"
			case "missing-operation":
				_, err = s.db.Exec("DELETE FROM managed_runtime_operations")
			case "retired-operation":
				err = s.RetireOperation(ctx, op)
			case "corrupt-operation":
				_, err = s.db.Exec("UPDATE managed_runtime_operations SET identity_json='broken'")
			case "missing-secret":
				sb := s.secretBinding(ref, b)
				err = os.Remove(filepath.Join(s.secrets.path, sb.Ref()+".json"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.ResumePublication(ctx, ref); err == nil {
				t.Fatal("lost ownership evidence recovered")
			}
		})
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

func TestPublicationRecoveryRetainsExactIntent(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	op, err := s.Assign(ctx, "operation", filepath.Join(t.TempDir(), "target"), "source", b.IdentityBinding)
	if err != nil {
		t.Fatal(err)
	}
	b.PhysicalIdentity = op.PhysicalIdentity
	if err := s.Attach(ctx, op, b); err != nil {
		t.Fatal(err)
	}
	var password string
	if err := s.Activate(ctx, "instance", b, func(secret Secret) error { password = secret.Password; return nil }, func() error { return ErrUnavailable }); err == nil {
		t.Fatal("publication failure hidden")
	}
	// Fresh coordinator, durable binding and original secret after external mutation.
	reopened, err := NewService(s.db, s.secrets, s.domain)
	if err != nil {
		t.Fatal(err)
	}
	intent, actual, err := reopened.ResumePublication(ctx, "instance")
	if err != nil || intent.RuntimeBinding != b || actual.Ref != op.Ref {
		t.Fatal("recovery lost ownership", err)
	}
	if err := reopened.Activate(ctx, "instance", b, func(secret Secret) error {
		if secret.Password != password {
			t.Fatal("recovery rotated secret")
		}
		return nil
	}, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Retire(ctx, "instance", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reopened.ResumePublication(ctx, "instance"); err == nil {
		t.Fatal("retirement resurrected")
	}
}
