// Package aliasruntimev2 implements the CLI-owned Runtime v2 authoring and
// legacy-compatibility boundary specified by
// docs/architecture/runtime-v2-composition-structure.md.
package aliasruntimev2

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/composition"
	"gopkg.in/yaml.v3"
)

const (
	// MaxYAMLBytes bounds the untrusted YAML transport before parsing.
	MaxYAMLBytes = runtimev2.MaxJSONBytes
	// MaxYAMLDepth counts the logical root mapping as depth one.
	MaxYAMLDepth = 32
	// MaxYAMLNodes counts mappings, sequences, scalar values, and mapping keys.
	MaxYAMLNodes = 65_536
)

// DecodeAliasDocumentYAML atomically decodes the string-only YAML subset used
// by sqlrs.runtime.v2.aliases.v1. The destination is unchanged on failure.
func DecodeAliasDocumentYAML(raw []byte, target *composition.AliasDocument) error {
	if target == nil {
		return errors.New("runtime v2 alias YAML target is required")
	}
	if len(raw) > MaxYAMLBytes {
		return errors.New("runtime v2 alias YAML exceeds the byte limit")
	}
	if !utf8.Valid(raw) {
		return errors.New("runtime v2 alias YAML must be valid UTF-8")
	}
	if hasYAMLDirective(raw) {
		return errors.New("runtime v2 alias YAML directives are not supported")
	}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return errors.New("runtime v2 alias YAML is invalid")
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("runtime v2 alias YAML must contain one document")
		}
		return errors.New("runtime v2 alias YAML has invalid trailing content")
	}

	root := document.Content[0]
	if err := validateYAMLTree(root); err != nil {
		return err
	}
	// validateYAMLTree closes the node union to JSON-compatible strings,
	// sequences, and mappings, so this encoding cannot fail.
	encoded, _ := json.Marshal(yamlNodeValue(root))
	var decoded composition.AliasDocument
	if err := composition.DecodeAliasDocumentJSON(encoded, &decoded); err != nil {
		return err
	}
	*target = decoded
	return nil
}

func hasYAMLDirective(raw []byte) bool {
	for index, line := range bytes.Split(raw, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if index == 0 {
			line = bytes.TrimPrefix(line, []byte{0xef, 0xbb, 0xbf})
		}
		if len(line) > 0 && line[0] == '%' {
			return true
		}
	}
	return false
}

type yamlVisit struct {
	node  *yaml.Node
	depth int
}

func validateYAMLTree(root *yaml.Node) error {
	if root == nil {
		return errors.New("runtime v2 alias YAML root is required")
	}
	stack := []yamlVisit{{node: root, depth: 1}}
	count := 0
	for len(stack) > 0 {
		last := len(stack) - 1
		visit := stack[last]
		stack = stack[:last]
		count++
		if count > MaxYAMLNodes {
			return errors.New("runtime v2 alias YAML exceeds the node limit")
		}
		if visit.depth > MaxYAMLDepth {
			return errors.New("runtime v2 alias YAML exceeds the depth limit")
		}
		node := visit.node
		if node == nil || node.Anchor != "" || node.Alias != nil || node.Kind == yaml.AliasNode {
			return errors.New("runtime v2 alias YAML graph features are not supported")
		}
		switch node.Kind {
		case yaml.MappingNode:
			if node.Tag != "!!map" || len(node.Content)%2 != 0 {
				return errors.New("runtime v2 alias YAML contains an invalid mapping")
			}
			seen := make(map[string]struct{}, len(node.Content)/2)
			for index := 0; index < len(node.Content); index += 2 {
				key := node.Content[index]
				if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Anchor != "" {
					return errors.New("runtime v2 alias YAML mapping keys must be strings")
				}
				if key.Value == "<<" {
					return errors.New("runtime v2 alias YAML merge keys are not supported")
				}
				if _, exists := seen[key.Value]; exists {
					return errors.New("runtime v2 alias YAML contains a duplicate key")
				}
				seen[key.Value] = struct{}{}
			}
		case yaml.SequenceNode:
			if node.Tag != "!!seq" {
				return errors.New("runtime v2 alias YAML contains a custom sequence tag")
			}
		case yaml.ScalarNode:
			if node.Tag != "!!str" {
				return errors.New("runtime v2 alias YAML scalar coercion is not supported")
			}
		default:
			return errors.New("runtime v2 alias YAML contains an unsupported node")
		}
		for index := len(node.Content) - 1; index >= 0; index-- {
			stack = append(stack, yamlVisit{node: node.Content[index], depth: visit.depth + 1})
		}
	}
	return nil
}

func yamlNodeValue(node *yaml.Node) any {
	if node.Kind == yaml.MappingNode {
		value := make(map[string]any, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			value[node.Content[index].Value] = yamlNodeValue(node.Content[index+1])
		}
		return value
	}
	if node.Kind == yaml.SequenceNode {
		value := make([]any, len(node.Content))
		for index, childNode := range node.Content {
			value[index] = yamlNodeValue(childNode)
		}
		return value
	}
	// validateYAMLTree guarantees that the remaining closed-union variant is a
	// string scalar.
	return node.Value
}
