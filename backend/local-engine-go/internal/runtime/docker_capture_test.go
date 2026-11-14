package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// C04: a stopped exact executor and complete private manifest are both
// necessary; Docker logs are never queried for plan data.
func TestDockerReadCompleteLiquibaseOutputUsesExactInspection(t *testing.T) {
	id := strings.Repeat("a", 64)
	name := "sqlrs-step-lb-plan-1"
	identity := CaptureIdentity{JobID: "job", AttemptNo: 1, ExecutionDigest: "sha256:" + strings.Repeat("b", 64), HelperDigest: "sha256:" + strings.Repeat("c", 64), ExitCode: 0}
	dir := t.TempDir()
	manifest := liquibaseCaptureManifest{Protocol: "lb-capture-v2", JobID: "job", TaskID: "plan", AttemptNo: 1, ExecutionDigest: identity.ExecutionDigest, HelperDigest: identity.HelperDigest, ChildExitCode: 0, StdoutEOF: true, StderrEOF: true, StdoutSHA256: captureHex(nil), StderrSHA256: captureHex(nil)}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for file, content := range map[string][]byte{"manifest.complete": data, "stdout.complete": nil, "stderr.complete": nil} {
		if err := os.WriteFile(filepath.Join(dir, file), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	makeRuntime := func(status string, exitCode int) *DockerRuntime {
		response := `{"Id":"` + id + `","Name":"/` + name + `","State":{"Status":"` + status + `","ExitCode":` + strconv.Itoa(exitCode) + `}}`
		runner := &captureFilesRequirementsRunner{archives: privateCaptureArchives(t, dir, id, "")}
		runner.responses = []runResponse{{output: id + "\n"}, {output: response}, {output: id + "\n"}, {output: response}}
		return NewDocker(Options{Binary: "docker", Runner: runner})
	}
	rt := makeRuntime("exited", 0)
	if result, err := rt.ReadCompleteLiquibaseOutput(context.Background(), name, id, dir, identity, 1024); err != nil || result.StdoutDigest != captureDigest(nil) {
		t.Fatalf("capture=%+v err=%v", result, err)
	}
	if _, err := makeRuntime("running", 0).ReadCompleteLiquibaseOutput(context.Background(), name, id, dir, identity, 1024); err == nil {
		t.Fatal("running executor accepted")
	}
	if _, err := makeRuntime("exited", 1).ReadCompleteLiquibaseOutput(context.Background(), name, id, dir, identity, 1024); err == nil {
		t.Fatal("wrong exit accepted")
	}
	if _, err := makeRuntime("exited", 0).ReadCompleteLiquibaseOutput(context.Background(), name, strings.Repeat("d", 64), dir, identity, 1024); err == nil {
		t.Fatal("wrong executor ID accepted")
	}
}
