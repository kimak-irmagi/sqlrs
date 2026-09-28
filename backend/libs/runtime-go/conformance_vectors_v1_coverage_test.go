package runtimev2

import (
	"encoding/json"
	"testing"
)

func conformanceCasesByID(t *testing.T) map[string]vectorCaseWire {
	t.Helper()
	bundle, err := LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]vectorCaseWire{}
	for name, raw := range bundle.Files() {
		if len(name) < len("vectors/.json") || name[:len("vectors/")] != "vectors/" {
			continue
		}
		var vector vectorFileWire
		if err := json.Unmarshal(raw, &vector); err != nil {
			t.Fatal(err)
		}
		for _, item := range vector.Cases {
			result[item.ID] = item
		}
	}
	return result
}

func caseWithInput(t *testing.T, item vectorCaseWire, mutate func(map[string]any)) vectorCaseWire {
	t.Helper()
	var input map[string]any
	if err := json.Unmarshal(item.Input, &input); err != nil {
		t.Fatal(err)
	}
	mutate(input)
	item.Input = mustJSONLine(t, input)
	return item
}

func requireEvaluationError(t *testing.T, item vectorCaseWire) {
	t.Helper()
	if _, err := evaluateVectorCase(item); err == nil {
		t.Fatalf("case %q unexpectedly evaluated", item.ID)
	}
}

func TestConformanceValueAndFieldOperationFaultMatrix(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	for name, input := range map[string]conformanceValueInput{
		"list child": {Type: "list", Members: []conformanceValueInput{{Type: "string", Value: invalidUTF8}}},
		"map child": {Type: "map", Entries: []struct {
			Key   string                `json:"key"`
			Value conformanceValueInput `json:"value"`
		}{{Key: "x", Value: conformanceValueInput{Type: "unknown"}}}},
		"unknown": {Type: "unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := buildConformanceValue(input); err == nil {
				t.Fatal("invalid conformance value accepted")
			}
		})
	}
	member := conformanceValueInput{Type: "string", Value: "x"}
	if value, err := buildConformanceValue(conformanceValueInput{Type: "set", Members: []conformanceValueInput{member}}); err != nil || !value.Valid() {
		t.Fatalf("set fixture failed: %v", err)
	}
	if value, err := buildConformanceValue(conformanceValueInput{Type: "depth-over", Depth: 2}); err != nil || value.Depth() != 2 {
		t.Fatalf("bounded depth fixture failed: %v", err)
	}

	canonical := conformanceValueInput{Type: "null"}
	secret := &secretReferenceWire{Provider: "vault", Identifier: "id", Version: "v1"}
	valid := []conformanceFieldInput{
		{Name: "field", Kind: IdentityFieldText, Text: "civ1:sha256:not-a-token"},
		{Name: "field", Kind: IdentityFieldCanonicalValue, CanonicalValue: &canonical},
		{Name: "field", Kind: IdentityFieldSecretReference, SecretReference: secret},
	}
	for _, input := range valid {
		field, err := buildConformanceField(input)
		if err != nil || !field.Valid() {
			t.Fatalf("valid %s field failed: %v", input.Kind, err)
		}
		raw, _ := json.Marshal(input)
		output, err := evaluateVectorCase(vectorCaseWire{Operation: "identity-field", Input: raw})
		if err != nil || output["kind"] != input.Kind {
			t.Fatalf("identity-field operation %s failed: %v", input.Kind, err)
		}
	}
	invalid := []conformanceFieldInput{
		{Name: "field", Kind: IdentityFieldCanonicalValue},
		{Name: "field", Kind: IdentityFieldCanonicalValue, CanonicalValue: &conformanceValueInput{Type: "unknown"}},
		{Name: "field", Kind: IdentityFieldSecretReference},
		{Name: "field", Kind: IdentityFieldSecretReference, SecretReference: &secretReferenceWire{}},
		{Name: "field", Kind: "unknown"},
	}
	for _, input := range invalid {
		if _, err := buildConformanceField(input); err == nil {
			t.Fatalf("invalid %q field accepted", input.Kind)
		}
	}
}

func TestConformanceCaseEvaluatorFaultMatrix(t *testing.T) {
	cases := conformanceCasesByID(t)
	if _, err := rawObjectMember([]byte(`{`), "x"); err == nil {
		t.Fatal("malformed input accepted")
	}
	for _, raw := range []json.RawMessage{[]byte(`{}`), []byte(`{"x":null}`)} {
		if _, err := rawObjectMember(raw, "x"); err == nil {
			t.Fatal("missing/null input member accepted")
		}
	}
	if _, err := decodeConformanceEnvelope([]byte(`{"envelope":{}}`)); err == nil {
		t.Fatal("invalid envelope accepted")
	}

	canonical := cases["canonical-value/null"]
	canonical.Input = []byte(`{`)
	requireEvaluationError(t, canonical)
	canonical.Input = []byte(`{"type":"unknown"}`)
	requireEvaluationError(t, canonical)
	identityField := vectorCaseWire{Operation: "identity-field", Input: []byte(`{`)}
	requireEvaluationError(t, identityField)
	identityField.Input = []byte(`{"name":"field","kind":"canonical-value","canonical_value":{"type":"unknown"}}`)
	requireEvaluationError(t, identityField)

	factory := cases["fingerprints/factory"]
	malformed := factory
	malformed.Input = []byte(`{`)
	requireEvaluationError(t, malformed)
	wrongKind := factory
	wrongKind.Operation = "transform"
	requireEvaluationError(t, wrongKind)

	compose := cases["composition/extensions"]
	for _, member := range []string{"extension", "result"} {
		candidate := caseWithInput(t, compose, func(input map[string]any) { delete(input, member) })
		requireEvaluationError(t, candidate)
	}
	for _, member := range []string{"extension", "result"} {
		candidate := caseWithInput(t, compose, func(input map[string]any) { input[member] = map[string]any{} })
		requireEvaluationError(t, candidate)
	}
	var factoryInput map[string]any
	_ = json.Unmarshal(factory.Input, &factoryInput)
	for _, member := range []string{"extension", "result"} {
		candidate := caseWithInput(t, compose, func(input map[string]any) { input[member] = factoryInput["envelope"] })
		requireEvaluationError(t, candidate)
	}
	derived := cases["fingerprints/derived"]
	var derivedInput map[string]any
	_ = json.Unmarshal(derived.Input, &derivedInput)
	candidate := caseWithInput(t, compose, func(input map[string]any) { input["result"] = derivedInput["envelope"] })
	requireEvaluationError(t, candidate)

	for _, id := range []string{"lineage/recipe", "lineage/relative", "secrets/safe"} {
		item := cases[id]
		item.Input = []byte(`{}`)
		requireEvaluationError(t, item)
		item.Input = []byte(`{"envelope":{}}`)
		requireEvaluationError(t, item)
	}
	legacy := cases["compatibility/legacy"]
	legacy.Input = []byte(`{`)
	requireEvaluationError(t, legacy)
	legacy.Input = []byte(`{"schema_version":"other"}`)
	requireEvaluationError(t, legacy)
	requireEvaluationError(t, vectorCaseWire{Operation: "unsupported", Input: []byte(`{"x":1}`)})
}

func TestVerifyVectorCaseFailureMatching(t *testing.T) {
	cases := conformanceCasesByID(t)
	item := cases["integrity/digest"]
	item.Expected = []byte(`{`)
	if err := verifyVectorCase(item, "case"); err == nil {
		t.Fatal("malformed expected accepted")
	}
	item = cases["integrity/digest"]
	item.Expected = []byte(`{"status":"error","error":"wrong"}`)
	if err := verifyVectorCase(item, "case"); err == nil {
		t.Fatal("non-object expected error accepted")
	}
	item = cases["integrity/digest"]
	item.Expected = []byte(`{"status":"error","error":{"code":"endpoint_mismatch","path":"endpoint"}}`)
	if err := verifyVectorCase(item, "case"); err == nil {
		t.Fatal("wrong expected error accepted")
	}
	item = cases["integrity/digest"]
	item.Expected = []byte(`{"status":"ok","digest":"x"}`)
	if err := verifyVectorCase(item, "case"); err == nil {
		t.Fatal("evaluation error accepted as success")
	}
	if err := verifyVectorCase(cases["integrity/digest"], "case"); err != nil {
		t.Fatalf("valid negative vector failed: %v", err)
	}
}

func TestVectorExpectedShapeFailureMatrix(t *testing.T) {
	for _, raw := range []json.RawMessage{
		[]byte(`{`),
		[]byte(`{"status":"other","value":1}`),
		[]byte(`{"status":"error","error":{"code":"unknown","path":"x"}}`),
		[]byte(`{"status":"error","error":{"code":"digest_mismatch"}}`),
	} {
		if err := validateVectorExpected(raw, "case"); err == nil {
			t.Fatalf("invalid expected accepted: %s", raw)
		}
	}
	if err := validateVectorExpected([]byte(`{"status":"error","error":{"code":"syntax_invalid","path":""}}`), "case"); err != nil {
		t.Fatalf("document-wide error path rejected: %v", err)
	}
}
