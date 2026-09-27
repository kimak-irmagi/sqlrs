package runtimev2store

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func testStateID() runtimev2.StateID {
	return runtimev2.StateID("sha256:" + strings.Repeat("a", 64))
}

func validMaterializationInput() MaterializationInput {
	runtimeID, jobID, size, componentSize := "runtime-1", "job-1", int64(42), int64(21)
	return MaterializationInput{
		StateID: testStateID(), MaterializationID: "materialization-1", Backend: "local.snapshot",
		RuntimeID: &runtimeID, JobID: &jobID,
		CreatedAt: time.Date(2026, 9, 27, 12, 34, 56, 123, time.FixedZone("offset", 2*60*60)),
		SizeBytes: &size, Metadata: json.RawMessage(`{"b":2,"a":1}`),
		Components: []MaterializationComponentInput{
			{Name: "data", Kind: "directory", Locator: "snapshot://data", SizeBytes: &componentSize, Metadata: json.RawMessage(`{"z":true}`)},
			{Name: "config", Kind: "file", Locator: "snapshot://config"},
		},
	}
}

func TestNewMaterializationCanonicalizesAndCopies(t *testing.T) {
	input := validMaterializationInput()
	value, err := NewMaterialization(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Metadata[2] = 'x'
	input.Components[0].Metadata[2] = 'x'
	*input.RuntimeID = "changed"

	if value.RecordVersion() != RecordVersion || value.StateID() != testStateID() ||
		value.MaterializationID() != "materialization-1" || value.Backend() != "local.snapshot" {
		t.Fatalf("identity accessors returned unexpected values")
	}
	if got := value.CreatedAt(); got != "2026-09-27T10:34:56.000000123Z" {
		t.Fatalf("created_at = %q", got)
	}
	if got := string(value.Metadata()); got != `{"a":1,"b":2}` {
		t.Fatalf("metadata = %s", got)
	}
	components := value.Components()
	if len(components) != 2 || components[0].Name() != "config" || components[1].Name() != "data" {
		t.Fatalf("components are not in canonical name order: %+v", components)
	}
	if got := string(components[0].Metadata()); got != `{}` {
		t.Fatalf("default metadata = %s", got)
	}
	metadata := value.Metadata()
	metadata[0] = '['
	components[1].metadata[0] = '['
	if !bytes.Equal(value.Metadata(), []byte(`{"a":1,"b":2}`)) ||
		!bytes.Equal(value.Components()[1].Metadata(), []byte(`{"z":true}`)) {
		t.Fatal("accessors exposed mutable storage")
	}
}

func TestNewMaterializationTreatsComponentOrderAsNonSemantic(t *testing.T) {
	input := validMaterializationInput()
	a, err := NewMaterialization(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Components[0], input.Components[1] = input.Components[1], input.Components[0]
	b, err := NewMaterialization(input)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Equal(b) {
		t.Fatal("component permutation changed the materialization")
	}
}

func TestNewMaterializationRejectsInvalidBounds(t *testing.T) {
	control := "bad\nvalue"
	negative := int64(-1)
	cases := map[string]func(*MaterializationInput){
		"state id":           func(v *MaterializationInput) { v.StateID = "bad" },
		"empty id":           func(v *MaterializationInput) { v.MaterializationID = "" },
		"long id":            func(v *MaterializationInput) { v.MaterializationID = strings.Repeat("x", 257) },
		"control id":         func(v *MaterializationInput) { v.MaterializationID = control },
		"backend grammar":    func(v *MaterializationInput) { v.Backend = "UPPER" },
		"runtime id":         func(v *MaterializationInput) { value := strings.Repeat("x", 257); v.RuntimeID = &value },
		"job id":             func(v *MaterializationInput) { value := ""; v.JobID = &value },
		"timestamp":          func(v *MaterializationInput) { v.CreatedAt = time.Time{} },
		"size":               func(v *MaterializationInput) { v.SizeBytes = &negative },
		"metadata shape":     func(v *MaterializationInput) { v.Metadata = json.RawMessage(`[]`) },
		"metadata duplicate": func(v *MaterializationInput) { v.Metadata = json.RawMessage(`{"a":1,"a":2}`) },
		"metadata trailing":  func(v *MaterializationInput) { v.Metadata = json.RawMessage(`{} {}`) },
		"metadata size": func(v *MaterializationInput) {
			v.Metadata = json.RawMessage(`{"a":"` + strings.Repeat("x", MaxMetadataBytes) + `"}`)
		},
		"component count":     func(v *MaterializationInput) { v.Components = make([]MaterializationComponentInput, MaxComponents+1) },
		"duplicate component": func(v *MaterializationInput) { v.Components = append(v.Components, v.Components[0]) },
		"component name":      func(v *MaterializationInput) { v.Components[0].Name = "UPPER" },
		"component kind":      func(v *MaterializationInput) { v.Components[0].Kind = "UPPER" },
		"component locator":   func(v *MaterializationInput) { v.Components[0].Locator = strings.Repeat("x", MaxLocatorBytes+1) },
		"component size":      func(v *MaterializationInput) { v.Components[0].SizeBytes = &negative },
		"component metadata":  func(v *MaterializationInput) { v.Components[0].Metadata = json.RawMessage(`null`) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			input := validMaterializationInput()
			change(&input)
			if _, err := NewMaterialization(input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want %v", err, ErrInvalid)
			}
		})
	}
}
