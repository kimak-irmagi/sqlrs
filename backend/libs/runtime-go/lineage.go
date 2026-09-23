package runtimev2

import "encoding/json"

type lineageStepData struct {
	transform   TransformProvenance
	fingerprint Fingerprint
	state       State
}

// LineageStep binds transform provenance and fingerprint to its derived state.
type LineageStep struct{ data *lineageStepData }

func (s LineageStep) Transform() TransformProvenance {
	if s.data == nil {
		return TransformProvenance{}
	}
	return s.data.transform
}
func (s LineageStep) Fingerprint() Fingerprint {
	if s.data == nil {
		return ""
	}
	return s.data.fingerprint
}
func (s LineageStep) State() State {
	if s.data == nil {
		return State{}
	}
	return s.data.state
}

type recipeData struct {
	factory    FactoryProvenance
	transforms []TransformProvenance
}

// Recipe is an immutable factory plus an ordered transform sequence.
type Recipe struct{ data *recipeData }

// NewRecipe validates provenance and takes a defensive copy of transforms.
func NewRecipe(factory FactoryProvenance, transforms []TransformProvenance) (Recipe, error) {
	if factory.data == nil {
		return Recipe{}, invalid(CodeInvalidShape, "factory")
	}
	if len(transforms) > MaxTransforms {
		return Recipe{}, invalid(CodeTooLarge, "transforms")
	}
	copyTransforms := append([]TransformProvenance(nil), transforms...)
	for index, transform := range copyTransforms {
		if transform.data == nil {
			return Recipe{}, invalid(CodeInvalidShape, "transforms["+itoa(index)+"]")
		}
	}
	return Recipe{data: &recipeData{factory: factory, transforms: copyTransforms}}, nil
}

// Factory returns the recipe's immutable resolved factory provenance.
func (r Recipe) Factory() FactoryProvenance {
	if r.data == nil {
		return FactoryProvenance{}
	}
	return r.data.factory
}

// Transforms returns a defensive copy of the ordered transform provenance.
func (r Recipe) Transforms() []TransformProvenance {
	if r.data == nil {
		return nil
	}
	return append([]TransformProvenance(nil), r.data.transforms...)
}

type recipeLineageData struct {
	factory FactoryProvenance
	root    State
	steps   []LineageStep
}
type relativeLineageData struct {
	anchor StateID
	steps  []LineageStep
}

// RecipeLineage contains a factory root and every derived recipe step.
type RecipeLineage struct{ data *recipeLineageData }

// RelativeLineage contains the steps derived after an external anchor.
type RelativeLineage struct{ data *relativeLineageData }

// Build deterministically derives the complete lineage for recipe.
func Build(recipe Recipe) (RecipeLineage, error) {
	if recipe.data == nil {
		return RecipeLineage{}, invalid(CodeInvalidShape, "recipe")
	}
	root, err := FactoryState(recipe.data.factory)
	if err != nil {
		return RecipeLineage{}, err
	}
	steps := make([]LineageStep, 0, len(recipe.data.transforms))
	parent := root.ID()
	for index, transform := range recipe.data.transforms {
		step, err := Derive(parent, transform)
		if err != nil {
			return RecipeLineage{}, prefixError(err, "transforms["+itoa(index)+"]")
		}
		steps = append(steps, step)
		parent = step.State().ID()
	}
	return RecipeLineage{data: &recipeLineageData{factory: recipe.data.factory, root: root, steps: steps}}, nil
}

// Extend derives an ordered transform suffix from an existing logical state.
func Extend(anchor StateID, transforms []TransformProvenance) (RelativeLineage, error) {
	if _, err := parseDigest(string(anchor), "anchor"); err != nil {
		return RelativeLineage{}, err
	}
	if len(transforms) > MaxTransforms {
		return RelativeLineage{}, invalid(CodeTooLarge, "transforms")
	}
	steps := make([]LineageStep, 0, len(transforms))
	parent := anchor
	for index, transform := range transforms {
		step, err := Derive(parent, transform)
		if err != nil {
			return RelativeLineage{}, prefixError(err, "transforms["+itoa(index)+"]")
		}
		steps = append(steps, step)
		parent = step.State().ID()
	}
	return RelativeLineage{data: &relativeLineageData{anchor: anchor, steps: steps}}, nil
}

func (l RecipeLineage) Root() State {
	if l.data == nil {
		return State{}
	}
	return l.data.root
}

// Factory returns the provenance from which the lineage root was derived.
func (l RecipeLineage) Factory() FactoryProvenance {
	if l.data == nil {
		return FactoryProvenance{}
	}
	return l.data.factory
}
func (l RecipeLineage) Steps() []LineageStep {
	if l.data == nil {
		return nil
	}
	return append([]LineageStep(nil), l.data.steps...)
}
func (l RecipeLineage) Endpoint() State {
	if l.data == nil {
		return State{}
	}
	if len(l.data.steps) == 0 {
		return l.data.root
	}
	return l.data.steps[len(l.data.steps)-1].State()
}
func (l RelativeLineage) Steps() []LineageStep {
	if l.data == nil {
		return nil
	}
	return append([]LineageStep(nil), l.data.steps...)
}

// Anchor returns the pre-existing StateID extended by this relative lineage.
func (l RelativeLineage) Anchor() StateID {
	if l.data == nil {
		return ""
	}
	return l.data.anchor
}
func (l RelativeLineage) EndpointID() StateID {
	if l.data == nil {
		return ""
	}
	if len(l.data.steps) == 0 {
		return l.data.anchor
	}
	return l.data.steps[len(l.data.steps)-1].State().ID()
}

type stepWire struct {
	Transform            json.RawMessage `json:"transform"`
	TransformFingerprint *string         `json:"transform_fingerprint"`
	State                json.RawMessage `json:"state"`
}

func (s LineageStep) MarshalJSON() ([]byte, error) {
	if s.data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	transform, _ := json.Marshal(s.data.transform)
	state, _ := json.Marshal(s.data.state)
	fingerprint := string(s.data.fingerprint)
	return json.Marshal(stepWire{Transform: transform, TransformFingerprint: &fingerprint, State: state})
}
func (s *LineageStep) UnmarshalJSON(data []byte) error {
	if s == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire stepWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	if len(wire.Transform) == 0 || isJSONNull(wire.Transform) {
		return invalid(CodeInvalidShape, "transform")
	}
	if wire.TransformFingerprint == nil {
		return invalid(CodeInvalidShape, "transform_fingerprint")
	}
	if len(wire.State) == 0 || isJSONNull(wire.State) {
		return invalid(CodeInvalidShape, "state")
	}
	var transform TransformProvenance
	if err := json.Unmarshal(wire.Transform, &transform); err != nil {
		return prefixError(err, "transform")
	}
	var state State
	if err := json.Unmarshal(wire.State, &state); err != nil {
		return prefixError(err, "state")
	}
	if state.data == nil || state.data.kind != "derived" {
		return invalid(CodeInvalidShape, "state.state_kind")
	}
	expected, err := Derive(state.ParentID(), transform)
	if err != nil {
		return err
	}
	if string(expected.Fingerprint()) != *wire.TransformFingerprint {
		return invalid(CodeIntegrityMismatch, "transform_fingerprint")
	}
	if expected.State().ID() != state.ID() {
		return invalid(CodeIntegrityMismatch, "state.id")
	}
	*s = LineageStep{data: &lineageStepData{transform: transform, fingerprint: Fingerprint(*wire.TransformFingerprint), state: state}}
	return nil
}

type recipeWire struct {
	SchemaVersion *string            `json:"schema_version"`
	Factory       json.RawMessage    `json:"factory"`
	Transforms    *[]json.RawMessage `json:"transforms"`
}

func (r Recipe) MarshalJSON() ([]byte, error) {
	if r.data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	v := SchemaVersion
	f, _ := json.Marshal(r.data.factory)
	ts := make([]json.RawMessage, len(r.data.transforms))
	for i := range r.data.transforms {
		ts[i], _ = json.Marshal(r.data.transforms[i])
	}
	return json.Marshal(recipeWire{SchemaVersion: &v, Factory: f, Transforms: &ts})
}
func (r *Recipe) UnmarshalJSON(data []byte) error {
	if r == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var w recipeWire
	if err := decodeStrict(data, &w); err != nil {
		return err
	}
	if w.SchemaVersion == nil {
		return invalid(CodeInvalidShape, "schema_version")
	}
	if *w.SchemaVersion != SchemaVersion {
		return invalid(CodeInvalidVersion, "schema_version")
	}
	if len(w.Factory) == 0 {
		return invalid(CodeInvalidShape, "factory")
	}
	var f FactoryProvenance
	if err := json.Unmarshal(w.Factory, &f); err != nil {
		return prefixError(err, "factory")
	}
	if w.Transforms == nil {
		return invalid(CodeInvalidShape, "transforms")
	}
	ts := make([]TransformProvenance, len(*w.Transforms))
	for i := range *w.Transforms {
		if err := json.Unmarshal((*w.Transforms)[i], &ts[i]); err != nil {
			return prefixError(err, "transforms["+itoa(i)+"]")
		}
	}
	value, err := NewRecipe(f, ts)
	if err != nil {
		return err
	}
	*r = value
	return nil
}

type recipeLineageWire struct {
	SchemaVersion *string            `json:"schema_version"`
	Factory       json.RawMessage    `json:"factory"`
	Root          json.RawMessage    `json:"root"`
	Steps         *[]json.RawMessage `json:"steps"`
}

func (l RecipeLineage) MarshalJSON() ([]byte, error) {
	if l.data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	v := SchemaVersion
	f, _ := json.Marshal(l.data.factory)
	r, _ := json.Marshal(l.data.root)
	ss := make([]json.RawMessage, len(l.data.steps))
	for i := range l.data.steps {
		ss[i], _ = json.Marshal(l.data.steps[i])
	}
	return json.Marshal(recipeLineageWire{SchemaVersion: &v, Factory: f, Root: r, Steps: &ss})
}
func (l *RecipeLineage) UnmarshalJSON(data []byte) error {
	if l == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var w recipeLineageWire
	if err := decodeStrict(data, &w); err != nil {
		return err
	}
	if w.SchemaVersion == nil {
		return invalid(CodeInvalidShape, "schema_version")
	}
	if *w.SchemaVersion != SchemaVersion {
		return invalid(CodeInvalidVersion, "schema_version")
	}
	var f FactoryProvenance
	if len(w.Factory) == 0 {
		return invalid(CodeInvalidShape, "factory")
	}
	if err := json.Unmarshal(w.Factory, &f); err != nil {
		return prefixError(err, "factory")
	}
	var root State
	if len(w.Root) == 0 {
		return invalid(CodeInvalidShape, "root")
	}
	if err := json.Unmarshal(w.Root, &root); err != nil {
		return prefixError(err, "root")
	}
	if root.data == nil || root.data.kind != "factory" {
		return invalid(CodeInvalidShape, "root.state_kind")
	}
	expectedRoot, err := FactoryState(f)
	if err != nil {
		return err
	}
	if root.ID() != expectedRoot.ID() {
		return invalid(CodeIntegrityMismatch, "root.id")
	}
	if w.Steps == nil {
		return invalid(CodeInvalidShape, "steps")
	}
	steps, parent := make([]LineageStep, len(*w.Steps)), root.ID()
	for i := range *w.Steps {
		if err := json.Unmarshal((*w.Steps)[i], &steps[i]); err != nil {
			return prefixError(err, "steps["+itoa(i)+"]")
		}
		if steps[i].State().ParentID() != parent {
			return invalid(CodeIntegrityMismatch, "steps["+itoa(i)+"].state.parent_id")
		}
		parent = steps[i].State().ID()
	}
	l.data = &recipeLineageData{factory: f, root: root, steps: steps}
	return nil
}

type relativeLineageWire struct {
	SchemaVersion *string            `json:"schema_version"`
	Anchor        *string            `json:"anchor"`
	Steps         *[]json.RawMessage `json:"steps"`
}

func (l RelativeLineage) MarshalJSON() ([]byte, error) {
	if l.data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	v, a := SchemaVersion, string(l.data.anchor)
	ss := make([]json.RawMessage, len(l.data.steps))
	for i := range l.data.steps {
		ss[i], _ = json.Marshal(l.data.steps[i])
	}
	return json.Marshal(relativeLineageWire{SchemaVersion: &v, Anchor: &a, Steps: &ss})
}
func (l *RelativeLineage) UnmarshalJSON(data []byte) error {
	if l == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var w relativeLineageWire
	if err := decodeStrict(data, &w); err != nil {
		return err
	}
	if w.SchemaVersion == nil {
		return invalid(CodeInvalidShape, "schema_version")
	}
	if *w.SchemaVersion != SchemaVersion {
		return invalid(CodeInvalidVersion, "schema_version")
	}
	if w.Anchor == nil {
		return invalid(CodeInvalidShape, "anchor")
	}
	if _, err := parseDigest(*w.Anchor, "anchor"); err != nil {
		return err
	}
	if w.Steps == nil {
		return invalid(CodeInvalidShape, "steps")
	}
	steps, parent := make([]LineageStep, len(*w.Steps)), StateID(*w.Anchor)
	for i := range *w.Steps {
		if err := json.Unmarshal((*w.Steps)[i], &steps[i]); err != nil {
			return prefixError(err, "steps["+itoa(i)+"]")
		}
		if steps[i].State().ParentID() != parent {
			return invalid(CodeIntegrityMismatch, "steps["+itoa(i)+"].state.parent_id")
		}
		parent = steps[i].State().ID()
	}
	l.data = &relativeLineageData{anchor: StateID(*w.Anchor), steps: steps}
	return nil
}
