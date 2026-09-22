package instanceaccess

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedidentity"
)

func accessFixture(t *testing.T) (*Service, managedidentity.RuntimeBinding) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := InstallSchema(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	secrets, err := OpenSecrets(filepath.Join(t.TempDir(), "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { secrets.Close() })
	service, err := NewService(db, secrets, "domain")
	if err != nil {
		t.Fatal(err)
	}
	id, err := managedidentity.Generate(strings.NewReader(strings.Repeat("b", 16)))
	if err != nil {
		t.Fatal(err)
	}
	return service, managedidentity.RuntimeBinding{RuntimeRef: "runtime", PhysicalIdentity: "physical", IdentityBinding: id}
}

func TestBootstrapDoesNotRegenerateAfterMetadataCommit(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	if _, _, err := s.Bootstrap(ctx, b.IdentityBinding, false); err != ErrNotFound {
		t.Fatal(err)
	}
	ref, secret, err := s.Bootstrap(ctx, b.IdentityBinding, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, reserve := range []bool{false, true} {
		got, value, err := s.Bootstrap(ctx, b.IdentityBinding, reserve)
		if err != nil || got != ref || value != secret {
			t.Fatal("bootstrap changed", err)
		}
	}
	if err := os.Remove(filepath.Join(s.secrets.path, ref.Ref()+".json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Bootstrap(ctx, b.IdentityBinding, true); err != ErrNotFound {
		t.Fatal("missing committed secret regenerated", err)
	}
	if _, _, err := s.Bootstrap(ctx, managedidentity.IdentityBinding{}, true); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DROP TRIGGER bootstrap_access_immutable; UPDATE bootstrap_access SET binding_json='corrupt'"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Bootstrap(ctx, b.IdentityBinding, false); err != ErrInvalid {
		t.Fatal(err)
	}
}

func TestAccessFailuresNeverPublish(t *testing.T) {
	for _, failure := range []string{"secret", "intent", "applying", "verified", "publish", "cancel", "invalid", "retired"} {
		t.Run(failure, func(t *testing.T) {
			s, b := accessFixture(t)
			ctx := context.Background()
			published := false
			if failure == "secret" {
				s.secrets.Close()
			}
			if failure == "intent" {
				s.db.Exec(`CREATE TRIGGER reject_access BEFORE INSERT ON instance_access BEGIN SELECT RAISE(ABORT,'sensitive failure'); END`)
			}
			if failure == "applying" || failure == "verified" {
				s.db.Exec(`CREATE TRIGGER reject_stage BEFORE UPDATE OF stage ON instance_access WHEN NEW.stage='` + failure + `' BEGIN SELECT RAISE(ABORT,'sensitive failure'); END`)
			}
			if failure == "invalid" {
				b.PhysicalIdentity = "../invalid"
			}
			if failure == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if failure == "retired" {
				if err := s.Activate(ctx, "instance", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
				if err := s.Retire(ctx, "instance", func() error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			err := s.Activate(ctx, "instance", b, func(Secret) error { return nil }, func() error {
				if failure == "publish" {
					return errors.New("sensitive failure")
				}
				published = true
				return nil
			})
			if err == nil || strings.Contains(err.Error(), "sensitive failure") || published {
				t.Fatal("failure published or leaked", err)
			}
		})
	}
}

func TestAccessReadsAndRetirementFailures(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	if _, err := s.Lookup(ctx, "missing"); err != ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, "missing", b); err != ErrNotFound {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, "missing", nil); err != ErrNotFound {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, "instance", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Lookup(ctx, "instance"); err != nil || got.RuntimeBinding != b {
		t.Fatal("verified lookup", err)
	}
	observed := false
	if err := s.Use(ctx, "instance", func(binding AccessBinding, ref SecretBinding) error {
		observed = binding.RuntimeBinding == b && ref.Ref() == binding.SecretRef
		return nil
	}); err != nil || !observed {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, "instance", func() error { return errors.New("cannot stop") }); err != ErrUnavailable {
		t.Fatal(err)
	}
	if err := s.Use(ctx, "instance", func(AccessBinding, SecretBinding) error { t.Fatal("retiring runtime used"); return nil }); err != ErrConflict {
		t.Fatal(err)
	}
	if _, err := s.Lookup(ctx, "instance"); err != ErrConflict {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, "instance", b, func(Secret) error { return nil }, func() error { t.Fatal("resurrected"); return nil }); err != ErrConflict {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, "instance", nil); err != ErrInvalid {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, "instance", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, "instance", func() error { t.Fatal("repeated physical deletion"); return nil }); err != nil {
		t.Fatal(err)
	}
	s.db.Close()
	if _, err := s.Lookup(ctx, "instance"); err != ErrUnavailable {
		t.Fatal(err)
	}
	if _, _, err := s.Bootstrap(ctx, b.IdentityBinding, true); err != ErrUnavailable {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, "instance", b, func(Secret) error { return nil }, func() error { return nil }); err != ErrUnavailable {
		t.Fatal(err)
	}
}

func TestSealWriteAndRetirementFenceFailure(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	op, err := s.Assign(ctx, "operation", t.TempDir(), "base", b.IdentityBinding)
	if err != nil {
		t.Fatal(err)
	}
	b.PhysicalIdentity = op.PhysicalIdentity
	if err := s.Attach(ctx, op, b); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER refuse_seal BEFORE INSERT ON managed_state_seals BEGIN SELECT RAISE(ABORT,'private canary'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSeal(ctx, "state", op, b); err != ErrUnavailable {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, "instance", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER refuse_retirement BEFORE UPDATE OF stage ON instance_access BEGIN SELECT RAISE(ABORT,'private canary'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, "instance", func() error { t.Fatal("deleted before durable fence"); return nil }); err != ErrUnavailable {
		t.Fatal(err)
	}
}

func TestAccessSchemaAndCancellation(t *testing.T) {
	s, b := accessFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewService(nil, s.secrets, "domain"); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := NewService(s.db, nil, "domain"); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := NewService(s.db, s.secrets, "../bad"); err != ErrInvalid {
		t.Fatal(err)
	}
	if err := InstallSchema(context.Background(), nil); err != ErrUnavailable {
		t.Fatal(err)
	}
	if err := InstallSchema(ctx, nil); err != context.Canceled {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	if err := InstallSchema(context.Background(), tx); err != ErrUnavailable {
		t.Fatal(err)
	}
	if _, _, err := s.Bootstrap(ctx, b.IdentityBinding, true); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := s.Assign(ctx, "operation", t.TempDir(), "base", b.IdentityBinding); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestOperationFailuresPreserveAssignment(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	target := filepath.Join(t.TempDir(), "base")
	if _, err := s.Assign(ctx, "../bad", target, "base", b.IdentityBinding); err != ErrInvalid {
		t.Fatal(err)
	}
	op, err := s.Assign(ctx, "base", target, "base", b.IdentityBinding)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BaseReady(ctx, op); err != nil {
		t.Fatal(err)
	}
	if err := s.BaseReady(ctx, op); err != ErrConflict {
		t.Fatal("repeated base completion accepted", err)
	}
	if err := s.Attach(ctx, op, managedidentity.RuntimeBinding{}); err != ErrInvalid {
		t.Fatal(err)
	}
	if err := s.RecordSeal(ctx, "state", Operation{Ref: "missing"}, b); err != ErrNotFound {
		t.Fatal(err)
	}
	if err := s.CheckSeal(ctx, "missing", b.IdentityBinding); err != ErrUnavailable {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO managed_state_seals VALUES('broken','base','corrupt')`); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckSeal(ctx, "broken", b.IdentityBinding); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER refuse_operation BEFORE INSERT ON managed_runtime_operations BEGIN SELECT RAISE(ABORT,'private canary'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign(ctx, "other", target, "base", b.IdentityBinding); err != ErrUnavailable {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE managed_runtime_operations SET identity_json='corrupt' WHERE operation_ref='base'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign(ctx, "base", target, "base", b.IdentityBinding); err != ErrInvalid {
		t.Fatal(err)
	}
	s.db.Close()
	if err := s.BaseReady(ctx, op); err != ErrUnavailable {
		t.Fatal(err)
	}
}

func TestActivationCancellationAfterProofAndMissingSecret(t *testing.T) {
	s, b := accessFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Activate(ctx, "instance", b, func(Secret) error { cancel(); return nil }, func() error { t.Fatal("cancelled activation published"); return nil }); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := s.Lookup(context.Background(), "instance"); err != ErrConflict {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background(), "instance", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	sb := s.secretBinding("instance", b)
	if err := os.Remove(filepath.Join(s.secrets.path, sb.Ref()+".json")); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background(), "instance", b, func(Secret) error { t.Fatal("lost secret regenerated"); return nil }, func() error { return nil }); err != ErrNotFound {
		t.Fatal(err)
	}
	if err := s.Use(context.Background(), "instance", func(AccessBinding, SecretBinding) error { t.Fatal("missing secret used"); return nil }); err != ErrNotFound {
		t.Fatal(err)
	}
	if err := s.Use(context.Background(), "missing", nil); err != ErrNotFound {
		t.Fatal(err)
	}
}

func TestMalformedAccessAndBootstrapWriteFailure(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	if err := s.Activate(ctx, "instance", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER instance_access_immutable; UPDATE instance_access SET binding_json='corrupt'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(ctx, "instance"); err != ErrInvalid {
		t.Fatal(err)
	}
	if err := s.transition(ctx, "missing", "reserved", "applying"); err != ErrConflict {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER refuse_bootstrap BEFORE INSERT ON bootstrap_access BEGIN SELECT RAISE(ABORT,'private canary'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Bootstrap(ctx, b.IdentityBinding, true); err != ErrUnavailable {
		t.Fatal(err)
	}
	s.secrets.Close()
	if _, _, err := s.Bootstrap(ctx, b.IdentityBinding, true); err != ErrUnavailable {
		t.Fatal(err)
	}
}
