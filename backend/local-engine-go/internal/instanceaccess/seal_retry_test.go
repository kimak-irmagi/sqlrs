package instanceaccess

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/store"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
)

// A failed capture leaves no published state row. A new physical operation must
// be able to retry after retirement without adopting a live/published capture.
func TestSealRetryAfterFailedCapture(t *testing.T) {
	for _, scenario := range []string{"unpublished", "published", "active", "other-identity", "missing-owner", "mismatched-capture", "write-failure"} {
		t.Run(scenario, func(t *testing.T) {
			s, b := accessFixture(t)
			st, err := storesqlite.New(s.db)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			old, err := s.Assign(ctx, "old", t.TempDir(), "source", b.IdentityBinding)
			if err != nil {
				t.Fatal(err)
			}
			b.PhysicalIdentity = old.PhysicalIdentity
			if err := s.Attach(ctx, old, b); err != nil {
				t.Fatal(err)
			}
			if err := s.RecordSeal(ctx, "state", old, b); err != nil {
				t.Fatal(err)
			}
			if scenario == "published" {
				if err := st.CreateState(ctx, store.StateCreate{StateID: "state", StateFingerprint: "state", ImageID: "image", PrepareKind: "psql", CreatedAt: "2026-09-16"}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "active" {
				if err := s.RetireOperation(ctx, old); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "other-identity" {
				b.IdentityBinding, err = managedidentity.Generate(strings.NewReader(strings.Repeat("z", 16)))
				if err != nil {
					t.Fatal(err)
				}
			}
			next, err := s.Assign(ctx, "next", t.TempDir(), "source", b.IdentityBinding)
			if err != nil {
				t.Fatal(err)
			}
			b.PhysicalIdentity, b.RuntimeRef = next.PhysicalIdentity, "next-runtime"
			if err := s.Attach(ctx, next, b); err != nil {
				t.Fatal(err)
			}
			if scenario == "missing-owner" {
				if _, err := s.db.Exec("DELETE FROM managed_runtime_operations WHERE operation_ref='old'"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "write-failure" {
				if _, err := s.db.Exec(`CREATE TRIGGER reject_reseal BEFORE UPDATE ON managed_state_seals BEGIN SELECT RAISE(ABORT,'private canary'); END`); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "mismatched-capture" {
				if _, err := s.db.Exec("UPDATE managed_state_seals SET binding_json='corrupt' WHERE state_id='state'"); err != nil {
					t.Fatal(err)
				}
			}
			// Reconstruct coordination from persisted ownership, not process memory.
			s, err = NewService(s.db, s.secrets, s.domain)
			if err != nil {
				t.Fatal(err)
			}
			err = s.RecordSeal(ctx, "state", next, b)
			if scenario == "unpublished" {
				if err != nil {
					t.Fatal("failed capture poisoned retry", err)
				}
				if err := s.CheckSeal(ctx, "state", b.IdentityBinding); err != nil {
					t.Fatal(err)
				}
				if err := s.RecordSeal(ctx, "state", next, b); err != nil {
					t.Fatal("retry is not idempotent", err)
				}
			} else if err == nil || strings.Contains(err.Error(), "private canary") {
				t.Fatal("unsafe seal replacement or leaked diagnostic", err)
			}
			if scenario == "write-failure" && !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
			var owner string
			if err := s.db.QueryRow("SELECT operation_ref FROM managed_state_seals WHERE state_id='state'").Scan(&owner); err != nil {
				t.Fatal(err)
			}
			want := old.Ref
			if scenario == "unpublished" {
				want = next.Ref
			}
			if owner != want {
				t.Fatalf("seal owner = %s, want %s", owner, want)
			}
		})
	}
}
