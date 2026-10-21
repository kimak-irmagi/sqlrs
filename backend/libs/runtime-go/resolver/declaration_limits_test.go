package resolver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// DL06: the normalized source survives the actual checksummed cache envelope;
// enlarging a leaf must not enlarge the independent record budget.
func TestLargeDeclarationCacheRecord(t *testing.T) {
	workspace := Workspace{Root: t.TempDir()}
	makeKey := func(fields []runtimev2.DeclarationField) CacheKey {
		t.Helper()
		declaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", Fields: fields})
		if err != nil {
			t.Fatal(err)
		}
		key, err := NewCacheKey(workspace, Descriptor{Role: "input", Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", SemanticVersion: "1"}, NormalizedDeclaration{Declaration: declaration})
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	payload := strings.Repeat("x", 256<<10)
	fields := []runtimev2.DeclarationField{{Name: "payload", Value: payload}, {Name: "mode", Value: "one"}}
	key := makeKey(fields)
	fields[0], fields[1] = fields[1], fields[0]
	if makeKey(fields).String() != key.String() {
		t.Fatal("field order changed key")
	}
	record, err := NewCacheRecord(key, coverageSchema(), coverageResolution(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := record.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCacheRecordJSON(raw, coverageSchema())
	if err != nil || !decoded.Matches(key) {
		t.Fatalf("round trip: %v", err)
	}
	if !bytes.Contains(raw, []byte(payload)) || !bytes.Equal(decoded.record.Declaration, key.declaration) {
		t.Fatal("payload truncated")
	}
	fields = make([]runtimev2.DeclarationField, 16)
	for i := range fields {
		fields[i] = runtimev2.DeclarationField{Name: fmt.Sprintf("f%03d", i), Value: payload}
	}
	// Independent standard encoding measures the envelope overhead without
	// invoking a constructor whose boundary is under test.
	wire := struct {
		SchemaVersion       string                       `json:"schema_version"`
		Owner               string                       `json:"owner"`
		Kind                string                       `json:"kind"`
		SpecificationSchema string                       `json:"specification_schema"`
		Fields              []runtimev2.DeclarationField `json:"fields"`
	}{runtimev2.SchemaVersion, "owner", "kind", "owner.kind.v1", fields}
	full, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	fields[15].Value = payload[:len(payload)-(len(full)-(runtimev2.MaxJSONBytes-1))]
	largeKey := makeKey(fields)
	if len(largeKey.declaration) != runtimev2.MaxJSONBytes-1 {
		t.Fatal("wrong boundary fixture")
	}
	if got, err := NewCacheRecord(largeKey, coverageSchema(), coverageResolution(t)); err != ErrInvalidDeclaration || got.record != nil {
		t.Fatalf("record envelope overflow: %v", err)
	}
}
