package benchmark

import (
	"debug/buildinfo"
	"fmt"
	"path/filepath"
	"strings"
)

// validateArtifact requires exact clean Linux source provenance before execution.
// See issue89-performance-actions-tests.md PA02.
func validateArtifact(info *buildinfo.BuildInfo, expected string) error {
	values := map[string]string{}
	for _, setting := range info.Settings {
		values[setting.Key] = setting.Value
	}
	if len(expected) != 40 || values["vcs.revision"] != expected || values["vcs.modified"] != "false" || values["GOOS"] != "linux" || values["GOARCH"] != "amd64" {
		return fmt.Errorf("artifact provenance differs")
	}
	return nil
}

// isWithin checks path-component containment, never a lexical prefix alone.
func isWithin(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// These external DTOs deliberately describe only the observed API fields.
// They import no product code and do not add compatibility behavior to sqlrs.
type Request struct {
	PrepareKind, ImageID, WorkDir, LiquibaseVersion, LiquibaseExec, LiquibaseExecMode string
	LiquibaseArgs                                                                     []string
}
type Accepted struct {
	JobID string `json:"job_id"`
}
type Status struct {
	Status string         `json:"status"`
	Result *Result        `json:"result"`
	Error  *ErrorResponse `json:"error"`
}
type Result struct {
	DSN string `json:"dsn"`
}
type ErrorResponse struct{ Code, Message, Details string }

const (
	StatusSucceeded        = "succeeded"
	StatusFailed           = "failed"
	StatusRecoveryRequired = "recovery_required"
)
