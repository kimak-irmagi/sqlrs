package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stepCaptureFixture(t *testing.T) (string, StepCaptureIdentity, map[string]any) {
	t.Helper()
	dir := t.TempDir()
	identity := StepCaptureIdentity{JobID: "job", TaskID: "execute-2", SessionID: "session-1",
		InvocationRef: "invocation-3", AttemptNo: 1, WorkerGeneration: 7,
		RunnerID: strings.Repeat("a", 64), ExecutionDigest: "sha256:" + strings.Repeat("b", 64),
		HelperDigest: "sha256:" + strings.Repeat("c", 64)}
	manifest := map[string]any{"protocol": "lb-step-capture-v2", "prior_retirement_ref": nil,
		"job_id": identity.JobID, "task_id": identity.TaskID, "session_id": identity.SessionID,
		"invocation_ref": identity.InvocationRef, "attempt_no": identity.AttemptNo,
		"worker_generation": identity.WorkerGeneration, "runner_id": identity.RunnerID,
		"execution_digest": identity.ExecutionDigest, "helper_digest": identity.HelperDigest,
		"child_exit_code": 0, "stdout_eof": true, "stderr_eof": true,
		"stdout_bytes": 0, "stdout_sha256": captureHex(nil),
		"stderr_bytes": 0, "stderr_sha256": captureHex(nil)}
	writeStepCaptureFixture(t, dir, manifest, nil, nil)
	return dir, identity, manifest
}

func writeStepCaptureFixture(t *testing.T, dir string, manifest map[string]any, stdout, stderr []byte) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string][]byte{"manifest.complete": data, "stdout.complete": stdout, "stderr.complete": stderr} {
		if err := os.WriteFile(filepath.Join(dir, name), value, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// N03/R03c: the child result is authoritative only with both EOFs, all bytes,
// and the exact session/invocation identity. A natural nonzero child exit is
// a known failure, not a missing capture. See liquibase-reusable-container-tests.md.
func TestReadCompleteLiquibaseStepCapture(t *testing.T) {
	for _, exitCode := range []int{0, 7} {
		t.Run(map[int]string{0: "success", 7: "natural failure"}[exitCode], func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			manifest["child_exit_code"] = exitCode
			writeStepCaptureFixture(t, dir, manifest, nil, nil)
			capture, err := ReadCompleteLiquibaseStepCapture(dir, identity, 1024)
			if err != nil || capture.ExitCode != exitCode || capture.StdoutDigest != captureDigest(nil) || capture.ManifestDigest == "" {
				t.Fatalf("capture=%+v err=%v", capture, err)
			}
		})
	}
	for _, field := range []string{"protocol", "job_id", "task_id", "session_id", "invocation_ref", "runner_id", "execution_digest", "helper_digest", "stdout_sha256", "stderr_sha256"} {
		t.Run("wrong "+field, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			manifest[field] = "different"
			writeStepCaptureFixture(t, dir, manifest, nil, nil)
			if _, err := ReadCompleteLiquibaseStepCapture(dir, identity, 1024); err == nil {
				t.Fatal("altered capture identity or hash was accepted")
			}
		})
	}
	for _, field := range []string{"attempt_no", "worker_generation", "stdout_bytes", "stderr_bytes"} {
		t.Run("wrong "+field, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			manifest[field] = -1
			writeStepCaptureFixture(t, dir, manifest, nil, nil)
			if _, err := ReadCompleteLiquibaseStepCapture(dir, identity, 1024); err == nil {
				t.Fatal("altered capture ordinal or length was accepted")
			}
		})
	}
	for _, field := range []string{"child_exit_code", "stdout_eof", "stderr_eof"} {
		t.Run("missing "+field, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			delete(manifest, field)
			writeStepCaptureFixture(t, dir, manifest, nil, nil)
			if _, err := ReadCompleteLiquibaseStepCapture(dir, identity, 1024); err == nil {
				t.Fatal("incomplete child result was accepted")
			}
		})
	}
	for _, name := range []string{"manifest.complete", "stdout.complete", "stderr.complete"} {
		t.Run("missing "+name, func(t *testing.T) {
			dir, identity, _ := stepCaptureFixture(t)
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadCompleteLiquibaseStepCapture(dir, identity, 1024); err == nil {
				t.Fatal("missing complete file was accepted")
			}
		})
	}
	for _, name := range []string{"stdout.complete", "stderr.complete"} {
		t.Run("corrupt "+name, func(t *testing.T) {
			dir, identity, _ := stepCaptureFixture(t)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("protected SQL"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadCompleteLiquibaseStepCapture(dir, identity, 1024); err == nil {
				t.Fatal("corrupt stream was accepted")
			}
		})
	}
}

// N03: completion is checked while the exact parent container is alive.
// Surviving untrusted child processes fence the session, and Docker diagnostic
// output never supplies a missing child manifest.
func TestDockerReadLiquibaseStepOutputRequiresIdleExactRunner(t *testing.T) {
	for _, tc := range []struct {
		name, status, idle string
		probeErr           error
		wantOK             bool
	}{
		{name: "running idle", status: "running", idle: "sqlrs-step-idle-v1\n", wantOK: true},
		{name: "stopped complete branch", status: "exited", wantOK: true},
		{name: "surviving child", status: "running", idle: "child survives\n"},
		{name: "unknown process state", status: "running", probeErr: errors.New("lost exec reply")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, identity, _ := stepCaptureFixture(t)
			name := "sqlrs-lb-session-1"
			inspection := `{"Id":"` + identity.RunnerID + `","Name":"/` + name + `","State":{"Status":"` + tc.status + `","ExitCode":0}}`
			runner := &captureFilesRequirementsRunner{archives: sessionBundleFixture(t, dir, identity.RunnerID, identity.InvocationRef)}
			runner.responses = []runResponse{{output: inspection}}
			if tc.status == "running" {
				runner.responses = append(runner.responses, runResponse{output: tc.idle, err: tc.probeErr})
			}
			runner.responses = append(runner.responses, runResponse{output: inspection})
			rt := NewDocker(Options{Binary: "docker", Runner: runner})
			_, err := rt.ReadCompleteLiquibaseStepOutput(context.Background(), name, identity.RunnerID, dir, identity, 1024)
			if (err == nil) != tc.wantOK {
				t.Fatalf("err=%v wantOK=%t calls=%+v", err, tc.wantOK, runner.calls)
			}
			for _, call := range runner.calls {
				if len(call.args) > 0 && call.args[0] == "logs" {
					t.Fatal("diagnostic logs were used as step capture")
				}
			}
		})
	}
}

// N03: Docker exec returning zero cannot replace the protected manifest.
// A fully captured nonzero exit is returned as a known child result even
// with successful helper transport; the child code belongs to the manifest.
func TestInvokeTrackedLiquibaseStepRequiresCapturedResult(t *testing.T) {
	for _, tc := range []struct {
		name      string
		manifest  bool
		childExit int
		execErr   error
		wantOK    bool
	}{
		{name: "early zero without manifest"},
		{name: "complete child success", manifest: true, wantOK: true},
		{name: "complete child failure", manifest: true, childExit: 7, wantOK: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			manifest["child_exit_code"] = tc.childExit
			writeStepCaptureFixture(t, dir, manifest, nil, nil)
			manifestBytes, stdout, stderr := stepResultFiles(t, dir)
			wire := literalStepResultFrame(manifestBytes, stdout, stderr, nil, false)
			if !tc.manifest {
				if err := os.Remove(filepath.Join(dir, "manifest.complete")); err != nil {
					t.Fatal(err)
				}
				wire = nil
			}
			name := "sqlrs-lb-session-1"
			inspection := `{"Id":"` + identity.RunnerID + `","Name":"/` + name + `","State":{"Status":"running","ExitCode":0}}`
			runner := &inlineFixtureRunner{&captureFilesRequirementsRunner{}}
			runner.responses = []runResponse{{output: inspection},
				{output: string(wire), err: tc.execErr}, {output: inspection}}
			rt := NewDocker(Options{Binary: "docker", Runner: runner})
			capture, err := rt.InvokeTrackedLiquibaseStep(context.Background(), name, dir, identity,
				[]string{"update-count", "--count", "1"}, 1024)
			if (err == nil) != tc.wantOK || tc.wantOK && capture.ExitCode != tc.childExit {
				t.Fatalf("capture=%+v err=%v wantOK=%t", capture, err, tc.wantOK)
			}
			args := runner.calls[1].args
			if len(args) == 0 || args[0] != "exec" || !containsArg(args, "-u", "0") ||
				!containsArg(args, "-e", "SQLRS_INVOCATION_REF="+identity.InvocationRef) ||
				!strings.Contains(strings.Join(args, " "), identity.RunnerID+" /bin/sh /sqlrs/helper/step-capture.sh liquibase update-count --count 1") {
				t.Fatalf("unattributed step invocation: %v", args)
			}
		})
	}
}
