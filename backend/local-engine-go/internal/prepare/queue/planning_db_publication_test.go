package queue_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/statefs"
	enginestore "github.com/sqlrs/engine-local/internal/store"
)

// PG07/PG09 metadata control executes all actual queue/access writers from the
// retained planner through working publication. The journal intentionally has
// one clone shared by the closed planner and three published apply attempts.
// Native PG09 remains responsible for PostgreSQL data and physical DB counts.
func TestPlanningDBHandoffPublishedHistory(t *testing.T) {
	exitCode := 0
	f := newPlanningDBFixtureForClone(t, &exitCode, nil, func(c *queue.ClonePlan) {
		c.Ref = "jobs/planning-handoff-plan/runtime_retained"
	})
	if queue.PlanningDBPublicationTargetForTest(t.Context(), f.db, f.attempt.JobID, "instance", f.op) {
		t.Fatal("unowned planner path permitted publication")
	}
	f.complete(t)
	f.retireCapture(t)
	l := f.firstLaunch(t)
	s, err := any(f.q).(planningDBTransferWriter).TransferPlanningDBToFirstSession(t.Context(), l)
	handoffCheck(t, err)
	if queue.PlanningDBPublicationTargetForTest(t.Context(), f.db, f.attempt.JobID, "instance", f.op) {
		t.Fatal("transfer without publication receipt permitted publication")
	}
	handoffCheck(t, f.q.RequestLiquibaseSessionRunnerCreate(t.Context(), s))
	s.RunnerID = strings.Repeat("c", 64)
	handoffCheck(t, f.q.AttachLiquibaseSessionRunner(t.Context(), s, s.RunnerID))
	handoffCheck(t, f.q.RequestLiquibaseSessionRunnerStart(t.Context(), s))
	handoffCheck(t, f.q.MarkLiquibaseSessionRunnerRunning(t.Context(), s))
	s.RunnerPhase = "running"
	_, err = f.q.RecordLiquibaseSessionReadiness(t.Context(), s, mainPathJournalReady{db: f.db, want: s}, mainPathJournalUTC)
	handoffCheck(t, err)
	job, found, err := f.q.GetJob(t.Context(), f.attempt.JobID)
	if err != nil || !found {
		t.Fatal("published job missing", err)
	}
	binding := managedidentity.RuntimeBinding{RuntimeRef: f.op.RuntimeRef, PhysicalIdentity: f.op.PhysicalIdentity, IdentityBinding: f.op.Identity}
	states := []string{strings.Repeat("5", 64), strings.Repeat("6", 64), strings.Repeat("7", 64)}
	for i, task := range []string{"one", "two", "three"} {
		invocation := mainPathNextInvocation(t, f.q, s, l.ExecutionDigest, task, "retained-publish-"+task)
		prune, err := f.q.ReadLiquibaseCapturePrunePlan(t.Context(), s, invocation)
		handoffCheck(t, err)
		if prune != nil {
			handoffCheck(t, f.q.ConfirmLiquibaseCapturePruned(t.Context(), s, invocation, &mainPathPruneProbe{db: f.db, want: *prune}, mainPathJournalUTC))
		}
		handoffCheck(t, f.q.RequestLiquibaseSessionExec(t.Context(), s, invocation, mainPathJournalUTC))
		complete, err := f.q.CompleteLiquibaseSessionInvocation(t.Context(), s, invocation, queue.LiquibaseInvocationCompletion{ExitCode: 0, ManifestDigest: pgDigest("e"), ManifestBytes: 500}, mainPathJournalUTC)
		handoffCheck(t, err)
		quiet, err := f.q.RecordLiquibaseSessionDBStopped(t.Context(), s, invocation, f.clone.PlannedRuntimeName, f.op.RuntimeRef, mainPathJournalUTC)
		handoffCheck(t, err)
		handoffCheck(t, f.access.RecordSeal(t.Context(), states[i], f.op, binding))
		dir := filepath.Join(t.TempDir(), states[i])
		handoffCheck(t, os.MkdirAll(dir, 0700))
		handoffCheck(t, os.WriteFile(statefs.PublishedMarkerPath(dir, "copy"), []byte("ok"), 0600))
		entry := enginestore.StateCreate{StateID: states[i], StateFingerprint: states[i], ImageID: job.ResolvedImageID, PrepareKind: "lb", PrepareArgsNormalized: "fixture update-count", CreatedAt: mainPathJournalUTC, LineageRef: job.LineageRef, IdentityDigest: job.IdentityDigest, LiquibaseExecutionFingerprint: s.ExecutionFingerprint}
		if i > 0 {
			entry.ParentStateID = &states[i-1]
		}
		handoffCheck(t, f.q.PublishLiquibaseSessionState(t.Context(), s, invocation, entry, dir, "copy", complete, quiet, mainPathJournalUTC))
		s, _, err = f.q.ReadLiquibaseSession(t.Context(), s.SessionID)
		handoffCheck(t, err)
		if i < 2 {
			handoffCheck(t, f.q.RequestLiquibaseSessionDBRestart(t.Context(), s))
			handoffCheck(t, f.q.MarkLiquibaseSessionDBReady(t.Context(), s, f.clone.PlannedRuntimeName, f.op.RuntimeRef))
			s.DBPhase = "running"
		}
	}
	handoffCheck(t, f.q.RequestLiquibaseSessionRunnerStop(t.Context(), s))
	handoffCheck(t, f.q.AcknowledgeLiquibaseSessionRunnerStop(t.Context(), s))
	handoffCheck(t, f.q.MarkLiquibaseSessionRunnerStopped(t.Context(), s))
	s.RunnerPhase = "stopped"
	running := "running"
	handoffCheck(t, f.q.UpdateTask(t.Context(), job.JobID, "prepare-instance", queue.TaskUpdate{Status: &running}))
	handoffCheck(t, f.q.RequestLiquibaseSessionDBRestart(t.Context(), s))
	handoffCheck(t, f.q.MarkLiquibaseSessionDBReady(t.Context(), s, f.clone.PlannedRuntimeName, f.op.RuntimeRef))
	s.DBPhase = "running"
	active := enginestore.InstanceStatusActive
	instanceSum := sha256.Sum256([]byte("instance\x00" + job.JobID + "\x00"))
	entry := enginestore.InstanceCreate{InstanceID: hex.EncodeToString(instanceSum[:])[:32], StateID: states[2], ImageID: job.ResolvedImageID, CreatedAt: mainPathJournalUTC, RuntimeID: &f.op.RuntimeRef, RuntimeDir: &f.op.Target, Status: &active}
	handoffCheck(t, f.access.Activate(t.Context(), entry.InstanceID, binding, func(instanceaccess.Secret) error { return nil }, func() error { return nil }))
	intent, err := f.q.PublishWorkingInstanceAndBeginFinalization(t.Context(), s, entry, binding, f.op)
	handoffCheck(t, err)
	// CP-PG04 checks the exact predicate independently of outer publication's
	// argument rejection, retaining the actual valid receipt as positive control.
	for _, variant := range []string{"valid", "source", "operation", "path", "instance", "job"} {
		op, instance, jobID := f.op, entry.InstanceID, job.JobID
		switch variant {
		case "source":
			op.Source = "foreign"
		case "operation":
			op.Ref = "foreign"
		case "path":
			op.Target = filepath.Join(filepath.Dir(op.Target), "foreign")
		case "instance":
			instance = "foreign"
		case "job":
			jobID = "foreign"
		}
		if queue.PlanningDBPublicationTargetForTest(t.Context(), f.db, jobID, instance, op) != (variant == "valid") {
			t.Fatal("planner path authority", variant)
		}
	}
	absence := &handoffAbsence{db: f.db, session: s, database: f.op.RuntimeRef}
	handoffCheck(t, f.q.RetireTransferredLiquibaseSession(t.Context(), s, absence, mainPathJournalUTC))
	if intent.InstancePin != entry.InstanceID || f.count(t, "SELECT count(*) FROM prepare_clones WHERE job_id=?", job.JobID) != 1 || f.count(t, "SELECT count(*) FROM prepare_clones WHERE clone_id=? AND phase='released'", f.clone.ID) != 1 || f.count(t, "SELECT count(*) FROM prepare_liquibase_planning_db_ownership WHERE job_id=? AND session_id=? AND disposed_at IS NULL", job.JobID, s.SessionID) != 1 {
		t.Fatal("normal publication lost single-clone transfer history")
	}
	before := mainPathJournalRows(t, f.db)
	old := &mainPathPreviewAbsence{db: f.db, attempt: f.attempt, clone: f.clone, dbID: f.op.RuntimeRef, input: f.capture.ProtectedInput}
	if err := f.q.ReleasePlanningClone(t.Context(), f.attempt, 1, old); err == nil || old.cloneCalls != 0 {
		t.Fatal("released working history revived old planning cleanup")
	}
	f.unchanged(t, before)
}
