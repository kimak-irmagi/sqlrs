package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/inputset"
	"github.com/sqlrs/cli/internal/refctx"
	"github.com/sqlrs/cli/internal/remotesource"
)

type stageMode string

const (
	stageModePrepare stageMode = "prepare"
	stageModePlan    stageMode = "plan"
)

type stageRunRequest struct {
	mode                    stageMode
	class                   string
	kind                    string
	parsed                  prepareArgs
	workspaceRoot           string
	cwd                     string
	invocationCwd           string
	aliasPath               string
	ref                     *refctx.Context
	output                  string
	relativizeLiquibasePath bool
}

type stageRuntime struct {
	opts       cli.PrepareOptions
	watch      bool
	cleanup    func() error
	trace      prepareTraceBase
	traceReq   stageRunRequest
	actualRef  *refctx.Context
	traceReady bool
}

type prepareStageResult struct {
	result   client.PrepareJobResult
	handled  bool
	accepted *client.PrepareJobAccepted
}

func buildStageRuntime(stderr io.Writer, runOpts cli.PrepareOptions, cfg config.LoadedConfig, req stageRunRequest) (stageRuntime, error) {
	if req.mode == stageModePlan && req.parsed.WatchSpecified {
		return stageRuntime{}, ExitErrorf(2, "plan does not support --watch/--no-watch")
	}
	if req.kind == "lb" && len(req.parsed.PsqlArgs) == 0 {
		return stageRuntime{}, ExitErrorf(2, "liquibase command is required")
	}

	imageID, source, err := resolvePrepareImage(req.parsed.Image, cfg)
	if err != nil {
		return stageRuntime{}, err
	}
	if imageID == "" {
		return stageRuntime{}, ExitErrorf(2, "Missing base image id (set --image or dbms.image)")
	}
	if runOpts.Verbose {
		fmt.Fprint(stderr, formatImageSource(imageID, source))
	}

	runtime := stageRuntime{
		opts:     runOpts,
		watch:    req.parsed.Watch,
		traceReq: req,
	}
	runtime.opts.ImageID = imageID
	runtime.opts.DisableControlPrompt = usesPrepareRef(req.parsed, req.ref)

	actualRef, refCleanup, err := resolvePrepareBindingContext(req.workspaceRoot, req.cwd, req.parsed, req.ref)
	if err != nil {
		return stageRuntime{}, err
	}
	runtime.actualRef = actualRef
	runtime.cleanup = refCleanup

	switch req.kind {
	case "psql":
		bound, err := bindPreparePsqlInputsFn(runOpts, req.workspaceRoot, req.cwd, req.parsed, actualRef, os.Stdin)
		if err != nil {
			return stageRuntime{}, err
		}
		runtime.cleanup = bound.cleanup
		runtime.opts.PsqlArgs = bound.PsqlArgs
		runtime.opts.Stdin = bound.Stdin
		runtime.opts.PrepareKind = "psql"
	case "lb":
		liquibaseExec, err := resolveLiquibaseExec(cfg)
		if err != nil {
			return stageRuntime{}, combineBindingCleanupError(err, runStageCleanup(refCleanup))
		}
		liquibaseExecMode, err := resolveLiquibaseExecMode(cfg)
		if err != nil {
			return stageRuntime{}, combineBindingCleanupError(err, runStageCleanup(refCleanup))
		}
		bound, err := bindPrepareLiquibaseInputsFn(runOpts, req.workspaceRoot, req.cwd, req.parsed, actualRef, liquibaseExec, liquibaseExecMode, req.relativizeLiquibasePath)
		if err != nil {
			return stageRuntime{}, err
		}
		runtime.cleanup = bound.cleanup
		runtime.opts.LiquibaseArgs = bound.LiquibaseArgs
		runtime.opts.LiquibaseExec = liquibaseExec
		runtime.opts.LiquibaseExecMode = liquibaseExecMode
		runtime.opts.LiquibaseEnv = resolveLiquibaseEnv()
		runtime.opts.WorkDir = bound.WorkDir
		runtime.opts.PrepareKind = "lb"
	default:
		switch req.mode {
		case stageModePlan:
			return stageRuntime{}, combineBindingCleanupError(ExitErrorf(2, "unsupported plan kind: %s", req.kind), runStageCleanup(refCleanup))
		default:
			return stageRuntime{}, combineBindingCleanupError(ExitErrorf(2, "unsupported prepare kind: %s", req.kind), runStageCleanup(refCleanup))
		}
	}

	if req.mode == stageModePlan {
		runtime.opts.PlanOnly = true
	}
	sourceSync, err := buildRemoteSourceSyncOptions(stderr, runtime.opts, req, actualRef)
	if err != nil {
		return stageRuntime{}, combineBindingCleanupError(err, runStageCleanup(runtime.cleanup))
	}
	runtime.opts.SourceSync = sourceSync
	return runtime, nil
}

func buildRemoteSourceSyncOptions(stderr io.Writer, opts cli.PrepareOptions, req stageRunRequest, actualRef *refctx.Context) (*remotesource.Options, error) {
	if strings.ToLower(strings.TrimSpace(opts.Mode)) != "remote" {
		return nil, nil
	}
	mode := strings.ToLower(strings.TrimSpace(opts.SourceSyncMode))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "off":
		return nil, nil
	case "auto":
	default:
		return nil, ExitErrorf(2, "unsupported sourceSync.mode: %s", opts.SourceSyncMode)
	}

	root := strings.TrimSpace(req.workspaceRoot)
	workDir := strings.TrimSpace(req.cwd)
	var sourceFS inputset.FileSystem
	if actualRef != nil {
		root = strings.TrimSpace(actualRef.WorkspaceRoot)
		if root == "" {
			root = strings.TrimSpace(actualRef.RepoRoot)
		}
		sourceFS = actualRef.FileSystem
		workDir = strings.TrimSpace(actualRef.BaseDir)
	}
	if root == "" {
		root = strings.TrimSpace(req.cwd)
	}
	if root == "" {
		root = strings.TrimSpace(req.invocationCwd)
	}
	if workDir == "" {
		workDir = root
	}
	return &remotesource.Options{
		Enabled:       true,
		MaxRounds:     opts.SourceSyncMaxRounds,
		WorkspaceRoot: root,
		WorkDir:       workDir,
		WorkspaceID:   opts.ProfileName,
		FileSystem:    sourceFS,
		Progress:      newSourceSyncProgress(stderr, opts.Verbose),
	}, nil
}

func runStageCleanup(cleanup func() error) error {
	if cleanup == nil {
		return nil
	}
	return cleanup()
}

func ensurePrepareTrace(runtime *stageRuntime) error {
	if runtime == nil || runtime.traceReady {
		return nil
	}
	trace, err := collectPrepareTrace(runtime.traceReq, runtime.opts, runtime.actualRef)
	if err != nil {
		return err
	}
	runtime.trace = trace
	runtime.traceReady = true
	return nil
}

func runPlanStage(runtime stageRuntime) (cli.PlanResult, error) {
	return runPlanFn(context.Background(), runtime.opts)
}

func renderPlanStage(stdout io.Writer, output string, result cli.PlanResult) error {
	if output == "json" {
		return writeJSON(stdout, result)
	}
	return cli.PrintPlan(stdout, result)
}

func executePlanStage(stdout io.Writer, runtime stageRuntime, output string) error {
	result, err := runPlanStage(runtime)
	if err != nil {
		return finishPrepareCleanup(err, runtime.cleanup)
	}
	return finishPrepareCleanup(renderPlanStage(stdout, output, result), runtime.cleanup)
}

func runPrepareStage(w stdoutAndErr, runtime stageRuntime) (prepareStageResult, error) {
	if !runtime.watch {
		accepted, err := submitPrepareFn(context.Background(), runtime.opts)
		if err != nil {
			return prepareStageResult{}, err
		}
		printPrepareJobRefs(w.stdout, accepted)
		if runtime.opts.CompositeRun {
			printRunSkipped(w.stdout, "prepare_not_watched")
		}
		return prepareStageResult{handled: true, accepted: &accepted}, nil
	}

	prepareResult, err := runPrepareFn(context.Background(), runtime.opts)
	if err != nil {
		var detached *cli.PrepareDetachedError
		if errors.As(err, &detached) {
			accepted := client.PrepareJobAccepted{
				JobID:     detached.JobID,
				StatusURL: "/v1/prepare-jobs/" + detached.JobID,
				EventsURL: "/v1/prepare-jobs/" + detached.JobID + "/events",
			}
			printPrepareJobRefs(w.stdout, accepted)
			if runtime.opts.CompositeRun {
				printRunSkipped(w.stdout, "prepare_detached")
			}
			return prepareStageResult{handled: true, accepted: &accepted}, nil
		}
		return prepareStageResult{}, err
	}
	return prepareStageResult{result: prepareResult}, nil
}

func executePrepareStage(w stdoutAndErr, runtime stageRuntime) (result client.PrepareJobResult, handled bool, err error) {
	stageResult, err := runPrepareStage(w, runtime)
	if err != nil {
		return client.PrepareJobResult{}, false, finishPrepareCleanup(err, runtime.cleanup)
	}
	return stageResult.result, stageResult.handled, finishPrepareCleanup(nil, runtime.cleanup)
}
