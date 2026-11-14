package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/inputset"
	inputliquibase "github.com/sqlrs/cli/internal/inputset/liquibase"
	inputpsql "github.com/sqlrs/cli/internal/inputset/psql"
	"github.com/sqlrs/cli/internal/refctx"
)

type prepareStageBinding struct {
	PsqlArgs      []string
	Stdin         *string
	LiquibaseArgs []string
	WorkDir       string
	cleanup       func() error
}

func bindPreparePsqlInputs(runOpts cli.PrepareOptions, workspaceRoot string, cwd string, parsed prepareArgs, existing *refctx.Context, stdin io.Reader) (binding prepareStageBinding, err error) {
	ctx, cleanup, err := resolvePrepareBindingContext(workspaceRoot, cwd, parsed, existing)
	if err != nil {
		return prepareStageBinding{}, err
	}
	cleanupCurrent := cleanup
	defer func() {
		if err != nil && cleanupCurrent != nil {
			err = combineBindingCleanupError(err, cleanupCurrent())
		}
	}()

	converter := buildPathConverter(runOpts)
	if ctx == nil || ctx.RefMode != "blob" {
		boundaryRoot := workspaceRoot
		baseDir := cwd
		if ctx != nil {
			boundaryRoot = ctx.WorkspaceRoot
			baseDir = ctx.BaseDir
		}
		args, stdinValue, err := normalizePsqlArgs(parsed.PsqlArgs, boundaryRoot, baseDir, stdin, converter)
		if err != nil {
			return prepareStageBinding{}, err
		}
		return prepareStageBinding{
			PsqlArgs: args,
			Stdin:    stdinValue,
			cleanup:  cleanupCurrent,
		}, nil
	}

	resolver := inputset.NewWorkspaceResolver(ctx.WorkspaceRoot, ctx.BaseDir, nil)
	args, stdinValue, err := inputpsql.NormalizeArgs(parsed.PsqlArgs, resolver, stdin)
	if err != nil {
		return prepareStageBinding{}, wrapInputsetError(err)
	}
	if !psqlHasFileArgs(args) {
		args, err = convertPsqlFileArgs(args, converter)
		if err != nil {
			return prepareStageBinding{}, err
		}
		return prepareStageBinding{
			PsqlArgs: args,
			Stdin:    stdinValue,
			cleanup:  cleanup,
		}, nil
	}

	collected, err := inputpsql.Collect(args, resolver, ctx.FileSystem)
	if err != nil {
		return prepareStageBinding{}, wrapInputsetError(err)
	}
	stageRoot, err := materializeRefFiles(resolver.Root, entryAbsPaths(collected), nil, ctx.FileSystem)
	if err != nil {
		return prepareStageBinding{}, err
	}
	cleanupCurrent = joinCleanup(func() error { return os.RemoveAll(stageRoot) }, cleanup)
	args = rewritePsqlFileArgsToRoot(args, resolver.Root, stageRoot)
	args, err = convertPsqlFileArgs(args, converter)
	if err != nil {
		return prepareStageBinding{}, err
	}

	return prepareStageBinding{
		PsqlArgs: args,
		Stdin:    stdinValue,
		cleanup:  cleanupCurrent,
	}, nil
}

func bindPrepareLiquibaseInputs(runOpts cli.PrepareOptions, workspaceRoot string, cwd string, parsed prepareArgs, existing *refctx.Context, relativizePaths bool) (binding prepareStageBinding, err error) {
	ctx, cleanup, err := resolvePrepareBindingContext(workspaceRoot, cwd, parsed, existing)
	if err != nil {
		return prepareStageBinding{}, err
	}
	cleanupCurrent := cleanup
	defer func() {
		if err != nil && cleanupCurrent != nil {
			err = combineBindingCleanupError(err, cleanupCurrent())
		}
	}()

	converter := buildPathConverter(runOpts)
	if ctx == nil || ctx.RefMode != "blob" {
		boundaryRoot := workspaceRoot
		baseDir := cwd
		if ctx != nil {
			boundaryRoot = ctx.WorkspaceRoot
			baseDir = effectiveLiquibaseBindBaseDir(cwd, ctx, existing, parsed)
		}
		args, err := normalizeLiquibaseArgs(parsed.PsqlArgs, boundaryRoot, baseDir, converter)
		if err != nil {
			return prepareStageBinding{}, err
		}
		if relativizePaths {
			args = relativizeLiquibaseArgs(args, boundaryRoot, baseDir)
		}
		workDir, err := normalizeWorkDir(baseDir, converter)
		if err != nil {
			return prepareStageBinding{}, err
		}
		if !relativizePaths {
			workDir = deriveLiquibaseWorkDirFromArgs(args, workDir)
		}
		return prepareStageBinding{
			LiquibaseArgs: args,
			WorkDir:       workDir,
			cleanup:       cleanupCurrent,
		}, nil
	}

	baseDir := effectiveLiquibaseBindBaseDir(cwd, ctx, existing, parsed)
	resolver := inputset.NewWorkspaceResolver(ctx.WorkspaceRoot, baseDir, nil)
	args, err := inputliquibase.NormalizeArgs(parsed.PsqlArgs, resolver, true)
	if err != nil {
		return prepareStageBinding{}, wrapInputsetError(err)
	}
	stageRoot := ""
	stageBase := resolver.BaseDir
	if _, _, hasLocalPaths, err := liquibaseLocalArtifacts(args, resolver, ctx.FileSystem); err != nil {
		return prepareStageBinding{}, err
	} else if hasLocalPaths {
		// Liquibase may load secondary local assets during execution that are not
		// represented in include/includeAll traversal (for example sqlFile or
		// loadData). Stage the whole logical workspace root to preserve host-side
		// Liquibase semantics in blob mode.
		stageRoot, err = materializeRefTree(resolver.Root, ctx.FileSystem)
		if err != nil {
			return prepareStageBinding{}, err
		}
		stageBase = mapPathToStageRoot(resolver.Root, stageRoot, resolver.BaseDir)
		args = rewriteLiquibaseArgsToRoot(args, resolver.Root, stageRoot)
		cleanupCurrent = joinCleanup(func() error { return os.RemoveAll(stageRoot) }, cleanup)
	}

	if relativizePaths {
		relRoot := resolver.Root
		if stageRoot != "" {
			relRoot = stageRoot
		}
		args = relativizeLiquibaseArgs(args, relRoot, stageBase)
	} else {
		args, err = convertLiquibaseHostPaths(args, converter)
		if err != nil {
			return prepareStageBinding{}, err
		}
	}

	workDir, err := normalizeWorkDir(stageBase, converter)
	if err != nil {
		return prepareStageBinding{}, err
	}
	if !relativizePaths {
		workDir = deriveLiquibaseWorkDirFromArgs(args, workDir)
	}

	return prepareStageBinding{
		LiquibaseArgs: args,
		WorkDir:       workDir,
		cleanup:       cleanupCurrent,
	}, nil
}

func resolvePrepareBindingContext(workspaceRoot string, cwd string, parsed prepareArgs, existing *refctx.Context) (*refctx.Context, func() error, error) {
	if existing != nil {
		return existing, existing.Cleanup, nil
	}
	if strings.TrimSpace(parsed.Ref) == "" {
		return nil, nil, nil
	}
	ctx, err := refctx.Resolve(workspaceRoot, cwd, parsed.Ref, parsed.RefMode, parsed.RefKeepWorktree)
	if err != nil {
		return nil, nil, err
	}
	return &ctx, ctx.Cleanup, nil
}

// Raw Liquibase `--ref` commands must keep resolving relative paths from the
// projected current working directory, while alias-backed ref invocations use
// the already projected alias directory passed as `cwd`.
func effectiveLiquibaseBindBaseDir(cwd string, ctx *refctx.Context, existing *refctx.Context, parsed prepareArgs) string {
	if strings.TrimSpace(parsed.Ref) != "" {
		if ctx != nil && strings.TrimSpace(ctx.BaseDir) != "" {
			return ctx.BaseDir
		}
		return cwd
	}
	return effectiveRefBindBaseDir(cwd, ctx, existing)
}

func effectiveRefBindBaseDir(cwd string, ctx *refctx.Context, existing *refctx.Context) string {
	if existing != nil && strings.TrimSpace(cwd) != "" {
		return filepath.Clean(cwd)
	}
	if ctx != nil {
		return ctx.BaseDir
	}
	return cwd
}

func joinCleanup(funcs ...func() error) func() error {
	return func() error {
		var errs []string
		for _, fn := range funcs {
			if fn == nil {
				continue
			}
			if err := fn(); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if len(errs) > 0 {
			return ExitErrorf(1, "%s", strings.Join(errs, "; "))
		}
		return nil
	}
}

func combineBindingCleanupError(runErr error, cleanupErr error) error {
	if cleanupErr == nil {
		return runErr
	}
	if runErr == nil {
		return cleanupErr
	}
	return ExitErrorf(1, "%s; cleanup: %s", runErr.Error(), cleanupErr.Error())
}

func entryAbsPaths(set inputset.InputSet) []string {
	paths := make([]string, 0, len(set.Entries))
	for _, entry := range set.Entries {
		paths = append(paths, entry.AbsPath)
	}
	return paths
}

func psqlHasFileArgs(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-f" || arg == "--file":
			return i+1 < len(args) && strings.TrimSpace(args[i+1]) != "-" && strings.TrimSpace(args[i+1]) != ""
		case strings.HasPrefix(arg, "--file="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--file="))
			return value != "" && value != "-"
		case strings.HasPrefix(arg, "-f") && len(arg) > 2:
			value := strings.TrimSpace(arg[2:])
			return value != "" && value != "-"
		}
	}
	return false
}

func materializeRefFiles(logicalRoot string, files []string, dirs []string, fs inputset.FileSystem) (string, error) {
	stageRoot, err := os.MkdirTemp("", "sqlrs-ref-stage-*")
	if err != nil {
		return "", err
	}
	if err := materializeMappedDirs(logicalRoot, stageRoot, dirs); err != nil {
		_ = os.RemoveAll(stageRoot)
		return "", err
	}
	for _, path := range dedupePaths(files) {
		stagePath := mapPathToStageRoot(logicalRoot, stageRoot, path)
		if err := os.MkdirAll(filepath.Dir(stagePath), 0o700); err != nil {
			_ = os.RemoveAll(stageRoot)
			return "", err
		}
		data, err := fs.ReadFile(path)
		if err != nil {
			_ = os.RemoveAll(stageRoot)
			return "", err
		}
		if err := os.WriteFile(stagePath, data, 0o600); err != nil {
			_ = os.RemoveAll(stageRoot)
			return "", err
		}
	}
	return stageRoot, nil
}

func materializeMappedDirs(logicalRoot string, stageRoot string, dirs []string) error {
	for _, dir := range dedupePaths(dirs) {
		stageDir := mapPathToStageRoot(logicalRoot, stageRoot, dir)
		if err := os.MkdirAll(stageDir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func materializeRefTree(logicalRoot string, fs inputset.FileSystem) (string, error) {
	stageRoot, err := os.MkdirTemp("", "sqlrs-ref-stage-*")
	if err != nil {
		return "", err
	}
	if err := materializeRefTreeFromPath(logicalRoot, logicalRoot, stageRoot, fs); err != nil {
		_ = os.RemoveAll(stageRoot)
		return "", err
	}
	return stageRoot, nil
}

func materializeRefTreeFromPath(logicalRoot string, current string, stageRoot string, fs inputset.FileSystem) error {
	info, err := fs.Stat(current)
	if err != nil {
		return err
	}
	stagePath := mapPathToStageRoot(logicalRoot, stageRoot, current)
	if info.IsDir() {
		if err := os.MkdirAll(stagePath, 0o700); err != nil {
			return err
		}
		entries, err := fs.ReadDir(current)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := materializeRefTreeFromPath(logicalRoot, filepath.Join(current, entry.Name()), stageRoot, fs); err != nil {
				return err
			}
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(stagePath), 0o700); err != nil {
		return err
	}
	data, err := fs.ReadFile(current)
	if err != nil {
		return err
	}
	return os.WriteFile(stagePath, data, 0o600)
}

func mapPathToStageRoot(logicalRoot string, stageRoot string, value string) string {
	value = filepath.Clean(strings.TrimSpace(value))
	if stageRoot == "" || logicalRoot == "" || value == "" || !filepath.IsAbs(value) {
		return value
	}
	rel, err := filepath.Rel(filepath.Clean(logicalRoot), value)
	if err != nil {
		return value
	}
	if rel == "." {
		return stageRoot
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return value
	}
	return filepath.Join(stageRoot, rel)
}

func rewritePsqlFileArgsToRoot(args []string, logicalRoot string, stageRoot string) []string {
	rewritten := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-f" || arg == "--file":
			rewritten = append(rewritten, arg)
			if i+1 >= len(args) {
				continue
			}
			rewritten = append(rewritten, mapPathToStageRoot(logicalRoot, stageRoot, args[i+1]))
			i++
		case strings.HasPrefix(arg, "--file="):
			value := strings.TrimPrefix(arg, "--file=")
			rewritten = append(rewritten, "--file="+mapPathToStageRoot(logicalRoot, stageRoot, value))
		case strings.HasPrefix(arg, "-f") && len(arg) > 2:
			value := arg[2:]
			rewritten = append(rewritten, "-f"+mapPathToStageRoot(logicalRoot, stageRoot, value))
		default:
			rewritten = append(rewritten, arg)
		}
	}
	return rewritten
}

func convertPsqlFileArgs(args []string, convert func(string) (string, error)) ([]string, error) {
	if convert == nil {
		return args, nil
	}
	rewritten := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-f" || arg == "--file":
			rewritten = append(rewritten, arg)
			if i+1 >= len(args) {
				continue
			}
			value, err := convertDirectFileArg(args[i+1], convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, value)
			i++
		case strings.HasPrefix(arg, "--file="):
			value, err := convertDirectFileArg(strings.TrimPrefix(arg, "--file="), convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, "--file="+value)
		case strings.HasPrefix(arg, "-f") && len(arg) > 2:
			value, err := convertDirectFileArg(arg[2:], convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, "-f"+value)
		default:
			rewritten = append(rewritten, arg)
		}
	}
	return rewritten, nil
}

func convertDirectFileArg(value string, convert func(string) (string, error)) (string, error) {
	if value == "-" || strings.TrimSpace(value) == "" || !filepath.IsAbs(value) {
		return value, nil
	}
	return convert(value)
}

func liquibaseLocalArtifacts(args []string, resolver inputset.Resolver, fs inputset.FileSystem) ([]string, []string, bool, error) {
	files := make([]string, 0, 4)
	dirs := make([]string, 0, 2)
	changelog := ""

	appendSearchPath := func(value string) {
		for _, item := range strings.Split(value, ",") {
			cleaned := strings.TrimSpace(item)
			if cleaned == "" || inputset.LooksLikeLiquibaseRemoteRef(cleaned) {
				continue
			}
			dirs = append(dirs, cleaned)
		}
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file":
			if i+1 >= len(args) {
				continue
			}
			value := strings.TrimSpace(args[i+1])
			if value != "" && !inputset.LooksLikeLiquibaseRemoteRef(value) {
				files = append(files, value)
				if arg == "--changelog-file" {
					changelog = value
				}
			}
			i++
		case arg == "--searchPath" || arg == "--search-path":
			if i+1 >= len(args) {
				continue
			}
			appendSearchPath(args[i+1])
			i++
		case strings.HasPrefix(arg, "--changelog-file="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--changelog-file="))
			if value != "" && !inputset.LooksLikeLiquibaseRemoteRef(value) {
				files = append(files, value)
				changelog = value
			}
		case strings.HasPrefix(arg, "--defaults-file="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--defaults-file="))
			if value != "" && !inputset.LooksLikeLiquibaseRemoteRef(value) {
				files = append(files, value)
			}
		case strings.HasPrefix(arg, "--searchPath="):
			appendSearchPath(strings.TrimPrefix(arg, "--searchPath="))
		case strings.HasPrefix(arg, "--search-path="):
			appendSearchPath(strings.TrimPrefix(arg, "--search-path="))
		}
	}

	if changelog != "" {
		collected, err := inputliquibase.Collect(args, resolver, fs)
		if err != nil {
			return nil, nil, false, wrapInputsetError(err)
		}
		files = append(files, entryAbsPaths(collected)...)
	}
	return dedupePaths(files), dedupePaths(dirs), len(files) > 0 || len(dirs) > 0, nil
}

func rewriteLiquibaseArgsToRoot(args []string, logicalRoot string, stageRoot string) []string {
	rewritten := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file":
			rewritten = append(rewritten, arg)
			if i+1 >= len(args) {
				continue
			}
			value := args[i+1]
			if inputset.LooksLikeLiquibaseRemoteRef(value) {
				rewritten = append(rewritten, value)
			} else {
				rewritten = append(rewritten, mapPathToStageRoot(logicalRoot, stageRoot, value))
			}
			i++
		case arg == "--searchPath" || arg == "--search-path":
			rewritten = append(rewritten, arg)
			if i+1 >= len(args) {
				continue
			}
			rewritten = append(rewritten, rewriteLiquibaseSearchPathToRoot(args[i+1], logicalRoot, stageRoot))
			i++
		case strings.HasPrefix(arg, "--changelog-file="):
			value := strings.TrimPrefix(arg, "--changelog-file=")
			if inputset.LooksLikeLiquibaseRemoteRef(value) {
				rewritten = append(rewritten, arg)
			} else {
				rewritten = append(rewritten, "--changelog-file="+mapPathToStageRoot(logicalRoot, stageRoot, value))
			}
		case strings.HasPrefix(arg, "--defaults-file="):
			value := strings.TrimPrefix(arg, "--defaults-file=")
			if inputset.LooksLikeLiquibaseRemoteRef(value) {
				rewritten = append(rewritten, arg)
			} else {
				rewritten = append(rewritten, "--defaults-file="+mapPathToStageRoot(logicalRoot, stageRoot, value))
			}
		case strings.HasPrefix(arg, "--searchPath="):
			value := strings.TrimPrefix(arg, "--searchPath=")
			rewritten = append(rewritten, "--searchPath="+rewriteLiquibaseSearchPathToRoot(value, logicalRoot, stageRoot))
		case strings.HasPrefix(arg, "--search-path="):
			value := strings.TrimPrefix(arg, "--search-path=")
			rewritten = append(rewritten, "--search-path="+rewriteLiquibaseSearchPathToRoot(value, logicalRoot, stageRoot))
		default:
			rewritten = append(rewritten, arg)
		}
	}
	return rewritten
}

func rewriteLiquibaseSearchPathToRoot(value string, logicalRoot string, stageRoot string) string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		cleaned := strings.TrimSpace(item)
		switch {
		case cleaned == "":
			out = append(out, cleaned)
		case inputset.LooksLikeLiquibaseRemoteRef(cleaned):
			out = append(out, cleaned)
		default:
			out = append(out, mapPathToStageRoot(logicalRoot, stageRoot, cleaned))
		}
	}
	return strings.Join(out, ",")
}

func convertLiquibaseHostPaths(args []string, convert func(string) (string, error)) ([]string, error) {
	if convert == nil {
		return args, nil
	}
	rewritten := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file":
			rewritten = append(rewritten, arg)
			if i+1 >= len(args) {
				continue
			}
			value, err := convertLiquibaseValue(args[i+1], convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, value)
			i++
		case arg == "--searchPath" || arg == "--search-path":
			rewritten = append(rewritten, arg)
			if i+1 >= len(args) {
				continue
			}
			value, err := convertLiquibaseSearchPath(args[i+1], convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, value)
			i++
		case strings.HasPrefix(arg, "--changelog-file="):
			value, err := convertLiquibaseValue(strings.TrimPrefix(arg, "--changelog-file="), convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, "--changelog-file="+value)
		case strings.HasPrefix(arg, "--defaults-file="):
			value, err := convertLiquibaseValue(strings.TrimPrefix(arg, "--defaults-file="), convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, "--defaults-file="+value)
		case strings.HasPrefix(arg, "--searchPath="):
			value, err := convertLiquibaseSearchPath(strings.TrimPrefix(arg, "--searchPath="), convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, "--searchPath="+value)
		case strings.HasPrefix(arg, "--search-path="):
			value, err := convertLiquibaseSearchPath(strings.TrimPrefix(arg, "--search-path="), convert)
			if err != nil {
				return nil, err
			}
			rewritten = append(rewritten, "--search-path="+value)
		default:
			rewritten = append(rewritten, arg)
		}
	}
	return rewritten, nil
}

func convertLiquibaseValue(value string, convert func(string) (string, error)) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || inputset.LooksLikeLiquibaseRemoteRef(value) || !filepath.IsAbs(value) {
		return value, nil
	}
	return convert(value)
}

func convertLiquibaseSearchPath(value string, convert func(string) (string, error)) (string, error) {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		cleaned := strings.TrimSpace(item)
		if cleaned == "" || inputset.LooksLikeLiquibaseRemoteRef(cleaned) || !filepath.IsAbs(cleaned) {
			out = append(out, cleaned)
			continue
		}
		converted, err := convert(cleaned)
		if err != nil {
			return "", err
		}
		out = append(out, converted)
	}
	return strings.Join(out, ","), nil
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := filepath.Clean(strings.TrimSpace(path))
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}
	return out
}
