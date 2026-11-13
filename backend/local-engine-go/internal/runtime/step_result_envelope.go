package runtime

import (
	"bytes"
	"strconv"
)

// stepResultEnvelope owns bounded views of one private transport buffer. It
// carries no cleanup authority; the native caller must separately bind the
// unchanged live parent and the saved prune authorization. IR01–IR04:
// docs/architecture/issue89-same-exec-result-tests.md.
type stepResultEnvelope struct {
	files  sessionCaptureBundle
	detail []byte
	pruned bool
}

// stepResultWireLimit adds fixed manifest/detail/framing budgets to the two
// already-admitted stream caps; its caller first enforces the 1..64MiB range.
func stepResultWireLimit(streamLimit int64) int64 {
	return 2*streamLimit + 8192 + 4096 + 256
}

// decodeStepResultEnvelope accepts only the explicit ordered binary grammar.
// Sizes are checked before indexing; arbitrary output is never echoed in errors.
// Streams are subsequently checked by the existing completion/hash verifier.
func decodeStepResultEnvelope(wire []byte, streamLimit int64) (stepResultEnvelope, error) {
	invalid := func() (stepResultEnvelope, error) { return stepResultEnvelope{}, errCaptureArtifactRead }
	if streamLimit < 1 || streamLimit > 64<<20 || int64(len(wire)) > stepResultWireLimit(streamLimit) || !bytes.HasPrefix(wire, []byte("sqlrs-lb-result-v1\n")) {
		return invalid()
	}
	wire = wire[len("sqlrs-lb-result-v1\n"):]
	result := stepResultEnvelope{files: make(sessionCaptureBundle, 3)}
	for _, member := range []struct {
		name  string
		limit int64
	}{
		{"manifest", 8192}, {"stdout", streamLimit}, {"stderr", streamLimit}, {"detail", 4096},
	} {
		end := bytes.IndexByte(wire, '\n')
		if end < 0 || end > 63 || !bytes.HasPrefix(wire[:end], []byte(member.name+" ")) {
			return invalid()
		}
		sizeText := string(wire[len(member.name)+1 : end])
		size, err := strconv.ParseInt(sizeText, 10, 64)
		if err != nil || size < 0 || size > member.limit || strconv.FormatInt(size, 10) != sizeText {
			return invalid()
		}
		wire = wire[end+1:]
		if size > int64(len(wire)) {
			return invalid()
		}
		data := wire[:int(size)]
		wire = wire[int(size):]
		if member.name == "detail" {
			result.detail = data
		} else {
			result.files[member.name+".complete"] = data
		}
	}
	switch string(wire) {
	case "sqlrs-lb-result-end-v1 idle none\n":
	case "sqlrs-lb-result-end-v1 idle absent\n":
		result.pruned = true
	default:
		return invalid()
	}
	return result, nil
}
