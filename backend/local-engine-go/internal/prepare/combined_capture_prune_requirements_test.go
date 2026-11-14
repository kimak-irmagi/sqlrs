package prepare

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// combinedCoordinatorTransport obtains opaque receipts through the actual
// Docker runtime parser/observation. No test-only receipt constructor exists.
type combinedCoordinatorTransport struct {
	identity      engineRuntime.StepCaptureIdentity
	name          string
	wire          []byte
	calls, proofs int
}

func (r *combinedCoordinatorTransport) Run(_ context.Context, _ string, args []string, _ *string) (string, error) {
	r.calls++
	if len(args) > 1 && args[0] == "container" && args[1] == "inspect" {
		b, _ := json.Marshal(map[string]any{"Id": r.identity.RunnerID, "Name": "/" + r.name, "State": map[string]any{"Status": "running", "ExitCode": 0}})
		return string(b), nil
	}
	if len(args) > 0 && args[0] == "exec" {
		return "", nil
	}
	return "", errors.New("unexpected coordinator transport")
}

func (r *combinedCoordinatorTransport) RunBounded(_ context.Context, _ string, args []string, out, diagnostic int64) ([]byte, error) {
	r.calls++
	if diagnostic != 4096 {
		return nil, errors.New("wrong bounded diagnostic cap")
	}
	if len(args) == 3 && args[0] == "cp" && args[1] == r.identity.RunnerID+":/sqlrs/capture/"+r.identity.InvocationRef+"/." {
		return r.wire, nil
	}
	if len(args) > 0 && args[0] == "exec" && out == 64 {
		r.proofs++
		return []byte("sqlrs-step-idle-pruned-v1\n"), nil
	}
	return nil, errors.New("unexpected coordinator bounded transport")
}

func combinedCoordinatorArchive(i engineRuntime.StepCaptureIdentity, exit int) ([]byte, error) {
	empty := fmt.Sprintf("%x", sha256.Sum256(nil))
	m := map[string]any{"protocol": "lb-step-capture-v2", "job_id": i.JobID, "task_id": i.TaskID, "session_id": i.SessionID, "invocation_ref": i.InvocationRef, "attempt_no": i.AttemptNo, "worker_generation": i.WorkerGeneration, "runner_id": i.RunnerID, "execution_digest": i.ExecutionDigest, "helper_digest": i.HelperDigest, "prior_retirement_ref": i.PriorRetirementRef, "child_exit_code": exit, "stdout_eof": true, "stderr_eof": true, "stdout_bytes": 0, "stderr_bytes": 0, "stdout_sha256": empty, "stderr_sha256": empty}
	manifest, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, entry := range []struct {
		name string
		data []byte
	}{{"manifest.complete", manifest}, {"stdout.complete", nil}, {"stderr.complete", nil}} {
		if err := w.WriteHeader(&tar.Header{Name: entry.name, Mode: 0600, Size: int64(len(entry.data)), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := w.Write(entry.data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

type combinedCoordinatorRuntime struct {
	*sessionCoordinatorRuntime
	t         *testing.T
	exit      int
	empty     bool
	after     context.CancelFunc
	transport *combinedCoordinatorTransport
}

func (r *combinedCoordinatorRuntime) InvokeTrackedLiquibaseStep(ctx context.Context, name, _ string, i engineRuntime.StepCaptureIdentity, _ []string, _ int64) (engineRuntime.CompleteStepCapture, error) {
	if !r.journal.pending || !r.journal.requested {
		r.t.Fatal("combined invocation preceded saved intent")
	}
	r.calls++
	wire, err := combinedCoordinatorArchive(i, r.exit)
	if err != nil {
		r.t.Fatal(err)
	}
	r.transport = &combinedCoordinatorTransport{identity: i, name: name, wire: wire}
	c, err := engineRuntime.NewDocker(engineRuntime.Options{Runner: r.transport}).ReadCompleteLiquibaseStepOutput(ctx, name, i.RunnerID, "", i, 1024)
	if r.empty {
		c = engineRuntime.CompleteStepCapture{ExitCode: r.exit, ManifestDigest: c.ManifestDigest, ManifestBytes: c.ManifestBytes}
	}
	if r.after != nil {
		r.after()
	}
	return c, err
}

// CPR05/06: coordinator consumes an actual runtime receipt without issuing the
// separate probe. The real SQLite/slot boundary is independently qualified by
// the companion queue test, then connected end-to-end by native prepare.
func TestCombinedCapturePruneCoordinator(t *testing.T) {
	for _, variant := range []string{"control", "child_7", "missing_receipt", "confirmation_refused", "cancel_before_confirmation"} {
		t.Run(variant, func(t *testing.T) {
			c, j, base := sessionCoordinatorFixture()
			j.session.RunnerID, j.session.RunnerPhase = base.id, "running"
			base.status = "running"
			j.completed = 1
			j.priorInvocation = "published-first"
			journal := &mainPathDispatchJournal{sessionCoordinatorJournal: j}
			r := &combinedCoordinatorRuntime{sessionCoordinatorRuntime: base, t: t}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch variant {
			case "child_7":
				r.exit = 7
			case "missing_receipt":
				r.empty = true
			case "confirmation_refused":
				journal.confirmationErr = errors.New("confirmation declined")
			case "cancel_before_confirmation":
				r.after = cancel
			}
			c.journal, c.runtime = journal, r
			plan := queue.LiquibaseInvocationPlan{InvocationRef: "combined-next", StdoutRef: "combined-next-out", StderrRef: "combined-next-err", ManifestRef: "combined-next-manifest", ProtocolRevision: "lb-step-capture-v2"}
			_, capture, _, err := c.invokeStep(ctx, j.session, "execute-next", "sha256:"+strings.Repeat("d", 64), plan, "", []string{"update-count", "--count", "1"})
			positive := variant == "control" || variant == "child_7"
			if positive {
				if err != nil || capture.ExitCode != r.exit || j.completed != 2 || j.pending {
					t.Fatal("valid native receipt/outcome refused", err)
				}
			} else if err == nil || j.completed != 1 || !j.pending {
				t.Fatal("missing/refused receipt advanced completion")
			}
			if base.calls != 1 || r.transport == nil || r.transport.proofs != 1 || r.transport.calls != 4 || journal.confirmationCalls != 1 {
				t.Fatal("confirmation repeated native proof/SQL or missed its selected boundary")
			}
			if variant == "confirmation_refused" && !errors.Is(err, journal.confirmationErr) {
				t.Fatal("wrong confirmation refusal")
			}
			if variant == "cancel_before_confirmation" && !errors.Is(err, context.Canceled) {
				t.Fatal("confirmation lost cancellation classification")
			}
		})
	}
}
