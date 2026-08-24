package prepare

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/statefs"
	"github.com/sqlrs/engine-local/internal/store"
)

type PsqlRunRequest struct {
	Args    []string
	Env     map[string]string
	Stdin   *string
	WorkDir string
}

type LiquibaseRunRequest struct {
	ExecPath string
	ExecMode string
	ImageID  string
	Args     []string
	Env      map[string]string
	WorkDir  string
	Mounts   []engineRuntime.Mount
	Network  string
}

type psqlRunner interface {
	Run(ctx context.Context, instance engineRuntime.Instance, req PsqlRunRequest) (string, error)
}

type liquibaseRunner interface {
	Run(ctx context.Context, req LiquibaseRunRequest) (string, error)
}

var listNetInterfaces = net.Interfaces
var ifaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

type containerPsqlRunner struct {
	runtime engineRuntime.Runtime
}

type containerLiquibaseRunner struct {
	runtime engineRuntime.Runtime
}

type containerRunner interface {
	RunContainer(ctx context.Context, req engineRuntime.RunRequest) (string, error)
}

func (r containerPsqlRunner) Run(ctx context.Context, instance engineRuntime.Instance, req PsqlRunRequest) (string, error) {
	if r.runtime == nil {
		return "", fmt.Errorf("runtime is required")
	}
	return r.runtime.Exec(ctx, instance.ID, engineRuntime.ExecRequest{
		User:  "postgres",
		Args:  req.Args,
		Env:   req.Env,
		Dir:   req.WorkDir,
		Stdin: req.Stdin,
	})
}

func (r containerLiquibaseRunner) Run(ctx context.Context, req LiquibaseRunRequest) (string, error) {
	if r.runtime == nil {
		return "", fmt.Errorf("runtime is required")
	}
	runner, ok := r.runtime.(containerRunner)
	if !ok {
		return "", fmt.Errorf("runtime does not support container runs")
	}
	return runner.RunContainer(ctx, engineRuntime.RunRequest{
		ImageID: req.ImageID,
		Args:    req.Args,
		Env:     req.Env,
		Dir:     req.WorkDir,
		Mounts:  req.Mounts,
		Network: req.Network,
	})
}

type statePaths struct {
	root      string
	engine    string
	version   string
	baseDir   string
	statesDir string
	stateDir  string
}

var (
	postgresDataDirRoot = engineRuntime.PostgresDataDirRoot
	postgresDataDir     = engineRuntime.PostgresDataDir
)

func resolveStatePaths(root string, imageID string, stateID string, fs statefs.StateFS) (statePaths, error) {
	if strings.TrimSpace(root) == "" {
		return statePaths{}, fmt.Errorf("state store root is required")
	}
	if fs == nil {
		return statePaths{}, fmt.Errorf("statefs is required")
	}
	baseDir, err := fs.BaseDir(root, imageID)
	if err != nil {
		return statePaths{}, err
	}
	statesDir, err := fs.StatesDir(root, imageID)
	if err != nil {
		return statePaths{}, err
	}
	stateDir := ""
	if strings.TrimSpace(stateID) != "" {
		stateDir, err = fs.StateDir(root, imageID, stateID)
		if err != nil {
			return statePaths{}, err
		}
	}
	return statePaths{
		root:      root,
		engine:    filepath.Base(filepath.Dir(filepath.Dir(baseDir))),
		version:   filepath.Base(filepath.Dir(baseDir)),
		baseDir:   baseDir,
		statesDir: statesDir,
		stateDir:  stateDir,
	}, nil
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	out := append([]string{}, base...)
	index := map[string]int{}
	for i, entry := range out {
		key := entry
		if idx := strings.Index(entry, "="); idx != -1 {
			key = entry[:idx]
		}
		index[normalizeEnvKey(key)] = i
	}
	for key, value := range overrides {
		entry := key + "=" + value
		if idx, ok := index[normalizeEnvKey(key)]; ok {
			out[idx] = entry
			continue
		}
		out = append(out, entry)
	}
	return out
}

func normalizeEnvKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

func (e *taskExecutor) executeStateTask(ctx context.Context, jobID string, prepared preparedRequest, task taskState) (string, *ErrorResponse) {
	m := e.m
	if ctx.Err() != nil {
		return "", errorResponse("cancelled", "task cancelled", "")
	}
	if task.Input == nil {
		return "", errorResponse("internal_error", "task input is required", "")
	}
	outputStateID := task.OutputStateID
	taskHash := task.TaskHash

	runner, ephemeral := m.runnerForJob(jobID)
	if runner == nil {
		return "", errorResponse("internal_error", "job runner missing", "")
	}
	if ephemeral {
		defer m.cleanupRuntime(context.Background(), runner)
	}

	var contentLocker *contentLock
	var rt *jobRuntime

	if prepared.request.PrepareKind == "psql" {
		step, err := psqlStepForPreparedTask(prepared, task.TaskID)
		if err != nil {
			return "", errorResponse("internal_error", "cannot resolve psql step", err.Error())
		}
		lock := &contentLock{files: map[string]*os.File{}}
		digest, err := computePsqlContentDigestWithLock(step.inputs, prepared.psqlWorkDir, lock)
		if err != nil {
			_ = lock.Close()
			return "", errorResponse("invalid_argument", "cannot compute psql content hash", err.Error())
		}
		taskHash = psqlTaskHash(prepared.request.PrepareKind, digest.hash, m.version)
		contentLocker = lock
	}

	if prepared.request.PrepareKind == "lb" {
		// Liquibase task hash is precomputed during planning and must remain stable
		// across execution retries to allow cache hits. Recompute only when missing
		// to support recovery of legacy queued tasks without persisted hashes.
		if strings.TrimSpace(taskHash) == "" {
			planned, errResp := e.ensureRuntime(ctx, jobID, prepared, task.Input, runner)
			if errResp != nil {
				return "", errResp
			}
			rt = planned
			lock, errResp := ensureLiquibaseContentLock(prepared, task.ChangesetPath)
			if errResp != nil {
				return "", errResp
			}
			contentLocker = lock
			changesets, errResp := e.runLiquibaseUpdateSQL(ctx, jobID, prepared, rt)
			if errResp != nil {
				_ = lock.Close()
				return "", errResp
			}
			if len(changesets) == 0 {
				_ = lock.Close()
				return "", errorResponse("internal_error", "liquibase returned no pending changesets", "")
			}
			parentFingerprintID := ""
			if task.Input != nil {
				parentFingerprintID = task.Input.ID
			}
			taskHash = liquibaseFingerprint(strings.TrimSpace(parentFingerprintID), []LiquibaseChangeset{changesets[0]})
		}
	}

	if contentLocker != nil {
		defer contentLocker.Close()
	}

	if taskHash != "" {
		if output, errResp := m.computeOutputStateID(task.Input.Kind, task.Input.ID, taskHash); errResp == nil {
			outputStateID = output
		}
	}

	cached, err := m.isStateCached(outputStateID)
	if err != nil {
		return "", errorResponse("internal_error", "cannot check state cache", err.Error())
	}
	forceRebuild := false
	m.logInfoJob(jobID, "state cache decision task=%s input_kind=%s input_id=%s task_hash=%s output_state=%s cached=%t",
		task.TaskID,
		task.Input.Kind,
		task.Input.ID,
		taskHash,
		outputStateID,
		cached,
	)
	cachedFlag := cached
	if outputStateID != task.OutputStateID || task.TaskHash != taskHash || (task.Cached == nil || *task.Cached != cachedFlag) {
		update := queue.TaskUpdate{
			TaskHash:      nullableString(taskHash),
			OutputStateID: nullableString(outputStateID),
			Cached:        &cachedFlag,
		}
		_ = m.queue.UpdateTask(ctx, jobID, task.TaskID, update)
		task.OutputStateID = outputStateID
		task.TaskHash = taskHash
		task.Cached = &cachedFlag
	}
	if cached {
		m.logTask(jobID, task.TaskID, "cached output_state=%s", outputStateID)
		e.backfillCachedStateSizeIfMissing(ctx, jobID, outputStateID)
		// Cached states are trusted after marker/metadata checks above; defer
		// runtime startup to prepare-instance to avoid per-step container churn.
		if runner.getRuntime() != nil {
			m.cleanupRuntime(context.Background(), runner)
			rt = nil
			m.logInfoJob(jobID, "cached runtime released state=%s", outputStateID)
		}
		return outputStateID, nil
	}
	if capErr := m.ensureCacheCapacity(ctx, jobID, "prepare_step"); capErr != nil {
		return "", capErr
	}

	imageID := prepared.effectiveImageID()
	if strings.TrimSpace(imageID) == "" {
		return "", errorResponse("internal_error", "resolved image id is required", "")
	}
	paths, err := resolveStatePaths(m.stateStoreRoot, imageID, outputStateID, m.statefs)
	if err != nil {
		return "", errorResponse("internal_error", "cannot resolve state paths", err.Error())
	}
	if err := os.MkdirAll(paths.statesDir, 0o700); err != nil {
		if noSpaceResp := noSpaceErrorResponse("insufficient storage before state execution", "prepare_step", err); noSpaceResp != nil {
			return "", noSpaceResp
		}
		return "", errorResponse("internal_error", "cannot create state dir", err.Error())
	}
	if err := m.statefs.EnsureStateDir(ctx, paths.stateDir); err != nil {
		if noSpaceResp := noSpaceErrorResponse("insufficient storage before state execution", "prepare_step", err); noSpaceResp != nil {
			return "", noSpaceResp
		}
		return "", errorResponse("internal_error", "cannot create state dir", err.Error())
	}

	var errResp *ErrorResponse
	kind := snapshotKind(m.statefs)
	lockPath := stateBuildLockPath(paths.stateDir, kind)
	lockErr := withStateBuildLock(ctx, paths.stateDir, lockPath, kind, func() error {
		if !forceRebuild {
			cached, err := m.isStateCached(outputStateID)
			if err != nil {
				errResp = errorResponse("internal_error", "cannot check state cache", err.Error())
				return errStateBuildFailed
			}
			if cached {
				m.logTask(jobID, task.TaskID, "cached output_state=%s", outputStateID)
				e.backfillCachedStateSizeIfMissing(ctx, jobID, outputStateID)
				return nil
			}
		}
		if forceRebuild || kind == "btrfs" || stateBuildMarkerExists(paths.stateDir, kind) {
			if err := resetStateDir(ctx, m.statefs, paths.stateDir); err != nil {
				errResp = errorResponse("internal_error", "cannot reset state dir", err.Error())
				return errStateBuildFailed
			}
		}

		if rt == nil {
			planned, innerResp := e.ensureRuntime(ctx, jobID, prepared, task.Input, runner)
			if innerResp != nil {
				errResp = innerResp
				return errStateBuildFailed
			}
			rt = planned
		}
		if execErr := e.executePrepareStep(ctx, jobID, prepared, rt, task); execErr != nil {
			if noSpaceResp := noSpaceFromErrorResponse("prepare step failed due to insufficient storage", "prepare_step", execErr); noSpaceResp != nil {
				errResp = noSpaceResp
			} else {
				errResp = execErr
			}
			return errStateBuildFailed
		}
		if capErr := m.ensureCacheCapacity(ctx, jobID, "snapshot"); capErr != nil {
			errResp = capErr
			return errStateBuildFailed
		}

		m.appendLog(jobID, "pg_ctl: stop for snapshot")
		pgCtx := engineRuntime.WithLogSink(ctx, func(line string) {
			m.appendLog(jobID, "pg_ctl: "+line)
		})
		if err := m.dbms.PrepareSnapshot(pgCtx, rt.instance); err != nil {
			if noSpaceResp := noSpaceErrorResponse("insufficient storage during snapshot", "snapshot", err); noSpaceResp != nil {
				errResp = noSpaceResp
			} else {
				errResp = errorResponse("internal_error", "snapshot prepare failed", err.Error())
			}
			return errStateBuildFailed
		}
		resumed := false
		defer func() {
			if resumed {
				return
			}
			_ = m.dbms.ResumeSnapshot(context.Background(), rt.instance)
		}()

		m.appendLog(jobID, "snapshot: start")
		m.logInfoJob(jobID, "snapshot start dir=%s", paths.stateDir)
		if err := m.statefs.Snapshot(ctx, rt.dataDir, paths.stateDir); err != nil {
			if noSpaceResp := noSpaceErrorResponse("insufficient storage during snapshot", "snapshot", err); noSpaceResp != nil {
				errResp = noSpaceResp
			} else {
				errResp = errorResponse("internal_error", "snapshot failed", err.Error())
			}
			return errStateBuildFailed
		}
		m.appendLog(jobID, "snapshot: complete")
		m.logInfoJob(jobID, "snapshot complete dir=%s", paths.stateDir)
		m.appendLog(jobID, "pg_ctl: start after snapshot")
		pgResumeCtx := engineRuntime.WithLogSink(ctx, func(line string) {
			m.appendLog(jobID, "pg_ctl: "+line)
		})
		if err := m.dbms.ResumeSnapshot(pgResumeCtx, rt.instance); err != nil {
			if noSpaceResp := noSpaceErrorResponse("insufficient storage during snapshot", "snapshot", err); noSpaceResp != nil {
				errResp = noSpaceResp
			} else {
				errResp = errorResponse("internal_error", "snapshot resume failed", err.Error())
			}
			return errStateBuildFailed
		}
		resumed = true

		parentID := parentStateID(task.Input)
		createdAt := m.now().UTC().Format(time.RFC3339Nano)
		stateSize, measureErr := storeUsageFn(paths.stateDir)
		if measureErr != nil {
			errResp = capacityError("internal_error", "cannot measure state size", map[string]any{
				"phase": "metadata_commit",
				"error": measureErr.Error(),
			})
			return errStateBuildFailed
		}
		if stateSize < 0 {
			stateSize = 0
		}
		entry := store.StateCreate{
			StateID:               outputStateID,
			ParentStateID:         parentID,
			StateFingerprint:      outputStateID,
			ImageID:               imageID,
			PrepareKind:           prepared.request.PrepareKind,
			PrepareArgsNormalized: prepared.argsNormalized,
			CreatedAt:             createdAt,
			SizeBytes:             &stateSize,
		}
		if err := m.store.CreateState(ctx, entry); err != nil {
			if ctx.Err() != nil {
				errResp = errorResponse("cancelled", "task cancelled", "")
				return errStateBuildFailed
			}
			_ = m.statefs.RemovePath(context.Background(), paths.stateDir)
			if noSpaceResp := noSpaceErrorResponse("insufficient storage during metadata commit", "metadata_commit", err); noSpaceResp != nil {
				errResp = noSpaceResp
			} else {
				errResp = errorResponse("internal_error", "cannot store state", err.Error())
			}
			return errStateBuildFailed
		}
		if err := writeStateBuildMarker(paths.stateDir, kind); err != nil {
			if noSpaceResp := noSpaceErrorResponse("insufficient storage during metadata commit", "metadata_commit", err); noSpaceResp != nil {
				errResp = noSpaceResp
			} else {
				errResp = errorResponse("internal_error", "cannot write state marker", err.Error())
			}
			return errStateBuildFailed
		}
		if capErr := m.ensureCacheCapacity(ctx, jobID, "metadata_commit", outputStateID); capErr != nil {
			errResp = capErr
			return errStateBuildFailed
		}
		return nil
	})
	if errResp != nil {
		return "", errResp
	}
	if lockErr != nil {
		if ctx.Err() != nil {
			return "", errorResponse("cancelled", "task cancelled", "")
		}
		return "", errorResponse("internal_error", "cannot acquire state build lock", lockErr.Error())
	}
	return outputStateID, nil
}

func (e *taskExecutor) backfillCachedStateSizeIfMissing(ctx context.Context, jobID string, stateID string) {
	m := e.m
	entry, ok, err := m.store.GetState(ctx, stateID)
	if err != nil {
		m.logInfoJob(jobID, "cached state size backfill skipped state=%s reason=get_state_failed err=%v", stateID, err)
		return
	}
	if !ok || strings.TrimSpace(entry.ImageID) == "" {
		return
	}
	if entry.SizeBytes != nil && *entry.SizeBytes > 0 {
		return
	}
	paths, err := resolveStatePaths(m.stateStoreRoot, entry.ImageID, stateID, m.statefs)
	if err != nil {
		m.logInfoJob(jobID, "cached state size backfill skipped state=%s reason=resolve_paths_failed err=%v", stateID, err)
		return
	}
	sizeBytes, err := storeUsageFn(paths.stateDir)
	if err != nil {
		m.logInfoJob(jobID, "cached state size backfill skipped state=%s reason=measure_failed err=%v", stateID, err)
		return
	}
	if sizeBytes < 0 {
		sizeBytes = 0
	}
	if err := m.store.UpdateStateSize(ctx, stateID, sizeBytes); err != nil {
		m.logInfoJob(jobID, "cached state size backfill skipped state=%s reason=store_update_failed err=%v", stateID, err)
		return
	}
}

func (e *taskExecutor) executePrepareStep(ctx context.Context, jobID string, prepared preparedRequest, rt *jobRuntime, task taskState) *ErrorResponse {
	switch prepared.request.PrepareKind {
	case "psql":
		return e.executePsqlStep(ctx, jobID, prepared, rt, task)
	case "lb":
		return e.executeLiquibaseStep(ctx, jobID, prepared, rt, task)
	default:
		return errorResponse("internal_error", "unsupported prepare kind", prepared.request.PrepareKind)
	}
}

func noSpaceFromErrorResponse(message string, phase string, errResp *ErrorResponse) *ErrorResponse {
	if errResp == nil {
		return nil
	}
	if errResp.Code != "internal_error" {
		return nil
	}
	if noSpaceResp := noSpaceErrorResponse(message, phase, errors.New(errResp.Details)); noSpaceResp != nil {
		return noSpaceResp
	}
	return noSpaceErrorResponse(message, phase, errors.New(errResp.Message))
}

func (e *taskExecutor) executePsqlStep(ctx context.Context, jobID string, prepared preparedRequest, rt *jobRuntime, task taskState) *ErrorResponse {
	m := e.m
	step, err := psqlStepForPreparedTask(prepared, task.TaskID)
	if err != nil {
		return errorResponse("internal_error", "cannot resolve psql step", err.Error())
	}
	psqlArgs, workdir, err := buildPsqlExecArgs(step.args, rt.scriptMount)
	if err != nil {
		return errorResponse("internal_error", "cannot prepare psql arguments", err.Error())
	}
	if m.psql == nil {
		return errorResponse("internal_error", "psql runner is required", "")
	}
	m.appendLog(jobID, "psql: start")
	var sinkCalled atomic.Bool
	psqlCtx := engineRuntime.WithLogSink(ctx, func(line string) {
		sinkCalled.Store(true)
		m.appendLog(jobID, "psql: "+line)
	})
	output, err := m.psql.Run(psqlCtx, rt.instance, PsqlRunRequest{
		Args:    psqlArgs,
		Env:     map[string]string{},
		Stdin:   step.stdin,
		WorkDir: workdir,
	})
	if !sinkCalled.Load() && strings.TrimSpace(output) != "" {
		m.appendLogLines(jobID, "psql", output)
	}
	if err != nil {
		if ctx.Err() != nil {
			return errorResponse("cancelled", "task cancelled", "")
		}
		details := strings.TrimSpace(err.Error())
		if renderedOutput := strings.TrimSpace(output); renderedOutput != "" {
			details += "\n" + renderedOutput
		}
		if noSpaceResp := noSpaceErrorResponse("prepare step failed due to insufficient storage", "prepare_step", errors.New(details)); noSpaceResp != nil {
			return noSpaceResp
		}
		if noSpaceResp := noSpaceErrorResponse("prepare step failed due to insufficient storage", "prepare_step", err); noSpaceResp != nil {
			return noSpaceResp
		}
		return errorResponse("internal_error", "psql execution failed", details)
	}
	if ctx.Err() != nil {
		return errorResponse("cancelled", "task cancelled", "")
	}
	return nil
}

func (e *taskExecutor) executeLiquibaseStep(ctx context.Context, jobID string, prepared preparedRequest, rt *jobRuntime, task taskState) *ErrorResponse {
	m := e.m
	if m.liquibase == nil {
		return errorResponse("internal_error", "liquibase runner is required", "")
	}
	if strings.TrimSpace(rt.instance.Host) == "" || rt.instance.Port == 0 {
		return errorResponse("internal_error", "runtime instance is missing connection info", "")
	}
	execMode := normalizeExecMode(prepared.request.LiquibaseExecMode)
	rawExecPath := strings.TrimSpace(prepared.request.LiquibaseExec)
	windowsMode := shouldUseWindowsBat(rawExecPath, execMode)
	execPath, err := normalizeLiquibaseExecPath(rawExecPath, windowsMode)
	if err != nil {
		return errorResponse("internal_error", "cannot resolve liquibase executable", err.Error())
	}
	var mapper PathMapper
	if windowsMode && isWSL() {
		mapper = wslPathMapper{}
	}
	args, err := mapLiquibaseArgs(prepared.normalizedArgs, mapper)
	if err != nil {
		return errorResponse("internal_error", "cannot map liquibase arguments", err.Error())
	}
	workDir := strings.TrimSpace(prepared.request.WorkDir)
	if windowsMode && workDir == "" {
		workDir = deriveLiquibaseWorkDir(args)
	}
	if workDir != "" && mapper != nil {
		mappedDir, mapErr := mapper.MapPath(workDir)
		if mapErr != nil {
			return errorResponse("internal_error", "cannot map liquibase workdir", mapErr.Error())
		}
		workDir = mappedDir
	}
	if !windowsMode {
		args = relativizeLiquibaseHostFileArgs(args, workDir)
	}
	args = applyLiquibaseTaskArgs(args, task)
	args = prependLiquibaseConnectionArgs(args, rt.instance, windowsMode)
	env, err := mapLiquibaseEnv(prepared.request.LiquibaseEnv, windowsMode)
	if err != nil {
		return errorResponse("internal_error", "cannot map liquibase env", err.Error())
	}

	execLine := formatExecLine(execPath, args)
	m.appendLog(jobID, fmt.Sprintf("liquibase: exec %s", execLine))
	m.logJob(jobID, "liquibase exec %s", execLine)
	m.appendLog(jobID, "liquibase: start")
	var sinkCalled atomic.Bool
	lbCtx := engineRuntime.WithLogSink(ctx, func(line string) {
		sinkCalled.Store(true)
		m.appendLog(jobID, "liquibase: "+line)
	})
	output, err := m.liquibase.Run(lbCtx, LiquibaseRunRequest{
		ExecPath: execPath,
		ExecMode: execMode,
		Args:     args,
		Env:      env,
		WorkDir:  workDir,
		Mounts:   prepared.liquibaseMounts,
		Network:  "",
	})
	if !sinkCalled.Load() && strings.TrimSpace(output) != "" {
		m.appendLogLines(jobID, "liquibase", output)
	}
	if err != nil {
		if ctx.Err() != nil {
			return errorResponse("cancelled", "task cancelled", "")
		}
		details := strings.TrimSpace(output)
		if details == "" {
			details = err.Error()
		}
		if noSpaceResp := noSpaceErrorResponse("prepare step failed due to insufficient storage", "prepare_step", errors.New(details)); noSpaceResp != nil {
			return noSpaceResp
		}
		if noSpaceResp := noSpaceErrorResponse("prepare step failed due to insufficient storage", "prepare_step", err); noSpaceResp != nil {
			return noSpaceResp
		}
		return errorResponse("internal_error", "liquibase execution failed", details)
	}
	if ctx.Err() != nil {
		return errorResponse("cancelled", "task cancelled", "")
	}
	return nil
}

func prependLiquibaseConnectionArgs(args []string, instance engineRuntime.Instance, windowsMode bool) []string {
	host := instance.Host
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	}
	if windowsMode && isWSL() && (host == "127.0.0.1" || strings.EqualFold(host, "localhost")) {
		if resolved, err := resolveWSLPrimaryIPv4(); err == nil && strings.TrimSpace(resolved) != "" {
			host = resolved
		}
	}
	port := instance.Port
	if port == 0 {
		port = 5432
	}
	conn := []string{
		fmt.Sprintf("--url=jdbc:postgresql://%s:%d/postgres", host, port),
		"--username=sqlrs",
	}
	if len(args) == 0 {
		return conn
	}
	out := make([]string, 0, len(conn)+len(args))
	out = append(out, conn...)
	out = append(out, args...)
	return out
}

func resolveWSLPrimaryIPv4() (string, error) {
	ifaces, err := listNetInterfaces()
	if err != nil {
		return "", err
	}
	var firstGlobal string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifaceAddrs(iface)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet == nil || ipNet.IP == nil {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
				continue
			}
			if iface.Name == "eth0" {
				return ip.String(), nil
			}
			if firstGlobal == "" {
				firstGlobal = ip.String()
			}
		}
	}
	if firstGlobal == "" {
		return "", fmt.Errorf("no global ipv4")
	}
	return firstGlobal, nil
}

func formatExecLine(execPath string, args []string) string {
	execPath = strings.TrimSpace(execPath)
	if execPath == "" {
		execPath = "liquibase"
	}
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, formatExecArg(execPath))
	for _, arg := range args {
		parts = append(parts, formatExecArg(arg))
	}
	return strings.Join(parts, " ")
}

func formatExecArg(value string) string {
	if strings.TrimSpace(value) == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\"") {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

func deriveLiquibaseWorkDir(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file":
			if i+1 < len(args) {
				if dir := windowsPathDir(args[i+1]); dir != "" {
					return dir
				}
			}
		case strings.HasPrefix(arg, "--changelog-file="):
			if dir := windowsPathDir(strings.TrimPrefix(arg, "--changelog-file=")); dir != "" {
				return dir
			}
		case strings.HasPrefix(arg, "--defaults-file="):
			if dir := windowsPathDir(strings.TrimPrefix(arg, "--defaults-file=")); dir != "" {
				return dir
			}
		}
	}
	return ""
}

func windowsPathDir(path string) string {
	path = strings.TrimSpace(path)
	if !looksLikeWindowsPath(path) {
		return ""
	}
	lastSlash := strings.LastIndex(path, `\`)
	lastFwd := strings.LastIndex(path, "/")
	sep := lastSlash
	if lastFwd > sep {
		sep = lastFwd
	}
	if sep <= 2 {
		return ""
	}
	return path[:sep]
}

func (e *taskExecutor) createInstance(ctx context.Context, jobID string, prepared preparedRequest, stateID string) (*Result, *ErrorResponse) {
	m := e.m
	if ctx.Err() != nil {
		return nil, errorResponse("cancelled", "job cancelled", "")
	}
	if strings.TrimSpace(stateID) == "" {
		return nil, errorResponse("internal_error", "state id is required", "")
	}

	runner, ephemeral := m.runnerForJob(jobID)
	if runner == nil {
		return nil, errorResponse("internal_error", "job runner missing", "")
	}
	if ephemeral {
		defer m.cleanupRuntime(context.Background(), runner)
	}

	rt := runner.getRuntime()
	if rt == nil {
		var errResp *ErrorResponse
		rt, errResp = e.startRuntime(ctx, jobID, prepared, &TaskInput{Kind: "state", ID: stateID})
		if errResp != nil {
			return nil, errResp
		}
		runner.setRuntime(rt)
	}
	if rt.instance.Host == "" || rt.instance.Port == 0 {
		return nil, errorResponse("internal_error", "runtime instance is missing connection info", "")
	}

	instanceID, err := randomHex(16)
	if err != nil {
		return nil, errorResponse("internal_error", "cannot generate instance id", err.Error())
	}
	createdAt := m.now().UTC().Format(time.RFC3339Nano)
	status := store.InstanceStatusActive
	var runtimeID *string
	imageID := prepared.effectiveImageID()
	if strings.TrimSpace(imageID) == "" {
		return nil, errorResponse("internal_error", "resolved image id is required", "")
	}
	if strings.TrimSpace(rt.instance.ID) != "" {
		runtimeID = strPtr(rt.instance.ID)
	}
	var runtimeDir *string
	if strings.TrimSpace(rt.runtimeDir) != "" {
		runtimeDir = strPtr(rt.runtimeDir)
	}
	if err := m.store.CreateInstance(ctx, store.InstanceCreate{
		InstanceID: instanceID,
		StateID:    stateID,
		ImageID:    imageID,
		CreatedAt:  createdAt,
		RuntimeID:  runtimeID,
		RuntimeDir: runtimeDir,
		Status:     &status,
	}); err != nil {
		if ctx.Err() != nil {
			return nil, errorResponse("cancelled", "job cancelled", "")
		}
		return nil, errorResponse("internal_error", "cannot store instance", err.Error())
	}
	m.appendLog(jobID, fmt.Sprintf("instance created %s", instanceID))
	result := Result{
		DSN:                   buildDSN(rt.instance.Host, rt.instance.Port),
		InstanceID:            instanceID,
		StateID:               stateID,
		ImageID:               imageID,
		PrepareKind:           prepared.request.PrepareKind,
		PrepareArgsNormalized: prepared.argsNormalized,
	}
	return &result, nil
}

func (e *taskExecutor) ensureRuntime(ctx context.Context, jobID string, prepared preparedRequest, input *TaskInput, runner *jobRunner) (*jobRuntime, *ErrorResponse) {
	if runner == nil {
		return nil, errorResponse("internal_error", "job runner missing", "")
	}
	if rt := runner.getRuntime(); rt != nil {
		return rt, nil
	}
	rt, errResp := e.startRuntime(ctx, jobID, prepared, input)
	if errResp != nil {
		return nil, errResp
	}
	runner.setRuntime(rt)
	return rt, nil
}

func (e *taskExecutor) startRuntime(ctx context.Context, jobID string, prepared preparedRequest, input *TaskInput) (*jobRuntime, *ErrorResponse) {
	m := e.m
	if ctx.Err() != nil {
		return nil, errorResponse("cancelled", "job cancelled", "")
	}
	if input == nil {
		return nil, errorResponse("internal_error", "task input is required", "")
	}

	imageID := prepared.effectiveImageID()
	if strings.TrimSpace(imageID) == "" {
		return nil, errorResponse("internal_error", "resolved image id is required", "")
	}
	m.logInfoJob(jobID, "runtime start input_kind=%s input_id=%s image=%s", input.Kind, input.ID, imageID)
	m.logInfoJob(jobID, "runtime start state_store_root=%s", m.stateStoreRoot)
	ctx = engineRuntime.WithLogSink(ctx, func(line string) {
		m.appendLog(jobID, "docker: "+line)
	})
	var srcDir string
	var stateDir string
	switch input.Kind {
	case "image":
		m.appendLog(jobID, fmt.Sprintf("docker: init base %s", imageID))
		paths, err := resolveStatePaths(m.stateStoreRoot, imageID, "", m.statefs)
		if err != nil {
			return nil, errorResponse("internal_error", "cannot resolve state paths", err.Error())
		}
		if err := e.snapshot.ensureBaseState(ctx, imageID, paths.baseDir); err != nil {
			if ctx.Err() != nil {
				return nil, errorResponse("cancelled", "job cancelled", "")
			}
			return nil, errorResponse("internal_error", "cannot initialize base state", err.Error())
		}
		srcDir = paths.baseDir
	case "state":
		if strings.TrimSpace(input.ID) == "" {
			return nil, errorResponse("internal_error", "input state id is required", "")
		}
		entry, ok, err := m.store.GetState(ctx, input.ID)
		if err != nil {
			return nil, errorResponse("internal_error", "cannot load input state", err.Error())
		}
		if !ok {
			return nil, errorResponse("internal_error", "input state not found", input.ID)
		}
		if strings.TrimSpace(entry.ImageID) != "" {
			imageID = entry.ImageID
		}
		paths, err := resolveStatePaths(m.stateStoreRoot, imageID, input.ID, m.statefs)
		if err != nil {
			return nil, errorResponse("internal_error", "cannot resolve state paths", err.Error())
		}
		stateDir = paths.stateDir
		if dirtyPath := postmasterPIDPath(paths.stateDir); dirtyPath != "" {
			m.logInfoJob(jobID, "postmaster.pid present in state dir path=%s", dirtyPath)
			return nil, errorResponse("internal_error", "state snapshot is dirty (postmaster.pid present)", dirtyPath)
		}
		ok, err = hasPGVersion(paths.stateDir)
		if err != nil {
			return nil, errorResponse("internal_error", "cannot inspect state PG_VERSION", err.Error())
		}
		if !ok {
			return nil, errorResponse("internal_error", "state snapshot missing PG_VERSION", paths.stateDir)
		}
		m.logInfoJob(jobID, "postmaster.pid not found in state dir=%s", paths.stateDir)
		srcDir = paths.stateDir
	default:
		return nil, errorResponse("internal_error", "unsupported task input", input.Kind)
	}

	runtimeDir := filepath.Join(m.stateStoreRoot, "jobs", jobID, "runtime")
	m.logInfoJob(jobID, "runtime start runtime_dir=%s", runtimeDir)
	if stateDir != "" {
		if rel, err := filepath.Rel(stateDir, runtimeDir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return nil, errorResponse("internal_error", "runtime dir is nested inside state dir", fmt.Sprintf("runtime=%s state=%s", runtimeDir, stateDir))
		}
	}
	if err := removeAllFn(runtimeDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		staleRuntimeDir := fmt.Sprintf("%s.stale-%d", runtimeDir, time.Now().UnixNano())
		if renameErr := os.Rename(runtimeDir, staleRuntimeDir); renameErr != nil {
			if !errors.Is(renameErr, os.ErrNotExist) && !errors.Is(renameErr, syscall.ENOTDIR) {
				return nil, errorResponse("internal_error", "cannot reset runtime dir", fmt.Sprintf("remove=%v rename=%v", err, renameErr))
			}
		} else {
			m.logInfoJob(jobID, "runtime dir reset via rename old=%s new=%s remove_err=%v", runtimeDir, staleRuntimeDir, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(runtimeDir), 0o700); err != nil {
		if noSpaceResp := noSpaceErrorResponse("insufficient storage before state execution", "prepare_step", err); noSpaceResp != nil {
			return nil, noSpaceResp
		}
		return nil, errorResponse("internal_error", "cannot create runtime dir", err.Error())
	}
	clone, err := m.statefs.Clone(ctx, srcDir, runtimeDir)
	if err != nil {
		if noSpaceResp := noSpaceErrorResponse("insufficient storage before state execution", "prepare_step", err); noSpaceResp != nil {
			return nil, noSpaceResp
		}
		return nil, errorResponse("internal_error", "cannot clone state", err.Error())
	}
	if dirtyPath := postmasterPIDPath(clone.MountDir); dirtyPath != "" {
		m.logInfoJob(jobID, "postmaster.pid present in runtime dir path=%s", dirtyPath)
		return nil, errorResponse("internal_error", "runtime data dir is dirty (postmaster.pid present)", dirtyPath)
	}
	if input.Kind == "state" {
		ok, err := hasPGVersion(clone.MountDir)
		if err != nil {
			return nil, errorResponse("internal_error", "cannot inspect runtime PG_VERSION", err.Error())
		}
		if !ok {
			return nil, errorResponse("internal_error", "runtime data dir missing PG_VERSION", clone.MountDir)
		}
	}
	m.logInfoJob(jobID, "postmaster.pid not found in runtime dir=%s", clone.MountDir)

	rtScriptMount, err := scriptMountForFiles(prepared.filePaths)
	if err != nil {
		_ = clone.Cleanup()
		return nil, errorResponse("internal_error", "cannot prepare scripts", err.Error())
	}

	m.appendLog(jobID, "docker: start container")
	ctx = engineRuntime.WithLogSink(ctx, func(line string) {
		m.appendLog(jobID, "docker: "+line)
	})
	allowInitdb := strings.TrimSpace(input.Kind) == "image"
	containerName := "sqlrs-prepare-" + jobID
	if suffix, err := randomHex(4); err == nil {
		containerName = containerName + "-" + suffix
	}
	instance, err := m.runtime.Start(ctx, engineRuntime.StartRequest{
		ImageID:     imageID,
		DataDir:     clone.MountDir,
		Name:        containerName,
		Mounts:      runtimeMountsFrom(rtScriptMount),
		AllowInitdb: allowInitdb,
	})
	if err != nil {
		_ = clone.Cleanup()
		m.logInfoJob(jobID, "runtime start failed image=%s input=%s err=%v", imageID, input.Kind, err)
		if ctx.Err() != nil {
			return nil, errorResponse("cancelled", "job cancelled", "")
		}
		return nil, errorResponse("internal_error", "cannot start runtime", err.Error())
	}
	m.appendLog(jobID, fmt.Sprintf("docker: container started %s", instance.ID))
	m.logJob(jobID, "runtime started container=%s host=%s port=%d snapshot=%s", instance.ID, instance.Host, instance.Port, m.statefs.Kind())
	m.appendLog(jobID, "docker: postgres ready")

	return &jobRuntime{
		instance:    instance,
		dataDir:     clone.MountDir,
		runtimeDir:  runtimeDir,
		cleanup:     clone.Cleanup,
		scriptMount: rtScriptMount,
	}, nil
}

var removeAllFn = os.RemoveAll

func (s *snapshotOrchestrator) ensureBaseState(ctx context.Context, imageID string, baseDir string) error {
	m := s.m
	if strings.TrimSpace(baseDir) == "" {
		return fmt.Errorf("base dir is required")
	}
	if initMarkerExists(baseDir) {
		return nil
	}
	if err := ensureBaseDir(ctx, m.statefs, baseDir); err != nil {
		return err
	}
	if err := withInitLock(ctx, baseDir, func() error {
		if initMarkerExists(baseDir) {
			return nil
		}
		if ok, err := hasPGVersion(baseDir); ok {
			return writeInitMarker(baseDir)
		} else if err != nil {
			return writeInitMarker(baseDir)
		}
		if err := resetBaseDirContents(baseDir); err != nil {
			return err
		}
		if err := m.runtime.InitBase(ctx, imageID, baseDir); err != nil {
			return err
		}
		return writeInitMarker(baseDir)
	}); err != nil {
		return err
	}
	return nil
}

const (
	baseInitMarkerName = ".init.ok"
	baseInitLockName   = ".init.lock"
)

func initMarkerExists(baseDir string) bool {
	_, err := os.Stat(filepath.Join(baseDir, baseInitMarkerName))
	return err == nil
}

func writeInitMarker(baseDir string) error {
	path := filepath.Join(baseDir, baseInitMarkerName)
	return os.WriteFile(path, []byte("ok"), 0o600)
}

func hasPGVersion(baseDir string) (bool, error) {
	for _, path := range pgVersionPaths(baseDir) {
		if _, err := os.Stat(path); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			if os.IsPermission(err) {
				return false, err
			}
			return false, err
		}
	}
	return false, nil
}

func pgVersionPaths(baseDir string) []string {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil
	}
	paths := []string{filepath.Join(baseDir, "PG_VERSION")}
	pgDataDir := pgDataHostDir(baseDir)
	if pgDataDir != "" && pgDataDir != baseDir {
		paths = append(paths, filepath.Join(pgDataDir, "PG_VERSION"))
	}
	return paths
}

func pgDataHostDir(baseDir string) string {
	rel := strings.TrimPrefix(postgresDataDir, postgresDataDirRoot)
	rel = strings.TrimPrefix(rel, "/")
	if strings.TrimSpace(rel) == "" {
		return baseDir
	}
	return filepath.Join(baseDir, filepath.FromSlash(rel))
}

func postmasterPIDPath(stateDir string) string {
	stateDir = strings.TrimSpace(stateDir)
	if stateDir == "" {
		return ""
	}
	candidates := []string{
		filepath.Join(stateDir, "postmaster.pid"),
	}
	if pgDataDir := pgDataHostDir(stateDir); pgDataDir != "" && pgDataDir != stateDir {
		candidates = append(candidates, filepath.Join(pgDataDir, "postmaster.pid"))
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func (s *snapshotOrchestrator) invalidateDirtyCachedState(ctx context.Context, jobID string, prepared preparedRequest, stateID string) (bool, *ErrorResponse) {
	m := s.m
	if strings.TrimSpace(stateID) == "" {
		return false, nil
	}
	entry, ok, err := m.store.GetState(ctx, stateID)
	if err != nil {
		return false, errorResponse("internal_error", "cannot load cached state", err.Error())
	}
	imageID := prepared.effectiveImageID()
	if ok && strings.TrimSpace(entry.ImageID) != "" {
		imageID = entry.ImageID
	}
	if strings.TrimSpace(imageID) == "" {
		return false, errorResponse("internal_error", "resolved image id is required", "")
	}
	paths, err := resolveStatePaths(m.stateStoreRoot, imageID, stateID, m.statefs)
	if err != nil {
		return false, errorResponse("internal_error", "cannot resolve cached state paths", err.Error())
	}
	if dirtyPath := postmasterPIDPath(paths.stateDir); dirtyPath != "" {
		m.logInfoJob(jobID, "cached state dirty state=%s path=%s", stateID, dirtyPath)
		if err := m.statefs.RemovePath(context.Background(), paths.stateDir); err != nil {
			return false, errorResponse("internal_error", "cannot remove dirty cached state dir", err.Error())
		}
		if err := m.store.DeleteState(ctx, stateID); err != nil {
			return false, errorResponse("internal_error", "cannot delete dirty cached state", err.Error())
		}
		return true, nil
	}
	ok, err = hasPGVersion(paths.stateDir)
	if err != nil {
		return false, errorResponse("internal_error", "cannot inspect cached state PG_VERSION", err.Error())
	}
	if !ok {
		m.logInfoJob(jobID, "cached state missing PG_VERSION state=%s dir=%s", stateID, paths.stateDir)
		if err := m.statefs.RemovePath(context.Background(), paths.stateDir); err != nil {
			return false, errorResponse("internal_error", "cannot remove cached state dir missing PG_VERSION", err.Error())
		}
		if err := m.store.DeleteState(ctx, stateID); err != nil {
			return false, errorResponse("internal_error", "cannot delete cached state missing PG_VERSION", err.Error())
		}
		return true, nil
	}
	return false, nil
}

func resetBaseDirContents(baseDir string) error {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	keep := map[string]bool{baseInitLockName: true}
	hasOther := false
	for _, entry := range entries {
		if keep[entry.Name()] {
			continue
		}
		hasOther = true
		break
	}
	if !hasOther {
		return nil
	}
	for _, entry := range entries {
		if keep[entry.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(baseDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func withInitLock(ctx context.Context, baseDir string, fn func() error) error {
	if fn == nil {
		return fmt.Errorf("lock callback is required")
	}
	lockPath := filepath.Join(baseDir, baseInitLockName)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if shouldRetryLockAcquire(err, lockPath) {
			if initMarkerExists(baseDir) {
				_ = os.Remove(lockPath)
				return nil
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		return err
	}
}

const (
	stateBuildMarkerName = ".build.ok"
	stateBuildLockName   = ".build.lock"
)

var errStateBuildFailed = errors.New("state build failed")

func stateBuildMarkerExists(stateDir string, kind string) bool {
	_, err := os.Stat(stateBuildMarkerPath(stateDir, kind))
	return err == nil
}

func writeStateBuildMarker(stateDir string, kind string) error {
	path := stateBuildMarkerPath(stateDir, kind)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("ok"), 0o600)
}

const stateBuildLockDirName = ".build"

func snapshotKind(fs statefs.StateFS) string {
	if fs == nil {
		return ""
	}
	return fs.Kind()
}

func stateBuildMarkerPath(stateDir string, kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "btrfs" {
		base := filepath.Dir(stateDir)
		stateID := filepath.Base(stateDir)
		return filepath.Join(base, stateBuildLockDirName, stateID+".ok")
	}
	return filepath.Join(stateDir, stateBuildMarkerName)
}

func stateBuildLockPath(stateDir string, kind string) string {
	if kind == "btrfs" {
		base := filepath.Dir(stateDir)
		stateID := filepath.Base(stateDir)
		return filepath.Join(base, stateBuildLockDirName, stateID+".lock")
	}
	return filepath.Join(stateDir, stateBuildLockName)
}

func withStateBuildLock(ctx context.Context, stateDir string, lockPath string, kind string, fn func() error) error {
	if fn == nil {
		return fmt.Errorf("lock callback is required")
	}
	if strings.TrimSpace(lockPath) == "" {
		return fmt.Errorf("lock path is required")
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if shouldRetryLockAcquire(err, lockPath) {
			if stateBuildMarkerExists(stateDir, kind) {
				_ = os.Remove(lockPath)
				return nil
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		return err
	}
}

func shouldRetryLockAcquire(err error, lockPath string) bool {
	if isLockBusyError(err, lockPath) {
		return true
	}
	if isPermissionError(err) {
		_, statErr := os.Stat(lockPath)
		if errors.Is(statErr, os.ErrNotExist) {
			// Lock file may disappear between failed open and stat while another
			// worker releases it; retry instead of surfacing transient EACCES.
			return true
		}
		if isPermissionError(statErr) {
			return true
		}
	}
	if !os.IsExist(err) && !errors.Is(err, os.ErrExist) {
		return false
	}
	info, statErr := os.Stat(lockPath)
	if statErr == nil {
		return info.Mode().IsRegular()
	}
	if errors.Is(statErr, os.ErrNotExist) {
		// Another process may have released the lock between OpenFile and Stat.
		// Retry instead of surfacing a transient "file exists" error.
		return true
	}
	if runtime.GOOS == "windows" && isPermissionError(statErr) {
		return true
	}
	return false
}

func isLockBusyError(err error, lockPath string) bool {
	if err == nil || !isPermissionError(err) {
		return false
	}
	info, statErr := os.Stat(lockPath)
	if statErr == nil {
		// Lock paths are expected to be plain files created with O_EXCL.
		// If the path exists but is not a regular file (for example, a directory),
		// treat it as an invalid path error instead of a transient busy lock.
		return info.Mode().IsRegular()
	}
	if runtime.GOOS == "windows" && isPermissionError(statErr) {
		return true
	}
	return false
}

func isPermissionError(err error) bool {
	return os.IsPermission(err) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}

func ensureBaseDir(ctx context.Context, fs statefs.StateFS, baseDir string) error {
	if fs == nil {
		return fmt.Errorf("statefs is required")
	}
	return fs.EnsureBaseDir(ctx, baseDir)
}

func resetStateDir(ctx context.Context, fs statefs.StateFS, stateDir string) error {
	if fs == nil {
		return fmt.Errorf("statefs is required")
	}
	if err := fs.RemovePath(ctx, stateDir); err != nil {
		return err
	}
	return fs.EnsureStateDir(ctx, stateDir)
}

func (m *PrepareService) cleanupRuntime(ctx context.Context, runner *jobRunner) {
	if runner == nil {
		return
	}
	rt := runner.getRuntime()
	if rt == nil {
		return
	}
	runner.setRuntime(nil)
	stopCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := m.runtime.Stop(stopCtx, rt.instance.ID); err != nil {
		_ = m.runtime.Stop(context.Background(), rt.instance.ID)
	}
	if rt.cleanup != nil {
		_ = rt.cleanup()
	}
}

func (m *PrepareService) runnerForJob(jobID string) (*jobRunner, bool) {
	if strings.TrimSpace(jobID) != "" {
		if runner := m.getRunner(jobID); runner != nil {
			return runner, false
		}
	}
	return &jobRunner{}, true
}

func parentStateID(input *TaskInput) *string {
	if input == nil || input.Kind != "state" || strings.TrimSpace(input.ID) == "" {
		return nil
	}
	return strPtr(input.ID)
}
