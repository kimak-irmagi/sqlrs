package sqlite

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/managedidentity"
)

func lineageCandidate(t *testing.T, n byte) managedidentity.Record {
	t.Helper()
	binding, err := managedidentity.Generate(bytes.NewReader(bytes.Repeat([]byte{n}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	return managedidentity.Record{Selector: managedidentity.BaseSelector{DomainRef: "engine-store-1", EngineKind: "postgres", ImageDigest: "sha256:" + strings.Repeat("a", 64), InitSpecDigest: strings.Repeat("b", 64), PolicyVersion: managedidentity.PolicyVersion}, Binding: binding, CreatedAt: time.Now().UTC()}
}

func TestLineageDurableConcurrentReservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// Install only on this disposable empty fixture. Startup must not enable the
	// new schema until the separately implemented upgrade preflight succeeds.
	if _, err := st.db.Exec(ManagedLineageSchemaSQL()); err != nil {
		t.Fatal(err)
	}
	candidates := make([]managedidentity.Record, 16)
	for i := range candidates {
		candidates[i] = lineageCandidate(t, byte(i+1))
	}
	var group sync.WaitGroup
	result := make(chan managedidentity.Record, len(candidates))
	failures := make(chan error, len(candidates))
	for _, candidate := range candidates {
		group.Add(1)
		go func(c managedidentity.Record) {
			defer group.Done()
			r, e := st.Reserve(context.Background(), c)
			result <- r
			failures <- e
		}(candidate)
	}
	group.Wait()
	close(result)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	winner, err := st.Find(context.Background(), candidates[0].Selector)
	if err != nil {
		t.Fatal(err)
	}
	for r := range result {
		if r != winner {
			t.Fatal("multiple committed identities")
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Get(context.Background(), winner.Selector.DomainRef, winner.Binding.LineageRef)
	if err != nil || got != winner {
		t.Fatalf("restart: %+v %v", got, err)
	}
	if _, err := reopened.Get(context.Background(), "foreign", winner.Binding.LineageRef); !errors.Is(err, managedidentity.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestLineageRejectsMissingCorruptAndClosedStorage(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	candidate := lineageCandidate(t, 1)
	ctx := context.Background()
	if _, err := st.Find(ctx, candidate.Selector); !errors.Is(err, managedidentity.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(ManagedLineageSchemaSQL()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Find(ctx, candidate.Selector); !errors.Is(err, managedidentity.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.Reserve(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec("UPDATE managed_base_lineages SET identity_digest = ?", strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Find(ctx, candidate.Selector); !errors.Is(err, managedidentity.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := st.Reserve(ctx, lineageCandidate(t, 2)); !errors.Is(err, managedidentity.ErrInvalid) {
		t.Fatal("corrupt winner replaced", err)
	}
	if _, err := st.db.Exec("UPDATE managed_base_lineages SET identity_digest = ?, created_at = ?", candidate.Binding.IdentityDigest, "not-a-time"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, candidate.Selector.DomainRef, candidate.Binding.LineageRef); !errors.Is(err, managedidentity.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Reserve(ctx, candidate); !errors.Is(err, managedidentity.ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestLineageRejectsInvalidInputAndHonorsCancellation(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if _, err := st.Reserve(ctx, managedidentity.Record{}); !errors.Is(err, managedidentity.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := st.Find(ctx, managedidentity.BaseSelector{}); !errors.Is(err, managedidentity.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, "", "ref"); !errors.Is(err, managedidentity.ErrInvalid) {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := st.Reserve(canceled, lineageCandidate(t, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := st.Find(canceled, lineageCandidate(t, 1).Selector); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
