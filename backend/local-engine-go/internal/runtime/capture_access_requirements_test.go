package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// accessCaptureFixture provides independently declared bytes for the CA01/03/05
// controls. It grants no Docker authority to the filesystem verifier. See
// docs/architecture/container-capture-access-tests.md.
type accessCaptureFixture struct {
	dir      string
	identity CaptureIdentity
	manifest liquibaseCaptureManifest
	stdout   []byte
	stderr   []byte
}

func newAccessCaptureFixture(t *testing.T, stdout, stderr []byte, exit int) accessCaptureFixture {
	t.Helper()
	f := accessCaptureFixture{dir: t.TempDir(), stdout: stdout, stderr: stderr,
		identity: CaptureIdentity{JobID: "capture-access-job", AttemptNo: 2,
			ExecutionDigest: "sha256:" + strings.Repeat("b", 64),
			HelperDigest:    "sha256:" + strings.Repeat("c", 64), ExitCode: exit}}
	f.manifest = liquibaseCaptureManifest{Protocol: "lb-capture-v2", StdoutEOF: true, StderrEOF: true,
		JobID: f.identity.JobID, TaskID: "plan", AttemptNo: f.identity.AttemptNo,
		ExecutionDigest: f.identity.ExecutionDigest, HelperDigest: f.identity.HelperDigest,
		ChildExitCode: exit, StdoutBytes: int64(len(stdout)), StderrBytes: int64(len(stderr)),
		StdoutSHA256: captureHex(stdout), StderrSHA256: captureHex(stderr)}
	f.writeManifest(t)
	for name, data := range map[string][]byte{"stdout.complete": stdout, "stderr.complete": stderr} {
		if err := os.WriteFile(filepath.Join(f.dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f accessCaptureFixture) writeManifest(t *testing.T) {
	t.Helper()
	data, err := json.Marshal(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "manifest.complete"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func accessInspection(id, name, status string, exit int) []runResponse {
	data, _ := json.Marshal(map[string]any{"Id": id, "Name": "/" + name,
		"State": map[string]any{"Status": status, "ExitCode": exit}})
	return []runResponse{{output: id}, {output: string(data)}}
}

// CA01/OT-PC01: private runtime reads use Docker; the filesystem-only wrapper
// produces the same bytes without issuing any additional Docker commands.
func TestCaptureAccessLocalPlanningControls(t *testing.T) {
	for _, tc := range []struct {
		name            string
		out, diagnostic []byte
		exit            int
	}{
		{name: "empty"},
		{name: "binary", out: []byte{0, 255, 128, '\n'}, diagnostic: []byte{254, 0, 'e'}},
		{name: "natural_failure", diagnostic: []byte("private-error-marker"), exit: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccessCaptureFixture(t, tc.out, tc.diagnostic, tc.exit)
			id, name := strings.Repeat("a", 64), "sqlrs-step-capture-access-plan"
			runner := &captureFilesRequirementsRunner{archives: privateCaptureArchives(t, f.dir, id, "")}
			runner.responses = append(accessInspection(id, name, "exited", tc.exit), accessInspection(id, name, "exited", tc.exit)...)
			rt := NewDocker(Options{Binary: "configured-docker", Runner: runner})
			got, err := rt.ReadCompleteLiquibaseOutput(context.Background(), name, id, f.dir, f.identity, 1024)
			if err != nil || !reflect.DeepEqual(got.Stdout, nonNilBytes(tc.out)) ||
				!reflect.DeepEqual(got.Stderr, nonNilBytes(tc.diagnostic)) ||
				got.StdoutDigest != captureDigest(tc.out) || got.StderrDigest != captureDigest(tc.diagnostic) {
				t.Fatalf("local capture changed: err=%v", err)
			}
			if len(runner.calls) != 4 || len(runner.copies) != 3 {
				t.Fatalf("private capture did not use exact transport/inspections: text=%d copies=%d", len(runner.calls), len(runner.copies))
			}
			for _, call := range runner.calls {
				if call.name != "configured-docker" {
					t.Fatal("configured Docker binary bypassed")
				}
			}
			pure, err := ReadCompleteLiquibaseCapture(f.dir, f.identity, 1024)
			if err != nil || !reflect.DeepEqual(pure, got) || len(runner.calls) != 4 || len(runner.copies) != 3 {
				t.Fatal("filesystem helper changed capture or issued Docker commands")
			}
		})
	}
}

func nonNilBytes(data []byte) []byte {
	return append([]byte{}, data...)
}

// CA02/03/10: validation and ordinary filesystem failures must not be converted
// into convenient success by consulting a second source. Sentinel bytes stay
// private, and even an already-read stdout must not escape after stderr fails.
func TestCaptureAccessPlanningRefusesAlternateSource(t *testing.T) {
	for _, kind := range []string{"missing_manifest", "missing_stdout", "missing_stderr",
		"manifest_json", "manifest_unknown_field", "manifest_trailing_json",
		"wrong_job", "wrong_task", "wrong_attempt", "wrong_execution", "wrong_helper", "wrong_exit",
		"stdout_hash", "stderr_hash", "stdout_length", "stderr_length", "oversize_stdout", "directory_stdout"} {
		t.Run(kind, func(t *testing.T) {
			f := newAccessCaptureFixture(t, []byte("private-sql-marker"), []byte("private-diagnostic-marker"), 0)
			path := func(name string) string { return filepath.Join(f.dir, name+".complete") }
			write := func(name string, data []byte) {
				t.Helper()
				if err := os.WriteFile(path(name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "missing_manifest", "missing_stdout", "missing_stderr":
				if err := os.Remove(path(strings.TrimPrefix(kind, "missing_"))); err != nil {
					t.Fatal(err)
				}
			case "manifest_json":
				write("manifest", []byte("private-sql-marker"))
			case "manifest_unknown_field", "manifest_trailing_json":
				data, err := os.ReadFile(path("manifest"))
				if err != nil {
					t.Fatal(err)
				}
				if kind == "manifest_unknown_field" {
					data = append(data[:len(data)-1], []byte(`,"unknown":true}`)...)
				} else {
					data = append(data, []byte(` {}`)...)
				}
				write("manifest", data)
			case "wrong_job":
				f.manifest.JobID = "other"
				f.writeManifest(t)
			case "wrong_task":
				f.manifest.TaskID = "other"
				f.writeManifest(t)
			case "wrong_attempt":
				f.manifest.AttemptNo++
				f.writeManifest(t)
			case "wrong_execution":
				f.manifest.ExecutionDigest = "sha256:" + strings.Repeat("d", 64)
				f.writeManifest(t)
			case "wrong_helper":
				f.manifest.HelperDigest = "sha256:" + strings.Repeat("d", 64)
				f.writeManifest(t)
			case "wrong_exit":
				f.manifest.ChildExitCode = 7
				f.writeManifest(t)
			case "stdout_hash":
				f.manifest.StdoutSHA256 = strings.Repeat("d", 64)
				f.writeManifest(t)
			case "stderr_hash":
				f.manifest.StderrSHA256 = strings.Repeat("d", 64)
				f.writeManifest(t)
			case "stdout_length":
				f.manifest.StdoutBytes++
				f.writeManifest(t)
			case "stderr_length":
				f.manifest.StderrBytes++
				f.writeManifest(t)
			case "oversize_stdout":
				write("stdout", []byte(strings.Repeat("x", 1025)))
			case "directory_stdout":
				if err := os.Remove(path("stdout")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path("stdout"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			id, name := strings.Repeat("a", 64), "sqlrs-step-capture-access-plan"
			runner := &captureFilesRequirementsRunner{archives: privateCaptureArchives(t, f.dir, id, "")}
			runner.responses = accessInspection(id, name, "exited", 0)
			got, err := NewDocker(Options{Runner: runner}).ReadCompleteLiquibaseOutput(context.Background(), name, id, f.dir, f.identity, 1024)
			if err == nil || !reflect.DeepEqual(got, CompleteCapture{}) || len(runner.calls) != 2 {
				t.Fatalf("invalid capture returned data or consulted another source: err=%v calls=%d", err, len(runner.calls))
			}
			if len(runner.copies) == 0 {
				t.Fatal("invalid artifact did not reach its bound private source")
			}
			for _, secret := range []string{"private-sql-marker", "private-diagnostic-marker"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("capture bytes leaked in error")
				}
			}
		})
	}
}

// CA05: successful artifact reads do not replace the idle probe and final
// identity check. The fake has enough responses to make an accidental success
// visible rather than relying on an exhausted response queue.
func TestCaptureAccessSessionPostReadProof(t *testing.T) {
	for _, kind := range []string{"control", "idle_error", "idle_unknown", "parent_exited", "parent_replaced"} {
		t.Run(kind, func(t *testing.T) {
			dir, identity, _ := stepCaptureFixture(t)
			name := "sqlrs-lb-session-capture-access"
			responses := accessInspection(identity.RunnerID, name, "running", 0)
			probe := runResponse{output: "sqlrs-step-idle-v1\n"}
			if kind == "idle_error" {
				probe.err = errors.New("probe unavailable")
			}
			if kind == "idle_unknown" {
				probe.output = "unknown\n"
			}
			responses = append(responses, probe)
			finalID, status := identity.RunnerID, "running"
			if kind == "parent_exited" {
				status = "exited"
			}
			if kind == "parent_replaced" {
				finalID = strings.Repeat("d", 64)
			}
			responses = append(responses, accessInspection(finalID, name, status, 0)...)
			runner := &captureFilesRequirementsRunner{archives: privateCaptureArchives(t, dir, identity.RunnerID, identity.InvocationRef)}
			runner.responses = responses
			got, err := NewDocker(Options{Runner: runner}).ReadCompleteLiquibaseStepOutput(context.Background(), name, identity.RunnerID, dir, identity, 1024)
			if kind == "control" {
				if err != nil || got.ManifestDigest == "" {
					t.Fatalf("control: %v", err)
				}
			} else if err == nil || !reflect.DeepEqual(got, CompleteStepCapture{}) {
				t.Fatal("unproved step returned a result")
			}
			wantCalls := 5
			if kind == "idle_error" || kind == "idle_unknown" {
				wantCalls = 3
			}
			if len(runner.calls) != wantCalls {
				t.Fatalf("calls=%d want=%d", len(runner.calls), wantCalls)
			}
			if !reflect.DeepEqual(runner.calls[2].args, []string{"exec", "-u", "0", identity.RunnerID, "/bin/sh", "-c", stepIdleProbe}) {
				t.Fatal("idle proof changed or runner was signalled")
			}
		})
	}
}
