package queue_test

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// CP2-D uses actual cache publications and exact task bindings. The negative
// checkpoints never replace durable authority with caller flags.
func TestPlanningDBCacheAuthorityRecheck(t *testing.T) {
	for _, variant := range []string{"noncached", "incomplete", "all-cached-mismatch", "wrong-first-binding", "cancelled-cache", "owner-read", "owner-snapshot", "request-fact"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			states := planningDBPublishCache(t, f)
			f.complete(t)
			f.retireCapture(t)
			l := f.firstLaunchWithStates(t, states)
			binding := queue.ExecutionBinding{Digest: l.ExecutionDigest, PrepareKind: "lb", CommandDigest: pgDigest("9"), InputManifestDigest: f.record.BundleManifestDigest, ExecutorImageDigest: f.record.ImageManifestDigest, TargetIdentityDigest: f.record.TargetIdentityDigest, PolicyRevision: "lb-container-v2", EffectScope: "unknown", AdapterBindingDigest: f.record.ExecutionFingerprint}
			for _, task := range []string{"two", "three"} {
				if variant != "wrong-first-binding" || task != "two" {
					handoffCheck(t, f.q.BindStep(t.Context(), f.attempt.JobID, task, binding, mainPathJournalUTC))
				}
			}
			yes, no, succeeded, running := true, false, "succeeded", "running"
			cached := &yes
			if variant == "noncached" {
				cached = &no
			}
			handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, "one", queue.TaskUpdate{Status: &succeeded, Cached: cached}))
			if variant == "wrong-first-binding" {
				handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, "two", queue.TaskUpdate{Status: &running}))
			} else if variant != "noncached" && variant != "incomplete" {
				for _, task := range []string{"two", "three"} {
					handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, task, queue.TaskUpdate{Status: &succeeded, Cached: &yes}))
				}
			}
			reason, positive := "full_cache", "full_cache"
			if variant == "wrong-first-binding" || variant == "all-cached-mismatch" {
				reason = "source_mismatch"
			}
			if variant == "wrong-first-binding" {
				positive = reason
			}
			if variant == "cancelled-cache" {
				_, e := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
				handoffCheck(t, e)
				positive = "accepted_cancel"
			}
			var request int64
			if variant == "request-fact" {
				var e error
				request, e = f.q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, reason, mainPathJournalUTC)
				handoffCheck(t, e)
			}
			q, c := planningDBFaultQueue(t, f)
			switch variant {
			case "owner-read":
				c.query = "SELECT job_id,task_id,attempt_no,clone_id,retained_evidence_id"
			case "owner-snapshot":
				c.query = "SELECT facts_json,scope_digest,facts_digest"
				c.whenID = f.retained
			case "request-fact":
				c.query = "SELECT facts_json,scope_digest,facts_digest"
				c.whenID = request
			}
			before := mainPathJournalRows(t, f.db)
			id, e := q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, reason, mainPathJournalUTC)
			if e == nil || id != 0 {
				t.Fatal("incomplete cache authority granted cleanup", id, e)
			}
			f.unchanged(t, before)
			switch variant {
			case "noncached", "incomplete":
				for _, task := range []string{"one", "two", "three"} {
					handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, task, queue.TaskUpdate{Status: &succeeded, Cached: &yes}))
				}
			case "wrong-first-binding":
				handoffCheck(t, f.q.BindStep(t.Context(), f.attempt.JobID, "two", binding, mainPathJournalUTC))
			}
			_, e = q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, positive, mainPathJournalUTC)
			handoffCheck(t, e)
		})
	}
}

func TestPlanningDBDisposalTupleRecheck(t *testing.T) {
	for _, variant := range []string{"read-rollback", "changed-tuple", "stale-phase", "conflicting-time", "caller"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			_, e := f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
			handoffCheck(t, e)
			request, e := f.q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
			handoffCheck(t, e)
			handoffCheck(t, f.access.RetireOperation(t.Context(), f.op))
			f.op.Stage = "retired"
			absence := &planningDBAbsence{f: f, request: request}
			if variant == "conflicting-time" {
				handoffCheck(t, f.q.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, absence, mainPathJournalUTC))
			}
			q, c := planningDBFaultQueue(t, f)
			a, at := f.attempt, mainPathJournalUTC
			if variant == "caller" {
				a.PlannedExecutorName = "foreign"
			}
			if variant == "conflicting-time" {
				at = "2026-11-13T00:00:01Z"
			}
			if variant == "read-rollback" {
				c.rollback = true
			}
			verify := planningDBVerifyFunc(func(ctx context.Context, clone queue.ClonePlan, id string) error {
				if e := absence.VerifyReleasedClone(ctx, clone, id); e != nil {
					return e
				}
				if !c.hit {
					switch variant {
					case "changed-tuple":
						c.query = "SELECT job_id,task_id,attempt_no,clone_id,retained_evidence_id"
						c.column = "first_execution_digest"
						c.value = pgDigest("f")
					case "stale-phase":
						c.query = "SELECT a.job_id,a.task_id,a.attempt_no"
						c.indexes = map[int]driver.Value{22: "released"}
					}
				}
				return nil
			})
			before := mainPathJournalRows(t, f.db)
			e = q.CompletePlanningDBDisposal(t.Context(), a, 1, f.op, verify, at)
			if e == nil {
				t.Fatal("stale disposal proof granted completion")
			}
			if variant != "caller" && variant != "conflicting-time" && !c.hit {
				t.Fatal("wrong negative boundary")
			}
			if variant == "stale-phase" && c.changed != 1 {
				t.Fatal("clone phase was not independently changed", c.changed)
			}
			f.unchanged(t, before)
			handoffCheck(t, q.CompletePlanningDBDisposal(t.Context(), f.attempt, 1, f.op, absence, mainPathJournalUTC))
		})
	}
}
