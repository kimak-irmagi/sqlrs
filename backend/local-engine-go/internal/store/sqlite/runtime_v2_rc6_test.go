package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

const rc6FixtureSHA256 = "9a6723b087cd88d7ce15bd9b5660e56835d97d08774c8ca1ec065cbaec517455"

func copyRC6Fixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "rc6", "populated-rc6.db"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != rc6FixtureSHA256 {
		t.Fatalf("rc.6 fixture checksum = %s", hex.EncodeToString(digest[:]))
	}
	path := filepath.Join(t.TempDir(), "store.db")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRuntimeV2PopulatedRC6UpgradeAndReverseCompatibility(t *testing.T) {
	path := copyRC6Fixture(t)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checks := map[string]int{
		`SELECT count(*) FROM states WHERE state_id IN ('rc6-root','rc6-child')`:               2,
		`SELECT count(*) FROM instances WHERE instance_id='rc6-instance'`:                      1,
		`SELECT count(*) FROM names WHERE name='rc6-main'`:                                     1,
		`SELECT count(*) FROM prepare_jobs WHERE job_id='rc6-job'`:                             1,
		`SELECT count(*) FROM prepare_tasks WHERE job_id='rc6-job'`:                            1,
		`SELECT count(*) FROM prepare_events WHERE job_id='rc6-job'`:                           1,
		`SELECT count(*) FROM settings WHERE value='1752abbb44e72e5d4eaf6d310e70e5777a1bc9b0'`: 1,
		`SELECT count(*) FROM unrelated_state_summary`:                                         2,
		`SELECT count(*) FROM runtime_v2_store_format WHERE slot=1`:                            1,
	}
	for query, want := range checks {
		var got int
		if err := store.db.QueryRowContext(context.Background(), query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s = %d, want %d: %v", query, got, want, err)
		}
	}
	// Representative SQL from the pinned rc.6 namespace continues to work after
	// the new reserved objects exist; old code neither reads nor rewrites them.
	if _, err := store.db.Exec(`INSERT INTO states(state_id,state_fingerprint,image_id,prepare_kind,prepare_args_normalized,created_at) VALUES('rc6-after-upgrade','rc6-after-fingerprint','postgres:16','psql','{}','2026-02-01')`); err != nil {
		t.Fatal(err)
	}
	var marker int
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_store_format`).Scan(&marker); err != nil || marker != 1 {
		t.Fatalf("rc.6 CRUD damaged v2 marker: %d %v", marker, err)
	}
}
