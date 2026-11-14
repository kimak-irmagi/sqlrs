package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// literalStepResultFrame is a test oracle, not a call to a product encoder.
// IR01/02: docs/architecture/issue89-same-exec-result-tests.md.
func literalStepResultFrame(manifest, stdout, stderr, detail []byte, pruned bool) []byte {
	var wire bytes.Buffer
	wire.WriteString("sqlrs-lb-result-v1\n")
	for _, member := range []struct {
		name string
		data []byte
	}{{"manifest", manifest}, {"stdout", stdout}, {"stderr", stderr}, {"detail", detail}} {
		fmt.Fprintf(&wire, "%s %d\n", member.name, len(member.data))
		wire.Write(member.data)
	}
	footer := "none"
	if pruned {
		footer = "absent"
	}
	fmt.Fprintf(&wire, "sqlrs-lb-result-end-v1 idle %s\n", footer)
	return wire.Bytes()
}

func stepResultFrameFixture(t *testing.T, prior bool, exit int, stdout, stderr, detail []byte) ([]byte, StepCaptureIdentity, string) {
	t.Helper()
	_, identity, dir := combinedCaptureFixture(t, prior, "running", exit)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.complete"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["stdout_bytes"], manifest["stdout_sha256"] = len(stdout), captureHex(stdout)
	manifest["stderr_bytes"], manifest["stderr_sha256"] = len(stderr), captureHex(stderr)
	writeStepCaptureFixture(t, dir, manifest, stdout, stderr)
	data, err = os.ReadFile(filepath.Join(dir, "manifest.complete"))
	if err != nil {
		t.Fatal(err)
	}
	return literalStepResultFrame(data, stdout, stderr, detail, prior), identity, dir
}

func stepResultFiles(t *testing.T, dir string) (manifest, stdout, stderr []byte) {
	t.Helper()
	for name, target := range map[string]*[]byte{"manifest.complete": &manifest, "stdout.complete": &stdout, "stderr.complete": &stderr} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		*target = data
	}
	return
}

func TestStepResultEnvelopeExactBinary(t *testing.T) {
	for _, exit := range []int{0, 7, 255} {
		for _, prior := range []bool{false, true} {
			stdout := []byte{0, 255, 128, '\n'}
			stderr := []byte("sqlrs-lb-result-end-v1 idle absent\n")
			wire, identity, dir := stepResultFrameFixture(t, prior, exit, stdout, stderr, []byte("private-detail-marker"))
			result, err := decodeStepResultEnvelope(wire, 1024)
			if err != nil || result.pruned != prior || string(result.detail) != "private-detail-marker" {
				t.Fatal("literal frame rejected", err)
			}
			capture, err := readCompleteLiquibaseStepCapture(result.files, identity, 1024)
			original, readErr := ReadCompleteLiquibaseStepCapture(dir, identity, 1024)
			if err != nil || readErr != nil || !reflect.DeepEqual(capture, original) || capture.ExitCode != exit || !bytes.Equal(capture.Stdout, stdout) || !bytes.Equal(capture.Stderr, stderr) || capture.pruneObservation != nil {
				t.Fatal("binary/child result changed or decoder minted a receipt", err, readErr)
			}
		}
	}
	wire, identity, _ := stepResultFrameFixture(t, false, 0, nil, nil, nil)
	result, err := decodeStepResultEnvelope(wire, 1)
	if err != nil || len(result.detail) != 0 {
		t.Fatal("empty streams/detail rejected", err)
	}
	if capture, err := readCompleteLiquibaseStepCapture(result.files, identity, 1); err != nil || len(capture.Stdout) != 0 || len(capture.Stderr) != 0 {
		t.Fatal(err)
	}
}

func TestStepResultEnvelopeRejectsIncompleteAndMalformed(t *testing.T) {
	wire, _, _ := stepResultFrameFixture(t, false, 0, []byte("x"), []byte("y"), nil)
	for n := 0; n < len(wire); n++ {
		if _, err := decodeStepResultEnvelope(wire[:n], 1024); err == nil {
			t.Fatal("partial frame accepted", n)
		}
	}
	for _, bad := range [][]byte{
		append(append([]byte(nil), wire...), 0),
		bytes.Replace(wire, []byte("sqlrs-lb-result-v1"), []byte("sqlrs-lb-result-v2"), 1),
		bytes.Replace(wire, []byte("stdout 1\n"), []byte("stderr 1\n"), 1),
		bytes.Replace(wire, []byte("stdout 1\n"), []byte("stdout 01\n"), 1),
		bytes.Replace(wire, []byte("stdout 1\n"), []byte("stdout +1\n"), 1),
		bytes.Replace(wire, []byte("stdout 1\n"), []byte("stdout -1\n"), 1),
		bytes.Replace(wire, []byte("stdout 1\n"), []byte("stdout 1 \n"), 1),
		bytes.Replace(wire, []byte("stdout 1\n"), []byte("stdout 18446744073709551616\n"), 1),
		bytes.Replace(wire, []byte("stdout 1\n"), []byte("stdout "+strings.Repeat("9", 64)+"\n"), 1),
		bytes.Replace(wire, []byte("detail 0\n"), []byte("detail 1\n"), 1),
		bytes.Replace(wire, []byte("idle none\n"), []byte("busy none\n"), 1),
		bytes.Replace(wire, []byte("idle none\n"), []byte("idle unknown\n"), 1),
	} {
		if _, err := decodeStepResultEnvelope(bad, 1024); err == nil {
			t.Fatal("malformed frame accepted")
		}
	}
	for _, limit := range []int64{0, -1, 64<<20 + 1} {
		if _, err := decodeStepResultEnvelope(wire, limit); err == nil {
			t.Fatal("invalid stream limit accepted", limit)
		}
	}
	if _, err := decodeStepResultEnvelope(bytes.Repeat([]byte("x"), 2+8192+4096+257), 1); err == nil {
		t.Fatal("wire over quota accepted")
	}
}

func TestStepResultEnvelopeIndependentQuotasAndHashes(t *testing.T) {
	_, identity, dir := stepResultFrameFixture(t, false, 0, []byte("x"), []byte("y"), nil)
	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.complete"))
	if err != nil {
		t.Fatal(err)
	}
	manifest = append(manifest, bytes.Repeat([]byte(" "), 8192-len(manifest))...)
	valid := literalStepResultFrame(manifest, []byte("x"), []byte("y"), bytes.Repeat([]byte("d"), 4096), false)
	result, err := decodeStepResultEnvelope(valid, 1)
	if err != nil || len(result.detail) != 4096 {
		t.Fatal("exact quotas rejected", err)
	}
	if _, err := readCompleteLiquibaseStepCapture(result.files, identity, 1); err != nil {
		t.Fatal("exact manifest quota rejected", err)
	}
	for _, bad := range [][]byte{
		literalStepResultFrame(append(append([]byte(nil), manifest...), ' '), []byte("x"), []byte("y"), nil, false),
		literalStepResultFrame(manifest, []byte("xx"), []byte("y"), nil, false),
		literalStepResultFrame(manifest, []byte("x"), []byte("yy"), nil, false),
		literalStepResultFrame(manifest, []byte("x"), []byte("y"), bytes.Repeat([]byte("d"), 4097), false),
	} {
		if _, err := decodeStepResultEnvelope(bad, 1); err == nil {
			t.Fatal("member quota bypassed")
		}
	}
	for _, bad := range [][]byte{
		literalStepResultFrame(manifest, []byte("z"), []byte("y"), nil, false),
		literalStepResultFrame(manifest, []byte("x"), []byte("z"), nil, false),
		literalStepResultFrame(bytes.Replace(manifest, []byte(`"job_id":"job"`), []byte(`"job_id":"foreign"`), 1), []byte("x"), []byte("y"), nil, false),
	} {
		result, err := decodeStepResultEnvelope(bad, 1024)
		if err == nil {
			_, err = readCompleteLiquibaseStepCapture(result.files, identity, 1024)
		}
		if err == nil {
			t.Fatal("manifest identity/hash bypassed")
		}
	}
}
