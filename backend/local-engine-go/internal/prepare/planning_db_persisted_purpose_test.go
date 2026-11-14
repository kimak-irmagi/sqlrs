package prepare

import (
	"context"
	"reflect"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// PG02/PG08 and CP-PG01: use the admitted SQLite job at the coordinator's
// purpose boundary. The legacy queue record deliberately omits component-only
// fields; it must not decide whether a live planning DB can be retained.
type persistedPlanningPurposeJournal struct {
	*planningCoordinatorJournal
	reader *queue.SQLiteStore
}

// Preserve CP-PG01's existing job-read failure cases and ordered assertions
// while providing the component-specific reader used by the coordinator.
func (j *planningCoordinatorJournal) LoadContainerJobView(ctx context.Context, id string) (queue.ContainerJobView, error) {
	job, found, err := j.GetJob(ctx, id)
	if err != nil || !found {
		return queue.ContainerJobView{}, queue.ErrPlanningSchema
	}
	return queue.ContainerJobView{Purpose: job.Purpose}, nil
}

func (j persistedPlanningPurposeJournal) GetJob(ctx context.Context, id string) (queue.JobRecord, bool, error) {
	return j.reader.GetJob(ctx, id)
}

func (j persistedPlanningPurposeJournal) LoadContainerJobView(ctx context.Context, id string) (queue.ContainerJobView, error) {
	return j.reader.LoadContainerJobView(ctx, id)
}

func TestPlanningCoordinatorPersistedPurpose(t *testing.T) {
	for _, variant := range []string{"prepare", "plan-only", "cache_explain"} {
		t.Run(variant, func(t *testing.T) {
			admission, p, job, _, _ := costTrimBindingFixture(t)
			reader := admission.queue.(*queue.SQLiteStore)
			if variant != "prepare" {
				job.JobID += "-" + variant
				job.PlanOnly, p.request.PlanOnly = true, true
				job.Purpose = "prepare"
				if variant == "cache_explain" {
					job.Purpose = "cache_explain"
				}
				if err := admission.admitContainerLiquibase(t.Context(), job, p); err != nil {
					t.Fatal(err)
				}
			}
			m, boundary, runtime, candidate := planningCoordinatorFixture(t)
			candidate.preview.Attempt.JobID = job.JobID
			journal := persistedPlanningPurposeJournal{planningCoordinatorJournal: boundary, reader: reader}
			result, err := m.captureNormalPlanningDB(t.Context(), journal, runtime, p, candidate.preview, trackedExecutorResult{ExitEvidenceID: 1}, candidate.runtime, "capture")
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"capture", "retain", "output", "save", "complete-retained", "load"}
			if variant != "prepare" {
				want = []string{"capture", "cleanup", "quiescence", "output", "save", "complete-absent", "load"}
			}
			if !reflect.DeepEqual(*boundary.calls, want) || (result.retained != nil) != (variant == "prepare") {
				t.Fatal("persisted purpose selected the wrong ownership path", *boundary.calls, result.retained != nil)
			}
		})
	}
}
