package deletion

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/store"
	_ "modernc.org/sqlite"
)

type managedDeleteRuntime struct {
	fakeRuntime
	binding managedidentity.RuntimeBinding
}

func (r *managedDeleteRuntime) StopManaged(ctx context.Context, b managedidentity.RuntimeBinding) error {
	r.binding = b
	return r.Stop(ctx, b.RuntimeRef)
}

func TestManagedDeletionKeepsFenceAndDataAfterFailure(t *testing.T) {
	for _, tree := range []bool{false, true} {
		t.Run(map[bool]string{false: "instance", true: "tree"}[tree], func(t *testing.T) {
			ctx := context.Background()
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", filepath.Join(root, "access.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(instanceaccess.SchemaSQL); err != nil {
				t.Fatal(err)
			}
			secrets, err := instanceaccess.OpenSecrets(filepath.Join(root, "secrets"))
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
			binding := managedidentity.RuntimeBinding{IdentityBinding: id, RuntimeRef: "container", PhysicalIdentity: "physical"}
			if err := access.Activate(ctx, "instance", binding, func(instanceaccess.Secret) error { return nil }, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "clone")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			data := filepath.Join(dir, "keep")
			if err := os.WriteFile(data, []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			st := &fakeStore{instances: map[string]store.InstanceEntry{"instance": {InstanceID: "instance", RuntimeID: &binding.RuntimeRef, RuntimeDir: &dir}}}
			rt := &managedDeleteRuntime{fakeRuntime: fakeRuntime{stopErr: errors.New("daemon unavailable")}}
			m, err := NewManager(Options{Store: st, Runtime: rt, Access: access})
			if err != nil {
				t.Fatal(err)
			}
			remove := func() error {
				if tree {
					return m.deleteTree(ctx, DeleteNode{Kind: "instance", ID: "instance", RuntimeID: &binding.RuntimeRef, RuntimeDir: &dir})
				}
				_, _, err := m.DeleteInstance(ctx, "instance", DeleteOptions{})
				return err
			}
			if err := remove(); err == nil {
				t.Fatal("uncertain stop authorized delete")
			}
			if _, err := os.Stat(data); err != nil || len(st.deletedInstances) != 0 {
				t.Fatal("data erased after uncertain stop", err)
			}
			if _, err := access.Lookup(ctx, "instance"); err == nil {
				t.Fatal("retiring access still usable")
			}
			rt.stopErr = nil
			st.deleteInstanceErr = errors.New("metadata write unavailable")
			if err := remove(); err == nil {
				t.Fatal("metadata failure hidden")
			}
			st.deleteInstanceErr = nil
			if err := remove(); err != nil {
				t.Fatal("retirement retry", err)
			}
			if rt.binding != binding {
				t.Fatal("cleanup lost runtime ownership")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("owned directory remains", err)
			}
			if err := m.stopManagedRuntime(ctx, nil, instanceaccess.AccessBinding{RuntimeBinding: binding}); err != instanceaccess.ErrConflict {
				t.Fatal(err)
			}
			m.runtime = &fakeRuntime{}
			if err := m.stopManagedRuntime(ctx, &binding.RuntimeRef, instanceaccess.AccessBinding{RuntimeBinding: binding}); err != instanceaccess.ErrUnavailable {
				t.Fatal(err)
			}
		})
	}
}
