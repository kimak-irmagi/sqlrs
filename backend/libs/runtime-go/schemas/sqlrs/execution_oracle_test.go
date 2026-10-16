package sqlrs_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

// ES03: this test-only oracle implements the published canonical-v1 grammar,
// not the facade's mapping. Expected trees are fixed independently below.
// It must never be imported into production or used to generate actual values.
func TestExecutionIndependentCanonicalGrammar(t *testing.T) {
	host := executionBase()
	host.Runtime.Kind, host.Runtime.IdentityKind = sqlrs.ExecutionRuntimeHost, sqlrs.ExecutionIdentityHostExecutable
	host.User, host.Workdir.Root, host.Network = "uid:1000", "workspace", sqlrs.ExecutionNetworkLocalContext
	host.Argv[0] = executionPathWord("tool", "")
	hostFields := executionMinimalFields()
	hostFields["primitive"] = "host"
	hostFields["runtime"].(map[string]any)["identity_kind"] = "host-executable"
	hostFields["invocation"] = map[string]any{"argv": []any{[]any{map[string]any{"kind": "path", "path": map[string]any{"root": "tool", "relative": ""}, "format": "native"}}}, "environment": map[string]any{}, "workdir": map[string]any{"root": "workspace", "relative": "work"}, "user": "uid:1000"}
	hostFields["network"].(map[string]any)["mode"] = "local-context"
	for _, tc := range []struct {
		name   string
		input  sqlrs.ExecutionEnvironmentInput
		fields map[string]any
	}{
		{"oci", executionBase(), executionMinimalFields()},
		{"partial-host", host, hostFields},
		{"sources-services-secrets", executionResources(t), executionResourceFields()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := executionBuild(t, tc.input)
			want := executionOracleIdentity(tc.fields)
			got, err := runtimev2.CanonicalResolvedExtensionBytes(actual)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(want)
			if !bytes.Equal(got, want) || string(executionFingerprint(t, actual)) != "sha256:"+hex.EncodeToString(digest[:]) {
				t.Fatal("independent canonical preimage/fingerprint mismatch")
			}
		})
	}
}

func executionResourceFields() map[string]any {
	v := executionMinimalFields()
	v["inputs"] = map[string]any{"project": map[string]any{"source": map[string]any{"provider": "files", "kind": "tree", "identity_schema": "files.tree.v1", "fingerprint": executionDigest}, "profile": "regular-tree.v1"}}
	path := func(root, relative string) map[string]any { return map[string]any{"root": root, "relative": relative} }
	v["invocation"] = map[string]any{
		"argv":        []any{[]any{map[string]any{"kind": "literal", "literal": "tool"}}, []any{map[string]any{"kind": "literal", "literal": "--file="}, map[string]any{"kind": "path", "path": path("mount:project", "change.sql"), "format": "native"}}, []any{map[string]any{"kind": "path", "path": path("mount:config", ""), "format": "native"}}},
		"environment": map[string]any{"DATABASE": []any{map[string]any{"kind": "service", "role": "database", "field": "database"}}, "SEED": []any{map[string]any{"kind": "secret", "slot": "seed"}}},
		"workdir":     path("mount:project", ""), "user": "",
	}
	v["mounts"] = []any{map[string]any{"name": "project", "target": path("image", "work"), "kind": "input", "access": "read-only", "input_name": "project"}, map[string]any{"name": "scratch", "target": path("image", "work/tmp"), "kind": "scratch", "access": "read-write"}, map[string]any{"name": "config", "target": path("image", "secrets/config"), "kind": "secret", "access": "read-only", "secret_slot": "seed"}}
	v["network"] = map[string]any{"mode": "none", "services": map[string]any{"database": map[string]any{"protocol": "postgresql", "database": "app", "user": "app", "transport": "tcp", "authentication_secret": "auth", "authentication_profile": "postgresql.env.v1"}}}
	v["secrets"] = map[string]any{"auth": map[string]any{"purpose": "authentication", "service_role": "database"}, "seed": map[string]any{"purpose": "semantic", "reference": map[string]any{"provider": "vault", "identifier": "seed-reference", "version": "v1"}}}
	return v
}

func executionOracleIdentity(fields map[string]any) []byte {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	fieldSet := oracleU32(uint32(len(names)))
	for _, name := range names {
		kind, payload := uint16(2), []byte(nil)
		if name == "primitive" {
			kind, payload = 1, []byte(fields[name].(string))
		} else {
			payload = oracleSum(bytes.Join([][]byte{oracleFrame([]byte("sqlrs.runtime.v2/identity-value")), oracleFrame(executionOracleNode(fields[name]))}, nil))
		}
		commitment := oracleSum(bytes.Join([][]byte{oracleFrame([]byte(name)), oracleU16(kind), oracleFrame(payload)}, nil))
		fieldSet = append(fieldSet, bytes.Join([][]byte{oracleFrame([]byte(name)), oracleU16(kind), commitment}, nil)...)
	}
	result := append(oracleFrame([]byte("sqlrs.runtime.v2.canonical.v1/resolved-extension")), oracleU32(5)...)
	for index, value := range [][]byte{[]byte("sqlrs.runtime.v2.canonical.v1"), []byte("sqlrs.execution"), []byte("environment"), []byte("sqlrs.execution-environment.v1"), fieldSet} {
		result = append(result, oracleU16(uint16(index+1))...)
		result = append(result, oracleFrame(value)...)
	}
	return result
}

func executionOracleNode(value any) []byte {
	var tag uint16
	var payload []byte
	switch value := value.(type) {
	case string:
		tag, payload = 1, []byte(value)
	case []any:
		tag, payload = 2, oracleU32(uint32(len(value)))
		for _, child := range value {
			payload = append(payload, oracleFrame(executionOracleNode(child))...)
		}
	case map[string]any:
		tag, payload = 3, oracleU32(uint32(len(value)))
		type entry struct{ key, value []byte }
		entries := make([]entry, 0, len(value))
		for key, child := range value {
			entries = append(entries, entry{executionOracleNode(key), executionOracleNode(child)})
		}
		sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i].key, entries[j].key) < 0 })
		for _, entry := range entries {
			payload = append(payload, oracleFrame(entry.key)...)
			payload = append(payload, oracleFrame(entry.value)...)
		}
	default:
		panic("unsupported oracle fixture")
	}
	return append(oracleU16(tag), oracleFrame(payload)...)
}
func oracleFrame(value []byte) []byte {
	result := make([]byte, 8, len(value)+8)
	binary.BigEndian.PutUint64(result, uint64(len(value)))
	return append(result, value...)
}
func oracleU16(value uint16) []byte { return binary.BigEndian.AppendUint16(nil, value) }
func oracleU32(value uint32) []byte { return binary.BigEndian.AppendUint32(nil, value) }
func oracleSum(value []byte) []byte { result := sha256.Sum256(value); return result[:] }
