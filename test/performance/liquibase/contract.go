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

// classifyEngineFailure emits only fixed operation classes, never error text,
// paths, SQL or connection strings. PA10 includes diagnostics on failed runs.
func classifyEngineFailure(err *ErrorResponse) string {
	if err == nil {
		return "unknown"
	}
	switch err.Message {
	case "cannot initialize base state":
		return "base_state"
	case "cannot parse liquibase changesets":
		return "plan_parse"
	case "snapshot failed", "snapshot prepare failed", "snapshot resume failed":
		return "snapshot"
	case "cannot start runtime":
		return "runtime_start"
	case "cannot clone state":
		return "clone_state"
	case "cannot prepare scripts":
		return "scripts_prepare"
	case "liquibase execution failed":
		detail := strings.ToLower(err.Details)
		for _, rule := range []struct{ part, kind string }{{"connection refused", "connection_refused"}, {"no suitable driver", "jdbc_missing"}, {"permission denied", "permission_denied"}, {"was not found", "input_missing"}, {"no such file or directory", "file_missing"}, {"unexpected argument", "cli_arguments"}} {
			if strings.Contains(detail, rule.part) {
				return rule.kind
			}
		}
		return "liquibase_execute"
	}
	return "unknown"
}

const (
	StatusSucceeded        = "succeeded"
	StatusFailed           = "failed"
	StatusRecoveryRequired = "recovery_required"
)
