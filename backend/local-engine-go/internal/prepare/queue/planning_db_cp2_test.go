package queue_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// CP2-H reaches the writer's independent metadata comparison; the observer
// explicitly approves only the unchanged physical operation, not caller fields.
func TestPlanningDBIndependentObserverRecheck(t *testing.T) {
	for _, variant := range []string{"caller", "operation-source", "phase-after-observation"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			q, c := planningDBFaultQueue(t, f)
			a := f.attempt
			operation := f.op
			if variant == "operation-source" {
				operation.Source = "foreign"
			}
			if variant == "caller" {
				a.ExecutorID = strings.Repeat("f", 64)
			}
			calls := 0
			observe := func(ctx context.Context, p queue.PlanningRecoverySnapshot, op instanceaccess.Operation) error {
				calls++
				// Observe the saved physical operation independently of the
				// caller's Source; only the journal can compare that saved field.
				if e := f.observe(ctx, p, f.op); e != nil {
					return e
				}
				if variant == "phase-after-observation" {
					c.query = "SELECT a.job_id,a.task_id,a.attempt_no"
					c.indexes = map[int]driver.Value{8: "created"}
				}
				return nil
			}
			before := mainPathJournalRows(t, f.db)
			id, e := q.RecordPlanningDBRetention(t.Context(), a, 1, operation, observe, mainPathJournalUTC)
			if e == nil || id != 0 || calls != 1 {
				t.Fatal("stale observation granted retention", id, e, calls)
			}
			if variant == "phase-after-observation" && !c.hit {
				t.Fatal("did not reach post-observation read")
			}
			if variant == "phase-after-observation" && c.changed != 1 {
				t.Fatal("phase was not independently changed", c.changed)
			}
			f.unchanged(t, before)
			_, e = q.RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC)
			handoffCheck(t, e)
		})
	}
}

func TestPlanningDBCompletionRecheck(t *testing.T) {
	for _, variant := range []string{"caller", "unproved-phase", "idempotent-proof"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.saveRetainedPlan(t)
			if variant == "idempotent-proof" {
				handoffCheck(t, f.q.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
			}
			q, c := planningDBFaultQueue(t, f)
			a := f.attempt
			switch variant {
			case "caller":
				a.ExecutorID = strings.Repeat("f", 64)
			case "unproved-phase":
				c.query = "SELECT a.job_id,a.task_id,a.attempt_no"
				c.indexes = map[int]driver.Value{8: "created"}
			case "idempotent-proof":
				c.query = "SELECT count(*) FROM prepare_liquibase_planning_db_ownership o JOIN"
				c.column = "count(*)"
				c.value = int64(0)
			}
			before := mainPathJournalRows(t, f.db)
			e := q.CompleteRetainedPlanning(t.Context(), a, 1, f.exit, f.retained, f.output, mainPathJournalUTC)
			if e == nil {
				t.Fatal("unproved completion recheck accepted")
			}
			if variant != "caller" && !c.hit {
				t.Fatal("wrong refusal boundary")
			}
			if variant == "unproved-phase" && c.changed != 1 {
				t.Fatal("completion phase was not independently changed", c.changed)
			}
			f.unchanged(t, before)
			handoffCheck(t, q.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
		})
	}
}

func TestPlanningDBSnapshotNotRepresentable(t *testing.T) {
	for _, variant := range []string{"json", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.saveRetainedPlan(t)
			q, c := planningDBFaultQueue(t, f)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.query = "SELECT * FROM prepare_jobs"
			if variant == "json" {
				c.column = "plan_only"
				c.value = math.Inf(1)
			} else {
				c.rows = "cancel"
				c.cancel = cancel
			}
			before := mainPathJournalRows(t, f.db)
			e := q.CompleteRetainedPlanning(ctx, f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC)
			if e == nil || !c.hit {
				t.Fatal("unreadable snapshot granted completion", e, c.hit)
			}
			f.unchanged(t, before)
			handoffCheck(t, q.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
		})
	}
}

// CP2-R: SQLite's exhausted Rows is already closed by database/sql.Next.
// A driver close failure must survive through Rows.Err before and after the
// duplicate explicit Close-error branch is removed from the suffix reader.
func TestPlanningDBSuffixCloseFailure(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	f.retireCapture(t)
	l := f.firstLaunch(t)
	q, c := planningDBFaultQueue(t, f)
	c.query = "SELECT job_id,task_id,position,type,status,planner_kind"
	c.rows = "close"
	before := mainPathJournalRows(t, f.db)
	_, e := q.TransferPlanningDBToFirstSession(t.Context(), l)
	if e == nil || !c.hit {
		t.Fatal("close failure granted SQL authority", e, c.hit)
	}
	f.unchanged(t, before)
	_, e = q.TransferPlanningDBToFirstSession(t.Context(), l)
	handoffCheck(t, e)
}

// CP2-T is a positive physical-owner metadata transfer, with a source produced
// by three actual publications. It must not allocate another clone/generation.
func TestPlanningDBPublishedStateSourceTransfer(t *testing.T) {
	f := planningDBStateSource(t)
	f.complete(t)
	f.retireCapture(t)
	states := []string{strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)}
	launch := f.firstLaunchFromSource(t, states, "state", f.clone.SourceStateID)
	before := f.count(t, "SELECT count(*) FROM prepare_clones WHERE job_id=?", f.attempt.JobID)
	s, e := f.q.TransferPlanningDBToFirstSession(t.Context(), launch)
	handoffCheck(t, e)
	if s.StartKind != "state" || s.StartID != f.clone.SourceStateID || s.CloneID != f.clone.ID || s.WorkerGeneration != f.attempt.WorkerGeneration || f.count(t, "SELECT count(*) FROM prepare_clones WHERE job_id=?", f.attempt.JobID) != before {
		t.Fatal("state source was replaced", s)
	}
	var runtime, source, target, operation string
	var generation int
	handoffCheck(t, f.db.QueryRow(`SELECT c.db_runtime_id,o.source_ref,o.target_path,o.operation_ref,c.clone_generation FROM prepare_clones c JOIN managed_runtime_operations o ON o.runtime_ref=c.db_runtime_id WHERE c.clone_id=?`, s.CloneID).Scan(&runtime, &source, &target, &operation, &generation))
	if runtime != f.op.RuntimeRef || source != f.clone.SourceStateID || target != f.op.Target || operation != f.op.Ref || generation != f.clone.Generation {
		t.Fatal("state transfer rewrote original operation")
	}
	journal := mainPathJournalRows(t, f.db)
	_, e = f.q.TransferPlanningDBToFirstSession(t.Context(), launch)
	handoffCheck(t, e)
	f.unchanged(t, journal)
}

func TestPlanningDBTransferredReadbackRefusal(t *testing.T) {
	for _, variant := range []string{"retained-fact", "typed-fact", "suffix-plan", "session-readback"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			f.retireCapture(t)
			l := f.firstLaunch(t)
			if variant == "session-readback" {
				_, e := f.q.TransferPlanningDBToFirstSession(t.Context(), l)
				handoffCheck(t, e)
			}
			q, c := planningDBFaultQueue(t, f)
			switch variant {
			case "retained-fact":
				c.query = "SELECT facts_json,scope_digest,facts_digest"
				c.whenID = f.retained
			case "typed-fact":
				var raw string
				handoffCheck(t, f.db.QueryRow("SELECT facts_json FROM prepare_attempt_evidence WHERE evidence_id=?", f.retained).Scan(&raw))
				var fact map[string]any
				handoffCheck(t, json.Unmarshal([]byte(raw), &fact))
				fact["observation"].(map[string]any)["clone_generation"] = "not-a-generation"
				rawBytes, e := json.Marshal(fact)
				handoffCheck(t, e)
				c.query = "SELECT facts_json,scope_digest,facts_digest"
				c.whenID = f.retained
				c.values = map[string]driver.Value{"facts_json": string(rawBytes), "facts_digest": planningDBFactDigest(string(rawBytes))}
			case "suffix-plan":
				c.query = "SELECT plan_codec_revision,execution_digest,result_attempt_no"
				c.skip = 2
			case "session-readback":
				c.query = "SELECT session_id,job_id,clone_id,session_no"
				c.column = "runner_config_digest"
				c.value = pgDigest("f")
			}
			before := mainPathJournalRows(t, f.db)
			_, e := q.TransferPlanningDBToFirstSession(t.Context(), l)
			if e == nil || !c.hit {
				t.Fatal("incomplete transferred readback accepted", e, c.hit)
			}
			f.unchanged(t, before)
			_, e = q.TransferPlanningDBToFirstSession(t.Context(), l)
			handoffCheck(t, e)
		})
	}
}
