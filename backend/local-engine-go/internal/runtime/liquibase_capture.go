package runtime

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
)

//go:embed assets/liquibase-capture-v2.sh
var liquibaseCaptureHelper []byte

const LiquibaseCaptureProtocol = planningcodec.CaptureProtocolRevision

// LiquibaseCaptureHelper returns the exact engine-owned bytes to publish in
// the content-addressed artifact store before attempting a planning launch.
func LiquibaseCaptureHelper() ([]byte, string) {
	return append([]byte(nil), liquibaseCaptureHelper...), captureDigest(liquibaseCaptureHelper)
}

// CaptureIdentity is the immutable journal binding against which the private
// helper's completion manifest is checked after an exact executor stop.
type CaptureIdentity struct {
	JobID           string
	AttemptNo       int
	ExecutionDigest string
	HelperDigest    string
	ExitCode        int
}

// CompleteCapture consists of both protected streams with a verified
// completion marker. Zero bytes are complete only when the marker proves it.
type CompleteCapture struct {
	Stdout, Stderr             []byte
	StdoutDigest, StderrDigest string
}

type liquibaseCaptureManifest struct {
	Protocol        string `json:"protocol"`
	JobID           string `json:"job_id"`
	TaskID          string `json:"task_id"`
	AttemptNo       int    `json:"attempt_no"`
	ExecutionDigest string `json:"execution_digest"`
	HelperDigest    string `json:"helper_digest"`
	ChildExitCode   int    `json:"child_exit_code"`
	StdoutEOF       bool   `json:"stdout_eof"`
	StderrEOF       bool   `json:"stderr_eof"`
	StdoutBytes     int64  `json:"stdout_bytes"`
	StdoutSHA256    string `json:"stdout_sha256"`
	StderrBytes     int64  `json:"stderr_bytes"`
	StderrSHA256    string `json:"stderr_sha256"`
}

// ReadCompleteLiquibaseCapture checks the private manifest and every byte of
// both streams. The caller must first inspect and prove the exact planned
// Docker executor has stopped with identity.ExitCode; Docker logs are not a
// substitute. maxStreamBytes is a mandatory quota, not a truncation limit.
func ReadCompleteLiquibaseCapture(dir string, identity CaptureIdentity, maxStreamBytes int64) (CompleteCapture, error) {
	if dir == "" {
		return CompleteCapture{}, fmt.Errorf("invalid capture identity or limit")
	}
	return readCompleteLiquibaseCapture(filesystemCaptureFiles{dir: dir}, identity, maxStreamBytes)
}

// readCompleteLiquibaseCapture validates the same complete record regardless of
// acquisition transport. A failed later stream never returns an earlier stream.
func readCompleteLiquibaseCapture(files captureFileReader, identity CaptureIdentity, maxStreamBytes int64) (CompleteCapture, error) {
	if identity.JobID == "" || identity.AttemptNo <= 0 || maxStreamBytes <= 0 || !validCaptureDigest(identity.ExecutionDigest) || !validCaptureDigest(identity.HelperDigest) {
		return CompleteCapture{}, fmt.Errorf("invalid capture identity or limit")
	}
	manifestBytes, err := files.readCapture("manifest.complete", 8192)
	if err != nil {
		return CompleteCapture{}, fmt.Errorf("complete capture manifest unavailable: %w", err)
	}
	var manifest liquibaseCaptureManifest
	if !decodeCaptureObject(manifestBytes, &manifest, captureRequiredFields, "") {
		return CompleteCapture{}, fmt.Errorf("invalid capture manifest")
	}
	if manifest.Protocol != LiquibaseCaptureProtocol || manifest.JobID != identity.JobID || manifest.TaskID != "plan" ||
		manifest.AttemptNo != identity.AttemptNo || manifest.ExecutionDigest != identity.ExecutionDigest || manifest.HelperDigest != identity.HelperDigest || manifest.ChildExitCode != identity.ExitCode ||
		manifest.ChildExitCode < 0 || manifest.ChildExitCode > 255 || !manifest.StdoutEOF || !manifest.StderrEOF ||
		manifest.StdoutBytes < 0 || manifest.StderrBytes < 0 || !validCaptureHex(manifest.StdoutSHA256) || !validCaptureHex(manifest.StderrSHA256) {
		return CompleteCapture{}, fmt.Errorf("capture manifest identity or length differs")
	}
	stdout, err := files.readCapture("stdout.complete", maxStreamBytes)
	if err != nil {
		return CompleteCapture{}, fmt.Errorf("complete stdout unavailable: %w", err)
	}
	stderr, err := files.readCapture("stderr.complete", maxStreamBytes)
	if err != nil {
		return CompleteCapture{}, fmt.Errorf("complete stderr unavailable: %w", err)
	}
	if int64(len(stdout)) != manifest.StdoutBytes || int64(len(stderr)) != manifest.StderrBytes ||
		captureHex(stdout) != manifest.StdoutSHA256 || captureHex(stderr) != manifest.StderrSHA256 {
		return CompleteCapture{}, fmt.Errorf("protected capture bytes differ from manifest")
	}
	return CompleteCapture{Stdout: stdout, Stderr: stderr, StdoutDigest: captureDigest(stdout), StderrDigest: captureDigest(stderr)}, nil
}

func readRegularBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("capture file is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit || int64(len(content)) != info.Size() {
		return nil, fmt.Errorf("capture file length changed")
	}
	return content, nil
}

func captureHex(value []byte) string    { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func captureDigest(value []byte) string { return "sha256:" + captureHex(value) }
func validCaptureDigest(value string) bool {
	return len(value) == 71 && value[:7] == "sha256:" && validCaptureHex(value[7:])
}
func validCaptureHex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
