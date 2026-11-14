package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// privateJSONFixture declares the PC16 wire contract independently of production
// manifest structs/constants. These are filesystem-parser tests only; they make
// no claim about Docker privacy, readiness, authorization or physical retirement.
// Requirements: liquibase-private-capture-storage-schema.md section 4.
type privateJSONFixture struct {
	dir          string
	manifest     map[string]any
	stdout       []byte
	stderr       []byte
	read         func() (CompleteCapture, int, error)
	expectedStep *StepCaptureIdentity
}

func newPrivateJSONFixture(t *testing.T, step bool, exit int, stdout, stderr []byte) privateJSONFixture {
	t.Helper()
	f := privateJSONFixture{dir: t.TempDir(), stdout: stdout, stderr: stderr}
	helper := "sha256:" + strings.Repeat("c", 64)
	execution := "sha256:" + strings.Repeat("b", 64)
	f.manifest = map[string]any{
		"protocol": "lb-capture-v2", "job_id": "private-json-job", "task_id": "plan",
		"attempt_no": 1, "execution_digest": execution, "helper_digest": helper,
		"child_exit_code": exit, "stdout_eof": true, "stderr_eof": true,
		"stdout_bytes": len(stdout), "stdout_sha256": captureHex(stdout),
		"stderr_bytes": len(stderr), "stderr_sha256": captureHex(stderr),
	}
	if step {
		identity := StepCaptureIdentity{JobID: "private-json-job", TaskID: "execute",
			SessionID: "session-json", InvocationRef: "invocation-json", AttemptNo: 1,
			WorkerGeneration: 1, RunnerID: strings.Repeat("a", 64),
			ExecutionDigest: execution, HelperDigest: helper}
		f.expectedStep = &identity
		f.manifest["protocol"] = "lb-step-capture-v2"
		f.manifest["task_id"] = identity.TaskID
		f.manifest["session_id"] = identity.SessionID
		f.manifest["invocation_ref"] = identity.InvocationRef
		f.manifest["worker_generation"] = identity.WorkerGeneration
		f.manifest["runner_id"] = identity.RunnerID
		f.manifest["prior_retirement_ref"] = nil
		f.read = func() (CompleteCapture, int, error) {
			capture, err := ReadCompleteLiquibaseStepCapture(f.dir, identity, 1024)
			return capture.CompleteCapture, capture.ExitCode, err
		}
	} else {
		identity := CaptureIdentity{JobID: "private-json-job", AttemptNo: 1,
			ExecutionDigest: execution, HelperDigest: helper, ExitCode: exit}
		f.read = func() (CompleteCapture, int, error) {
			capture, err := ReadCompleteLiquibaseCapture(f.dir, identity, 1024)
			return capture, identity.ExitCode, err
		}
	}
	f.write(t)
	return f
}

func (f privateJSONFixture) write(t *testing.T) {
	t.Helper()
	data, err := json.Marshal(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		"manifest.complete": data, "stdout.complete": f.stdout, "stderr.complete": f.stderr,
	} {
		if err := os.WriteFile(filepath.Join(f.dir, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// requireControl prevents an unsupported v2 protocol from making every malformed
// JSON test look successful. Each negative must first accept its intact capture.
func (f privateJSONFixture) requireControl(t *testing.T, exit int) {
	t.Helper()
	capture, observedExit, err := f.read()
	if err != nil {
		t.Fatalf("intact private/v2 parser control unavailable; malformed-input gate not reached: %v", err)
	}
	if observedExit != exit || !bytes.Equal(capture.Stdout, f.stdout) || !bytes.Equal(capture.Stderr, f.stderr) ||
		capture.StdoutDigest != captureDigest(f.stdout) || capture.StderrDigest != captureDigest(f.stderr) {
		t.Fatal("intact private/v2 control changed exit, stream bytes or hashes")
	}
}

// TestLiquibasePrivateCaptureJSONControls covers explicit zero-valued fields,
// empty/binary streams, natural nonzero exits and nullable/nonempty prior refs.
func TestLiquibasePrivateCaptureJSONControls(t *testing.T) {
	for _, step := range []bool{false, true} {
		kind := map[bool]string{false: "planning", true: "step"}[step]
		for _, exit := range []int{0, 7, 255} {
			t.Run(kind+"/exit_"+map[int]string{0: "0", 7: "7", 255: "255"}[exit], func(t *testing.T) {
				f := newPrivateJSONFixture(t, step, exit, nil, nil)
				f.requireControl(t, exit)
				f.stdout, f.stderr = []byte{0, 255, 128, '\n'}, []byte{254, 0, 129}
				f.manifest["stdout_bytes"], f.manifest["stdout_sha256"] = len(f.stdout), captureHex(f.stdout)
				f.manifest["stderr_bytes"], f.manifest["stderr_sha256"] = len(f.stderr), captureHex(f.stderr)
				if step {
					f.manifest["prior_retirement_ref"] = "prune-prior"
					prior := "prune-prior"
					f.expectedStep.PriorRetirementRef = &prior
				}
				f.write(t)
				f.requireControl(t, exit)
			})
		}
	}
}

// TestLiquibasePrivateCaptureJSONRequiredAndUnique checks every required field,
// including explicit zero byte lengths/exit and explicit nullable prior ref.
// Identical duplicate values still reject; last-value-wins is not the protocol.
func TestLiquibasePrivateCaptureJSONRequiredAndUnique(t *testing.T) {
	for _, step := range []bool{false, true} {
		kind := map[bool]string{false: "planning", true: "step"}[step]
		seed := newPrivateJSONFixture(t, step, 0, nil, nil)
		fields := make([]string, 0, len(seed.manifest))
		for field := range seed.manifest {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			for _, mode := range []string{"missing", "duplicate", "duplicate_escaped", "null"} {
				if field == "prior_retirement_ref" && mode == "null" {
					continue // Explicit null is a required positive control, not an error.
				}
				t.Run(kind+"/"+field+"/"+mode, func(t *testing.T) {
					f := newPrivateJSONFixture(t, step, 0, nil, nil)
					f.requireControl(t, 0)
					value, err := json.Marshal(f.manifest[field])
					if err != nil {
						t.Fatal(err)
					}
					switch mode {
					case "missing":
						delete(f.manifest, field)
					case "null":
						f.manifest[field] = nil
					}
					f.write(t)
					if strings.HasPrefix(mode, "duplicate") {
						data, err := os.ReadFile(filepath.Join(f.dir, "manifest.complete"))
						if err != nil {
							t.Fatal(err)
						}
						key := field
						if mode == "duplicate_escaped" {
							key = fmt.Sprintf(`\u%04x%s`, field[0], field[1:])
						}
						data = append(data[:len(data)-1], []byte(",\""+key+"\":"+string(value)+"}")...)
						if err := os.WriteFile(filepath.Join(f.dir, "manifest.complete"), data, 0600); err != nil {
							t.Fatal(err)
						}
					}
					capture, _, err := f.read()
					if err == nil || !reflect.DeepEqual(capture, CompleteCapture{}) {
						t.Fatal("malformed private manifest accepted or returned partial streams")
					}
				})
			}
		}
	}
}

// TestLiquibasePrivateCaptureJSONWireBoundaries distinguishes exact accepted
// limits from truncation, malformed EOF/exit values, and trailing/unknown data.
// Ready/prune JSON and Docker tar/wire quotas remain separate PC05/PC10/CA tests.
func TestLiquibasePrivateCaptureJSONWireBoundaries(t *testing.T) {
	for _, step := range []bool{false, true} {
		kind := map[bool]string{false: "planning", true: "step"}[step]
		for _, name := range []string{
			"manifest_8192", "manifest_8193", "stdout_1024", "stdout_1025",
			"stderr_1024", "stderr_1025", "exit_negative", "exit_256",
			"stdout_not_eof", "stderr_not_eof", "unknown_field", "trailing_json",
			"prior_empty",
			"empty_json", "array_root", "null_root", "empty_object", "truncated_object",
			"malformed_key", "malformed_value", "invalid_utf8", "exit_string", "eof_string",
		} {
			if name == "prior_empty" && !step {
				continue
			}
			t.Run(kind+"/"+name, func(t *testing.T) {
				f := newPrivateJSONFixture(t, step, 0, nil, nil)
				f.requireControl(t, 0)
				wantOK := name == "manifest_8192" || name == "stdout_1024" || name == "stderr_1024"
				switch name {
				case "exit_string":
					f.manifest["child_exit_code"] = "0"
				case "eof_string":
					f.manifest["stdout_eof"] = "true"
				case "exit_negative":
					f.manifest["child_exit_code"] = -1
				case "exit_256":
					f.manifest["child_exit_code"] = 256
				case "stdout_not_eof":
					f.manifest["stdout_eof"] = false
				case "stderr_not_eof":
					f.manifest["stderr_eof"] = false
				case "unknown_field":
					f.manifest["unrecognized"] = "private-json-sentinel"
				case "prior_empty":
					f.manifest["prior_retirement_ref"] = ""
				case "stdout_1024", "stdout_1025", "stderr_1024", "stderr_1025":
					size := 1024
					if strings.HasSuffix(name, "1025") {
						size++
					}
					stream := bytes.Repeat([]byte{0xff}, size)
					if strings.HasPrefix(name, "stdout") {
						f.stdout = stream
						f.manifest["stdout_bytes"], f.manifest["stdout_sha256"] = size, captureHex(stream)
					} else {
						f.stderr = stream
						f.manifest["stderr_bytes"], f.manifest["stderr_sha256"] = size, captureHex(stream)
					}
				}
				f.write(t)
				malformed := map[string][]byte{
					"empty_json":       nil,
					"array_root":       []byte(`[]`),
					"null_root":        []byte(`null`),
					"empty_object":     []byte(`{}`),
					"truncated_object": []byte(`{`),
					"malformed_key":    []byte(`{false:1}`),
					"malformed_value":  []byte(`{"protocol":}`),
				}
				if data, replace := malformed[name]; replace {
					if err := os.WriteFile(filepath.Join(f.dir, "manifest.complete"), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if name == "invalid_utf8" {
					path := filepath.Join(f.dir, "manifest.complete")
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					data = bytes.Replace(data, []byte("private-json-job"), []byte{'p', 0xff}, 1)
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if strings.HasPrefix(name, "manifest_") || name == "trailing_json" {
					path := filepath.Join(f.dir, "manifest.complete")
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if name == "trailing_json" {
						data = append(data, []byte(" {}")...)
					} else {
						size := 8192
						if name == "manifest_8193" {
							size++
						}
						if len(data) > size {
							t.Fatal("fixture already exceeds selected manifest boundary")
						}
						data = append(data, bytes.Repeat([]byte(" "), size-len(data))...)
					}
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if wantOK {
					f.requireControl(t, 0)
					return
				}
				capture, _, err := f.read()
				if err == nil || !reflect.DeepEqual(capture, CompleteCapture{}) {
					t.Fatal("invalid/beyond-limit capture accepted or partially returned")
				}
				if strings.Contains(err.Error(), "private-json-sentinel") {
					t.Fatal("manifest diagnostic exposed protected input")
				}
			})
		}
	}
}

// TestLiquibasePrivateCapturePriorRefMatchesReservation checks the nullable
// wire link against an independently supplied saved slot, never against a ref
// inferred from the manifest being verified.
func TestLiquibasePrivateCapturePriorRefMatchesReservation(t *testing.T) {
	for _, variant := range []string{"missing_expected", "wrong_expected", "missing_manifest"} {
		t.Run(variant, func(t *testing.T) {
			f := newPrivateJSONFixture(t, true, 0, nil, nil)
			prior := "saved-retirement"
			f.expectedStep.PriorRetirementRef = &prior
			f.manifest["prior_retirement_ref"] = prior
			f.write(t)
			f.requireControl(t, 0)
			switch variant {
			case "missing_expected":
				f.expectedStep.PriorRetirementRef = nil
			case "wrong_expected":
				other := "foreign-retirement"
				f.expectedStep.PriorRetirementRef = &other
			case "missing_manifest":
				f.manifest["prior_retirement_ref"] = nil
				f.write(t)
			}
			if capture, _, err := f.read(); err == nil || !reflect.DeepEqual(capture, CompleteCapture{}) {
				t.Fatal("foreign/missing reservation link accepted or returned private streams")
			}
		})
	}
}
