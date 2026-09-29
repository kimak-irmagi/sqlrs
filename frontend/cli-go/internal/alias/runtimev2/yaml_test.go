package aliasruntimev2

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/composition"
	"gopkg.in/yaml.v3"
)

func TestDecodeAliasDocumentYAMLEquivalence(t *testing.T) {
	raw := []byte(`schema_version: sqlrs.runtime.v2.aliases.v1
aliases:
  database:
    type: recipe
    base:
      factory:
        kind: postgres
        reference: postgres:17
        arguments: []
        attributes: {}
    steps:
      - transform:
          kind: psql
          reference: db/schema.sql
          arguments: ["-f", "db/schema.sql"]
          attributes: {}
`)
	var got composition.AliasDocument
	if err := DecodeAliasDocumentYAML(raw, &got); err != nil {
		t.Fatalf("DecodeAliasDocumentYAML: %v", err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var want composition.AliasDocument
	if err := composition.DecodeAliasDocumentJSON(gotJSON, &want); err != nil {
		t.Fatalf("strict JSON rejected YAML result: %v", err)
	}
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("YAML and JSON documents differ:\n%s\n%s", gotJSON, wantJSON)
	}
}

func TestDecodeAliasDocumentYAMLRejectsUnsafeOrAmbiguousSyntaxAtomically(t *testing.T) {
	seedRaw := []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\n")
	var target composition.AliasDocument
	if err := DecodeAliasDocumentYAML(seedRaw, &target); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(target)

	cases := map[string][]byte{
		"malformed":      []byte("["),
		"empty":          []byte(""),
		"duplicate key":  []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\naliases: {}\n"),
		"anchor":         []byte("schema_version: &version sqlrs.runtime.v2.aliases.v1\naliases: {}\n"),
		"alias":          []byte("schema_version: &version sqlrs.runtime.v2.aliases.v1\naliases: *version\n"),
		"merge":          []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases:\n  base: &base {type: transform}\n  next: {<<: *base}\n"),
		"custom tag":     []byte("schema_version: !version sqlrs.runtime.v2.aliases.v1\naliases: {}\n"),
		"directive":      []byte("%YAML 1.2\n---\nschema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\n"),
		"multiple docs":  []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\n---\nschema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\n"),
		"bad second doc": []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\n---\n[\n"),
		"unknown field":  []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\nextra: value\n"),
		"boolean":        []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: true\n"),
		"integer":        []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: 1\n"),
		"float":          []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: 1.5\n"),
		"timestamp":      []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: 2026-09-29\n"),
		"binary":         []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: !!binary SGVsbG8=\n"),
		"null":           []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: null\n"),
		"invalid UTF-8":  append([]byte("schema_version: "), 0xff),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if err := DecodeAliasDocumentYAML(raw, &target); err == nil {
				t.Fatal("unsafe YAML unexpectedly succeeded")
			}
			after, _ := json.Marshal(target)
			if !bytes.Equal(before, after) {
				t.Fatalf("failure mutated target:\n%s\n%s", before, after)
			}
		})
	}
	if err := DecodeAliasDocumentYAML(seedRaw, nil); err == nil {
		t.Fatal("nil target unexpectedly succeeded")
	}
}

func TestDecodeAliasDocumentYAMLResourceLimits(t *testing.T) {
	base := []byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\n")
	maximum := append(append([]byte(nil), base...), '#')
	maximum = append(maximum, bytes.Repeat([]byte{'x'}, MaxYAMLBytes-len(maximum))...)
	if len(maximum) != MaxYAMLBytes {
		t.Fatalf("maximum fixture size = %d", len(maximum))
	}
	if err := DecodeAliasDocumentYAML(maximum, new(composition.AliasDocument)); err != nil {
		t.Fatalf("maximum YAML byte size rejected: %v", err)
	}
	if err := DecodeAliasDocumentYAML(bytes.Repeat([]byte{'x'}, MaxYAMLBytes+1), new(composition.AliasDocument)); err == nil {
		t.Fatal("oversized YAML unexpectedly succeeded")
	}

	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	current := root
	for depth := 1; depth < MaxYAMLDepth; depth++ {
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "k"}
		next := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		current.Content = []*yaml.Node{key, next}
		current = next
	}
	if err := validateYAMLTree(root); err != nil {
		t.Fatalf("maximum YAML depth rejected: %v", err)
	}
	current.Content = []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "k"},
		{Kind: yaml.MappingNode, Tag: "!!map"},
	}
	if err := validateYAMLTree(root); err == nil {
		t.Fatal("YAML depth overflow unexpectedly succeeded")
	}

	nodes := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: make([]*yaml.Node, MaxYAMLNodes-1)}
	for index := range nodes.Content {
		nodes.Content[index] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "x"}
	}
	if err := validateYAMLTree(nodes); err != nil {
		t.Fatalf("maximum YAML node count rejected: %v", err)
	}
	nodes.Content = append(nodes.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "x"})
	if err := validateYAMLTree(nodes); err == nil {
		t.Fatal("YAML node overflow unexpectedly succeeded")
	}
}

func TestValidateYAMLTreeRejectsEveryClosedUnionViolation(t *testing.T) {
	stringNode := func(value string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	}
	cases := map[string]*yaml.Node{
		"nil":            nil,
		"alias pointer":  {Kind: yaml.ScalarNode, Tag: "!!str", Alias: stringNode("x")},
		"custom mapping": {Kind: yaml.MappingNode, Tag: "!map"},
		"odd mapping":    {Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{stringNode("key")}},
		"non-string key": {Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"}, stringNode("value"),
		}},
		"merge key": {Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			stringNode("<<"), stringNode("value"),
		}},
		"custom sequence": {Kind: yaml.SequenceNode, Tag: "!sequence"},
		"document node":   {Kind: yaml.DocumentNode, Tag: ""},
	}
	for name, root := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateYAMLTree(root); err == nil {
				t.Fatal("invalid YAML node tree unexpectedly succeeded")
			}
		})
	}
}

func TestDecodeAliasDocumentYAMLAcceptsQuotedStringScalars(t *testing.T) {
	raw := []byte("schema_version: \"sqlrs.runtime.v2.aliases.v1\"\naliases: {}\n")
	var target composition.AliasDocument
	if err := DecodeAliasDocumentYAML(raw, &target); err != nil {
		t.Fatalf("quoted strings rejected: %v", err)
	}
	if encoded, _ := json.Marshal(target); !strings.Contains(string(encoded), `"aliases":{}`) {
		t.Fatalf("unexpected document: %s", encoded)
	}
}

func TestDecodeAliasDocumentYAMLParserErrorsDoNotDiscloseInput(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("[super-secret-value"),
		[]byte("schema_version: sqlrs.runtime.v2.aliases.v1\naliases: {}\n---\n[super-secret-value"),
	} {
		err := DecodeAliasDocumentYAML(raw, new(composition.AliasDocument))
		if err == nil {
			t.Fatal("malformed YAML unexpectedly succeeded")
		}
		if strings.Contains(err.Error(), "super-secret-value") || len(err.Error()) > 256 {
			t.Fatalf("parser diagnostic is unsafe: %q", err)
		}
	}
}
