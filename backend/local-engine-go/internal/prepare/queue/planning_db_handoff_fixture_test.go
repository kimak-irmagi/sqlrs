package queue_test

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// Consumer assertions keep PG02–PG07 buildable and red before the approved O5
// writers exist. The callback represents the trusted read-only DB/identity
// observation, not an extension point exposed to a client or a fault injector.
// Requirements: issue89-planning-db-handoff-schema.md §§2–5.
type retainedPlanningWriter interface {
	RecordPlanningDBRetention(context.Context, queue.AttemptRecord, int, instanceaccess.Operation, func(context.Context, queue.PlanningRecoverySnapshot, instanceaccess.Operation) error, string) (int64, error)
	CompleteRetainedPlanning(context.Context, queue.AttemptRecord, int, int64, int64, int64, string) error
}

type planningDBTransferWriter interface {
	TransferPlanningDBToFirstSession(context.Context, queue.LiquibaseSessionLaunch) (queue.LiquibaseSessionRecord, error)
}

type planningDBDisposalWriter interface {
	RequestPlanningDBDisposal(context.Context, queue.AttemptRecord, int, string, string) (int64, error)
	CompletePlanningDBDisposal(context.Context, queue.AttemptRecord, int, instanceaccess.Operation, queue.CloneAbsenceVerifier, string) error
}

type planningDBFixture struct {
	db                     *sql.DB
	q                      *queue.SQLiteStore
	attempt                queue.AttemptRecord
	clone                  queue.ClonePlan
	op                     instanceaccess.Operation
	access                 *instanceaccess.Service
	record                 queue.LiquibaseExecutionRecord
	capture                queue.PlanningCaptureNames
	exit, retained, output int64
	planDigest             string
	changes                []planningcodec.Changeset
	observationCalls       int
}

func pgDigest(char string) string { return "sha256:" + strings.Repeat(char, 64) }

// Real normal admission, planner milestones and managed-operation assignment;
// no direct insert, disabled guard, DB-absence fact or synthetic released clone.
// The other job supplied by mainPathSessionJournal is intentionally left intact
// so complete-journal comparisons also detect cross-job changes.
func newPlanningDBFixture(t *testing.T, exitCode int, configure ...func(*queue.JobRecord)) *planningDBFixture {
	t.Helper()
	return newPlanningDBFixtureAtExit(t, &exitCode, configure...)
}

// A nil exit code leaves the actual planner running for PG02's live-child
// rejection control; the existing stopped-fixture path is unchanged.
func newPlanningDBFixtureAtExit(t *testing.T, exitCode *int, configure ...func(*queue.JobRecord)) *planningDBFixture {
	t.Helper()
	return newPlanningDBFixtureForSource(t, exitCode, nil, configure...)
}

// PG07 may select a genuinely published state as the immutable planning source.
// The default image fixture and all its existing callers retain their inputs.
func newPlanningDBFixtureForSource(t *testing.T, exitCode *int, source func(*planningDBFixture) string, configure ...func(*queue.JobRecord)) *planningDBFixture {
	return newPlanningDBFixtureForClone(t, exitCode, source, nil, configure...)
}

// The publication control uses the actual planner directory layout. Other
// fixtures retain their original immutable clone references.
func newPlanningDBFixtureForClone(t *testing.T, exitCode *int, source func(*planningDBFixture) string, configureClone func(*queue.ClonePlan), configure ...func(*queue.JobRecord)) *planningDBFixture {
	t.Helper()
	db, q, _, _ := mainPathSessionJournal(t)
	job, found, err := q.GetJob(t.Context(), "main-path-job")
	if err != nil || !found {
		t.Fatal("base job", err)
	}
	record, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID)
	if err != nil || !found {
		t.Fatal("base binding", err)
	}
	job.JobID, job.Status, job.StartedAt, job.Signature = "planning-handoff", "queued", nil, nil
	for _, configureJob := range configure {
		configureJob(&job)
	}
	handoffCheck(t, q.AdmitLiquibaseJob(t.Context(), job, record.RequestedTag, record.BundleManifestDigest))
	handoffCheck(t, q.BindLiquibaseExecution(t.Context(), job.JobID, record))
	running := "running"
	handoffCheck(t, q.UpdateJob(t.Context(), job.JobID, queue.JobUpdate{Status: &running}))
	binding, err := queue.ProjectLiquibasePlanningBinding(record, `["updateSQL"]`, pgDigest("6"))
	handoffCheck(t, err)
	clone := queue.ClonePlan{ID: "retained-preview", Ref: "jobs/planning-handoff/runtime_retained", PlannedRuntimeName: "sqlrs-db-retained", Generation: 1, SourceKind: "image", SourceImageID: job.ResolvedImageID}
	if configureClone != nil {
		configureClone(&clone)
	}
	if source != nil {
		clone.SourceKind, clone.SourceImageID = "state", ""
		clone.SourceStateID = source(&planningDBFixture{db: db, q: q, record: record})
	}
	capture := queue.PlanningCaptureNames{ProtectedInput: "retained-input", Stdout: "retained-stdout", Stderr: "retained-stderr", Manifest: "retained-manifest", HelperDigest: pgDigest("6"), ProtocolRevision: planningcodec.CaptureProtocolRevision}
	launch := queue.PlanningLaunch{BoundLaunch: queue.BoundLaunch{
		Attempt: queue.AttemptPlan{JobID: job.JobID, TaskID: "plan", CloneID: clone.ID, ExecutionDigest: binding.Digest, WorkerGeneration: 1, CreatedAt: mainPathJournalUTC},
		Binding: binding, Clone: clone, Leases: []queue.ArtifactLease{{ID: "retained-helper", Kind: "capture_helper", Digest: pgDigest("6")}},
	}, Capture: capture}
	a, err := q.PlanBoundPlanningAttempt(t.Context(), launch)
	handoffCheck(t, err)
	root, err := filepath.EvalSymlinks(t.TempDir())
	handoffCheck(t, err)
	secrets, err := instanceaccess.OpenSecrets(filepath.Join(root, "secrets"))
	handoffCheck(t, err)
	t.Cleanup(func() { _ = secrets.Close() })
	var domain string
	handoffCheck(t, db.QueryRow("SELECT domain_ref FROM managed_store_format WHERE slot=1").Scan(&domain))
	access, err := instanceaccess.NewServiceWithPublicationPins(db, secrets, domain, q)
	handoffCheck(t, err)
	id, err := managedidentity.Generate(bytes.NewReader(bytes.Repeat([]byte{0x35}, 16)))
	handoffCheck(t, err)
	// Match observeStartManagedRuntimeWithPlan: Source is the immutable input
	// ID, while Target is the physical private directory. Neither may be rewritten.
	sourceID := clone.SourceImageID
	if clone.SourceKind == "state" {
		sourceID = clone.SourceStateID
	}
	op, err := access.Assign(t.Context(), "runtime_retained", filepath.Join(root, filepath.FromSlash(clone.Ref)), sourceID, id)
	handoffCheck(t, err)
	dbID := strings.Repeat("8", 64)
	handoffCheck(t, access.Attach(t.Context(), op, managedidentity.RuntimeBinding{RuntimeRef: dbID, PhysicalIdentity: op.PhysicalIdentity, IdentityBinding: id}))
	op.RuntimeRef, op.Stage = dbID, "running"
	handoffCheck(t, q.ActivatePlanningClone(t.Context(), a, 1, clone.PlannedRuntimeName, dbID))
	handoffCheck(t, q.RequestAttemptCreate(t.Context(), a, 1))
	a.ExecutorID = strings.Repeat("9", 64)
	handoffCheck(t, q.AttachAttemptExecutor(t.Context(), a, 1, a.ExecutorID))
	handoffCheck(t, q.RequestAttemptStart(t.Context(), a, 1, mainPathJournalUTC))
	var exit int64
	if exitCode != nil {
		exit, err = q.ObserveAttemptExit(t.Context(), a, 1, a.ExecutorID, *exitCode, mainPathJournalUTC)
		handoffCheck(t, err)
	}
	db.SetMaxOpenConns(1)
	return &planningDBFixture{db: db, q: q, attempt: a, clone: clone, op: op, access: access, record: record, capture: capture, exit: exit,
		changes: []planningcodec.Changeset{{ID: "one", Author: "sqlrs", Path: "db/main.sql", SQLHash: strings.Repeat("4", 64)}, {ID: "two", Author: "sqlrs", Path: "db/main.sql", SQLHash: strings.Repeat("5", 64)}, {ID: "three", Author: "sqlrs", Path: "db/main.sql", SQLHash: strings.Repeat("6", 64)}}}
}

func (f *planningDBFixture) writer(t *testing.T) retainedPlanningWriter {
	t.Helper()
	w, ok := any(f.q).(retainedPlanningWriter)
	if !ok {
		t.Fatal("approved retained-planning writers are not implemented")
	}
	return w
}

// This is a metadata observation double, not a claim of PostgreSQL/Docker
// verification. PG09 supplies the native proof. With a one-connection pool this
// read also proves that the callback is not running inside a write transaction.
func (f *planningDBFixture) observe(ctx context.Context, saved queue.PlanningRecoverySnapshot, op instanceaccess.Operation) error {
	f.observationCalls++
	if saved.Attempt.AttemptPlan != f.attempt.AttemptPlan || saved.Attempt.AttemptNo != 1 || saved.Attempt.ExecutorID != f.attempt.ExecutorID || saved.Attempt.Phase != "observed_stopped" || saved.Clone != f.clone || saved.ClonePhase != "active" || saved.CloneRuntimeID != f.op.RuntimeRef || op != f.op {
		return queue.ErrPlanningSchema
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var n int
	if err := f.db.QueryRowContext(ctx, `SELECT count(*) FROM managed_runtime_operations
WHERE operation_ref=? AND target_path=? AND source_ref=? AND physical_identity=? AND runtime_ref=? AND stage='running'`, op.Ref, op.Target, op.Source, op.PhysicalIdentity, op.RuntimeRef).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return queue.ErrPlanningSchema
	}
	return nil
}

func (f *planningDBFixture) saveRetainedPlan(t *testing.T) {
	t.Helper()
	w := f.writer(t)
	var err error
	f.retained, err = w.RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC)
	handoffCheck(t, err)
	f.output, err = f.q.RecordVerifiedPlanningCapture(t.Context(), f.attempt, 1, queue.PlanningCaptureProof{StdoutDigest: pgDigest("2"), StderrDigest: pgDigest("3"), StdoutBytes: 100, ManifestRef: f.capture.Manifest}, mainPathJournalUTC)
	handoffCheck(t, err)
	f.planDigest, err = f.q.SavePlanningResult(t.Context(), f.attempt.JobID, 1, pgDigest("2"), f.changes, mainPathJournalUTC)
	handoffCheck(t, err)
}

func (f *planningDBFixture) complete(t *testing.T) {
	t.Helper()
	f.saveRetainedPlan(t)
	handoffCheck(t, f.writer(t).CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
}

func (f *planningDBFixture) retireCapture(t *testing.T) *mainPathPreviewAbsence {
	t.Helper()
	_, err := f.q.SavePrivatePlanningCaptureDisposal(t.Context(), f.attempt, 1, "plan_saved", f.planDigest, mainPathJournalUTC)
	handoffCheck(t, err)
	probe := &mainPathPreviewAbsence{db: f.db, attempt: f.attempt, clone: f.clone, dbID: f.op.RuntimeRef, input: f.capture.ProtectedInput}
	handoffCheck(t, f.q.RetirePrivatePlanningCapture(t.Context(), f.attempt, 1, probe, mainPathJournalUTC))
	handoffCheck(t, f.q.RetirePlanningArtifact(t.Context(), f.attempt, 1, "input", probe, mainPathJournalUTC))
	return probe
}

func (f *planningDBFixture) firstLaunch(t *testing.T) queue.LiquibaseSessionLaunch {
	t.Helper()
	return f.firstLaunchWithStates(t, []string{strings.Repeat("5", 64), strings.Repeat("6", 64), strings.Repeat("7", 64)})
}

// PG06 can use checkpoints published by the unrelated real session instead of
// synthesizing cache flags without matching state/provenance rows.
func (f *planningDBFixture) firstLaunchWithStates(t *testing.T, states []string) queue.LiquibaseSessionLaunch {
	t.Helper()
	return f.firstLaunchFromSource(t, states, "image", f.clone.SourceImageID)
}

// CP2-T preserves default image fixtures while accepting the actual immutable
// state source for the positive retained-state transfer control.
func (f *planningDBFixture) firstLaunchFromSource(t *testing.T, states []string, sourceKind, sourceID string) queue.LiquibaseSessionLaunch {
	t.Helper()
	image, state := sourceKind, "state"
	tasks := []queue.TaskRecord{{JobID: f.attempt.JobID, TaskID: "plan", Position: 0, Type: "plan", Status: "succeeded"}}
	for i, name := range []string{"one", "two", "three"} {
		kind, input := image, sourceID
		if i > 0 {
			kind, input = state, states[i-1]
		}
		change := f.changes[i]
		tasks = append(tasks, queue.TaskRecord{JobID: f.attempt.JobID, TaskID: name, Position: i + 1, Type: "state_execute", Status: "queued", InputKind: &kind, InputID: &input, OutputStateID: &states[i], TaskHash: &change.SQLHash, ChangesetID: &change.ID, ChangesetAuthor: &change.Author, ChangesetPath: &change.Path})
	}
	tasks = append(tasks, queue.TaskRecord{JobID: f.attempt.JobID, TaskID: "prepare-instance", Position: 4, Type: "prepare_instance", Status: "queued", InputKind: &state, InputID: &states[2]})
	handoffCheck(t, f.q.AppendPlanningSuffix(t.Context(), f.attempt.JobID, tasks))
	running := "running"
	handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, "one", queue.TaskUpdate{Status: &running}))
	execution := queue.ExecutionBinding{Digest: pgDigest("9"), PrepareKind: "lb", CommandDigest: pgDigest("9"), InputManifestDigest: f.record.BundleManifestDigest, ExecutorImageDigest: f.record.ImageManifestDigest, TargetIdentityDigest: f.record.TargetIdentityDigest, PolicyRevision: "lb-container-v2", EffectScope: "unknown", AdapterBindingDigest: f.record.ExecutionFingerprint}
	handoffCheck(t, f.q.BindStep(t.Context(), f.attempt.JobID, "one", execution, mainPathJournalUTC))
	return queue.LiquibaseSessionLaunch{Session: queue.LiquibaseSessionRecord{SessionID: "retained-session", JobID: f.attempt.JobID, CloneID: f.clone.ID, SessionNo: 1, WorkerGeneration: 1, ExecutionFingerprint: f.record.ExecutionFingerprint, StartKind: image, StartID: sourceID, StartPosition: 1, PlannedRunnerName: "sqlrs-lb-retained-session", RunnerConfigDigest: pgDigest("8"), CaptureHelperDigest: pgDigest("6"), ProtectedConfigRef: "retained-session-input", CreatedAt: mainPathJournalUTC}, Clone: f.clone, FirstTaskID: "one", ExecutionDigest: execution.Digest, HelperLeaseID: "retained-session-helper"}
}

func (f *planningDBFixture) unchanged(t *testing.T, before map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("refusal changed the journal, including the unrelated job")
	}
}

func (f *planningDBFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	handoffCheck(t, f.db.QueryRow(query, args...).Scan(&n))
	return n
}
