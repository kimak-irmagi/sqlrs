package resolver_test

import (
	"encoding/json"
	"testing"

	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
)

// This test keeps CacheRecord consumable without package-private access.
func TestCacheRecordPublicSurface(t *testing.T) {
	declaration := workspaceDeclaration(t)
	key, err := resolver.NewCacheKey(
		resolver.Workspace{Root: t.TempDir()},
		descriptor("one"),
		resolver.NormalizedDeclaration{Declaration: declaration},
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err := resolver.NewCacheRecord(key, fileTestSchema(declaration.Owner(), declaration.Kind()), resolution(t, declaration))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := resolver.DecodeCacheRecordJSON(raw, fileTestSchema(declaration.Owner(), declaration.Kind()))
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Matches(key) || decoded.Resolution().Identity.IdentitySchema() == "" {
		t.Fatal("public record lost its key or resolution")
	}
}
