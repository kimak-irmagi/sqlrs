package runtime

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestCaptureArtifactDockerPrivateLayer qualifies CA06/13's actual archive
// compatibility and private-layer acquisition, not the unfinished planner/session
// protocol or recovery coordinator. Its producer is an owned root-only fixture,
// not Liquibase; no database, SQL, host capture mount or shared volume is used.
func TestCaptureArtifactDockerPrivateLayer(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("Docker suite not selected; no native archive qualification")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		t.Fatal("native artifact qualification requires an ordinary Linux engine UID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	if err != nil {
		t.Fatal("required native Liquibase image unavailable")
	}
	image := strings.TrimSpace(string(imageBytes))
	if !validCaptureDigest(image) {
		t.Fatal("native image identity unavailable")
	}
	t.Logf("native artifact transport OS=%s engine_uid=%d image=%s", runtime.GOOS, os.Geteuid(), image)
	command := `set -eu
umask 077
mkdir -p /sqlrs/capture/owned-invocation
printf '\000\377\200\012' > /sqlrs/capture/stdout.complete
printf '\000\377\200\012' > /sqlrs/capture/owned-invocation/stderr.complete
exec sleep 120`
	_, runErr, name, id := runRetainedCaptureTestContainer(t, ctx, []string{"run", "-d", "--network", "none", "--user", "0", "--entrypoint", "/bin/sh", image, "-c", command})
	if runErr != nil || !createdContainerID.MatchString(id) {
		t.Fatal("owned private artifact producer failed to start")
	}
	ready, readyCancel := context.WithTimeout(ctx, 20*time.Second)
	defer readyCancel()
	for {
		err := exec.CommandContext(ready, "docker", "exec", "--user", "0", id, "/bin/sh", "-c", "test -f /sqlrs/capture/owned-invocation/stderr.complete").Run()
		if err == nil {
			break
		}
		if ready.Err() != nil {
			t.Fatal("private artifact producer readiness not reached")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// A real existing file is unreadable to the same UID as the untrusted
	// Liquibase child; host-side permission repair is neither needed nor used.
	if err := exec.CommandContext(ctx, "docker", "exec", "--user", "1001", id, "/bin/sh", "-c", "test ! -r /sqlrs/capture/stdout.complete").Run(); err != nil {
		t.Fatal("untrusted child can read private fixture output")
	}
	rt := NewDocker(Options{Binary: "docker"})
	for _, phase := range []string{"running", "exited"} {
		if phase == "exited" {
			if err := exec.CommandContext(ctx, "docker", "stop", "--time", "1", id).Run(); err != nil {
				t.Fatal("exact producer stop failed")
			}
		}
		inspection, found, err := rt.InspectNamedContainer(ctx, name)
		if err != nil || !found || inspection.ID != id || inspection.Status != phase {
			t.Fatalf("producer phase not reached: expected=%s found=%t err=%v", phase, found, err)
		}
		readCtx, readCancel := context.WithTimeout(ctx, 30*time.Second)
		for _, artifact := range []struct{ invocation, file string }{{"", "stdout.complete"}, {"owned-invocation", "stderr.complete"}} {
			data, err := rt.readCaptureArtifact(readCtx, id, artifact.invocation, artifact.file, 1024)
			if err != nil || !bytes.Equal(data, []byte{0, 255, 128, '\n'}) {
				readCancel()
				t.Fatalf("native private artifact differs: phase=%s file=%s failed=%t", phase, artifact.file, err != nil)
			}
		}
		readCancel()
		after, found, err := rt.InspectNamedContainer(ctx, name)
		if err != nil || !found || after.ID != id || after.Status != phase {
			t.Fatal("artifact copy changed or replaced its exact parent")
		}
	}
}
