package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func TestRuntimeV2LineageConflictRollsBackWholeObservationGeneration(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	lineage := runtimeV2Recipe(t, "2", 1)
	step := lineage.Steps()[0]
	if _, err := store.db.Exec(`INSERT INTO runtime_v2_states(state_id,record_version,state_kind,parent_state_id,state_json,resolved_identity_json,stored_at) VALUES(?,?,?,?,?,?,?)`, step.State().ID(), RuntimeV2RecordVersion, "derived", base.Root().ID(), `{}`, `{}`, "2026-01-01T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_provenance_observations WHERE state_id=?`, base.Root().ID()).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := store.PutRecipeLineage(context.Background(), lineage); err == nil {
		t.Fatal("conflicting lineage accepted")
	}
	var after int
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_provenance_observations WHERE state_id=?`, base.Root().ID()).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("failed generation left root observation: before=%d after=%d", before, after)
	}
}

func TestRuntimeV2ConcurrentIdempotentWritersConverge(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	lineages := []runtimev2.RecipeLineage{runtimeV2Recipe(t, "1", 2), runtimeV2Recipe(t, "2", 2)}
	var wait sync.WaitGroup
	errorsCh := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			lineage := lineages[index%2]
			errorsCh <- store.PutRecipeLineage(context.Background(), lineage)
		}(i)
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var states, observations int
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_states`).Scan(&states); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_provenance_observations`).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if states != 3 || observations != 6 {
		t.Fatalf("converged counts states=%d observations=%d", states, observations)
	}
}
