package runtimev2

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

const (
	ConformanceBundleSchemaVersion = "sqlrs.runtime.conformance.bundle-schema.v1"
	ConformanceBundleVersion       = "runtime-v2-canonical-v1.1"
	conformanceDigestDomain        = "sqlrs.runtime.conformance.bundle.v1"
)

//go:embed conformance/canonical-v1
var canonicalBundleFS embed.FS

// ConformanceDescriptor prevents a valid digest from relabeling bundle metadata.
type ConformanceDescriptor struct{ BundleSchemaVersion, BundleVersion, SemanticSchema string }

// CurrentConformanceDescriptor is compiled into this module release.
var CurrentConformanceDescriptor = ConformanceDescriptor{ConformanceBundleSchemaVersion, ConformanceBundleVersion, CanonicalSchemaVersion}

type conformanceEntry struct {
	Path   string `json:"path"`
	Size   uint64 `json:"size"`
	SHA256 string `json:"sha256"`
}
type conformanceManifest struct {
	BundleSchemaVersion string             `json:"bundle_schema_version"`
	BundleVersion       string             `json:"bundle_version"`
	SemanticSchema      string             `json:"semantic_schema"`
	Entries             []conformanceEntry `json:"entries"`
}

// ConformanceBundle is an immutable, fully verified byte bundle.
type ConformanceBundle struct {
	files  map[string][]byte
	digest string
}

func (b ConformanceBundle) Digest() string { return b.digest }
func (b ConformanceBundle) Files() map[string][]byte {
	result := make(map[string][]byte, len(b.files))
	for name, value := range b.files {
		result[name] = append([]byte(nil), value...)
	}
	return result
}

// LoadConformanceBundle verifies the module's embedded current bundle.
func LoadConformanceBundle() (ConformanceBundle, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(canonicalBundleFS, "conformance/canonical-v1", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := canonicalBundleFS.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(path, "conformance/canonical-v1/")
		files[name] = raw
		return nil
	})
	if err != nil {
		return ConformanceBundle{}, err
	}
	return ParseConformanceBundle(files, CurrentConformanceDescriptor)
}

var bundlePathPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(/[a-z0-9][a-z0-9._-]*)+$`)
var vectorIDPattern = regexp.MustCompile(`^[a-z][a-z0-9._/-]{0,127}$`)

// ParseConformanceBundle validates metadata, bytes, detached digest, and vectors.
func ParseConformanceBundle(files map[string][]byte, expected ConformanceDescriptor) (ConformanceBundle, error) {
	manifestRaw, ok := files["manifest.json"]
	if !ok {
		return ConformanceBundle{}, invalid(CodeFileMissing, "manifest")
	}
	detached, ok := files["manifest.sha256"]
	if !ok {
		return ConformanceBundle{}, invalid(CodeFileMissing, "manifest.sha256")
	}
	if !validTextFile(manifestRaw) {
		return ConformanceBundle{}, invalid(CodeVectorInvalid, "manifest")
	}
	var manifest conformanceManifest
	if err := decodeCanonicalStrict(manifestRaw, &manifest); err != nil {
		return ConformanceBundle{}, prefixError(err, "manifest")
	}
	if manifest.BundleSchemaVersion != expected.BundleSchemaVersion || manifest.BundleVersion != expected.BundleVersion || manifest.SemanticSchema != expected.SemanticSchema {
		return ConformanceBundle{}, invalid(CodeBundleMetadataMismatch, "manifest")
	}
	listed := map[string]struct{}{}
	previous := ""
	preimage := append(encodeString(conformanceDigestDomain), encodeString(manifest.BundleSchemaVersion)...)
	preimage = append(preimage, encodeUint32(uint32(len(manifest.Entries)))...)
	for index, entry := range manifest.Entries {
		path := "manifest.entries[" + itoa(index) + "]"
		if !bundlePathPattern.MatchString(entry.Path) || entry.Path == "manifest.json" || entry.Path == "manifest.sha256" || (index > 0 && previous >= entry.Path) {
			return ConformanceBundle{}, invalid(CodePathInvalid, path+".path")
		}
		content, exists := files[entry.Path]
		if !exists {
			return ConformanceBundle{}, invalid(CodeFileMissing, "files[\""+entry.Path+"\"]")
		}
		if uint64(len(content)) != entry.Size {
			return ConformanceBundle{}, invalid(CodeSizeMismatch, "files[\""+entry.Path+"\"]")
		}
		digest := sha256.Sum256(content)
		if entry.SHA256 != hex.EncodeToString(digest[:]) {
			return ConformanceBundle{}, invalid(CodeFileDigestMismatch, "files[\""+entry.Path+"\"]")
		}
		if !validTextFile(content) {
			return ConformanceBundle{}, invalid(CodeVectorInvalid, "files[\""+entry.Path+"\"]")
		}
		preimage = append(preimage, encodeString(entry.Path)...)
		preimage = append(preimage, encodeUint64(entry.Size)...)
		preimage = append(preimage, digest[:]...)
		listed[entry.Path] = struct{}{}
		previous = entry.Path
	}
	for name := range files {
		if name != "manifest.json" && name != "manifest.sha256" {
			if _, ok := listed[name]; !ok {
				return ConformanceBundle{}, invalid(CodeFileUnlisted, "files[\""+name+"\"]")
			}
		}
	}
	digest := sha256.Sum256(preimage)
	digestText := "sha256:" + hex.EncodeToString(digest[:]) + "\n"
	if string(detached) != digestText {
		return ConformanceBundle{}, invalid(CodeManifestDigestMismatch, "manifest.sha256")
	}
	if err := validateVectorFiles(manifest, files); err != nil {
		return ConformanceBundle{}, err
	}
	owned := make(map[string][]byte, len(files))
	for name, value := range files {
		owned[name] = append([]byte(nil), value...)
	}
	return ConformanceBundle{files: owned, digest: strings.TrimSpace(digestText)}, nil
}

func validTextFile(raw []byte) bool {
	return len(raw) > 0 && raw[len(raw)-1] == '\n' && (len(raw) == 1 || raw[len(raw)-2] != '\n') && !strings.Contains(string(raw), "\r")
}

type vectorFileWire struct {
	VectorSchema string               `json:"vector_schema"`
	Cases        []vectorCaseWire     `json:"cases"`
	Relations    []vectorRelationWire `json:"relations"`
}
type vectorCaseWire struct {
	ID        string          `json:"id"`
	Tags      []string        `json:"tags"`
	Operation string          `json:"operation"`
	Input     json.RawMessage `json:"input"`
	Expected  json.RawMessage `json:"expected"`
}

type vectorRelationWire struct {
	ID         string   `json:"id"`
	Tags       []string `json:"tags"`
	Comparison string   `json:"comparison"`
	Projection string   `json:"projection"`
	Cases      []string `json:"cases"`
}

type vectorExpectedWire struct {
	Status string           `json:"status"`
	Error  *vectorErrorWire `json:"error,omitempty"`
}

type vectorErrorWire struct {
	Code ValidationCode `json:"code"`
	Path *string        `json:"path"`
}

var vectorOperations = map[string]struct{}{
	"canonical-value": {}, "identity-field": {}, "factory": {}, "transform": {},
	"resolved-extension": {}, "root-state": {}, "derived-state": {},
	"compose-factory": {}, "compose-transform": {}, "recipe": {},
	"relative-lineage": {}, "decode-envelope": {}, "explain-safe": {},
	"explain-internal": {}, "legacy-decode": {}, "canonical-token": {},
	"canonical-value-envelope": {},
}

var vectorErrorCodes = map[ValidationCode]struct{}{
	CodeDocumentTooLarge: {}, CodeSyntaxInvalid: {}, CodeUnknownMember: {},
	CodeDuplicateMember: {}, CodeShapeInvalid: {}, CodeValueInvalid: {},
	CodeRevisionMismatch: {}, CodeLimitExceeded: {},
	CodeNonCanonical: {}, CodeDescriptorInvalid: {}, CodeCommitmentMismatch: {},
	CodeDigestMismatch: {}, CodeLineageMismatch: {}, CodeEndpointMismatch: {},
	CodeAuthorizationDenied: {}, CodeRelationMismatch: {},
}

func validVectorTags(tags []string) bool {
	return sort.StringsAreSorted(tags) && !hasAdjacentDuplicate(tags)
}

func hasAdjacentDuplicate(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] == values[index] {
			return true
		}
	}
	return false
}

func caseTagMatches(tag string, item vectorCaseWire) bool {
	var expected vectorExpectedWire
	_ = json.Unmarshal(item.Expected, &expected)
	switch tag {
	case "legacy-non-reinterpretation":
		return item.Operation == "legacy-decode"
	case "canonical-value", "fuzz-seed":
		return item.Operation == "canonical-value"
	case "factory", "root-state":
		return item.Operation == "factory" || item.Operation == "root-state"
	case "transform":
		return item.Operation == "transform"
	case "resolved-extension":
		return item.Operation == "resolved-extension"
	case "derived-state":
		return item.Operation == "derived-state"
	case "extension-composition":
		return item.Operation == "compose-factory" || item.Operation == "compose-transform"
	case "ordered-recipe":
		return item.Operation == "recipe"
	case "relative-lineage":
		return item.Operation == "relative-lineage"
	case "secret-reference":
		return item.Operation == "identity-field" || item.Operation == "explain-safe" || item.Operation == "explain-internal"
	case "safe-redaction":
		return item.Operation == "explain-safe"
	case "internal-disclosure":
		return item.Operation == "explain-internal"
	case "integrity-tampering":
		return item.Operation == "decode-envelope" && expected.Status == "error"
	case "limits":
		return (item.Operation == "canonical-value" || item.Operation == "canonical-value-envelope") && expected.Error != nil && expected.Error.Code == CodeLimitExceeded
	default:
		return false
	}
}

func relationTagMatches(tag string, relation vectorRelationWire) bool {
	switch tag {
	case "mutable-reference-same-resolution", "diagnostics-only", "supported-builder-operational-metadata":
		return relation.Projection == "digest" && relation.Comparison == "equal"
	case "identity-changing":
		return relation.Projection == "digest" && relation.Comparison == "not-equal"
	default:
		return false
	}
}

func validateVectorExpected(raw json.RawMessage, path string) error {
	var object map[string]json.RawMessage
	if err := decodeCanonicalStrict(raw, &object); err != nil {
		return prefixError(err, path+".expected")
	}
	var expected vectorExpectedWire
	_ = json.Unmarshal(raw, &expected)
	if expected.Status == "ok" {
		if expected.Error != nil || len(object) < 2 {
			return invalid(CodeVectorInvalid, path+".expected")
		}
		return nil
	}
	if expected.Status != "error" || expected.Error == nil || len(object) != 2 {
		return invalid(CodeVectorInvalid, path+".expected")
	}
	if err := decodeCanonicalStrict(object["error"], expected.Error); err != nil {
		return prefixError(err, path+".expected.error")
	}
	if _, ok := vectorErrorCodes[expected.Error.Code]; !ok || expected.Error.Path == nil {
		return invalid(CodeVectorInvalid, path+".expected.error")
	}
	return nil
}

func validateVectorFiles(manifest conformanceManifest, files map[string][]byte) error {
	required := map[string]bool{}
	for _, tag := range []string{"legacy-non-reinterpretation", "canonical-value", "factory", "transform", "resolved-extension", "root-state", "derived-state", "extension-composition", "ordered-recipe", "relative-lineage", "mutable-reference-same-resolution", "identity-changing", "diagnostics-only", "secret-reference", "safe-redaction", "internal-disclosure", "integrity-tampering", "supported-builder-operational-metadata", "limits", "fuzz-seed"} {
		required[tag] = false
	}
	ids := map[string]struct{}{}
	cases := map[string]vectorCaseWire{}
	relations := make([]struct {
		path  string
		value vectorRelationWire
	}, 0)
	for _, entry := range manifest.Entries {
		var vector vectorFileWire
		if err := decodeCanonicalStrict(files[entry.Path], &vector); err != nil {
			return prefixError(err, "vectors[\""+entry.Path+"\"]")
		}
		if vector.VectorSchema != "sqlrs.runtime.v2.canonical.conformance-vector.v1" {
			return invalid(CodeVectorInvalid, "vectors[\""+entry.Path+"\"].vector_schema")
		}
		previous := ""
		for index, item := range vector.Cases {
			path := "vectors[\"" + entry.Path + "\"].cases[" + itoa(index) + "]"
			if !vectorIDPattern.MatchString(item.ID) || (index > 0 && previous >= item.ID) {
				return invalid(CodeVectorInvalid, path+".id")
			}
			if _, exists := ids[item.ID]; exists {
				return invalid(CodeVectorInvalid, path+".id")
			}
			ids[item.ID] = struct{}{}
			if _, ok := vectorOperations[item.Operation]; !ok || len(item.Input) == 0 || isJSONNull(item.Input) || len(item.Expected) == 0 {
				return invalid(CodeVectorInvalid, path)
			}
			var input map[string]json.RawMessage
			if err := decodeCanonicalStrict(item.Input, &input); err != nil || len(input) == 0 {
				return invalid(CodeVectorInvalid, path+".input")
			}
			if err := validateVectorExpected(item.Expected, path); err != nil {
				return err
			}
			if err := verifyVectorCase(item, path); err != nil {
				return err
			}
			if !validVectorTags(item.Tags) {
				return invalid(CodeVectorInvalid, path+".tags")
			}
			for tagIndex, tag := range item.Tags {
				if _, ok := required[tag]; ok {
					if !caseTagMatches(tag, item) {
						return invalid(CodeVectorInvalid, path+".tags["+itoa(tagIndex)+"]")
					}
					required[tag] = true
				}
			}
			cases[item.ID] = item
			previous = item.ID
		}
		previous = ""
		for index, relation := range vector.Relations {
			path := "vectors[\"" + entry.Path + "\"].relations[" + itoa(index) + "]"
			if !vectorIDPattern.MatchString(relation.ID) || (index > 0 && previous >= relation.ID) {
				return invalid(CodeVectorInvalid, path+".id")
			}
			if _, exists := ids[relation.ID]; exists {
				return invalid(CodeVectorInvalid, path+".id")
			}
			ids[relation.ID] = struct{}{}
			if !validVectorTags(relation.Tags) || len(relation.Cases) != 2 || relation.Cases[0] == relation.Cases[1] {
				return invalid(CodeVectorInvalid, path)
			}
			if relation.Comparison != "equal" && relation.Comparison != "not-equal" {
				return invalid(CodeVectorInvalid, path+".comparison")
			}
			switch relation.Projection {
			case "canonical-bytes", "canonical-value-token", "field-commitment", "digest", "state-ids", "endpoint", "descriptor", "observation", "explanation":
			default:
				return invalid(CodeVectorInvalid, path+".projection")
			}
			for tagIndex, tag := range relation.Tags {
				if _, ok := required[tag]; ok {
					if !relationTagMatches(tag, relation) {
						return invalid(CodeVectorInvalid, path+".tags["+itoa(tagIndex)+"]")
					}
					required[tag] = true
				}
			}
			relations = append(relations, struct {
				path  string
				value vectorRelationWire
			}{path, relation})
			previous = relation.ID
		}
	}
	for _, relation := range relations {
		for index, caseID := range relation.value.Cases {
			if _, ok := cases[caseID]; !ok {
				return invalid(CodeRelationMismatch, relation.path+".cases["+itoa(index)+"]")
			}
		}
		left := relationProjection(cases[relation.value.Cases[0]], relation.value.Projection)
		right := relationProjection(cases[relation.value.Cases[1]], relation.value.Projection)
		if left == "" || right == "" || (left == right) != (relation.value.Comparison == "equal") {
			return invalid(CodeRelationMismatch, relation.path)
		}
	}
	for tag, present := range required {
		if !present {
			return invalid(CodeVectorInvalid, "coverage."+tag)
		}
	}
	return nil
}

func relationProjection(item vectorCaseWire, projection string) string {
	var expected map[string]json.RawMessage
	_ = json.Unmarshal(item.Expected, &expected)
	raw, ok := expected[projection]
	if !ok {
		return ""
	}
	return string(raw)
}
