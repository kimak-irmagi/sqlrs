package runtimev2

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
)

const (
	factoryDomain   = "sqlrs.runtime.v2/factory-state"
	transformDomain = "sqlrs.runtime.v2/transform"
	stateDomain     = "sqlrs.runtime.v2/state"
)

type canonicalField struct {
	tag     uint16
	payload []byte
}

func encodeString(value string) []byte { return encodeBytes([]byte(value)) }

func encodeBytes(value []byte) []byte {
	result := make([]byte, 8+len(value))
	binary.BigEndian.PutUint64(result[:8], uint64(len(value)))
	copy(result[8:], value)
	return result
}

func encodeRecord(domain string, fields []canonicalField) []byte {
	ordered := append([]canonicalField(nil), fields...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].tag < ordered[j].tag })
	result := encodeString(domain)
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], uint32(len(ordered)))
	result = append(result, count[:]...)
	for _, field := range ordered {
		var tag [2]byte
		binary.BigEndian.PutUint16(tag[:], field.tag)
		result = append(result, tag[:]...)
		result = append(result, encodeBytes(field.payload)...)
	}
	return result
}

func encodeFieldSet(fields []ResolvedField) []byte {
	result := make([]byte, 4)
	binary.BigEndian.PutUint32(result, uint32(len(fields)))
	for _, field := range fields {
		result = append(result, encodeString(field.Name)...)
		result = append(result, encodeString(field.Value)...)
	}
	return result
}

func canonicalIdentity(domain string, data *identityData) []byte {
	return encodeRecord(domain, []canonicalField{
		{tag: 1, payload: []byte(data.schemaVersion)},
		{tag: 2, payload: []byte(data.provider)},
		{tag: 3, payload: []byte(data.kind)},
		{tag: 4, payload: []byte(data.identitySchema)},
		{tag: 5, payload: encodeFieldSet(data.fields)},
	})
}

func canonicalFactory(identity ResolvedFactoryIdentity) []byte {
	return canonicalIdentity(factoryDomain, identity.data)
}

func canonicalTransform(identity ResolvedTransformIdentity) []byte {
	return canonicalIdentity(transformDomain, identity.data)
}

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func hashRecord(domain string, fields []canonicalField) string {
	return hashBytes(encodeRecord(domain, fields))
}

func parseDigest(value string, path string) ([]byte, error) {
	if len(value) != len("sha256:")+sha256.Size*2 || value[:7] != "sha256:" {
		return nil, invalid(CodeInvalidValue, path)
	}
	raw, err := hex.DecodeString(value[7:])
	if err != nil || hex.EncodeToString(raw) != value[7:] || len(raw) != sha256.Size {
		return nil, invalid(CodeInvalidValue, path)
	}
	return raw, nil
}
