package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/inputset"
	inputliquibase "github.com/sqlrs/cli/internal/inputset/liquibase"
	inputpsql "github.com/sqlrs/cli/internal/inputset/psql"
	"github.com/sqlrs/cli/internal/refctx"
	"github.com/sqlrs/cli/internal/remotesource"
)

type provenanceCommand struct {
	Family string `json:"family"`
	Class  string `json:"class,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

type provenanceCache struct {
	Decision        string `json:"decision,omitempty"`
	ReasonCode      string `json:"reasonCode,omitempty"`
	Signature       string `json:"signature,omitempty"`
	MatchedStateID  string `json:"matchedStateId,omitempty"`
	ResolvedImageID string `json:"resolvedImageId,omitempty"`
}

type provenanceOutcome struct {
	Status   string `json:"status"`
	PlanOnly bool   `json:"planOnly,omitempty"`
	StateID  string `json:"stateId,omitempty"`
	JobID    string `json:"jobId,omitempty"`
	Error    string `json:"error,omitempty"`
}

type provenanceArtifact struct {
	Timestamp      string                      `json:"timestamp"`
	Command        provenanceCommand           `json:"command"`
	WorkspaceRoot  string                      `json:"workspaceRoot,omitempty"`
	CWD            string                      `json:"cwd,omitempty"`
	AliasPath      string                      `json:"aliasPath,omitempty"`
	Prepare        cli.CacheExplainPrepareSpec `json:"prepare"`
	RefContext     *cli.CacheExplainRefContext `json:"refContext,omitempty"`
	NormalizedArgs []string                    `json:"normalizedArgs,omitempty"`
	Inputs         []cli.CacheExplainInput     `json:"inputs"`
	Cache          provenanceCache             `json:"cache"`
	Outcome        provenanceOutcome           `json:"outcome"`
}

type prepareTraceBase struct {
	Command        provenanceCommand
	WorkspaceRoot  string
	CWD            string
	AliasPath      string
	Prepare        cli.CacheExplainPrepareSpec
	RefContext     *cli.CacheExplainRefContext
	NormalizedArgs []string
	Inputs         []cli.CacheExplainInput
}

var explainPrepareCacheFn = explainPrepareCache
var writeProvenanceArtifactFn = writeProvenanceArtifact

func explainPrepareCache(ctx context.Context, opts cli.PrepareOptions) (client.CacheExplainPrepareResponse, error) {
	cliClient, err := cli.PrepareClient(ctx, opts)
	if err != nil {
		return client.CacheExplainPrepareResponse{}, err
	}
	request := cli.BuildPrepareJobRequest(opts, opts.PlanOnly)
	if request.PrepareKind == "lb" {
		return explainLiquibaseCacheJob(ctx, cliClient, opts, request)
	}
	if opts.SourceSync == nil || !opts.SourceSync.Enabled {
		return cliClient.ExplainPrepareCache(ctx, request)
	}
	syncOpts := *opts.SourceSync
	syncOpts.Uploader = cliClient
	return remotesource.Execute(ctx, syncOpts, request, func(ctx context.Context, req client.PrepareJobRequest) (client.CacheExplainPrepareResponse, error) {
		return cliClient.ExplainPrepareCache(ctx, req)
	})
}

// explainLiquibaseCacheJob waits for the saved result of the accepted job.
// The 202 admission body is never interpreted as a cache decision; an
// ambiguous POST is not retried except for the explicit missing-source 409
// handshake. See docs/architecture/liquibase-planning-recovery-design.md.
func explainLiquibaseCacheJob(ctx context.Context, cliClient *client.Client, opts cli.PrepareOptions, request client.PrepareJobRequest) (client.CacheExplainPrepareResponse, error) {
	var accepted client.PrepareJobAccepted
	var err error
	if opts.SourceSync == nil || !opts.SourceSync.Enabled {
		accepted, err = cliClient.AdmitLiquibaseCacheExplain(ctx, request)
	} else {
		syncOpts := *opts.SourceSync
		syncOpts.Uploader = cliClient
		accepted, err = remotesource.Execute(ctx, syncOpts, request, func(ctx context.Context, req client.PrepareJobRequest) (client.PrepareJobAccepted, error) {
			return cliClient.AdmitLiquibaseCacheExplain(ctx, req)
		})
	}
	if err != nil {
		return client.CacheExplainPrepareResponse{}, fmt.Errorf("Liquibase cache explanation admission failed or its response was lost; inspect the ordinary prepare-job list before retrying: %w", err)
	}
	status, err := cli.RunWatch(ctx, opts, accepted.JobID)
	if err != nil {
		return client.CacheExplainPrepareResponse{}, err
	}
	if status.Purpose != "cache_explain" || status.PrepareKind != "lb" || status.Result != nil || status.CacheExplainResult == nil {
		return client.CacheExplainPrepareResponse{}, fmt.Errorf("Liquibase cache explanation job %s finished without a saved explanation", accepted.JobID)
	}
	return *status.CacheExplainResult, nil
}

func collectPrepareTrace(req stageRunRequest, opts cli.PrepareOptions, actualRef *refctx.Context) (prepareTraceBase, error) {
	trace := prepareTraceBase{
		Command: provenanceCommand{
			Family: string(req.mode),
			Class:  traceCommandClass(req.class),
			Kind:   req.kind,
		},
		WorkspaceRoot: req.workspaceRoot,
		CWD:           req.invocationCwd,
		Prepare: cli.CacheExplainPrepareSpec{
			Class: traceCommandClass(req.class),
			Kind:  req.kind,
			Image: opts.ImageID,
		},
	}
	collectionRoot, baseDir, fs := traceCollectorContext(req, actualRef)
	if actualRef != nil {
		trace.RefContext = &cli.CacheExplainRefContext{
			Requested:      actualRef.GitRef,
			ResolvedCommit: actualRef.ResolvedCommit,
			Mode:           actualRef.RefMode,
		}
	}
	trace.AliasPath = relativeTracePath(collectionRoot, req.aliasPath)

	switch req.kind {
	case "psql":
		resolver := inputset.NewWorkspaceResolver(collectionRoot, baseDir, nil)
		normalizedArgs, _, err := inputpsql.NormalizeArgs(req.parsed.PsqlArgs, resolver, strings.NewReader(stringPtrOrEmpty(opts.Stdin)))
		if err != nil {
			return prepareTraceBase{}, wrapInputsetError(err)
		}
		trace.NormalizedArgs = normalizedArgs
		set, err := inputpsql.CollectInvocationInputs(normalizedArgs, resolver, opts.Stdin, fs)
		if err != nil {
			return prepareTraceBase{}, wrapInputsetError(err)
		}
		trace.Inputs = append(trace.Inputs, traceInputsFromSet(set, collectionRoot)...)
	case "lb":
		resolver := inputset.NewWorkspaceResolver(collectionRoot, baseDir, nil)
		normalizedArgs, err := inputliquibase.NormalizeArgs(req.parsed.PsqlArgs, resolver, true)
		if err != nil {
			return prepareTraceBase{}, wrapInputsetError(err)
		}
		trace.NormalizedArgs = normalizedArgs
		set, err := inputliquibase.CollectInvocationInputs(normalizedArgs, resolver, fs)
		if err != nil {
			return prepareTraceBase{}, wrapInputsetError(err)
		}
		trace.Inputs = append(trace.Inputs, traceInputsFromSet(set, collectionRoot)...)
	default:
		trace.NormalizedArgs = append([]string{}, req.parsed.PsqlArgs...)
	}

	if trace.Command.Class == "alias" && strings.TrimSpace(req.aliasPath) != "" {
		input, err := traceInputFromPath(req.aliasPath, collectionRoot, fs)
		if err != nil {
			return prepareTraceBase{}, err
		}
		trace.Inputs = append([]cli.CacheExplainInput{input}, trace.Inputs...)
	}
	return trace, nil
}

func traceCollectorContext(req stageRunRequest, actualRef *refctx.Context) (string, string, inputset.FileSystem) {
	root := req.workspaceRoot
	baseDir := req.cwd
	var fs inputset.FileSystem = inputset.OSFileSystem{}
	if actualRef != nil {
		fs = actualRef.FileSystem
		if actualRef.WorkspaceRoot != "" {
			root = actualRef.WorkspaceRoot
		} else if actualRef.RepoRoot != "" && strings.TrimSpace(root) == "" {
			root = actualRef.RepoRoot
		}
		baseDir = traceBindBaseDir(req, actualRef)
	}
	return root, baseDir, fs
}

func traceBindBaseDir(req stageRunRequest, actualRef *refctx.Context) string {
	if actualRef == nil {
		return req.cwd
	}
	switch strings.TrimSpace(strings.ToLower(req.kind)) {
	case "lb":
		return effectiveRefBindBaseDir(req.cwd, actualRef, req.ref)
	default:
		if strings.TrimSpace(actualRef.BaseDir) != "" {
			return actualRef.BaseDir
		}
		return req.cwd
	}
}

func traceCommandClass(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "alias":
		return "alias"
	default:
		return "raw"
	}
}

func traceInputsFromSet(set inputset.InputSet, root string) []cli.CacheExplainInput {
	inputs := make([]cli.CacheExplainInput, 0, len(set.Entries))
	for _, entry := range set.Entries {
		inputs = append(inputs, cli.CacheExplainInput{
			Path: relativeTracePath(root, entry.AbsPath),
			Hash: entry.Hash,
		})
	}
	return inputs
}

func traceInputFromPath(path string, root string, fs inputset.FileSystem) (cli.CacheExplainInput, error) {
	hash, err := traceInputHash(path, fs)
	if err != nil {
		return cli.CacheExplainInput{}, err
	}
	return cli.CacheExplainInput{
		Path: relativeTracePath(root, path),
		Hash: hash,
	}, nil
}

func traceInputHash(path string, fs inputset.FileSystem) (string, error) {
	if oid, ok := fs.(inputset.BlobOIDer); ok {
		return oid.BlobOID(path)
	}
	data, err := fs.ReadFile(path)
	if err != nil {
		return "", err
	}
	return inputset.HashContent(data), nil
}

func stringPtrOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func relativeTracePath(root string, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	cleaned := filepath.Clean(trimmed)
	base := strings.TrimSpace(root)
	if base != "" {
		if rel, err := filepath.Rel(base, cleaned); err == nil {
			if rel == "." {
				return "."
			}
			if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return filepath.ToSlash(rel)
			}
		}
	}
	return filepath.ToSlash(cleaned)
}

func cacheExplainResult(trace prepareTraceBase, response client.CacheExplainPrepareResponse) cli.CacheExplainResult {
	result := cli.CacheExplainResult{
		Decision:   response.Decision,
		ReasonCode: response.ReasonCode,
		Prepare:    trace.Prepare,
		RefContext: trace.RefContext,
		Cache: cli.CacheExplainDecision{
			Signature:       response.Signature,
			MatchedStateID:  response.MatchedStateID,
			ResolvedImageID: response.ResolvedImageID,
		},
		Inputs: append([]cli.CacheExplainInput{}, trace.Inputs...),
	}
	if result.Decision == "" {
		result.Decision = "miss"
	}
	return result
}

func provenanceFromTrace(trace prepareTraceBase, response client.CacheExplainPrepareResponse, outcome provenanceOutcome) provenanceArtifact {
	return provenanceArtifact{
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Command:        trace.Command,
		WorkspaceRoot:  trace.WorkspaceRoot,
		CWD:            trace.CWD,
		AliasPath:      trace.AliasPath,
		Prepare:        trace.Prepare,
		RefContext:     trace.RefContext,
		NormalizedArgs: append([]string{}, trace.NormalizedArgs...),
		Inputs:         append([]cli.CacheExplainInput{}, trace.Inputs...),
		Cache: provenanceCache{
			Decision:        response.Decision,
			ReasonCode:      response.ReasonCode,
			Signature:       response.Signature,
			MatchedStateID:  response.MatchedStateID,
			ResolvedImageID: response.ResolvedImageID,
		},
		Outcome: outcome,
	}
}

func resolveProvenancePath(invocationCwd string, path string) string {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return ""
	}
	if filepath.IsAbs(cleaned) {
		return filepath.Clean(cleaned)
	}
	base := strings.TrimSpace(invocationCwd)
	if base == "" {
		base = "."
	}
	return filepath.Clean(filepath.Join(base, cleaned))
}

func writeProvenanceArtifact(path string, artifact provenanceArtifact) error {
	target := resolveProvenancePath(artifact.CWD, path)
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("provenance path is required")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	return os.WriteFile(target, append(data, '\n'), 0o600)
}

func executePlanStageWithProvenance(stdout io.Writer, runtime stageRuntime, output string, provenancePath string) error {
	if strings.TrimSpace(provenancePath) == "" {
		return executePlanStage(stdout, runtime, output)
	}
	if err := ensurePrepareTrace(&runtime); err != nil {
		return finishPrepareCleanup(err, runtime.cleanup)
	}

	explainOpts := runtime.opts
	explainOpts.PlanOnly = false

	// Provenance keeps the prepare-oriented cache decision that a later prepare
	// would reuse, even when the current command is `plan`.
	explain, err := explainPrepareCacheFn(context.Background(), explainOpts)
	if err != nil {
		return finishPrepareCleanup(err, runtime.cleanup)
	}

	result, err := runPlanStage(runtime)
	if err != nil {
		artifact := provenanceFromTrace(runtime.trace, explain, provenanceOutcome{
			Status:   "failed",
			PlanOnly: true,
			Error:    err.Error(),
		})
		writeErr := writeProvenanceArtifactFn(provenancePath, artifact)
		return finishPrepareCleanup(joinStageErrors(err, writeErr), runtime.cleanup)
	}

	renderErr := renderPlanStage(stdout, output, result)
	artifact := provenanceFromTrace(runtime.trace, explain, provenanceOutcome{
		Status:   "succeeded",
		PlanOnly: true,
		StateID:  finalPlanStateID(result.Tasks),
	})
	writeErr := writeProvenanceArtifactFn(provenancePath, artifact)
	return finishPrepareCleanup(joinStageErrors(renderErr, writeErr), runtime.cleanup)
}

func executePrepareStageWithProvenance(w stdoutAndErr, runtime stageRuntime, provenancePath string) (client.PrepareJobResult, bool, error) {
	if strings.TrimSpace(provenancePath) == "" {
		return executePrepareStage(w, runtime)
	}
	if err := ensurePrepareTrace(&runtime); err != nil {
		return client.PrepareJobResult{}, false, finishPrepareCleanup(err, runtime.cleanup)
	}

	explain, err := explainPrepareCacheFn(context.Background(), runtime.opts)
	if err != nil {
		return client.PrepareJobResult{}, false, finishPrepareCleanup(err, runtime.cleanup)
	}

	stageResult, err := runPrepareStage(w, runtime)
	outcome := provenanceOutcome{}
	switch {
	case err != nil:
		outcome.Status = "failed"
		outcome.PlanOnly = false
		outcome.Error = err.Error()
	case stageResult.accepted != nil:
		outcome.Status = "succeeded"
		outcome.JobID = stageResult.accepted.JobID
	default:
		outcome.Status = "succeeded"
		outcome.StateID = stageResult.result.StateID
	}

	writeErr := writeProvenanceArtifactFn(provenancePath, provenanceFromTrace(runtime.trace, explain, outcome))
	finalErr := finishPrepareCleanup(joinStageErrors(err, writeErr), runtime.cleanup)
	return stageResult.result, stageResult.handled, finalErr
}

func joinStageErrors(primary error, secondary error) error {
	switch {
	case primary == nil:
		return secondary
	case secondary == nil:
		return primary
	default:
		return fmt.Errorf("%s; %s", primary.Error(), secondary.Error())
	}
}

func finalPlanStateID(tasks []client.PlanTask) string {
	for i := len(tasks) - 1; i >= 0; i-- {
		task := tasks[i]
		switch task.Type {
		case "prepare_instance":
			if task.Input != nil && task.Input.Kind == "state" && task.Input.ID != "" {
				return task.Input.ID
			}
		case "state_execute":
			if task.OutputStateID != "" {
				return task.OutputStateID
			}
		}
	}
	return ""
}
