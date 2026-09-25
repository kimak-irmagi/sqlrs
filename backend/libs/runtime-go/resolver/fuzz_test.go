package resolver

import (
	"encoding/json"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func FuzzWorkspacePathAndEvidence(f *testing.F) {
	f.Add("input.sql", `{"schema_version":"sqlrs.workspace-file.evidence.v1","path":"input.sql","size":1}`)
	f.Add("../escape", `{"schema_version":"other","path":"x","size":-1}`)
	identity, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: workspaceOwner, Kind: workspaceKind,
		IdentitySchema: workspaceIdentity, Fields: []runtimev2.ResolvedField{{Name: "content.digest", Value: digestOf([]byte("x"))}},
	})
	if err != nil {
		f.Fatal(err)
	}
	provider := &workspaceFileResolver{}
	f.Fuzz(func(t *testing.T, path, evidence string) {
		_, _ = normalizeWorkspacePath(path)
		_ = provider.ValidateResolution(Resolution{Identity: identity, Evidence: json.RawMessage(evidence)})
	})
}

func FuzzCacheJSONScanner(f *testing.F) {
	for _, seed := range []string{`{}`, `[]`, `{"a":1,"a":2}`, `{`, `null`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) { _ = rejectDuplicateJSON([]byte(raw)) })
}
