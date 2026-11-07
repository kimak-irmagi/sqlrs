package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// artifactLeaseTable selects the sole installed lease authority. The common
// migration removes the v1 table in the same transaction as its row copy;
// normal job admission never writes both namespaces.
func artifactLeaseTable(ctx context.Context, tx *sql.Tx) (string, error) {
	var legacy, common int
	if err := tx.QueryRowContext(ctx, `SELECT
count(*) FILTER (WHERE name='liquibase_artifact_leases'),
count(*) FILTER (WHERE name='prepare_artifact_leases')
FROM sqlite_master WHERE type='table' AND name IN
('liquibase_artifact_leases','prepare_artifact_leases')`).Scan(&legacy, &common); err != nil {
		return "", err
	}
	if legacy == 1 && common == 0 {
		return "liquibase_artifact_leases", nil
	}
	if common == 1 && legacy == 0 {
		return "prepare_artifact_leases", nil
	}
	return "", ErrLiquibaseBinding
}

// commonEvidenceDigest is the canonical sha256 encoding used by the common
// attempt store for both scope and immutable observation contents.
func commonEvidenceDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ExecutionBinding is the immutable, fully resolved step identity. Digests
// refer to retained artifacts, not mutable project paths or image tags.
type ExecutionBinding struct {
	Digest, PrepareKind, CommandDigest, InputManifestDigest string
	ExecutorImageDigest, TargetIdentityDigest               string
	PolicyRevision, EffectScope, EnforcementDigest          string
	AdapterBindingDigest                                    string
	PartialPrefixSafe                                       bool
}

// ClonePlan names the exact original clone before any filesystem or Docker
// action. A later replay uses a separate, decision-authorized clone.
type ClonePlan struct {
	ID, Ref, PlannedRuntimeName string
	Generation                  int
	SourceKind, SourceImageID   string
	SourceStateID               string
}

func verifyExecutionBinding(ctx context.Context, tx *sql.Tx, want ExecutionBinding, partial int) error {
	var got ExecutionBinding
	var gotPartial int
	var enforcement, adapter sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT prepare_kind,command_digest,input_manifest_digest,
executor_image_digest,target_identity_digest,policy_revision,effect_scope,
partial_prefix_safe,enforcement_digest,adapter_binding_digest
FROM prepare_execution_bindings WHERE execution_digest=?`, want.Digest).Scan(
		&got.PrepareKind, &got.CommandDigest, &got.InputManifestDigest,
		&got.ExecutorImageDigest, &got.TargetIdentityDigest,
		&got.PolicyRevision, &got.EffectScope, &gotPartial, &enforcement, &adapter)
	if err != nil {
		return err
	}
	got.Digest = want.Digest
	got.EnforcementDigest, got.AdapterBindingDigest = enforcement.String, adapter.String
	got.PartialPrefixSafe = gotPartial == 1
	if got != want || gotPartial != partial {
		return fmt.Errorf("%w: execution binding differs", ErrCommonAttempt)
	}
	return nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// BindStep persists resolved command, input, image, and target identity before
// a cache lookup. The exact same binding is accepted on recovery; a changed
// declaration under a saved digest is rejected. A cache hit needs no clone or
// executor. See section 3 of the recovery storage schema.
func (s *SQLiteStore) BindStep(ctx context.Context, jobID, taskID string, binding ExecutionBinding, at string) error {
	if s == nil || s.db == nil || jobID == "" || taskID == "" || at == "" ||
		binding.Digest == "" || binding.PrepareKind == "" || binding.PolicyRevision == "" {
		return ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j JOIN prepare_tasks t
ON t.job_id=j.job_id WHERE j.job_id=? AND j.prepare_kind=?
AND j.status='running' AND t.task_id=? AND t.type='state_execute'
AND t.status IN ('queued','running')`, jobID, binding.PrepareKind, taskID).Scan(&eligible); err != nil || eligible != 1 {
		return ErrCommonAttempt
	}
	partial := 0
	if binding.PartialPrefixSafe {
		partial = 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_execution_bindings(
execution_digest,prepare_kind,command_digest,input_manifest_digest,
executor_image_digest,target_identity_digest,policy_revision,effect_scope,
partial_prefix_safe,enforcement_digest,adapter_binding_digest,created_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(execution_digest) DO NOTHING`,
		binding.Digest, binding.PrepareKind, binding.CommandDigest,
		binding.InputManifestDigest, binding.ExecutorImageDigest,
		binding.TargetIdentityDigest, binding.PolicyRevision,
		binding.EffectScope, partial, nullIfEmpty(binding.EnforcementDigest),
		nullIfEmpty(binding.AdapterBindingDigest), at); err != nil {
		return err
	}
	if err := verifyExecutionBinding(ctx, tx, binding, partial); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_step_bindings(job_id,task_id,execution_digest,bound_at)
VALUES(?,?,?,?) ON CONFLICT(job_id,task_id) DO NOTHING`, jobID, taskID, binding.Digest, at); err != nil {
		return err
	}
	var saved string
	if err := tx.QueryRowContext(ctx, `SELECT execution_digest FROM prepare_step_bindings
WHERE job_id=? AND task_id=?`, jobID, taskID).Scan(&saved); err != nil || saved != binding.Digest {
		return ErrCommonAttempt
	}
	return tx.Commit()
}

func commonAttemptName(jobID, taskID string, attemptNo int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("common-attempt-v1\n%s\n%s\n%d", jobID, taskID, attemptNo)))
	return "sqlrs-step-" + hex.EncodeToString(sum[:20])
}

var dockerContainerID = regexp.MustCompile(`^[0-9a-f]{64}$`)

var (
	// ErrLiquibaseFinalization rejects unavailable or corrupt private authority.
	ErrLiquibaseFinalization = errors.New("invalid Liquibase finalization journal")
	// ErrJobCancellationRejected is a definite non-admission, never permission
	// for the caller to signal an executor or cancel the finalization context.
	ErrJobCancellationRejected = errors.New("Liquibase job cancellation is not eligible")
	// ErrJobCancellationIndeterminate means commit failed and its durable outcome
	// could not be read. Callers must not pretend either acceptance or rejection.
	ErrJobCancellationIndeterminate = errors.New("Liquibase cancellation commit outcome is unknown")
)

type cancellationRowReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// validFinalizationUTC matches the storage component's Z/+00:00 nanosecond
// contract. Preserve accepted bytes; retries must not normalize/rewrite time.
func validFinalizationUTC(value string) bool {
	if !strings.HasSuffix(value, "Z") && !strings.HasSuffix(value, "+00:00") {
		return false
	}
	normalized := value
	if strings.HasSuffix(normalized, "+00:00") {
		normalized = strings.TrimSuffix(normalized, "+00:00") + "Z"
	}
	if len(normalized) < 20 || len(normalized) > 30 {
		return false
	}
	// time.Parse deliberately accepts a comma fraction and a one-digit hour.
	// The storage contract does not: use exactly the same lexical shape before
	// asking time.Parse to validate the calendar. Never normalize stored bytes.
	if normalized[10] != 'T' || normalized[13] != ':' || normalized[16] != ':' {
		return false
	}
	if len(normalized) != 20 {
		if len(normalized) < 22 || normalized[19] != '.' {
			return false
		}
		for _, digit := range normalized[20 : len(normalized)-1] {
			if digit < '0' || digit > '9' {
				return false
			}
		}
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

// ErrFinalizationIndeterminate requires recovery/readback before any cleanup,
// alternate outcome or runtime disposal. A missing reply is not a rollback.
var ErrFinalizationIndeterminate = errors.New("Liquibase finalization commit outcome is unknown")

// ObserveAttemptExit atomically records a verified Docker exit and moves the
// attempt to observed_stopped. Exit code zero or nonzero never resolves SQL
// effects; database quiescence and policy evaluation remain separate steps.
// See A03/Q01 in the approved recovery tests.
func (s *SQLiteStore) ObserveAttemptExit(ctx context.Context, attempt AttemptRecord, generation int, executorID string, exitCode int, observedAt string) (int64, error) {
	if s == nil || s.db == nil || !dockerContainerID.MatchString(executorID) ||
		exitCode < 0 || exitCode > 255 || observedAt == "" {
		return 0, ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE prepare_step_attempts
SET phase='observed_stopped',finished_at=?
WHERE job_id=? AND task_id=? AND attempt_no=? AND clone_id=?
AND execution_digest=? AND planned_executor_name=? AND executor_id=?
AND worker_generation=? AND legacy_unbound=0 AND phase='start_requested'
AND resolution IS NULL`, observedAt, attempt.JobID, attempt.TaskID,
		attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest,
		attempt.PlannedExecutorName, executorID, generation)
	if err != nil || oneRowChanged(result) != nil {
		return 0, ErrCommonAttempt
	}
	observation, err := json.Marshal(struct {
		ExecutorID string `json:"executor_id"`
		ExitCode   int    `json:"exit_code"`
	}{executorID, exitCode})
	if err != nil {
		return 0, err
	}
	id, err := appendAttemptEvidenceTx(ctx, tx, attempt, generation,
		"executor_inspection", observation, observedAt)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// appendAttemptEvidenceTx is the shared write primitive for observations that
// must commit with a phase transition and independent read-only observations.
func appendAttemptEvidenceTx(ctx context.Context, tx *sql.Tx, attempt AttemptRecord, generation int, kind string, observation json.RawMessage, observedAt string) (int64, error) {
	if attempt.JobID == "" || attempt.TaskID == "" ||
		attempt.AttemptNo <= 0 || attempt.CloneID == "" || attempt.ExecutionDigest == "" ||
		attempt.PlannedExecutorName == "" || attempt.LegacyUnbound || generation < 0 || observedAt == "" {
		return 0, ErrCommonAttempt
	}
	switch kind {
	case "executor_inspection", "never_started", "db_quiescence", "probe", "publication", "planning_output", "termination_requested", "termination_acknowledged", cancelledPlanningEvidence:
	default:
		return 0, ErrCommonAttempt
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(observation, &fields) != nil || fields == nil {
		return 0, ErrCommonAttempt
	}
	facts, err := json.Marshal(struct {
		CloneID         string          `json:"clone_id"`
		ExecutionDigest string          `json:"execution_digest"`
		Observation     json.RawMessage `json:"observation"`
	}{attempt.CloneID, attempt.ExecutionDigest, observation})
	if err != nil || len(facts) > 8192 {
		return 0, ErrCommonAttempt
	}
	scope := commonEvidenceDigest(fmt.Sprintf("attempt-scope-v1\n%s\n%s\n%d\n%s\n%s", attempt.JobID,
		attempt.TaskID, attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest))
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_step_attempts
WHERE job_id=? AND task_id=? AND attempt_no=? AND clone_id=?
AND execution_digest=? AND planned_executor_name=? AND worker_generation=?
AND legacy_unbound=0`, attempt.JobID, attempt.TaskID, attempt.AttemptNo,
		attempt.CloneID, attempt.ExecutionDigest, attempt.PlannedExecutorName,
		generation).Scan(&eligible); err != nil || eligible != 1 {
		return 0, ErrCommonAttempt
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO prepare_attempt_evidence(
job_id,task_id,attempt_no,kind,scope_digest,facts_json,facts_digest,
worker_generation,observed_at) VALUES(?,?,?,?,?,?,?,?,?)`, attempt.JobID,
		attempt.TaskID, attempt.AttemptNo, kind, scope, string(facts),
		commonEvidenceDigest(string(facts)), generation, observedAt)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil || id < 1 {
		return 0, ErrCommonAttempt
	}
	return id, nil
}

// ArtifactLease keeps a bound input or driver alive for a recoverable job.
type ArtifactLease struct {
	ID, Kind, Digest string
}

// BoundLaunch contains all authoritative rows needed before Docker create.
type BoundLaunch struct {
	Attempt AttemptPlan
	Binding ExecutionBinding
	Clone   ClonePlan
	Leases  []ArtifactLease
}

// BoundAttempt is the persisted launch record used for recovery. It contains
// no mutable source paths or raw credentials.
type BoundAttempt struct {
	Attempt                 AttemptRecord
	Binding                 ExecutionBinding
	Clone                   ClonePlan
	ClonePhase, DBRuntimeID string
	Leases                  []ArtifactLease
}

// AttemptObservation summarizes only persisted, identity-checked Docker exit
// and clone-quiescence evidence. It never infers a missing observation.
type AttemptObservation struct {
	ExitEvidenceID, QuiescenceEvidenceID int64
	ExitCode                             int
}

// ReadAttemptObservation supplies ordered proof IDs for recovery publication.
// A conflicting or malformed duplicate fails closed instead of selecting the
// most convenient observation.
func (s *SQLiteStore) ReadAttemptObservation(ctx context.Context,
	bound BoundAttempt) (AttemptObservation, error) {
	a := bound.Attempt
	if s == nil || s.db == nil || a.JobID == "" || a.TaskID == "" ||
		a.AttemptNo < 1 || a.ExecutorID == "" || a.CloneID != bound.Clone.ID ||
		a.ExecutionDigest != bound.Binding.Digest || bound.DBRuntimeID == "" {
		return AttemptObservation{}, ErrCommonAttempt
	}
	rows, err := s.db.QueryContext(ctx, `SELECT evidence_id,kind,
json_extract(facts_json,'$.observation.executor_id'),
json_extract(facts_json,'$.observation.exit_code'),
json_extract(facts_json,'$.observation.runtime_absent'),
json_extract(facts_json,'$.observation.runtime_name'),
json_extract(facts_json,'$.observation.runtime_id')
FROM prepare_attempt_evidence WHERE job_id=? AND task_id=? AND attempt_no=?
AND worker_generation=? AND kind IN ('executor_inspection','db_quiescence')
ORDER BY evidence_id`, a.JobID, a.TaskID, a.AttemptNo, a.WorkerGeneration)
	if err != nil {
		return AttemptObservation{}, err
	}
	defer rows.Close()
	var result AttemptObservation
	for rows.Next() {
		var id int64
		var kind string
		var executor, runtimeName, runtimeID sql.NullString
		var exitCode, absent sql.NullInt64
		if err := rows.Scan(&id, &kind, &executor, &exitCode,
			&absent, &runtimeName, &runtimeID); err != nil {
			return AttemptObservation{}, err
		}
		switch kind {
		case "executor_inspection":
			if result.ExitEvidenceID != 0 || executor.String != a.ExecutorID ||
				!exitCode.Valid || exitCode.Int64 < 0 || exitCode.Int64 > 255 {
				return AttemptObservation{}, ErrCommonAttempt
			}
			result.ExitEvidenceID, result.ExitCode = id, int(exitCode.Int64)
		case "db_quiescence":
			if result.QuiescenceEvidenceID != 0 || result.ExitEvidenceID == 0 ||
				absent.Int64 != 1 || runtimeName.String != bound.Clone.PlannedRuntimeName ||
				runtimeID.String != bound.DBRuntimeID {
				return AttemptObservation{}, ErrCommonAttempt
			}
			result.QuiescenceEvidenceID = id
		default:
			return AttemptObservation{}, ErrCommonAttempt
		}
	}
	if err := rows.Err(); err != nil {
		return AttemptObservation{}, err
	}
	if result.ExitEvidenceID == 0 {
		return AttemptObservation{}, ErrCommonAttempt
	}
	return result, nil
}

// CloneAbsenceVerifier verifies that the exact planned database runtime is
// absent and the physical clone bytes have been removed. It may not infer
// absence from a failed Docker inspection. See C01/C04 and the recovery
// storage schema's clone-retention boundary.
type CloneAbsenceVerifier interface {
	VerifyReleasedClone(context.Context, ClonePlan, string) error
}

// RequestAttemptCreate durably crosses the Docker create boundary before the
// API call, so planned remains proof that create was not requested. A crash
// after this transition requires exact-name fencing during R02 recovery.
func (s *SQLiteStore) RequestAttemptCreate(ctx context.Context, attempt AttemptRecord, generation int) error {
	if s == nil || s.db == nil || attempt.JobID == "" || attempt.TaskID == "" || attempt.AttemptNo < 1 || attempt.PlannedExecutorName == "" {
		return ErrCommonAttempt
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_step_attempts SET phase='create_requested'
WHERE job_id=? AND task_id=? AND attempt_no=? AND worker_generation=?
AND planned_executor_name=? AND phase='planned' AND executor_id IS NULL
AND resolution IS NULL AND legacy_unbound=0
AND EXISTS (SELECT 1 FROM prepare_clones c WHERE c.clone_id=prepare_step_attempts.clone_id
  AND c.job_id=prepare_step_attempts.job_id AND c.phase='active')
AND EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=prepare_step_attempts.job_id
  AND j.status='running')
AND EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=prepare_step_attempts.job_id
  AND t.task_id=prepare_step_attempts.task_id AND t.status='running')`,
		attempt.JobID, attempt.TaskID, attempt.AttemptNo, generation, attempt.PlannedExecutorName)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	return nil
}

// AttachAttemptExecutor records Docker's exact returned ID only after a
// durable create request. A mismatched generation or a second attach cannot
// change the immutable planned locator.
func (s *SQLiteStore) AttachAttemptExecutor(ctx context.Context, attempt AttemptRecord, generation int, executorID string) error {
	if s == nil || s.db == nil || !dockerContainerID.MatchString(executorID) {
		return ErrCommonAttempt
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_step_attempts
SET executor_id=?,phase='created'
WHERE job_id=? AND task_id=? AND attempt_no=? AND worker_generation=?
AND planned_executor_name=? AND phase='create_requested' AND executor_id IS NULL`, executorID,
		attempt.JobID, attempt.TaskID, attempt.AttemptNo, generation,
		attempt.PlannedExecutorName)
	if err != nil {
		return err
	}
	if err := oneRowChanged(result); err != nil {
		return ErrCommonAttempt
	}
	return nil
}

// RequestAttemptStart persists the start request before Docker start. A
// stopped/inactive clone or stale generation cannot request a mutating run.
func (s *SQLiteStore) RequestAttemptStart(ctx context.Context, attempt AttemptRecord, generation int, startedAt string) error {
	if s == nil || s.db == nil || startedAt == "" {
		return ErrCommonAttempt
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_step_attempts
SET phase='start_requested',started_at=?
WHERE job_id=? AND task_id=? AND attempt_no=? AND worker_generation=?
AND planned_executor_name=? AND phase='created' AND executor_id IS NOT NULL
AND EXISTS (SELECT 1 FROM prepare_clones c WHERE c.clone_id=prepare_step_attempts.clone_id
  AND c.job_id=prepare_step_attempts.job_id AND c.phase='active')
AND EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=prepare_step_attempts.job_id
  AND j.status='running')
AND EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=prepare_step_attempts.job_id
  AND t.task_id=prepare_step_attempts.task_id AND t.status='running')`, startedAt,
		attempt.JobID, attempt.TaskID, attempt.AttemptNo, generation,
		attempt.PlannedExecutorName)
	if err != nil {
		return err
	}
	if err := oneRowChanged(result); err != nil {
		return ErrCommonAttempt
	}
	return nil
}

func commonLiquibaseDigest(values ...any) string {
	data, _ := json.Marshal(values)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
