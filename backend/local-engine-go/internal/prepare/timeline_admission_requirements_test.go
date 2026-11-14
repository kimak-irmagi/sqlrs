package prepare

import (
	"github.com/sqlrs/engine-local/internal/preparecost"
	"testing"
)

// TL03 observes real validation refusal before any job/image effect; the
// diagnostic callback must return exactly the original validation error.
func TestTimelineAdmissionInputsActualRefusal(t *testing.T) {
	m := newManager(t, &fakeStore{})
	recorder := preparecost.NewRecorder(true, 1)
	ctx, finish := preparecost.Start(preparecost.WithRecorder(t.Context(), recorder), preparecost.Admission)
	_, err := m.submitObservedForPurpose(ctx, Request{}, "prepare", nil)
	finish(preparecost.Failed)
	report := recorder.Finish()
	if err == nil || !report.Complete || len(report.Spans) != 2 || report.Spans[1].Operation != preparecost.AdmissionInputs || report.Spans[1].Outcome != preparecost.Failed {
		t.Fatal("actual validation boundary/result missing")
	}
}
