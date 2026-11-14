package runtime

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// CT08: real runtime entry points propagate the observer to real command/copy
// boundaries; source-catalog membership alone is not evidence of connection.
func TestCostRuntimeReadBoundaries(t *testing.T) {
	for _, variant := range []string{"off", "running", "exited", "idle-failure"} {
		t.Run(variant, func(t *testing.T) {
			dir, identity, _ := stepCaptureFixture(t)
			name, phase := "sqlrs-lb-cost-boundary", "running"
			if variant == "exited" {
				phase = "exited"
			}
			runner := &captureFilesRequirementsRunner{archives: sessionBundleFixture(t, dir, identity.RunnerID, identity.InvocationRef)}
			runner.responses = presentInspection(identity.RunnerID, name, phase, 23)
			if phase == "running" {
				probe := runResponse{output: "sqlrs-step-idle-v1\n"}
				if variant == "idle-failure" {
					probe.err = errors.New("private-probe-error")
				}
				runner.responses = append(runner.responses, probe)
			}
			runner.responses = append(runner.responses, presentInspection(identity.RunnerID, name, phase, 23)...)
			recorder := preparecost.NewRecorder(variant != "off", 1)
			ctx := preparecost.WithPhase(preparecost.WithRecorder(t.Context(), recorder), preparecost.PhaseStep, 3)
			got, err := NewDocker(Options{Runner: runner}).ReadCompleteLiquibaseStepOutput(ctx, name, identity.RunnerID, "", identity, 1024)
			failed := variant == "idle-failure"
			if (err != nil) != failed || !failed && got.ManifestDigest == "" {
				t.Fatal("observation changed capture outcome", err)
			}
			report := recorder.Finish()
			if variant == "off" {
				if len(report.Spans) != 0 || len(runner.calls) != 3 || len(runner.copies) != 1 {
					t.Fatal("disabled recording changed runtime work")
				}
				return
			}
			want := []string{"capture.read", "docker.command", "docker.capture_copy", "docker.command", "docker.command"}
			if variant == "exited" || failed {
				want = want[:4]
			}
			keys := make([]string, len(report.Spans))
			for i, span := range report.Spans {
				keys[i] = string(span.Operation)
				if span.Step != 3 || span.Phase != preparecost.PhaseStep || i > 0 && span.Parent != 1 {
					t.Fatal("command lost exact caller/step nesting", span)
				}
			}
			if !report.Complete || !reflect.DeepEqual(keys, want) {
				t.Fatal("real runtime boundaries bypassed observer", keys, want)
			}
			outcome := preparecost.Succeeded
			if failed {
				outcome = preparecost.Failed
			}
			if report.Spans[0].Outcome != outcome {
				t.Fatal("capture outcome was not observed at its boundary")
			}
			if _, err := preparecost.Summarize(report.Spans, max(report.ElapsedNS, 1)); err != nil {
				t.Fatal("connected observations violate containment", err)
			}
		})
	}
}

func TestCostRuntimeInventoryIsOncePerStartup(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "refused"}[fail], func(t *testing.T) {
			response := runResponse{output: "openjdk version \"21.0.11\"\nLiquibase Version: 5.0.4\n__SQLRS_PGJARS__\n"}
			if fail {
				response.err = errors.New("private-inventory-error")
			}
			runner := &fakeRunner{responses: []runResponse{response}}
			recorder := preparecost.NewRecorder(true, 9)
			ctx := preparecost.WithPhase(preparecost.WithRecorder(t.Context(), recorder), preparecost.PhaseStartup, 0)
			digest := "sha256:" + strings.Repeat("a", 64)
			_, err := NewDocker(Options{Runner: runner}).InspectRunnerImage(ctx, RunnerImageBinding{ManifestDigest: digest, DigestRef: "liquibase/liquibase@" + digest})
			if (err != nil) != fail || len(runner.calls) != 1 {
				t.Fatal("observing inventory changed work", err)
			}
			report := recorder.Finish()
			if !report.Complete || len(report.Spans) != 2 || report.Spans[0].Operation != preparecost.ImageInventory ||
				string(report.Spans[1].Operation) != "docker.command" || report.Spans[1].Parent != 1 || report.Spans[0].Step != 0 || report.Spans[0].Phase != preparecost.PhaseStartup {
				t.Fatal("inventory lost startup/caller/transport attribution", report)
			}
			outcome := preparecost.Succeeded
			if fail {
				outcome = preparecost.Failed
			}
			if report.Spans[0].Outcome != outcome || report.Spans[1].Outcome != outcome {
				t.Fatal("inventory refusal became an observed success")
			}
		})
	}
}
