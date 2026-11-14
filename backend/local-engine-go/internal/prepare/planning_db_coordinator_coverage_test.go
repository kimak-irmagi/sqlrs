package prepare

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/finalizationcodec"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	rt "github.com/sqlrs/engine-local/internal/runtime"
)

// CP-PG01/02: boundary doubles expose the actual coordinator order, without
// native effects or replacing the independent real-SQL journal acceptance.
type planningCoordinatorJournal struct {
	normalLiquibaseJournal
	calls    *[]string
	fail     string
	unknown  bool
	observe  bool
	job      queue.JobRecord
	metadata []planningcodec.Changeset
}

var errPlanningCoordinator = errors.New("CP-PG boundary refused")

func (j *planningCoordinatorJournal) step(s string) error {
	*j.calls = append(*j.calls, s)
	if j.fail == s {
		if j.unknown {
			return errors.Join(queue.ErrFinalizationIndeterminate, errPlanningCoordinator)
		}
		return errPlanningCoordinator
	}
	return nil
}
func (j *planningCoordinatorJournal) GetJob(context.Context, string) (queue.JobRecord, bool, error) {
	e := j.step("job")
	return j.job, j.fail != "missing-job", e
}
func (j *planningCoordinatorJournal) RecordPlanningDBRetention(ctx context.Context, _ queue.AttemptRecord, _ int, op instanceaccess.Operation, observe func(context.Context, queue.PlanningRecoverySnapshot, instanceaccess.Operation) error, _ string) (int64, error) {
	if j.observe {
		return 0, observe(ctx, queue.PlanningRecoverySnapshot{}, op)
	}
	return 2, j.step("retain")
}
func (j *planningCoordinatorJournal) CompleteRetainedPlanning(context.Context, queue.AttemptRecord, int, int64, int64, int64, string) error {
	return j.step("complete-retained")
}
func (j *planningCoordinatorJournal) TransferPlanningDBToFirstSession(context.Context, queue.LiquibaseSessionLaunch) (queue.LiquibaseSessionRecord, error) {
	panic("unexpected transfer")
}
func (j *planningCoordinatorJournal) RecordPlanningCloneQuiescence(context.Context, queue.AttemptRecord, int, string, string, string) (int64, error) {
	return 2, j.step("quiescence")
}
func (j *planningCoordinatorJournal) RecordVerifiedPlanningCapture(context.Context, queue.AttemptRecord, int, queue.PlanningCaptureProof, string) (int64, error) {
	return 3, j.step("output")
}
func (j *planningCoordinatorJournal) SavePlanningResult(_ context.Context, _ string, _ int, _ string, c []planningcodec.Changeset, _ string) (string, error) {
	j.metadata = c
	return "plan", j.step("save")
}
func (j *planningCoordinatorJournal) CompletePlanning(context.Context, queue.AttemptRecord, int, int64, int64, int64, string) error {
	return j.step("complete-absent")
}
func (j *planningCoordinatorJournal) LoadPlanningResult(context.Context, string) (queue.PlanningResult, bool, error) {
	e := j.step("load")
	return queue.PlanningResult{PlanDigest: "plan", Changesets: j.metadata}, j.fail != "missing-plan", e
}
func (j *planningCoordinatorJournal) RequestPlanningDBDisposal(_ context.Context, _ queue.AttemptRecord, _ int, reason, _ string) (int64, error) {
	return 4, j.step("request:" + reason)
}
func (j *planningCoordinatorJournal) CompletePlanningDBDisposal(ctx context.Context, _ queue.AttemptRecord, _ int, op instanceaccess.Operation, v queue.CloneAbsenceVerifier, _ string) error {
	if op.Stage != "retired" {
		return errors.New("operation changed")
	}
	return j.step("disposed")
}
func (j *planningCoordinatorJournal) BeginFinalization(_ context.Context, _ string, o finalizationcodec.Outcome, _ string) (queue.FinalizationSnapshot, error) {
	if o.Kind != finalizationcodec.Cancelled {
		panic("wrong outcome")
	}
	return queue.FinalizationSnapshot{}, j.step("finalize")
}

type planningCoordinatorRuntime struct {
	normalLiquibaseRuntime
	*fakeRuntime
	calls     *[]string
	output    string
	failRead  bool
	absentErr error
}

func (r *planningCoordinatorRuntime) ReadCompleteLiquibaseOutput(context.Context, string, string, string, rt.CaptureIdentity, int64) (rt.CompleteCapture, error) {
	*r.calls = append(*r.calls, "capture")
	if r.failRead {
		return rt.CompleteCapture{}, errPlanningCoordinator
	}
	return rt.CompleteCapture{Stdout: []byte(r.output)}, nil
}
func (r *planningCoordinatorRuntime) VerifyContainerAbsent(context.Context, string) error {
	return r.absentErr
}

func planningCoordinatorFixture(t *testing.T) (*PrepareService, *planningCoordinatorJournal, *planningCoordinatorRuntime, *normalPlanningDB) {
	t.Helper()
	calls := []string{}
	j := &planningCoordinatorJournal{calls: &calls, job: queue.JobRecord{Purpose: "prepare"}}
	r := &planningCoordinatorRuntime{fakeRuntime: &fakeRuntime{}, calls: &calls, output: "-- Changeset db/one.sql::1::alice\nSELECT 1;\n"}
	dir := t.TempDir()
	m := &PrepareService{runtime: r.fakeRuntime, now: func() time.Time { return time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) }}
	database := &jobRuntime{instance: rt.Instance{ID: "database"}, runtimeDir: dir, cleanup: func() error { calls = append(calls, "cleanup"); return nil }}
	preview := queue.PlanningRecoverySnapshot{Clone: queue.ClonePlan{ID: "clone"}, CloneRuntimeID: "database"}
	preview.Attempt.JobID = "job"
	return m, j, r, &normalPlanningDB{runtime: database, preview: preview}
}

func TestPlanningCoordinatorCaptureOrder(t *testing.T) {
	for _, variant := range []string{"retained", "empty", "plan-only", "explain", "failed", "read", "parse", "job", "missing-job", "retain", "output", "save", "complete-retained", "load", "missing-plan", "quiescence", "complete-absent", "missing-capability", "observe", "cleanup-error"} {
		t.Run(variant, func(t *testing.T) {
			m, j, r, c := planningCoordinatorFixture(t)
			p := preparedRequest{}
			exit := 0
			nonretain := false
			q := normalLiquibaseJournal(j)
			switch variant {
			case "missing-capability":
				q = struct{ normalLiquibaseJournal }{j}
			case "observe":
				j.observe = true
			case "cleanup-error":
				p.request.PlanOnly = true
				managed, prepared, _, _, _ := managedExecutionFixture(t)
				op, e := managed.access.Assign(t.Context(), "cleanup-error", c.runtime.runtimeDir, "source", prepared.managed.IdentityBinding)
				if e != nil {
					t.Fatal(e)
				}
				m.access = managed.access
				c.runtime.operation = op
				r.fakeRuntime.stopErr = errPlanningCoordinator
			case "empty":
				r.output = ""
				nonretain = true
			case "plan-only":
				p.request.PlanOnly = true
				nonretain = true
			case "explain":
				j.job.Purpose = "cache_explain"
				nonretain = true
			case "failed":
				exit = 1
				nonretain = true
			case "read":
				r.failRead = true
			case "parse":
				r.output = "-- Changeset invalid\n"
			case "quiescence", "complete-absent":
				p.request.PlanOnly = true
				nonretain = true
				j.fail = variant
			default:
				if variant != "retained" {
					j.fail = variant
				}
			}
			result, err := m.captureNormalPlanningDB(t.Context(), q, r, p, c.preview, trackedExecutorResult{ExitCode: exit, ExitEvidenceID: 1}, c.runtime, "capture")
			success := variant == "retained" || variant == "empty" || variant == "plan-only" || variant == "explain" || variant == "failed"
			if (err == nil) != success {
				t.Fatal("unexpected result", variant, err, *j.calls)
			}
			if success {
				want := []string{"capture", "job", "retain", "output", "save", "complete-retained", "load"}
				if nonretain {
					want = []string{"capture", "job", "cleanup", "quiescence", "output", "save", "complete-absent", "load"}
				}
				if !reflect.DeepEqual(*j.calls, want) || (result.retained != nil) == nonretain {
					t.Fatal("ownership/order", *j.calls, result)
				}
			}
			if !success && result.retained != nil {
				t.Fatal("failed completion granted a candidate")
			}
		})
	}
}

func TestPlanningCoordinatorDisposalOrder(t *testing.T) {
	for _, variant := range []string{"success", "request", "unknown", "cleanup", "disposed", "cancel-request", "cancel-finalize", "cancel-completion", "nil", "missing-journal"} {
		t.Run(variant, func(t *testing.T) {
			m, j, r, c := planningCoordinatorFixture(t)
			q := normalLiquibaseJournal(j)
			reason := "full_cache"
			switch variant {
			case "request", "unknown":
				j.fail = "request:" + reason
				j.unknown = variant == "unknown"
			case "cleanup":
				managed, p, _, _, _ := managedExecutionFixture(t)
				op, err := managed.access.Assign(t.Context(), "cleanup-test", c.runtime.runtimeDir, "source", p.managed.IdentityBinding)
				if err != nil {
					t.Fatal(err)
				}
				m.access = managed.access
				c.runtime.operation = op
				c.runtime.cleanup = func() error { *j.calls = append(*j.calls, "cleanup"); return errPlanningCoordinator }
			case "disposed":
				j.fail = "disposed"
			case "cancel-request":
				j.fail = "request:accepted_cancel"
			case "cancel-finalize":
				j.fail = "finalize"
			case "nil":
				c = nil
			case "missing-journal":
				q = struct{ normalLiquibaseJournal }{j}
			}
			var err error
			if variant == "cancel-request" || variant == "cancel-finalize" || variant == "cancel-completion" {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				err = m.finishRetainedPlanningCancellation(ctx, q, r, c)
			} else {
				err = m.disposeNormalPlanningDB(t.Context(), q, r, c, reason)
			}
			if (err == nil) != (variant == "success") {
				t.Fatal("unexpected disposal result", err)
			}
			if variant == "request" || variant == "unknown" {
				if !reflect.DeepEqual(*j.calls, []string{"request:full_cache"}) {
					t.Fatal("destructive fallback", *j.calls)
				}
			}
			if variant == "success" && !reflect.DeepEqual(*j.calls, []string{"request:full_cache", "cleanup", "disposed"}) {
				t.Fatal("wrong cleanup order", *j.calls)
			}
		})
	}
}

type planningDBInspector struct {
	*fakeRuntime
	actual rt.Instance
	err    error
}

func (i *planningDBInspector) InspectManaged(context.Context, managedidentity.RuntimeBinding) (rt.Instance, error) {
	return i.actual, i.err
}

// Actual authenticated verification is reached only after the exact runtime
// and non-symlink directory checks; missing bootstrap refuses before networking.
func TestPlanningCoordinatorLiveObservation(t *testing.T) {
	for _, variant := range []string{"snapshot", "clone", "operation", "path", "inspector", "inspect-error", "runtime-id", "binding", "missing-dir", "file", "auth"} {
		t.Run(variant, func(t *testing.T) {
			m, p, _, _, _ := managedExecutionFixture(t)
			dir := t.TempDir()
			database := &jobRuntime{instance: rt.Instance{ID: "database", Binding: managedidentity.RuntimeBinding{IdentityBinding: p.managed.IdentityBinding, RuntimeRef: "database"}}, runtimeDir: dir}
			preview := queue.PlanningRecoverySnapshot{Clone: queue.ClonePlan{ID: "clone"}, CloneRuntimeID: "database"}
			preview.Attempt.CloneID = "clone"
			op := normalPlanningOperation(database, "running")
			op.Target = dir
			database.operation = op
			inspect := &planningDBInspector{fakeRuntime: &fakeRuntime{}, actual: database.instance}
			m.runtime = inspect
			switch variant {
			case "snapshot":
				preview.CloneRuntimeID = "foreign"
			case "clone":
				preview.Attempt.CloneID = "foreign"
			case "operation":
				op.Ref = "foreign"
			case "path":
				database.runtimeDir = dir + "-foreign"
			case "inspector":
				m.runtime = &fakeRuntime{}
			case "inspect-error":
				inspect.err = errPlanningCoordinator
			case "runtime-id":
				inspect.actual.ID = "foreign"
			case "binding":
				inspect.actual.Binding.RuntimeRef = "foreign"
			case "missing-dir":
				os.Remove(dir)
			case "file":
				os.Remove(dir)
				os.WriteFile(dir, []byte("file"), 0600)
			}
			if err := m.observeRetainedPlanningDB(t.Context(), preview, database, op); err == nil {
				t.Fatal("unproved live database accepted", variant)
			}
		})
	}
}

func TestPlanningCoordinatorAbsenceIncludesDirectory(t *testing.T) {
	for _, variant := range []string{"absent", "database-live", "directory-live"} {
		t.Run(variant, func(t *testing.T) {
			_, _, r, c := planningCoordinatorFixture(t)
			if variant == "database-live" {
				r.absentErr = errPlanningCoordinator
			}
			if variant != "directory-live" {
				if err := os.Remove(c.runtime.runtimeDir); err != nil {
					t.Fatal(err)
				}
			}
			v := normalPlanningDBAbsence{normalLiquibaseAbsence: normalLiquibaseAbsence{runtime: r, planning: c.preview}, target: c.runtime.runtimeDir}
			err := v.VerifyReleasedClone(t.Context(), c.preview.Clone, c.preview.CloneRuntimeID)
			if (err == nil) != (variant == "absent") {
				t.Fatal("incorrect absence", err)
			}
		})
	}
}

// Keep the independent original source-kind branch in this local fixture.
func TestPlanningCoordinatorSource(t *testing.T) {
	if planningCloneSource(queue.ClonePlan{SourceKind: "image", SourceImageID: "image"}) != "image" {
		t.Fatal("image source changed")
	}
	if planningCloneSource(queue.ClonePlan{SourceKind: "state", SourceStateID: "state"}) != "state" {
		t.Fatal("source changed")
	}
}
