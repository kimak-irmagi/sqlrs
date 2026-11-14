package prepare

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// CT06/CT07: use the actual constructor and worker, not a recorder-only model.
// A capability refusal must remain a refusal with or without measurements.
func TestCostWorkerStartupSelectionAndFailedRoots(t *testing.T) {
	for _, value := range []string{"", "0", "true", " 1", "1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SQLRS_PREPARE_COST", value)
			m := newManager(t, &fakeStore{})
			enabled := value == "1"
			if (m.costEmitter != nil) != enabled {
				t.Fatal("constructor did not freeze exact explicit selection")
			}
			// Later environment changes cannot opt an existing engine in/out.
			t.Setenv("SQLRS_PREPARE_COST", "1")
			var output bytes.Buffer
			if enabled {
				m.costEmitter = preparecost.NewEmitter(&output)
			}
			for _, id := range []string{"private-first-job", "private-second-job"} {
				(&jobCoordinator{m: m}).runContainerLiquibase(preparedRequest{}, id)
				if len(m.running) != 0 {
					t.Fatal("recording retained a finished worker")
				}
			}
			if !enabled {
				if output.Len() != 0 || m.costSequence.Load() != 0 {
					t.Fatal("disabled worker recorded evidence")
				}
				return
			}
			text := output.String()
			if strings.Count(text, "begin ") != 2 || strings.Count(text, "end ") != 2 ||
				!strings.Contains(text, "begin 1 ") || !strings.Contains(text, "begin 2 ") ||
				strings.Count(text, "prepare.worker worker 0 ") != 2 || strings.Count(text, " failed\n") != 2 || strings.Contains(text, "private-") {
				t.Fatal("worker roots/outcome/framing/privacy differ from independent expectations")
			}
		})
	}
}

type costWorkerJournal struct{ normalLiquibaseJournal }

// The real worker must not swallow/change a panic, and still releases its
// registration and finishes its observer without pretending prepare succeeded.
func TestCostWorkerPanicRetainsOutcomeAndCleanup(t *testing.T) {
	t.Setenv("SQLRS_PREPARE_COST", "1")
	m, p, _, _, _ := managedExecutionFixture(t)
	var output bytes.Buffer
	m.costEmitter = preparecost.NewEmitter(&output)
	m.queue = &costWorkerJournal{}
	m.runtime = &engineRuntime.DockerRuntime{}
	primary := &struct{ value int }{42}
	m.validateStore = func(string) error { panic(primary) }
	func() {
		defer func() {
			if recover() != primary {
				t.Fatal("measurement changed the original panic")
			}
		}()
		(&jobCoordinator{m: m}).runContainerLiquibase(p, "private-panic-job")
	}()
	if len(m.running) != 0 || strings.Count(output.String(), " panicked\n") != 1 || strings.Contains(output.String(), "private-") {
		t.Fatal("panic observer lost exact outcome/cleanup or disclosed identity")
	}
}
