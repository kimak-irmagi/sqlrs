package runtime

import (
	"archive/tar"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// captureFilesRequirementsRunner models a configured Docker runner with exact
// pre/post inspection and independently supplied private file archives. It is H
// evidence only. No archive is synthesized by the product from local filenames.
type captureFilesRequirementsRunner struct {
	fakeRunner
	archives  map[string][]byte
	copies    []runCall
	deadlines []time.Time
	afterCopy func()
	afterText func(int)
}

func (r *captureFilesRequirementsRunner) Run(ctx context.Context, name string, args []string, stdin *string) (string, error) {
	output, err := r.fakeRunner.Run(ctx, name, args, stdin)
	if r.afterText != nil {
		r.afterText(len(r.calls))
	}
	return output, err
}

func (r *captureFilesRequirementsRunner) RunBounded(ctx context.Context, name string, args []string, stdout, stderr int64) ([]byte, error) {
	r.copies = append(r.copies, runCall{name: name, args: append([]string(nil), args...)})
	deadline, _ := ctx.Deadline()
	r.deadlines = append(r.deadlines, deadline)
	if len(args) != 3 || args[0] != "cp" || args[2] != "-" || stdout <= 32768 || stderr != 4096 {
		return nil, errors.New("unexpected bounded capture request")
	}
	data, ok := r.archives[args[1]]
	if !ok {
		return nil, errors.New("exact capture artifact unavailable")
	}
	if r.afterCopy != nil {
		r.afterCopy()
	}
	return data, nil
}

// privateCaptureArchives is fixture construction, not product path translation.
// Links are represented as links, never followed while constructing a source.
func privateCaptureArchives(t *testing.T, dir, id, invocation string) map[string][]byte {
	t.Helper()
	root := id + ":/sqlrs/capture/"
	if invocation != "" {
		root += invocation + "/"
	}
	result := map[string][]byte{}
	for _, file := range []string{"manifest.complete", "stdout.complete", "stderr.complete"} {
		path := filepath.Join(dir, file)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		kind := byte(tar.TypeReg)
		var body []byte
		if info.Mode().IsRegular() {
			body, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			kind = tar.TypeSymlink
		} else if info.IsDir() {
			kind = tar.TypeDir
		} else {
			kind = tar.TypeFifo
		}
		result[root+file] = artifactRequirementsTar(t, file, body, kind, false)
	}
	return result
}

// TestPrivatePlanningRuntimeIgnoresHostCapture checks PC04/PC14's exclusive
// private source; host presence, absence and corrupt bytes cannot change it.
func TestPrivatePlanningRuntimeIgnoresHostCapture(t *testing.T) {
	for _, local := range []string{"present", "absent", "corrupt"} {
		t.Run(local, func(t *testing.T) {
			f := newAccessCaptureFixture(t, []byte{0, 255, 128}, nil, 0)
			id, name := strings.Repeat("a", 64), "sqlrs-step-private-plan"
			runner := &captureFilesRequirementsRunner{archives: privateCaptureArchives(t, f.dir, id, "")}
			runner.responses = append(accessInspection(id, name, "exited", 0), accessInspection(id, name, "exited", 0)...)
			if local == "absent" {
				for _, file := range []string{"manifest.complete", "stdout.complete", "stderr.complete"} {
					if err := os.Remove(filepath.Join(f.dir, file)); err != nil {
						t.Fatal(err)
					}
				}
			} else if local == "corrupt" {
				if err := os.WriteFile(filepath.Join(f.dir, "stdout.complete"), []byte("host-source-must-be-ignored"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).ReadCompleteLiquibaseOutput(context.Background(), name, id, f.dir, f.identity, 1024)
			if err != nil || string(got.Stdout) != string(f.stdout) {
				t.Fatalf("bound private capture failed with host=%s: failed=%t", local, err != nil)
			}
			if len(runner.copies) != 3 || len(runner.calls) != 4 {
				t.Fatalf("expected three copies and both exact inspections, got copies=%d text=%d", len(runner.copies), len(runner.calls))
			}
			for index, copy := range runner.copies {
				if copy.name != "configured-docker" || !strings.HasPrefix(copy.args[1], id+":/sqlrs/capture/") {
					t.Fatal("private copy bypassed exact configured source")
				}
				if runner.deadlines[index].IsZero() || runner.deadlines[index] != runner.deadlines[0] {
					t.Fatal("private files did not share a single bounded deadline")
				}
			}
		})
	}
}

// TestPrivatePlanningRuntimeRequiresPostInspection rejects a changed/missing
// parent even when every private file and manifest is otherwise valid.
func TestPrivatePlanningRuntimeRequiresPostInspection(t *testing.T) {
	for _, variant := range []string{"changed_id", "changed_state", "changed_exit", "missing_parent"} {
		t.Run(variant, func(t *testing.T) {
			f := newAccessCaptureFixture(t, nil, nil, 0)
			id, name := strings.Repeat("a", 64), "sqlrs-step-private-post"
			post := accessInspection(id, name, "exited", 0)
			switch variant {
			case "changed_id":
				post = accessInspection(strings.Repeat("b", 64), name, "exited", 0)
			case "changed_state":
				post = accessInspection(id, name, "running", 0)
			case "changed_exit":
				post = accessInspection(id, name, "exited", 7)
			case "missing_parent":
				post = []runResponse{{output: ""}}
			}
			runner := &captureFilesRequirementsRunner{archives: privateCaptureArchives(t, f.dir, id, "")}
			runner.responses = append(accessInspection(id, name, "exited", 0), post...)
			got, err := NewDocker(Options{Runner: runner}).ReadCompleteLiquibaseOutput(context.Background(), name, id, f.dir, f.identity, 1024)
			if err == nil || len(got.Stdout) != 0 || len(got.Stderr) != 0 {
				t.Fatal("changed parent accepted as complete planning capture")
			}
			if len(runner.copies) != 3 {
				t.Fatal("post-inspection fault did not reach private capture transport")
			}
		})
	}
}

// TestPrivateStepRuntimeLiveAndStopped covers PC11's separate branches. The
// child exit is from the complete manifest, not the parent's unrelated exit.
func TestPrivateStepRuntimeLiveAndStopped(t *testing.T) {
	for _, phase := range []string{"running", "exited"} {
		t.Run(phase, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			manifest["child_exit_code"] = 7
			writeStepCaptureFixture(t, dir, manifest, nil, nil)
			name := "sqlrs-lb-private-step"
			runner := &captureFilesRequirementsRunner{archives: sessionBundleFixture(t, dir, identity.RunnerID, identity.InvocationRef)}
			runner.responses = presentInspection(identity.RunnerID, name, phase, 23)
			if phase == "running" {
				runner.responses = append(runner.responses, runResponse{output: "sqlrs-step-idle-v1\n"})
			}
			runner.responses = append(runner.responses, presentInspection(identity.RunnerID, name, phase, 23)...)
			got, err := NewDocker(Options{Runner: runner}).ReadCompleteLiquibaseStepOutput(context.Background(), name, identity.RunnerID, dir, identity, 1024)
			if err != nil || got.ExitCode != 7 {
				t.Fatalf("complete child result rejected: phase=%s failed=%t exit=%d", phase, err != nil, got.ExitCode)
			}
			if len(runner.copies) != 1 {
				t.Fatal("step did not use private artifact transport")
			}
			probes := 0
			for _, call := range runner.calls {
				if len(call.args) > 0 && call.args[0] == "exec" {
					probes++
				}
			}
			want := 1
			if phase == "exited" {
				want = 0
			}
			if probes != want {
				t.Fatalf("phase=%s process probes=%d want=%d", phase, probes, want)
			}
		})
	}
}
