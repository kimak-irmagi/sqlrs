package runtimev2

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type conformanceValueInput struct {
	Type    string                  `json:"type"`
	Value   string                  `json:"value,omitempty"`
	Members []conformanceValueInput `json:"members,omitempty"`
	Entries []struct {
		Key   string                `json:"key"`
		Value conformanceValueInput `json:"value"`
	} `json:"entries,omitempty"`
	Depth int `json:"depth,omitempty"`
}

type conformanceFieldInput struct {
	Name            string                 `json:"name"`
	Kind            IdentityFieldKind      `json:"kind"`
	Text            string                 `json:"text,omitempty"`
	CanonicalValue  *conformanceValueInput `json:"canonical_value,omitempty"`
	SecretReference *secretReferenceWire   `json:"secret_reference,omitempty"`
}

func buildConformanceField(input conformanceFieldInput) (IdentityField, error) {
	switch input.Kind {
	case IdentityFieldText:
		return NewTextIdentityField(input.Name, input.Text)
	case IdentityFieldCanonicalValue:
		if input.CanonicalValue == nil {
			return IdentityField{}, canonicalInvalid(CodeVectorInvalid, "input.canonical_value")
		}
		value, err := buildConformanceValue(*input.CanonicalValue)
		if err != nil {
			return IdentityField{}, err
		}
		return NewCanonicalValueIdentityField(input.Name, value)
	case IdentityFieldSecretReference:
		if input.SecretReference == nil {
			return IdentityField{}, canonicalInvalid(CodeVectorInvalid, "input.secret_reference")
		}
		reference, err := NewSecretReference(input.SecretReference.Provider, input.SecretReference.Identifier, input.SecretReference.Version)
		if err != nil {
			return IdentityField{}, err
		}
		return NewSecretReferenceIdentityField(input.Name, reference)
	default:
		return IdentityField{}, canonicalInvalid(CodeVectorInvalid, "input.kind")
	}
}

func buildConformanceValue(input conformanceValueInput) (CanonicalValue, error) {
	switch input.Type {
	case "null":
		return CanonicalNull(), nil
	case "string":
		return CanonicalString(input.Value)
	case "list", "set":
		members := make([]CanonicalValue, len(input.Members))
		for index, item := range input.Members {
			value, err := buildConformanceValue(item)
			if err != nil {
				return CanonicalValue{}, prefixError(err, "members["+itoa(index)+"]")
			}
			members[index] = value
		}
		if input.Type == "set" {
			return CanonicalSet(members)
		}
		return CanonicalList(members)
	case "map":
		entries := make([]CanonicalMapEntry, len(input.Entries))
		for index, item := range input.Entries {
			value, err := buildConformanceValue(item.Value)
			if err != nil {
				return CanonicalValue{}, prefixError(err, "entries["+itoa(index)+"].value")
			}
			entries[index] = CanonicalMapEntry{Key: item.Key, Value: value}
		}
		return CanonicalMap(entries)
	case "depth-over":
		value := CanonicalNull()
		for index := 1; index < input.Depth; index++ {
			var err error
			value, err = CanonicalList([]CanonicalValue{value})
			if err != nil {
				return CanonicalValue{}, err
			}
		}
		return value, nil
	case "duplicate-map":
		return CanonicalMap([]CanonicalMapEntry{{Key: "a", Value: CanonicalNull()}, {Key: "a", Value: CanonicalNull()}})
	case "duplicate-set":
		return CanonicalSet([]CanonicalValue{CanonicalNull(), CanonicalNull()})
	case "invalid-utf8":
		return CanonicalString(string([]byte{0xff}))
	case "members-over":
		return CanonicalList(make([]CanonicalValue, MaxCanonicalCollectionMembers+1))
	case "nodes-over":
		groups := make([]CanonicalValue, 17)
		for groupIndex := range groups {
			members := make([]CanonicalValue, MaxCanonicalCollectionMembers)
			for memberIndex := range members {
				members[memberIndex] = CanonicalNull()
			}
			groups[groupIndex], _ = CanonicalList(members)
		}
		return CanonicalList(groups)
	case "string-over":
		return CanonicalString(strings.Repeat("x", MaxCanonicalStringBytes+1))
	case "key-over":
		return CanonicalMap([]CanonicalMapEntry{{Key: strings.Repeat("k", MaxCanonicalMapKeyBytes+1), Value: CanonicalNull()}})
	case "bytes-over":
		members := make([]CanonicalValue, MaxCanonicalCollectionMembers)
		for index := range members {
			members[index], _ = CanonicalString(strings.Repeat("x", MaxCanonicalStringBytes))
		}
		return CanonicalList(members)
	default:
		return CanonicalValue{}, canonicalInvalid(CodeVectorInvalid, "input.type")
	}
}

func conformanceValueOutput(value CanonicalValue) map[string]any {
	preimage := append(encodeString(CanonicalIdentityValueDomain), encodeBytes(value.CanonicalBytes())...)
	return map[string]any{
		"canonical-bytes":       hex.EncodeToString(value.CanonicalBytes()),
		"preimage_hex":          hex.EncodeToString(preimage),
		"canonical-value-token": value.Token().String(),
	}
}

func rawObjectMember(raw json.RawMessage, name string) (json.RawMessage, error) {
	var input map[string]json.RawMessage
	if err := decodeCanonicalStrict(raw, &input); err != nil {
		return nil, err
	}
	member, ok := input[name]
	if !ok || len(member) == 0 || isJSONNull(member) {
		return nil, canonicalInvalid(CodeVectorInvalid, "input."+name)
	}
	return member, nil
}

func envelopeConformanceOutput(envelope FingerprintEnvelope) map[string]any {
	raw, _ := json.Marshal(envelope)
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	return map[string]any{"digest": string(envelope.Descriptor().Digest()), "descriptor": wire["descriptor"]}
}

func decodeConformanceEnvelope(raw json.RawMessage) (FingerprintEnvelope, error) {
	member, err := rawObjectMember(raw, "envelope")
	if err != nil {
		return FingerprintEnvelope{}, err
	}
	var envelope FingerprintEnvelope
	if err := json.Unmarshal(member, &envelope); err != nil {
		return FingerprintEnvelope{}, err
	}
	return envelope, nil
}

type conformanceAuthorizer struct{}

func (conformanceAuthorizer) AuthorizeInternalDisclosure(context.Context) error { return nil }

func evaluateVectorCase(item vectorCaseWire) (map[string]any, error) {
	switch item.Operation {
	case "canonical-value":
		var input conformanceValueInput
		if err := decodeCanonicalStrict(item.Input, &input); err != nil {
			return nil, err
		}
		value, err := buildConformanceValue(input)
		if err != nil {
			return nil, err
		}
		return conformanceValueOutput(value), nil
	case "canonical-token":
		var input struct {
			Value string `json:"value"`
		}
		if err := decodeCanonicalStrict(item.Input, &input); err != nil {
			return nil, err
		}
		token, err := ParseCanonicalValueToken(input.Value)
		if err != nil {
			return nil, err
		}
		return map[string]any{"canonical-value-token": token.String()}, nil
	case "canonical-value-envelope":
		var input struct {
			Hex    string `json:"hex"`
			Repeat int    `json:"repeat,omitempty"`
		}
		if err := decodeCanonicalStrict(item.Input, &input); err != nil {
			return nil, err
		}
		raw, err := hex.DecodeString(input.Hex)
		if err != nil || len(raw) == 0 || input.Repeat < 0 || input.Repeat > MaxCanonicalValueBytes+1 {
			return nil, canonicalInvalid(CodeVectorInvalid, "input")
		}
		if input.Repeat > 0 {
			raw = bytes.Repeat(raw, input.Repeat)
		}
		value, err := ParseCanonicalValueEnvelope(raw)
		if err != nil {
			return nil, err
		}
		return conformanceValueOutput(value), nil
	case "identity-field":
		var input conformanceFieldInput
		if err := decodeCanonicalStrict(item.Input, &input); err != nil {
			return nil, err
		}
		field, err := buildConformanceField(input)
		if err != nil {
			return nil, err
		}
		return map[string]any{"field-commitment": string(field.Commitment()), "kind": field.Kind()}, nil
	case "factory", "transform", "resolved-extension", "root-state", "derived-state":
		envelope, err := decodeConformanceEnvelope(item.Input)
		if err != nil {
			return nil, err
		}
		expectedKind := map[string]FingerprintKind{
			"factory": FingerprintKindFactoryState, "root-state": FingerprintKindFactoryState,
			"transform": FingerprintKindTransform, "resolved-extension": FingerprintKindResolvedExtension,
			"derived-state": FingerprintKindDerivedState,
		}[item.Operation]
		if envelope.Descriptor().Kind() != expectedKind {
			return nil, canonicalInvalid(CodeDescriptorInvalid, "descriptor.kind")
		}
		return envelopeConformanceOutput(envelope), nil
	case "compose-factory", "compose-transform":
		extensionRaw, err := rawObjectMember(item.Input, "extension")
		if err != nil {
			return nil, err
		}
		resultRaw, err := rawObjectMember(item.Input, "result")
		if err != nil {
			return nil, err
		}
		var extension, result FingerprintEnvelope
		if err := json.Unmarshal(extensionRaw, &extension); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(resultRaw, &result); err != nil {
			return nil, err
		}
		if extension.Descriptor().Kind() != FingerprintKindResolvedExtension || result.data.identity == nil {
			return nil, canonicalInvalid(CodeRelationMismatch, "input")
		}
		binding := "extension.input.0"
		found := false
		for _, field := range result.data.identity.fields {
			if field.field.Name() == binding && field.field.Kind() == IdentityFieldText && field.field.Text() == string(extension.Descriptor().Digest()) {
				found = true
			}
		}
		if !found {
			return nil, canonicalInvalid(CodeRelationMismatch, "input.result.subject.fields")
		}
		return envelopeConformanceOutput(result), nil
	case "recipe":
		raw, err := rawObjectMember(item.Input, "envelope")
		if err != nil {
			return nil, err
		}
		var lineage CanonicalRecipeEnvelope
		if err := json.Unmarshal(raw, &lineage); err != nil {
			return nil, err
		}
		return lineageConformanceOutput(lineage.Endpoint(), lineage.Steps()), nil
	case "relative-lineage":
		raw, err := rawObjectMember(item.Input, "envelope")
		if err != nil {
			return nil, err
		}
		var lineage CanonicalRelativeEnvelope
		if err := json.Unmarshal(raw, &lineage); err != nil {
			return nil, err
		}
		return lineageConformanceOutput(lineage.Endpoint(), lineage.Steps()), nil
	case "decode-envelope":
		_, err := decodeConformanceEnvelope(item.Input)
		return nil, err
	case "explain-safe", "explain-internal":
		envelope, err := decodeConformanceEnvelope(item.Input)
		if err != nil {
			return nil, err
		}
		var explanation FingerprintExplanation
		if item.Operation == "explain-safe" {
			explanation, _ = ExplainSafe(envelope) // The strict envelope above is valid.
		} else {
			explanation, _ = ExplainInternal(context.Background(), envelope, conformanceAuthorizer{}) // The fixed authorizer always permits a live context.
		}
		raw, _ := json.Marshal(explanation)
		var projection any
		_ = json.Unmarshal(raw, &projection)
		return map[string]any{"explanation": projection, "digest": string(envelope.Descriptor().Digest())}, nil
	case "legacy-decode":
		var input struct {
			SchemaVersion string `json:"schema_version"`
		}
		if err := decodeCanonicalStrict(item.Input, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion == SchemaVersion {
			return nil, canonicalInvalid(CodeRevisionMismatch, "schema_version")
		}
		return nil, canonicalInvalid(CodeVectorInvalid, "input.schema_version")
	default:
		return nil, canonicalInvalid(CodeVectorInvalid, "operation")
	}
}

func lineageConformanceOutput(endpoint FingerprintEnvelope, steps []FingerprintEnvelope) map[string]any {
	ids := make([]string, len(steps))
	for index, step := range steps {
		ids[index] = string(step.Descriptor().Digest())
	}
	return map[string]any{"endpoint": string(endpoint.Descriptor().Digest()), "state-ids": ids}
}

func verifyVectorCase(item vectorCaseWire, path string) error {
	var expected map[string]any
	if err := json.Unmarshal(item.Expected, &expected); err != nil {
		return canonicalInvalid(CodeVectorInvalid, path+".expected")
	}
	actual, evaluationErr := evaluateVectorCase(item)
	if expected["status"] == "error" {
		var validation *ValidationError
		errorObject, ok := expected["error"].(map[string]any)
		if !ok || !errors.As(evaluationErr, &validation) || string(validation.Code) != errorObject["code"] || validation.Path != errorObject["path"] {
			return canonicalInvalid(CodeVectorInvalid, path+".expected")
		}
		return nil
	}
	if evaluationErr != nil {
		return canonicalInvalid(CodeVectorInvalid, path+".expected")
	}
	for name, expectedValue := range expected {
		if name == "status" {
			continue
		}
		actualJSON, _ := json.Marshal(actual[name])
		expectedJSON, _ := json.Marshal(expectedValue)
		if string(actualJSON) != string(expectedJSON) {
			return canonicalInvalid(CodeVectorInvalid, path+".expected."+name)
		}
	}
	return nil
}
