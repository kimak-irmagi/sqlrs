package consumertest

import (
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func TestStandaloneConsumer(t *testing.T) {
	identity, err := runtimev2.NewTransformIdentity(runtimev2.TransformIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion,
		Provider:      "sqlrs",
		Kind:          "psql",
		IdentitySchema: "transform.v1",
		Fields:         []runtimev2.ResolvedField{{Name: "sql", Value: "select 1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint, err := runtimev2.TransformFingerprint(identity); err != nil || fingerprint == "" {
		t.Fatalf("fingerprint = %q, error = %v", fingerprint, err)
	}
	if identity.Provider() != "sqlrs" || identity.Kind() != "psql" || identity.IdentitySchema() != "transform.v1" {
		t.Fatalf("resolved identity accessors lost data: %+v", identity.Fields())
	}
}
