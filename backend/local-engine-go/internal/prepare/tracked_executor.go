package prepare

import (
	"context"
	"fmt"
	"time"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// trackedAttemptJournal is the narrow durable capability needed to cross the
// Docker effect boundary. The name is already planned on the bound attempt.
type trackedAttemptJournal interface {
	RequestAttemptCreate(context.Context, queue.AttemptRecord, int) error
	AttachAttemptExecutor(context.Context, queue.AttemptRecord, int, string) error
	RequestAttemptStart(context.Context, queue.AttemptRecord, int, string) error
	ObserveAttemptExit(context.Context, queue.AttemptRecord, int, string, int, string) (int64, error)
}

type trackedContainerRuntime interface {
	CreateContainer(context.Context, engineRuntime.RunRequest) (string, error)
	StartContainer(context.Context, string) error
	WaitContainer(context.Context, string) (string, int, error)
}

type trackedExecutorResult struct {
	Output         string
	ExitCode       int
	ExitEvidenceID int64
	ExecutorID     string
}

// runTrackedExecutor never infers database effects from a process exit. It
// orders planned name, created ID, start request, and observed exit. A saved
// second attempt may resume at create_requested or created only after exact
// name/ID inspection; it never allocates a third attempt. Any error leaves
// the attempt open for inspection and clone quiescence; callers must neither
// replay nor clean its credential then.
// See A01-A03/I02-I04 in the recovery test design.
func runTrackedExecutor(ctx context.Context, journal trackedAttemptJournal, runtime trackedContainerRuntime,
	attempt queue.AttemptRecord, request engineRuntime.RunRequest, now func() string) (trackedExecutorResult, error) {
	if journal == nil || runtime == nil || now == nil || attempt.JobID == "" ||
		attempt.TaskID == "" || attempt.CloneID == "" || attempt.ExecutionDigest == "" ||
		attempt.AttemptNo <= 0 || attempt.PlannedExecutorName == "" ||
		!(attempt.Phase == "planned" || attempt.AttemptNo == 2 &&
			(attempt.Phase == "create_requested" || attempt.Phase == "created")) ||
		request.ImageID == "" {
		return trackedExecutorResult{}, fmt.Errorf("bound planned attempt and tracked runtime are required")
	}
	request.Name = attempt.PlannedExecutorName
	if attempt.TaskID == "plan" {
		// Planning output is protected capture, never a diagnostic log. Strip
		// an inherited sink before any Docker operation (C05b).
		ctx = engineRuntime.WithoutLogSink(ctx)
	}
	var id string
	if attempt.Phase != "planned" {
		inspector, ok := runtime.(interface {
			InspectNamedContainer(context.Context, string) (engineRuntime.ContainerInspection, bool, error)
		})
		if !ok {
			return trackedExecutorResult{}, fmt.Errorf("saved executor inspection is unavailable")
		}
		inspection, found, err := inspector.InspectNamedContainer(ctx, attempt.PlannedExecutorName)
		if err != nil {
			return trackedExecutorResult{}, err
		}
		if attempt.Phase == "create_requested" && (found || attempt.ExecutorID != "") {
			return trackedExecutorResult{}, fmt.Errorf("unowned executor may have been created")
		}
		if attempt.Phase == "created" {
			if !found || attempt.ExecutorID == "" || inspection.ID != attempt.ExecutorID ||
				inspection.Status != "created" {
				return trackedExecutorResult{}, fmt.Errorf("saved executor is not exactly created")
			}
			id = attempt.ExecutorID
		}
	}
	if attempt.Phase != "created" {
		if attempt.Phase == "planned" {
			if err := journal.RequestAttemptCreate(ctx, attempt, attempt.WorkerGeneration); err != nil {
				return trackedExecutorResult{}, err
			}
		}
		var err error
		id, err = runtime.CreateContainer(ctx, request)
		if err != nil {
			return trackedExecutorResult{}, err
		}
		if err := journal.AttachAttemptExecutor(ctx, attempt, attempt.WorkerGeneration, id); err != nil {
			return trackedExecutorResult{ExecutorID: id}, err
		}
	}
	if err := journal.RequestAttemptStart(ctx, attempt, attempt.WorkerGeneration, now()); err != nil {
		return trackedExecutorResult{ExecutorID: id}, err
	}
	if err := runtime.StartContainer(ctx, id); err != nil {
		return trackedExecutorResult{ExecutorID: id}, err
	}
	output, exitCode, err := runtime.WaitContainer(ctx, id)
	if err != nil {
		return trackedExecutorResult{Output: output, ExecutorID: id}, err
	}
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	evidenceID, err := journal.ObserveAttemptExit(commitCtx, attempt,
		attempt.WorkerGeneration, id, exitCode, now())
	if err != nil {
		return trackedExecutorResult{Output: output, ExitCode: exitCode, ExecutorID: id}, err
	}
	return trackedExecutorResult{Output: output, ExitCode: exitCode, ExitEvidenceID: evidenceID, ExecutorID: id}, nil
}
