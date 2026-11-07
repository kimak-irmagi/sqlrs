package main

import (
	"github.com/sqlrs/engine-local/internal/sqliteconn"
	"path/filepath"
	"testing"
)

// MP07: the real startup factory must configure every physical connection,
// including a replacement, before a finalization writer receives it.
func TestMainPathStartupUsesManagedConnections(t *testing.T) {
	db, err := openDBFn(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for range 2 {
		conn, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sqliteconn.ObserveClose(conn); err != nil {
			_ = conn.Close()
			t.Fatal("startup connection is not observable", err)
		}
		var fk int
		if err := conn.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
			t.Fatal("foreign keys are not configured", err)
		}
		_ = conn.Close()
		db.SetMaxIdleConns(0)
	}
}
