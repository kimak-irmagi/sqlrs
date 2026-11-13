package prepare

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// PG08's local double checks the actual startup method, including every clone
// locator, before returning a refused/indeterminate transfer. Native PG08/PG09
// independently qualify real SQL, runner readiness and saved suffix metadata.
type planningDBLocalJournal struct {
	normalLiquibaseJournal
	wantClone                             queue.ClonePlan
	job, task, execution, fingerprint     string
	cause                                 error
	transfers, allocations, ordinaryPlans int
}

func (q *planningDBLocalJournal) NextCloneGeneration(context.Context, string) (int, error) {
	q.allocations++
	return 0, errors.New("unexpected second allocation")
}
func (q *planningDBLocalJournal) PlanLiquibaseSession(context.Context, queue.LiquibaseSessionLaunch) (queue.LiquibaseSessionRecord, error) {
	q.ordinaryPlans++
	return queue.LiquibaseSessionRecord{}, errors.New("unexpected fallback session")
}
func (q *planningDBLocalJournal) RecordPlanningDBRetention(context.Context, queue.AttemptRecord, int, instanceaccess.Operation, func(context.Context, queue.PlanningRecoverySnapshot, instanceaccess.Operation) error, string) (int64, error) {
	return 0, errors.New("unexpected retention during transfer")
}
func (q *planningDBLocalJournal) CompleteRetainedPlanning(context.Context, queue.AttemptRecord, int, int64, int64, int64, string) error {
	return errors.New("unexpected plan completion during transfer")
}
func (q *planningDBLocalJournal) RequestPlanningDBDisposal(context.Context, queue.AttemptRecord, int, string, string) (int64, error) {
	return 0, errors.New("unexpected destructive fallback")
}
func (q *planningDBLocalJournal) CompletePlanningDBDisposal(context.Context, queue.AttemptRecord, int, instanceaccess.Operation, queue.CloneAbsenceVerifier, string) error {
	return errors.New("unexpected destructive fallback")
}
func (q *planningDBLocalJournal) TransferPlanningDBToFirstSession(_ context.Context, l queue.LiquibaseSessionLaunch) (queue.LiquibaseSessionRecord, error) {
	q.transfers++
	if !reflect.DeepEqual(l.Clone, q.wantClone) || l.Session.JobID != q.job || l.Session.CloneID != q.wantClone.ID || l.Session.WorkerGeneration != 1 || l.Session.StartKind != q.wantClone.SourceKind || l.Session.StartID != planningCloneSource(q.wantClone) || l.FirstTaskID != q.task || l.ExecutionDigest != q.execution || l.Session.ExecutionFingerprint != q.fingerprint || l.Session.SessionNo != 1 || l.Session.StartPosition != 1 || l.RequireFirstSessionProof {
		return queue.LiquibaseSessionRecord{}, errors.New("transfer changed immutable identity or cursor")
	}
	return queue.LiquibaseSessionRecord{}, q.cause
}

func TestPlanningDBHandoffLocalOrder(t *testing.T) {
	for _, variant := range []string{"refused", "indeterminate"} {
		t.Run(variant, func(t *testing.T) {
			cause := errors.New("exact transfer boundary")
			if variant == "indeterminate" {
				cause = errors.Join(queue.ErrFinalizationIndeterminate, cause)
			}
			clone := queue.ClonePlan{ID: strings.Repeat("a", 24), Generation: 1, SourceKind: "image", SourceImageID: "sha256:" + strings.Repeat("b", 64), Ref: "jobs/job/runtime-preview", PlannedRuntimeName: "sqlrs-db-preview"}
			fingerprint := "sha256:" + strings.Repeat("c", 64)
			q := &planningDBLocalJournal{wantClone: clone, job: "job", task: "one", execution: "sha256:" + strings.Repeat("d", 64), fingerprint: fingerprint, cause: cause}
			root := t.TempDir()
			m := &PrepareService{stateStoreRoot: root, now: func() time.Time { return time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) }, containerLiquibase: &LiquibaseContainerDeps{Sources: inputbundle.Store{Root: filepath.Join(root, "blobs")}}}
			p := preparedRequest{liquibaseFingerprint: fingerprint, liquibaseProjection: &liquibaseProjectionCopy{root: filepath.Join(root, "copy-"+strings.Repeat("e", 32))}}
			database := &jobRuntime{instance: engineRuntime.Instance{ID: strings.Repeat("f", 64)}}
			candidate := &normalPlanningDB{runtime: database, preview: queue.PlanningRecoverySnapshot{Clone: clone, CloneRuntimeID: database.instance.ID}}
			working, err := m.startNormalLiquibaseSession(t.Context(), q, nil, "job", p, taskState{PlanTask: PlanTask{TaskID: "one", Input: &TaskInput{Kind: "image", ID: clone.SourceImageID}}}, 1, queue.ExecutionBinding{Digest: q.execution}, candidate)
			if working != nil || !errors.Is(err, cause) || q.transfers != 1 || q.allocations != 0 || q.ordinaryPlans != 0 || candidate.runtime != database || candidate.preview.CloneRuntimeID != database.instance.ID {
				t.Fatal("refused/unknown transfer allocated, fell back or changed the exact candidate", err)
			}
		})
	}
}
