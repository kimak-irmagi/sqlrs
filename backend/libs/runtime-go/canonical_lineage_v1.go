package runtimev2

import "encoding/json"

type canonicalLineageData struct {
	anchor FingerprintEnvelope
	steps  []FingerprintEnvelope
}

// CanonicalRecipeEnvelope is a strict factory root plus ordered derived states.
type CanonicalRecipeEnvelope struct{ data *canonicalLineageData }

// CanonicalRelativeEnvelope is a strict verified anchor plus ordered derived states.
type CanonicalRelativeEnvelope struct{ data *canonicalLineageData }

func cloneEnvelope(source FingerprintEnvelope) FingerprintEnvelope {
	if source.data == nil {
		return FingerprintEnvelope{}
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return FingerprintEnvelope{}
	}
	var result FingerprintEnvelope
	if json.Unmarshal(raw, &result) != nil {
		return FingerprintEnvelope{}
	}
	return result
}

func buildCanonicalLineage(anchor FingerprintEnvelope, transforms []FingerprintEnvelope, recipe bool) (*canonicalLineageData, error) {
	if anchor.data == nil {
		return nil, canonicalInvalid(CodeShapeInvalid, "anchor")
	}
	if recipe && anchor.data.descriptor.kind != FingerprintKindFactoryState {
		return nil, canonicalInvalid(CodeDescriptorInvalid, "root.descriptor.kind")
	}
	if !recipe && anchor.data.descriptor.kind != FingerprintKindFactoryState && anchor.data.descriptor.kind != FingerprintKindDerivedState {
		return nil, canonicalInvalid(CodeDescriptorInvalid, "anchor.descriptor.kind")
	}
	parent := CanonicalStateID(anchor.data.descriptor.digest)
	steps := make([]FingerprintEnvelope, len(transforms))
	for index, transform := range transforms {
		derived, err := NewDerivedStateEnvelope(parent, transform)
		if err != nil {
			return nil, prefixError(err, "transforms["+itoa(index)+"]")
		}
		steps[index] = derived
		parent = CanonicalStateID(derived.data.descriptor.digest)
	}
	return &canonicalLineageData{anchor: cloneEnvelope(anchor), steps: steps}, nil
}

// NewCanonicalRecipeEnvelope derives a complete ordered recipe lineage.
func NewCanonicalRecipeEnvelope(root FingerprintEnvelope, transforms []FingerprintEnvelope) (CanonicalRecipeEnvelope, error) {
	data, err := buildCanonicalLineage(root, transforms, true)
	if err != nil {
		return CanonicalRecipeEnvelope{}, err
	}
	return CanonicalRecipeEnvelope{data: data}, nil
}

// NewCanonicalRelativeEnvelope derives a suffix from a verified full anchor envelope.
func NewCanonicalRelativeEnvelope(anchor FingerprintEnvelope, transforms []FingerprintEnvelope) (CanonicalRelativeEnvelope, error) {
	data, err := buildCanonicalLineage(anchor, transforms, false)
	if err != nil {
		return CanonicalRelativeEnvelope{}, err
	}
	return CanonicalRelativeEnvelope{data: data}, nil
}

func lineageSteps(source []FingerprintEnvelope) []FingerprintEnvelope {
	result := make([]FingerprintEnvelope, len(source))
	for index, value := range source {
		result[index] = cloneEnvelope(value)
	}
	return result
}
func (l CanonicalRecipeEnvelope) Root() FingerprintEnvelope {
	if l.data == nil {
		return FingerprintEnvelope{}
	}
	return cloneEnvelope(l.data.anchor)
}
func (l CanonicalRecipeEnvelope) Steps() []FingerprintEnvelope {
	if l.data == nil {
		return nil
	}
	return lineageSteps(l.data.steps)
}
func (l CanonicalRecipeEnvelope) Endpoint() FingerprintEnvelope {
	if l.data == nil {
		return FingerprintEnvelope{}
	}
	if len(l.data.steps) == 0 {
		return cloneEnvelope(l.data.anchor)
	}
	return cloneEnvelope(l.data.steps[len(l.data.steps)-1])
}
func (l CanonicalRelativeEnvelope) Anchor() FingerprintEnvelope {
	if l.data == nil {
		return FingerprintEnvelope{}
	}
	return cloneEnvelope(l.data.anchor)
}
func (l CanonicalRelativeEnvelope) Steps() []FingerprintEnvelope {
	if l.data == nil {
		return nil
	}
	return lineageSteps(l.data.steps)
}
func (l CanonicalRelativeEnvelope) Endpoint() FingerprintEnvelope {
	if l.data == nil {
		return FingerprintEnvelope{}
	}
	if len(l.data.steps) == 0 {
		return cloneEnvelope(l.data.anchor)
	}
	return cloneEnvelope(l.data.steps[len(l.data.steps)-1])
}

type canonicalLineageWire struct {
	SchemaVersion string             `json:"schema_version"`
	Anchor        json.RawMessage    `json:"anchor"`
	Steps         *[]json.RawMessage `json:"steps"`
	Endpoint      string             `json:"endpoint"`
}

func marshalCanonicalLineage(data *canonicalLineageData) ([]byte, error) {
	if data == nil {
		return nil, canonicalInvalid(CodeShapeInvalid, "$")
	}
	anchor, err := json.Marshal(data.anchor)
	if err != nil {
		return nil, err
	}
	steps := make([]json.RawMessage, len(data.steps))
	endpoint := data.anchor.data.descriptor.digest
	for index, value := range data.steps {
		steps[index], err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
		endpoint = value.data.descriptor.digest
	}
	return json.Marshal(canonicalLineageWire{CanonicalSchemaVersion, anchor, &steps, endpoint})
}
func unmarshalCanonicalLineage(raw []byte, recipe bool) (*canonicalLineageData, error) {
	var wire canonicalLineageWire
	if err := decodeCanonicalStrict(raw, &wire); err != nil {
		return nil, err
	}
	if wire.SchemaVersion != CanonicalSchemaVersion {
		return nil, canonicalInvalid(CodeRevisionMismatch, "schema_version")
	}
	if len(wire.Anchor) == 0 || isJSONNull(wire.Anchor) {
		return nil, canonicalInvalid(CodeShapeInvalid, "anchor")
	}
	var anchor FingerprintEnvelope
	if err := json.Unmarshal(wire.Anchor, &anchor); err != nil {
		return nil, prefixError(err, "anchor")
	}
	if wire.Steps == nil {
		return nil, canonicalInvalid(CodeShapeInvalid, "steps")
	}
	transforms := make([]FingerprintEnvelope, len(*wire.Steps))
	expectedSteps := make([]FingerprintEnvelope, len(*wire.Steps))
	parent := CanonicalStateID(anchor.data.descriptor.digest)
	for index, rawStep := range *wire.Steps {
		var step FingerprintEnvelope
		if err := json.Unmarshal(rawStep, &step); err != nil {
			return nil, prefixError(err, "steps["+itoa(index)+"]")
		}
		if step.data.descriptor.kind != FingerprintKindDerivedState || step.data.transform == nil {
			return nil, canonicalInvalid(CodeLineageMismatch, "steps["+itoa(index)+"]")
		}
		transforms[index] = FingerprintEnvelope{data: step.data.transform}
		expected, err := NewDerivedStateEnvelope(parent, transforms[index])
		if err != nil {
			return nil, err
		}
		if expected.data.descriptor.digest != step.data.descriptor.digest || step.data.parent != parent {
			return nil, canonicalInvalid(CodeLineageMismatch, "steps["+itoa(index)+"]")
		}
		expectedSteps[index] = step
		parent = CanonicalStateID(step.data.descriptor.digest)
	}
	if string(parent) != wire.Endpoint {
		return nil, canonicalInvalid(CodeEndpointMismatch, "endpoint")
	}
	data, err := buildCanonicalLineage(anchor, transforms, recipe)
	if err != nil {
		return nil, err
	}
	data.steps = expectedSteps
	return data, nil
}
func (l CanonicalRecipeEnvelope) MarshalJSON() ([]byte, error) {
	return marshalCanonicalLineage(l.data)
}
func (l *CanonicalRecipeEnvelope) UnmarshalJSON(raw []byte) error {
	if l == nil {
		return canonicalInvalid(CodeShapeInvalid, "")
	}
	data, err := unmarshalCanonicalLineage(raw, true)
	if err != nil {
		return err
	}
	l.data = data
	return nil
}
func (l CanonicalRelativeEnvelope) MarshalJSON() ([]byte, error) {
	return marshalCanonicalLineage(l.data)
}
func (l *CanonicalRelativeEnvelope) UnmarshalJSON(raw []byte) error {
	if l == nil {
		return canonicalInvalid(CodeShapeInvalid, "")
	}
	data, err := unmarshalCanonicalLineage(raw, false)
	if err != nil {
		return err
	}
	l.data = data
	return nil
}
