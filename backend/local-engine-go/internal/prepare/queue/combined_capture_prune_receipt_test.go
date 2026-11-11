package queue_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/statefs"
	enginestore "github.com/sqlrs/engine-local/internal/store"
	sqlitedriver "modernc.org/sqlite"
)

// Only transport is controlled: the actual runtime must validate the archive,
// native response, parent observations and return the opaque receipt itself.
type combinedQueueTransport struct {
	id, name string
	wire     []byte
	calls    int
}

func (r *combinedQueueTransport) Run(_ context.Context, _ string, args []string, _ *string) (string, error) {
	r.calls++
	if len(args) != 5 || args[0] != "container" || args[4] != r.id {
		return "", errors.New("unexpected native inspection")
	}
	b, _ := json.Marshal(map[string]any{"Id": r.id, "Name": "/" + r.name, "State": map[string]any{"Status": "running", "ExitCode": 0}})
	return string(b), nil
}
func (r *combinedQueueTransport) RunBounded(_ context.Context, _ string, args []string, _, _ int64) ([]byte, error) {
	r.calls++
	if len(args) == 3 && args[0] == "cp" {
		return r.wire, nil
	}
	if len(args) > 0 && args[0] == "exec" {
		return []byte("sqlrs-step-idle-pruned-v1\n"), nil
	}
	return nil, errors.New("unexpected bounded observation")
}

func combinedQueueCapture(t *testing.T, s queue.LiquibaseSessionRecord, b queue.LiquibaseInvocationRecord, plan queue.CapturePrunePlan) (engineRuntime.CompleteStepCapture, engineRuntime.StepCaptureIdentity, *combinedQueueTransport) {
	t.Helper()
	a := engineRuntime.CapturePruneAuthorization{RetirementRef: plan.RetirementRef, SessionID: plan.SessionID, WorkerGeneration: plan.WorkerGeneration, RunnerID: plan.RunnerID, PriorInvocationRef: plan.PriorInvocationRef, PriorManifestDigest: plan.PriorManifestDigest, NextInvocationRef: plan.NextInvocationRef, HelperDigest: plan.HelperDigest}
	i := engineRuntime.StepCaptureIdentity{JobID: s.JobID, TaskID: b.Attempt.TaskID, SessionID: s.SessionID, InvocationRef: b.InvocationRef, AttemptNo: b.Attempt.AttemptNo, WorkerGeneration: s.WorkerGeneration, RunnerID: s.RunnerID, ExecutionDigest: b.Attempt.ExecutionDigest, HelperDigest: s.CaptureHelperDigest, PriorRetirementRef: b.PriorRetirementRef, Prune: &a}
	empty := fmt.Sprintf("%x", sha256.Sum256(nil))
	manifest, err := json.Marshal(map[string]any{"protocol": "lb-step-capture-v2", "job_id": i.JobID, "task_id": i.TaskID, "session_id": i.SessionID, "invocation_ref": i.InvocationRef, "attempt_no": i.AttemptNo, "worker_generation": i.WorkerGeneration, "runner_id": i.RunnerID, "execution_digest": i.ExecutionDigest, "helper_digest": i.HelperDigest, "prior_retirement_ref": i.PriorRetirementRef, "child_exit_code": 0, "stdout_eof": true, "stderr_eof": true, "stdout_bytes": 0, "stderr_bytes": 0, "stdout_sha256": empty, "stderr_sha256": empty})
	if err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	w := tar.NewWriter(&wire)
	for _, entry := range []struct {
		name string
		data []byte
	}{{"manifest.complete", manifest}, {"stdout.complete", nil}, {"stderr.complete", nil}} {
		if err := w.WriteHeader(&tar.Header{Name: entry.name, Size: int64(len(entry.data)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := &combinedQueueTransport{id: s.RunnerID, name: s.PlannedRunnerName, wire: wire.Bytes()}
	capture, err := engineRuntime.NewDocker(engineRuntime.Options{Runner: r}).ReadCompleteLiquibaseStepOutput(t.Context(), s.PlannedRunnerName, s.RunnerID, "", i, 1024)
	if err != nil {
		t.Fatal("actual runtime receipt unavailable", err)
	}
	return capture, i, r
}

type combinedQueueVerifier struct {
	db       *sql.DB
	identity engineRuntime.StepCaptureIdentity
	capture  engineRuntime.CompleteStepCapture
	calls    int
}

func (v *combinedQueueVerifier) VerifyLiquibaseCapturePruned(ctx context.Context, p queue.CapturePrunePlan) error {
	v.calls++
	query, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var slots int
	// MaxOpenConns(1): this query cannot complete while the queue holds its
	// own transaction/connection. The physical observation must precede it.
	if err := v.db.QueryRowContext(query, "SELECT count(*) FROM prepare_liquibase_capture_slots WHERE session_id=?", p.SessionID).Scan(&slots); err != nil {
		return err
	}
	a := engineRuntime.CapturePruneAuthorization{RetirementRef: p.RetirementRef, SessionID: p.SessionID, WorkerGeneration: p.WorkerGeneration, RunnerID: p.RunnerID, PriorInvocationRef: p.PriorInvocationRef, PriorManifestDigest: p.PriorManifestDigest, NextInvocationRef: p.NextInvocationRef, HelperDigest: p.HelperDigest}
	if slots != 2 || !v.capture.MatchesPruneObservation(v.identity, a) {
		return queue.ErrCommonAttempt
	}
	return nil
}

// CPR05: normal real queue + actual runtime receipt, with only the two approved
// transaction controls. Exhaustive crash/connection matrices stay deferred.
func TestCombinedCapturePruneQueue(t *testing.T) {
	for _, variant := range []string{"control", "missing_receipt", "wrong_completion", "before_effect", "lost_commit_reply"} {
		t.Run(variant, func(t *testing.T) {
			db, q, s, execution := mainPathSessionJournal(t)
			db.SetMaxOpenConns(1)
			a, err := q.PlanLiquibaseSessionInvocation(t.Context(), s, "execute", execution, mainPathInvocationPlan("combined-first"))
			if err != nil {
				t.Fatal(err)
			}
			s = mainPathPublishStep(t, db, q, s, a, strings.Repeat("2", 64))
			b := mainPathNextInvocation(t, q, s, execution, "execute-next", "combined-next")
			p, err := q.ReadLiquibaseCapturePrunePlan(t.Context(), s, b)
			if err != nil || p == nil {
				t.Fatal("real saved authority missing", err)
			}
			if err := q.RequestLiquibaseSessionExec(t.Context(), s, b, mainPathJournalUTC); err != nil {
				t.Fatal(err)
			}
			capture, identity, transport := combinedQueueCapture(t, s, b, *p)
			verifier := &combinedQueueVerifier{db: db, identity: identity, capture: capture}
			var reply *handoffReplyConnector
			var file string
			if err := db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file); err != nil {
				t.Fatal(err)
			}
			if variant == "lost_commit_reply" {
				reply = &handoffReplyConnector{path: file, native: &sqlitedriver.Driver{}}
				handle := sql.OpenDB(reply)
				handle.SetMaxOpenConns(1)
				defer handle.Close()
				q, err = queue.New(handle)
				if err != nil {
					t.Fatal(err)
				}
				reply.armed = true
			}
			if variant == "missing_receipt" {
				verifier.capture = engineRuntime.CompleteStepCapture{}
			}
			if variant == "wrong_completion" {
				verifier.capture.ManifestBytes++
			}
			if variant == "before_effect" {
				if _, err := db.Exec(`CREATE TRIGGER test_combined_before_effect BEFORE UPDATE ON prepare_liquibase_capture_retirements BEGIN SELECT RAISE(ABORT,'CPR before effect'); END`); err != nil {
					t.Fatal(err)
				}
			}
			before := mainPathJournalRows(t, db)
			err = q.ConfirmLiquibaseCapturePruned(t.Context(), s, b, verifier, mainPathJournalUTC)
			positive := variant == "control" || variant == "lost_commit_reply"
			if positive {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || !reflect.DeepEqual(before, mainPathJournalRows(t, db)) {
					t.Fatal("refused proof/transaction changed journal", err)
				}
				if variant == "before_effect" && !strings.Contains(err.Error(), "CPR before effect") {
					t.Fatal("wrong transaction boundary", err)
				}
			}
			if verifier.calls != 1 || transport.calls != 4 {
				t.Fatal("journal repeated native observation")
			}
			if !positive {
				return
			}
			if reply != nil && reply.commits != 1 {
				t.Fatal("lost real commit reply was not reached once")
			}
			after := mainPathJournalRows(t, db)
			for table, rows := range before {
				if table != "prepare_liquibase_capture_slots" && table != "prepare_liquibase_capture_retirements" && after[table] != rows {
					t.Fatal("confirmation changed unrelated rows", table)
				}
			}
			peer, err := sql.Open("sqlite", file)
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if !reflect.DeepEqual(after, mainPathJournalRows(t, peer)) {
				t.Fatal("native confirmation not independently committed")
			}
			if err := q.ConfirmLiquibaseCapturePruned(t.Context(), s, b, nil, mainPathJournalUTC); err != nil || verifier.calls != 1 || transport.calls != 4 {
				t.Fatal("already confirmed proof repeated effects", err)
			}
			complete, err := q.CompleteLiquibaseSessionInvocation(t.Context(), s, b, queue.LiquibaseInvocationCompletion{ExitCode: capture.ExitCode, ManifestDigest: capture.ManifestDigest, ManifestBytes: capture.ManifestBytes}, mainPathJournalUTC)
			if err != nil {
				t.Fatal(err)
			}
			quiet, err := q.RecordLiquibaseSessionDBStopped(t.Context(), s, b, "sqlrs-db-main-path", strings.Repeat("d", 64), mainPathJournalUTC)
			if err != nil {
				t.Fatal(err)
			}
			job, _, err := q.GetJob(t.Context(), s.JobID)
			if err != nil {
				t.Fatal(err)
			}
			state := strings.Repeat("3", 64)
			dir := filepath.Join(t.TempDir(), state)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(statefs.PublishedMarkerPath(dir, "copy"), []byte("ok"), 0600); err != nil {
				t.Fatal(err)
			}
			parent := s.CursorID
			entry := enginestore.StateCreate{StateID: state, StateFingerprint: state, ParentStateID: &parent, ImageID: job.ResolvedImageID, PrepareKind: "lb", PrepareArgsNormalized: "fixture update-count", CreatedAt: mainPathJournalUTC, LineageRef: job.LineageRef, IdentityDigest: job.IdentityDigest, LiquibaseExecutionFingerprint: s.ExecutionFingerprint}
			if err := q.PublishLiquibaseSessionState(t.Context(), s, b, entry, dir, "copy", complete, quiet, mainPathJournalUTC); err != nil {
				t.Fatal(err)
			}
			s, _, err = q.ReadLiquibaseSession(t.Context(), s.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if err := q.RequestLiquibaseSessionDBRestart(t.Context(), s); err != nil {
				t.Fatal(err)
			}
			if err := q.MarkLiquibaseSessionDBReady(t.Context(), s, "sqlrs-db-main-path", strings.Repeat("d", 64)); err != nil {
				t.Fatal(err)
			}
			s.DBPhase = "running"
			third := mainPathNextInvocation(t, q, s, execution, "execute-third", "combined-third")
			if third.CaptureSlot != a.CaptureSlot {
				t.Fatal("opaque proof did not free the exact prior slot")
			}
			if transport.calls != 4 {
				t.Fatal("publication/third admission repeated native work")
			}
		})
	}
}
