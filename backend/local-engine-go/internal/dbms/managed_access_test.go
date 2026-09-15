package dbms

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedidentity"
)

func managedAccessFixture(t *testing.T) ManagedConnection {
	t.Helper()
	identity, err := managedidentity.Generate(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	return ManagedConnection{Binding: managedidentity.RuntimeBinding{RuntimeRef: "container-1", PhysicalIdentity: "clone-1", IdentityBinding: identity}, Host: "127.0.0.1", Port: 5432, Password: strings.Repeat("a", 64)}
}

func TestManagedAccessConfiguration(t *testing.T) {
	req := managedAccessFixture(t)
	cfg, err := managedConnectionConfig(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != req.Host || cfg.Port != req.Port || cfg.User != req.Binding.Username || cfg.Database != "postgres" || cfg.Password != req.Password || cfg.RequireAuth != "scram-sha-256" || cfg.TLSConfig != nil || len(cfg.Fallbacks) != 0 || len(cfg.RuntimeParams) != 0 {
		t.Fatal("managed connection configuration differs from explicit binding")
	}
	for _, name := range []string{"PGSERVICE", "PGPASSWORD", "PGHOST", "PGPASSFILE", "PGOPTIONS"} {
		if _, err := managedConnectionConfig(req, []string{name + "=private-canary"}); err != ErrManagedAccessUnavailable {
			t.Fatalf("ambient config accepted: %s %v", name, err)
		}
	}
}

func TestManagedAccessRejectsInvalidBindingAndEndpoint(t *testing.T) {
	valid := managedAccessFixture(t)
	for _, change := range []func(*ManagedConnection){
		func(r *ManagedConnection) { r.Binding.RuntimeRef = "" },
		func(r *ManagedConnection) { r.Binding.Username = "postgres" },
		func(r *ManagedConnection) { r.Host = "/tmp" },
		func(r *ManagedConnection) { r.Host = "remote.example" },
		func(r *ManagedConnection) { r.Host = "192.0.2.1" },
		func(r *ManagedConnection) { r.Port = 0 },
		func(r *ManagedConnection) { r.Password = "" },
		func(r *ManagedConnection) { r.Password = strings.Repeat("g", 64) },
	} {
		req := valid
		change(&req)
		if _, err := VerifyManagedAccess(context.Background(), req); err != ErrManagedAccessUnavailable {
			t.Fatalf("invalid request accepted: %v", err)
		}
	}
}

func TestManagedConnectionDiagnosticsAreRedacted(t *testing.T) {
	req := managedAccessFixture(t)
	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		if got := fmt.Sprintf(format, req); strings.Contains(got, req.Password) {
			t.Fatal("password leaked by formatting")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifyManagedAccess(ctx, req); err != context.Canceled {
		t.Fatalf("canceled verification: %v", err)
	}
}
