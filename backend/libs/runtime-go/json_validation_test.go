package runtimev2_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func TestSemanticJSONRoundTrips(t *testing.T) {
	recipeLineage := buildRecipe(t, testFactory(t, "a"), testTransform(t, "b", "one.sql"))
	assertJSONRoundTrip(t, recipeLineage, &runtimev2.RecipeLineage{}, func(got *runtimev2.RecipeLineage) runtimev2.StateID { return got.Endpoint().ID() })
	relative, err := runtimev2.Extend(recipeLineage.Root().ID(), []runtimev2.TransformProvenance{testTransform(t, "b", "one.sql")})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONRoundTrip(t, relative, &runtimev2.RelativeLineage{}, func(got *runtimev2.RelativeLineage) runtimev2.StateID { return got.EndpointID() })
}

func TestJSONRejectsUnknownDuplicateNullTrailingAndInvalidUTF8(t *testing.T) {
	valid := `{"schema_version":"sqlrs.runtime.v2","provider":"sqlrs","kind":"psql","identity_schema":"schema.v1","fields":[]}`
	cases := map[string]string{
		"unknown":   strings.TrimSuffix(valid, "}") + `,"runtime_id":"physical"}`,
		"duplicate": strings.Replace(valid, `"provider":"sqlrs"`, `"provider":"sqlrs","provider":"other"`, 1),
		"null":      strings.Replace(valid, `"provider":"sqlrs"`, `"provider":null`, 1),
		"trailing":  valid + `{}`,
		"bad utf8":  string(append([]byte(valid[:len(valid)-1]), 0xff, '}')),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			var got runtimev2.ResolvedTransformIdentity
			if err := runtimev2.DecodeJSON([]byte(raw), &got); !errors.Is(err, runtimev2.ErrInvalid) {
				t.Fatalf("invalid JSON accepted or wrong error: %v", err)
			}
		})
	}
}

func TestFailedUnmarshalLeavesReceiverUnchanged(t *testing.T) {
	original, err := runtimev2.NewTransformIdentity(identityInput("a"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := runtimev2.TransformFingerprint(original)
	if err := json.Unmarshal([]byte(`{"schema_version":"wrong"}`), &original); !errors.Is(err, runtimev2.ErrInvalid) {
		t.Fatalf("wrong error: %v", err)
	}
	got, _ := runtimev2.TransformFingerprint(original)
	if got != want {
		t.Fatal("failed unmarshal mutated receiver")
	}
}

func TestJSONTamperingIsRejected(t *testing.T) {
	lineage := buildRecipe(t, testFactory(t, "a"), testTransform(t, "b", "one.sql"))
	raw, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), string(lineage.Endpoint().ID()), "sha256:"+strings.Repeat("0", 64), 1)
	var got runtimev2.RecipeLineage
	err = json.Unmarshal([]byte(tampered), &got)
	var validation *runtimev2.ValidationError
	if !errors.As(err, &validation) || validation.Code != runtimev2.CodeIntegrityMismatch {
		t.Fatalf("tamper not rejected as integrity mismatch: %v", err)
	}
}

func TestIdentifierAndCollectionValidation(t *testing.T) {
	bad := []string{"", "Upper", "1start", ".dot", "-dash", "_under", "with space", "path/name", "имя"}
	for _, value := range bad {
		t.Run(value, func(t *testing.T) {
			input := identityInput("a")
			input.Provider = value
			if _, err := runtimev2.NewTransformIdentity(input); !errors.Is(err, runtimev2.ErrInvalid) {
				t.Fatalf("invalid identifier accepted: %q", value)
			}
		})
	}
	empty := identityInput("a")
	empty.Fields = []runtimev2.ResolvedField{}
	if _, err := runtimev2.NewTransformIdentity(empty); err != nil {
		t.Fatalf("empty field set rejected: %v", err)
	}
	wrongVersion := identityInput("a")
	wrongVersion.SchemaVersion = "sqlrs.runtime.v3"
	if _, err := runtimev2.NewTransformIdentity(wrongVersion); !errors.Is(err, runtimev2.ErrInvalid) {
		t.Fatal("unsupported schema version accepted")
	}
}

func TestValidationLimits(t *testing.T) {
	input := identityInput("a")
	input.Provider = "a" + strings.Repeat("b", runtimev2.MaxIdentifierBytes)
	if _, err := runtimev2.NewTransformIdentity(input); !errors.Is(err, runtimev2.ErrInvalid) {
		t.Fatal("oversized identifier accepted")
	}
	input = identityInput("a")
	input.Fields[0].Value = strings.Repeat("x", runtimev2.MaxResolvedValueBytes+1)
	if _, err := runtimev2.NewTransformIdentity(input); !errors.Is(err, runtimev2.ErrInvalid) {
		t.Fatal("oversized value accepted")
	}
	input = identityInput("a")
	input.Fields = make([]runtimev2.ResolvedField, runtimev2.MaxResolvedFields+1)
	for i := range input.Fields {
		input.Fields[i] = runtimev2.ResolvedField{Name: "f" + string(rune('a'+i%26)) + strings.Repeat("x", i/26), Value: "v"}
	}
	if _, err := runtimev2.NewTransformIdentity(input); !errors.Is(err, runtimev2.ErrInvalid) {
		t.Fatal("oversized field set accepted")
	}
}

func assertJSONRoundTrip[T any](t *testing.T, value any, target *T, endpoint func(*T) runtimev2.StateID) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
	var want runtimev2.StateID
	switch v := value.(type) {
	case runtimev2.RecipeLineage:
		want = v.Endpoint().ID()
	case runtimev2.RelativeLineage:
		want = v.EndpointID()
	}
	if endpoint(target) != want {
		t.Fatal("endpoint changed across JSON round trip")
	}
}

func FuzzResolvedTransformJSON(f *testing.F) {
	f.Add([]byte(`{"schema_version":"sqlrs.runtime.v2","provider":"sqlrs","kind":"psql","identity_schema":"schema.v1","fields":[]}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		var value runtimev2.ResolvedTransformIdentity
		if json.Unmarshal(raw, &value) == nil {
			if _, err := runtimev2.TransformFingerprint(value); err != nil {
				t.Fatalf("accepted invalid identity: %v", err)
			}
		}
	})
}
