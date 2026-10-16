package runtimev2

import "encoding/binary"

// CanonicalValueKind identifies an existing canonical-v1 node, not a wire tag.
// Requirements: docs/architecture/runtime-v2-execution-schema-structure.md §8.
type CanonicalValueKind string

const (
	// CanonicalValueKindNull denotes a valid null node, unlike the zero value.
	CanonicalValueKindNull CanonicalValueKind = "null"
	// CanonicalValueKindString denotes an immutable UTF-8 string.
	CanonicalValueKindString CanonicalValueKind = "string"
	// CanonicalValueKindList denotes an ordered collection.
	CanonicalValueKindList CanonicalValueKind = "list"
	// CanonicalValueKindMap denotes a map in canonical encoded-key order.
	CanonicalValueKindMap CanonicalValueKind = "map"
	// CanonicalValueKindSet denotes members in canonical encoded-value order.
	CanonicalValueKindSet CanonicalValueKind = "set"
)

// Kind returns the node kind, or zero for an invalid CanonicalValue.
func (v CanonicalValue) Kind() CanonicalValueKind {
	if v.data == nil {
		return ""
	}
	switch binary.BigEndian.Uint16(v.data.encoded[:2]) {
	case canonicalNullTag:
		return CanonicalValueKindNull
	case canonicalStringTag:
		return CanonicalValueKindString
	case canonicalListTag:
		return CanonicalValueKindList
	case canonicalMapTag:
		return CanonicalValueKindMap
	case canonicalSetTag:
		return CanonicalValueKindSet
	}
	return ""
}

// StringValue returns a string node's text; wrong-kind and zero return "", false.
func (v CanonicalValue) StringValue() (string, bool) {
	if v.Kind() != CanonicalValueKindString {
		return "", false
	}
	return string(v.data.encoded[10:]), true
}

// Elements returns defensive immutable children for a list/set. Valid empty
// collections return a non-nil empty slice; all other kinds return nil, false.
func (v CanonicalValue) Elements() ([]CanonicalValue, bool) {
	if kind := v.Kind(); kind != CanonicalValueKindList && kind != CanonicalValueKindSet {
		return nil, false
	}
	payload := v.data.encoded[10:]
	count, offset := int(binary.BigEndian.Uint32(payload[:4])), 4
	result := make([]CanonicalValue, count)
	for index := range result {
		// Opaque nodes are already fully validated by constructors/the decoder.
		// Reuse its child framing, validation and immutable ownership here.
		child, next, _ := framedChild(payload, offset)
		result[index], _, _ = decodeCanonicalNode(child, 1)
		offset = next
	}
	return result, true
}

// Entries returns defensive map entries in canonical encoded-key order.
// Children own their bytes; zero/wrong-kind return nil, false.
func (v CanonicalValue) Entries() ([]CanonicalMapEntry, bool) {
	if v.Kind() != CanonicalValueKindMap {
		return nil, false
	}
	payload := v.data.encoded[10:]
	count, offset := int(binary.BigEndian.Uint32(payload[:4])), 4
	result := make([]CanonicalMapEntry, count)
	for index := range result {
		keyBytes, next, _ := framedChild(payload, offset)
		key, _, _ := decodeCanonicalNode(keyBytes, 1)
		result[index].Key, _ = key.StringValue()
		child, next, _ := framedChild(payload, next)
		result[index].Value, _, _ = decodeCanonicalNode(child, 1)
		offset = next
	}
	return result, true
}

// EncodedSize reads the existing framed byte length without copying, allocation
// or decoding. An invalid value has size zero. Requirements: structure §8–9.
func (v CanonicalValue) EncodedSize() int {
	if v.data == nil {
		return 0
	}
	return len(v.data.encoded)
}
