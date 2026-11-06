package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// privatePruneDockerFixture owns one real initialized parent and one completed
// raw capture. Publication authority is supplied explicitly by this helper-level
// test; only queue tests can qualify durable publication/pin admission.
type privatePruneDockerFixture struct {
	runtime  *DockerRuntime
	identity StepCaptureIdentity
	name, id string
	first    CompleteStepCapture
}

func newPrivatePruneDockerFixture(t *testing.T, ctx context.Context) privatePruneDockerFixture {
	t.Helper()
	helper, err := os.ReadFile("assets/liquibase-step-capture-v2.sh")
	if err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(t.TempDir(), "step-capture.sh")
	if err := os.WriteFile(helperPath, helper, 0444); err != nil {
		t.Fatal(err)
	}
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	if err != nil {
		t.Fatal("required image unavailable")
	}
	image := strings.TrimSpace(string(imageBytes))
	if !validCaptureDigest(image) {
		t.Fatal("unresolved image")
	}
	ready := SessionReadinessIdentity{SessionID: "private-prune-session", WorkerGeneration: 2, RunnerConfigDigest: "sha256:" + strings.Repeat("b", 64), HelperDigest: captureDigest(helper)}
	_, runErr, name, id := runRetainedCaptureTestContainer(t, ctx, []string{"run", "-d", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh",
		"-v", dockerBindSpec(helperPath, "/private-helper/step-capture.sh", true), "-e", "SQLRS_SESSION_ID=" + ready.SessionID,
		"-e", "SQLRS_WORKER_GENERATION=2", "-e", "SQLRS_RUNNER_CONFIG_DIGEST=" + ready.RunnerConfigDigest, "-e", "SQLRS_HELPER_DIGEST=" + ready.HelperDigest,
		image, "/private-helper/step-capture.sh", "serve"})
	if runErr != nil || !createdContainerID.MatchString(id) {
		t.Fatal("owned prune fixture failed to launch")
	}
	rt := NewDocker(Options{Binary: "docker"})
	ready.RunnerID = id
	readyCtx, readyCancel := context.WithTimeout(ctx, 15*time.Second)
	defer readyCancel()
	for {
		if _, err := rt.ReadLiquibaseSessionReadiness(readyCtx, name, ready); err == nil {
			break
		}
		if readyCtx.Err() != nil {
			t.Fatal("prune fixture readiness unavailable")
		}
		time.Sleep(25 * time.Millisecond)
	}
	f := privatePruneDockerFixture{runtime: rt, name: name, id: id, identity: StepCaptureIdentity{JobID: "private-prune-job", TaskID: "step-1", SessionID: ready.SessionID,
		InvocationRef: "private-prune-1", AttemptNo: 1, WorkerGeneration: 2, RunnerID: id, ExecutionDigest: "sha256:" + strings.Repeat("d", 64), HelperDigest: ready.HelperDigest}}
	if data, err := exec.CommandContext(ctx, "docker", f.execArgs("private-prune-1", "", false)...).CombinedOutput(); err != nil || len(data) != 0 {
		t.Fatal("first private child unavailable")
	}
	f.first, err = rt.ReadCompleteLiquibaseStepOutput(ctx, name, id, "", f.identity, 1024)
	if err != nil {
		t.Fatal("first capture not verified")
	}
	return f
}

func (f privatePruneDockerFixture) execArgs(invocation, authorization string, probe bool) []string {
	task := "step-2"
	if invocation == f.identity.InvocationRef {
		task = f.identity.TaskID
	}
	args := []string{"exec", "-u", "0", "-e", "SQLRS_CAPTURE_DIR=/sqlrs/capture/" + invocation,
		"-e", "SQLRS_JOB_ID=" + f.identity.JobID, "-e", "SQLRS_TASK_ID=" + task, "-e", "SQLRS_INVOCATION_REF=" + invocation,
		"-e", "SQLRS_ATTEMPT_NO=1", "-e", "SQLRS_RUNNER_ID=" + f.id, "-e", "SQLRS_EXECUTION_DIGEST=" + f.identity.ExecutionDigest,
		"-e", "SQLRS_CAPTURE_MAX_BYTES=1024", "-e", "SQLRS_CAPTURE_PRUNE_AUTHORIZATION=" + authorization, f.id, "/bin/sh", "/private-helper/step-capture.sh"}
	if probe {
		return append(args, "probe-prune")
	}
	return append(args, "/bin/sh", "-c", `printf x >> /tmp/sqlrs-private-prune-count; printf private-prune-output`)
}

func TestLiquibasePrivatePruneDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("native prune suite not selected")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		t.Fatal("ordinary Linux engine UID required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	f := newPrivatePruneDockerFixture(t, ctx)
	authorization := func() map[string]any {
		return map[string]any{"protocol": "lb-capture-prune-v1", "retirement_ref": "private-retirement-1",
			"session_id": f.identity.SessionID, "worker_generation": 2, "runner_id": f.id, "prior_invocation_ref": f.identity.InvocationRef,
			"prior_manifest_digest": f.first.ManifestDigest, "next_invocation_ref": "private-prune-2", "helper_digest": f.identity.HelperDigest}
	}
	variants := []string{"unknown", "duplicate", "escaped_duplicate", "oversize", "trailing", "null", "string_generation", "zero_generation", "overflow_generation", "protocol", "session", "runner", "helper", "prior", "self", "next", "manifest_hash"}
	for key := range authorization() {
		variants = append(variants, "missing_"+key)
	}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			fields := authorization()
			switch variant {
			case "unknown":
				fields["path"] = "/private-target"
			case "null":
				fields["retirement_ref"] = nil
			case "string_generation":
				fields["worker_generation"] = "2"
			case "zero_generation":
				fields["worker_generation"] = 0
			case "protocol":
				fields["protocol"] = "lb-capture-prune-v2"
			case "session":
				fields["session_id"] = "foreign-session"
			case "runner":
				fields["runner_id"] = strings.Repeat("e", 64)
			case "helper":
				fields["helper_digest"] = "sha256:" + strings.Repeat("e", 64)
			case "prior":
				fields["prior_invocation_ref"] = "../private-prune-1"
			case "self":
				fields["prior_invocation_ref"] = "private-prune-2"
			case "next":
				fields["next_invocation_ref"] = "foreign-next"
			case "manifest_hash":
				fields["prior_manifest_digest"] = "sha256:" + strings.Repeat("e", 64)
			}
			if strings.HasPrefix(variant, "missing_") {
				delete(fields, strings.TrimPrefix(variant, "missing_"))
			}
			data, _ := json.Marshal(fields)
			switch variant {
			case "duplicate":
				data = append(data[:len(data)-1], []byte(`,"retirement_ref":"private-retirement-1"}`)...)
			case "escaped_duplicate":
				data = append(data[:len(data)-1], []byte(`,"retirement\u005fref":"private-retirement-1"}`)...)
			case "oversize":
				data = append(data, []byte(strings.Repeat(" ", 4097))...)
			case "trailing":
				data = append(data, []byte(" {}")...)
			case "overflow_generation":
				data = []byte(strings.Replace(string(data), `"worker_generation":2`, `"worker_generation":9223372036854775808`, 1))
			}
			wire, err := exec.CommandContext(ctx, "docker", f.execArgs("private-prune-2", string(data), false)...).CombinedOutput()
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 90 || len(wire) != 0 {
				t.Fatal("invalid prune authorization not rejected privately")
			}
			old, err := f.runtime.ReadCompleteLiquibaseStepOutput(ctx, f.name, f.id, "", f.identity, 1024)
			if err != nil || old.ManifestDigest != f.first.ManifestDigest {
				t.Fatal("rejected prune changed old capture")
			}
			check := `test ! -e /sqlrs/capture/private-prune-2 && test "$(cat /tmp/sqlrs-private-prune-count)" = x`
			if err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", f.id, "/bin/sh", "-c", check).Run(); err != nil {
				t.Fatal("rejected prune created a slot or started child")
			}
		})
	}
	data, _ := json.Marshal(authorization())
	// Valid escapes must decode, not merely be rejected together with malformed
	// objects. The receipt still contains the exact saved unescaped token.
	data = []byte(strings.Replace(string(data), `"retirement_ref":"private-retirement-1"`, `"retirement\u005fref":"private\u002dretirement-1"`, 1))
	t.Run("valid_prune_and_child", func(t *testing.T) {
		wire, err := exec.CommandContext(ctx, "docker", f.execArgs("private-prune-2", string(data), false)...).CombinedOutput()
		if err != nil || len(wire) != 0 {
			t.Fatal("valid typed prune did not execute the next child privately")
		}
		identity := f.identity
		identity.TaskID = "step-2"
		identity.InvocationRef = "private-prune-2"
		prior := "private-retirement-1"
		identity.PriorRetirementRef = &prior
		capture, err := f.runtime.ReadCompleteLiquibaseStepOutput(ctx, f.name, f.id, "", identity, 1024)
		if err != nil || capture.ExitCode != 0 || string(capture.Stdout) != "private-prune-output" {
			t.Fatal("second capture lacks exact prune receipt")
		}
		check := `test ! -e /sqlrs/capture/private-prune-1 && test "$(cat /tmp/sqlrs-private-prune-count)" = xx`
		if err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", f.id, "/bin/sh", "-c", check).Run(); err != nil {
			t.Fatal("prune target or actual child count differs")
		}
		wire, err = exec.CommandContext(ctx, "docker", f.execArgs("private-prune-2", string(data), true)...).CombinedOutput()
		if err != nil || string(wire) != "sqlrs-capture-prune-absent-v1\n" {
			t.Fatal("read-only exact absence probe failed")
		}
		if err := exec.CommandContext(ctx, "docker", f.execArgs("private-prune-2", string(data), false)...).Run(); err == nil {
			t.Fatal("combined prune/child command replay accepted")
		}
		if err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", f.id, "/bin/sh", "-c", check).Run(); err != nil {
			t.Fatal("probe/replay changed actual child count")
		}
	})
}

// PC08: confirmed/idempotently observed old-path absence may precede a proved
// first invocation. This helper-only D control supplies its authority explicitly;
// it does not derive SQL non-start from absence or qualify journal recovery.
func TestLiquibasePrivatePruneAbsentTargetDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("native absent-target suite not selected")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		t.Fatal("ordinary Linux engine UID required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	f := newPrivatePruneDockerFixture(t, ctx)
	remove := `rm /sqlrs/capture/private-prune-1/manifest.complete /sqlrs/capture/private-prune-1/stdout.complete /sqlrs/capture/private-prune-1/stderr.complete && rmdir /sqlrs/capture/private-prune-1`
	if err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", f.id, "/bin/sh", "-c", remove).Run(); err != nil {
		t.Fatal("exact owned old-output disposal failed")
	}
	authority, _ := json.Marshal(map[string]any{"protocol": "lb-capture-prune-v1", "retirement_ref": "already-confirmed-retirement",
		"session_id": f.identity.SessionID, "worker_generation": 2, "runner_id": f.id, "prior_invocation_ref": f.identity.InvocationRef,
		"prior_manifest_digest": f.first.ManifestDigest, "next_invocation_ref": "private-prune-2", "helper_digest": f.identity.HelperDigest})
	wire, err := exec.CommandContext(ctx, "docker", f.execArgs("private-prune-2", string(authority), true)...).CombinedOutput()
	if err != nil || string(wire) != "sqlrs-capture-prune-absent-v1\n" {
		t.Fatal("exact absent-target probe failed")
	}
	check := `test ! -e /sqlrs/capture/private-prune-2 && test "$(cat /tmp/sqlrs-private-prune-count)" = x`
	if err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", f.id, "/bin/sh", "-c", check).Run(); err != nil {
		t.Fatal("read-only probe created files or started child")
	}
	wire, err = exec.CommandContext(ctx, "docker", f.execArgs("private-prune-2", string(authority), false)...).CombinedOutput()
	if err != nil || len(wire) != 0 {
		t.Fatal("authorized old absence blocked the first next invocation")
	}
	identity := f.identity
	identity.TaskID = "step-2"
	identity.InvocationRef = "private-prune-2"
	prior := "already-confirmed-retirement"
	identity.PriorRetirementRef = &prior
	if capture, err := f.runtime.ReadCompleteLiquibaseStepOutput(ctx, f.name, f.id, "", identity, 1024); err != nil || string(capture.Stdout) != "private-prune-output" {
		t.Fatal("first next capture after confirmed prune differs")
	}
	if err := exec.CommandContext(ctx, "docker", f.execArgs("private-prune-2", string(authority), false)...).Run(); err == nil {
		t.Fatal("existing current-directory fence was bypassed")
	}
	count, err := exec.CommandContext(ctx, "docker", "exec", "-u", "0", f.id, "/bin/cat", "/tmp/sqlrs-private-prune-count").Output()
	if err != nil || string(count) != "xx" {
		t.Fatal("confirmed old absence authorized a repeated child")
	}
}
