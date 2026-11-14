package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// sessionCaptureBundle owns slices of one bounded wire buffer, never extracted
// files. It supplies the same manifest/stream verifier as the single-file source.
// O3/CT04: docs/architecture/issue89-cost-attribution-and-trimming.md §2.
type sessionCaptureBundle map[string][]byte

func (b sessionCaptureBundle) readCapture(file string, limit int64) ([]byte, error) {
	value, found := b[file]
	if !found || limit <= 0 || int64(len(value)) > limit {
		return nil, errCaptureArtifactRead
	}
	return value, nil
}

// readSessionCaptureBundle reads only the attributed invocation's contents.
// Its caller retains parent/idle observations and one shared read deadline.
// A missing file/transport result never falls back to local or individual reads.
func (r *DockerRuntime) readSessionCaptureBundle(ctx context.Context, id, invocation string, limit int64) (sessionCaptureBundle, error) {
	if r == nil || !createdContainerID.MatchString(id) || !stepCaptureToken.MatchString(invocation) || limit <= 0 || limit > captureStreamLimit {
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
	// All additions are bounded by the admitted 64 MiB per-stream limit.
	wireLimit := 2*limit + 8192 + captureArchiveAllowance
	wire, err := preparecost.Observe(ctx, preparecost.CaptureCopy, func(child context.Context) ([]byte, error) {
		return runner.RunBounded(child, r.binary, []string{"cp", id + ":/sqlrs/capture/" + invocation + "/.", "-"}, wireLimit, captureCommandStderrCap)
	})
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil || int64(len(wire)) > wireLimit {
		return nil, errCaptureArtifactRead
	}
	return decodeSessionCaptureArchive(wire, limit)
}

// decodeSessionCaptureArchive validates raw headers/metadata before archive/tar
// can hide PAX overrides. Then the standard reader verifies every checksum and
// effective header. Declared sizes never allocate memory or extract files.
func decodeSessionCaptureArchive(data []byte, limit int64) (sessionCaptureBundle, error) {
	if limit <= 0 || limit > captureStreamLimit || int64(len(data)) > 2*limit+8192+captureArchiveAllowance {
		return nil, errCaptureArtifactRead
	}
	type checkedEntry struct {
		name string
		size int64
		kind byte
	}
	var checked []checkedEntry
	result := make(sessionCaptureBundle, 3)
	offset, metadataBytes := int64(0), int64(0)
	directory, pendingMetadata := false, false
	for {
		if offset+512 > int64(len(data)) {
			return nil, errCaptureArtifactRead
		}
		header := data[offset : offset+512]
		if captureZeroBytes(header) {
			if offset+1024 > int64(len(data)) || !captureZeroBytes(data[offset:]) || len(result) != 3 || pendingMetadata {
				return nil, errCaptureArtifactRead
			}
			break
		}
		size, valid := captureTarSize(header)
		if !valid {
			return nil, errCaptureArtifactRead
		}
		kind := header[156]
		if kind == tar.TypeXHeader {
			if size > captureArchiveAllowance || offset+512+size > int64(len(data)) {
				return nil, errCaptureArtifactRead
			}
			end := offset + 512 + ((size+511)/512)*512
			metadataBytes += end - offset
			if metadataBytes > captureArchiveAllowance || end > int64(len(data)) ||
				!capturePAXSafe(data[offset+512:offset+512+size]) || !captureZeroBytes(data[offset+512+size:end]) {
				return nil, errCaptureArtifactRead
			}
			offset, pendingMetadata = end, true
			continue
		}
		if kind != tar.TypeReg && kind != tar.TypeRegA && kind != tar.TypeDir {
			return nil, errCaptureArtifactRead
		}
		name := string(bytes.TrimRight(header[:100], "\x00"))
		if string(header[257:263]) == "ustar\x00" {
			if prefix := string(bytes.TrimRight(header[345:500], "\x00")); prefix != "" {
				name = prefix + "/" + name
			}
		}
		if !captureZeroBytes(header[157:257]) {
			return nil, errCaptureArtifactRead
		}
		if kind == tar.TypeDir {
			if directory || len(result) != 0 || size != 0 || name != "." && name != "./" {
				return nil, errCaptureArtifactRead
			}
			directory = true
		} else {
			// Recognise only one literal dot prefix, never path.Clean aliases.
			file := strings.TrimPrefix(name, "./")
			fileLimit := limit
			switch file {
			case "manifest.complete":
				fileLimit = 8192
			case "stdout.complete", "stderr.complete":
			default:
				return nil, errCaptureArtifactRead
			}
			if _, duplicate := result[file]; duplicate || size > fileLimit || offset+512+size > int64(len(data)) {
				return nil, errCaptureArtifactRead
			}
			result[file] = data[offset+512 : offset+512+size]
		}
		// Size was bounded before rounding/addition (directories require zero).
		end := offset + 512 + ((size+511)/512)*512
		if end > int64(len(data)) || !captureZeroBytes(data[offset+512+size:end]) {
			return nil, errCaptureArtifactRead
		}
		if kind == tar.TypeRegA {
			kind = tar.TypeReg // archive/tar normalises legacy regular headers.
		}
		checked = append(checked, checkedEntry{name, size, kind})
		offset, pendingMetadata = end, false
	}
	reader := tar.NewReader(bytes.NewReader(data))
	for _, entry := range checked {
		actual, err := reader.Next()
		if err != nil || actual.Name != entry.name || actual.Size != entry.size || actual.Typeflag != entry.kind || actual.Linkname != "" {
			return nil, errCaptureArtifactRead
		}
	}
	if _, err := reader.Next(); err != io.EOF {
		return nil, errCaptureArtifactRead
	}
	return result, nil
}

// captureZeroBytes checks padding/EOF without allocating an all-zero buffer.
func captureZeroBytes(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
}
