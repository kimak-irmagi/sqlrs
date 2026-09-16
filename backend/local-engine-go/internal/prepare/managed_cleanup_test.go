package prepare

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store"
)

func TestManagedCleanupPreservesTargetWhenStopIsUncertain(t *testing.T) {
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
	b, err := managedidentity.Generate(strings.NewReader(strings.Repeat("a", 16)))
	if err != nil {
		t.Fatal(err)
	}
	op, err := access.Assign(ctx, "operation", filepath.Join(t.TempDir(), "clone"), "source", b)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &fakeRuntime{stopErr: errors.New("unavailable")}
	m := newManagerWithDeps(t, &fakeStore{states: []store.StateCreate{}}, newQueueStore(t), &testDeps{runtime: runtime})
	m.access = access
	removed := false
	rt := &jobRuntime{operation: op, instance: engineRuntime.Instance{ID: "container"}, cleanup: func() error { removed = true; return nil }}
	runner := &jobRunner{rt: rt}
	if err := m.cleanupRuntime(ctx, runner); err == nil || removed || runner.getRuntime() != rt {
		t.Fatal("uncertain stop discarded physical ownership", err)
	}
	runtime.stopErr = nil
	if err := m.cleanupRuntime(ctx, runner); err != nil || !removed || runner.getRuntime() != nil {
		t.Fatal("cleanup retry failed", err)
	}
}
