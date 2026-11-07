package queue_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/managedstore"
	"github.com/sqlrs/engine-local/internal/prepare/finalizationcodec"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/statefs"
	enginestore "github.com/sqlrs/engine-local/internal/store"
)

// This consumer assertion keeps the first red tests buildable before adding
// the approved queue capability. It is not a production extension mechanism.
type handoffPublisher interface {
	PublishWorkingInstanceAndBeginFinalization(context.Context, queue.LiquibaseSessionRecord, enginestore.InstanceCreate, managedidentity.RuntimeBinding, instanceaccess.Operation) (queue.FinalizationSnapshot, error)
}

type workingHandoffFixture struct {
	db      *sql.DB
	q       *queue.SQLiteStore
	session queue.LiquibaseSessionRecord
	entry   enginestore.InstanceCreate
	binding managedidentity.RuntimeBinding
	op      instanceaccess.Operation
	access  *instanceaccess.Service
}

func handoffCheck(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s:%d: %v", filepath.Base(file), line, err)
	}
}

// The fixture goes through real preview completion/disposal, working admission,
// SQL publication and managed-access writers. Native work is a separate WH08 test.
func newWorkingHandoffFixture(t *testing.T) workingHandoffFixture {
	t.Helper()
	return newWorkingHandoffFixtureSteps(t, 3)
}

func newWorkingHandoffFixtureSteps(t *testing.T, steps int) workingHandoffFixture {
	t.Helper()
	db, q, _, _ := mainPathSessionJournal(t)
	job, found, err := q.GetJob(t.Context(), "main-path-job")
	if err != nil || !found {
		t.Fatal(err)
	}
	record, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID)
	if err != nil || !found {
		t.Fatal(err)
	}
	job.JobID, job.Status = "working-handoff", "queued"
	job.Signature = nil
	handoffCheck(t, q.AdmitLiquibaseJob(t.Context(), job, record.RequestedTag, record.BundleManifestDigest))
	handoffCheck(t, q.BindLiquibaseExecution(t.Context(), job.JobID, record))
	running := "running"
	handoffCheck(t, q.UpdateJob(t.Context(), job.JobID, queue.JobUpdate{Status: &running}))
	digest := func(c string) string { return "sha256:" + strings.Repeat(c, 64) }
	b, err := queue.ProjectLiquibasePlanningBinding(record, `["updateSQL"]`, digest("6"))
	handoffCheck(t, err)
	preview := queue.ClonePlan{ID: "handoff-preview", Ref: "jobs/working-handoff/preview", PlannedRuntimeName: "sqlrs-db-handoff-preview", Generation: 1, SourceKind: "image", SourceImageID: job.ResolvedImageID}
	launch := queue.PlanningLaunch{BoundLaunch: queue.BoundLaunch{Attempt: queue.AttemptPlan{JobID: job.JobID, TaskID: "plan", CloneID: preview.ID, ExecutionDigest: b.Digest, WorkerGeneration: 1, CreatedAt: mainPathJournalUTC}, Binding: b, Clone: preview, Leases: []queue.ArtifactLease{{ID: "handoff-preview-helper", Kind: "capture_helper", Digest: digest("6")}}}, Capture: queue.PlanningCaptureNames{ProtectedInput: "handoff-input", Stdout: "handoff-out", Stderr: "handoff-err", Manifest: "handoff-manifest", HelperDigest: digest("6"), ProtocolRevision: planningcodec.CaptureProtocolRevision}}
	a, err := q.PlanBoundPlanningAttempt(t.Context(), launch)
	handoffCheck(t, err)
	previewDB, previewRunner := strings.Repeat("8", 64), strings.Repeat("9", 64)
	handoffCheck(t, q.ActivatePlanningClone(t.Context(), a, 1, preview.PlannedRuntimeName, previewDB))
	handoffCheck(t, q.RequestAttemptCreate(t.Context(), a, 1))
	handoffCheck(t, q.AttachAttemptExecutor(t.Context(), a, 1, previewRunner))
	a.ExecutorID = previewRunner
	handoffCheck(t, q.RequestAttemptStart(t.Context(), a, 1, mainPathJournalUTC))
	exit, err := q.ObserveAttemptExit(t.Context(), a, 1, previewRunner, 0, mainPathJournalUTC)
	handoffCheck(t, err)
	quiet, err := q.RecordPlanningCloneQuiescence(t.Context(), a, 1, preview.PlannedRuntimeName, previewDB, mainPathJournalUTC)
	handoffCheck(t, err)
	output, err := q.RecordVerifiedPlanningCapture(t.Context(), a, 1, queue.PlanningCaptureProof{StdoutDigest: digest("2"), StderrDigest: digest("3"), StdoutBytes: 4, ManifestRef: launch.Capture.Manifest}, mainPathJournalUTC)
	handoffCheck(t, err)
	metadata := []planningcodec.Changeset{{ID: "one", Author: "sqlrs", Path: "db/main.sql", SQLHash: strings.Repeat("4", 64)}, {ID: "two", Author: "sqlrs", Path: "db/main.sql", SQLHash: strings.Repeat("5", 64)}, {ID: "three", Author: "sqlrs", Path: "db/main.sql", SQLHash: strings.Repeat("6", 64)}}
	planDigest, err := q.SavePlanningResult(t.Context(), job.JobID, 1, digest("2"), metadata, mainPathJournalUTC)
	handoffCheck(t, err)
	handoffCheck(t, q.CompletePlanning(t.Context(), a, 1, exit, quiet, output, mainPathJournalUTC))
	_, err = q.SavePrivatePlanningCaptureDisposal(t.Context(), a, 1, "plan_saved", planDigest, mainPathJournalUTC)
	handoffCheck(t, err)
	absence := &mainPathPreviewAbsence{db: db, attempt: a, clone: preview, dbID: previewDB, input: launch.Capture.ProtectedInput}
	handoffCheck(t, q.RetirePrivatePlanningCapture(t.Context(), a, 1, absence, mainPathJournalUTC))
	handoffCheck(t, q.RetirePlanningArtifact(t.Context(), a, 1, "input", absence, mainPathJournalUTC))
	handoffCheck(t, q.ReleasePlanningClone(t.Context(), a, 1, absence))
	imageKind, stateKind := "image", "state"
	states := []string{strings.Repeat("5", 64), strings.Repeat("6", 64), strings.Repeat("7", 64)}
	tasks := []queue.TaskRecord{}
	for i, state := range states {
		kind, input := imageKind, job.ResolvedImageID
		if i > 0 {
			kind, input = stateKind, states[i-1]
		}
		tasks = append(tasks, queue.TaskRecord{JobID: job.JobID, TaskID: []string{"one", "two", "three"}[i], Position: i + 1, Type: "state_execute", Status: "queued", InputKind: &kind, InputID: &input, OutputStateID: &state})
	}
	finalState := states[2]
	tasks = append(tasks, queue.TaskRecord{JobID: job.JobID, TaskID: "prepare-instance", Position: 4, Type: "prepare_instance", Status: "queued", InputKind: &stateKind, InputID: &finalState})
	handoffCheck(t, q.AppendPlanningSuffix(t.Context(), job.JobID, append([]queue.TaskRecord{{JobID: job.JobID, TaskID: "plan", Position: 0, Type: "plan", Status: "succeeded"}}, tasks...)))
	execution := queue.ExecutionBinding{Digest: digest("9"), PrepareKind: "lb", CommandDigest: digest("9"), InputManifestDigest: record.BundleManifestDigest, ExecutorImageDigest: record.ImageManifestDigest, TargetIdentityDigest: record.TargetIdentityDigest, PolicyRevision: "lb-container-v2", EffectScope: "unknown", AdapterBindingDigest: record.ExecutionFingerprint}
	handoffCheck(t, q.UpdateTask(t.Context(), job.JobID, "one", queue.TaskUpdate{Status: &running}))
	handoffCheck(t, q.BindStep(t.Context(), job.JobID, "one", execution, mainPathJournalUTC))
	session := queue.LiquibaseSessionRecord{SessionID: "handoff-session", JobID: job.JobID, CloneID: "handoff-clone", SessionNo: 1, WorkerGeneration: 1, ExecutionFingerprint: record.ExecutionFingerprint, StartKind: "image", StartID: job.ResolvedImageID, StartPosition: 1, PlannedRunnerName: "sqlrs-lb-handoff", RunnerConfigDigest: digest("8"), CaptureHelperDigest: digest("6"), ProtectedConfigRef: "handoff-config", CreatedAt: mainPathJournalUTC}
	opRef := "runtime_handoff"
	root := t.TempDir()
	clone := queue.ClonePlan{ID: session.CloneID, Ref: "jobs/working-handoff/" + opRef, PlannedRuntimeName: "sqlrs-db-handoff", Generation: 2, SourceKind: "image", SourceImageID: job.ResolvedImageID}
	session, err = q.PlanLiquibaseSession(t.Context(), queue.LiquibaseSessionLaunch{Session: session, Clone: clone, FirstTaskID: "one", ExecutionDigest: execution.Digest, HelperLeaseID: "handoff-helper"})
	handoffCheck(t, err)
	database := strings.Repeat("c", 64)
	handoffCheck(t, q.ActivateLiquibaseSessionClone(t.Context(), session, clone.PlannedRuntimeName, database))
	handoffCheck(t, q.RequestLiquibaseSessionRunnerCreate(t.Context(), session))
	session.RunnerID = strings.Repeat("f", 64)
	handoffCheck(t, q.AttachLiquibaseSessionRunner(t.Context(), session, session.RunnerID))
	handoffCheck(t, q.RequestLiquibaseSessionRunnerStart(t.Context(), session))
	handoffCheck(t, q.MarkLiquibaseSessionRunnerRunning(t.Context(), session))
	session.RunnerPhase = "running"
	_, err = q.RecordLiquibaseSessionReadiness(t.Context(), session, mainPathJournalReady{db: db, want: session}, mainPathJournalUTC)
	handoffCheck(t, err)
	identity, err := managedidentity.Generate(bytes.NewReader(bytes.Repeat([]byte{0x35}, 16)))
	handoffCheck(t, err)
	var domain string
	handoffCheck(t, db.QueryRow("SELECT domain_ref FROM managed_store_format WHERE slot=1").Scan(&domain))
	secrets, err := instanceaccess.OpenSecrets(filepath.Join(root, "secrets"))
	handoffCheck(t, err)
	t.Cleanup(func() { _ = secrets.Close() })
	access, err := instanceaccess.NewServiceWithPublicationPins(db, secrets, domain, q)
	handoffCheck(t, err)
	op, err := access.Assign(t.Context(), opRef, filepath.Join(root, "jobs", job.JobID, opRef), session.StartID, identity)
	handoffCheck(t, err)
	binding := managedidentity.RuntimeBinding{RuntimeRef: database, PhysicalIdentity: op.PhysicalIdentity, IdentityBinding: identity}
	handoffCheck(t, access.Attach(t.Context(), op, binding))
	op.RuntimeRef, op.Stage = database, "running"
	for i, task := range tasks[:steps] {
		if i > 0 {
			handoffCheck(t, q.UpdateTask(t.Context(), job.JobID, task.TaskID, queue.TaskUpdate{Status: &running}))
			handoffCheck(t, q.BindStep(t.Context(), job.JobID, task.TaskID, execution, mainPathJournalUTC))
		}
		invocation, err := q.PlanLiquibaseSessionInvocation(t.Context(), session, task.TaskID, execution.Digest, mainPathInvocationPlan("handoff-"+task.TaskID))
		handoffCheck(t, err)
		prune, err := q.ReadLiquibaseCapturePrunePlan(t.Context(), session, invocation)
		handoffCheck(t, err)
		if prune != nil {
			handoffCheck(t, q.ConfirmLiquibaseCapturePruned(t.Context(), session, invocation, &mainPathPruneProbe{db: db, want: *prune}, mainPathJournalUTC))
		}
		handoffCheck(t, q.RequestLiquibaseSessionExec(t.Context(), session, invocation, mainPathJournalUTC))
		complete, err := q.CompleteLiquibaseSessionInvocation(t.Context(), session, invocation, queue.LiquibaseInvocationCompletion{ExitCode: 0, ManifestDigest: digest("e"), ManifestBytes: 500}, mainPathJournalUTC)
		handoffCheck(t, err)
		quiet, err := q.RecordLiquibaseSessionDBStopped(t.Context(), session, invocation, clone.PlannedRuntimeName, database, mainPathJournalUTC)
		handoffCheck(t, err)
		handoffCheck(t, access.RecordSeal(t.Context(), states[i], op, binding))
		dir := filepath.Join(root, states[i])
		handoffCheck(t, os.MkdirAll(dir, 0700))
		handoffCheck(t, os.WriteFile(statefs.PublishedMarkerPath(dir, "copy"), []byte("ok"), 0600))
		entry := enginestore.StateCreate{StateID: states[i], StateFingerprint: states[i], ImageID: job.ResolvedImageID, PrepareKind: "lb", PrepareArgsNormalized: "fixture update-count", CreatedAt: mainPathJournalUTC, LineageRef: job.LineageRef, IdentityDigest: job.IdentityDigest, LiquibaseExecutionFingerprint: session.ExecutionFingerprint}
		if i > 0 {
			entry.ParentStateID = &states[i-1]
		}
		handoffCheck(t, q.PublishLiquibaseSessionState(t.Context(), session, invocation, entry, dir, "copy", complete, quiet, mainPathJournalUTC))
		session, _, err = q.ReadLiquibaseSession(t.Context(), session.SessionID)
		handoffCheck(t, err)
		// Leave the last database stopped so WH04 must exercise the narrow
		// final-publication restart with an already stopped Liquibase parent.
		if i < steps-1 {
			handoffCheck(t, q.RequestLiquibaseSessionDBRestart(t.Context(), session))
			handoffCheck(t, q.MarkLiquibaseSessionDBReady(t.Context(), session, clone.PlannedRuntimeName, database))
			session.DBPhase = "running"
		}
	}
	handoffCheck(t, q.RequestLiquibaseSessionRunnerStop(t.Context(), session))
	handoffCheck(t, q.AcknowledgeLiquibaseSessionRunnerStop(t.Context(), session))
	handoffCheck(t, q.MarkLiquibaseSessionRunnerStopped(t.Context(), session))
	session.RunnerPhase = "stopped"
	handoffCheck(t, q.UpdateTask(t.Context(), job.JobID, "prepare-instance", queue.TaskUpdate{Status: &running}))
	sum := sha256.Sum256([]byte("instance\x00" + job.JobID + "\x00"))
	instanceID := hex.EncodeToString(sum[:])[:32]
	active := enginestore.InstanceStatusActive
	entry := enginestore.InstanceCreate{InstanceID: instanceID, StateID: finalState, ImageID: job.ResolvedImageID, CreatedAt: mainPathJournalUTC, RuntimeID: &database, RuntimeDir: &op.Target, Status: &active}
	return workingHandoffFixture{db: db, q: q, session: session, entry: entry, binding: binding, op: op, access: access}
}

func (f *workingHandoffFixture) ready(t *testing.T) {
	t.Helper()
	handoffCheck(t, f.q.RequestLiquibaseSessionDBRestart(t.Context(), f.session))
	handoffCheck(t, f.q.MarkLiquibaseSessionDBReady(t.Context(), f.session, "sqlrs-db-handoff", f.binding.RuntimeRef))
	f.session.DBPhase = "running"
	handoffCheck(t, f.access.Activate(t.Context(), f.entry.InstanceID, f.binding, func(instanceaccess.Secret) error { return nil }, func() error { return nil }))
}

func TestWorkingInstanceHandoffResume(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	// A stopped parent must not acquire SQL permission through the DB-only resume.
	before := mainPathJournalRows(t, f.db)
	_, err := f.q.PlanLiquibaseSessionInvocation(t.Context(), f.session, "three", "sha256:"+strings.Repeat("9", 64), mainPathInvocationPlan("forbidden-fourth"))
	if err == nil || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("publication restart admitted another SQL invocation", err)
	}
}

func TestWorkingInstanceHandoffResumeRefusesUnfinishedSuffix(t *testing.T) {
	f := newWorkingHandoffFixtureSteps(t, 1)
	before := mainPathJournalRows(t, f.db)
	if err := f.q.RequestLiquibaseSessionDBRestart(t.Context(), f.session); err == nil || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("stopped-parent restart admitted an unfinished suffix", err)
	}
	_, err := f.db.Exec("UPDATE prepare_liquibase_sessions SET db_phase='restart_requested' WHERE session_id=?", f.session.SessionID)
	if err == nil || !strings.Contains(err.Error(), "publication-only resume requires completed suffix") || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("SQL guard did not refuse the unfinished stopped-parent resume", err)
	}
}

func TestWorkingInstanceHandoffPublication(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	publisher, ok := any(f.q).(handoffPublisher)
	if !ok {
		t.Fatal("working-instance publication capability is missing")
	}
	intent, err := publisher.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
	handoffCheck(t, err)
	if intent.InstancePin != f.entry.InstanceID {
		t.Fatal("publication did not retain its exact instance")
	}
	var source, state, runtime, reason string
	var absent sql.NullString
	var uses int
	handoffCheck(t, f.db.QueryRow(`SELECT o.source_ref,i.state_id,i.runtime_id,d.reason,d.db_absent_at,s.use_count FROM instances i JOIN states s ON s.state_id=i.state_id JOIN managed_runtime_operations o ON o.runtime_ref=i.runtime_id JOIN prepare_liquibase_session_disposals d ON d.handoff_instance_id=i.instance_id WHERE i.instance_id=?`, f.entry.InstanceID).Scan(&source, &state, &runtime, &reason, &absent, &uses))
	if source != f.session.StartID || state != f.entry.StateID || source == state || runtime != f.binding.RuntimeRef || reason != "instance_transferred" || absent.Valid || uses != 1 {
		t.Fatal("publication replaced provenance, runtime, absence or use count")
	}
	before := mainPathJournalRows(t, f.db)
	_, err = publisher.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
	handoffCheck(t, err)
	if !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("repeat publication changed committed history")
	}
}

func TestWorkingInstanceHandoffAuthority(t *testing.T) {
	for _, variant := range []string{"session", "job", "clone", "generation", "runner", "state", "database", "target", "source", "physical", "not-stopped", "start-kind", "start-position", "session-number", "session-created"} {
		t.Run(variant, func(t *testing.T) {
			f := newWorkingHandoffFixture(t)
			f.ready(t)
			publisher, ok := any(f.q).(handoffPublisher)
			if !ok {
				t.Fatal("working-instance publication capability is missing")
			}
			s, e, b, o := f.session, f.entry, f.binding, f.op
			switch variant {
			case "session":
				s.SessionID = "foreign"
			case "job":
				s.JobID = "foreign"
			case "clone":
				s.CloneID = "foreign"
			case "generation":
				s.WorkerGeneration++
			case "runner":
				s.RunnerID = strings.Repeat("1", 64)
			case "state":
				e.StateID = strings.Repeat("1", 64)
			case "database":
				b.RuntimeRef = strings.Repeat("1", 64)
			case "target":
				o.Target = filepath.Join(t.TempDir(), "foreign")
			case "source":
				o.Source = e.StateID
			case "physical":
				b.PhysicalIdentity = "physical_foreign"
			case "not-stopped":
				s.RunnerPhase = "running"
			case "start-kind":
				s.StartKind = "state"
			case "start-position":
				s.StartPosition++
			case "session-number":
				s.SessionNo++
			case "session-created":
				s.CreatedAt = "2026-11-07T00:00:00Z"
			}
			before := mainPathJournalRows(t, f.db)
			_, err := publisher.PublishWorkingInstanceAndBeginFinalization(t.Context(), s, e, b, o)
			if !errors.Is(err, queue.ErrLiquibaseFinalization) || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
				t.Fatal("foreign publication did not refuse without effects", err)
			}
		})
	}
}

type handoffAbsence struct {
	db       *sql.DB
	session  queue.LiquibaseSessionRecord
	database string
	calls    int
	err      error
}

func (v *handoffAbsence) VerifyTransferredLiquibaseSession(ctx context.Context, s queue.LiquibaseSessionRecord, clone queue.ClonePlan, database string) error {
	v.calls++
	if s != v.session || clone.ID != s.CloneID || database != v.database {
		return queue.ErrLiquibaseFinalization
	}
	// The pool has one connection: this read would block if the observer were
	// invoked from inside the queue's write transaction.
	var pin string
	if err := v.db.QueryRowContext(ctx, "SELECT instance_pin FROM prepare_liquibase_job_finalizations WHERE job_id=?", s.JobID).Scan(&pin); err != nil {
		return err
	}
	if pin == "" {
		return queue.ErrLiquibaseFinalization
	}
	return v.err
}

func TestWorkingInstanceHandoffPrivateCleanup(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	publisher, ok := any(f.q).(handoffPublisher)
	if !ok {
		t.Fatal("working-instance publication capability is missing")
	}
	_, err := publisher.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
	handoffCheck(t, err)
	probe := &handoffAbsence{db: f.db, session: f.session, database: f.binding.RuntimeRef, err: errors.New("exact parent still present")}
	before := mainPathJournalRows(t, f.db)
	err = f.q.RetireTransferredLiquibaseSession(t.Context(), f.session, probe, mainPathJournalUTC)
	if !errors.Is(err, probe.err) || probe.calls != 1 || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("parent refusal modified ownership", err)
	}
	probe.err = nil
	handoffCheck(t, f.q.RetireTransferredLiquibaseSession(t.Context(), f.session, probe, mainPathJournalUTC))
	var slots, live int
	var absent sql.NullString
	var phase, source string
	handoffCheck(t, f.db.QueryRow("SELECT count(*) FROM prepare_liquibase_capture_slots WHERE session_id=?", f.session.SessionID).Scan(&slots))
	handoffCheck(t, f.db.QueryRow("SELECT count(*) FROM instances WHERE instance_id=? AND runtime_id=?", f.entry.InstanceID, f.binding.RuntimeRef).Scan(&live))
	handoffCheck(t, f.db.QueryRow("SELECT db_absent_at FROM prepare_liquibase_session_disposals WHERE session_id=?", f.session.SessionID).Scan(&absent))
	handoffCheck(t, f.db.QueryRow("SELECT phase FROM prepare_clones WHERE clone_id=?", f.session.CloneID).Scan(&phase))
	handoffCheck(t, f.db.QueryRow("SELECT source_ref FROM managed_runtime_operations WHERE operation_ref=? AND stage='running'", f.op.Ref).Scan(&source))
	if slots != 0 || live != 1 || absent.Valid || phase != "released" || source != f.session.StartID {
		t.Fatal("private retirement destroyed or retained the wrong owner")
	}
	saved, found, err := f.q.ReadLiquibaseSession(t.Context(), f.session.SessionID)
	if err != nil || !found || saved.RetiredAt == nil || saved.RunnerPhase != "retired" || saved.CursorID != "" || saved.DBPhase != "running" {
		t.Fatal("invalid transferred history", err)
	}
}

// WH01: reach the real deferred-FK COMMIT boundary with an otherwise valid
// future-bound transfer. A refused INSERT would not prove this property.
func TestWorkingInstanceHandoffDeferredOutcome(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	_, digest, err := finalizationcodec.Encode(finalizationcodec.Outcome{Kind: finalizationcodec.PrepareSuccess, EventProfile: finalizationcodec.StatusResult, Prepare: &finalizationcodec.PrepareReference{InstanceID: f.entry.InstanceID, StateID: f.entry.StateID}})
	handoffCheck(t, err)
	before := mainPathJournalRows(t, f.db)
	tx, err := f.db.BeginTx(t.Context(), nil)
	handoffCheck(t, err)
	_, err = tx.ExecContext(t.Context(), `INSERT INTO prepare_liquibase_session_disposals(session_id,reason,authority_ref,requested_at,runner_id,db_runtime_id,handoff_job_id,handoff_instance_id,handoff_state_id,handoff_operation_ref,handoff_runtime_dir,handoff_outcome_digest) VALUES(?,'instance_transferred',?,?,?,?,?,?,?,?,?,?)`, f.session.SessionID, digest, f.entry.CreatedAt, f.session.RunnerID, f.binding.RuntimeRef, f.session.JobID, f.entry.InstanceID, f.entry.StateID, f.op.Ref, f.op.Target, digest)
	handoffCheck(t, err)
	err = tx.Commit()
	var coded interface{ Code() int }
	if !errors.As(err, &coded) || coded.Code() != 787 {
		t.Fatal("unpaired transfer did not reach deferred FOREIGN KEY refusal", err)
	}
	var file string
	handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
	peer, err := sql.Open("sqlite", file)
	handoffCheck(t, err)
	defer peer.Close()
	if !reflect.DeepEqual(before, mainPathJournalRows(t, peer)) {
		t.Fatal("peer observed an unpaired committed transfer")
	}
	// SQLite may keep a transaction active after failed COMMIT; clean the exact
	// test handle only after observing peer visibility, not to manufacture it.
	var count int
	handoffCheck(t, f.db.QueryRow("SELECT count(*) FROM prepare_liquibase_session_disposals WHERE session_id=?", f.session.SessionID).Scan(&count))
	if count != 0 {
		_, err = f.db.Exec("ROLLBACK")
		handoffCheck(t, err)
	}
	if !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("explicit test transaction cleanup lost original rows")
	}
}

func TestWorkingInstanceHandoffCommit(t *testing.T) {
	for _, fault := range []string{"after_transfer", "after_instance", "commit_busy"} {
		t.Run(fault, func(t *testing.T) {
			f := newWorkingHandoffFixture(t)
			f.ready(t)
			publisher, ok := any(f.q).(handoffPublisher)
			if !ok {
				t.Fatal("publication capability missing")
			}
			var release func()
			if fault == "commit_busy" {
				_, err := f.db.Exec("PRAGMA busy_timeout=0")
				handoffCheck(t, err)
				var file string
				handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
				peer, err := sql.Open("sqlite", file)
				handoffCheck(t, err)
				defer peer.Close()
				reader, err := peer.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
				handoffCheck(t, err)
				var rows int
				handoffCheck(t, reader.QueryRow("SELECT count(*) FROM instances").Scan(&rows))
				release = func() { handoffCheck(t, reader.Rollback()) }
			} else {
				target, event := "prepare_liquibase_job_finalizations", "INSERT"
				if fault == "after_instance" {
					target, event = "instances", "INSERT"
				}
				when := "NEW.job_id='working-handoff'"
				if fault == "after_instance" {
					when = "NEW.instance_id='" + f.entry.InstanceID + "'"
				}
				timing := "BEFORE"
				if fault == "after_instance" {
					timing = "AFTER"
				}
				_, err := f.db.Exec("CREATE TRIGGER wh_refuse " + timing + " " + event + " ON " + target + " WHEN " + when + " BEGIN SELECT RAISE(ABORT,'WH precise publication boundary'); END")
				handoffCheck(t, err)
				release = func() { _, err := f.db.Exec("DROP TRIGGER wh_refuse"); handoffCheck(t, err) }
			}
			before := mainPathJournalRows(t, f.db)
			_, err := publisher.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
			if fault == "commit_busy" {
				var coded interface{ Code() int }
				if !errors.As(err, &coded) || coded.Code() != 5 {
					t.Fatal("did not reach real COMMIT BUSY", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "WH precise publication boundary") {
				t.Fatal("wrong early publication failure", err)
			}
			if !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
				t.Fatal("failed publication leaked partial rows on original handle")
			}
			release()
			_, err = publisher.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
			handoffCheck(t, err)
			var uses int
			handoffCheck(t, f.db.QueryRow("SELECT use_count FROM states WHERE state_id=?", f.entry.StateID).Scan(&uses))
			if uses != 1 {
				t.Fatal("retry double-counted publication")
			}
		})
	}
}

func TestWorkingInstanceHandoffImmutableReceipt(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	publisher := any(f.q).(handoffPublisher)
	_, err := publisher.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
	handoffCheck(t, err)
	for _, field := range []string{"handoff_job_id", "handoff_instance_id", "handoff_state_id", "handoff_operation_ref", "handoff_runtime_dir", "handoff_outcome_digest", "db_absent_at"} {
		before := mainPathJournalRows(t, f.db)
		_, err := f.db.Exec("UPDATE prepare_liquibase_session_disposals SET "+field+"=? WHERE session_id=?", "foreign", f.session.SessionID)
		if err == nil || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
			t.Fatalf("receipt field %s lost immutability", field)
		}
	}
}

type handoffProjectionProbe struct {
	db    *sql.DB
	calls int
}

func (v *handoffProjectionProbe) VerifyLiquibaseProjectionsAbsent(ctx context.Context, intent queue.FinalizationSnapshot) error {
	v.calls++
	var n int
	if err := v.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_liquibase_session_disposals WHERE handoff_job_id=?", intent.JobID).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return queue.ErrLiquibaseFinalization
	}
	return nil
}

// WH05/WH06: neither early projection proof nor a historical transfer extends
// resource lifetime. Reopens only read durable history; they do not replay SQL.
func TestWorkingInstanceHandoffOwnership(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	publisher := any(f.q).(handoffPublisher)
	intent, err := publisher.PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
	handoffCheck(t, err)
	projections := &handoffProjectionProbe{db: f.db}
	before := mainPathJournalRows(t, f.db)
	if _, err := f.q.ConfirmProjectionsRemoved(t.Context(), intent, projections, mainPathJournalUTC); err == nil || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("pending private ownership admitted terminal proof", err)
	}
	if err := f.q.DeleteJob(t.Context(), f.session.JobID); err == nil || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("active transfer admitted job deletion", err)
	}
	_, err = f.db.Exec("DELETE FROM instances WHERE instance_id=?", f.entry.InstanceID)
	if err == nil || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("live publication pin did not prevent instance deletion", err)
	}
	probe := &handoffAbsence{db: f.db, session: f.session, database: f.binding.RuntimeRef}
	handoffCheck(t, f.q.RetireTransferredLiquibaseSession(t.Context(), f.session, probe, mainPathJournalUTC))
	confirmed, err := f.q.ConfirmProjectionsRemoved(t.Context(), intent, projections, mainPathJournalUTC)
	handoffCheck(t, err)
	handoffCheck(t, f.q.CommitFinalization(t.Context(), confirmed, mainPathJournalUTC, "2026-11-08T12:00:00Z"))
	// Direct metadata deletion proves the historical fields are not permanent
	// foreign keys. WH08 separately exercises real public native disposal.
	_, err = f.db.Exec("DELETE FROM instances WHERE instance_id=?", f.entry.InstanceID)
	handoffCheck(t, err)
	_, err = f.db.Exec("DELETE FROM states WHERE state_id=?", f.entry.StateID)
	handoffCheck(t, err)
	var file string
	handoffCheck(t, f.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file))
	history := mainPathJournalRows(t, f.db)
	for i := 0; i < 2; i++ {
		handoffCheck(t, f.db.Close())
		db, err := sql.Open("sqlite", file)
		handoffCheck(t, err)
		f.db = db
		t.Cleanup(func() { _ = db.Close() })
		_, err = managedstore.Initialize(t.Context(), db, func(context.Context) error { t.Fatal("completed-history reopen must not inventory"); return nil })
		handoffCheck(t, err)
		if !reflect.DeepEqual(history, mainPathJournalRows(t, db)) {
			t.Fatal("reopen changed terminal history")
		}
		var pin sql.NullString
		var state string
		handoffCheck(t, db.QueryRow("SELECT f.instance_pin,d.handoff_state_id FROM prepare_liquibase_session_disposals d JOIN prepare_liquibase_job_finalizations f ON f.job_id=d.handoff_job_id WHERE d.session_id=?", f.session.SessionID).Scan(&pin, &state))
		if pin.Valid || state != f.entry.StateID {
			t.Fatal("history repinned or forgot its output")
		}
	}
}

func TestWorkingInstanceHandoffOutcomeOrder(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	_, err := f.db.Exec("INSERT INTO prepare_liquibase_job_cancellations(job_id,requested_at) VALUES(?,?)", f.session.JobID, mainPathJournalUTC)
	handoffCheck(t, err)
	before := mainPathJournalRows(t, f.db)
	_, err = any(f.q).(handoffPublisher).PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
	if err == nil || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("accepted cancellation lost to success", err)
	}
}

func TestWorkingInstanceHandoffNullableShape(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	_, digest, err := finalizationcodec.Encode(finalizationcodec.Outcome{Kind: finalizationcodec.PrepareSuccess, EventProfile: finalizationcodec.StatusResult, Prepare: &finalizationcodec.PrepareReference{InstanceID: f.entry.InstanceID, StateID: f.entry.StateID}})
	handoffCheck(t, err)
	statement := `INSERT INTO prepare_liquibase_session_disposals(session_id,reason,authority_ref,requested_at,runner_id,db_runtime_id,handoff_job_id,handoff_instance_id,handoff_state_id,handoff_operation_ref,handoff_runtime_dir,handoff_outcome_digest) VALUES(?,'instance_transferred',?,?,?,?,?,?,?,?,?,?)`
	args := []any{f.session.SessionID, digest, f.entry.CreatedAt, f.session.RunnerID, f.binding.RuntimeRef, f.session.JobID, f.entry.InstanceID, f.entry.StateID, f.op.Ref, f.op.Target, digest}
	// Positive control reaches the same INSERT, then explicitly rolls back.
	tx, err := f.db.BeginTx(t.Context(), nil)
	handoffCheck(t, err)
	_, err = tx.ExecContext(t.Context(), statement, args...)
	handoffCheck(t, err)
	handoffCheck(t, tx.Rollback())
	for index := 5; index < len(args); index++ {
		for _, bad := range []any{nil, ""} {
			before := mainPathJournalRows(t, f.db)
			changed := append([]any(nil), args...)
			changed[index] = bad
			_, err := f.db.Exec(statement, changed...)
			var coded interface{ Code() int }
			if !errors.As(err, &coded) || coded.Code() != 1811 && coded.Code() != 275 || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
				t.Fatalf("partial handoff field %d accepted or wrong refusal: %v", index, err)
			}
		}
	}
	// Other disposal families cannot smuggle any handoff identity. This reaches
	// the ordinary final_published authority, whose all-NULL control is valid.
	ordinary := `INSERT INTO prepare_liquibase_session_disposals(session_id,reason,authority_ref,requested_at,runner_id,db_runtime_id,handoff_job_id) VALUES(?,'final_published',?,?,?,?,?)`
	args = []any{f.session.SessionID, f.entry.StateID, f.entry.CreatedAt, f.session.RunnerID, f.binding.RuntimeRef, nil}
	tx, err = f.db.BeginTx(t.Context(), nil)
	handoffCheck(t, err)
	_, err = tx.ExecContext(t.Context(), ordinary, args...)
	handoffCheck(t, err)
	handoffCheck(t, tx.Rollback())
	args[5] = f.session.JobID
	before := mainPathJournalRows(t, f.db)
	_, err = f.db.Exec(ordinary, args...)
	var coded interface{ Code() int }
	if !errors.As(err, &coded) || coded.Code() != 275 || !reflect.DeepEqual(before, mainPathJournalRows(t, f.db)) {
		t.Fatal("ordinary disposal admitted handoff identity", err)
	}
}

func TestWorkingInstanceHandoffTerminalJobDeletion(t *testing.T) {
	f := newWorkingHandoffFixture(t)
	f.ready(t)
	intent, err := any(f.q).(handoffPublisher).PublishWorkingInstanceAndBeginFinalization(t.Context(), f.session, f.entry, f.binding, f.op)
	handoffCheck(t, err)
	handoffCheck(t, f.q.RetireTransferredLiquibaseSession(t.Context(), f.session, &handoffAbsence{db: f.db, session: f.session, database: f.binding.RuntimeRef}, mainPathJournalUTC))
	confirmed, err := f.q.ConfirmProjectionsRemoved(t.Context(), intent, &handoffProjectionProbe{db: f.db}, mainPathJournalUTC)
	handoffCheck(t, err)
	handoffCheck(t, f.q.CommitFinalization(t.Context(), confirmed, mainPathJournalUTC, "2026-11-08T12:00:00Z"))
	handoffCheck(t, f.q.DeleteJob(t.Context(), f.session.JobID))
	var live int
	handoffCheck(t, f.db.QueryRow("SELECT count(*) FROM instances WHERE instance_id=?", f.entry.InstanceID).Scan(&live))
	if live != 1 {
		t.Fatal("terminal job deletion removed transferred instance")
	}
}
