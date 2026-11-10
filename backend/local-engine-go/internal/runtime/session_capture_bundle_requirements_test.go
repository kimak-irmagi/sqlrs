package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type costTrimTarEntry struct {
	name string
	body []byte
	kind byte
}

// CT04: exact individual bounds, wire framing, metadata and the shared verifier
// interface remain independently bounded after changing the acquisition unit.
func TestCostTrimSessionBundleLimitsAndMetadata(t *testing.T) {
	entries := []costTrimTarEntry{{"manifest.complete", bytes.Repeat([]byte{'m'}, 8192), tar.TypeReg},
		{"stdout.complete", []byte("1234"), tar.TypeReg}, {"stderr.complete", []byte("4321"), tar.TypeReg}}
	wire := costTrimBundleTar(t, entries)
	bundle, err := decodeSessionCaptureArchive(wire, 4)
	if err != nil || len(bundle["manifest.complete"]) != 8192 {
		t.Fatal("exact per-file limits rejected", err)
	}
	for _, file := range []string{"manifest.complete", "stdout.complete", "stderr.complete", "foreign"} {
		limit := int64(4)
		if file == "manifest.complete" {
			limit = 8192
		}
		data, err := bundle.readCapture(file, limit)
		if file == "foreign" {
			if err == nil || data != nil {
				t.Fatal("unknown fixed file returned bytes")
			}
		} else if err != nil || int64(len(data)) != limit {
			t.Fatal("exact bounded reader failed", err)
		}
	}
	for _, limit := range []int64{0, -1, 3} {
		if data, err := bundle.readCapture("stdout.complete", limit); err == nil || data != nil {
			t.Fatal("invalid reader limit returned bytes")
		}
	}
	for _, limit := range []int64{0, -1, captureStreamLimit + 1} {
		if data, err := decodeSessionCaptureArchive(wire, limit); err == nil || data != nil {
			t.Fatal("invalid decoder limit returned bytes")
		}
	}
	base := costTrimBundleTar(t, []costTrimTarEntry{{"stdout.complete", nil, tar.TypeReg}, {"stderr.complete", nil, tar.TypeReg}, {"manifest.complete", nil, tar.TypeReg}})
	for _, variant := range []string{"empty", "empty-eof", "linkname", "global-pax", "gnu-longname", "metadata-size", "metadata-padding", "metadata-checksum", "orphan-metadata", "directory-size", "metadata-budget"} {
		t.Run(variant, func(t *testing.T) {
			data := append([]byte(nil), base...)
			limit := int64(1024)
			switch variant {
			case "empty":
				data = nil
			case "empty-eof":
				data = make([]byte, 1024)
			case "linkname":
				copy(data[157:257], "foreign")
				artifactRequirementsChecksum(data)
			case "global-pax":
				data[156] = tar.TypeXGlobalHeader
				artifactRequirementsChecksum(data)
			case "gnu-longname":
				data[156] = tar.TypeGNULongName
				artifactRequirementsChecksum(data)
			case "metadata-size":
				data = artifactRequirementsPAX(t, "mtime", "123", data)
				copy(data[124:136], "77777777777\x00")
				artifactRequirementsChecksum(data)
			case "metadata-padding":
				data = artifactRequirementsPAX(t, "mtime", "123", data)
				data[1023] = 1
			case "metadata-checksum":
				data = artifactRequirementsPAX(t, "mtime", "123", data)
				data[148] ^= 1
			case "orphan-metadata":
				data = append(data[:len(data)-1024], artifactRequirementsPAX(t, "mtime", "123", make([]byte, 1024))...)
			case "directory-size":
				data = costTrimBundleTar(t, append([]costTrimTarEntry{{".", nil, tar.TypeDir}}, []costTrimTarEntry{{"stdout.complete", nil, tar.TypeReg}, {"stderr.complete", nil, tar.TypeReg}, {"manifest.complete", nil, tar.TypeReg}}...))
				copy(data[124:136], "00000000001\x00")
				artifactRequirementsChecksum(data)
			case "metadata-budget":
				limit = 65536
				for range 5 {
					data = artifactRequirementsPAX(t, "comment", strings.Repeat("x", 8192), data)
				}
			}
			if files, err := decodeSessionCaptureArchive(data, limit); err == nil || files != nil {
				t.Fatal("invalid metadata/framing returned files")
			}
		})
	}
}

func costTrimBundleTar(t *testing.T, entries []costTrimTarEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := tar.NewWriter(&buffer)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0600, Typeflag: entry.kind, Size: int64(len(entry.body))}
		if entry.kind != tar.TypeReg && entry.kind != tar.TypeRegA {
			header.Size = 0
		}
		if entry.kind == tar.TypeSymlink || entry.kind == tar.TypeLink {
			header.Linkname = "foreign"
		}
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := w.Write(entry.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// CT04/O3: the three-file transport has a closed grammar and independent byte
// expectations. Fixtures are ordinary tar, never product-generated archives.
// See docs/architecture/issue89-cost-attribution-and-trimming.md §2.
func TestCostTrimSessionBundleArchive(t *testing.T) {
	for _, variant := range []string{"plain", "directory", "dot-prefix", "reverse", "empty-streams", "safe-pax",
		"missing", "duplicate", "extra", "absolute", "traversal", "nested", "symlink", "hardlink", "fifo",
		"directory-as-file", "foreign-directory", "duplicate-directory", "late-directory", "checksum", "short-header",
		"short-payload", "missing-eof", "nonzero-padding", "nonzero-tail", "manifest-cap", "stdout-cap", "stderr-cap",
		"huge-size", "binary-size", "path-pax", "size-pax", "shadowed-pax"} {
		t.Run(variant, func(t *testing.T) {
			entries := []costTrimTarEntry{{"stdout.complete", []byte{0, 255, 128, 10}, tar.TypeReg},
				{"stderr.complete", []byte("private-error-marker"), tar.TypeReg},
				{"manifest.complete", []byte("private-manifest-marker"), tar.TypeReg}}
			positive := variant == "plain" || variant == "directory" || variant == "dot-prefix" || variant == "reverse" || variant == "empty-streams" || variant == "safe-pax"
			switch variant {
			case "directory":
				entries = append([]costTrimTarEntry{{".", nil, tar.TypeDir}}, entries...)
			case "dot-prefix":
				for i := range entries {
					entries[i].name = "./" + entries[i].name
				}
				entries = append([]costTrimTarEntry{{"./", nil, tar.TypeDir}}, entries...)
			case "reverse":
				entries[0], entries[2] = entries[2], entries[0]
			case "empty-streams":
				entries[0].body, entries[1].body = nil, nil
			case "missing":
				entries = entries[:2]
			case "duplicate":
				entries = append(entries, entries[0])
			case "extra":
				entries = append(entries, costTrimTarEntry{"private-secret-marker", nil, tar.TypeReg})
			case "absolute":
				entries[0].name = "/stdout.complete"
			case "traversal":
				entries[0].name = "../stdout.complete"
			case "nested":
				entries[0].name = "foreign/stdout.complete"
			case "symlink":
				entries[0].kind = tar.TypeSymlink
			case "hardlink":
				entries[0].kind = tar.TypeLink
			case "fifo":
				entries[0].kind = tar.TypeFifo
			case "directory-as-file":
				entries[0].kind = tar.TypeDir
			case "foreign-directory":
				entries = append([]costTrimTarEntry{{"foreign/", nil, tar.TypeDir}}, entries...)
			case "duplicate-directory":
				entries = append([]costTrimTarEntry{{".", nil, tar.TypeDir}, {"./", nil, tar.TypeDir}}, entries...)
			case "late-directory":
				entries = append(entries, costTrimTarEntry{".", nil, tar.TypeDir})
			case "manifest-cap":
				entries[2].body = bytes.Repeat([]byte{'x'}, 8193)
			case "stdout-cap":
				entries[0].body = bytes.Repeat([]byte{'x'}, 1025)
			case "stderr-cap":
				entries[1].body = bytes.Repeat([]byte{'x'}, 1025)
			}
			wire := costTrimBundleTar(t, entries)
			switch variant {
			case "checksum":
				wire[148] ^= 1
			case "short-header":
				wire = wire[:511]
			case "short-payload":
				wire = wire[:513]
			case "missing-eof":
				wire = wire[:len(wire)-512]
			case "nonzero-padding":
				wire[516] = 1
			case "nonzero-tail":
				wire = append(wire, 1)
			case "huge-size":
				copy(wire[124:136], "77777777777\x00")
				artifactRequirementsChecksum(wire)
			case "binary-size":
				wire[124] = 128
				artifactRequirementsChecksum(wire)
			case "safe-pax":
				wire = artifactRequirementsPAX(t, "mtime", "123.25", wire)
			case "path-pax":
				wire = artifactRequirementsPAX(t, "path", "stdout.complete", wire)
			case "size-pax":
				wire = artifactRequirementsPAX(t, "size", "4", wire)
			case "shadowed-pax":
				wire = artifactRequirementsPAX(t, "path", "foreign", artifactRequirementsPAX(t, "mtime", "123", wire))
			}
			got, err := decodeSessionCaptureArchive(wire, 1024)
			if !positive {
				if err == nil || len(got) != 0 {
					t.Fatal("invalid bundle returned partial/complete files", variant)
				}
				if strings.Contains(err.Error(), "private-") {
					t.Fatal("private archive name/contents leaked")
				}
				return
			}
			if err != nil || len(got) != 3 {
				t.Fatalf("valid bundle rejected: files=%d err=%v", len(got), err)
			}
			for _, entry := range entries {
				if entry.kind != tar.TypeReg {
					continue
				}
				if !bytes.Equal(got[strings.TrimPrefix(entry.name, "./")], entry.body) {
					t.Fatal("bundle changed independent bytes")
				}
			}
		})
	}
}

func TestCostTrimSessionBundleTransport(t *testing.T) {
	id, invocation := strings.Repeat("a", 64), "invocation-cost"
	wire := costTrimBundleTar(t, []costTrimTarEntry{{"manifest.complete", nil, tar.TypeReg}, {"stdout.complete", nil, tar.TypeReg}, {"stderr.complete", nil, tar.TypeReg}})
	for _, variant := range []string{"control", "invalid-id", "invalid-invocation", "empty-invocation", "zero-limit", "large-limit",
		"no-deadline", "cancel-before", "cancel-copy", "copy-error", "wire-cap", "no-bounded-runner", "nil-runtime"} {
		t.Run(variant, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			deadline, _ := ctx.Deadline()
			runner := &artifactRequirementsRunner{archive: wire}
			rt := NewDocker(Options{Binary: "configured-docker", Runner: runner})
			requestedID, requestedInvocation, limit := id, invocation, int64(1024)
			wantCalls := 1
			switch variant {
			case "invalid-id":
				requestedID, wantCalls = "short", 0
			case "invalid-invocation":
				requestedInvocation, wantCalls = "../foreign", 0
			case "empty-invocation":
				requestedInvocation, wantCalls = "", 0
			case "zero-limit":
				limit, wantCalls = 0, 0
			case "large-limit":
				limit, wantCalls = captureStreamLimit+1, 0
			case "no-deadline":
				ctx, wantCalls = t.Context(), 0
			case "cancel-before":
				cancel()
				wantCalls = 0
			case "cancel-copy":
				runner.onCopy = cancel
			case "copy-error":
				runner.err = errors.New("private-copy-error")
			case "wire-cap":
				runner.archive = make([]byte, 2*limit+8192+captureArchiveAllowance+1)
			case "no-bounded-runner":
				rt, wantCalls = NewDocker(Options{Runner: &fakeRunner{}}), 0
			case "nil-runtime":
				rt, wantCalls = nil, 0
			}
			got, err := rt.readSessionCaptureBundle(ctx, requestedID, requestedInvocation, limit)
			if runner.calls != wantCalls || len(runner.fakeRunner.calls) != 0 {
				t.Fatalf("transport boundary=%d want=%d text=%d", runner.calls, wantCalls, len(runner.fakeRunner.calls))
			}
			if wantCalls == 1 && (!reflect.DeepEqual(runner.args, []string{"cp", id + ":/sqlrs/capture/" + invocation + "/.", "-"}) ||
				runner.name != "configured-docker" || runner.stdoutCap != 2*limit+8192+captureArchiveAllowance ||
				runner.stderrCap != 4096 || !runner.deadline.Equal(deadline)) {
				t.Fatal("bundle source, quota or shared deadline changed")
			}
			if variant == "control" {
				if err != nil || len(got) != 3 {
					t.Fatal("valid one-copy bundle failed", err)
				}
			} else if err == nil || len(got) != 0 || strings.Contains(err.Error(), "private-copy-error") {
				t.Fatal("failed acquisition exposed files/private diagnostics", err)
			}
		})
	}
}

func TestCostTrimSessionBundleRetainsManifestChecks(t *testing.T) {
	for _, variant := range []string{"control", "wrong-session", "stream-mismatch"} {
		t.Run(variant, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			if variant == "wrong-session" {
				manifest["session_id"] = "foreign"
				writeStepCaptureFixture(t, dir, manifest, nil, nil)
			}
			if variant == "stream-mismatch" {
				if err := os.WriteFile(filepath.Join(dir, "stdout.complete"), []byte("private-bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var entries []costTrimTarEntry
			for _, name := range []string{"manifest.complete", "stdout.complete", "stderr.complete"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				entries = append(entries, costTrimTarEntry{name, data, tar.TypeReg})
			}
			bundle, err := decodeSessionCaptureArchive(costTrimBundleTar(t, entries), 1024)
			if err != nil {
				t.Fatal("semantic case did not reach manifest verifier", err)
			}
			capture, err := readCompleteLiquibaseStepCapture(bundle, identity, 1024)
			if variant == "control" {
				if err != nil || capture.ExitCode != 0 || capture.ManifestDigest == "" {
					t.Fatal("valid bundle failed manifest verification", err)
				}
			} else if err == nil || !reflect.DeepEqual(capture, CompleteStepCapture{}) {
				t.Fatal("one-copy source bypassed semantic completion checks")
			}
		})
	}
}
