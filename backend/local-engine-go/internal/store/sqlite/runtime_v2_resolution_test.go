package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
)

func runtimeV2CacheFixture(t *testing.T, workspace string, version string) (resolver.CacheKey, resolver.Resolution) {
	t.Helper()
	declaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind",
		SpecificationSchema: "owner.kind.v1", Fields: []runtimev2.DeclarationField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := resolver.NewCacheKey(resolver.Workspace{Root: workspace}, resolver.Descriptor{
		Role: "input", Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", SemanticVersion: version,
	}, resolver.NormalizedDeclaration{Declaration: declaration})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind",
		IdentitySchema: "owner.kind.v1", Fields: []runtimev2.ResolvedField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return key, resolver.Resolution{Identity: identity, Evidence: json.RawMessage(`{"generation":1}`)}
}

func TestRuntimeV2ResolutionCacheMissStoreLoadRestartAndReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	workspace := t.TempDir()
	key, resolution := runtimeV2CacheFixture(t, workspace, "1")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC) }
	if loaded, err := store.Load(context.Background(), key); err != nil || loaded.Hit {
		t.Fatalf("miss = %+v %v", loaded, err)
	}
	if err := store.Store(context.Background(), key, resolution); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.Load(context.Background(), key)
	if err != nil || !loaded.Hit || string(loaded.Resolution.Evidence) != `{"generation":1}` {
		t.Fatalf("load = %+v %v", loaded, err)
	}
	replacement := resolution
	replacement.Evidence = json.RawMessage(`{"generation":2}`)
	if err := store.Store(context.Background(), key, replacement); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(context.Background(), key)
	if err != nil || string(loaded.Resolution.Evidence) != `{"generation":2}` {
		t.Fatalf("replacement = %+v %v", loaded, err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_resolutions WHERE cache_key=?`, key.String()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("row count = %d %v", count, err)
	}
}

func TestRuntimeV2ResolutionCacheRejectsIndexedMismatchAndBadEnvelope(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	workspace := t.TempDir()
	keyA, resolution := runtimeV2CacheFixture(t, workspace, "1")
	keyB, _ := runtimeV2CacheFixture(t, workspace, "2")
	record, err := resolver.NewCacheRecord(keyA, resolution)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := record.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO runtime_v2_resolutions(cache_key,record_version,cache_record_json,stored_at) VALUES(?,?,?,?)`, keyB.String(), RuntimeV2RecordVersion, raw, "2026-01-01T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), keyB); !errors.Is(err, resolver.ErrCorruptCache) {
		t.Fatalf("indexed mismatch = %v", err)
	}
	if _, err := store.db.Exec(`UPDATE runtime_v2_resolutions SET cache_record_json=? WHERE cache_key=?`, []byte(`{"schema_version":"future"}`), keyB.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), keyB); !errors.Is(err, resolver.ErrIncompatibleCache) {
		t.Fatalf("incompatible envelope = %v", err)
	}
}

func TestRuntimeV2ResolutionCacheHonorsCancellation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key, resolution := runtimeV2CacheFixture(t, t.TempDir(), "1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Load(ctx, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("load cancellation = %v", err)
	}
	if err := store.Store(ctx, key, resolution); !errors.Is(err, context.Canceled) {
		t.Fatalf("store cancellation = %v", err)
	}
}

func TestRuntimeV2ResolutionCacheRejectsRowVersionAndTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		mutation string
		want     error
	}{
		{"record version", `PRAGMA ignore_check_constraints=ON; UPDATE runtime_v2_resolutions SET record_version='future'`, resolver.ErrIncompatibleCache},
		{"timestamp", `UPDATE runtime_v2_resolutions SET stored_at='2026-01-01T00:00:00Z'`, resolver.ErrCorruptCache},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "store.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			key, resolution := runtimeV2CacheFixture(t, t.TempDir(), "1")
			if err := store.Store(context.Background(), key, resolution); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.Exec(test.mutation); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Load(context.Background(), key); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
