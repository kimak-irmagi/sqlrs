package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDockerInspectNamedContainerDistinguishesAbsentAndCreated(t *testing.T) {
	id := strings.Repeat("a", 64)
	name := "sqlrs-lb-test"
	runner := &fakeRunner{responses: []runResponse{
		{output: ""},
		{output: id + "\n"},
		{output: `{"Id":"` + id + `","Name":"/` + name + `","State":{"Status":"created"}}`},
	}}
	rt := NewDocker(Options{Binary: "docker", Runner: runner})
	if _, found, err := rt.InspectNamedContainer(context.Background(), name); err != nil || found {
		t.Fatalf("missing container found=%t err=%v", found, err)
	}
	inspection, found, err := rt.InspectNamedContainer(context.Background(), name)
	if err != nil || !found || inspection.ID != id || inspection.Status != "created" {
		t.Fatalf("created inspection=%+v found=%t err=%v", inspection, found, err)
	}
	if len(runner.calls) != 3 || runner.calls[0].args[0] != "container" || runner.calls[2].args[0] != "container" || runner.calls[2].args[1] != "inspect" {
		t.Fatalf("unexpected Docker calls=%+v", runner.calls)
	}
}

func TestDockerInspectNamedContainerRejectsMismatchedIdentity(t *testing.T) {
	id := strings.Repeat("a", 64)
	runner := &fakeRunner{responses: []runResponse{{output: id + "\n"}, {output: `{"Id":"` + id + `","Name":"/different","State":{"Status":"created"}}`}}}
	rt := NewDocker(Options{Binary: "docker", Runner: runner})
	if _, _, err := rt.InspectNamedContainer(context.Background(), "sqlrs-lb-test"); err == nil {
		t.Fatal("accepted inspection of a different container")
	}
}

// I02/A02: the same exact-name inspection works for the new shared attempt
// namespace, without accepting arbitrary Docker names from a request.
func TestDockerInspectNamedSharedAttemptContainer(t *testing.T) {
	id := strings.Repeat("b", 64)
	name := "sqlrs-step-psql-1"
	runner := &fakeRunner{responses: []runResponse{
		{output: id + "\n"},
		{output: `{"Id":"` + id + `","Name":"/` + name + `","State":{"Status":"running"}}`},
	}}
	rt := NewDocker(Options{Binary: "docker", Runner: runner})
	inspection, found, err := rt.InspectNamedContainer(context.Background(), name)
	if err != nil || !found || inspection.ID != id || inspection.Status != "running" {
		t.Fatalf("shared attempt inspection=%+v found=%t err=%v", inspection, found, err)
	}
	if _, _, err := rt.InspectNamedContainer(context.Background(), "sqlrs-user-container"); err == nil {
		t.Fatal("arbitrary name was accepted")
	}
	if len(runner.calls) != 2 {
		t.Fatalf("invalid name reached Docker: calls=%v", runner.calls)
	}
}

func TestDockerContainerAttemptUsesCreateStartWaitRemove(t *testing.T) {
	id := strings.Repeat("a", 64)
	runner := &fakeRunner{responses: []runResponse{{output: id + "\n"}, {output: id + "\n"}, {output: "0\n"}, {output: "Liquibase done\n"}, {}}}
	rt := NewDocker(Options{Binary: "docker", Runner: runner})
	req := RunRequest{ImageID: "liquibase/liquibase@sha256:abc", Name: "sqlrs-lb-attempt", Network: "container:postgres", Dir: "/sqlrs/mnt/path1", Mounts: []Mount{{HostPath: "/host/source", ContainerPath: "/sqlrs/mnt/path1", ReadOnly: true}}, Args: []string{"update"}}
	created, err := rt.CreateContainer(context.Background(), req)
	if err != nil || created != id {
		t.Fatalf("create id=%q err=%v", created, err)
	}
	if len(runner.calls) != 1 || runner.calls[0].args[0] != "create" || !containsArg(runner.calls[0].args, "--name", req.Name) || !containsArg(runner.calls[0].args, "-v", "/host/source:/sqlrs/mnt/path1:ro") {
		t.Fatalf("create args=%+v", runner.calls)
	}
	if err := rt.StartContainer(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	logs, code, err := rt.WaitContainer(context.Background(), id)
	if err != nil || code != 0 || logs != "Liquibase done\n" {
		t.Fatalf("wait logs=%q code=%d err=%v", logs, code, err)
	}
	if err := rt.RemoveStoppedContainer(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	for i, verb := range []string{"create", "start", "wait", "logs", "rm"} {
		if runner.calls[i].args[0] != verb {
			t.Fatalf("call %d=%v, want %s", i, runner.calls[i].args, verb)
		}
	}
}

// C01a: the pinned helper must replace the image entrypoint, while the
// helper process can start its Liquibase child under a different identity.
func TestDockerCreatePlanningHelperEntrypoint(t *testing.T) {
	id := strings.Repeat("c", 64)
	runner := &fakeRunner{responses: []runResponse{{output: id + "\n"}}}
	rt := NewDocker(Options{Binary: "docker", Runner: runner})
	request := RunRequest{ImageID: "liquibase/liquibase@sha256:" + strings.Repeat("a", 64), Name: "sqlrs-lb-plan-test", User: "0", EntryPoint: "/bin/sh", Args: []string{"/sqlrs/helper/capture.sh", "/bin/sh", "-c", "true"}}
	if _, err := rt.CreateContainer(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	args := runner.calls[0].args
	if !containsArg(args, "--entrypoint", "/bin/sh") || !containsArg(args, "-u", "0") {
		t.Fatalf("helper identity/entrypoint missing: %v", args)
	}
}

// A03: losing diagnostic logs after docker wait must not discard the proved
// exit code needed by the common attempt journal.
func TestDockerWaitKeepsExitProofWhenLogsFail(t *testing.T) {
	id := strings.Repeat("a", 64)
	runner := &fakeRunner{responses: []runResponse{
		{output: "0\n"}, {err: errors.New("logs unavailable")},
	}}
	rt := NewDocker(Options{Binary: "docker", Runner: runner})
	logs, code, err := rt.WaitContainer(context.Background(), id)
	if err != nil || code != 0 || logs != "" {
		t.Fatalf("lost proved exit: logs=%q code=%d err=%v", logs, code, err)
	}
}

func TestDockerContainerAttemptRejectsUnknownOutcome(t *testing.T) {
	rt := NewDocker(Options{Binary: "docker", Runner: &fakeRunner{responses: []runResponse{{output: "not-an-id"}}}})
	if _, err := rt.CreateContainer(context.Background(), RunRequest{ImageID: "image", Name: "planned"}); err == nil {
		t.Fatal("invalid create ID accepted")
	}
	if err := rt.StartContainer(context.Background(), "bad-id"); err == nil {
		t.Fatal("invalid start ID accepted")
	}
}

// R04e/R04f: signalling uses only the saved exact Docker ID. A lost response
// is an error and cannot be mistaken for an acknowledgement.
func TestDockerSignalContainerRequiresExactAcknowledgement(t *testing.T) {
	id := strings.Repeat("a", 64)
	runner := &fakeRunner{responses: []runResponse{{output: id + "\n"}, {output: "other\n"}, {err: errors.New("lost response")}}}
	rt := NewDocker(Options{Binary: "docker", Runner: runner})
	if err := rt.SignalContainer(context.Background(), "invalid"); err == nil || len(runner.calls) != 0 {
		t.Fatal("invalid ID reached Docker")
	}
	if err := rt.SignalContainer(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if !containsArg(runner.calls[0].args, "--signal=SIGKILL", id) {
		t.Fatalf("signal did not target exact ID: %v", runner.calls[0].args)
	}
	if err := rt.SignalContainer(context.Background(), id); err == nil {
		t.Fatal("mismatched Docker response was acknowledged")
	}
	if err := rt.SignalContainer(context.Background(), id); err == nil {
		t.Fatal("lost Docker response was acknowledged")
	}
}
