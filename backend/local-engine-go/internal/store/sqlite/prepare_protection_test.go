package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sqlrs/engine-local/internal/store"
)

// CP-PG05: a standalone store has no prepare owner namespace; an unreadable
// store must return an error rather than allowing physical source removal.
// Actual active/released clone protection uses the full managed schema in
// queue's planning-DB source deletion tests.
func TestPrepareSourceProtectionUnavailableAndStandalone(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "standalone.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{db: db}
	defer db.Close()
	protected, err := store.PrepareProtectsState(t.Context(), s, "source")
	if err != nil || protected {
		t.Fatal("standalone source unexpectedly pinned", protected, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if protected, err = store.PrepareProtectsState(t.Context(), s, "source"); err == nil || protected {
		t.Fatal("unreadable namespace granted deletion", protected, err)
	}
	if protected, err = store.PrepareProtectsState(t.Context(), struct{ store.Store }{}, "source"); err != nil || protected {
		t.Fatal("nonjournal store changed", protected, err)
	}
}
