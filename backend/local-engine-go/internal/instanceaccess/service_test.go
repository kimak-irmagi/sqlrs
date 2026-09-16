package instanceaccess

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedidentity"
	_ "modernc.org/sqlite"
)

func TestActivationLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := sql.Open("sqlite", path)
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
	s, err := NewService(db, secrets, "store_test")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := managedidentity.Generate(strings.NewReader(strings.Repeat("a", 16)))
	if err != nil {
		t.Fatal(err)
	}
	binding := managedidentity.RuntimeBinding{RuntimeRef: "runtime_test", PhysicalIdentity: "physical_test", IdentityBinding: identity}
	var password string
	published := false
	apply := func(secret Secret) error {
		if password != "" && password != secret.Password {
			t.Fatal("retry rotated password")
		}
		password = secret.Password
		return nil
	}
	publish := func() error { published = true; return nil }
	if err := s.Activate(ctx, "instance_test", binding, apply, publish); err != nil || !published {
		t.Fatal("activation", err)
	}
	conn, err := s.Resolve(ctx, "instance_test", binding)
	if err != nil || conn.Password != password {
		t.Fatal("resolve", err)
	}
	s, err = NewService(db, secrets, "store_test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, "instance_test", binding, apply, publish); err != nil {
		t.Fatal("retry", err)
	}
	changed := binding
	changed.PhysicalIdentity = "other"
	if err := s.Activate(ctx, "instance_test", changed, apply, publish); !errors.Is(err, ErrConflict) {
		t.Fatal("rebound physical runtime", err)
	}
	var raw string
	if err := db.QueryRow("SELECT binding_json FROM instance_access WHERE instance_ref='instance_test'").Scan(&raw); err != nil || strings.Contains(raw, password) {
		t.Fatal("secret in control database", err)
	}
	removed := false
	if err := s.Retire(ctx, "instance_test", func() error { removed = true; return nil }); err != nil || !removed {
		t.Fatal("retire", err)
	}
	if _, err := s.Resolve(ctx, "instance_test", binding); err == nil {
		t.Fatal("retired credentials exposed")
	}
	published = false
	if err := s.Activate(ctx, "instance_test", binding, apply, publish); err == nil || published {
		t.Fatal("retirement resurrected instance")
	}
	if err := s.Activate(ctx, "instance_failed", binding, func(Secret) error { return errors.New("secret canary") }, publish); err != ErrUnavailable || published {
		t.Fatal("failed proof published or leaked", err)
	}
	if _, err := s.Resolve(ctx, "instance_failed", binding); err == nil {
		t.Fatal("failed proof resolved")
	}
}
