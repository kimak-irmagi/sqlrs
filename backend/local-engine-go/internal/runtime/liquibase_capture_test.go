package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// C02/C03: zero bytes need a completion marker; each stream and every
// immutable identity field are verified independently on recovery.
func TestReadCompleteLiquibaseCapture(t *testing.T) {
	helper, digest := LiquibaseCaptureHelper()
	if len(helper) == 0 || digest != captureDigest(helper) {
		t.Fatal("embedded helper digest mismatch")
	}
	dir := t.TempDir()
	identity := CaptureIdentity{JobID: "job-1", AttemptNo: 1, ExecutionDigest: "sha256:" + strings.Repeat("a", 64), HelperDigest: digest, ExitCode: 0}
	manifest := liquibaseCaptureManifest{Protocol: "lb-capture-v2", JobID: identity.JobID, TaskID: "plan", AttemptNo: 1, ExecutionDigest: identity.ExecutionDigest, HelperDigest: digest, ChildExitCode: 0, StdoutEOF: true, StderrEOF: true, StdoutBytes: 0, StdoutSHA256: captureHex(nil), StderrBytes: 0, StderrSHA256: captureHex(nil)}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest := func() {
		t.Helper()
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		write("manifest.complete", data)
	}
	write("stdout.complete", nil)
	write("stderr.complete", nil)
	if _, err := ReadCompleteLiquibaseCapture(dir, identity, 1024); err == nil {
		t.Fatal("accepted missing manifest as empty output")
	}
	writeManifest()
	if result, err := ReadCompleteLiquibaseCapture(dir, identity, 1024); err != nil || len(result.Stdout) != 0 || result.StdoutDigest != captureDigest(nil) {
		t.Fatalf("empty capture=%+v err=%v", result, err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*liquibaseCaptureManifest)
	}{
		{"job", func(m *liquibaseCaptureManifest) { m.JobID = "other" }},
		{"attempt", func(m *liquibaseCaptureManifest) { m.AttemptNo = 2 }},
		{"execution", func(m *liquibaseCaptureManifest) { m.ExecutionDigest = "sha256:" + strings.Repeat("b", 64) }},
		{"helper", func(m *liquibaseCaptureManifest) { m.HelperDigest = "sha256:" + strings.Repeat("b", 64) }},
		{"exit", func(m *liquibaseCaptureManifest) { m.ChildExitCode = 1 }},
		{"length", func(m *liquibaseCaptureManifest) { m.StdoutBytes = 1 }},
		{"hash", func(m *liquibaseCaptureManifest) { m.StdoutSHA256 = strings.Repeat("b", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := manifest
			tc.mutate(&changed)
			data, _ := json.Marshal(changed)
			write("manifest.complete", data)
			if _, err := ReadCompleteLiquibaseCapture(dir, identity, 1024); err == nil {
				t.Fatal("accepted altered manifest")
			}
		})
	}
	writeManifest()
	write("stdout.complete", []byte("raw SQL"))
	if _, err := ReadCompleteLiquibaseCapture(dir, identity, 1024); err == nil {
		t.Fatal("accepted corrupted stdout")
	}
	write("stdout.complete", nil)
	if err := os.Remove(filepath.Join(dir, "stderr.complete")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCompleteLiquibaseCapture(dir, identity, 1024); err == nil {
		t.Fatal("accepted missing stderr")
	}
}
