package runtimev2

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strings"
	"unicode/utf8"
)

// CanonicalSchemaVersion identifies the typed canonical-v1 semantic revision.
// It is deliberately distinct from the immutable legacy SchemaVersion.
// Requirements: runtime-v2-canonical-contract-flow.md.
const CanonicalSchemaVersion = "sqlrs.runtime.v2.canonical.v1"

const (
	CanonicalIdentityValueDomain            = "sqlrs.runtime.v2/identity-value"
	CanonicalFactoryStateDomain             = "sqlrs.runtime.v2.canonical.v1/factory-state"
	CanonicalTransformDomain                = "sqlrs.runtime.v2.canonical.v1/transform"
	CanonicalResolvedExtensionDomain        = "sqlrs.runtime.v2.canonical.v1/resolved-extension"
	CanonicalDerivedStateDomain             = "sqlrs.runtime.v2.canonical.v1/state"
	MaxCanonicalValueDepth                  = 32
	MaxCanonicalValueNodes                  = 4096
	MaxCanonicalCollectionMembers           = 256
	MaxCanonicalStringBytes                 = 4096
	MaxCanonicalMapKeyBytes                 = 1024
	MaxCanonicalValueBytes                  = 1 << 20
	MaxCanonicalEnvelopeBytes               = 4 << 20
	canonicalNullTag                 uint16 = 0
	canonicalStringTag               uint16 = 1
	canonicalListTag                 uint16 = 2
	canonicalMapTag                  uint16 = 3
	canonicalSetTag                  uint16 = 4
)

type canonicalValueData struct {
	encoded []byte
	depth   int
	nodes   int
}

// CanonicalValue is an immutable type-tagged structured identity value.
type CanonicalValue struct{ data *canonicalValueData }

// CanonicalValueToken is the non-reversible civ1 digest reference for a value.
type CanonicalValueToken struct {
	digest [sha256.Size]byte
	valid  bool
}

// CanonicalMapEntry preserves map source order long enough to detect duplicates.
type CanonicalMapEntry struct {
	Key   string
	Value CanonicalValue
}

// CanonicalNull returns the single valid null value.
func CanonicalNull() CanonicalValue {
	return canonicalNode(canonicalNullTag, nil, 1, 1)
}

// CanonicalString validates and copies an unnormalized UTF-8 string.
func CanonicalString(value string) (CanonicalValue, error) {
	if !utf8.ValidString(value) {
		return CanonicalValue{}, canonicalInvalid(CodeValueInvalid, "value")
	}
	if len(value) > MaxCanonicalStringBytes {
		return CanonicalValue{}, canonicalInvalid(CodeLimitExceeded, "value")
	}
	return checkedCanonicalNode(canonicalStringTag, []byte(value), 1, 1, "value")
}

// CanonicalList retains caller order and validates aggregate budgets.
func CanonicalList(values []CanonicalValue) (CanonicalValue, error) {
	if len(values) > MaxCanonicalCollectionMembers {
		return CanonicalValue{}, canonicalInvalid(CodeLimitExceeded, "members")
	}
	children := make([][]byte, len(values))
	depth, nodes := 1, 1
	for index, value := range values {
		if !value.Valid() {
			return CanonicalValue{}, canonicalInvalid(CodeShapeInvalid, "members["+itoa(index)+"]")
		}
		children[index] = value.CanonicalBytes()
		if value.data.depth+1 > depth {
			depth = value.data.depth + 1
		}
		nodes += value.data.nodes
	}
	return canonicalCollection(canonicalListTag, children, depth, nodes, "members")
}

// CanonicalMap rejects duplicate source keys before sorting encoded entries.
func CanonicalMap(entries []CanonicalMapEntry) (CanonicalValue, error) {
	if len(entries) > MaxCanonicalCollectionMembers {
		return CanonicalValue{}, canonicalInvalid(CodeLimitExceeded, "entries")
	}
	type encodedEntry struct {
		key, value   []byte
		depth, nodes int
	}
	encoded := make([]encodedEntry, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		path := "entries[" + itoa(index) + "]"
		if _, exists := seen[entry.Key]; exists {
			return CanonicalValue{}, canonicalInvalid(CodeNonCanonical, path+".key")
		}
		seen[entry.Key] = struct{}{}
		if !utf8.ValidString(entry.Key) {
			return CanonicalValue{}, canonicalInvalid(CodeValueInvalid, path+".key")
		}
		if len(entry.Key) > MaxCanonicalMapKeyBytes {
			return CanonicalValue{}, canonicalInvalid(CodeLimitExceeded, path+".key")
		}
		if !entry.Value.Valid() {
			return CanonicalValue{}, canonicalInvalid(CodeShapeInvalid, path+".value")
		}
		key, _ := CanonicalString(entry.Key)
		encoded[index] = encodedEntry{key.CanonicalBytes(), entry.Value.CanonicalBytes(), entry.Value.data.depth, entry.Value.data.nodes}
	}
	sort.Slice(encoded, func(i, j int) bool { return bytes.Compare(encoded[i].key, encoded[j].key) < 0 })
	payload := append([]byte{}, encodeUint32(uint32(len(encoded)))...)
	depth, nodes := 1, 1
	for _, entry := range encoded {
		payload = append(payload, encodeBytes(entry.key)...)
		payload = append(payload, encodeBytes(entry.value)...)
		childDepth := entry.depth + 1
		if childDepth > depth {
			depth = childDepth
		}
		nodes += 1 + entry.nodes // the string key node plus the value subtree
	}
	return checkedCanonicalNode(canonicalMapTag, payload, depth, nodes, "entries")
}

// CanonicalSet sorts complete encoded members and rejects duplicates.
func CanonicalSet(values []CanonicalValue) (CanonicalValue, error) {
	if len(values) > MaxCanonicalCollectionMembers {
		return CanonicalValue{}, canonicalInvalid(CodeLimitExceeded, "members")
	}
	type member struct {
		encoded      []byte
		depth, nodes int
	}
	members := make([]member, len(values))
	for index, value := range values {
		if !value.Valid() {
			return CanonicalValue{}, canonicalInvalid(CodeShapeInvalid, "members["+itoa(index)+"]")
		}
		members[index] = member{value.CanonicalBytes(), value.data.depth, value.data.nodes}
	}
	sort.Slice(members, func(i, j int) bool { return bytes.Compare(members[i].encoded, members[j].encoded) < 0 })
	children := make([][]byte, len(members))
	depth, nodes := 1, 1
	for index, value := range members {
		if index > 0 && bytes.Equal(members[index-1].encoded, value.encoded) {
			return CanonicalValue{}, canonicalInvalid(CodeNonCanonical, "members["+itoa(index)+"]")
		}
		children[index] = value.encoded
		if value.depth+1 > depth {
			depth = value.depth + 1
		}
		nodes += value.nodes
	}
	return canonicalCollection(canonicalSetTag, children, depth, nodes, "members")
}

func canonicalCollection(tag uint16, children [][]byte, depth, nodes int, path string) (CanonicalValue, error) {
	payload := append([]byte{}, encodeUint32(uint32(len(children)))...)
	for _, child := range children {
		payload = append(payload, encodeBytes(child)...)
	}
	return checkedCanonicalNode(tag, payload, depth, nodes, path)
}

func canonicalNode(tag uint16, payload []byte, depth, nodes int) CanonicalValue {
	encoded := append(encodeUint16(tag), encodeBytes(payload)...)
	return CanonicalValue{data: &canonicalValueData{encoded: encoded, depth: depth, nodes: nodes}}
}

func checkedCanonicalNode(tag uint16, payload []byte, depth, nodes int, path string) (CanonicalValue, error) {
	if depth > MaxCanonicalValueDepth || nodes > MaxCanonicalValueNodes {
		return CanonicalValue{}, canonicalInvalid(CodeLimitExceeded, path)
	}
	value := canonicalNode(tag, payload, depth, nodes)
	if len(value.data.encoded) > MaxCanonicalValueBytes {
		return CanonicalValue{}, canonicalInvalid(CodeLimitExceeded, path)
	}
	return value, nil
}

// Valid reports whether the opaque value was constructed or decoded safely.
func (v CanonicalValue) Valid() bool { return v.data != nil }

// CanonicalBytes returns a defensive copy of the framed node bytes.
func (v CanonicalValue) CanonicalBytes() []byte {
	if v.data == nil {
		return nil
	}
	return append([]byte(nil), v.data.encoded...)
}

// Depth returns the root-at-one tree depth, or zero for an invalid value.
func (v CanonicalValue) Depth() int {
	if v.data == nil {
		return 0
	}
	return v.data.depth
}

// NodeCount returns the total key/value/member node count.
func (v CanonicalValue) NodeCount() int {
	if v.data == nil {
		return 0
	}
	return v.data.nodes
}

// Token hashes the dedicated domain and complete framed root node.
func (v CanonicalValue) Token() CanonicalValueToken {
	if v.data == nil {
		return CanonicalValueToken{}
	}
	preimage := append(encodeString(CanonicalIdentityValueDomain), encodeBytes(v.data.encoded)...)
	return CanonicalValueToken{digest: sha256.Sum256(preimage), valid: true}
}

// Valid reports whether the token has passed strict parsing or was derived.
func (t CanonicalValueToken) Valid() bool { return t.valid }

// String returns the external civ1 token or an empty string for the zero value.
func (t CanonicalValueToken) String() string {
	if !t.valid {
		return ""
	}
	return "civ1:sha256:" + hex.EncodeToString(t.digest[:])
}

// ParseCanonicalValueToken strictly parses the one supported token format.
func ParseCanonicalValueToken(value string) (CanonicalValueToken, error) {
	const prefix = "civ1:sha256:"
	if len(value) != len(prefix)+sha256.Size*2 || !strings.HasPrefix(value, prefix) {
		return CanonicalValueToken{}, canonicalInvalid(CodeValueInvalid, "token")
	}
	raw := value[len(prefix):]
	if raw != strings.ToLower(raw) {
		return CanonicalValueToken{}, canonicalInvalid(CodeValueInvalid, "token")
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != sha256.Size {
		return CanonicalValueToken{}, canonicalInvalid(CodeValueInvalid, "token")
	}
	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	return CanonicalValueToken{digest: digest, valid: true}, nil
}

// ParseCanonicalValueEnvelope decodes exactly one canonical framed node.
func ParseCanonicalValueEnvelope(raw []byte) (CanonicalValue, error) {
	if len(raw) > MaxCanonicalValueBytes {
		return CanonicalValue{}, canonicalInvalid(CodeLimitExceeded, "value")
	}
	value, consumed, err := decodeCanonicalNode(raw, 1)
	if err != nil {
		return CanonicalValue{}, err
	}
	if consumed != len(raw) {
		return CanonicalValue{}, canonicalInvalid(CodeShapeInvalid, "value")
	}
	return value, nil
}

func decodeCanonicalNode(raw []byte, depth int) (CanonicalValue, int, error) {
	if depth > MaxCanonicalValueDepth || len(raw) < 10 {
		return CanonicalValue{}, 0, canonicalInvalid(CodeShapeInvalid, "value")
	}
	tag := binary.BigEndian.Uint16(raw[:2])
	length := binary.BigEndian.Uint64(raw[2:10])
	if length > uint64(len(raw)-10) || length > MaxCanonicalValueBytes {
		return CanonicalValue{}, 0, canonicalInvalid(CodeShapeInvalid, "value")
	}
	end := 10 + int(length)
	payload := raw[10:end]
	switch tag {
	case canonicalNullTag:
		if len(payload) != 0 {
			return CanonicalValue{}, 0, canonicalInvalid(CodeShapeInvalid, "value")
		}
	case canonicalStringTag:
		if !utf8.Valid(payload) {
			return CanonicalValue{}, 0, canonicalInvalid(CodeValueInvalid, "value")
		}
		if len(payload) > MaxCanonicalStringBytes {
			return CanonicalValue{}, 0, canonicalInvalid(CodeLimitExceeded, "value")
		}
	case canonicalListTag, canonicalSetTag:
		if err := validateCanonicalMembers(payload, depth, tag == canonicalSetTag); err != nil {
			return CanonicalValue{}, 0, err
		}
	case canonicalMapTag:
		if err := validateCanonicalMap(payload, depth); err != nil {
			return CanonicalValue{}, 0, err
		}
	default:
		return CanonicalValue{}, 0, canonicalInvalid(CodeValueInvalid, "value")
	}
	value := canonicalNode(tag, payload, depth, 1)
	metrics, err := canonicalMetrics(raw[:end], 1)
	if err != nil {
		return CanonicalValue{}, 0, err
	}
	value.data.depth, value.data.nodes = metrics.depth, metrics.nodes
	return value, end, nil
}

type canonicalMetric struct{ depth, nodes int }

// canonicalAllocationPlan validates hostile collection counts before the
// decoder enters materialization, recursion, or sorting. It intentionally uses
// uint64 arithmetic so 32-bit builds reject conversion and multiplication
// overflow identically. Requirements: canonical-contract-tests.md LM03/LM04.
type canonicalAllocationPlan struct {
	count       int
	minimumSize int
}

func allocationPlan(count, framesPerMember uint64, available int, path string) (canonicalAllocationPlan, error) {
	if count > MaxCanonicalCollectionMembers {
		return canonicalAllocationPlan{}, canonicalInvalid(CodeLimitExceeded, path)
	}
	maxInt := uint64(^uint(0) >> 1)
	if count > maxInt || framesPerMember != 0 && count > maxInt/framesPerMember/8 {
		return canonicalAllocationPlan{}, canonicalInvalid(CodeLimitExceeded, path)
	}
	minimum := count * framesPerMember * 8
	if minimum > uint64(available) {
		return canonicalAllocationPlan{}, canonicalInvalid(CodeShapeInvalid, path)
	}
	return canonicalAllocationPlan{count: int(count), minimumSize: int(minimum)}, nil
}

func canonicalMetrics(raw []byte, depth int) (canonicalMetric, error) {
	if len(raw) < 10 {
		return canonicalMetric{}, canonicalInvalid(CodeShapeInvalid, "value")
	}
	tag := binary.BigEndian.Uint16(raw[:2])
	payloadLen := binary.BigEndian.Uint64(raw[2:10])
	if payloadLen > uint64(len(raw)-10) {
		return canonicalMetric{}, canonicalInvalid(CodeShapeInvalid, "value")
	}
	payload := raw[10 : 10+int(payloadLen)]
	result := canonicalMetric{depth: depth, nodes: 1}
	if tag != canonicalListTag && tag != canonicalSetTag && tag != canonicalMapTag {
		return result, nil
	}
	if len(payload) < 4 {
		return canonicalMetric{}, canonicalInvalid(CodeShapeInvalid, "value")
	}
	count, offset := int(binary.BigEndian.Uint32(payload[:4])), 4
	for index := 0; index < count; index++ {
		times := 1
		if tag == canonicalMapTag {
			times = 2
		}
		for part := 0; part < times; part++ {
			child, next, err := framedChild(payload, offset)
			if err != nil {
				return canonicalMetric{}, err
			}
			metric, err := canonicalMetrics(child, depth+1)
			if err != nil {
				return canonicalMetric{}, err
			}
			result.nodes += metric.nodes
			if metric.depth > result.depth {
				result.depth = metric.depth
			}
			offset = next
		}
	}
	if offset != len(payload) || result.nodes > MaxCanonicalValueNodes || result.depth > MaxCanonicalValueDepth {
		return canonicalMetric{}, canonicalInvalid(CodeLimitExceeded, "value")
	}
	return result, nil
}

func validateCanonicalMembers(payload []byte, depth int, sorted bool) error {
	if len(payload) < 4 {
		return canonicalInvalid(CodeShapeInvalid, "members")
	}
	plan, err := allocationPlan(uint64(binary.BigEndian.Uint32(payload[:4])), 1, len(payload)-4, "members")
	if err != nil {
		return err
	}
	count := plan.count
	offset := 4
	var previous []byte
	for index := 0; index < count; index++ {
		child, next, err := framedChild(payload, offset)
		if err != nil {
			return err
		}
		if _, consumed, err := decodeCanonicalNode(child, depth+1); err != nil || consumed != len(child) {
			if err != nil {
				return err
			}
			return canonicalInvalid(CodeShapeInvalid, "members")
		}
		if sorted && previous != nil && bytes.Compare(previous, child) >= 0 {
			return canonicalInvalid(CodeNonCanonical, "members["+itoa(index)+"]")
		}
		previous, offset = child, next
	}
	if offset != len(payload) {
		return canonicalInvalid(CodeShapeInvalid, "members")
	}
	return nil
}

func validateCanonicalMap(payload []byte, depth int) error {
	if len(payload) < 4 {
		return canonicalInvalid(CodeShapeInvalid, "entries")
	}
	plan, err := allocationPlan(uint64(binary.BigEndian.Uint32(payload[:4])), 2, len(payload)-4, "entries")
	if err != nil {
		return err
	}
	count := plan.count
	offset := 4
	var previous []byte
	for index := 0; index < count; index++ {
		key, next, err := framedChild(payload, offset)
		if err != nil {
			return err
		}
		keyValue, consumed, err := decodeCanonicalNode(key, depth+1)
		if err != nil || consumed != len(key) || binary.BigEndian.Uint16(key[:2]) != canonicalStringTag {
			return canonicalInvalid(CodeShapeInvalid, "entries["+itoa(index)+"].key")
		}
		if len(keyValue.data.encoded)-10 > MaxCanonicalMapKeyBytes {
			return canonicalInvalid(CodeLimitExceeded, "entries["+itoa(index)+"].key")
		}
		if previous != nil && bytes.Compare(previous, key) >= 0 {
			return canonicalInvalid(CodeNonCanonical, "entries["+itoa(index)+"].key")
		}
		value, after, err := framedChild(payload, next)
		if err != nil {
			return err
		}
		if _, consumed, err := decodeCanonicalNode(value, depth+1); err != nil || consumed != len(value) {
			if err != nil {
				return err
			}
			return canonicalInvalid(CodeShapeInvalid, "entries["+itoa(index)+"].value")
		}
		previous, offset = key, after
	}
	if offset != len(payload) {
		return canonicalInvalid(CodeShapeInvalid, "entries")
	}
	return nil
}

func framedChild(payload []byte, offset int) ([]byte, int, error) {
	if offset < 0 || len(payload)-offset < 8 {
		return nil, 0, canonicalInvalid(CodeShapeInvalid, "value")
	}
	length := binary.BigEndian.Uint64(payload[offset : offset+8])
	if length > uint64(len(payload)-offset-8) || length > MaxCanonicalValueBytes {
		return nil, 0, canonicalInvalid(CodeShapeInvalid, "value")
	}
	start, end := offset+8, offset+8+int(length)
	return payload[start:end], end, nil
}
