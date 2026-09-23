package runtimev2

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

type goldenFixture struct {
	Case            string `json:"case"`
	ComparisonGroup string `json:"comparison_group"`
	Input           struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	} `json:"input"`
	Expected goldenExpected `json:"expected"`
}

type goldenExpected struct {
	Canonical struct {
		Factory       string   `json:"factory"`
		Transforms    []string `json:"transforms"`
		DerivedStates []string `json:"derived_states"`
	} `json:"canonical"`
	TransformFingerprints []string          `json:"transform_fingerprints"`
	States                []json.RawMessage `json:"states"`
	Endpoint              string            `json:"endpoint"`
}

type relativeGoldenInput struct {
	SchemaVersion string            `json:"schema_version"`
	Anchor        StateID           `json:"anchor"`
	Transforms    []json.RawMessage `json:"transforms"`
}

func TestGoldenVectors(t *testing.T) {
	fixtures := loadGoldenFixtures(t)
	groups := map[string]string{}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Case, func(t *testing.T) {
			endpoint := verifyGoldenFixture(t, fixture)
			if fixture.ComparisonGroup != "" {
				if prior, ok := groups[fixture.ComparisonGroup]; ok && prior != endpoint {
					t.Fatalf("comparison group endpoint = %s, want %s", endpoint, prior)
				}
				groups[fixture.ComparisonGroup] = endpoint
			}
		})
	}
}

func TestGoldenVectorsFreshProcess(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestGoldenVectors$")
	command.Env = append(os.Environ(), "RUNTIME_V2_GOLDEN_HELPER=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fresh-process golden verification failed: %v\n%s", err, output)
	}
}

func loadGoldenFixtures(t testing.TB) []goldenFixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "golden", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures := make([]goldenFixture, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var fixture goldenFixture
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func verifyGoldenFixture(t *testing.T, fixture goldenFixture) string {
	t.Helper()
	var transforms []TransformProvenance
	var states []State
	var endpoint StateID
	var factoryCanonical string
	switch fixture.Input.Type {
	case "recipe":
		var recipe Recipe
		if err := DecodeJSON(fixture.Input.Value, &recipe); err != nil {
			t.Fatal(err)
		}
		lineage, err := Build(recipe)
		if err != nil {
			t.Fatal(err)
		}
		transforms = recipe.data.transforms
		states = append(states, lineage.Root())
		for _, step := range lineage.Steps() {
			states = append(states, step.State())
		}
		endpoint = lineage.Endpoint().ID()
		factoryCanonical = hex.EncodeToString(canonicalFactory(recipe.data.factory.data.identity))
	case "relative":
		var input relativeGoldenInput
		if err := decodeStrict(fixture.Input.Value, &input); err != nil {
			t.Fatal(err)
		}
		if input.SchemaVersion != SchemaVersion {
			t.Fatalf("schema version = %q", input.SchemaVersion)
		}
		transforms = make([]TransformProvenance, len(input.Transforms))
		for index := range input.Transforms {
			if err := DecodeJSON(input.Transforms[index], &transforms[index]); err != nil {
				t.Fatal(err)
			}
		}
		lineage, err := Extend(input.Anchor, transforms)
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range lineage.Steps() {
			states = append(states, step.State())
		}
		endpoint = lineage.EndpointID()
	default:
		t.Fatalf("unknown input type %q", fixture.Input.Type)
	}

	if factoryCanonical != fixture.Expected.Canonical.Factory {
		t.Fatalf("factory canonical bytes differ")
	}
	if string(endpoint) != fixture.Expected.Endpoint {
		t.Fatalf("endpoint = %s, want %s", endpoint, fixture.Expected.Endpoint)
	}
	if len(transforms) != len(fixture.Expected.TransformFingerprints) || len(states) != len(fixture.Expected.States) {
		t.Fatal("golden collection length differs")
	}
	parentIndex := 0
	if fixture.Input.Type == "recipe" {
		parentIndex = 1
	}
	for index, transform := range transforms {
		canonical := hex.EncodeToString(canonicalTransform(transform.data.identity))
		if canonical != fixture.Expected.Canonical.Transforms[index] {
			t.Fatalf("transform %d canonical bytes differ", index)
		}
		fingerprint, _ := TransformFingerprint(transform.data.identity)
		if string(fingerprint) != fixture.Expected.TransformFingerprints[index] {
			t.Fatalf("transform %d fingerprint differs", index)
		}
		state := states[index+parentIndex]
		parent, _ := parseDigest(string(state.ParentID()), "parent_id")
		digest, _ := parseDigest(string(state.TransformFingerprint()), "transform_fingerprint")
		canonicalState := hex.EncodeToString(encodeRecord(stateDomain, []canonicalField{{tag: 1, payload: parent}, {tag: 2, payload: digest}}))
		if canonicalState != fixture.Expected.Canonical.DerivedStates[index] {
			t.Fatalf("derived state %d canonical bytes differ", index)
		}
	}
	for index, state := range states {
		actual, _ := json.Marshal(state)
		if !equalJSON(actual, fixture.Expected.States[index]) {
			t.Fatalf("state %d differs:\n%s\n%s", index, actual, fixture.Expected.States[index])
		}
	}
	return string(endpoint)
}

func equalJSON(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
