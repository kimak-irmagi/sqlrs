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

// hostRequest gives RC7's native normalizer and actual stock CLI the same
// project directory. RC7 prepareRequest reads os.Getwd(), not Request.WorkDir;
// Windows batch mode concealed that distinction in the former collector.
// PA04 requires native Linux paths, not a product compatibility patch.
func hostRequest(req Request, fixture string) (Request, error) {
	dir := ""
	switch fixture {
	case "jhipster":
		dir = "jhipster-sample-app"
	case "mobilitydata-user", "mobilitydata-main":
		dir = "mobilitydata"
	default:
		return Request{}, fmt.Errorf("unknown public fixture")
	}
	if !filepath.IsAbs(req.WorkDir) {
		return Request{}, fmt.Errorf("absolute fixture root required")
	}
	req.WorkDir = filepath.Join(req.WorkDir, dir)
	req.LiquibaseArgs = append([]string(nil), req.LiquibaseArgs...)
	for index := 1; index < len(req.LiquibaseArgs); index++ {
		if req.LiquibaseArgs[index-1] == "--searchPath" {
			req.LiquibaseArgs[index] = "."
		}
		if req.LiquibaseArgs[index-1] == "--changelog-file" {
			req.LiquibaseArgs[index] = strings.TrimPrefix(req.LiquibaseArgs[index], dir+"/")
		}
	}
	return req, nil
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

// oracleMismatchParts diagnoses dimensions only; never emit rows or values.
func oracleMismatchParts(actual []string, expected string) []string {
	want := strings.Split(expected, "\n")
	if len(actual) != len(want) {
		return []string{"row_count"}
	}
	seen := map[string]bool{}
	result := []string{}
	add := func(part string) {
		if !seen[part] {
			seen[part] = true
			result = append(result, part)
		}
	}
	for index, line := range want {
		if actual[index] == line {
			continue
		}
		if strings.HasPrefix(line, "change|") || strings.HasPrefix(line, "changelog|") {
			a, b := strings.Split(actual[index], "|"), strings.Split(line, "|")
			if len(a) != len(b) || len(b) != 8 {
				add("row_shape")
				continue
			}
			for field, name := range []string{"kind", "order", "id", "author", "path", "checksum", "contexts", "labels"} {
				if a[field] != b[field] {
					add(name)
				}
			}
		} else {
			add("schema")
		}
	}
	return result
}

const (
	StatusSucceeded        = "succeeded"
	StatusFailed           = "failed"
	StatusRecoveryRequired = "recovery_required"
)
