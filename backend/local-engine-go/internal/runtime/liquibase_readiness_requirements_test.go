package runtime

import (
	"archive/tar"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// PC05/PC16: fixture bytes and native observations are supplied independently.
// These are H checks, not evidence that the real helper initialized privately.
func readinessRequirementsFixture(t *testing.T) (SessionReadinessIdentity, []byte) {
	t.Helper()
	want := SessionReadinessIdentity{SessionID: "saved-session", WorkerGeneration: 2,
		RunnerID: strings.Repeat("a", 64), RunnerConfigDigest: "sha256:" + strings.Repeat("b", 64),
		HelperDigest: "sha256:" + strings.Repeat("c", 64)}
	data, err := json.Marshal(map[string]any{"protocol": "lb-capture-ready-v1", "session_id": want.SessionID,
		"worker_generation": want.WorkerGeneration, "runner_config_digest": want.RunnerConfigDigest,
		"helper_digest": want.HelperDigest, "capture_protocol_revision": "lb-step-capture-v2"})
	if err != nil {
		t.Fatal(err)
	}
	return want, data
}

func readinessRequirementsRunner(t *testing.T, want SessionReadinessIdentity, data []byte) *captureFilesRequirementsRunner {
	t.Helper()
	runner := &captureFilesRequirementsRunner{archives: map[string][]byte{
		want.RunnerID + ":/sqlrs/capture/ready.complete": artifactRequirementsTar(t, "ready.complete", data, tar.TypeReg, false),
	}}
	runner.responses = append(accessInspection(want.RunnerID, "sqlrs-lb-session-ready", "running", 0),
		accessInspection(want.RunnerID, "sqlrs-lb-session-ready", "running", 0)...)
	return runner
}

func TestLiquibasePrivateReadinessExactObservation(t *testing.T) {
	for _, variant := range []string{"control", "session", "generation", "config", "helper", "protocol", "capture_protocol",
		"missing", "unknown", "duplicate", "escaped_duplicate", "null_generation", "string_generation", "zero_generation",
		"fraction_generation", "trailing", "array", "invalid_utf8", "oversize", "absent", "wrong_file", "symlink",
		"before_id", "before_exited", "before_paused", "after_id", "after_exited", "after_absent"} {
		t.Run(variant, func(t *testing.T) {
			want, data := readinessRequirementsFixture(t)
			runner := readinessRequirementsRunner(t, want, data)
			var fields map[string]any
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "session":
				fields["session_id"] = "other-session"
			case "generation":
				fields["worker_generation"] = 3
			case "config":
				fields["runner_config_digest"] = "sha256:" + strings.Repeat("d", 64)
			case "helper":
				fields["helper_digest"] = "sha256:" + strings.Repeat("d", 64)
			case "protocol":
				fields["protocol"] = "lb-capture-ready-v2"
			case "capture_protocol":
				fields["capture_protocol_revision"] = "lb-step-capture-v1"
			case "missing":
				delete(fields, "worker_generation")
			case "unknown":
				fields["runner_id"] = want.RunnerID
			case "null_generation":
				fields["worker_generation"] = nil
			case "string_generation":
				fields["worker_generation"] = "2"
			case "zero_generation":
				fields["worker_generation"] = 0
			case "fraction_generation":
				fields["worker_generation"] = 2.5
			}
			data, _ = json.Marshal(fields)
			switch variant {
			case "duplicate":
				data = append(data[:len(data)-1], []byte(`,"session_id":"saved-session"}`)...)
			case "escaped_duplicate":
				data = append(data[:len(data)-1], []byte(`,"session\u005fid":"saved-session"}`)...)
			case "trailing":
				data = append(data, []byte(" {}")...)
			case "array":
				data = []byte("[]")
			case "invalid_utf8":
				data = append([]byte(`{"x":"`), 255, '"', '}')
			case "oversize":
				data = append(data, []byte(strings.Repeat(" ", 8193))...)
			case "before_id":
				runner.responses = accessInspection(strings.Repeat("f", 64), "sqlrs-lb-session-ready", "running", 0)
			case "before_exited":
				runner.responses = accessInspection(want.RunnerID, "sqlrs-lb-session-ready", "exited", 0)
			case "before_paused":
				runner.responses = accessInspection(want.RunnerID, "sqlrs-lb-session-ready", "paused", 0)
			case "after_id":
				runner.responses = append(accessInspection(want.RunnerID, "sqlrs-lb-session-ready", "running", 0), accessInspection(strings.Repeat("f", 64), "sqlrs-lb-session-ready", "running", 0)...)
			case "after_exited":
				runner.responses = append(accessInspection(want.RunnerID, "sqlrs-lb-session-ready", "running", 0), accessInspection(want.RunnerID, "sqlrs-lb-session-ready", "exited", 0)...)
			case "after_absent":
				runner.responses = accessInspection(want.RunnerID, "sqlrs-lb-session-ready", "running", 0)
			}
			key := want.RunnerID + ":/sqlrs/capture/ready.complete"
			file, kind := "ready.complete", byte(tar.TypeReg)
			if variant == "wrong_file" {
				file = "manifest.complete"
			}
			if variant == "symlink" {
				kind = tar.TypeSymlink
				data = nil
			}
			runner.archives[key] = artifactRequirementsTar(t, file, data, kind, false)
			if variant == "absent" {
				delete(runner.archives, key)
			}
			got, err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).ReadLiquibaseSessionReadiness(context.Background(), "sqlrs-lb-session-ready", want)
			if variant == "control" {
				if err != nil || got.ReadyDigest != captureDigest(data) || got.ReadyBytes != int64(len(data)) || len(runner.copies) != 1 || len(runner.calls) != 4 {
					t.Fatalf("exact ready marker was not independently verified: err=%v digest_ok=%t bytes=%d/%d copies=%d inspections=%d", err, got.ReadyDigest == captureDigest(data), got.ReadyBytes, len(data), len(runner.copies), len(runner.calls))
				}
				if runner.deadlines[0].IsZero() {
					t.Fatal("ready copy lacks a bounded deadline")
				}
				return
			}
			if err == nil || !reflect.DeepEqual(got, SessionReadiness{}) {
				t.Fatal("invalid readiness returned evidence")
			}
			if strings.HasPrefix(variant, "before_") && len(runner.copies) != 0 {
				t.Fatal("foreign/inactive parent reached private copy")
			}
		})
	}
}

func TestLiquibasePrivateReadinessRejectsInvalidBinding(t *testing.T) {
	for _, variant := range []string{"session", "runner", "generation", "config", "helper", "cancelled", "nil_runtime"} {
		t.Run(variant, func(t *testing.T) {
			want, data := readinessRequirementsFixture(t)
			runner := readinessRequirementsRunner(t, want, data)
			runtime := NewDocker(Options{Binary: "configured-docker", Runner: runner})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch variant {
			case "session":
				want.SessionID = "../foreign"
			case "runner":
				want.RunnerID = "short"
			case "generation":
				want.WorkerGeneration = 0
			case "config":
				want.RunnerConfigDigest = "private-invalid-value"
			case "helper":
				want.HelperDigest = "private-invalid-value"
			case "cancelled":
				cancel()
			case "nil_runtime":
				runtime = nil
			}
			got, err := runtime.ReadLiquibaseSessionReadiness(ctx, "sqlrs-lb-session-ready", want)
			if err == nil || got != (SessionReadiness{}) || len(runner.calls) != 0 || len(runner.copies) != 0 {
				t.Fatal("invalid readiness binding reached Docker or returned evidence")
			}
		})
	}
}

// PC05/CA08: copy cancellation and the caller deadline cannot be reset or
// converted into readiness, including cancellation during final inspection.
func TestLiquibasePrivateReadinessDeadlineAndCancellation(t *testing.T) {
	for _, variant := range []string{"caller_deadline", "cancel_during_copy", "cancel_after_inspection"} {
		t.Run(variant, func(t *testing.T) {
			want, data := readinessRequirementsFixture(t)
			runner := readinessRequirementsRunner(t, want, data)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			deadline, _ := ctx.Deadline()
			if variant == "cancel_during_copy" {
				runner.afterCopy = cancel
			}
			if variant == "cancel_after_inspection" {
				runner.afterText = func(calls int) {
					if calls == 4 {
						cancel()
					}
				}
			}
			proof, err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).ReadLiquibaseSessionReadiness(ctx, "sqlrs-lb-session-ready", want)
			if len(runner.deadlines) != 1 || !runner.deadlines[0].Equal(deadline) {
				t.Fatal("caller deadline replaced or not propagated")
			}
			if variant == "caller_deadline" {
				if err != nil || proof.ReadyDigest != captureDigest(data) {
					t.Fatal("intact caller-budget control failed")
				}
				return
			}
			if err == nil || proof != (SessionReadiness{}) {
				t.Fatal("cancelled observation returned readiness")
			}
		})
	}
}
