package sqlrs_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

// ES21-ES22 use the real record codec and manager, not a fake State cache.
type executionTestProvider struct {
	trace           *[]string
	fresh           runtimev2.CanonicalResolvedExtensionIdentity
	status          resolver.RevalidationStatus
	invalidEvidence bool
	acquires        int
}

func (*executionTestProvider) Descriptor() resolver.Descriptor {
	return resolver.Descriptor{Role: "execution_environment", Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1", SemanticVersion: "1"}
}
func (*executionTestProvider) IdentitySchema() runtimev2.ExtensionIdentitySchema {
	return sqlrs.ExecutionEnvironmentSchema()
}
func (p *executionTestProvider) Normalize(_ context.Context, _ resolver.Workspace, declaration runtimev2.ExtensionDeclaration) (resolver.NormalizedDeclaration, error) {
	return resolver.NormalizedDeclaration{Declaration: declaration}, nil
}
func (p *executionTestProvider) Resolve(context.Context, resolver.Workspace, resolver.NormalizedDeclaration) (resolver.Resolution, error) {
	*p.trace = append(*p.trace, "resolve")
	return resolver.Resolution{Identity: p.fresh, Evidence: json.RawMessage(`{"generation":1}`)}, nil
}
func (p *executionTestProvider) ValidateResolution(value resolver.Resolution) error {
	*p.trace = append(*p.trace, "validate")
	if _, err := sqlrs.ParseExecutionEnvironmentIdentity(value.Identity); err != nil {
		return err
	}
	var evidence struct {
		Generation int `json:"generation"`
	}
	if err := json.Unmarshal(value.Evidence, &evidence); err != nil || evidence.Generation < 1 {
		return errors.New("invalid evidence")
	}
	return nil
}
func (p *executionTestProvider) Revalidate(context.Context, resolver.Workspace, resolver.Resolution) (resolver.Revalidation, error) {
	*p.trace = append(*p.trace, "revalidate")
	evidence := json.RawMessage(`{"generation":2}`)
	if p.invalidEvidence {
		evidence = json.RawMessage(`{"generation":0}`)
	}
	return resolver.Revalidation{Status: p.status, Reason: "probe", Evidence: evidence}, nil
}
func (p *executionTestProvider) Acquire(context.Context, resolver.Workspace, resolver.Resolution) (resolver.Artifact, error) {
	p.acquires++
	return nil, errors.New("unexpected acquire")
}

type executionRecordCache struct {
	trace  *[]string
	raw    []byte
	stores []resolver.Resolution
}

func (c *executionRecordCache) Load(_ context.Context, key resolver.CacheKey, schema runtimev2.ExtensionIdentitySchema) (resolver.CacheLoad, error) {
	*c.trace = append(*c.trace, "load")
	if c.raw == nil {
		return resolver.CacheLoad{}, nil
	}
	record, err := resolver.DecodeCacheRecordJSON(c.raw, schema)
	if err != nil {
		return resolver.CacheLoad{}, err
	}
	if !record.Matches(key) {
		return resolver.CacheLoad{}, resolver.ErrIncompatibleCache
	}
	return resolver.CacheLoad{Hit: true, Resolution: record.Resolution()}, nil
}
func (c *executionRecordCache) Store(_ context.Context, key resolver.CacheKey, schema runtimev2.ExtensionIdentitySchema, value resolver.Resolution) error {
	*c.trace = append(*c.trace, "store")
	record, err := resolver.NewCacheRecord(key, schema, value)
	if err != nil {
		return err
	}
	c.raw, err = record.MarshalJSON()
	if err == nil {
		c.stores = append(c.stores, value)
	}
	return err
}

func TestExecutionResolverCacheOutcomeMatrix(t *testing.T) {
	valid := executionBuild(t, executionBase())
	mutant := executionMinimalFields()
	mutant["runtime"].(map[string]any)["identity_kind"] = "host-executable"
	invalid := executionGeneric(t, mutant)
	for _, tc := range []struct {
		name                   string
		cached, fresh          runtimev2.CanonicalResolvedExtensionIdentity
		status                 resolver.RevalidationStatus
		badEvidence, wantError bool
		wantTrace              []string
		wantHit                bool
		prior                  resolver.RevalidationStatus
		reason                 string
	}{
		{name: "miss-invalid", fresh: invalid, wantError: true, wantTrace: []string{"load", "resolve", "validate"}},
		{name: "invalid-valid", cached: invalid, fresh: valid, wantTrace: []string{"load", "validate", "resolve", "validate", "store"}, reason: "invalid_cached_resolution"},
		{name: "invalid-invalid", cached: invalid, fresh: invalid, wantError: true, wantTrace: []string{"load", "validate", "resolve", "validate"}},
		{name: "current", cached: valid, fresh: invalid, status: resolver.Current, wantTrace: []string{"load", "validate", "revalidate", "validate", "store"}, wantHit: true, prior: resolver.Current, reason: "probe"},
		{name: "current-bad-evidence", cached: valid, status: resolver.Current, badEvidence: true, wantError: true, wantTrace: []string{"load", "validate", "revalidate", "validate"}},
		{name: "stale", cached: valid, fresh: valid, status: resolver.Stale, wantTrace: []string{"load", "validate", "revalidate", "resolve", "validate", "store"}, wantHit: true, prior: resolver.Stale, reason: "probe"},
		{name: "unknown", cached: valid, fresh: valid, status: resolver.Unknown, wantTrace: []string{"load", "validate", "revalidate", "resolve", "validate", "store"}, wantHit: true, prior: resolver.Unknown, reason: "probe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var trace []string
			provider := &executionTestProvider{trace: &trace, fresh: tc.fresh, status: tc.status, invalidEvidence: tc.badEvidence}
			cache := &executionRecordCache{trace: &trace}
			declaration, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1"})
			if err != nil {
				t.Fatal(err)
			}
			workspace := resolver.Workspace{Root: t.TempDir()}
			if tc.cached.Provider() != "" {
				key, err := resolver.NewCacheKey(workspace, provider.Descriptor(), resolver.NormalizedDeclaration{Declaration: declaration})
				if err != nil {
					t.Fatal(err)
				}
				record, err := resolver.NewCacheRecord(key, provider.IdentitySchema(), resolver.Resolution{Identity: tc.cached, Evidence: json.RawMessage(`{"generation":1}`)})
				if err != nil {
					t.Fatal(err)
				}
				cache.raw, err = record.MarshalJSON()
				if err != nil {
					t.Fatal(err)
				}
			}
			registry, err := resolver.NewRegistry(provider)
			if err != nil {
				t.Fatal(err)
			}
			manager, err := resolver.NewManager(registry, cache)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := manager.ResolveCurrent(context.Background(), workspace, declaration)
			if !reflect.DeepEqual(trace, tc.wantTrace) || provider.acquires != 0 {
				t.Fatalf("trace=%v, want=%v, acquire=%d", trace, tc.wantTrace, provider.acquires)
			}
			if tc.wantError {
				var failure *resolver.Error
				if !errors.As(err, &failure) || failure.Operation != "validate_resolution" || failure.Code != resolver.CodeInvalidResolution || !reflect.DeepEqual(outcome, resolver.Outcome{}) || len(cache.stores) != 0 {
					t.Fatalf("failure outcome=%+v, err=%v", outcome, err)
				}
				if !tc.badEvidence {
					var validation *runtimev2.ValidationError
					if !errors.As(err, &validation) {
						t.Fatal("semantic rejection was replaced by an unrelated error")
					}
				}
				return
			}
			if err != nil || outcome.CacheHit != tc.wantHit || outcome.PriorStatus != tc.prior || outcome.Reason != tc.reason || len(cache.stores) != 1 {
				t.Fatalf("outcome=%+v, stores=%d, err=%v", outcome, len(cache.stores), err)
			}
			if executionFingerprint(t, outcome.Resolution.Identity) != executionFingerprint(t, valid) || executionFingerprint(t, cache.stores[0].Identity) != executionFingerprint(t, valid) {
				t.Fatal("returned or stored invalid/old identity")
			}
			if tc.status == resolver.Current && string(outcome.Resolution.Evidence) != `{"generation":2}` {
				t.Fatal("lost revalidated evidence")
			}
		})
	}
}

func TestExecutionResolverDirectoryCacheRestart(t *testing.T) {
	var trace []string
	provider := &executionTestProvider{trace: &trace, fresh: executionBuild(t, executionBase()), status: resolver.Current}
	registry, err := resolver.NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1"})
	if err != nil {
		t.Fatal(err)
	}
	root, workspace := t.TempDir(), resolver.Workspace{Root: t.TempDir()}
	for index := range 2 {
		cache, err := resolver.NewDirectoryCache(root)
		if err != nil {
			t.Fatal(err)
		}
		manager, err := resolver.NewManager(registry, cache)
		if err != nil {
			t.Fatal(err)
		}
		outcome, err := manager.ResolveCurrent(context.Background(), workspace, declaration)
		if err != nil || outcome.CacheHit != (index == 1) || executionFingerprint(t, outcome.Resolution.Identity) != executionFingerprint(t, provider.fresh) {
			t.Fatalf("restart=%d: %+v, %v", index, outcome, err)
		}
	}
}
