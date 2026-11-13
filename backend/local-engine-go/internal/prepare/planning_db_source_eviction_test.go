package prepare

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlrs/engine-local/internal/store"
)

// PG07: the coordinator must consult durable source ownership before removing
// files, and repeat the check even for a previously enumerated candidate.
// The metadata double is distinct from native source/DB identity evidence.
type planningDBSourceProtectionStore struct {
	fakeStore
	protected bool
	err       error
	reads     int
}

func (s *planningDBSourceProtectionStore) StateProtectedByPrepare(context.Context, string) (bool, error) {
	s.reads++
	return s.protected, s.err
}

func TestPlanningDBSourceEvictionProtection(t *testing.T) {
	for _, variant := range []string{"retained", "disposal-pending", "read-unavailable", "released"} {
		t.Run(variant, func(t *testing.T) {
			size := int64(20)
			st := &planningDBSourceProtectionStore{fakeStore: fakeStore{statesByID: map[string]store.StateEntry{"source": {StateID: "source", ImageID: "image", CreatedAt: "2025-01-01T00:00:00Z", SizeBytes: &size}}}, protected: variant != "released"}
			if variant == "read-unavailable" {
				st.err = errors.New("source ownership unavailable")
			}
			fs := &fakeStateFS{}
			m := newManagerWithDeps(t, st, newQueueStore(t), &testDeps{statefs: fs})
			candidates, blocked, _, err := m.listEvictionCandidates(t.Context(), capacitySettings{})
			if variant == "read-unavailable" {
				if !errors.Is(err, st.err) {
					t.Fatal("unavailable owner was treated as unprotected", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if variant == "released" {
				if len(candidates) != 1 || blocked != 0 {
					t.Fatal("released history permanently pinned source")
				}
			} else if len(candidates) != 0 || blocked != 1 {
				t.Fatal("live source was offered for eviction")
			}
			// Deliberately use a stale candidate to ensure the destructive path
			// checks again; list filtering alone does not establish ownership.
			err = m.deleteEvictionCandidate(t.Context(), evictCandidate{StateID: "source", ImageID: "image"})
			if variant == "released" {
				if err != nil || len(fs.removeCalls) != 1 || len(st.deletedStates) != 1 {
					t.Fatal("normal released source removal failed", err)
				}
			} else if err == nil || len(fs.removeCalls) != 0 || len(st.deletedStates) != 0 {
				t.Fatal("eviction touched a protected or unknown source", err)
			}
			if st.reads < 2 {
				t.Fatal("source protection was not independently checked at both boundaries")
			}
		})
	}
}
