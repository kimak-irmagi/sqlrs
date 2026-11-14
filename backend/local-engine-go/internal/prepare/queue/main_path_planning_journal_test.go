package queue_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// MP05: the normal installer must support the real planning writers, not just
// contain their tables. Native capture verification remains a separate test.
func TestMainPathPlanningJournal(t *testing.T) {
	db, q, _, _ := mainPathSessionJournal(t)
	job, found, err := q.GetJob(t.Context(), "main-path-job")
	if err != nil || !found {
		t.Fatal(err)
	}
	record, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID)
	if err != nil || !found {
		t.Fatal(err)
	}
	job.JobID, job.Status, job.StartedAt = "main-path-preview", "queued", nil
	job.Signature = nil
	if err := q.AdmitLiquibaseJob(t.Context(), job, record.RequestedTag, record.BundleManifestDigest); err != nil {
		t.Fatal(err)
	}
	if err := q.BindLiquibaseExecution(t.Context(), job.JobID, record); err != nil {
		t.Fatal(err)
	}
	running := "running"
	if err := q.UpdateJob(t.Context(), job.JobID, queue.JobUpdate{Status: &running}); err != nil {
		t.Fatal(err)
	}
	digest := func(c string) string { return "sha256:" + strings.Repeat(c, 64) }
	binding, err := queue.ProjectLiquibasePlanningBinding(record, `["updateSQL"]`, digest("6"))
	if err != nil {
		t.Fatal(err)
	}
	clone := queue.ClonePlan{ID: "main-preview-clone", Ref: "jobs/main-path-preview/copy", PlannedRuntimeName: "sqlrs-db-main-preview", Generation: 1, SourceKind: "image", SourceImageID: job.ResolvedImageID}
	launch := queue.PlanningLaunch{BoundLaunch: queue.BoundLaunch{
		Attempt: queue.AttemptPlan{JobID: job.JobID, TaskID: "plan", CloneID: clone.ID, ExecutionDigest: binding.Digest, WorkerGeneration: 1, CreatedAt: mainPathJournalUTC},
		Binding: binding, Clone: clone, Leases: []queue.ArtifactLease{{ID: "preview-helper", Kind: "capture_helper", Digest: digest("6")}},
	}, Capture: queue.PlanningCaptureNames{ProtectedInput: "preview-input", Stdout: "preview-stdout", Stderr: "preview-stderr", Manifest: "preview-manifest", HelperDigest: digest("6"), ProtocolRevision: planningcodec.CaptureProtocolRevision}}
	attempt, err := q.PlanBoundPlanningAttempt(t.Context(), launch)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.AttemptNo != 1 || attempt.Phase != "planned" || attempt.AttemptPlan != launch.Attempt {
		t.Fatalf("unexpected attempt: %+v", attempt)
	}
	dbID, runnerID := strings.Repeat("d", 64), strings.Repeat("e", 64)
	if err := q.ActivatePlanningClone(t.Context(), attempt, 1, clone.PlannedRuntimeName, dbID); err != nil {
		t.Fatal(err)
	}
	if err := q.RequestAttemptCreate(t.Context(), attempt, 1); err != nil {
		t.Fatal(err)
	}
	if err := q.AttachAttemptExecutor(t.Context(), attempt, 1, runnerID); err != nil {
		t.Fatal(err)
	}
	attempt.ExecutorID = runnerID
	if err := q.RequestAttemptStart(t.Context(), attempt, 1, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	exit, err := q.ObserveAttemptExit(t.Context(), attempt, 1, runnerID, 0, mainPathJournalUTC)
	if err != nil {
		t.Fatal(err)
	}
	quiescent, err := q.RecordPlanningCloneQuiescence(t.Context(), attempt, 1, clone.PlannedRuntimeName, dbID, mainPathJournalUTC)
	if err != nil {
		t.Fatal(err)
	}
	output, err := q.RecordVerifiedPlanningCapture(t.Context(), attempt, 1, queue.PlanningCaptureProof{StdoutDigest: digest("2"), StderrDigest: digest("3"), StdoutBytes: 4, ManifestRef: launch.Capture.Manifest}, mainPathJournalUTC)
	if err != nil {
		t.Fatal(err)
	}
	metadata := []planningcodec.Changeset{{ID: "one", Author: "sqlrs", Path: "db/main.sql", SQLHash: strings.Repeat("4", 64)}}
	planDigest, err := q.SavePlanningResult(t.Context(), job.JobID, 1, digest("2"), metadata, mainPathJournalUTC)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CompletePlanning(t.Context(), attempt, 1, exit, quiescent, output, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	saved, found, err := q.LoadPlanningResult(t.Context(), job.JobID)
	if err != nil || !found || saved.PlanDigest != planDigest || len(saved.Changesets) != 1 || saved.Changesets[0] != metadata[0] {
		t.Fatalf("saved plan: %+v %v", saved, err)
	}
	var tasks, applied int
	if err := db.QueryRow("SELECT count(*),coalesce(sum(type='state_execute'),0) FROM prepare_tasks WHERE job_id=?", job.JobID).Scan(&tasks, &applied); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || applied != 0 {
		t.Fatalf("planning persisted imaginary execution: tasks=%d apply=%d", tasks, applied)
	}
	if !(exit < quiescent && quiescent < output) {
		t.Fatal("planning evidence lost its order")
	}
	disposal, err := q.SavePrivatePlanningCaptureDisposal(t.Context(), attempt, 1, "plan_saved", planDigest, mainPathJournalUTC)
	if err != nil || disposal.AuthorityRef != planDigest {
		t.Fatal("saved cleanup authority", err)
	}
	absence := &mainPathPreviewAbsence{db: db, attempt: attempt, clone: clone, dbID: dbID, input: launch.Capture.ProtectedInput}
	if err := q.RetirePrivatePlanningCapture(t.Context(), attempt, 1, absence, mainPathJournalUTC); err != nil {
		t.Fatal("private parent retirement", err)
	}
	if err := q.RetirePlanningArtifact(t.Context(), attempt, 1, "input", absence, mainPathJournalUTC); err != nil {
		t.Fatal("protected input retirement", err)
	}
	if err := q.ReleasePlanningClone(t.Context(), attempt, 1, absence); err != nil {
		t.Fatal("preview clone release", err)
	}
	if absence.executor != 1 || absence.inputCalls != 1 || absence.cloneCalls != 1 {
		t.Fatalf("missing exact observations: %+v", absence)
	}
}

// H-level exact observations only. These callbacks deliberately query the live
// SQLite handle; native absence is independently required by MP06.
type mainPathPreviewAbsence struct {
	db                               *sql.DB
	attempt                          queue.AttemptRecord
	clone                            queue.ClonePlan
	dbID, input                      string
	executor, inputCalls, cloneCalls int
}

func (v *mainPathPreviewAbsence) VerifyPlanningArtifactAbsent(ctx context.Context, kind, ref, id string) error {
	if id != v.attempt.ExecutorID {
		return queue.ErrPlanningSchema
	}
	switch kind {
	case "executor":
		if ref != v.attempt.PlannedExecutorName {
			return queue.ErrPlanningSchema
		}
		v.executor++
	case "input":
		if ref != v.input {
			return queue.ErrPlanningSchema
		}
		v.inputCalls++
	default:
		return queue.ErrPlanningSchema
	}
	var n int
	if err := v.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_planning_attempt_artifacts WHERE job_id=?", v.attempt.JobID).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return queue.ErrPlanningSchema
	}
	return nil
}

func (v *mainPathPreviewAbsence) VerifyReleasedClone(ctx context.Context, clone queue.ClonePlan, id string) error {
	if clone != v.clone || id != v.dbID {
		return queue.ErrPlanningSchema
	}
	v.cloneCalls++
	var n int
	if err := v.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_clones WHERE clone_id=?", clone.ID).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return queue.ErrPlanningSchema
	}
	return nil
}
