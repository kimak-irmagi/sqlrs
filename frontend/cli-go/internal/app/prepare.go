package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/inputset"
	inputliquibase "github.com/sqlrs/cli/internal/inputset/liquibase"
	inputpsql "github.com/sqlrs/cli/internal/inputset/psql"
	"github.com/sqlrs/cli/internal/pathutil"
	"github.com/sqlrs/cli/internal/refctx"
)

var runPrepareFn = cli.RunPrepare
var runPlanFn = cli.RunPlan
var submitPrepareFn = cli.SubmitPrepare
var bindPreparePsqlInputsFn = bindPreparePsqlInputs
var bindPrepareLiquibaseInputsFn = bindPrepareLiquibaseInputs

type prepareArgs struct {
	Image            string
	LiquibaseVersion string
	PsqlArgs         []string
	Watch            bool
	WatchSpecified   bool
	ProvenancePath   string
	Ref              string
	RefMode          string
	RefKeepWorktree  bool
}

type stdoutAndErr struct {
	stdout io.Writer
	stderr io.Writer
}

func parsePrepareArgs(args []string) (prepareArgs, bool, error) {
	opts := prepareArgs{Watch: true}
	if err := validateNoUnicodeDashFlags(args, 2); err != nil {
		return opts, false, err
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				opts.PsqlArgs = append(opts.PsqlArgs, args[i+1:]...)
			}
			mode, err := normalizeRefMode(opts.Ref, opts.RefMode, opts.RefKeepWorktree)
			if err != nil {
				return opts, false, err
			}
			opts.RefMode = mode
			if err := validatePrepareRefWatch(opts.Ref, opts.WatchSpecified, opts.Watch); err != nil {
				return opts, false, err
			}
			return opts, false, nil
		}
		switch {
		case arg == "--help" || arg == "-h":
			return opts, true, nil
		case arg == "--watch":
			opts.Watch = true
			opts.WatchSpecified = true
		case arg == "--no-watch":
			opts.Watch = false
			opts.WatchSpecified = true
		case arg == "--provenance-path":
			if i+1 >= len(args) {
				return opts, false, ExitErrorf(2, "Missing value for --provenance-path")
			}
			value := strings.TrimSpace(args[i+1])
			if value == "" {
				return opts, false, ExitErrorf(2, "Missing value for --provenance-path")
			}
			opts.ProvenancePath = value
			i++
		case arg == "--ref":
			if i+1 >= len(args) {
				return opts, false, ExitErrorf(2, "Missing value for --ref")
			}
			value := strings.TrimSpace(args[i+1])
			if value == "" {
				return opts, false, ExitErrorf(2, "Missing value for --ref")
			}
			opts.Ref = value
			i++
		case arg == "--ref-mode":
			if i+1 >= len(args) {
				return opts, false, ExitErrorf(2, "Missing value for --ref-mode")
			}
			opts.RefMode = strings.TrimSpace(args[i+1])
			i++
		case arg == "--ref-keep-worktree":
			opts.RefKeepWorktree = true
		case arg == "--image":
			if i+1 >= len(args) {
				return opts, false, ExitErrorf(2, "Missing value for --image")
			}
			value := strings.TrimSpace(args[i+1])
			if value == "" {
				return opts, false, ExitErrorf(2, "Missing value for --image")
			}
			opts.Image = value
			i++
		case strings.HasPrefix(arg, "--image="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--image="))
			if value == "" {
				return opts, false, ExitErrorf(2, "Missing value for --image")
			}
			opts.Image = value
		case arg == "--liquibase-version":
			if i+1 >= len(args) {
				return opts, false, ExitErrorf(2, "Missing value for --liquibase-version")
			}
			value := args[i+1]
			if !validLiquibaseVersion(value) {
				return opts, false, ExitErrorf(2, "Invalid --liquibase-version %q", value)
			}
			opts.LiquibaseVersion = value
			i++
		case strings.HasPrefix(arg, "--liquibase-version="):
			value := strings.TrimPrefix(arg, "--liquibase-version=")
			if !validLiquibaseVersion(value) {
				return opts, false, ExitErrorf(2, "Invalid --liquibase-version %q", value)
			}
			opts.LiquibaseVersion = value
		default:
			opts.PsqlArgs = append(opts.PsqlArgs, args[i:]...)
			mode, err := normalizeRefMode(opts.Ref, opts.RefMode, opts.RefKeepWorktree)
			if err != nil {
				return opts, false, err
			}
			opts.RefMode = mode
			if err := validatePrepareRefWatch(opts.Ref, opts.WatchSpecified, opts.Watch); err != nil {
				return opts, false, err
			}
			return opts, false, nil
		}
	}
	mode, err := normalizeRefMode(opts.Ref, opts.RefMode, opts.RefKeepWorktree)
	if err != nil {
		return opts, false, err
	}
	opts.RefMode = mode
	if err := validatePrepareRefWatch(opts.Ref, opts.WatchSpecified, opts.Watch); err != nil {
		return opts, false, err
	}
	return opts, false, nil
}

// validLiquibaseVersion enforces the user-facing tag syntax from the
// LiquibaseVersion schema in docs/api-guides/sqlrs-engine.openapi.yaml.
// Repository names, digests, and empty tags are never accepted as selectors.
func validLiquibaseVersion(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		first := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_'
		if first {
			continue
		}
		if i == 0 || c != '.' && c != '-' {
			return false
		}
	}
	return true
}

func normalizeRefMode(ref string, refMode string, refKeepWorktree bool) (string, error) {
	ref = strings.TrimSpace(ref)
	refMode = strings.TrimSpace(refMode)
	if ref == "" {
		if refMode != "" {
			return "", ExitErrorf(2, "--ref-mode requires --ref")
		}
		if refKeepWorktree {
			return "", ExitErrorf(2, "--ref-keep-worktree requires --ref")
		}
		return "", nil
	}
	if refMode == "" {
		refMode = "worktree"
	}
	if refMode != "worktree" && refMode != "blob" {
		return "", ExitErrorf(2, "--ref-mode %q is not supported (use blob or worktree)", refMode)
	}
	if refKeepWorktree && refMode != "worktree" {
		return "", ExitErrorf(2, "--ref-keep-worktree is only valid with --ref-mode worktree")
	}
	return refMode, nil
}

func validatePrepareRefWatch(ref string, watchSpecified bool, watch bool) error {
	if strings.TrimSpace(ref) == "" || !watchSpecified || watch {
		return nil
	}
	return ExitErrorf(2, "--no-watch is not supported with --ref")
}

func runPrepare(stdout, stderr io.Writer, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, args []string) error {
	result, handled, err := prepareResult(stdoutAndErr{stdout: stdout, stderr: stderr}, runOpts, cfg, workspaceRoot, cwd, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	fmt.Fprintf(stdout, "DSN=%s\n", result.DSN)
	return nil
}

func runPrepareParsed(stdout, stderr io.Writer, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, parsed prepareArgs, ref *refctx.Context) error {
	result, handled, err := prepareResultParsed(stdoutAndErr{stdout: stdout, stderr: stderr}, runOpts, cfg, workspaceRoot, cwd, parsed, ref)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	fmt.Fprintf(stdout, "DSN=%s\n", result.DSN)
	return nil
}

func runPrepareLiquibase(stdout, stderr io.Writer, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, args []string) error {
	return runPrepareLiquibaseWithPathMode(stdout, stderr, runOpts, cfg, workspaceRoot, cwd, args, true)
}

func runPrepareLiquibaseWithPathMode(stdout, stderr io.Writer, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, args []string, relativizePaths bool) error {
	result, handled, err := prepareResultLiquibaseWithPathMode(stdoutAndErr{stdout: stdout, stderr: stderr}, runOpts, cfg, workspaceRoot, cwd, args, relativizePaths)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	fmt.Fprintf(stdout, "DSN=%s\n", result.DSN)
	return nil
}

func runPrepareLiquibaseParsedWithPathMode(stdout, stderr io.Writer, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, parsed prepareArgs, ref *refctx.Context, relativizePaths bool) error {
	result, handled, err := prepareResultLiquibaseParsedWithPathMode(stdoutAndErr{stdout: stdout, stderr: stderr}, runOpts, cfg, workspaceRoot, cwd, parsed, ref, relativizePaths)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	fmt.Fprintf(stdout, "DSN=%s\n", result.DSN)
	return nil
}

func prepareResult(w stdoutAndErr, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, args []string) (client.PrepareJobResult, bool, error) {
	parsed, showHelp, err := parsePrepareArgs(args)
	if err != nil {
		return client.PrepareJobResult{}, false, err
	}
	if showHelp {
		cli.PrintPrepareUsage(w.stdout)
		return client.PrepareJobResult{}, true, nil
	}
	return prepareResultParsed(w, runOpts, cfg, workspaceRoot, cwd, parsed, nil)
}

func prepareResultParsed(w stdoutAndErr, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, parsed prepareArgs, ref *refctx.Context) (result client.PrepareJobResult, handled bool, err error) {
	return prepareResultStageRequest(w, runOpts, cfg, stageRunRequest{
		mode:          stageModePrepare,
		class:         "raw",
		kind:          "psql",
		parsed:        parsed,
		workspaceRoot: workspaceRoot,
		cwd:           cwd,
		invocationCwd: cwd,
		ref:           ref,
	})
}

func prepareResultLiquibase(w stdoutAndErr, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, args []string) (client.PrepareJobResult, bool, error) {
	return prepareResultLiquibaseWithPathMode(w, runOpts, cfg, workspaceRoot, cwd, args, true)
}

// Alias-backed liquibase stages already rebase file-bearing args to the alias
// file directory. When a searchPath is present, Liquibase must run from that
// app root so the changelog path and include graph stay aligned on the host.
func prepareResultLiquibaseWithPathMode(w stdoutAndErr, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, args []string, relativizePaths bool) (client.PrepareJobResult, bool, error) {
	parsed, showHelp, err := parsePrepareArgs(args)
	if err != nil {
		return client.PrepareJobResult{}, false, err
	}
	if showHelp {
		cli.PrintPrepareUsage(w.stdout)
		return client.PrepareJobResult{}, true, nil
	}
	return prepareResultLiquibaseParsedWithPathMode(w, runOpts, cfg, workspaceRoot, cwd, parsed, nil, relativizePaths)
}

func prepareResultLiquibaseParsedWithPathMode(w stdoutAndErr, runOpts cli.PrepareOptions, cfg config.LoadedConfig, workspaceRoot string, cwd string, parsed prepareArgs, ref *refctx.Context, relativizePaths bool) (result client.PrepareJobResult, handled bool, err error) {
	return prepareResultStageRequest(w, runOpts, cfg, stageRunRequest{
		mode:                    stageModePrepare,
		class:                   "raw",
		kind:                    "lb",
		parsed:                  parsed,
		workspaceRoot:           workspaceRoot,
		cwd:                     cwd,
		invocationCwd:           cwd,
		ref:                     ref,
		relativizeLiquibasePath: relativizePaths,
	})
}

func prepareResultStageRequest(w stdoutAndErr, runOpts cli.PrepareOptions, cfg config.LoadedConfig, req stageRunRequest) (result client.PrepareJobResult, handled bool, err error) {
	runtime, err := buildStageRuntime(w.stderr, runOpts, cfg, req)
	if err != nil {
		return client.PrepareJobResult{}, false, err
	}
	return executePrepareStageWithProvenance(w, runtime, req.parsed.ProvenancePath)
}

func usesPrepareRef(parsed prepareArgs, ref *refctx.Context) bool {
	return ref != nil || strings.TrimSpace(parsed.Ref) != ""
}

func finishPrepareCleanup(currentErr error, cleanup func() error) error {
	if cleanup == nil {
		return currentErr
	}
	cleanupErr := cleanup()
	if cleanupErr == nil || currentErr != nil {
		return currentErr
	}
	return cleanupErr
}

func printPrepareJobRefs(w io.Writer, accepted client.PrepareJobAccepted) {
	fmt.Fprintf(w, "JOB_ID=%s\n", accepted.JobID)
	fmt.Fprintf(w, "STATUS_URL=%s\n", accepted.StatusURL)
	fmt.Fprintf(w, "EVENTS_URL=%s\n", accepted.EventsURL)
}

func printRunSkipped(w io.Writer, reason string) {
	fmt.Fprintf(w, "RUN_SKIPPED=%s\n", reason)
}

func resolvePrepareImage(cliValue string, cfg config.LoadedConfig) (string, string, error) {
	if strings.TrimSpace(cliValue) != "" {
		return strings.TrimSpace(cliValue), "command line", nil
	}
	if cfg.ProjectConfigPath != "" {
		if value, ok, err := config.LookupDBMSImage(cfg.ProjectConfigPath); err != nil {
			return "", "", err
		} else if ok {
			return value, "workspace config", nil
		}
	}
	globalPath := filepath.Join(cfg.Paths.ConfigDir, "config.yaml")
	if fileExists(globalPath) {
		if value, ok, err := config.LookupDBMSImage(globalPath); err != nil {
			return "", "", err
		} else if ok {
			return value, "global config", nil
		}
	}
	return "", "", nil
}

func formatImageSource(imageID, source string) string {
	if imageID == "" || source == "" {
		return ""
	}
	return fmt.Sprintf("dbms.image=%s (source: %s)\n", imageID, source)
}

func normalizePsqlArgs(args []string, workspaceRoot string, cwd string, stdin io.Reader, convert func(string) (string, error)) ([]string, *string, error) {
	normalized, stdinValue, err := inputpsql.NormalizeArgs(
		args,
		inputset.NewWorkspaceResolver(workspaceRoot, cwd, convert),
		stdin,
	)
	if err != nil {
		return nil, nil, wrapInputsetError(err)
	}
	return normalized, stdinValue, nil
}

func normalizeFilePath(path string, workspaceRoot string, cwd string, convert func(string) (string, error)) (string, bool, error) {
	if path == "-" {
		return path, true, nil
	}
	resolved, err := inputset.NewWorkspaceResolver(workspaceRoot, cwd, convert).ResolvePath(path)
	if err != nil {
		return "", false, wrapInputsetError(err)
	}
	return resolved, false, nil
}

func rebasePathToWorkspaceRoot(path string, workspaceRoot string) string {
	rawPath := strings.TrimSpace(path)
	rawRoot := strings.TrimSpace(workspaceRoot)
	if rawPath == "" || rawRoot == "" {
		return rawPath
	}
	cleanedPath := filepath.Clean(rawPath)
	cleanedRoot := filepath.Clean(rawRoot)

	canonicalRoot := canonicalizeBoundaryPath(cleanedRoot)
	canonicalPath := canonicalizeBoundaryPath(cleanedPath)
	if canonicalRoot == "" || canonicalPath == "" || !isWithin(canonicalRoot, canonicalPath) {
		return cleanedPath
	}

	rel, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil {
		return cleanedPath
	}
	if rel == "." {
		return cleanedRoot
	}
	return filepath.Clean(filepath.Join(cleanedRoot, rel))
}

func canonicalizeBoundaryPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return pathutil.CanonicalizeBoundaryPath(trimmed)
}

// buildPathConverter scopes Windows-to-WSL path translation to local-engine
// execution. Remote requests retain host-native absolute paths so their
// arguments and source workspace context share the coordinate system required
// by docs/architecture/remote-source-input-sync-flow.md.
func buildPathConverter(opts cli.PrepareOptions) func(string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(opts.Mode), "remote") {
		return nil
	}
	if opts.WSLDistro == "" {
		return nil
	}
	if runtime.GOOS != "windows" {
		return nil
	}
	return windowsToWSLPath
}

func normalizeLiquibaseArgs(args []string, workspaceRoot string, cwd string, convert func(string) (string, error)) ([]string, error) {
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--changelog-file=") && strings.TrimSpace(strings.TrimPrefix(arg, "--changelog-file=")) == "":
			return nil, ExitErrorf(2, "Missing value for --changelog-file")
		case strings.HasPrefix(arg, "--defaults-file=") && strings.TrimSpace(strings.TrimPrefix(arg, "--defaults-file=")) == "":
			return nil, ExitErrorf(2, "Missing value for --defaults-file")
		case strings.HasPrefix(arg, "--searchPath=") && strings.TrimSpace(strings.TrimPrefix(arg, "--searchPath=")) == "":
			return nil, ExitErrorf(2, "Missing value for --searchPath")
		case strings.HasPrefix(arg, "--search-path=") && strings.TrimSpace(strings.TrimPrefix(arg, "--search-path=")) == "":
			return nil, ExitErrorf(2, "Missing value for --search-path")
		}
	}

	normalized, err := inputliquibase.NormalizeArgs(
		args,
		inputset.NewWorkspaceResolver(workspaceRoot, cwd, convert),
		true,
	)
	if err != nil {
		return nil, wrapInputsetError(err)
	}
	return normalized, nil
}

func normalizeWorkDir(cwd string, convert func(string) (string, error)) (string, error) {
	cleaned := strings.TrimSpace(cwd)
	if cleaned == "" {
		return "", nil
	}
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("path is not absolute: %s", cleaned)
	}
	if convert != nil {
		converted, err := convert(cleaned)
		if err != nil {
			return "", err
		}
		return converted, nil
	}
	return cleaned, nil
}

func deriveLiquibaseWorkDirFromArgs(args []string, fallback string) string {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--searchPath" || arg == "--search-path":
			if i+1 < len(args) {
				if dir := firstLiquibaseSearchPathDir(args[i+1]); dir != "" {
					return dir
				}
				i++
			}
		case strings.HasPrefix(arg, "--searchPath="):
			if dir := firstLiquibaseSearchPathDir(strings.TrimPrefix(arg, "--searchPath=")); dir != "" {
				return dir
			}
		case strings.HasPrefix(arg, "--search-path="):
			if dir := firstLiquibaseSearchPathDir(strings.TrimPrefix(arg, "--search-path=")); dir != "" {
				return dir
			}
		}
	}
	return fallback
}

func firstLiquibaseSearchPathDir(value string) string {
	for _, part := range strings.Split(value, ",") {
		item := strings.TrimSpace(part)
		if item == "" || inputset.LooksLikeLiquibaseRemoteRef(item) {
			continue
		}
		return filepath.Clean(item)
	}
	return ""
}

func rewriteLiquibasePathArg(flag string, value string, workspaceRoot string, cwd string, convert func(string) (string, error)) (string, error) {
	normalized, err := inputliquibase.NormalizeArgs(
		[]string{flag, value},
		inputset.NewWorkspaceResolver(workspaceRoot, cwd, convert),
		false,
	)
	if err != nil {
		return "", wrapInputsetError(err)
	}
	if len(normalized) < 2 {
		return "", ExitErrorf(2, "Missing value for %s", flag)
	}
	return normalized[1], nil
}

func relativizeLiquibaseArgs(args []string, workspaceRoot string, cwd string) []string {
	base := strings.TrimSpace(cwd)
	if base == "" {
		base = strings.TrimSpace(workspaceRoot)
	}
	if strings.TrimSpace(base) == "" {
		return args
	}
	normalized := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file":
			if i+1 >= len(args) {
				normalized = append(normalized, arg)
				continue
			}
			value := args[i+1]
			rel := toRelativeIfWithin(base, value)
			normalized = append(normalized, arg, rel)
			i++
		case strings.HasPrefix(arg, "--changelog-file="):
			value := strings.TrimPrefix(arg, "--changelog-file=")
			rel := toRelativeIfWithin(base, value)
			normalized = append(normalized, "--changelog-file="+rel)
		case strings.HasPrefix(arg, "--defaults-file="):
			value := strings.TrimPrefix(arg, "--defaults-file=")
			rel := toRelativeIfWithin(base, value)
			normalized = append(normalized, "--defaults-file="+rel)
		default:
			normalized = append(normalized, arg)
		}
	}
	return normalized
}

func toRelativeIfWithin(base string, value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	if !filepath.IsAbs(value) {
		return value
	}
	rel, err := filepath.Rel(base, value)
	if err != nil {
		return value
	}
	if rel == "." {
		return "."
	}
	if strings.HasPrefix(rel, "..") {
		return value
	}
	return rel
}

func looksLikeLiquibaseRemoteRef(value string) bool {
	return inputset.LooksLikeLiquibaseRemoteRef(value)
}

func wrapInputsetError(err error) error {
	if err == nil {
		return nil
	}
	var userErr *inputset.UserError
	if errors.As(err, &userErr) {
		return ExitErrorf(2, "%s", userErr.Message)
	}
	return err
}
