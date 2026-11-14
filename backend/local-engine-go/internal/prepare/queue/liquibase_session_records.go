package queue

import (
	"context"
	"errors"
)

// These are the approved session journal records consumed by the normal-path
// coordinator, extracted without installing a schema or enabling recovery.
// See docs/architecture/liquibase-container-component-structure.md.

// ErrCommonAttempt rejects a stale, unbound, or duplicate launch transition.
var ErrCommonAttempt = errors.New("invalid common transformation attempt transition")

// LiquibaseSessionRecord is the saved runner identity and moving immutable
// input cursor. Runtime handles do not replace this journal on recovery. See
// docs/architecture/liquibase-container-component-structure.md.
type LiquibaseSessionRecord struct {
	SessionID, JobID, CloneID                                  string
	SessionNo, WorkerGeneration                                int
	ExecutionFingerprint, StartKind, StartID                   string
	StartPosition                                              int
	PlannedRunnerName, RunnerConfigDigest, CaptureHelperDigest string
	ProtectedConfigRef, RunnerID, RunnerPhase, DBPhase         string
	CursorKind, CursorID                                       string
	CursorPosition                                             int
	CreatedAt                                                  string
	RetiredAt                                                  *string
}

// LiquibaseInvocationPlan allocates private capture references before a child
// command starts. References name engine artifacts, never physical paths.
type LiquibaseInvocationPlan struct {
	InvocationRef, StdoutRef, StderrRef, ManifestRef string
	ProtocolRevision, CreatedAt                      string
}

// LiquibaseInvocationRecord binds one common session_exec attempt to its
// exact parent runner and private output protocol. See the reusable-container
// storage schema, ordered transitions 2 and 3.
type LiquibaseInvocationRecord struct {
	LiquibaseInvocationPlan
	SessionID, HelperDigest string
	Attempt                 AttemptRecord
	// The reservation is durable, not inferred from a directory's existence.
	CaptureSlot        int
	PriorRetirementRef *string
}

// LiquibaseInvocationCompletion contains only verified manifest identity and
// child exit. Protected SQL and stream bytes never enter the SQLite journal.
type LiquibaseInvocationCompletion struct {
	ExitCode       int
	ManifestDigest string
	ManifestBytes  int64
}

// AttemptPlan is the already resolved, immutable identity of one mutating
// step. The coordinator must retain its clone and input closure before use.
// See docs/architecture/liquibase-container-component-structure.md.
type AttemptPlan struct {
	JobID, TaskID, CloneID, ExecutionDigest string
	WorkerGeneration                        int
	CreatedAt                               string
}

// AttemptRecord is the durable locator used after a crash, including when
// Docker create returned but its ID write was lost.
type AttemptRecord struct {
	AttemptPlan
	AttemptNo           int
	PlannedExecutorName string
	ExecutorID          string
	Phase               string
	StartedAt           string
	FinishedAt          string
	Resolution          string
	LegacyUnbound       bool
}

// CapturePrunePlan projects committed publication/slot authority. It contains
// identities only, never a caller-selected path or executable. VerifiedAt and
// AbsenceKind describe immutable history, not permission to execute SQL again.
// Requirements: liquibase-private-capture-ownership.md §9, MP01–MP04.
type CapturePrunePlan struct {
	RetirementRef, SessionID, RunnerID, HelperDigest           string
	WorkerGeneration                                           int
	PriorInvocationRef, PriorManifestDigest, NextInvocationRef string
	VerifiedAt, AbsenceKind                                    string
	priorJob, priorTask                                        string
	priorAttempt, priorSlot                                    int
}

// LiquibaseCapturePruneVerifier performs only a bounded exact-parent absence
// observation. The caller retains session exclusion; no SQL transaction or
// eviction lock may be held across this callback. §9, MP03/MP04.
type LiquibaseCapturePruneVerifier interface {
	VerifyLiquibaseCapturePruned(context.Context, CapturePrunePlan) error
}
