package queue

// Normal completion prerequisites from the approved journal. Shared readers
// validate saved authority; no recovery/replay dispatcher is installed.
// See docs/architecture/issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/prepare/finalizationcodec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// LiquibaseCacheSelection is the job-retained logical choice and temporary
// ownership of one independently published state. Discharged history does not
// pin target bytes. See liquibase-cache-selection-retention-schema.md.
type LiquibaseCacheSelection struct {
	JobID, TaskID, SessionID, CloneID                                   string
	AttemptNo, WorkerGeneration                                         int
	ExecutionDigest, TargetStateID                                      string
	SelectedAt                                                          string
	TransferredAt, SuccessorSessionID, ReleasedAt, ReleaseOutcomeDigest *string
}

func loadLiquibaseCacheTarget(ctx context.Context, reader sessionStopProofReader, jobID, taskID string, attemptNo int) (LiquibaseCacheSelection, bool, error) {
	var r LiquibaseCacheSelection
	var transferred, successor, released, digest sql.NullString
	var exact bool
	err := reader.QueryRowContext(ctx, `SELECT selection.job_id,selection.task_id,selection.attempt_no,
selection.session_id,selection.clone_id,selection.worker_generation,selection.execution_digest,selection.target_state_id,
selection.selected_at,selection.transferred_at,selection.successor_session_id,selection.released_at,selection.release_outcome_digest,
coalesce(s.job_id=selection.job_id AND s.clone_id=selection.clone_id AND s.worker_generation=selection.worker_generation
 AND i.session_id=selection.session_id AND a.clone_id=selection.clone_id AND a.worker_generation=selection.worker_generation
 AND a.execution_digest=selection.execution_digest AND a.executor_kind='session_exec'
 AND step.execution_digest=selection.execution_digest AND j.prepare_kind='lb',0)
FROM prepare_liquibase_cache_selections selection
LEFT JOIN prepare_jobs j ON j.job_id=selection.job_id
LEFT JOIN prepare_liquibase_sessions s ON s.session_id=selection.session_id
LEFT JOIN prepare_liquibase_invocations i USING(job_id,task_id,attempt_no)
LEFT JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
LEFT JOIN prepare_step_bindings step ON step.job_id=selection.job_id AND step.task_id=selection.task_id
WHERE selection.job_id=? AND selection.task_id=? AND selection.attempt_no=?`, jobID, taskID, attemptNo).Scan(
		&r.JobID, &r.TaskID, &r.AttemptNo, &r.SessionID, &r.CloneID, &r.WorkerGeneration, &r.ExecutionDigest, &r.TargetStateID,
		&r.SelectedAt, &transferred, &successor, &released, &digest, &exact)
	if errors.Is(err, sql.ErrNoRows) {
		return LiquibaseCacheSelection{}, false, nil
	}
	if err != nil {
		return LiquibaseCacheSelection{}, false, err
	}
	if !exact || r.JobID != jobID || r.TaskID != taskID || r.AttemptNo != attemptNo || r.WorkerGeneration < 1 ||
		r.SessionID == "" || r.CloneID == "" || r.TargetStateID == "" || !bindingDigest.MatchString(r.ExecutionDigest) ||
		!validFinalizationUTC(r.SelectedAt) || (transferred.Valid && !validFinalizationUTC(transferred.String)) ||
		(released.Valid && !validFinalizationUTC(released.String)) || (transferred.Valid && released.Valid) ||
		(successor.Valid && (!transferred.Valid || successor.String == "")) || released.Valid != digest.Valid ||
		(digest.Valid && !bindingDigest.MatchString(digest.String)) {
		return LiquibaseCacheSelection{}, false, ErrCommonAttempt
	}
	if transferred.Valid {
		r.TransferredAt = &transferred.String
	}
	if successor.Valid {
		r.SuccessorSessionID = &successor.String
	}
	if released.Valid {
		r.ReleasedAt = &released.String
	}
	if digest.Valid {
		r.ReleaseOutcomeDigest = &digest.String
	}
	return r, true, nil
}

// Verify immutable capture/session arguments even when an existing choice has
// already transferred. Mutable cursor/phase fields are intentionally not replay
// authority and may have legitimately progressed after a lost response.
func verifyCacheSelectionInvocation(ctx context.Context, reader sessionStopProofReader, session LiquibaseSessionRecord, i LiquibaseInvocationRecord) error {
	var exact bool
	err := reader.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM prepare_liquibase_sessions
WHERE `+liquibaseSessionIdentityPredicate+` AND runner_id=? AND EXISTS(
 SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 WHERE i.job_id=? AND i.task_id=? AND i.attempt_no=? AND i.session_id=prepare_liquibase_sessions.session_id
 AND i.invocation_ref=? AND i.stdout_ref=? AND i.stderr_ref=? AND i.manifest_ref=? AND i.helper_digest=? AND i.protocol_revision=?
 AND a.clone_id=prepare_liquibase_sessions.clone_id AND a.worker_generation=prepare_liquibase_sessions.worker_generation
 AND a.execution_digest=? AND a.executor_kind='session_exec'))`, append(sessionIdentityArgs(session), session.RunnerID,
		i.Attempt.JobID, i.Attempt.TaskID, i.Attempt.AttemptNo, i.InvocationRef, i.StdoutRef, i.StderrRef, i.ManifestRef,
		i.HelperDigest, i.ProtocolRevision, i.Attempt.ExecutionDigest)...).Scan(&exact)
	if err != nil {
		return err
	}
	if !exact {
		return ErrCommonAttempt
	}
	return nil
}

// earlyJobFailureStepAgreementSQL checks every recorded binding, including a
// working successor that has not invoked SQL yet. Matching adapter fingerprints
// alone do not excuse inconsistent target/image/policy metadata (JF03).
func earlyJobFailureStepAgreementSQL(job, adapter string) string {
	return `NOT EXISTS(SELECT 1 FROM prepare_step_bindings owner_step
LEFT JOIN prepare_execution_bindings owner_execution ON owner_execution.execution_digest=owner_step.execution_digest
WHERE owner_step.job_id=` + job + ` AND (owner_execution.execution_digest IS NULL
OR owner_execution.prepare_kind IS NOT 'lb'
OR owner_execution.adapter_binding_digest IS NOT ` + adapter + `.execution_fingerprint
OR owner_execution.input_manifest_digest IS NOT ` + adapter + `.bundle_manifest_digest
OR owner_execution.executor_image_digest IS NOT ` + adapter + `.image_manifest_digest
OR owner_execution.target_identity_digest IS NOT ` + adapter + `.target_identity_digest
OR owner_execution.policy_revision IS NOT ` + adapter + `.execution_policy_revision))`
}

// readSelectedFailureTailEvidence retains exact append-only IDs, bytes, hashes
// and times for every child of this session. These private values are compared
// in memory only; malformed rows are safely classified, never printed.
func readSelectedFailureTailEvidence(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord) ([]string, error) {
	var result []string
	err := scanFailureOwnerRows(ctx, tx, `SELECT json_array(e.evidence_id,e.job_id,e.task_id,e.attempt_no,e.kind,e.worker_generation,e.scope_digest,e.facts_json,e.facts_digest,e.observed_at)
FROM prepare_attempt_evidence e JOIN prepare_liquibase_invocations i USING(job_id,task_id,attempt_no)
WHERE i.session_id=? ORDER BY e.evidence_id`, session.SessionID, func(rows *sql.Rows) error {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		result = append(result, raw)
		return nil
	})
	if err != nil {
		return nil, failureOwnerReadError(ctx)
	}
	return result, nil
}

// stoppedFailureSessionState preserves the complete owner set and original
// immutable cursor identity. It returns no disposal authority for unknown
// create, unassociated child or a partially retired journal. Pending restores
// remain frozen sources until this exact session is physically disposed.
func stoppedFailureSessionState(owners LiquibaseFailureOwners, expected LiquibaseSessionRecord) (LiquibaseSessionRecord, LiquibaseFailureClone, []LiquibaseFailureInvocation, bool, error) {
	var session LiquibaseSessionRecord
	var clone LiquibaseFailureClone
	var children []LiquibaseFailureInvocation
	bad := func() (LiquibaseSessionRecord, LiquibaseFailureClone, []LiquibaseFailureInvocation, bool, error) {
		return session, clone, nil, false, ErrLiquibaseFinalization
	}
	found := false
	for _, item := range owners.Sessions {
		if item.SessionID == expected.SessionID {
			session, found = item, true
		}
	}
	if !found || !reflect.DeepEqual(sessionIdentityArgs(session), sessionIdentityArgs(expected)) || session.SessionNo != expected.SessionNo || session.StartKind != expected.StartKind || session.StartID != expected.StartID || session.StartPosition != expected.StartPosition || session.CreatedAt != expected.CreatedAt || session.CursorPosition != expected.CursorPosition || session.RunnerID != expected.RunnerID || !dockerContainerID.MatchString(session.RunnerID) || owners.Intent.ProjectionsRemovedAt != "" {
		return bad()
	}
	found = false
	for _, item := range owners.Clones {
		if item.Plan.ID == session.CloneID {
			clone, found = item, true
		}
	}
	if !found || !dockerContainerID.MatchString(clone.DBRuntimeID) {
		return bad()
	}
	for _, item := range owners.Invocations {
		if item.Record.SessionID == session.SessionID {
			children = append(children, item)
		}
	}
	for _, item := range owners.Attempts {
		if item.Record.CloneID != session.CloneID {
			continue
		}
		// O5 shares this clone with one closed planner. Its exact durable
		// transfer is part of the validated owner snapshot, not a session child
		// to stop/discard. An unrelated or unresolved attempt still refuses.
		if owner := owners.planningDB; owner != nil && owner.Session.String == session.SessionID &&
			owner.CloneID == session.CloneID && owner.TaskID == item.Record.TaskID && owner.AttemptNo == item.Record.AttemptNo &&
			item.ExecutorKind == "container" && item.Record.WorkerGeneration == session.WorkerGeneration &&
			item.Record.Phase == "closed" && item.Record.Resolution == "planning_result_saved" {
			continue
		}
		matched := false
		for _, child := range children {
			matched = matched || child.Record.Attempt.TaskID == item.Record.TaskID && child.Record.Attempt.AttemptNo == item.Record.AttemptNo && item.ExecutorKind == "session_exec"
		}
		if !matched {
			return bad()
		}
	}
	for _, item := range owners.Restores {
		if item.SessionID == session.SessionID && (session.RetiredAt != nil && item.Phase != "published" && item.Phase != "abandoned" || item.Phase != "requested" && item.Phase != "stopped" && item.Phase != "restored" && item.Phase != "published" && item.Phase != "abandoned") {
			return bad()
		}
	}
	if session.RetiredAt != nil {
		if session.RunnerPhase != "retired" || session.DBPhase != "stopped" || session.CursorKind != "" || session.CursorID != "" || clone.Phase != "released" || !validFinalizationUTC(*session.RetiredAt) || !retiredFailureCloneOperations(owners, clone.Plan) {
			return bad()
		}
		for _, child := range children {
			if child.RetiredAt == nil || child.Record.Attempt.Phase != "closed" || child.Record.Attempt.Resolution == "" {
				return bad()
			}
		}
		return session, clone, children, true, nil
	}
	if session.RunnerPhase != "stopped" || !reflect.DeepEqual(session, expected) || clone.Phase != "active" && clone.Phase != "quiescent" {
		return bad()
	}
	for _, child := range children {
		a := child.Record.Attempt
		if a.Resolution == "" && (child.RetiredAt != nil || a.Phase != "planned" && a.Phase != "exec_requested" && a.Phase != "exec_completed") || a.Resolution != "" && a.Phase != "closed" {
			return bad()
		}
	}
	return session, clone, children, false, nil
}

// failureStopLinks retains exact append-only observation IDs, not a new
// execution/replay decision. All three checksums/scopes are verified on load.
type failureStopLinks struct{ request, ack, stopped int64 }

func readSelectedFailureStopLinks(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, a AttemptRecord) (failureStopLinks, error) {
	var links failureStopLinks
	var count int
	err := tx.QueryRowContext(ctx, `SELECT count(*),coalesce(min(request.evidence_id),0),coalesce(min(ack.evidence_id),0),coalesce(min(stopped.evidence_id),0)
FROM prepare_attempt_evidence request JOIN prepare_attempt_evidence ack USING(job_id,task_id,attempt_no,worker_generation)
JOIN prepare_attempt_evidence stopped USING(job_id,task_id,attempt_no,worker_generation)
WHERE request.job_id=? AND request.task_id=? AND request.attempt_no=? AND request.worker_generation=?
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged' AND stopped.kind='executor_inspection'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id
AND json_extract(request.facts_json,'$.observation.session_id')=? AND json_extract(request.facts_json,'$.observation.runner_id')=?
AND json_extract(ack.facts_json,'$.observation.session_id')=? AND json_extract(ack.facts_json,'$.observation.runner_id')=?
AND json_extract(stopped.facts_json,'$.observation.session_id')=? AND json_extract(stopped.facts_json,'$.observation.runner_id')=?
AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1`, a.JobID, a.TaskID, a.AttemptNo, a.WorkerGeneration, session.SessionID, session.RunnerID, session.SessionID, session.RunnerID, session.SessionID, session.RunnerID).Scan(&count, &links.request, &links.ack, &links.stopped)
	if err != nil || count != 1 {
		return links, ErrLiquibaseFinalization
	}
	for _, id := range []int64{links.request, links.ack, links.stopped} {
		if _, err := readSelectedFailureEvidence(ctx, tx, a, id); err != nil {
			return links, err
		}
	}
	return links, nil
}

// readSelectedFailureEvidence validates checksum, scope, tuple and JSON
// envelope before the bytes may participate in destructive cleanup authority.
func readSelectedFailureEvidence(ctx context.Context, tx *sql.Tx, a AttemptRecord, id int64) (map[string]json.RawMessage, error) {
	var raw, digest, scope string
	if err := tx.QueryRowContext(ctx, `SELECT facts_json,facts_digest,scope_digest FROM prepare_attempt_evidence WHERE evidence_id=? AND job_id=? AND task_id=? AND attempt_no=? AND worker_generation=?`, id, a.JobID, a.TaskID, a.AttemptNo, a.WorkerGeneration).Scan(&raw, &digest, &scope); err != nil || commonEvidenceDigest(raw) != digest || scope != commonEvidenceDigest(fmt.Sprintf("attempt-scope-v1\n%s\n%s\n%d\n%s\n%s", a.JobID, a.TaskID, a.AttemptNo, a.CloneID, a.ExecutionDigest)) {
		return nil, ErrLiquibaseFinalization
	}
	var envelope struct {
		CloneID     string                     `json:"clone_id"`
		Digest      string                     `json:"execution_digest"`
		Observation map[string]json.RawMessage `json:"observation"`
	}
	if json.Unmarshal([]byte(raw), &envelope) != nil || envelope.CloneID != a.CloneID || envelope.Digest != a.ExecutionDigest || envelope.Observation == nil {
		return nil, ErrLiquibaseFinalization
	}
	return envelope.Observation, nil
}

// verifySelectedFailureDisposalTx checks the same relational proof used by SQL
// resolution guards plus every checksum and linked scope. It recognizes only
// the original committed proof IDs, never another newly generated observation.
func verifySelectedFailureDisposalTx(ctx context.Context, tx *sql.Tx, a AttemptRecord) error {
	var exact int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_step_attempts a WHERE a.job_id=? AND a.task_id=? AND a.attempt_no=? AND `+selectedFailureAttemptProofSQL("a", ""), a.JobID, a.TaskID, a.AttemptNo).Scan(&exact); err != nil || exact != 1 {
		return ErrLiquibaseFinalization
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT evidence_id FROM prepare_attempt_evidence WHERE job_id=? AND task_id=? AND attempt_no=? AND kind='selected_failure_disposal'`, a.JobID, a.TaskID, a.AttemptNo).Scan(&id); err != nil {
		return ErrLiquibaseFinalization
	}
	observation, err := readSelectedFailureEvidence(ctx, tx, a, id)
	if err != nil {
		return err
	}
	for _, field := range []string{"request_id", "ack_id", "stop_id", "database_id"} {
		var link int64
		if json.Unmarshal(observation[field], &link) != nil || link < 1 {
			return ErrLiquibaseFinalization
		}
		if _, err := readSelectedFailureEvidence(ctx, tx, a, link); err != nil {
			return err
		}
	}
	return nil
}

// selectedFailureEvidenceAuthoritySQL binds an existing invocation's disposal
// to the immutable job outcome and exact ordered runner/DB observations. The
// compiled aliases/phase expression are never supplied by an API client. SHA
// checks are additionally performed by the queue reader; SQL is not a native
// absence verifier. Requirements: failure storage §4, JF04/JF05/JF08.
func selectedFailureEvidenceAuthoritySQL(proof, attempt, originalPhase string, inserted bool) string {
	order := ""
	if inserted {
		order = ` AND $p.evidence_id>q.evidence_id`
	}
	phase := `json_extract($p.facts_json,'$.observation.original_phase') IN ('planned','exec_requested','exec_completed')`
	if originalPhase != "" {
		phase += ` AND json_extract($p.facts_json,'$.observation.original_phase')=` + originalPhase
	}
	return strings.NewReplacer("$p", proof, "$a", attempt).Replace(`EXISTS (
SELECT 1 FROM prepare_liquibase_invocations i
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id AND s.job_id=i.job_id
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=s.job_id
JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN prepare_step_bindings sb ON sb.job_id=i.job_id AND sb.task_id=i.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
JOIN prepare_attempt_evidence request ON request.evidence_id=json_extract($p.facts_json,'$.observation.request_id')
JOIN prepare_attempt_evidence ack ON ack.evidence_id=json_extract($p.facts_json,'$.observation.ack_id')
JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=json_extract($p.facts_json,'$.observation.stop_id')
JOIN prepare_attempt_evidence q ON q.evidence_id=json_extract($p.facts_json,'$.observation.database_id')
WHERE i.job_id=$a.job_id AND i.task_id=$a.task_id AND i.attempt_no=$a.attempt_no
AND $a.executor_kind='session_exec' AND $a.legacy_unbound=0
AND s.clone_id=$a.clone_id AND s.worker_generation=$a.worker_generation
AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND b.execution_digest=$a.execution_digest AND b.prepare_kind='lb'
AND b.adapter_binding_digest=s.execution_fingerprint AND s.execution_fingerprint=lb.execution_fingerprint
AND b.policy_revision=lb.execution_policy_revision AND b.input_manifest_digest=lb.bundle_manifest_digest
AND b.target_identity_digest=lb.target_identity_digest AND b.executor_image_digest=lb.image_manifest_digest
AND $p.job_id=$a.job_id AND $p.task_id=$a.task_id AND $p.attempt_no=$a.attempt_no
AND $p.worker_generation=$a.worker_generation AND $p.kind='selected_failure_disposal'
AND json_extract($p.facts_json,'$.clone_id')=$a.clone_id
AND json_extract($p.facts_json,'$.execution_digest')=$a.execution_digest
AND json_extract($p.facts_json,'$.observation.outcome_digest')=selected.outcome_digest
AND json_extract($p.facts_json,'$.observation.session_id')=s.session_id
AND json_extract($p.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract($p.facts_json,'$.observation.invocation_ref')=i.invocation_ref
AND json_extract($p.facts_json,'$.observation.clone_generation')=c.clone_generation
AND json_extract($p.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract($p.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract($p.facts_json,'$.observation.executor_absent')=1
AND json_extract($p.facts_json,'$.observation.runtime_absent')=1
AND ` + phase + `
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged'
AND stopped.kind='executor_inspection' AND q.kind='db_quiescence'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id` + order + `
AND request.job_id=$a.job_id AND request.task_id=$a.task_id AND request.attempt_no=$a.attempt_no AND request.worker_generation=$a.worker_generation
AND ack.job_id=$a.job_id AND ack.task_id=$a.task_id AND ack.attempt_no=$a.attempt_no AND ack.worker_generation=$a.worker_generation
AND stopped.job_id=$a.job_id AND stopped.task_id=$a.task_id AND stopped.attempt_no=$a.attempt_no AND stopped.worker_generation=$a.worker_generation
AND q.job_id=$a.job_id AND q.task_id=$a.task_id AND q.attempt_no=$a.attempt_no AND q.worker_generation=$a.worker_generation
AND json_extract(request.facts_json,'$.clone_id')=$a.clone_id AND json_extract(request.facts_json,'$.execution_digest')=$a.execution_digest
AND json_extract(ack.facts_json,'$.clone_id')=$a.clone_id AND json_extract(ack.facts_json,'$.execution_digest')=$a.execution_digest
AND json_extract(stopped.facts_json,'$.clone_id')=$a.clone_id AND json_extract(stopped.facts_json,'$.execution_digest')=$a.execution_digest
AND json_extract(q.facts_json,'$.clone_id')=$a.clone_id AND json_extract(q.facts_json,'$.execution_digest')=$a.execution_digest
AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.runtime_absent')=1)`)
}

// selectedFailureAttemptProofSQL is used both before resolution and during
// readback. A closed attempt must retain exactly one typed disposal proof.
func selectedFailureAttemptProofSQL(attempt, originalPhase string) string {
	return `EXISTS(SELECT 1 FROM prepare_attempt_evidence p WHERE ` + selectedFailureEvidenceAuthoritySQL("p", attempt, originalPhase, true) + `)
AND (SELECT count(*) FROM prepare_attempt_evidence single WHERE single.job_id=` + attempt + `.job_id AND single.task_id=` + attempt + `.task_id AND single.attempt_no=` + attempt + `.attempt_no AND single.kind='selected_failure_disposal')=1`
}

// jobOwnedOperationSQL recognizes exact direct working/instance paths and exact
// clone-journal references, including planning/replay physical owners. It never
// guesses an ownership suffix from a job ID. The native verifier must additionally
// validate the absolute engine root and saved physical identity before effects.
// Requirements: failure storage §3, full owner enumeration/freeze (JF04/05/08).
func jobOwnedOperationSQL(row, job string) string {
	target := `replace(` + row + `.target_path,char(92),'/')`
	ref := `replace(owner_clone.clone_ref,char(92),'/')`
	return `(substr(` + target + `,-length('/jobs/'||` + job + `||'/'||` + row + `.operation_ref))='/jobs/'||` + job + `||'/'||` + row + `.operation_ref
OR EXISTS(SELECT 1 FROM prepare_clones owner_clone WHERE owner_clone.job_id=` + job + `
AND substr(` + ref + `,1,5)='jobs/'
AND substr(` + ref + `,-length('/'||` + row + `.operation_ref))='/'||` + row + `.operation_ref
AND substr(` + target + `,-length('/'||` + ref + `))='/'||` + ref + `))`
}

// loadLiquibaseFailureOwnersTx shares the full consistent-snapshot validation
// with conditional retirement writes. It never acquires another connection.
func loadLiquibaseFailureOwnersTx(ctx context.Context, tx *sql.Tx, intent FinalizationSnapshot) (LiquibaseFailureOwners, error) {
	var none LiquibaseFailureOwners
	saved, found, err := loadFinalization(ctx, tx, intent.JobID)
	if err != nil || !found || !sameFinalizationIntent(saved, intent) || saved.Outcome.Kind != finalizationcodec.Failure {
		return none, failureOwnerReadError(ctx)
	}
	var count, version int
	if err := tx.QueryRowContext(ctx, `SELECT count(*),coalesce(max(format_version),0) FROM prepare_liquibase_finalization_store_format WHERE slot=1`).Scan(&count, &version); err != nil || count != 1 || version != 2 {
		return none, failureOwnerReadError(ctx)
	}
	if err := validateEarlyJobFailure(ctx, tx, saved.JobID, saved.Outcome); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	result := LiquibaseFailureOwners{Intent: saved}
	if owner, err := readPlanningDBOwner(ctx, tx, saved.JobID); err == nil {
		result.planningDB = &owner
	} else if !errors.Is(err, sql.ErrNoRows) {
		return none, failureOwnerReadError(ctx)
	}
	result.Job, err = scanJob(tx.QueryRowContext(ctx, `SELECT job_id,status,prepare_kind,image_id,plan_only,snapshot_mode,prepare_args_normalized,signature,request_json,created_at,started_at,finished_at,result_json,error_json,coalesce(resolved_image_id,''),coalesce(lineage_ref,''),coalesce(identity_digest,'') FROM prepare_jobs WHERE job_id=?`, saved.JobID))
	if err != nil {
		return none, failureOwnerReadError(ctx)
	}
	// The base queue scanner intentionally retains its independent deployment
	// shape. Read this component's extra columns in the SAME owned snapshot.
	var explanation sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT purpose,cache_explain_result_json FROM prepare_jobs WHERE job_id=?`, saved.JobID).Scan(&result.Job.Purpose, &explanation); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	result.Job.CacheExplainResultJSON = strPtr(explanation)
	result.Execution, found, err = readLiquibaseExecutionForJob(ctx, tx, saved.JobID)
	if err != nil || !found {
		return none, failureOwnerReadError(ctx)
	}
	if err := readFailureClones(ctx, tx, &result); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	if err := readFailureSessions(ctx, tx, &result); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	if err := readFailureAttempts(ctx, tx, &result); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	if err := readFailureInvocations(ctx, tx, &result); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	if err := readFailureOperations(ctx, tx, &result); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	if err := readFailureRestores(ctx, tx, &result); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	return result, nil
}

// failureOwnerReadError never exposes private row bytes, host paths or driver
// conversion errors. Cancellation remains distinguishable to the coordinator.
func failureOwnerReadError(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrLiquibaseFinalization
}

func readFailureClones(ctx context.Context, tx *sql.Tx, result *LiquibaseFailureOwners) error {
	return scanFailureOwnerRows(ctx, tx, `SELECT clone_id,clone_generation,source_kind,coalesce(source_image_id,''),coalesce(source_state_id,''),clone_ref,planned_db_runtime_name,phase,coalesce(db_runtime_id,'') FROM prepare_clones WHERE job_id=? ORDER BY clone_generation,clone_id`, result.Job.JobID, func(rows *sql.Rows) error {
		var item LiquibaseFailureClone
		p := &item.Plan
		if err := rows.Scan(&p.ID, &p.Generation, &p.SourceKind, &p.SourceImageID, &p.SourceStateID, &p.Ref, &p.PlannedRuntimeName, &item.Phase, &item.DBRuntimeID); err != nil {
			return err
		}
		if p.ID == "" || p.Generation <= 0 || p.Ref == "" || p.PlannedRuntimeName == "" || item.DBRuntimeID != "" && !dockerContainerID.MatchString(item.DBRuntimeID) {
			return ErrLiquibaseFinalization
		}
		if !(p.SourceKind == "image" && p.SourceImageID != "" && p.SourceStateID == "" || p.SourceKind == "state" && p.SourceStateID != "" && p.SourceImageID == "") {
			return ErrLiquibaseFinalization
		}
		result.Clones = append(result.Clones, item)
		return nil
	})
}

func readFailureSessions(ctx context.Context, tx *sql.Tx, result *LiquibaseFailureOwners) error {
	return scanFailureOwnerRows(ctx, tx, `SELECT session_id,job_id,clone_id,session_no,worker_generation,execution_fingerprint,start_kind,start_id,start_position,planned_runner_name,runner_config_digest,capture_helper_digest,protected_config_ref,coalesce(runner_id,''),runner_phase,db_phase,CASE WHEN cursor_state_id IS NOT NULL THEN 'state' WHEN cursor_image_id IS NOT NULL THEN 'image' ELSE '' END,coalesce(cursor_state_id,cursor_image_id,''),cursor_position,created_at,retired_at FROM prepare_liquibase_sessions WHERE job_id=? ORDER BY session_no,session_id`, result.Job.JobID, func(rows *sql.Rows) error {
		var item LiquibaseSessionRecord
		if err := rows.Scan(&item.SessionID, &item.JobID, &item.CloneID, &item.SessionNo, &item.WorkerGeneration, &item.ExecutionFingerprint, &item.StartKind, &item.StartID, &item.StartPosition, &item.PlannedRunnerName, &item.RunnerConfigDigest, &item.CaptureHelperDigest, &item.ProtectedConfigRef, &item.RunnerID, &item.RunnerPhase, &item.DBPhase, &item.CursorKind, &item.CursorID, &item.CursorPosition, &item.CreatedAt, &item.RetiredAt); err != nil {
			return err
		}
		if item.ExecutionFingerprint != result.Execution.ExecutionFingerprint || item.WorkerGeneration <= 0 || item.SessionNo <= 0 || item.RunnerID != "" && !dockerContainerID.MatchString(item.RunnerID) {
			return ErrLiquibaseFinalization
		}
		matched := false
		for _, clone := range result.Clones {
			if clone.Plan.ID == item.CloneID {
				matched = clone.Plan.SourceKind == item.StartKind && (item.StartKind == "image" && clone.Plan.SourceImageID == item.StartID || item.StartKind == "state" && clone.Plan.SourceStateID == item.StartID)
			}
		}
		if !matched {
			return ErrLiquibaseFinalization
		}
		result.Sessions = append(result.Sessions, item)
		return nil
	})
}

func readFailureAttempts(ctx context.Context, tx *sql.Tx, result *LiquibaseFailureOwners) error {
	return scanFailureOwnerRows(ctx, tx, `SELECT a.job_id,a.task_id,a.attempt_no,a.clone_id,a.execution_digest,a.worker_generation,coalesce(a.planned_executor_name,''),coalesce(a.executor_id,''),a.phase,a.created_at,coalesce(a.started_at,''),coalesce(a.finished_at,''),coalesce(a.resolution,''),a.legacy_unbound,a.executor_kind,b.execution_digest FROM prepare_step_attempts a LEFT JOIN prepare_step_bindings b ON b.job_id=a.job_id AND b.task_id=a.task_id WHERE a.job_id=? ORDER BY a.task_id,a.attempt_no`, result.Job.JobID, func(rows *sql.Rows) error {
		var item LiquibaseFailureAttempt
		p := &item.Record
		var bound sql.NullString
		var legacy int
		if err := rows.Scan(&p.JobID, &p.TaskID, &p.AttemptNo, &p.CloneID, &p.ExecutionDigest, &p.WorkerGeneration, &p.PlannedExecutorName, &p.ExecutorID, &p.Phase, &p.CreatedAt, &p.StartedAt, &p.FinishedAt, &p.Resolution, &legacy, &item.ExecutorKind, &bound); err != nil {
			return err
		}
		if legacy != 0 || !bound.Valid || bound.String != p.ExecutionDigest || p.AttemptNo <= 0 || p.WorkerGeneration < 0 || item.ExecutorKind != "container" && item.ExecutorKind != "session_exec" {
			return ErrLiquibaseFinalization
		}
		matched := false
		for _, clone := range result.Clones {
			matched = matched || clone.Plan.ID == p.CloneID
		}
		if !matched {
			return ErrLiquibaseFinalization
		}
		result.Attempts = append(result.Attempts, item)
		return nil
	})
}

func readFailureInvocations(ctx context.Context, tx *sql.Tx, result *LiquibaseFailureOwners) error {
	return scanFailureOwnerRows(ctx, tx, `SELECT job_id,task_id,attempt_no,session_id,invocation_ref,stdout_ref,stderr_ref,manifest_ref,helper_digest,protocol_revision,start_requested_at,complete_at,exit_code,completion_digest,completion_bytes,retired_at FROM prepare_liquibase_invocations WHERE job_id=? ORDER BY task_id,attempt_no`, result.Job.JobID, func(rows *sql.Rows) error {
		var item LiquibaseFailureInvocation
		p := &item.Record
		var exit, size sql.NullInt64
		var digest sql.NullString
		if err := rows.Scan(&p.Attempt.JobID, &p.Attempt.TaskID, &p.Attempt.AttemptNo, &p.SessionID, &p.InvocationRef, &p.StdoutRef, &p.StderrRef, &p.ManifestRef, &p.HelperDigest, &p.ProtocolRevision, &item.StartRequestedAt, &item.CompleteAt, &exit, &digest, &size, &item.RetiredAt); err != nil {
			return err
		}
		matched := false
		for _, attempt := range result.Attempts {
			if attempt.Record.TaskID == p.Attempt.TaskID && attempt.Record.AttemptNo == p.Attempt.AttemptNo && attempt.ExecutorKind == "session_exec" {
				p.Attempt = attempt.Record
				matched = true
			}
		}
		if !matched {
			return ErrLiquibaseFinalization
		}
		p.CreatedAt = p.Attempt.CreatedAt
		matched = false
		for _, session := range result.Sessions {
			matched = matched || session.SessionID == p.SessionID && session.CloneID == p.Attempt.CloneID && session.WorkerGeneration == p.Attempt.WorkerGeneration && session.CaptureHelperDigest == p.HelperDigest
		}
		if !matched || !validInvocationPlan(p.LiquibaseInvocationPlan) {
			return ErrLiquibaseFinalization
		}
		if item.CompleteAt != nil {
			if !exit.Valid || !size.Valid || !digest.Valid || exit.Int64 < 0 || exit.Int64 > 255 || size.Int64 < 0 || !bindingDigest.MatchString(digest.String) {
				return ErrLiquibaseFinalization
			}
			item.Completion = &LiquibaseInvocationCompletion{ExitCode: int(exit.Int64), ManifestDigest: digest.String, ManifestBytes: size.Int64}
		} else if exit.Valid || size.Valid || digest.Valid {
			return ErrLiquibaseFinalization
		}
		result.Invocations = append(result.Invocations, item)
		return nil
	})
}

func readFailureOperations(ctx context.Context, tx *sql.Tx, result *LiquibaseFailureOwners) error {
	return scanFailureOwnerRows(ctx, tx, `SELECT operation_ref,target_path,source_ref,identity_json,physical_identity,runtime_ref,stage FROM managed_runtime_operations WHERE `+jobOwnedOperationSQL("managed_runtime_operations", "?1")+` ORDER BY operation_ref`, result.Job.JobID, func(rows *sql.Rows) error {
		var item instanceaccess.Operation
		var raw string
		if err := rows.Scan(&item.Ref, &item.Target, &item.Source, &raw, &item.PhysicalIdentity, &item.RuntimeRef, &item.Stage); err != nil {
			return err
		}
		owned := filepath.Base(filepath.Dir(item.Target)) == result.Job.JobID
		for _, clone := range result.Clones {
			ref := filepath.ToSlash(clone.Plan.Ref)
			if !filepath.IsAbs(clone.Plan.Ref) && filepath.ToSlash(filepath.Clean(clone.Plan.Ref)) == ref && strings.HasPrefix(ref, "jobs/") && strings.HasSuffix(filepath.ToSlash(item.Target), "/"+ref) {
				owned = true
			}
		}
		if json.Unmarshal([]byte(raw), &item.Identity) != nil || item.Identity.Validate() != nil || item.Identity.LineageRef != result.Job.LineageRef || item.Identity.IdentityDigest != result.Job.IdentityDigest || item.PhysicalIdentity == "" || !filepath.IsAbs(item.Target) || filepath.Clean(item.Target) != item.Target || filepath.Base(item.Target) != item.Ref || !owned || filepath.Base(filepath.Dir(filepath.Dir(item.Target))) != "jobs" {
			return ErrLiquibaseFinalization
		}
		result.Operations = append(result.Operations, item)
		return nil
	})
}

func readFailureRestores(ctx context.Context, tx *sql.Tx, result *LiquibaseFailureOwners) error {
	return scanFailureOwnerRows(ctx, tx, `SELECT job_id,task_id,session_id,state_id,restore_phase FROM prepare_liquibase_cache_restores WHERE job_id=? ORDER BY task_id`, result.Job.JobID, func(rows *sql.Rows) error {
		var item LiquibaseCacheRestore
		if err := rows.Scan(&item.JobID, &item.TaskID, &item.SessionID, &item.StateID, &item.Phase); err != nil {
			return err
		}
		matched := false
		for _, session := range result.Sessions {
			matched = matched || session.SessionID == item.SessionID
		}
		if !matched || item.StateID == "" {
			return ErrLiquibaseFinalization
		}
		result.Restores = append(result.Restores, item)
		return nil
	})
}

// LiquibaseFailureOwners is one consistent read of the whole frozen owner set,
// including retired history. It is a locator snapshot, NOT a delete ticket or
// proof of native absence; each retirement writer must recheck exact authority.
// No mutable project file, image resolution or latest-attempt guess contributes.
// Requirements: checkpoint-capacity-recovery.md §28.2–3, JF05/JF08.
type LiquibaseFailureOwners struct {
	Intent      FinalizationSnapshot
	Job         JobRecord
	Execution   LiquibaseExecutionRecord
	Clones      []LiquibaseFailureClone
	Sessions    []LiquibaseSessionRecord
	Attempts    []LiquibaseFailureAttempt
	Invocations []LiquibaseFailureInvocation
	Operations  []instanceaccess.Operation
	Restores    []LiquibaseCacheRestore
	// Exact O5 ownership participates in complete before/after comparisons.
	// It does not independently grant a native cleanup or SQL permission.
	planningDB *planningDBOwner
}

// LiquibaseFailureClone preserves nullable DB attachment independently of a
// session: a persisted assigned clone can exist before first-session admission.
type LiquibaseFailureClone struct {
	Plan               ClonePlan
	Phase, DBRuntimeID string
}

// LiquibaseFailureAttempt retains the original executor model even for retired
// planning history; session-exec attempts do not invent container executor IDs.
type LiquibaseFailureAttempt struct {
	Record       AttemptRecord
	ExecutorKind string
}

// LiquibaseFailureInvocation never fabricates a completion to ease disposal.
// Unknown results and zero exits are preserved alongside their original refs.
type LiquibaseFailureInvocation struct {
	Record                                  LiquibaseInvocationRecord
	StartRequestedAt, CompleteAt, RetiredAt *string
	Completion                              *LiquibaseInvocationCompletion
}

// validateEarlyJobFailure uses one transaction/snapshot and refuses unreadable
// planning/binding authority. Unlike late finalization it intentionally permits
// live working resources; it grants no authority to remove their bytes.
func validateEarlyJobFailure(ctx context.Context, tx *sql.Tx, jobID string, outcome finalizationcodec.Outcome) error {
	if outcome.Kind != finalizationcodec.Failure || outcome.EventProfile != finalizationcodec.StatusError {
		return ErrLiquibaseFinalization
	}
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
WHERE j.job_id=? AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND j.status IN ('queued','running','recovery_required') AND j.finished_at IS NULL
AND j.result_json IS NULL AND j.cache_explain_result_json IS NULL
AND jb.requested_tag=lb.requested_tag AND jb.bundle_manifest_digest=lb.bundle_manifest_digest
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=j.job_id)
AND `+earlyJobFailureStepAgreementSQL("j.job_id", "lb"), jobID).Scan(&eligible); err != nil || eligible != 1 {
		return ErrLiquibaseFinalization
	}
	plan, found, err := loadPlanningResult(ctx, tx, jobID)
	if err != nil || !found {
		return ErrLiquibaseFinalization
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_planning_intents p
JOIN prepare_tasks t ON t.job_id=p.job_id AND t.task_id=p.task_id
JOIN prepare_step_attempts a ON a.job_id=p.job_id AND a.task_id='plan' AND a.attempt_no=p.result_attempt_no
JOIN prepare_clones c ON c.clone_id=p.preview_clone_id AND c.job_id=p.job_id
JOIN prepare_step_bindings b ON b.job_id=p.job_id AND b.task_id='plan'
JOIN prepare_execution_bindings e ON e.execution_digest=b.execution_digest
JOIN liquibase_job_bindings jb ON jb.job_id=p.job_id
WHERE p.job_id=? AND p.execution_digest=? AND p.result_attempt_no=?
AND t.type='plan' AND t.status='succeeded' AND (c.phase='released' OR (c.phase='active' AND EXISTS(
SELECT 1 FROM prepare_liquibase_planning_db_ownership owner WHERE owner.job_id=p.job_id AND owner.clone_id=c.clone_id
AND owner.disposed_at IS NULL AND owner.plan_digest=p.plan_digest
AND (owner.session_id IS NULL OR EXISTS(
SELECT 1 FROM prepare_liquibase_sessions s
JOIN prepare_tasks first ON first.job_id=s.job_id AND first.task_id=owner.first_task_id
JOIN prepare_step_bindings first_binding ON first_binding.job_id=first.job_id AND first_binding.task_id=first.task_id
WHERE s.session_id=owner.session_id AND s.job_id=p.job_id AND s.clone_id=c.clone_id
AND s.session_no=1 AND s.worker_generation=a.worker_generation
AND s.execution_fingerprint=jb.execution_fingerprint
AND s.start_kind=c.source_kind AND s.start_id=coalesce(c.source_state_id,c.source_image_id)
AND s.start_position=first.position AND first_binding.execution_digest=owner.first_execution_digest
AND owner.transferred_at IS NOT NULL AND owner.disposal_requested_evidence_id IS NULL)))))
AND a.clone_id=c.clone_id AND a.execution_digest=p.execution_digest
AND b.execution_digest=p.execution_digest AND e.prepare_kind='lb'
AND e.adapter_binding_digest=jb.execution_fingerprint AND e.input_manifest_digest=jb.bundle_manifest_digest
AND a.phase='closed' AND a.resolution='planning_result_saved'
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts pending WHERE pending.job_id=p.job_id AND pending.task_id='plan' AND (pending.phase<>'closed' OR pending.resolution IS NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_planning_attempt_artifacts x WHERE x.job_id=p.job_id AND (x.executor_retired_at IS NULL OR x.protected_input_retired_at IS NULL OR x.output_retired_at IS NULL OR x.stderr_retired_at IS NULL OR x.manifest_retired_at IS NULL))`, jobID, plan.ExecutionDigest, plan.AttemptNo).Scan(&eligible); err != nil || eligible != 1 {
		return ErrLiquibaseFinalization
	}
	return nil
}

// failureOperationMatchesClone uses the exact retained relative path, not an
// inferred job-name prefix. Absolute root/physical validation is native-owned.
func failureOperationMatchesClone(target, ref string, clone ClonePlan) bool {
	return filepath.Base(target) == ref && strings.HasSuffix(filepath.ToSlash(target), "/"+filepath.ToSlash(clone.Ref))
}

func retiredFailureCloneOperations(owners LiquibaseFailureOwners, clone ClonePlan) bool {
	for _, op := range owners.Operations {
		if failureOperationMatchesClone(op.Target, op.Ref, clone) && op.Stage != "retired" {
			return false
		}
	}
	return true
}

// privateDisposalPhaseState captures the complete ordered inventory as well as
// the exact private rows. No native callback is run with this borrower held.
// Requirements: private-capture ownership §5.1, SF04–SF06.
type privateDisposalPhaseState struct {
	capture   privateFailureCaptureState
	owners    LiquibaseFailureOwners
	authority *failurePhysicalAuthority
}

// validatePrivateDisposalBeforeNative closes its read transaction before any
// physical effect. Post-observation validation alone is too late to reject a
// damaged stop proof. The caller retains job/session exclusion across both.
func (s *SQLiteStore) validatePrivateDisposalBeforeNative(ctx context.Context, session LiquibaseSessionRecord, observed privateSessionDisposal) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	state, err := readPrivateDisposalPhaseState(ctx, tx, session)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(*state.capture.disposal, observed) {
		return ErrCommonAttempt
	}
	return tx.Rollback()
}

// readPrivateDisposalPhaseState shares association validation, not authority,
// between failure, cancellation and cache consumers. A selected failure uses its
// immutable full owner snapshot; other consumers retain their existing outcome.
func readPrivateDisposalPhaseState(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord) (privateDisposalPhaseState, error) {
	var state privateDisposalPhaseState
	var err error
	state.capture, err = readPrivateFailureCaptureState(ctx, tx, session)
	if err != nil {
		return state, err
	}
	if state.capture.disposal == nil {
		return state, ErrCommonAttempt
	}
	var exact int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+` AND runner_id=? AND runner_phase='stopped' AND retired_at IS NULL`, append(sessionIdentityArgs(session), session.RunnerID)...).Scan(&exact); err != nil || exact != 1 {
		return state, ErrCommonAttempt
	}
	intent, found, err := loadFinalization(ctx, tx, session.JobID)
	if err != nil {
		return state, err
	}
	if found && intent.Outcome.Kind == finalizationcodec.Failure {
		state.owners, _, err = selectedFailurePrivateOwnerTx(ctx, tx, intent, session)
		if err != nil {
			return state, err
		}
		basis, err := failurePhysicalAuthorityTx(ctx, tx, intent, session, state.owners, state.capture.disposal)
		if err != nil {
			return state, err
		}
		state.authority = &basis
		return state, nil
	}
	state.owners.Job.JobID = session.JobID
	state.owners.Execution.ExecutionFingerprint = session.ExecutionFingerprint
	for _, read := range []func(context.Context, *sql.Tx, *LiquibaseFailureOwners) error{
		readFailureClones, readFailureSessions, readFailureAttempts, readFailureInvocations, readFailureRestores,
	} {
		if err := read(ctx, tx, &state.owners); err != nil {
			return state, err
		}
	}
	return state, nil
}

// updatePrivateDisposalPhase holds the exact connection through settlement and
// complete readback. before/after are compared only after full reads finish;
// partial or contradictory evidence is uncertain, not a successful cleanup.
func (s *SQLiteStore) updatePrivateDisposalPhase(ctx context.Context, session LiquibaseSessionRecord, observed privateSessionDisposal, write func(context.Context, *sql.Tx, privateSessionDisposal) error) error {
	_, err := withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		before, err := readPrivateDisposalPhaseState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		if !reflect.DeepEqual(*before.capture.disposal, observed) {
			return struct{}{}, ErrCommonAttempt
		}
		if err := write(ctx, f.tx, observed); err != nil {
			return struct{}{}, err
		}
		wanted, err := readPrivateDisposalPhaseState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		if !reflect.DeepEqual(wanted.owners, before.owners) || !reflect.DeepEqual(wanted.authority, before.authority) {
			return struct{}{}, ErrCommonAttempt
		}
		return struct{}{}, f.commit(func(readCtx context.Context, tx *sql.Tx) (bool, error) {
			actual, err := readPrivateDisposalPhaseState(readCtx, tx, session)
			if err != nil {
				return false, err
			}
			if reflect.DeepEqual(actual, wanted) {
				return true, nil
			}
			if reflect.DeepEqual(actual, before) {
				return false, nil
			}
			return false, ErrCommonAttempt
		})
	})
	return err
}

// writePrivateSessionDisposalTx shares statements only. The caller owns commit,
// rollback and reconciliation; in particular an owned finalization connection
// must never reconcile its transaction through the pool. Ownership §5.1.
func writePrivateSessionDisposalTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, reason, authority, runner, database, at string) error {
	old, err := readPrivateSessionDisposal(ctx, tx, session)
	if err == sql.ErrNoRows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_session_disposals(session_id,reason,authority_ref,requested_at,runner_id,db_runtime_id) VALUES(?,?,?,?,?,?)`, session.SessionID, reason, authority, at, runner, database); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if old.reason != reason || old.authority != authority || old.runner != runner || old.database != database {
		return ErrCommonAttempt
	}
	// Preserve any earlier published-prune intent and all confirmed history.
	rows, err := tx.QueryContext(ctx, `SELECT slot.job_id,slot.task_id,slot.attempt_no FROM prepare_liquibase_capture_slots slot
WHERE slot.session_id=? AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_capture_retirements r WHERE r.session_id=slot.session_id AND r.job_id=slot.job_id AND r.task_id=slot.task_id AND r.attempt_no=slot.attempt_no)`, session.SessionID)
	if err != nil {
		return err
	}
	type target struct {
		job, task string
		attempt   int
	}
	var targets []target
	for rows.Next() {
		var v target
		if err := rows.Scan(&v.job, &v.task, &v.attempt); err != nil {
			_ = rows.Close()
			return err
		}
		targets = append(targets, v)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	for _, v := range targets {
		encoded, _ := json.Marshal([]any{"lb-capture-disposal-v1", session.SessionID, v.job, v.task, v.attempt})
		ref := "capture-disposal-" + strings.TrimPrefix(commonEvidenceDigest(string(encoded)), "sha256:")
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_capture_retirements(retirement_ref,job_id,task_id,attempt_no,session_id,reason,requested_at) VALUES(?,?,?,?,?,'session_disposal',?)`, ref, v.job, v.task, v.attempt, session.SessionID, at); err != nil {
			return err
		}
	}
	return nil
}

// RecordLiquibaseSessionDisposalDBAbsent confirms exact database absence before
// parent removal can be authorized. The existing clone verifier runs outside
// SQL; a lost write reply is reconciled read-only, never by removing SQL output.
// Requires an already saved disposal; does not mark the clone released.
func (s *SQLiteStore) RecordLiquibaseSessionDisposalDBAbsent(ctx context.Context, session LiquibaseSessionRecord, verifier CloneAbsenceVerifier, at string) error {
	if s == nil || s.db == nil || verifier == nil || !validFinalizationUTC(at) {
		return ErrCommonAttempt
	}
	d, err := readPrivateSessionDisposal(ctx, s.db, session)
	if err != nil {
		return err
	}
	if err := s.validatePrivateDisposalBeforeNative(ctx, session, d); err != nil {
		return err
	}
	clone, _, id, err := s.ReadLiquibaseSessionClone(ctx, session)
	if err != nil || id != d.database {
		return ErrCommonAttempt
	}
	if err := verifier.VerifyReleasedClone(ctx, clone, id); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.updatePrivateDisposalPhase(ctx, session, d, func(ctx context.Context, tx *sql.Tx, saved privateSessionDisposal) error {
		if saved.dbAbsent.Valid {
			return nil // Preserve the first committed observation time.
		}
		result, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_session_disposals SET db_absent_at=? WHERE session_id=? AND runner_id=? AND db_runtime_id=? AND db_absent_at IS NULL`, at, session.SessionID, saved.runner, saved.database)
		if err != nil {
			return err
		}
		return oneRowChanged(result)
	})
}

// RetireLiquibaseSessionPrivateCapture confirms the saved parent's absence
// only after durable DB absence, then confirms every outstanding capture and
// frees its occupied slot atomically. It preserves published-prune confirmations,
// invocation history and the unresolved SQL outcome for the final handoff.
// Native work is outside SQL; caller holds job/session exclusion. Schema §3.3.
func (s *SQLiteStore) RetireLiquibaseSessionPrivateCapture(ctx context.Context, session LiquibaseSessionRecord, verifier LiquibaseSessionAbsenceVerifier, at string) error {
	if s == nil || s.db == nil || verifier == nil || !validFinalizationUTC(at) {
		return ErrCommonAttempt
	}
	d, err := readPrivateSessionDisposal(ctx, s.db, session)
	if err != nil || !d.dbAbsent.Valid {
		return ErrCommonAttempt
	}
	if err := s.validatePrivateDisposalBeforeNative(ctx, session, d); err != nil {
		return err
	}
	clone, _, id, err := s.ReadLiquibaseSessionClone(ctx, session)
	if err != nil || id != d.database {
		return ErrCommonAttempt
	}
	if err := verifier.VerifyReleasedLiquibaseSession(ctx, session, clone, id); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.updatePrivateDisposalPhase(ctx, session, d, func(ctx context.Context, tx *sql.Tx, saved privateSessionDisposal) error {
		if !saved.runnerAbsent.Valid {
			if _, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_session_disposals SET runner_absent_at=? WHERE session_id=? AND runner_absent_at IS NULL`, at, session.SessionID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_capture_retirements SET verified_at=?,absence_kind='container_absent' WHERE session_id=? AND verified_at IS NULL`, at, session.SessionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM prepare_liquibase_capture_slots WHERE session_id=?`, session.SessionID); err != nil {
			return err
		}
		var missing int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_invocations i WHERE i.session_id=? AND NOT EXISTS
(SELECT 1 FROM prepare_liquibase_capture_retirements r WHERE r.session_id=i.session_id AND r.job_id=i.job_id AND r.task_id=i.task_id AND r.attempt_no=i.attempt_no AND r.verified_at IS NOT NULL)`, session.SessionID).Scan(&missing); err != nil || missing != 0 {
			return ErrCommonAttempt
		}
		return nil
	})
}

// publishedCaptureDisposalTarget validates the complete normal-completion
// authority even on repeated calls: a saved intent cannot hide a newly accepted
// cancellation or an unfinished suffix. Private-capture schema §3.4, MP06.
func publishedCaptureDisposalTarget(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord) (string, string, error) {
	var state, database string
	err := tx.QueryRowContext(ctx, `SELECT s.cursor_state_id,c.db_runtime_id
FROM prepare_liquibase_sessions s JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_jobs j ON j.job_id=s.job_id
JOIN states state ON state.state_id=s.cursor_state_id AND state.prepare_kind='lb'
JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id AND lb.execution_fingerprint=s.execution_fingerprint
WHERE s.session_id IN (SELECT session_id FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`)
AND s.runner_id=? AND s.runner_phase='stopped' AND s.retired_at IS NULL AND c.phase='quiescent'
AND s.capture_location='container_private' AND s.capture_protocol_revision='lb-step-capture-v2'
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0 AND j.status IN ('running','recovery_required')
AND NOT EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=s.job_id AND t.type='state_execute' AND t.status<>'succeeded')
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 WHERE i.session_id=s.session_id AND (a.phase<>'closed' OR a.resolution IS NOT 'published'))
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores restore WHERE restore.session_id=s.session_id AND restore.restore_phase NOT IN ('published','abandoned'))
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations cancel WHERE cancel.job_id=s.job_id)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=s.job_id AND f.outcome_kind IN ('failure','cancelled'))`,
		append(sessionIdentityArgs(session), session.RunnerID)...).Scan(&state, &database)
	if err != nil {
		return "", "", err
	}
	if !dockerContainerID.MatchString(database) {
		return "", "", ErrCommonAttempt
	}
	return state, database, nil
}

// SaveLiquibaseSessionPublishedDisposal records the already specified
// final_published authority before physical cleanup. It grants neither absence,
// terminal success nor SQL replay. An owned connection settles COMMIT errors
// before comparing the complete old/new private-capture inventory. MP06.
func (s *SQLiteStore) SaveLiquibaseSessionPublishedDisposal(ctx context.Context, session LiquibaseSessionRecord, at string) error {
	if s == nil || s.db == nil || !validFinalizationUTC(at) || !dockerContainerID.MatchString(session.RunnerID) {
		return ErrCommonAttempt
	}
	_, err := withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		state, database, err := publishedCaptureDisposalTarget(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		before, err := readPrivateFailureCaptureState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		if err := writePrivateSessionDisposalTx(ctx, f.tx, session, "final_published", state, session.RunnerID, database, at); err != nil {
			return struct{}{}, err
		}
		wanted, err := readPrivateFailureCaptureState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, f.commit(func(readCtx context.Context, tx *sql.Tx) (bool, error) {
			actualState, actualDatabase, err := publishedCaptureDisposalTarget(readCtx, tx, session)
			if err != nil {
				return false, err
			}
			if actualState != state || actualDatabase != database {
				return false, ErrCommonAttempt
			}
			actual, err := readPrivateFailureCaptureState(readCtx, tx, session)
			if err != nil {
				return false, err
			}
			if reflect.DeepEqual(actual, wanted) {
				return true, nil
			}
			if reflect.DeepEqual(actual, before) {
				return false, nil
			}
			return false, ErrCommonAttempt
		})
	})
	return err
}

// failureNonstartPhysicalAuthorityTx preserves an exact same-attempt nonstart
// anchor. The original immutable scope, observation digest and time must all
// match the checksummed evidence; a planned child without start permission is
// not a fabricated completed child or permission to allocate a fresh attempt.
// Requirements: private-capture storage §3.4, ownership §5.1, SF03/SF05.
func failureNonstartPhysicalAuthorityTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, owners LiquibaseFailureOwners, existing privateSessionDisposal) (failurePhysicalAuthority, error) {
	if existing.nonstartBasis.String != "attempt" {
		return failurePhysicalAuthority{}, ErrLiquibaseFinalization
	}
	anchor, err := strconv.ParseInt(existing.authority, 10, 64)
	if err != nil || anchor < 1 || strconv.FormatInt(anchor, 10) != existing.authority {
		return failurePhysicalAuthority{}, ErrLiquibaseFinalization
	}
	var task, scope, observation, observed string
	var attempt int
	if err := tx.QueryRowContext(ctx, `SELECT task_id,attempt_no,scope_digest,facts_digest,observed_at FROM prepare_attempt_evidence WHERE evidence_id=? AND job_id=? AND worker_generation=? AND kind='never_started'`, anchor, session.JobID, session.WorkerGeneration).Scan(&task, &attempt, &scope, &observation, &observed); err != nil {
		return failurePhysicalAuthority{}, failureOwnerReadError(ctx)
	}
	if scope != existing.nonstartScope.String || observation != existing.nonstartObservation.String || observed != existing.nonstartObserved.String {
		return failurePhysicalAuthority{}, ErrLiquibaseFinalization
	}
	for _, child := range owners.Invocations {
		i := child.Record
		if i.SessionID != session.SessionID || i.Attempt.TaskID != task || i.Attempt.AttemptNo != attempt {
			continue
		}
		if child.StartRequestedAt != nil || child.Completion != nil || child.CompleteAt != nil ||
			i.Attempt.Phase != "planned" && !(i.Attempt.Phase == "closed" && i.Attempt.Resolution == "never_started") {
			return failurePhysicalAuthority{}, ErrLiquibaseFinalization
		}
		if _, err := readSelectedFailureEvidence(ctx, tx, i.Attempt, anchor); err != nil {
			return failurePhysicalAuthority{}, err
		}
		if _, err := readSelectedFailureStopLinks(ctx, tx, session, i.Attempt); err != nil {
			return failurePhysicalAuthority{}, err
		}
		evidence, err := readSelectedFailureTailEvidence(ctx, tx, session)
		if err != nil {
			return failurePhysicalAuthority{}, err
		}
		return failurePhysicalAuthority{reason: existing.reason, reference: existing.authority, fencingEvidence: evidence}, nil
	}
	return failurePhysicalAuthority{}, ErrLiquibaseFinalization
}

// failurePhysicalAuthority separates a retained physical reason from the
// current immutable job failure. Typed cache provenance participates in complete
// readback without releasing its selection pin. Ownership §5.1, SF03/SF05.
type failurePhysicalAuthority struct {
	reason, reference string
	cache             *LiquibaseCacheSelection
	stateProvenance   string
	fencingEvidence   []string
	policy            string
}

// failurePhysicalAuthorityTx revalidates the original reason without rewriting
// it. A reason is not authority by itself: an exact guarded typed source is
// mandatory. Unimplemented/incompatible histories fail closed, never turn into
// a fresh selected_failure row or grant replay permission.
func failurePhysicalAuthorityTx(ctx context.Context, tx *sql.Tx, intent FinalizationSnapshot, session LiquibaseSessionRecord, owners LiquibaseFailureOwners, existing *privateSessionDisposal) (failurePhysicalAuthority, error) {
	want := failurePhysicalAuthority{reason: "selected_failure", reference: intent.Digest}
	if existing == nil {
		return want, nil
	}
	if existing.runner != session.RunnerID {
		return failurePhysicalAuthority{}, ErrLiquibaseFinalization
	}
	switch existing.reason {
	case "selected_failure":
		if existing.authority != intent.Digest {
			return failurePhysicalAuthority{}, ErrLiquibaseFinalization
		}
		return want, nil
	case "cache_selected":
		return failureCachePhysicalAuthorityTx(ctx, tx, session, owners, *existing)
	case "final_published":
		return failurePublishedPhysicalAuthorityTx(ctx, tx, session, *existing)
	case "unknown_discard":
		return failureUnknownPhysicalAuthorityTx(ctx, tx, session, owners, *existing)
	case "proven_nonstart":
		return failureNonstartPhysicalAuthorityTx(ctx, tx, session, owners, *existing)
	default:
		return failurePhysicalAuthority{}, ErrLiquibaseFinalization
	}
}

// failureCachePhysicalAuthorityTx validates a still-owned original selection.
// Completed history is handled by the already-retired logical path, which does
// not require historical target bytes to exist forever. An active predecessor
// with an already discharged selection is contradictory, not cleanup authority.
func failureCachePhysicalAuthorityTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, owners LiquibaseFailureOwners, existing privateSessionDisposal) (failurePhysicalAuthority, error) {
	for _, child := range owners.Invocations {
		i := child.Record
		if i.SessionID != session.SessionID {
			continue
		}
		key, _ := json.Marshal([]any{i.Attempt.JobID, i.Attempt.TaskID, i.Attempt.AttemptNo})
		if string(key) != existing.authority {
			continue
		}
		selection, found, err := loadLiquibaseCacheTarget(ctx, tx, i.Attempt.JobID, i.Attempt.TaskID, i.Attempt.AttemptNo)
		if err != nil || !found || selection.SessionID != session.SessionID || selection.CloneID != session.CloneID || selection.WorkerGeneration != session.WorkerGeneration || selection.ExecutionDigest != i.Attempt.ExecutionDigest || selection.TransferredAt != nil || selection.ReleasedAt != nil {
			return failurePhysicalAuthority{}, ErrLiquibaseFinalization
		}
		if err := verifyCacheSelectionInvocation(ctx, tx, session, i); err != nil {
			return failurePhysicalAuthority{}, err
		}
		if err := verifyLiquibaseCacheSourceTx(ctx, tx, session, i.Attempt.TaskID, selection.TargetStateID); err != nil {
			return failurePhysicalAuthority{}, err
		}
		// Capture the full immutable target/provenance rows for comparison after
		// a lost COMMIT reply, not just the target's ID or a positive row count.
		provenance, err := readFailureDisposalStateProvenance(ctx, tx, selection.TargetStateID)
		if err != nil {
			return failurePhysicalAuthority{}, err
		}
		return failurePhysicalAuthority{reason: existing.reason, reference: existing.authority, cache: &selection, stateProvenance: provenance}, nil
	}
	return failurePhysicalAuthority{}, ErrLiquibaseFinalization
}

// failurePublishedPhysicalAuthorityTx proves the original publication rather
// than treating its old physical reason as job success. The current failure is
// intentionally not part of the original publication's exclusion predicates.
func failurePublishedPhysicalAuthorityTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, existing privateSessionDisposal) (failurePhysicalAuthority, error) {
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions s
JOIN prepare_jobs j ON j.job_id=s.job_id
JOIN states state ON state.state_id=s.cursor_state_id
JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id
JOIN prepare_state_bindings binding ON binding.state_id=state.state_id
WHERE s.session_id=? AND s.cursor_state_id=? AND s.retired_at IS NULL
AND state.state_fingerprint=state.state_id AND state.prepare_kind='lb'
AND state.image_id=coalesce(nullif(j.resolved_image_id,''),j.image_id)
AND coalesce(state.lineage_ref,'')=coalesce(j.lineage_ref,'') AND coalesce(state.identity_digest,'')=coalesce(j.identity_digest,'')
AND lb.execution_fingerprint=s.execution_fingerprint
AND NOT EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=j.job_id AND t.type='state_execute' AND t.status<>'succeeded')
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 WHERE i.session_id=s.session_id AND (a.phase<>'closed' OR a.resolution IS NOT 'published' OR i.complete_at IS NULL OR i.exit_code IS NOT 0))
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=s.session_id AND r.restore_phase NOT IN ('published','abandoned'))
AND EXISTS (SELECT 1 FROM prepare_tasks t JOIN prepare_step_bindings step USING(job_id,task_id)
 WHERE t.job_id=s.job_id AND t.type='state_execute' AND t.status='succeeded' AND t.output_state_id=state.state_id
 AND t.position=s.cursor_position-1 AND step.execution_digest=binding.execution_digest
 AND ((t.input_kind='state' AND state.parent_state_id=t.input_id) OR (t.input_kind='image' AND state.parent_state_id IS NULL)))`, session.SessionID, existing.authority).Scan(&eligible); err != nil || eligible != 1 {
		return failurePhysicalAuthority{}, ErrLiquibaseFinalization
	}
	provenance, err := readFailureDisposalStateProvenance(ctx, tx, existing.authority)
	if err != nil {
		return failurePhysicalAuthority{}, err
	}
	return failurePhysicalAuthority{reason: existing.reason, reference: existing.authority, stateProvenance: provenance}, nil
}

// readFailureDisposalStateProvenance retains every immutable target field used
// in cleanup reconciliation. It reads metadata only, not removed capture bytes.
func readFailureDisposalStateProvenance(ctx context.Context, tx *sql.Tx, stateID string) (string, error) {
	var provenance string
	if err := tx.QueryRowContext(ctx, `SELECT json_array(state.state_id,state.state_fingerprint,state.parent_state_id,state.image_id,state.prepare_kind,state.prepare_args_normalized,state.created_at,state.lineage_ref,state.identity_digest,lb.execution_fingerprint,b.execution_digest)
FROM states state JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id
JOIN prepare_state_bindings b ON b.state_id=state.state_id WHERE state.state_id=?`, stateID).Scan(&provenance); err != nil {
		return "", failureOwnerReadError(ctx)
	}
	return provenance, nil
}

// selectedFailurePrivateOwnerTx validates the complete inventory, exact target
// and every unresolved child's checksummed stop chain on the owned connection.
// A no-invocation owner needs no fabricated attempt or stop-evidence row.
func selectedFailurePrivateOwnerTx(ctx context.Context, tx *sql.Tx, intent FinalizationSnapshot, expected LiquibaseSessionRecord) (LiquibaseFailureOwners, LiquibaseFailureClone, error) {
	owners, err := loadLiquibaseFailureOwnersTx(ctx, tx, intent)
	if err != nil {
		return owners, LiquibaseFailureClone{}, err
	}
	session, clone, children, retired, err := stoppedFailureSessionState(owners, expected)
	if err != nil || retired {
		return owners, clone, ErrLiquibaseFinalization
	}
	for _, child := range children {
		if child.Record.Attempt.Resolution == "" {
			if _, err := readSelectedFailureStopLinks(ctx, tx, session, child.Record.Attempt); err != nil {
				return owners, clone, err
			}
		} else if child.Record.Attempt.Resolution == "discarded_clone" {
			if err := verifySelectedFailureDisposalTx(ctx, tx, child.Record.Attempt); err != nil {
				return owners, clone, err
			}
		}
	}
	return owners, clone, nil
}

// failureUnknownPhysicalAuthorityTx revalidates the original pre-disposal stop
// anchor and pinned policy. It deliberately requires neither a future DB-absence
// fact nor a final discarded_clone resolution, and grants no fresh-copy retry.
// Requirements: private-capture storage §3.4 and ownership §5.1, SF03/SF05.
func failureUnknownPhysicalAuthorityTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, owners LiquibaseFailureOwners, existing privateSessionDisposal) (failurePhysicalAuthority, error) {
	anchor, err := strconv.ParseInt(existing.authority, 10, 64)
	if err != nil || anchor < 1 || strconv.FormatInt(anchor, 10) != existing.authority {
		return failurePhysicalAuthority{}, ErrLiquibaseFinalization
	}
	var task string
	var attempt int
	if err := tx.QueryRowContext(ctx, `SELECT task_id,attempt_no FROM prepare_attempt_evidence WHERE evidence_id=? AND job_id=? AND worker_generation=? AND kind='executor_inspection'`, anchor, session.JobID, session.WorkerGeneration).Scan(&task, &attempt); err != nil {
		return failurePhysicalAuthority{}, failureOwnerReadError(ctx)
	}
	for _, child := range owners.Invocations {
		i := child.Record
		if i.SessionID != session.SessionID || i.Attempt.TaskID != task || i.Attempt.AttemptNo != attempt {
			continue
		}
		links, err := readSelectedFailureStopLinks(ctx, tx, session, i.Attempt)
		if err != nil || links.stopped != anchor {
			return failurePhysicalAuthority{}, ErrLiquibaseFinalization
		}
		var policy string
		if err := tx.QueryRowContext(ctx, `SELECT json_array(b.execution_digest,b.prepare_kind,b.command_digest,b.input_manifest_digest,b.executor_image_digest,b.target_identity_digest,b.policy_revision,b.effect_scope,b.partial_prefix_safe,b.enforcement_digest,b.adapter_binding_digest)
FROM prepare_execution_bindings b JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=?
WHERE b.execution_digest=? AND b.prepare_kind='lb' AND b.policy_revision='lb-container-v2'
AND b.effect_scope='unknown' AND b.enforcement_digest IS NULL
AND b.adapter_binding_digest=lb.execution_fingerprint AND b.policy_revision=lb.execution_policy_revision
AND b.input_manifest_digest=lb.bundle_manifest_digest AND b.target_identity_digest=lb.target_identity_digest
AND b.executor_image_digest=lb.image_manifest_digest`, session.ExecutionFingerprint, i.Attempt.ExecutionDigest).Scan(&policy); err != nil {
			return failurePhysicalAuthority{}, failureOwnerReadError(ctx)
		}
		evidence, err := readSelectedFailureTailEvidence(ctx, tx, session)
		if err != nil {
			return failurePhysicalAuthority{}, err
		}
		return failurePhysicalAuthority{reason: existing.reason, reference: existing.authority, fencingEvidence: evidence, policy: policy}, nil
	}
	return failurePhysicalAuthority{}, ErrLiquibaseFinalization
}

type sessionStopProofReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ReadLiquibaseSessionClone returns the durable DB locator used to rebuild a
// process-local handle. It never derives ownership from a Docker name alone.
func (s *SQLiteStore) ReadLiquibaseSessionClone(ctx context.Context, session LiquibaseSessionRecord) (ClonePlan, string, string, error) {
	if s == nil || s.db == nil {
		return ClonePlan{}, "", "", ErrCommonAttempt
	}
	var clone ClonePlan
	var image, state, runtimeID sql.NullString
	var phase string
	err := s.db.QueryRowContext(ctx, `SELECT c.clone_id,c.clone_generation,c.source_kind,c.source_image_id,
c.source_state_id,c.clone_ref,c.planned_db_runtime_name,c.phase,c.db_runtime_id
FROM prepare_clones c WHERE c.clone_id=? AND c.job_id=?
AND EXISTS (SELECT 1 FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`)`,
		append([]any{session.CloneID, session.JobID}, sessionIdentityArgs(session)...)...).Scan(&clone.ID, &clone.Generation,
		&clone.SourceKind, &image, &state, &clone.Ref, &clone.PlannedRuntimeName, &phase, &runtimeID)
	if err != nil {
		return ClonePlan{}, "", "", err
	}
	clone.SourceImageID, clone.SourceStateID = image.String, state.String
	return clone, phase, runtimeID.String, nil
}

// LiquibaseCacheRestore records a protected immutable source, not a command
// invocation. A requested restore is never silently repeated after restart.
type LiquibaseCacheRestore struct{ JobID, TaskID, SessionID, StateID, Phase string }

// verifyLiquibaseCacheSourceTx verifies provenance only, not permission to
// restore over a writer. Restore requires no unresolved attempts; approved
// C03 disposal separately requires fencing and exact physical retirement.
func verifyLiquibaseCacheSourceTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, taskID, stateID string) error {
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND runner_phase IN ('running','stopped') AND retired_at IS NULL
AND NOT EXISTS (`+cancelledSessionSuffixPredicate+`)
AND EXISTS (SELECT 1 FROM prepare_tasks t JOIN prepare_jobs j ON j.job_id=t.job_id
JOIN states state ON state.state_id=t.output_state_id
JOIN prepare_step_bindings step ON step.job_id=t.job_id AND step.task_id=t.task_id
JOIN prepare_state_bindings binding ON binding.state_id=state.state_id AND binding.execution_digest=step.execution_digest
JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id
WHERE t.job_id=prepare_liquibase_sessions.job_id AND t.task_id=? AND state.state_id=?
AND t.type='state_execute' AND t.status IN ('running','recovery_required','succeeded')
AND t.position=prepare_liquibase_sessions.cursor_position AND state.state_fingerprint=state.state_id
AND state.prepare_kind='lb' AND state.image_id=coalesce(nullif(j.resolved_image_id,''),j.image_id)
AND j.status IN ('running','recovery_required')
AND coalesce(state.lineage_ref,'')=coalesce(j.lineage_ref,'') AND coalesce(state.identity_digest,'')=coalesce(j.identity_digest,'')
AND lb.execution_fingerprint=prepare_liquibase_sessions.execution_fingerprint
AND ((t.input_kind='state' AND t.input_id=prepare_liquibase_sessions.cursor_state_id AND state.parent_state_id=t.input_id)
OR (t.input_kind='image' AND t.input_id=prepare_liquibase_sessions.cursor_image_id AND state.parent_state_id IS NULL)))`,
		append(sessionIdentityArgs(session), session.RunnerID, taskID, stateID)...).Scan(&eligible); err != nil || eligible != 1 {
		return ErrCommonAttempt
	}
	return nil
}
