package runtimev2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestEmbeddedConformanceBundleVerifiesAndOwnsBytes(t *testing.T) {
	bundle, err := LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Digest() != "sha256:4009154c578097c86d42aa6f457208a2c84f7d70a86c696ad4ae37b55a78edb4" {
		t.Fatalf("unexpected digest %s", bundle.Digest())
	}
	files := bundle.Files()
	files["manifest.json"][0] = 'x'
	if bundle.Files()["manifest.json"][0] == 'x' {
		t.Fatal("bundle exposed mutable bytes")
	}
}

func TestConformanceBundlePathAndFileFailureMatrix(t *testing.T) {
	bundle, err := LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	assertCode := func(files map[string][]byte, code ValidationCode) {
		t.Helper()
		_, err := ParseConformanceBundle(files, CurrentConformanceDescriptor)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Code != code {
			t.Fatalf("got %v, want %s", err, code)
		}
	}
	files := bundle.Files()
	delete(files, "manifest.json")
	assertCode(files, CodeFileMissing)
	files = bundle.Files()
	delete(files, "manifest.sha256")
	assertCode(files, CodeFileMissing)
	files = bundle.Files()
	files["manifest.json"] = []byte("{}\r\n")
	assertCode(files, CodeVectorInvalid)
	files = bundle.Files()
	files["extra.json"] = []byte("{}\n")
	assertCode(files, CodeFileUnlisted)
	files = bundle.Files()
	files["manifest.sha256"] = []byte("sha256:" + strings.Repeat("0", 64) + "\n")
	assertCode(files, CodeManifestDigestMismatch)
	files = bundle.Files()
	var manifest conformanceManifest
	_ = json.Unmarshal(files["manifest.json"], &manifest)
	for _, candidate := range []string{"", ".", "../escape", "vectors//x.json", "/absolute.json", "C:/drive.json", `vectors\backslash.json`, "vectors/control\x00.json", "Vectors/upper.json", "vectors/é.json", "single", "manifest.json", "manifest.sha256"} {
		files = bundle.Files()
		_ = json.Unmarshal(files["manifest.json"], &manifest)
		manifest.Entries[0].Path = candidate
		files["manifest.json"] = mustJSONLine(t, manifest)
		assertCode(files, CodePathInvalid)
	}
	files = bundle.Files()
	_ = json.Unmarshal(files["manifest.json"], &manifest)
	delete(files, manifest.Entries[0].Path)
	assertCode(files, CodeFileMissing)
	files = bundle.Files()
	_ = json.Unmarshal(files["manifest.json"], &manifest)
	manifest.Entries[0].Size++
	files["manifest.json"] = mustJSONLine(t, manifest)
	assertCode(files, CodeSizeMismatch)
	files = bundle.Files()
	files["vectors/canonical-values.json"][0] ^= 1
	assertCode(files, CodeFileDigestMismatch)
}

func TestConformanceVectorSemanticFailureMatrix(t *testing.T) {
	bundle, err := LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(map[string]any){
		func(v map[string]any) { v["vector_schema"] = "wrong" },
		func(v map[string]any) { v["cases"].([]any)[0].(map[string]any)["id"] = "Bad ID" },
		func(v map[string]any) { v["cases"].([]any)[0].(map[string]any)["operation"] = "" },
		func(v map[string]any) {
			v["cases"].([]any)[0].(map[string]any)["tags"] = []any{"limits", "canonical-value"}
		},
		func(v map[string]any) {
			v["cases"].([]any)[0].(map[string]any)["tags"] = []any{"canonical-value", "canonical-value", "fuzz-seed", "limits"}
		},
		func(v map[string]any) {
			for _, item := range v["cases"].([]any) {
				candidate := item.(map[string]any)
				candidate["tags"] = []any{"canonical-value"}
			}
		},
	}
	for index, mutate := range mutations {
		files := bundle.Files()
		var vector map[string]any
		_ = json.Unmarshal(files["vectors/canonical-values.json"], &vector)
		mutate(vector)
		files["vectors/canonical-values.json"] = mustJSONLine(t, vector)
		sealBundle(t, files)
		_, err := ParseConformanceBundle(files, CurrentConformanceDescriptor)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Code != CodeVectorInvalid {
			t.Fatalf("mutation %d: %v", index, err)
		}
	}
}

func TestConformanceRequiredTagMustMatchOperation(t *testing.T) {
	bundle, err := LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	files := bundle.Files()
	var canonical, secrets map[string]any
	if err := json.Unmarshal(files["vectors/canonical-values.json"], &canonical); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(files["vectors/secrets-and-disclosure.json"], &secrets); err != nil {
		t.Fatal(err)
	}
	for _, raw := range secrets["cases"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == "secrets/safe" {
			item["tags"] = []any{}
		}
	}
	for _, raw := range canonical["cases"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == "canonical-value/null" {
			item["tags"] = []any{"canonical-value", "fuzz-seed", "safe-redaction"}
		}
	}
	files["vectors/canonical-values.json"] = mustJSONLine(t, canonical)
	files["vectors/secrets-and-disclosure.json"] = mustJSONLine(t, secrets)
	sealBundle(t, files)
	_, err = ParseConformanceBundle(files, CurrentConformanceDescriptor)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != CodeVectorInvalid {
		t.Fatalf("mis-tagged bundle error = %v", err)
	}
}

func TestConformanceVectorKnownAnswersAndRelationsAreRecomputed(t *testing.T) {
	bundle, err := LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		file string
		code ValidationCode
		edit func(map[string]any)
	}{
		{"vectors/canonical-values.json", CodeVectorInvalid, func(v map[string]any) {
			v["cases"].([]any)[0].(map[string]any)["expected"].(map[string]any)["canonical-value-token"] = "civ1:sha256:" + strings.Repeat("0", 64)
		}},
		{"vectors/fingerprints.json", CodeVectorInvalid, func(v map[string]any) {
			v["cases"].([]any)[0].(map[string]any)["expected"].(map[string]any)["digest"] = "sha256:" + strings.Repeat("0", 64)
		}},
		{"vectors/lineage.json", CodeVectorInvalid, func(v map[string]any) {
			v["cases"].([]any)[0].(map[string]any)["expected"].(map[string]any)["endpoint"] = "sha256:" + strings.Repeat("0", 64)
		}},
		{"vectors/canonical-values.json", CodeRelationMismatch, func(v map[string]any) {
			v["relations"].([]any)[0].(map[string]any)["cases"].([]any)[0] = "missing/case"
		}},
		{"vectors/canonical-values.json", CodeVectorInvalid, func(v map[string]any) {
			candidate := v["cases"].([]any)[0].(map[string]any)
			candidate["input"] = map[string]any{}
		}},
		{"vectors/canonical-values.json", CodeVectorInvalid, func(v map[string]any) {
			candidate := v["cases"].([]any)[0].(map[string]any)
			candidate["expected"] = map[string]any{"status": "ok"}
		}},
	}
	for _, test := range tests {
		files := bundle.Files()
		var vector map[string]any
		if err := json.Unmarshal(files[test.file], &vector); err != nil {
			t.Fatal(err)
		}
		test.edit(vector)
		files[test.file] = mustJSONLine(t, vector)
		sealBundle(t, files)
		_, err := ParseConformanceBundle(files, CurrentConformanceDescriptor)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Code != test.code {
			t.Fatalf("%s: got %v, want %s", test.file, err, test.code)
		}
	}
}

func mustJSONLine(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func sealBundle(t *testing.T, files map[string][]byte) {
	t.Helper()
	var manifest conformanceManifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	preimage := append(encodeString(conformanceDigestDomain), encodeString(manifest.BundleSchemaVersion)...)
	preimage = append(preimage, encodeUint32(uint32(len(manifest.Entries)))...)
	for index := range manifest.Entries {
		entry := &manifest.Entries[index]
		content := files[entry.Path]
		digest := sha256.Sum256(content)
		entry.Size = uint64(len(content))
		entry.SHA256 = hex.EncodeToString(digest[:])
		preimage = append(preimage, encodeString(entry.Path)...)
		preimage = append(preimage, encodeUint64(entry.Size)...)
		preimage = append(preimage, digest[:]...)
	}
	files["manifest.json"] = mustJSONLine(t, manifest)
	digest := sha256.Sum256(preimage)
	files["manifest.sha256"] = []byte("sha256:" + hex.EncodeToString(digest[:]) + "\n")
}

func TestConformanceBundleRejectsEntryTamperingAndMetadataRelabeling(t *testing.T) {
	bundle, err := LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	files := bundle.Files()
	files["vectors/limits.json"][0] ^= 1
	_, err = ParseConformanceBundle(files, CurrentConformanceDescriptor)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != CodeFileDigestMismatch {
		t.Fatalf("unexpected tamper error: %v", err)
	}
	files = bundle.Files()
	_, err = ParseConformanceBundle(files, ConformanceDescriptor{ConformanceBundleSchemaVersion, "other", CanonicalSchemaVersion})
	if !errors.As(err, &validation) || validation.Code != CodeBundleMetadataMismatch {
		t.Fatalf("unexpected metadata error: %v", err)
	}
}

func TestConformanceSchemaAndRelationFailureMatrix(t *testing.T) {
	bundle, err := LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	assertCode := func(files map[string][]byte, code ValidationCode) {
		t.Helper()
		_, err := ParseConformanceBundle(files, CurrentConformanceDescriptor)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Code != code {
			t.Fatalf("got %v, want %s", err, code)
		}
	}

	files := bundle.Files()
	files["manifest.json"] = []byte("{\n")
	assertCode(files, CodeSyntaxInvalid)
	files = bundle.Files()
	files["vectors/canonical-values.json"] = []byte("{]\n")
	sealBundle(t, files)
	assertCode(files, CodeSyntaxInvalid)
	files = bundle.Files()
	files["vectors/canonical-values.json"] = append(files["vectors/canonical-values.json"], '\r', '\n')
	sealBundle(t, files)
	assertCode(files, CodeVectorInvalid)

	mutate := func(file string, code ValidationCode, edit func(map[string]any)) {
		t.Helper()
		files := bundle.Files()
		var vector map[string]any
		if err := json.Unmarshal(files[file], &vector); err != nil {
			t.Fatal(err)
		}
		edit(vector)
		files[file] = mustJSONLine(t, vector)
		sealBundle(t, files)
		assertCode(files, code)
	}
	mutate("vectors/compatibility.json", CodeVectorInvalid, func(vector map[string]any) {
		vector["cases"].([]any)[0].(map[string]any)["id"] = "canonical-value/list-ab"
	})
	relation := func(vector map[string]any) map[string]any {
		return vector["relations"].([]any)[0].(map[string]any)
	}
	mutate("vectors/canonical-values.json", CodeVectorInvalid, func(vector map[string]any) {
		relation(vector)["id"] = "Bad ID"
	})
	mutate("vectors/canonical-values.json", CodeVectorInvalid, func(vector map[string]any) {
		relation(vector)["id"] = "canonical-value/list-ab"
	})
	mutate("vectors/canonical-values.json", CodeVectorInvalid, func(vector map[string]any) {
		relation(vector)["tags"] = []any{"z", "a"}
	})
	mutate("vectors/canonical-values.json", CodeVectorInvalid, func(vector map[string]any) {
		cases := relation(vector)["cases"].([]any)
		cases[1] = cases[0]
	})
	mutate("vectors/canonical-values.json", CodeVectorInvalid, func(vector map[string]any) {
		relation(vector)["comparison"] = "similar"
	})
	mutate("vectors/canonical-values.json", CodeVectorInvalid, func(vector map[string]any) {
		relation(vector)["projection"] = "unknown"
	})
	mutate("vectors/canonical-values.json", CodeRelationMismatch, func(vector map[string]any) {
		relation(vector)["projection"] = "observation"
	})
	mutate("vectors/canonical-values.json", CodeRelationMismatch, func(vector map[string]any) {
		relation(vector)["comparison"] = "equal"
	})
}
