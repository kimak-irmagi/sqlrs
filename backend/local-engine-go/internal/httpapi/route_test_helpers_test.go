package httpapi

import (
	"testing"

	"github.com/sqlrs/engine-local/internal/conntrack"
	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/registry"
)

func newRouteTestOptions(t *testing.T) (Options, func()) {
	t.Helper()

	db, st, queueStore := newMemoryHTTPStore(t)

	deleteMgr, err := deletion.NewManager(deletion.Options{
		Store: st,
		Conn:  conntrack.Noop{},
	})
	if err != nil {
		t.Fatalf("delete manager: %v", err)
	}

	opts := Options{
		Version:    "test",
		InstanceID: "instance",
		AuthToken:  "secret",
		Registry:   registry.New(st),
		Prepare:    newPrepareManager(t, st, queueStore),
		Deletion:   deleteMgr,
		Config:     &fakeConfig{schema: map[string]any{"type": "object"}},
	}

	cleanup := func() {
		_ = db.Close()
		_ = st.Close()
	}
	return opts, cleanup
}
