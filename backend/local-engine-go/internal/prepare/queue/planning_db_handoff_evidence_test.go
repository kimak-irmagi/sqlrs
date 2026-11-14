package queue_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

func planningDBFactDigest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// PG02 requires validation of stored evidence, not only positive evidence IDs.
// A hostile appended fact is either refused by the real SQL guard or by the
// completion writer. The original exact fact is the same-phase positive control.
// No immutable row is edited and no schema/guard is disabled.
func TestCompleteRetainedPlanningRejectsChangedEvidence(t *testing.T) {
	for _, variant := range []string{"scope", "digest", "generation", "clone", "execution", "empty-observation", "incomplete-observation"} {
		t.Run(variant, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			w := f.writer(t)
			var err error
			f.retained, err = w.RecordPlanningDBRetention(t.Context(), f.attempt, 1, f.op, f.observe, mainPathJournalUTC)
			handoffCheck(t, err)
			var raw, scope, digest string
			handoffCheck(t, f.db.QueryRow("SELECT facts_json,scope_digest,facts_digest FROM prepare_attempt_evidence WHERE evidence_id=?", f.retained).Scan(&raw, &scope, &digest))
			generation := 1
			var fields map[string]json.RawMessage
			handoffCheck(t, json.Unmarshal([]byte(raw), &fields))
			switch variant {
			case "scope":
				scope = pgDigest("f")
			case "digest":
				digest = pgDigest("f")
			case "generation":
				generation = 2
			case "clone":
				fields["clone_id"] = json.RawMessage(`"other-clone"`)
			case "execution":
				fields["execution_digest"] = json.RawMessage(`"` + pgDigest("f") + `"`)
			case "empty-observation":
				fields["observation"] = json.RawMessage(`{}`)
			case "incomplete-observation":
				var observation map[string]json.RawMessage
				handoffCheck(t, json.Unmarshal(fields["observation"], &observation))
				// Remove every field in turn would test a Cartesian matrix of
				// implementation spellings. Keeping one field proves a partial
				// observation is insufficient, independent of its field naming.
				for key, value := range observation {
					observation = map[string]json.RawMessage{key: value}
					break
				}
				fields["observation"], err = json.Marshal(observation)
				handoffCheck(t, err)
			}
			if variant != "scope" && variant != "digest" && variant != "generation" {
				body, err := json.Marshal(fields)
				handoffCheck(t, err)
				raw = string(body)
				digest = planningDBFactDigest(raw)
			}
			before := mainPathJournalRows(t, f.db)
			result, insertErr := f.db.Exec(`INSERT INTO prepare_attempt_evidence(job_id,task_id,attempt_no,kind,scope_digest,facts_json,facts_digest,worker_generation,observed_at) VALUES(?,'plan',1,'planning_db_retained',?,?,?,?,?)`, f.attempt.JobID, scope, raw, digest, generation, mainPathJournalUTC)
			var wrong int64
			if insertErr != nil {
				var code interface{ Code() int }
				if !errors.As(insertErr, &code) || code.Code()&255 != 19 {
					t.Fatal("unrelated hostile-fact preparation failure", insertErr)
				}
				f.unchanged(t, before)
			} else {
				wrong, err = result.LastInsertId()
				handoffCheck(t, err)
			}
			f.output, err = f.q.RecordVerifiedPlanningCapture(t.Context(), f.attempt, 1, queue.PlanningCaptureProof{StdoutDigest: pgDigest("2"), StderrDigest: pgDigest("3"), StdoutBytes: 100, ManifestRef: f.capture.Manifest}, mainPathJournalUTC)
			handoffCheck(t, err)
			f.planDigest, err = f.q.SavePlanningResult(t.Context(), f.attempt.JobID, 1, pgDigest("2"), f.changes, mainPathJournalUTC)
			handoffCheck(t, err)
			if insertErr == nil {
				before = mainPathJournalRows(t, f.db)
				if err := w.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, wrong, f.output, mainPathJournalUTC); err == nil {
					t.Fatal("changed stored evidence authorised retention")
				}
				f.unchanged(t, before)
			}
			handoffCheck(t, w.CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
		})
	}
}

// PG02's plan-byte control reaches the saved plan's existing append-only
// boundary. A failed mutation must not prevent completing the unchanged plan.
func TestCompleteRetainedPlanningSavedPlanImmutable(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.saveRetainedPlan(t)
	for _, statement := range []string{
		"UPDATE prepare_planning_changesets SET sql_hash='" + strings.Repeat("f", 64) + "' WHERE job_id=? AND ordinal=0",
		"UPDATE prepare_planning_intents SET parsed_output_digest='" + pgDigest("f") + "' WHERE job_id=?",
		"UPDATE prepare_planning_intents SET plan_digest='" + pgDigest("f") + "' WHERE job_id=?",
	} {
		before := mainPathJournalRows(t, f.db)
		if _, err := f.db.Exec(statement, f.attempt.JobID); err == nil {
			t.Fatal("saved plan bytes or output binding changed")
		}
		f.unchanged(t, before)
	}
	handoffCheck(t, f.writer(t).CompleteRetainedPlanning(t.Context(), f.attempt, 1, f.exit, f.retained, f.output, mainPathJournalUTC))
}
