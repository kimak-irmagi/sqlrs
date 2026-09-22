package run

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/registry"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store"
)

type managedRunRuntime struct {
	fakeRuntime
	inspectErr error
	observed   managedidentity.RuntimeBinding
}

func (r *managedRunRuntime) InspectManaged(_ context.Context, b managedidentity.RuntimeBinding) (engineRuntime.Instance, error) {
	r.observed = b
	return engineRuntime.Instance{ID: b.RuntimeRef, Binding: b}, r.inspectErr
}

func TestManagedRunUsesVerifiedBindingAndRetirementFence(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "access.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(instanceaccess.SchemaSQL); err != nil {
		t.Fatal(err)
	}
	secrets, err := instanceaccess.OpenSecrets(filepath.Join(t.TempDir(), "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	defer secrets.Close()
	access, err := instanceaccess.NewService(db, secrets, "domain")
	if err != nil {
		t.Fatal(err)
	}
	id, err := managedidentity.Generate(strings.NewReader(strings.Repeat("a", 16)))
	if err != nil {
		t.Fatal(err)
	}
	b := managedidentity.RuntimeBinding{IdentityBinding: id, RuntimeRef: "container-1", PhysicalIdentity: "physical"}
	const ref = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := access.Activate(ctx, ref, b, func(instanceaccess.Secret) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	st := openStore(t)
	defer st.Close()
	createInstance(t, st, ref)
	rt := &managedRunRuntime{}
	m, err := NewManager(Options{Registry: registry.New(st), Runtime: rt, Access: access})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"psql", "pgbench"} {
		if _, err := m.Run(ctx, Request{InstanceRef: ref, Kind: kind}); err != nil {
			t.Fatal(err)
		}
		call := rt.calls[len(rt.calls)-1]
		if call.Secret == nil || !strings.Contains(strings.Join(call.Args, " "), id.Username) || rt.observed != b {
			t.Fatal("managed connection lost binding")
		}
	}
	entry := store.InstanceEntry{InstanceID: ref}
	if _, err := m.execManaged(ctx, entry, "foreign", nil, nil); !errors.Is(err, instanceaccess.ErrConflict) {
		t.Fatal("runtime drift", err)
	}
	if _, err := m.execManaged(ctx, entry, b.RuntimeRef, []string{"bad\x00argument"}, nil); !errors.Is(err, instanceaccess.ErrInvalid) {
		t.Fatal("NUL accepted", err)
	}
	rt.inspectErr = errors.New("unavailable")
	before := len(rt.calls)
	if _, err := m.Run(ctx, Request{InstanceRef: ref, Kind: "psql"}); err == nil || len(rt.calls) != before {
		t.Fatal("uncertain runtime executed")
	}
	m.runtime = &fakeRuntime{}
	if _, err := m.execManaged(ctx, entry, b.RuntimeRef, nil, nil); !errors.Is(err, instanceaccess.ErrUnavailable) {
		t.Fatal("unsupported adapter", err)
	}
	m.runtime = rt
	rt.inspectErr = nil
	if err := access.Retire(ctx, ref, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run(ctx, Request{InstanceRef: ref, Kind: "psql"}); err == nil {
		t.Fatal("retired credential executed")
	}
}
