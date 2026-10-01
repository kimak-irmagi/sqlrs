package resolver_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

// CR01-CR03, CR08: a clean package-external provider uses only the one
// canonical resolver/registry/cache API. Requirements:
// docs/architecture/runtime-v2-canonical-resolver-tests.md.
type canonicalProvider struct {
	descriptor                                                  resolver.Descriptor
	schema                                                      runtimev2.ExtensionIdentitySchema
	identity                                                    runtimev2.CanonicalResolvedExtensionIdentity
	normalizeCalls, resolveCalls, revalidateCalls, acquireCalls int
}

func (p *canonicalProvider) Descriptor() resolver.Descriptor                   { return p.descriptor }
func (p *canonicalProvider) IdentitySchema() runtimev2.ExtensionIdentitySchema { return p.schema }
func (p *canonicalProvider) Normalize(_ context.Context, _ resolver.Workspace, input runtimev2.ExtensionDeclaration) (resolver.NormalizedDeclaration, error) {
	p.normalizeCalls++
	fields := input.Fields()
	if len(fields) != 1 || fields[0].Name != "ref" {
		return resolver.NormalizedDeclaration{}, resolver.ErrInvalidDeclaration
	}
	value, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: input.Owner(), Kind: input.Kind(), SpecificationSchema: input.SpecificationSchema(),
		Fields: []runtimev2.DeclarationField{{Name: "ref", Value: strings.ToLower(strings.TrimSpace(fields[0].Value))}},
	})
	return resolver.NormalizedDeclaration{Declaration: value}, err
}
func (p *canonicalProvider) Resolve(context.Context, resolver.Workspace, resolver.NormalizedDeclaration) (resolver.Resolution, error) {
	p.resolveCalls++
	return resolver.Resolution{Identity: p.identity, Evidence: json.RawMessage(`{"generation":1}`)}, nil
}
func (p *canonicalProvider) ValidateResolution(value resolver.Resolution) error {
	if value.Identity.Provider() != p.schema.Provider() || value.Identity.Kind() != p.schema.SemanticKind() || value.Identity.IdentitySchema() != p.schema.IdentitySchema() || !json.Valid(value.Evidence) {
		return resolver.ErrInvalidDeclaration
	}
	return nil
}
func (p *canonicalProvider) Revalidate(context.Context, resolver.Workspace, resolver.Resolution) (resolver.Revalidation, error) {
	p.revalidateCalls++
	return resolver.Revalidation{Status: resolver.Current, Reason: "stable", Evidence: json.RawMessage(`{"generation":2}`)}, nil
}
func (p *canonicalProvider) Acquire(context.Context, resolver.Workspace, resolver.Resolution) (resolver.Artifact, error) {
	p.acquireCalls++
	return nil, errors.New("not expected")
}

func TestCanonicalProvidersShareOneManagerAndRestartCache(t *testing.T) {
	textField, _ := runtimev2.NewTextIdentityField("digest", "sha256:abc")
	structured, _ := runtimev2.CanonicalMap([]runtimev2.CanonicalMapEntry{{Key: "tag", Value: runtimev2.CanonicalNull()}})
	valueField, _ := runtimev2.NewCanonicalValueIdentityField("tree", structured)
	secret, _ := runtimev2.NewSecretReference("vault", "item", "3")
	secretField, _ := runtimev2.NewSecretReferenceIdentityField("credential", secret)
	cases := []struct {
		name        string
		fields      []runtimev2.IdentityField
		disclosures []schemaauthor.DisclosureClass
	}{
		{"file", []runtimev2.IdentityField{textField}, []schemaauthor.DisclosureClass{schemaauthor.DisclosurePublic}},
		{"oci", []runtimev2.IdentityField{textField, valueField}, []schemaauthor.DisclosureClass{schemaauthor.DisclosurePublic, schemaauthor.DisclosureProtected}},
		{"git", []runtimev2.IdentityField{valueField, secretField}, []schemaauthor.DisclosureClass{schemaauthor.DisclosurePublic, schemaauthor.DisclosureProtected}},
		{"package", []runtimev2.IdentityField{secretField, textField}, []schemaauthor.DisclosureClass{schemaauthor.DisclosureProtected, schemaauthor.DisclosurePublic}},
	}
	providers := make([]*canonicalProvider, len(cases))
	registered := make([]resolver.Resolver, len(cases))
	declarations := make([]runtimev2.InputDeclaration, len(cases))
	for index, test := range cases {
		definitions := make([]schemaauthor.FieldDefinition, len(test.fields))
		for fieldIndex, field := range test.fields {
			definitions[fieldIndex] = schemaauthor.FieldDefinition{Name: field.Name(), Role: schemaauthor.FieldRoleSemantic, Kind: field.Kind(), Required: true, Disclosure: test.disclosures[fieldIndex]}
		}
		owner := "sample." + test.name
		schema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: owner, SemanticKind: test.name, IdentitySchema: owner + ".v1", Fields: definitions})
		if err != nil {
			t.Fatal(err)
		}
		builder, err := runtimev2.NewExtensionIdentityBuilder(schema)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range test.fields {
			if err := builder.AddIdentityField(field); err != nil {
				t.Fatal(err)
			}
		}
		composition, err := builder.Build()
		if err != nil {
			t.Fatal(err)
		}
		providers[index] = &canonicalProvider{descriptor: resolver.Descriptor{Role: "input", Owner: owner, Kind: test.name, SpecificationSchema: owner + ".declaration.v1", SemanticVersion: "1"}, schema: schema, identity: composition.Identity()}
		registered[index] = providers[index]
		declarations[index], err = runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: owner, Kind: test.name, SpecificationSchema: owner + ".declaration.v1", Fields: []runtimev2.DeclarationField{{Name: "ref", Value: "  REF  "}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	registry, err := resolver.NewRegistry(registered...)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cacheRoot := t.TempDir()
	cache, err := resolver.NewDirectoryCache(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := resolver.NewManager(registry, cache)
	if err != nil {
		t.Fatal(err)
	}
	workspace := resolver.Workspace{Root: root}
	for index, declaration := range declarations {
		first, err := manager.ResolveCurrent(context.Background(), workspace, declaration)
		if err != nil || first.CacheHit || providers[index].resolveCalls != 1 {
			t.Fatalf("%s fresh = %+v, %v", cases[index].name, first, err)
		}
	}
	restartedCache, err := resolver.NewDirectoryCache(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	manager, err = resolver.NewManager(registry, restartedCache)
	if err != nil {
		t.Fatal(err)
	}
	for index, declaration := range declarations {
		second, err := manager.ResolveCurrent(context.Background(), workspace, declaration)
		if err != nil || !second.CacheHit || second.PriorStatus != resolver.Current || providers[index].resolveCalls != 1 || providers[index].revalidateCalls != 1 || providers[index].acquireCalls != 0 {
			t.Fatalf("%s cached = %+v, %v", cases[index].name, second, err)
		}
		if string(second.Resolution.Evidence) != `{"generation":2}` || len(second.Resolution.Identity.Fields()) != len(cases[index].fields) {
			t.Fatalf("%s lost typed result", cases[index].name)
		}
		gotFields := map[string]runtimev2.IdentityField{}
		for _, field := range second.Resolution.Identity.Fields() {
			gotFields[field.Field.Name()] = field.Field
		}
		for _, want := range cases[index].fields {
			got, exists := gotFields[want.Name()]
			if !exists || got.Kind() != want.Kind() || got.Commitment() != want.Commitment() {
				t.Fatalf("%s lost typed field %s", cases[index].name, want.Name())
			}
			switch want.Kind() {
			case runtimev2.IdentityFieldText:
				if got.Text() != want.Text() {
					t.Fatal("text changed")
				}
			case runtimev2.IdentityFieldCanonicalValue:
				if string(got.CanonicalValue().CanonicalBytes()) != string(want.CanonicalValue().CanonicalBytes()) {
					t.Fatal("canonical value changed")
				}
			case runtimev2.IdentityFieldSecretReference:
				if got.SecretReference().Provider() != want.SecretReference().Provider() || got.SecretReference().Identifier() != want.SecretReference().Identifier() || got.SecretReference().Version() != want.SecretReference().Version() {
					t.Fatal("secret reference changed")
				}
			}
		}
	}
	alternate, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: declarations[0].Owner(), Kind: declarations[0].Kind(), SpecificationSchema: declarations[0].SpecificationSchema(),
		Fields: []runtimev2.DeclarationField{{Name: "ref", Value: "ref"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := manager.ResolveCurrent(context.Background(), workspace, alternate)
	if err != nil || !equivalent.CacheHit || providers[0].resolveCalls != 1 {
		t.Fatalf("equivalent normalized spelling = %+v, %v", equivalent, err)
	}
	differentScope, err := manager.ResolveCurrent(context.Background(), resolver.Workspace{Root: t.TempDir()}, alternate)
	if err != nil || differentScope.CacheHit || providers[0].resolveCalls != 2 {
		t.Fatalf("different physical workspace = %+v, %v", differentScope, err)
	}
	newVersion := *providers[0]
	newVersion.descriptor.SemanticVersion = "2"
	versionRegistry, err := resolver.NewRegistry(&newVersion)
	if err != nil {
		t.Fatal(err)
	}
	versionManager, err := resolver.NewManager(versionRegistry, restartedCache)
	if err != nil {
		t.Fatal(err)
	}
	versionOutcome, err := versionManager.ResolveCurrent(context.Background(), workspace, alternate)
	if err != nil || versionOutcome.CacheHit || newVersion.resolveCalls != 3 {
		t.Fatalf("semantic version change = %+v, %v", versionOutcome, err)
	}
	changedSchema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: providers[0].descriptor.Owner, SemanticKind: providers[0].descriptor.Kind, IdentitySchema: providers[0].schema.IdentitySchema(), Fields: []schemaauthor.FieldDefinition{{Name: "digest", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosureProtected}}})
	if err != nil {
		t.Fatal(err)
	}
	changedBuilder, err := runtimev2.NewExtensionIdentityBuilder(changedSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := changedBuilder.AddIdentityField(textField); err != nil {
		t.Fatal(err)
	}
	changedComposition, err := changedBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	changedProvider := *providers[0]
	changedProvider.schema, changedProvider.identity = changedSchema, changedComposition.Identity()
	changedRegistry, err := resolver.NewRegistry(&changedProvider)
	if err != nil {
		t.Fatal(err)
	}
	changedManager, err := resolver.NewManager(changedRegistry, restartedCache)
	if err != nil {
		t.Fatal(err)
	}
	changedOutcome, err := changedManager.ResolveCurrent(context.Background(), workspace, alternate)
	if err != nil || changedOutcome.CacheHit || changedOutcome.Reason != "corrupt_cache" || changedProvider.resolveCalls != 3 {
		t.Fatalf("same-descriptor disclosure change = %+v, %v", changedOutcome, err)
	}
	if _, err := resolver.NewRegistry(&canonicalProvider{descriptor: providers[0].descriptor, schema: providers[1].schema}); !errors.Is(err, resolver.ErrInvalidDeclaration) {
		t.Fatalf("mismatched schema = %v", err)
	}
}
