package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
)

const (
	captureArchiveAllowance int64 = 32768
	captureCommandStderrCap int64 = 4096
	captureStreamLimit      int64 = 67108864
)

var (
	errCaptureArtifactBinding = errors.New("invalid bounded capture artifact binding")
	errCaptureArtifactRead    = errors.New("bounded capture artifact unavailable")
)

// readCaptureArtifact acquires one of the three fixed output files from the
// caller's exact inspected container. An optional validated invocation token
// selects the session subdirectory; no host path or arbitrary file is accepted.
// The caller owns a shared complete-capture deadline and the before/after native
// identity checks. This method never resets that deadline or grants completion,
// disposal or SQL authority. Requirements: capture-access amendment §§2.1/3 and
// liquibase-private-capture-ownership.md bounded private reads.
func (r *DockerRuntime) readCaptureArtifact(ctx context.Context, id, invocation, file string, limit int64) ([]byte, error) {
	if file != "manifest.complete" && file != "stdout.complete" && file != "stderr.complete" {
		return nil, errCaptureArtifactBinding
	}
	return r.readPrivateCaptureFile(ctx, id, invocation, file, limit)
}

// readPrivateCaptureFile additionally permits the root readiness marker. It is
// not exposed as an arbitrary Docker filesystem reader: every filename, scope
// and cap is checked before transport, including the readiness-only root rule.
func (r *DockerRuntime) readPrivateCaptureFile(ctx context.Context, id, invocation, file string, limit int64) ([]byte, error) {
	if r == nil || !createdContainerID.MatchString(id) || limit <= 0 || limit > captureStreamLimit ||
		(invocation != "" && !stepCaptureToken.MatchString(invocation)) {
		return nil, errCaptureArtifactBinding
	}
	if file != "manifest.complete" && file != "stdout.complete" && file != "stderr.complete" && file != "ready.complete" {
		return nil, errCaptureArtifactBinding
	}
	if (file == "manifest.complete" || file == "ready.complete") && limit > 8192 || file == "ready.complete" && invocation != "" {
		return nil, errCaptureArtifactBinding
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return nil, errCaptureArtifactBinding
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runner, ok := r.runner.(boundedBinaryRunner)
	if !ok {
		return nil, errCaptureArtifactRead
	}
	root := "/sqlrs/capture/"
	if invocation != "" {
		root += invocation + "/"
	}
	wireLimit := limit + captureArchiveAllowance // limits above make addition safe
	archive, err := runner.RunBounded(ctx, r.binary, []string{"cp", id + ":" + root + file, "-"}, wireLimit, captureCommandStderrCap)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil || int64(len(archive)) > wireLimit {
		return nil, errCaptureArtifactRead
	}
	return decodeCaptureArchive(archive, file, limit)
}

// captureTarSize accepts the ordinary octal size field only. Output files are
// capped at 64 MiB, so binary/GNU numeric sizes are unnecessary and ambiguous
// huge declarations can be rejected without allocating their claimed payload.
func captureTarSize(header []byte) (int64, bool) {
	field := header[124:136]
	if field[0]&0x80 != 0 {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.Trim(string(field), " \x00"), 8, 64)
	return value, err == nil && value >= 0
}

// capturePAXSafe permits bounded metadata, but never path/link/size overrides or
// sparse interpretations. Every pre-file PAX record is checked, including ones
// a subsequent metadata record could shadow in archive/tar's effective header.
func capturePAXSafe(data []byte) bool {
	for len(data) > 0 {
		space := bytes.IndexByte(data, ' ')
		if space <= 0 {
			return false
		}
		length, err := strconv.ParseInt(string(data[:space]), 10, 64)
		if err != nil || length <= int64(space+2) || length > int64(len(data)) {
			return false
		}
		record := data[space+1 : length]
		if record[len(record)-1] != '\n' {
			return false
		}
		key, _, ok := bytes.Cut(record[:len(record)-1], []byte("="))
		if !ok || len(key) == 0 {
			return false
		}
		name := strings.ToLower(string(key))
		if name == "path" || name == "linkpath" || name == "size" || strings.HasPrefix(name, "gnu.sparse") || name == "schily.realsize" || name == "schily.filetype" {
			return false
		}
		data = data[length:]
	}
	return true
}

// decodeCaptureArchive validates raw framing before asking archive/tar to verify
// checksums and metadata syntax. That prevents transparent metadata handling
// from hiding links, sparse payloads, extra entries or size/path overrides.
// It returns only a slice of the already bounded invocation-local wire buffer;
// no extraction, file creation or allocation based on a declared size occurs.
func decodeCaptureArchive(data []byte, file string, limit int64) ([]byte, error) {
	offset := int64(0)
	for {
		if offset+512 > int64(len(data)) {
			return nil, errCaptureArtifactRead
		}
		header := data[offset : offset+512]
		size, valid := captureTarSize(header)
		if !valid {
			return nil, errCaptureArtifactRead
		}
		if header[156] != tar.TypeXHeader {
			if (header[156] != tar.TypeReg && header[156] != tar.TypeRegA) || size > limit {
				return nil, errCaptureArtifactRead
			}
			name := string(bytes.TrimRight(header[:100], "\x00"))
			if string(header[257:263]) == "ustar\x00" {
				if prefix := string(bytes.TrimRight(header[345:500], "\x00")); prefix != "" {
					name = prefix + "/" + name
				}
			}
			if name != file {
				return nil, errCaptureArtifactRead
			}
			end := offset + 512 + ((size+511)/512)*512
			if end+1024 > int64(len(data)) {
				return nil, errCaptureArtifactRead
			}
			// Includes data padding and the required two zero EOF blocks;
			// further ordinary Docker padding is allowed only when zero too.
			for _, value := range data[offset+512+size:] {
				if value != 0 {
					return nil, errCaptureArtifactRead
				}
			}
			reader := tar.NewReader(bytes.NewReader(data))
			actual, err := reader.Next()
			if err != nil || actual.Name != file || actual.Size != size || actual.Linkname != "" ||
				(actual.Typeflag != tar.TypeReg && actual.Typeflag != tar.TypeRegA) {
				return nil, errCaptureArtifactRead
			}
			return data[offset+512 : offset+512+size], nil
		}
		// Cap metadata before rounding so hostile int64 declarations cannot
		// overflow offset arithmetic or allocate an enormous PAX record.
		if size > captureArchiveAllowance || offset+512+size > int64(len(data)) {
			return nil, errCaptureArtifactRead
		}
		if !capturePAXSafe(data[offset+512 : offset+512+size]) {
			return nil, errCaptureArtifactRead
		}
		next := offset + 512 + ((size+511)/512)*512
		if next > int64(len(data)) || next > captureArchiveAllowance {
			return nil, errCaptureArtifactRead
		}
		for _, value := range data[offset+512+size : next] {
			if value != 0 {
				return nil, errCaptureArtifactRead
			}
		}
		offset = next
	}
}
