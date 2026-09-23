package runtimev2

import (
	"encoding/json"
)

// StateID is a lowercase SHA-256 digest identifying one logical state.
type StateID string

// Fingerprint is a lowercase SHA-256 digest of resolved identity.
type Fingerprint string

type stateData struct {
	kind                 string
	id                   StateID
	parentID             StateID
	factoryFingerprint   Fingerprint
	transformFingerprint Fingerprint
}

// State is an immutable logical factory or derived state.
type State struct{ data *stateData }

func (s State) ID() StateID {
	if s.data == nil {
		return ""
	}
	return s.data.id
}
func (s State) ParentID() StateID {
	if s.data == nil {
		return ""
	}
	return s.data.parentID
}
func (s State) FactoryFingerprint() Fingerprint {
	if s.data == nil {
		return ""
	}
	return s.data.factoryFingerprint
}
func (s State) TransformFingerprint() Fingerprint {
	if s.data == nil {
		return ""
	}
	return s.data.transformFingerprint
}

// FactoryState derives a root state solely from resolved factory identity.
func FactoryState(factory FactoryProvenance) (State, error) {
	if factory.data == nil {
		return State{}, invalid(CodeInvalidShape, "factory")
	}
	id := StateID(hashBytes(canonicalFactory(factory.data.identity)))
	return State{data: &stateData{kind: "factory", id: id, factoryFingerprint: Fingerprint(id)}}, nil
}

// TransformFingerprint hashes the canonical resolved transform identity.
func TransformFingerprint(transform ResolvedTransformIdentity) (Fingerprint, error) {
	if !transform.valid() {
		return "", invalid(CodeInvalidShape, "transform")
	}
	return Fingerprint(hashBytes(canonicalTransform(transform))), nil
}

// Derive creates one logical step from a parent and resolved transform.
func Derive(parent StateID, transform TransformProvenance) (LineageStep, error) {
	parentDigest, err := parseDigest(string(parent), "parent_id")
	if err != nil {
		return LineageStep{}, err
	}
	if transform.data == nil {
		return LineageStep{}, invalid(CodeInvalidShape, "transform")
	}
	fingerprint, err := TransformFingerprint(transform.data.identity)
	if err != nil {
		return LineageStep{}, err
	}
	transformDigest, _ := parseDigest(string(fingerprint), "transform_fingerprint")
	id := StateID(hashRecord(stateDomain, []canonicalField{{tag: 1, payload: parentDigest}, {tag: 2, payload: transformDigest}}))
	state := State{data: &stateData{kind: "derived", id: id, parentID: parent, transformFingerprint: fingerprint}}
	return LineageStep{data: &lineageStepData{transform: transform, fingerprint: fingerprint, state: state}}, nil
}

type stateWire struct {
	SchemaVersion        *string         `json:"schema_version"`
	StateKind            *string         `json:"state_kind"`
	ID                   *string         `json:"id"`
	FactoryFingerprint   json.RawMessage `json:"factory_fingerprint,omitempty"`
	ParentID             json.RawMessage `json:"parent_id,omitempty"`
	TransformFingerprint json.RawMessage `json:"transform_fingerprint,omitempty"`
}

func (s State) MarshalJSON() ([]byte, error) {
	if s.data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	version, kind, id := SchemaVersion, s.data.kind, string(s.data.id)
	wire := stateWire{SchemaVersion: &version, StateKind: &kind, ID: &id}
	if kind == "factory" {
		value := string(s.data.factoryFingerprint)
		wire.FactoryFingerprint, _ = json.Marshal(value)
	} else {
		parent, value := string(s.data.parentID), string(s.data.transformFingerprint)
		wire.ParentID, _ = json.Marshal(parent)
		wire.TransformFingerprint, _ = json.Marshal(value)
	}
	return json.Marshal(wire)
}

func (s *State) UnmarshalJSON(data []byte) error {
	if s == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire stateWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	if wire.SchemaVersion == nil {
		return invalid(CodeInvalidShape, "schema_version")
	}
	if *wire.SchemaVersion != SchemaVersion {
		return invalid(CodeInvalidVersion, "schema_version")
	}
	if wire.StateKind == nil {
		return invalid(CodeInvalidShape, "state_kind")
	}
	if wire.ID == nil {
		return invalid(CodeInvalidShape, "id")
	}
	if _, err := parseDigest(*wire.ID, "id"); err != nil {
		return err
	}
	var value State
	switch *wire.StateKind {
	case "factory":
		if len(wire.FactoryFingerprint) == 0 || isJSONNull(wire.FactoryFingerprint) || len(wire.ParentID) != 0 || len(wire.TransformFingerprint) != 0 {
			return invalid(CodeInvalidShape, "state_kind")
		}
		var fingerprint string
		if err := json.Unmarshal(wire.FactoryFingerprint, &fingerprint); err != nil {
			return invalid(CodeInvalidShape, "factory_fingerprint")
		}
		if _, err := parseDigest(fingerprint, "factory_fingerprint"); err != nil {
			return err
		}
		if *wire.ID != fingerprint {
			return invalid(CodeIntegrityMismatch, "id")
		}
		value = State{data: &stateData{kind: "factory", id: StateID(*wire.ID), factoryFingerprint: Fingerprint(fingerprint)}}
	case "derived":
		if len(wire.FactoryFingerprint) != 0 || len(wire.ParentID) == 0 || isJSONNull(wire.ParentID) || len(wire.TransformFingerprint) == 0 || isJSONNull(wire.TransformFingerprint) {
			return invalid(CodeInvalidShape, "state_kind")
		}
		var parentID, fingerprint string
		if err := json.Unmarshal(wire.ParentID, &parentID); err != nil {
			return invalid(CodeInvalidShape, "parent_id")
		}
		if err := json.Unmarshal(wire.TransformFingerprint, &fingerprint); err != nil {
			return invalid(CodeInvalidShape, "transform_fingerprint")
		}
		parent, err := parseDigest(parentID, "parent_id")
		if err != nil {
			return err
		}
		transform, err := parseDigest(fingerprint, "transform_fingerprint")
		if err != nil {
			return err
		}
		expected := hashRecord(stateDomain, []canonicalField{{tag: 1, payload: parent}, {tag: 2, payload: transform}})
		if expected != *wire.ID {
			return invalid(CodeIntegrityMismatch, "id")
		}
		value = State{data: &stateData{kind: "derived", id: StateID(*wire.ID), parentID: StateID(parentID), transformFingerprint: Fingerprint(fingerprint)}}
	default:
		return invalid(CodeInvalidValue, "state_kind")
	}
	*s = value
	return nil
}
