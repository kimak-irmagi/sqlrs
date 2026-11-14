package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// archiveCaptureRunner supplies archive bytes independently from the verifier.
// The fake does not parse, sanitize or repair them. Real acquisition caps and
// process cleanup are separately tested using real subprocesses in CA07–09.
type archiveCaptureRunner struct {
	*fakeRunner
	archives    map[string][]byte
	binaryCalls []runCall
	copyErr     error
}

func (r *archiveCaptureRunner) RunBounded(_ context.Context, name string, args []string, _, _ int64) ([]byte, error) {
	r.binaryCalls = append(r.binaryCalls, runCall{name: name, args: append([]string{}, args...)})
	if len(args) != 3 || args[0] != "cp" || args[2] != "-" {
		return nil, fmt.Errorf("unexpected copy invocation")
	}
	data, ok := r.archives[args[1]]
	if !ok {
		return nil, fmt.Errorf("unbound capture address")
	}
	return data, r.copyErr
}

// makeAccessTar uses the standard writer only to create fixtures, never as the
// product acceptance oracle. Malformed variants below mutate the wire bytes.
func makeAccessTar(t *testing.T, name string, data []byte, kind byte, extra bool) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := tar.NewWriter(&buffer)
	h := &tar.Header{Name: name, Typeflag: kind, Mode: 0600, Size: int64(len(data))}
	if kind != tar.TypeReg {
		h.Size = 0
		h.Linkname = "other"
	}
	if err := w.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if extra {
		if err := w.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: 0, Mode: 0600}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// CA06/08, OT-PC02: private/v2 always uses the exact container source. Every
// archive rejection must reach bounded transport; host EACCES is not a trigger
// or evidence. These H fixtures do not qualify native Linux permission isolation.
func TestCaptureAccessArchivePermissionBoundary(t *testing.T) {
	for _, kind := range []string{"manifest_private", "stdout_private", "stderr_private",
		"wrong_name", "absolute_name", "traversal_name", "duplicate", "symlink", "hardlink", "directory", "fifo", "device",
		"bad_header", "truncated_header", "truncated_payload", "nonzero_tail", "wire_overflow", "copy_failure"} {
		t.Run(kind, func(t *testing.T) {
			f := newAccessCaptureFixture(t, []byte{0, 255, 128, 10}, []byte("private-diagnostic-marker"), 0)
			id, name := strings.Repeat("a", 64), "sqlrs-step-capture-archive"
			manifest, err := json.Marshal(f.manifest)
			if err != nil {
				t.Fatal(err)
			}
			contents := map[string][]byte{"manifest.complete": manifest, "stdout.complete": f.stdout, "stderr.complete": f.stderr}
			denied := "stdout.complete"
			if kind == "manifest_private" {
				denied = "manifest.complete"
			}
			if kind == "stderr_private" {
				denied = "stderr.complete"
			}
			responses := append(accessInspection(id, name, "exited", 0), accessInspection(id, name, "exited", 0)...)
			runner := &archiveCaptureRunner{fakeRunner: &fakeRunner{responses: responses}, archives: make(map[string][]byte)}
			for file, data := range contents {
				runner.archives[id+":/sqlrs/capture/"+file] = makeAccessTar(t, file, data, tar.TypeReg, false)
			}
			address := id + ":/sqlrs/capture/" + denied
			switch kind {
			case "wrong_name":
				runner.archives[address] = makeAccessTar(t, "other.complete", f.stdout, tar.TypeReg, false)
			case "absolute_name":
				runner.archives[address] = makeAccessTar(t, "/stdout.complete", f.stdout, tar.TypeReg, false)
			case "traversal_name":
				runner.archives[address] = makeAccessTar(t, "../stdout.complete", f.stdout, tar.TypeReg, false)
			case "duplicate":
				runner.archives[address] = makeAccessTar(t, denied, f.stdout, tar.TypeReg, true)
			case "symlink":
				runner.archives[address] = makeAccessTar(t, denied, nil, tar.TypeSymlink, false)
			case "hardlink":
				runner.archives[address] = makeAccessTar(t, denied, nil, tar.TypeLink, false)
			case "directory":
				runner.archives[address] = makeAccessTar(t, denied, nil, tar.TypeDir, false)
			case "fifo":
				runner.archives[address] = makeAccessTar(t, denied, nil, tar.TypeFifo, false)
			case "device":
				runner.archives[address] = makeAccessTar(t, denied, nil, tar.TypeChar, false)
			case "bad_header":
				runner.archives[address][148] ^= 1
			case "truncated_header":
				runner.archives[address] = runner.archives[address][:200]
			case "truncated_payload":
				runner.archives[address] = runner.archives[address][:513]
			case "nonzero_tail":
				runner.archives[address] = append(runner.archives[address], 1)
			case "wire_overflow":
				runner.archives[address] = append(runner.archives[address], make([]byte, 1024+32769)...)
			case "copy_failure":
				runner.copyErr = fmt.Errorf("copy failed")
				address = id + ":/sqlrs/capture/manifest.complete"
			}
			got, err := NewDocker(Options{Binary: "configured-docker", Runner: runner}).ReadCompleteLiquibaseOutput(context.Background(), name, id, f.dir, f.identity, 1024)
			if len(runner.binaryCalls) == 0 {
				t.Fatalf("private transport not reached; archive case unqualified: %v", err)
			}
			valid := strings.HasSuffix(kind, "_private")
			wantCopies := 2
			if valid {
				wantCopies = 3
			}
			if kind == "copy_failure" {
				wantCopies = 1
			}
			if len(runner.binaryCalls) != wantCopies {
				t.Fatalf("private source copies=%d want=%d", len(runner.binaryCalls), wantCopies)
			}
			for index, call := range runner.binaryCalls {
				file := []string{"manifest.complete", "stdout.complete", "stderr.complete"}[index]
				if call.name != "configured-docker" || !reflect.DeepEqual(call.args, []string{"cp", id + ":/sqlrs/capture/" + file, "-"}) {
					t.Fatal("copy broadened path, order or runner authority")
				}
			}
			if !valid && !reflect.DeepEqual(runner.binaryCalls[len(runner.binaryCalls)-1].args, []string{"cp", address, "-"}) {
				t.Fatal("fault did not reach its exact artifact")
			}
			if valid {
				if err != nil || !bytes.Equal(got.Stdout, f.stdout) || !bytes.Equal(got.Stderr, f.stderr) {
					t.Fatal("private capture failed or changed bytes")
				}
			} else if err == nil || !reflect.DeepEqual(got, CompleteCapture{}) {
				t.Fatal("invalid archive produced a capture")
			}
		})
	}
}
