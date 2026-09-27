package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/runtimev2store"
)

func TestRuntimeV2StateStrictReconstructionRejectsCorruption(t *testing.T) {
	cases := map[string]struct {
		transforms int
		mutation   string
	}{
		"record version":                {0, `UPDATE runtime_v2_states SET record_version='future'`},
		"state kind":                    {0, `UPDATE runtime_v2_states SET state_kind='wrong'`},
		"state json":                    {0, `UPDATE runtime_v2_states SET state_json='{}'`},
		"noncanonical state":            {0, `UPDATE runtime_v2_states SET state_json=replace(state_json,'{"schema_version"','{ "schema_version"')`},
		"resolved identity":             {0, `UPDATE runtime_v2_states SET resolved_identity_json='{}'`},
		"noncanonical factory identity": {0, `UPDATE runtime_v2_states SET resolved_identity_json=replace(resolved_identity_json,'{"schema_version"','{ "schema_version"')`},
		"timestamp":                     {0, `UPDATE runtime_v2_states SET stored_at='2026-01-01T00:00:00Z'`},
		"parent copy":                   {1, `UPDATE runtime_v2_states SET parent_state_id='sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' WHERE state_kind='derived'`},
		"derived identity":              {1, `UPDATE runtime_v2_states SET resolved_identity_json='{}' WHERE state_kind='derived'`},
		"noncanonical derived identity": {1, `UPDATE runtime_v2_states SET resolved_identity_json=replace(resolved_identity_json,'{"schema_version"','{ "schema_version"') WHERE state_kind='derived'`},
		"derived as factory":            {1, `UPDATE runtime_v2_states SET state_kind='factory',parent_state_id=NULL WHERE state_kind='derived'`},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "store.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			lineage := runtimeV2Recipe(t, "1", test.transforms)
			if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
				t.Fatal(err)
			}
			// Explicitly test-only corruption path: disable constraints and remove the
			// immutable trigger before mutating this disposable database.
			if _, err := store.db.Exec(`PRAGMA foreign_keys=OFF; PRAGMA ignore_check_constraints=ON; DROP TRIGGER runtime_v2_states_immutable; ` + test.mutation); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.GetLogicalState(context.Background(), lineage.Endpoint().ID()); !errors.Is(err, runtimev2store.ErrCorrupt) {
				t.Fatalf("error = %v, want %v", err, runtimev2store.ErrCorrupt)
			}
		})
	}
}

func TestRuntimeV2ProvenancePageRejectsCorruptItem(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lineage := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints=ON; DROP TRIGGER runtime_v2_provenance_immutable; UPDATE runtime_v2_provenance_observations SET observation_digest='sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'`); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListProvenance(context.Background(), lineage.Root().ID(), runtimev2store.ProvenancePageRequest{Limit: 10})
	if !errors.Is(err, runtimev2store.ErrCorrupt) || len(page.Values()) != 0 {
		t.Fatalf("page=%+v error=%v", page.Values(), err)
	}
}

func TestRuntimeV2ProvenancePageRejectsVersionAndIdentityMismatch(t *testing.T) {
	for _, test := range []struct {
		name, mutation string
		endpoint       bool
	}{{"version", `UPDATE runtime_v2_provenance_observations SET record_version='future'`, false}, {"factory identity", `UPDATE runtime_v2_provenance_observations SET provenance_json=replace(provenance_json,'postgres','changed')`, false}, {"transform malformed", `UPDATE runtime_v2_provenance_observations SET provenance_json='{}' WHERE state_id=(SELECT state_id FROM runtime_v2_states WHERE state_kind='derived')`, true}} {
		t.Run(test.name, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "store.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			lineage := runtimeV2Recipe(t, "1", 1)
			if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.Exec(`PRAGMA ignore_check_constraints=ON; DROP TRIGGER runtime_v2_provenance_immutable; ` + test.mutation); err != nil {
				t.Fatal(err)
			}
			stateID := lineage.Root().ID()
			if test.endpoint {
				stateID = lineage.Endpoint().ID()
			}
			page, err := store.ListProvenance(context.Background(), stateID, runtimev2store.ProvenancePageRequest{Limit: 10})
			if !errors.Is(err, runtimev2store.ErrCorrupt) || len(page.Values()) != 0 {
				t.Fatalf("page=%+v error=%v", page.Values(), err)
			}
		})
	}
}

func TestRuntimeV2MaterializationPageRejectsCorruptRecords(t *testing.T) {
	cases := map[string]string{
		"record version":         `DROP TRIGGER runtime_v2_materializations_immutable; UPDATE runtime_v2_materializations SET record_version='future'`,
		"metadata":               `DROP TRIGGER runtime_v2_materializations_immutable; UPDATE runtime_v2_materializations SET metadata_json='[]'`,
		"noncanonical metadata":  `DROP TRIGGER runtime_v2_materializations_immutable; UPDATE runtime_v2_materializations SET metadata_json='{"purpose": "test"}'`,
		"timestamp":              `DROP TRIGGER runtime_v2_materializations_immutable; UPDATE runtime_v2_materializations SET created_at='not-a-time'`,
		"component":              `DROP TRIGGER runtime_v2_materialization_components_immutable; UPDATE runtime_v2_materialization_components SET metadata_json='null'`,
		"noncanonical component": `DROP TRIGGER runtime_v2_materialization_components_immutable; UPDATE runtime_v2_materialization_components SET metadata_json='{ }'`,
		"component version":      `DROP TRIGGER runtime_v2_materialization_components_immutable; UPDATE runtime_v2_materialization_components SET record_version='future'`,
	}
	for name, mutation := range cases {
		t.Run(name, func(t *testing.T) {
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
			if err := store.PutMaterialization(context.Background(), value); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.Exec(`PRAGMA ignore_check_constraints=ON; ` + mutation); err != nil {
				t.Fatal(err)
			}
			page, err := store.ListMaterializations(context.Background(), lineage.Root().ID(), runtimev2store.MaterializationPageRequest{Limit: 10})
			if !errors.Is(err, runtimev2store.ErrCorrupt) || len(page.Values()) != 0 {
				t.Fatalf("page=%+v error=%v", page.Values(), err)
			}
		})
	}
}
