package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// CT02/O1: a positive observation already has a saved full ID. Listing by name
// must not replace or precede inspecting it; discovery/absence keep their own API.
// Requirements: docs/architecture/issue89-cost-attribution-and-trimming.md §2.
func TestCostTrimInspectPresentContainer(t *testing.T) {
	id, name := strings.Repeat("a", 64), "sqlrs-lb-session-cost"
	for _, phase := range []string{"running", "exited"} {
		t.Run(phase, func(t *testing.T) {
			view := ContainerInspection{ID: id, Status: phase, ExitCode: 7, SessionID: "session",
				RunnerConfigDigest: "config", WorkerGeneration: "3", CaptureHelperDigest: "helper"}
			runner := &fakeRunner{responses: []runResponse{{output: costTrimInspectionJSON(id, name, phase)}}}
			got, err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).inspectPresentContainer(t.Context(), name, id)
			if err != nil || got != view {
				t.Fatalf("exact present observation=%+v err=%v", got, err)
			}
			want := []runCall{{name: "configured-docker", args: []string{"container", "inspect", "--format", "{{json .}}", id}}}
			if !reflect.DeepEqual(runner.calls, want) {
				t.Fatalf("expected one full-ID inspect without list, got %+v", runner.calls)
			}
		})
	}
}

func TestCostTrimInspectPresentContainerRefusals(t *testing.T) {
	id, name := strings.Repeat("a", 64), "sqlrs-lb-session-cost"
	for _, variant := range []string{"empty-id", "short-id", "invalid-id", "invalid-name", "foreign-id", "renamed",
		"missing-phase", "malformed-json", "truncated-json", "wrong-label-type", "absent", "daemon-error",
		"cancelled-before", "cancelled-after", "nil-runtime"} {
		t.Run(variant, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			requestedID, requestedName := id, name
			response := runResponse{output: costTrimInspectionJSON(id, name, "running")}
			before := false
			switch variant {
			case "empty-id":
				requestedID, before = "", true
			case "short-id":
				requestedID, before = id[:12], true
			case "invalid-id":
				requestedID, before = strings.Repeat("g", 64), true
			case "invalid-name":
				requestedName, before = "user-container", true
			case "foreign-id":
				response.output = costTrimInspectionJSON(strings.Repeat("b", 64), name, "running")
			case "renamed":
				response.output = costTrimInspectionJSON(id, name+"-renamed", "running")
			case "missing-phase":
				response.output = costTrimInspectionJSON(id, name, "")
			case "malformed-json":
				response.output = "not-json"
			case "truncated-json":
				response.output = response.output[:len(response.output)-1]
			case "wrong-label-type":
				response.output = `{"Id":"` + id + `","Name":"/` + name + `","State":{"Status":"running"},"Config":{"Labels":12}}`
			case "absent", "daemon-error":
				response.err = errors.New(variant)
			case "cancelled-before":
				cancel()
				before = true
			case "nil-runtime":
				before = true
			}
			runner := &costTrimInspectRunner{fakeRunner: fakeRunner{responses: []runResponse{response}}}
			if variant == "cancelled-after" {
				runner.after = cancel
			}
			rt := NewDocker(Options{Runner: runner})
			if variant == "nil-runtime" {
				rt = nil
			}
			got, err := rt.inspectPresentContainer(ctx, requestedName, requestedID)
			if err == nil || got != (ContainerInspection{}) {
				t.Fatalf("unproved positive observation returned %+v err=%v", got, err)
			}
			wantCalls := 1
			if before {
				wantCalls = 0
			}
			if len(runner.calls) != wantCalls {
				t.Fatalf("refusal boundary calls=%d want=%d", len(runner.calls), wantCalls)
			}
			if strings.HasPrefix(variant, "cancelled-") && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled observation returned %v", err)
			}
			for _, call := range runner.calls {
				if !reflect.DeepEqual(call.args, []string{"container", "inspect", "--format", "{{json .}}", id}) {
					t.Fatalf("failed inspect acquired discovery/mutation authority: %+v", call)
				}
			}
		})
	}
}

type costTrimInspectRunner struct {
	fakeRunner
	after func()
}

func (r *costTrimInspectRunner) Run(ctx context.Context, name string, args []string, stdin *string) (string, error) {
	output, err := r.fakeRunner.Run(ctx, name, args, stdin)
	if r.after != nil {
		r.after()
	}
	return output, err
}

func costTrimInspectionJSON(id, name, phase string) string {
	data, _ := json.Marshal(map[string]any{"Id": id, "Name": "/" + name,
		"State": map[string]any{"Status": phase, "ExitCode": 7},
		"Config": map[string]any{"Labels": map[string]string{"sqlrs.liquibase.session": "session",
			"sqlrs.liquibase.config": "config", "sqlrs.liquibase.generation": "3", "sqlrs.liquibase.helper": "helper"}}})
	return string(data)
}
