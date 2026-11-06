package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// captureArtifactTestReader is the approved fixed-file transport boundary, not
// an arbitrary file/mount API. Its caller already owns the inspected container
// and supplies the complete read's shared deadline. CA06–CA10 qualify bytes and
// command addressing here; native identity/liveness remains outside this test.
type captureArtifactTestReader interface {
	readCaptureArtifact(context.Context, string, string, string, int64) ([]byte, error)
}

type artifactRequirementsRunner struct {
	fakeRunner
	archive              []byte
	err                  error
	name                 string
	args                 []string
	stdoutCap, stderrCap int64
	deadline             time.Time
	calls                int
	onCopy               func()
}

func (r *artifactRequirementsRunner) RunBounded(ctx context.Context, name string, args []string, stdout, stderr int64) ([]byte, error) {
	r.calls++
	r.name, r.args, r.stdoutCap, r.stderrCap = name, append([]string(nil), args...), stdout, stderr
	r.deadline, _ = ctx.Deadline()
	if r.onCopy != nil {
		r.onCopy()
	}
	return r.archive, r.err
}

// artifactRequirementsTar uses the standard writer only to construct fixtures;
// expected acceptance comes from the fixed-file/limit contract, not a second
// invocation of the product parser.
func artifactRequirementsTar(t *testing.T, name string, body []byte, kind byte, extra bool) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := tar.NewWriter(&buffer)
	header := &tar.Header{Name: name, Mode: 0600, Size: int64(len(body)), Typeflag: kind}
	if kind != tar.TypeReg {
		header.Size, header.Linkname = 0, "foreign"
	}
	if err := w.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if extra {
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// artifactRequirementsPAX prepends a checksum-valid metadata header without
// letting tar.Writer normalize away deliberately hostile metadata test records.
func artifactRequirementsPAX(t *testing.T, key, value string, archive []byte) []byte {
	t.Helper()
	content := key + "=" + value + "\n"
	length := len(content) + 2
	var record string
	for {
		record = fmt.Sprintf("%d %s", length, content)
		if len(record) == length {
			break
		}
		length = len(record)
	}
	return artifactRequirementsRawPAX(t, []byte(record), archive)
}

func artifactRequirementsRawPAX(t *testing.T, record, archive []byte) []byte {
	t.Helper()
	meta := artifactRequirementsTar(t, "metadata", record, tar.TypeReg, false)
	meta[156] = tar.TypeXHeader
	artifactRequirementsChecksum(meta)
	end := 512 + ((len(record)+511)/512)*512
	return append(meta[:end:end], archive...)
}

// artifactRequirementsChecksum keeps deliberate field mutations checksum-valid,
// so rejection cannot be credited merely to an unrelated corrupt checksum.
func artifactRequirementsChecksum(header []byte) {
	for index := 148; index < 156; index++ {
		header[index] = ' '
	}
	checksum := 0
	for _, value := range header[:512] {
		checksum += int(value)
	}
	copy(header[148:156], fmt.Sprintf("%06o\x00 ", checksum))
}

func requireArtifactRequirementsReader(t *testing.T, r *artifactRequirementsRunner) captureArtifactTestReader {
	t.Helper()
	runtime := NewDocker(Options{Binary: "configured-docker", Runner: r})
	reader, ok := any(runtime).(captureArtifactTestReader)
	if !ok {
		t.Fatal("approved bounded fixed-artifact transport is unavailable; tar case not reached")
	}
	return reader
}

// TestCaptureArtifactWireGrammar covers CA06/07/08 without relying on host EACCES.
// Every negative first reads an intact exact-name archive through the same runner.
func TestCaptureArtifactWireGrammar(t *testing.T) {
	for _, variant := range []string{
		"empty", "binary", "ordinary_padding", "wire_at_cap", "wire_over_cap",
		"content_at_cap", "content_over_cap", "wrong_name", "absolute_name", "traversal_name",
		"duplicate", "symlink", "hardlink", "directory", "fifo", "device", "sparse",
		"bad_checksum", "short_header", "short_body", "missing_eof", "nonzero_tail",
		"copy_failure", "pax_time", "pax_path_override", "pax_size_override", "pax_sparse",
	} {
		t.Run(variant, func(t *testing.T) {
			id := strings.Repeat("a", 64)
			body := []byte{0, 255, 128, '\n'}
			runner := &artifactRequirementsRunner{archive: artifactRequirementsTar(t, "stdout.complete", body, tar.TypeReg, false)}
			reader := requireArtifactRequirementsReader(t, runner)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			control, err := reader.readCaptureArtifact(ctx, id, "", "stdout.complete", 1024)
			if err != nil || !bytes.Equal(control, body) || runner.calls != 1 {
				t.Fatalf("intact artifact control: calls=%d failed=%t", runner.calls, err != nil)
			}
			wantOK := false
			switch variant {
			case "empty":
				body, wantOK = nil, true
				runner.archive = artifactRequirementsTar(t, "stdout.complete", body, tar.TypeReg, false)
			case "binary":
				wantOK = true
			case "ordinary_padding", "wire_at_cap":
				wantOK = true
				size := 10240
				if variant == "wire_at_cap" {
					size = 1024 + 32768
				}
				runner.archive = append(runner.archive, make([]byte, size-len(runner.archive))...)
			case "wire_over_cap":
				runner.archive = append(runner.archive, make([]byte, 1024+32768+1-len(runner.archive))...)
			case "content_at_cap", "content_over_cap":
				size := 1024
				wantOK = variant == "content_at_cap"
				if !wantOK {
					size++
				}
				body = bytes.Repeat([]byte{'b'}, size)
				runner.archive = artifactRequirementsTar(t, "stdout.complete", body, tar.TypeReg, false)
			case "wrong_name":
				runner.archive = artifactRequirementsTar(t, "foreign", body, tar.TypeReg, false)
			case "absolute_name":
				runner.archive = artifactRequirementsTar(t, "/stdout.complete", body, tar.TypeReg, false)
			case "traversal_name":
				runner.archive = artifactRequirementsTar(t, "../stdout.complete", body, tar.TypeReg, false)
			case "duplicate":
				runner.archive = artifactRequirementsTar(t, "stdout.complete", body, tar.TypeReg, true)
			case "symlink", "hardlink", "directory", "fifo", "device", "sparse":
				kind := map[string]byte{"symlink": tar.TypeSymlink, "hardlink": tar.TypeLink, "directory": tar.TypeDir, "fifo": tar.TypeFifo, "device": tar.TypeChar, "sparse": tar.TypeGNUSparse}[variant]
				if variant == "sparse" {
					runner.archive[156] = kind
					artifactRequirementsChecksum(runner.archive)
				} else {
					runner.archive = artifactRequirementsTar(t, "stdout.complete", nil, kind, false)
				}
			case "bad_checksum":
				// Keep the allowed filename/type/size intact, otherwise an
				// earlier binding check could hide a missing checksum check.
				runner.archive[148] ^= 1
			case "short_header":
				runner.archive = runner.archive[:100]
			case "short_body":
				runner.archive = runner.archive[:513]
			case "missing_eof":
				runner.archive = runner.archive[:1024]
			case "nonzero_tail":
				runner.archive = append(runner.archive, 'x')
			case "copy_failure":
				runner.err = errors.New("private-copy-diagnostic-marker")
			case "pax_time":
				wantOK = true
				runner.archive = artifactRequirementsPAX(t, "mtime", "1700000000.123", runner.archive)
			case "pax_path_override":
				runner.archive = artifactRequirementsPAX(t, "path", "foreign", runner.archive)
			case "pax_size_override":
				runner.archive = artifactRequirementsPAX(t, "size", "0", runner.archive)
			case "pax_sparse":
				runner.archive = artifactRequirementsPAX(t, "GNU.sparse.map", "0,4", runner.archive)
			}
			got, err := reader.readCaptureArtifact(ctx, id, "", "stdout.complete", 1024)
			if (err == nil) != wantOK {
				t.Fatalf("artifact outcome failed=%t, want success=%t", err != nil, wantOK)
			}
			if wantOK && !bytes.Equal(got, body) {
				t.Fatal("archive changed binary artifact bytes")
			}
			if !wantOK && len(got) != 0 {
				t.Fatal("rejected archive returned partial bytes")
			}
			if err != nil && strings.Contains(err.Error(), "private-copy-diagnostic-marker") {
				t.Fatal("copy diagnostic leaked")
			}
			if runner.calls != 2 || runner.name != "configured-docker" || runner.stdoutCap != 1024+32768 || runner.stderrCap != 4096 {
				t.Fatal("artifact bypassed configured runner, acquisition quotas or command count")
			}
			deadline, _ := ctx.Deadline()
			if runner.deadline != deadline {
				t.Fatal("artifact reset the shared deadline")
			}
			if len(runner.args) != 3 || runner.args[0] != "cp" || runner.args[1] != id+":/sqlrs/capture/stdout.complete" || runner.args[2] != "-" {
				t.Fatal("artifact used an unbound path/command")
			}
		})
	}
}

// TestCaptureArtifactBindingAndDeadline verifies that only fixed file names and
// engine tokens reach the configured binary channel, never text/log fallbacks.
func TestCaptureArtifactBindingAndDeadline(t *testing.T) {
	for _, variant := range []string{"step_path", "invalid_id", "invalid_invocation", "arbitrary_file", "ready_not_capture", "zero_cap", "negative_cap", "over_stream_cap", "over_manifest_cap", "missing_deadline", "cancelled", "cancelled_during_copy", "unsupported_runner"} {
		t.Run(variant, func(t *testing.T) {
			id, invocation, file, limit := strings.Repeat("a", 64), "", "manifest.complete", int64(8192)
			runner := &artifactRequirementsRunner{archive: artifactRequirementsTar(t, file, []byte("{}"), tar.TypeReg, false)}
			reader := requireArtifactRequirementsReader(t, runner)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := reader.readCaptureArtifact(ctx, id, invocation, file, limit); err != nil {
				t.Fatalf("intact binding control: %v", err)
			}
			before := runner.calls
			switch variant {
			case "step_path":
				invocation = "saved-invocation"
			case "invalid_id":
				id = "short"
			case "invalid_invocation":
				invocation = "../foreign"
			case "arbitrary_file":
				file = "../private-secret"
			case "ready_not_capture":
				file = "ready.complete"
			case "zero_cap":
				limit = 0
			case "negative_cap":
				limit = -1
			case "over_stream_cap":
				file, limit = "stdout.complete", 67108865
			case "over_manifest_cap":
				limit = 8193
			case "missing_deadline":
				ctx = context.Background()
			case "cancelled":
				cancel()
			case "cancelled_during_copy":
				runner.onCopy = cancel
			case "unsupported_runner":
				text := &fakeRunner{}
				runtime := NewDocker(Options{Binary: "configured-docker", Runner: text})
				reader = any(runtime).(captureArtifactTestReader)
				defer func() {
					if len(text.calls) != 0 {
						t.Error("missing binary channel fell back to text runner")
					}
				}()
			}
			out, err := reader.readCaptureArtifact(ctx, id, invocation, file, limit)
			if variant == "step_path" {
				if err != nil || !bytes.Equal(out, []byte("{}")) || runner.args[1] != id+":/sqlrs/capture/saved-invocation/manifest.complete" {
					t.Fatal("step artifact did not use its exact saved invocation token")
				}
				return
			}
			wantCalls := before
			if variant == "cancelled_during_copy" {
				wantCalls++
			}
			if err == nil || len(out) != 0 || runner.calls != wantCalls {
				t.Fatal("invalid binding/deadline reached copy or returned bytes")
			}
		})
	}
}

// TestCaptureArtifactMetadataGrammar covers hidden/shadowed metadata and invalid
// framing independently of both wire overflow and the regular-file checksum.
func TestCaptureArtifactMetadataGrammar(t *testing.T) {
	for _, variant := range []string{"no_space", "bad_length", "short_record", "missing_newline", "missing_equals", "empty_key", "hidden_path", "metadata_too_large", "metadata_header_over_allowance", "metadata_padding", "binary_size", "invalid_octal_size", "prefixed_name", "declared_oversize"} {
		t.Run(variant, func(t *testing.T) {
			base := artifactRequirementsTar(t, "stdout.complete", []byte("data"), tar.TypeReg, false)
			runner := &artifactRequirementsRunner{archive: base}
			reader := requireArtifactRequirementsReader(t, runner)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := reader.readCaptureArtifact(ctx, strings.Repeat("a", 64), "", "stdout.complete", 65536); err != nil {
				t.Fatalf("intact metadata control: %v", err)
			}
			switch variant {
			case "no_space":
				runner.archive = artifactRequirementsRawPAX(t, []byte("bad"), base)
			case "bad_length":
				runner.archive = artifactRequirementsRawPAX(t, []byte("x k=v\n"), base)
			case "short_record":
				runner.archive = artifactRequirementsRawPAX(t, []byte("99 k=v\n"), base)
			case "missing_newline":
				runner.archive = artifactRequirementsRawPAX(t, []byte("7 k=xxX"), base)
			case "missing_equals":
				runner.archive = artifactRequirementsRawPAX(t, []byte("4 a\n"), base)
			case "empty_key":
				runner.archive = artifactRequirementsRawPAX(t, []byte("5 =v\n"), base)
			case "hidden_path":
				runner.archive = artifactRequirementsPAX(t, "path", "foreign", artifactRequirementsPAX(t, "mtime", "1700000000.123", base))
			case "metadata_too_large":
				runner.archive = artifactRequirementsPAX(t, "ignored", strings.Repeat("x", 32769), base)
			case "metadata_header_over_allowance":
				runner.archive = artifactRequirementsPAX(t, "ignored", strings.Repeat("x", 32753), base)
			case "metadata_padding":
				runner.archive = artifactRequirementsPAX(t, "mtime", "1700000000.123", base)
				runner.archive[700] = 'x'
			case "binary_size":
				runner.archive[124] = 0x80
				artifactRequirementsChecksum(runner.archive)
			case "invalid_octal_size":
				runner.archive[124] = 'x'
				artifactRequirementsChecksum(runner.archive)
			case "prefixed_name":
				copy(runner.archive[345:500], "foreign")
				artifactRequirementsChecksum(runner.archive)
			case "declared_oversize":
				copy(runner.archive[124:136], fmt.Sprintf("%011o\x00", int64(67108865)))
				artifactRequirementsChecksum(runner.archive)
			}
			out, err := reader.readCaptureArtifact(ctx, strings.Repeat("a", 64), "", "stdout.complete", 65536)
			if err == nil || len(out) != 0 {
				t.Fatal("unsafe metadata/framing accepted or returned bytes")
			}
			if runner.calls != 2 {
				t.Fatal("metadata refusal did not reach binary transport")
			}
		})
	}
}
