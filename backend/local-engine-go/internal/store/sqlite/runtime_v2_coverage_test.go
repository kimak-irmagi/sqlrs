package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
	"github.com/sqlrs/engine-local/internal/runtimev2store"
)

func TestRuntimeV2PublicOperationsRejectInvalidInputs(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.PutRecipeLineage(ctx, runtimev2.RecipeLineage{}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("zero recipe=%v", err)
	}
	if err := store.PutRelativeLineage(ctx, runtimev2.RelativeLineage{}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("zero relative=%v", err)
	}
	if _, _, err := store.GetLogicalState(ctx, "bad"); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("bad state=%v", err)
	}
	if _, err := store.TraceLineage(ctx, runtimev2.StateID("sha256:"+strings.Repeat("a", 64))); !errors.Is(err, runtimev2store.ErrCorrupt) {
		t.Fatalf("missing trace=%v", err)
	}
	if _, err := store.ListProvenance(ctx, "bad", runtimev2store.ProvenancePageRequest{Limit: 1}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("bad provenance state=%v", err)
	}
	if _, err := store.ListProvenance(ctx, runtimev2.StateID("sha256:"+strings.Repeat("a", 64)), runtimev2store.ProvenancePageRequest{Limit: 0}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("bad provenance limit=%v", err)
	}
	if _, err := store.ListProvenance(ctx, runtimev2.StateID("sha256:"+strings.Repeat("a", 64)), runtimev2store.ProvenancePageRequest{Limit: 1}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("missing provenance state=%v", err)
	}
	if err := store.PutMaterialization(ctx, runtimev2store.Materialization{}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("zero materialization=%v", err)
	}
	if _, err := store.ListMaterializations(ctx, "bad", runtimev2store.MaterializationPageRequest{Limit: 1}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("bad material state=%v", err)
	}
	if _, err := store.ListMaterializations(ctx, runtimev2.StateID("sha256:"+strings.Repeat("a", 64)), runtimev2store.MaterializationPageRequest{Limit: 101}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("bad material limit=%v", err)
	}
	if _, err := store.ListMaterializations(ctx, runtimev2.StateID("sha256:"+strings.Repeat("a", 64)), runtimev2store.MaterializationPageRequest{Limit: 1}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("missing material state=%v", err)
	}
	key, _ := runtimeV2CacheFixture(t, t.TempDir(), "1")
	if err := store.Store(ctx, key, runtimeV2TestSchema(), resolver.Resolution{}); !errors.Is(err, resolver.ErrInvalidDeclaration) {
		t.Fatalf("invalid cache resolution=%v", err)
	}
}

func TestRuntimeV2InternalMaterialLookupMiss(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	value, found, err := loadRuntimeV2Materialization(context.Background(), store.db, "missing")
	if err != nil || found || value.RecordVersion() != "" {
		t.Fatalf("lookup=%+v %v %v", value, found, err)
	}
}

func TestRuntimeV2PostBeginQueryFailures(t *testing.T) {
	t.Run("material state query", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		value := materializationForState(t, "sha256:"+strings.Repeat("a", 64), "one", time.Now())
		if _, err := store.db.Exec(`DROP TABLE runtime_v2_states`); err != nil {
			t.Fatal(err)
		}
		if err := store.PutMaterialization(context.Background(), value); err == nil {
			t.Fatal("state query failure hidden")
		}
	})
	t.Run("material list state query", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		stateID := runtimev2.StateID("sha256:" + strings.Repeat("a", 64))
		if _, err := store.db.Exec(`DROP TABLE runtime_v2_states`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListMaterializations(context.Background(), stateID, runtimev2store.MaterializationPageRequest{Limit: 1}); err == nil {
			t.Fatal("list state query failure hidden")
		}
	})
	t.Run("provenance rows query", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		lineage := runtimeV2Recipe(t, "1", 0)
		if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`DROP TABLE runtime_v2_provenance_observations`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListProvenance(context.Background(), lineage.Root().ID(), runtimev2store.ProvenancePageRequest{Limit: 1}); err == nil {
			t.Fatal("provenance query failure hidden")
		}
	})
	t.Run("material rows query", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		lineage := runtimeV2Recipe(t, "1", 0)
		if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`DROP TABLE runtime_v2_materializations`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListMaterializations(context.Background(), lineage.Root().ID(), runtimev2store.MaterializationPageRequest{Limit: 1}); err == nil {
			t.Fatal("material query failure hidden")
		}
	})
	t.Run("material lookup closed", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadRuntimeV2Materialization(context.Background(), db, "one"); err == nil {
			t.Fatal("lookup database error hidden")
		}
	})
}

func TestRuntimeV2CorruptDuplicateStateVersionFails(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lineage := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints=ON; DROP TRIGGER runtime_v2_states_immutable; UPDATE runtime_v2_states SET record_version='future'`); err != nil {
		t.Fatal(err)
	}
	if err := store.PutRecipeLineage(context.Background(), lineage); !errors.Is(err, runtimev2store.ErrCorrupt) {
		t.Fatalf("corrupt duplicate state=%v", err)
	}
}

func TestRuntimeV2InjectedWriteFailuresRollback(t *testing.T) {
	t.Run("recipe state", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if _, err := store.db.Exec(`CREATE TRIGGER fail_state BEFORE INSERT ON runtime_v2_states BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
			t.Fatal(err)
		}
		if err := store.PutRecipeLineage(context.Background(), runtimeV2Recipe(t, "1", 0)); err == nil {
			t.Fatal("injected state failure hidden")
		}
	})
	t.Run("recipe provenance", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if _, err := store.db.Exec(`CREATE TRIGGER fail_provenance BEFORE INSERT ON runtime_v2_provenance_observations BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
			t.Fatal(err)
		}
		if err := store.PutRecipeLineage(context.Background(), runtimeV2Recipe(t, "1", 0)); err == nil {
			t.Fatal("injected provenance failure hidden")
		}
		var count int
		if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_states`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed write left state: %d %v", count, err)
		}
	})
	t.Run("relative anchor query", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		lineage := runtimeV2Recipe(t, "1", 1)
		relative, _ := runtimev2.Extend(lineage.Root().ID(), []runtimev2.TransformProvenance{lineage.Steps()[0].Transform()})
		if _, err := store.db.Exec(`DROP TABLE runtime_v2_states`); err != nil {
			t.Fatal(err)
		}
		if err := store.PutRelativeLineage(context.Background(), relative); err == nil {
			t.Fatal("anchor query failure hidden")
		}
	})
	t.Run("relative step", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		base := runtimeV2Recipe(t, "1", 0)
		if err := store.PutRecipeLineage(context.Background(), base); err != nil {
			t.Fatal(err)
		}
		lineage := runtimeV2Recipe(t, "1", 1)
		relative, _ := runtimev2.Extend(base.Root().ID(), []runtimev2.TransformProvenance{lineage.Steps()[0].Transform()})
		if _, err := store.db.Exec(`CREATE TRIGGER fail_derived BEFORE INSERT ON runtime_v2_states WHEN NEW.state_kind='derived' BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
			t.Fatal(err)
		}
		if err := store.PutRelativeLineage(context.Background(), relative); err == nil {
			t.Fatal("relative step failure hidden")
		}
	})
	t.Run("material parent", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		lineage := runtimeV2Recipe(t, "1", 0)
		if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
			t.Fatal(err)
		}
		value := materializationForState(t, string(lineage.Root().ID()), "one", time.Now())
		if _, err := store.db.Exec(`CREATE TRIGGER fail_material BEFORE INSERT ON runtime_v2_materializations BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
			t.Fatal(err)
		}
		if err := store.PutMaterialization(context.Background(), value); err == nil {
			t.Fatal("material failure hidden")
		}
	})
	t.Run("material component", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		lineage := runtimeV2Recipe(t, "1", 0)
		if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
			t.Fatal(err)
		}
		value := materializationForState(t, string(lineage.Root().ID()), "one", time.Now())
		if _, err := store.db.Exec(`CREATE TRIGGER fail_component BEFORE INSERT ON runtime_v2_materialization_components BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
			t.Fatal(err)
		}
		if err := store.PutMaterialization(context.Background(), value); err == nil {
			t.Fatal("component failure hidden")
		}
		var count int
		if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_materializations`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed component left parent: %d %v", count, err)
		}
	})
}

func TestRuntimeV2DigestCollisionAndCorruptDuplicateFailClosed(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := runtimeV2Recipe(t, "1", 0)
	second := runtimeV2Recipe(t, "2", 0)
	if err := store.PutRecipeLineage(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	secondRaw, err := json.Marshal(second.Factory())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(secondRaw)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if _, err := store.db.Exec(`DROP TRIGGER runtime_v2_provenance_immutable; UPDATE runtime_v2_provenance_observations SET observation_digest=?`, digest); err != nil {
		t.Fatal(err)
	}
	if err := store.PutRecipeLineage(context.Background(), second); !errors.Is(err, runtimev2store.ErrCorrupt) {
		t.Fatalf("digest collision=%v", err)
	}
	value := materializationForState(t, string(first.Root().ID()), "one", time.Now())
	if err := store.PutMaterialization(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints=ON; DROP TRIGGER runtime_v2_materializations_immutable; UPDATE runtime_v2_materializations SET record_version='future'`); err != nil {
		t.Fatal(err)
	}
	if err := store.PutMaterialization(context.Background(), value); !errors.Is(err, runtimev2store.ErrCorrupt) {
		t.Fatalf("corrupt duplicate=%v", err)
	}
}

func TestRuntimeV2MaterializationOptionalFieldsRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lineage := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	runtimeID, jobID, size := "runtime", "job", int64(99)
	value, err := runtimev2store.NewMaterialization(runtimev2store.MaterializationInput{StateID: lineage.Root().ID(), MaterializationID: "optional", Backend: "local.snapshot", RuntimeID: &runtimeID, JobID: &jobID, CreatedAt: time.Now(), SizeBytes: &size})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutMaterialization(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListMaterializations(context.Background(), lineage.Root().ID(), runtimev2store.MaterializationPageRequest{Limit: 1})
	if err != nil || len(page.Values()) != 1 {
		t.Fatalf("page=%+v %v", page.Values(), err)
	}
	got := page.Values()[0]
	if got.RuntimeID() == nil || *got.RuntimeID() != runtimeID || got.JobID() == nil || *got.JobID() != jobID || got.SizeBytes() == nil || *got.SizeBytes() != size {
		t.Fatal("optional fields lost")
	}
}

func TestRuntimeV2OperationsPropagateClosedDatabaseErrors(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	lineage := runtimeV2Recipe(t, "1", 1)
	key, resolution := runtimeV2CacheFixture(t, t.TempDir(), "1")
	material := materializationForState(t, string(lineage.Root().ID()), "one", time.Now())
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	checks := []struct {
		name string
		run  func() error
	}{
		{"recipe", func() error { return store.PutRecipeLineage(ctx, lineage) }},
		{"relative", func() error {
			relative, _ := runtimev2.Extend(lineage.Root().ID(), []runtimev2.TransformProvenance{lineage.Steps()[0].Transform()})
			return store.PutRelativeLineage(ctx, relative)
		}},
		{"state", func() error { _, _, e := store.GetLogicalState(ctx, lineage.Root().ID()); return e }},
		{"trace", func() error { _, e := store.TraceLineage(ctx, lineage.Root().ID()); return e }},
		{"provenance", func() error {
			_, e := store.ListProvenance(ctx, lineage.Root().ID(), runtimev2store.ProvenancePageRequest{Limit: 1})
			return e
		}},
		{"put material", func() error { return store.PutMaterialization(ctx, material) }},
		{"list material", func() error {
			_, e := store.ListMaterializations(ctx, lineage.Root().ID(), runtimev2store.MaterializationPageRequest{Limit: 1})
			return e
		}},
		{"load cache", func() error { _, e := store.Load(ctx, key, runtimeV2TestSchema()); return e }},
		{"store cache", func() error { return store.Store(ctx, key, runtimeV2TestSchema(), resolution) }},
		{"classify", func() error { _, e := store.ClassifyState(ctx, "legacy"); return e }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err == nil {
				t.Fatal("closed database error was hidden")
			}
		})
	}
	if err := EnsureRuntimeV2Schema(ctx, db); err == nil {
		t.Fatal("closed database schema ensure succeeded")
	}
}

func TestRuntimeV2CursorStateBindingAtSQLiteBoundary(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	first := runtimeV2Recipe(t, "1", 0)
	secondIdentity, err := runtimev2.NewFactoryIdentity(runtimev2.FactoryIdentityInput{SchemaVersion: runtimev2.SchemaVersion, Provider: "test", Kind: "other", IdentitySchema: "test.other.v1", Fields: []runtimev2.ResolvedField{}})
	if err != nil {
		t.Fatal(err)
	}
	secondProvenance, _ := runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{Identity: secondIdentity})
	secondRecipe, _ := runtimev2.NewRecipe(secondProvenance, nil)
	second, _ := runtimev2.Build(secondRecipe)
	if err := store.PutRecipeLineage(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.PutRecipeLineage(ctx, second); err != nil {
		t.Fatal(err)
	}
	pc, _ := runtimev2store.NewProvenanceCursor(first.Root().ID(), 1, 0)
	if _, err := store.ListProvenance(ctx, second.Root().ID(), runtimev2store.ProvenancePageRequest{Limit: 1, Cursor: &pc}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("cross-state provenance=%v", err)
	}
	mc, _ := runtimev2store.NewMaterializationCursor(first.Root().ID(), 1, 0)
	if _, err := store.ListMaterializations(ctx, second.Root().ID(), runtimev2store.MaterializationPageRequest{Limit: 1, Cursor: &mc}); !errors.Is(err, runtimev2store.ErrInvalid) {
		t.Fatalf("cross-state material=%v", err)
	}
}

func TestRuntimeV2DerivedProvenancePage(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lineage := runtimeV2Recipe(t, "1", 1)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListProvenance(context.Background(), lineage.Endpoint().ID(), runtimev2store.ProvenancePageRequest{Limit: 1})
	if err != nil || len(page.Values()) != 1 {
		t.Fatalf("page=%+v %v", page.Values(), err)
	}
	if _, ok := page.Values()[0].Transform(); !ok {
		t.Fatal("derived provenance did not decode as transform")
	}
}

func TestRuntimeV2SchemaNilAndMissingSourceGuards(t *testing.T) {
	if err := InstallRuntimeV2Schema(context.Background(), nil); !errors.Is(err, ErrRuntimeV2Schema) {
		t.Fatalf("nil tx=%v", err)
	}
	if err := EnsureRuntimeV2Schema(context.Background(), nil); !errors.Is(err, ErrRuntimeV2Schema) {
		t.Fatalf("nil db=%v", err)
	}
	previous := schemaSQL
	schemaSQL = "legacy only"
	t.Cleanup(func() { schemaSQL = previous })
	if RuntimeV2SchemaSQL() != "" {
		t.Fatal("missing delimiters produced DDL")
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := InstallRuntimeV2Schema(context.Background(), tx); !errors.Is(err, ErrRuntimeV2Schema) {
		t.Fatalf("missing source install=%v", err)
	}
}

func TestRuntimeV2SchemaInjectedDDLMarkerAndExpectedManifestFailures(t *testing.T) {
	for name, body := range map[string]string{
		"ddl":    runtimeV2SchemaBegin + "\nCREATE TABLE broken(\n" + runtimeV2SchemaEnd,
		"marker": runtimeV2SchemaBegin + "\nCREATE TABLE runtime_v2_store_format(slot INTEGER PRIMARY KEY,format_version TEXT,semantic_version TEXT);\nCREATE TRIGGER reject_marker BEFORE INSERT ON runtime_v2_store_format BEGIN SELECT RAISE(ABORT,'injected'); END;\n" + runtimeV2SchemaEnd,
	} {
		t.Run(name, func(t *testing.T) {
			previous := schemaSQL
			schemaSQL = body
			t.Cleanup(func() { schemaSQL = previous })
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err := InstallRuntimeV2Schema(context.Background(), tx); err == nil {
				t.Fatal("injected schema failure hidden")
			}
		})
	}
	t.Run("expected manifest", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := EnsureRuntimeV2Schema(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		previous := schemaSQL
		schemaSQL = runtimeV2SchemaBegin + "\nCREATE TABLE broken(\n" + runtimeV2SchemaEnd
		t.Cleanup(func() { schemaSQL = previous })
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err := InstallRuntimeV2Schema(context.Background(), tx); !errors.Is(err, ErrRuntimeV2Schema) {
			t.Fatalf("expected manifest error=%v", err)
		}
	})
	t.Run("extra marker row", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := EnsureRuntimeV2Schema(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON; INSERT INTO runtime_v2_store_format(slot,format_version,semantic_version) VALUES(2,'sqlrs.runtime-persistence.v1','sqlrs.runtime.v2')`); err != nil {
			t.Fatal(err)
		}
		if err := EnsureRuntimeV2Schema(context.Background(), db); !errors.Is(err, ErrRuntimeV2Schema) {
			t.Fatalf("extra marker=%v", err)
		}
	})
}

func TestRuntimeV2RelativeLineageRejectsCorruptAnchor(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	extended := runtimeV2Recipe(t, "1", 1)
	relative, err := runtimev2.Extend(base.Root().ID(), []runtimev2.TransformProvenance{extended.Steps()[0].Transform()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TRIGGER runtime_v2_states_immutable; UPDATE runtime_v2_states SET state_json='{}'`); err != nil {
		t.Fatal(err)
	}
	if err := store.PutRelativeLineage(context.Background(), relative); !errors.Is(err, runtimev2store.ErrCorrupt) {
		t.Fatalf("error = %v, want %v", err, runtimev2store.ErrCorrupt)
	}
}

func TestRuntimeV2ClassificationRejectsUnknownRecordVersion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lineage := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints=ON; DROP TRIGGER runtime_v2_states_immutable; UPDATE runtime_v2_states SET record_version='future'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClassifyState(context.Background(), string(lineage.Root().ID())); !errors.Is(err, runtimev2store.ErrCorrupt) {
		t.Fatalf("error = %v, want %v", err, runtimev2store.ErrCorrupt)
	}
}

func TestRuntimeV2ProvenanceRejectsNoncanonicalJSONWithValidDigest(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lineage := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	var canonical []byte
	if err := store.db.QueryRow(`SELECT provenance_json FROM runtime_v2_provenance_observations`).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	noncanonical := append([]byte{'{', ' '}, canonical[1:]...)
	digestBytes := sha256.Sum256(noncanonical)
	digest := "sha256:" + hex.EncodeToString(digestBytes[:])
	if _, err := store.db.Exec(`DROP TRIGGER runtime_v2_provenance_immutable; UPDATE runtime_v2_provenance_observations SET provenance_json=?,observation_digest=?`, noncanonical, digest); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListProvenance(context.Background(), lineage.Root().ID(), runtimev2store.ProvenancePageRequest{Limit: 10})
	if !errors.Is(err, runtimev2store.ErrCorrupt) || len(page.Values()) != 0 {
		t.Fatalf("page=%+v error=%v", page.Values(), err)
	}
}
