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
	"regexp"
	"strings"
)

// ErrLiquibaseBinding means the job is not admitted and unbound, or immutable
// execution content under an existing fingerprint does not match.
var ErrLiquibaseBinding = errors.New("invalid Liquibase execution binding")

var bindingDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var bindingTag = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// LiquibaseExecutionRecord is the durable non-secret resolution from
// docs/architecture/liquibase-container-storage-schema.md. Its fingerprint
// must already have been computed by the bound Liquibase policy builder.
type LiquibaseExecutionRecord struct {
	ExecutionFingerprint  string
	DeclarationDigest     string
	RequestedTag          string
	TargetIdentityDigest  string
	PlatformOS            string
	PlatformArch          string
	PlatformVariant       string
	ImageIndexDigest      string
	ImageManifestDigest   string
	DriverMode            string
	DriverDigest          string
	DriverArtifactRef     string
	BundleManifestDigest  string
	EffectiveConfigDigest string
	PolicyRevision        string
	CreatedAt             string
}

// LiquibaseJobBinding is the admitted, immutable selector and source bundle.
// ExecutionFingerprint remains empty until BindLiquibaseExecution commits.
type LiquibaseJobBinding struct {
	RequestedTag         string
	BundleManifestDigest string
	ExecutionFingerprint string
}

// GetLiquibaseStateExecution returns the immutable execution provenance of a
// cached lb state. A missing row is not evidence that the state is reusable.
func (s *SQLiteStore) GetLiquibaseStateExecution(ctx context.Context, stateID string) (string, bool, error) {
	if s == nil || s.db == nil || stateID == "" {
		return "", false, ErrLiquibaseBinding
	}
	var fingerprint string
	err := s.db.QueryRowContext(ctx, `SELECT execution_fingerprint FROM liquibase_state_bindings WHERE state_id=?`, stateID).Scan(&fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !bindingDigest.MatchString(fingerprint) {
		return "", false, ErrLiquibaseBinding
	}
	return fingerprint, true, nil
}

// GetLiquibaseJobBinding restores the selector and source lease identity
// without reading client paths. See the recovery order in
// docs/architecture/liquibase-container-storage-schema.md.
func (s *SQLiteStore) GetLiquibaseJobBinding(ctx context.Context, jobID string) (LiquibaseJobBinding, bool, error) {
	if s == nil || s.db == nil || jobID == "" {
		return LiquibaseJobBinding{}, false, ErrLiquibaseBinding
	}
	var binding LiquibaseJobBinding
	var fingerprint sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT requested_tag,bundle_manifest_digest,execution_fingerprint
FROM liquibase_job_bindings WHERE job_id=?`, jobID).Scan(&binding.RequestedTag, &binding.BundleManifestDigest, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return LiquibaseJobBinding{}, false, nil
	}
	if err != nil {
		return LiquibaseJobBinding{}, false, err
	}
	binding.ExecutionFingerprint = fingerprint.String
	return binding, true, nil
}

// GetLiquibaseExecutionForJob reads only a committed pinned execution. A
// missing result means admission happened but resolution did not yet commit.
func (s *SQLiteStore) GetLiquibaseExecutionForJob(ctx context.Context, jobID string) (LiquibaseExecutionRecord, bool, error) {
	if s == nil || s.db == nil || jobID == "" {
		return LiquibaseExecutionRecord{}, false, ErrLiquibaseBinding
	}
	return readLiquibaseExecutionForJob(ctx, s.db, jobID)
}

// readLiquibaseExecutionForJob shares the exact immutable record validation
// with transaction-bound whole-owner recovery; no nested connection is needed.
func readLiquibaseExecutionForJob(ctx context.Context, reader cancellationRowReader, jobID string) (LiquibaseExecutionRecord, bool, error) {
	var record LiquibaseExecutionRecord
	var variant, indexDigest, artifactRef sql.NullString
	err := reader.QueryRowContext(ctx, `SELECT e.execution_fingerprint,e.declaration_digest,e.requested_tag,
e.target_identity_digest,e.platform_os,e.platform_arch,e.platform_variant,
e.image_index_digest,e.image_manifest_digest,e.driver_mode,e.driver_digest,
e.driver_artifact_ref,e.bundle_manifest_digest,e.effective_config_digest,
e.execution_policy_revision,e.created_at
FROM liquibase_job_bindings b JOIN liquibase_execution_bindings e
ON e.execution_fingerprint=b.execution_fingerprint WHERE b.job_id=?`, jobID).Scan(
		&record.ExecutionFingerprint, &record.DeclarationDigest, &record.RequestedTag,
		&record.TargetIdentityDigest, &record.PlatformOS, &record.PlatformArch, &variant,
		&indexDigest, &record.ImageManifestDigest, &record.DriverMode, &record.DriverDigest,
		&artifactRef, &record.BundleManifestDigest, &record.EffectiveConfigDigest,
		&record.PolicyRevision, &record.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return LiquibaseExecutionRecord{}, false, nil
	}
	if err != nil {
		return LiquibaseExecutionRecord{}, false, err
	}
	record.PlatformVariant = variant.String
	record.ImageIndexDigest = indexDigest.String
	record.DriverArtifactRef = artifactRef.String
	if !validExecutionRecord(record) {
		return LiquibaseExecutionRecord{}, false, ErrLiquibaseBinding
	}
	return record, true, nil
}

// AdmitLiquibaseJob atomically persists an accepted lb job, its immutable
// mutable-tag selector, and a job-scoped lease on a previously published and
// verified input bundle. Publication/verification precedes this transaction;
// no plan may run until BindLiquibaseExecution subsequently succeeds.
func (s *SQLiteStore) AdmitLiquibaseJob(ctx context.Context, job JobRecord, requestedTag, bundleDigest string) (returnedErr error) {
	if s == nil || s.db == nil || job.JobID == "" || job.PrepareKind != "lb" || job.Status != "queued" ||
		job.Signature != nil || job.CreatedAt == "" || !bindingTag.MatchString(requestedTag) || !bindingDigest.MatchString(bundleDigest) {
		return ErrLiquibaseBinding
	}
	if job.RequestJSON == nil {
		return ErrLiquibaseBinding
	}
	// The main delivery retains the original portable input snapshot only.
	// Expanded search modes/revision validation are the separate #173 contract.
	var admitted struct {
		Container *struct {
			NormalizedArgs []string `json:"normalized_args"`
			Mounts         []string `json:"mounts"`
			WorkDir        string   `json:"container_work_dir"`
		} `json:"_sqlrs_container_admission"`
	}
	if err := json.Unmarshal([]byte(*job.RequestJSON), &admitted); err != nil || admitted.Container == nil || len(admitted.Container.NormalizedArgs) == 0 || len(admitted.Container.Mounts) == 0 || admitted.Container.WorkDir == "" {
		return ErrLiquibaseBinding
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			returnedErr = errors.Join(returnedErr, err)
		}
	}()
	if err := insertJob(ctx, tx, job); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO liquibase_job_bindings(job_id,requested_tag,bundle_manifest_digest) VALUES(?,?,?)`,
		job.JobID, requestedTag, bundleDigest); err != nil {
		return err
	}
	leaseTable, err := artifactLeaseTable(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO `+leaseTable+`(
lease_id,artifact_kind,artifact_digest,owner_kind,owner_id,created_at)
VALUES(?,'bundle',?,'job',?,?)`, job.JobID+":bundle", bundleDigest, job.JobID, job.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// ConfirmLiquibaseAdmission observes the exact job/source/lease tuple in one
// read transaction after a lost commit reply. It neither inserts nor repairs
// rows, and never accepts a partial or different admission. See SP15/F3–F6.
func (s *SQLiteStore) ConfirmLiquibaseAdmission(ctx context.Context, job JobRecord, tag, bundle string) (confirmed bool, returnedErr error) {
	if s == nil || s.db == nil || job.RequestJSON == nil {
		return false, ErrLiquibaseBinding
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			confirmed = false
			returnedErr = errors.Join(returnedErr, err)
		}
	}()
	leases, err := artifactLeaseTable(ctx, tx)
	if err != nil {
		return false, err
	}
	var matches int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j
JOIN liquibase_job_bindings b ON b.job_id=j.job_id
JOIN `+leases+` l ON l.owner_kind='job' AND l.owner_id=j.job_id
WHERE j.job_id=? AND j.status=? AND j.purpose=? AND j.prepare_kind=? AND j.image_id=?
AND j.plan_only=? AND j.snapshot_mode=? AND j.prepare_args_normalized IS ?
AND j.signature IS NULL AND j.request_json=? AND j.created_at=?
AND j.started_at IS ? AND j.finished_at IS ? AND j.result_json IS ?
AND j.cache_explain_result_json IS ? AND j.error_json IS ?
AND COALESCE(j.resolved_image_id,'')=? AND COALESCE(j.lineage_ref,'')=? AND COALESCE(j.identity_digest,'')=?
AND b.requested_tag=? AND b.bundle_manifest_digest=? AND b.execution_fingerprint IS NULL
AND l.lease_id=? AND l.artifact_kind='bundle' AND l.artifact_digest=? AND l.created_at=? AND l.expires_at IS NULL`,
		job.JobID, job.Status, jobRecordPurpose(job.Purpose), job.PrepareKind, job.ImageID, boolToInt(job.PlanOnly), jobSnapshotMode(job.SnapshotMode), nullString(job.PrepareArgsNormalized), *job.RequestJSON, job.CreatedAt,
		nullString(job.StartedAt), nullString(job.FinishedAt), nullString(job.ResultJSON), nullString(job.CacheExplainResultJSON), nullString(job.ErrorJSON), job.ResolvedImageID, job.LineageRef, job.IdentityDigest,
		tag, bundle, job.JobID+":bundle", bundle, job.CreatedAt).Scan(&matches)
	return matches == 1, err
}

// BindLiquibaseExecution atomically saves a pinned execution row, binds one
// previously admitted lb job, and writes its domain-separated signature.
// Failed validation, a collision, or a lost binding race rolls back all three
// writes. Planning and cache lookup must follow a successful commit.
func (s *SQLiteStore) BindLiquibaseExecution(ctx context.Context, jobID string, record LiquibaseExecutionRecord) error {
	if s == nil || s.db == nil || jobID == "" || !validExecutionRecord(record) {
		return ErrLiquibaseBinding
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var planOnly int
	var signature, bound sql.NullString
	var requestedTag, bundleDigest string
	err = tx.QueryRowContext(ctx, `SELECT j.plan_only,j.signature,b.requested_tag,b.bundle_manifest_digest,b.execution_fingerprint
FROM prepare_jobs j JOIN liquibase_job_bindings b USING(job_id)
WHERE j.job_id=? AND j.prepare_kind='lb' AND j.status IN ('queued','running')`, jobID).Scan(&planOnly, &signature, &requestedTag, &bundleDigest, &bound)
	if err != nil {
		return ErrLiquibaseBinding
	}
	if signature.Valid || bound.Valid || requestedTag != record.RequestedTag || bundleDigest != record.BundleManifestDigest {
		return ErrLiquibaseBinding
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO liquibase_execution_bindings(
execution_fingerprint,declaration_digest,requested_tag,target_identity_digest,
platform_os,platform_arch,platform_variant,image_index_digest,image_manifest_digest,
driver_mode,driver_digest,driver_artifact_ref,bundle_manifest_digest,
effective_config_digest,execution_policy_revision,created_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(execution_fingerprint) DO NOTHING`,
		record.ExecutionFingerprint, record.DeclarationDigest, record.RequestedTag, record.TargetIdentityDigest,
		record.PlatformOS, record.PlatformArch, nullableBindingValue(record.PlatformVariant), nullableBindingValue(record.ImageIndexDigest), record.ImageManifestDigest,
		record.DriverMode, record.DriverDigest, nullableBindingValue(record.DriverArtifactRef), record.BundleManifestDigest,
		record.EffectiveConfigDigest, record.PolicyRevision, record.CreatedAt)
	if err != nil {
		return err
	}
	var equal int
	// The registry index is retained as resolution metadata, not as execution
	// identity. Two indexes can select the same immutable platform manifest;
	// the fingerprint deliberately binds the latter. See the Liquibase
	// container component structure, resolved execution fingerprint.
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM liquibase_execution_bindings WHERE
execution_fingerprint=? AND declaration_digest=? AND requested_tag=? AND target_identity_digest=?
AND platform_os=? AND platform_arch=? AND platform_variant IS ?
AND image_manifest_digest=? AND driver_mode=? AND driver_digest=? AND driver_artifact_ref IS ?
AND bundle_manifest_digest=? AND effective_config_digest=? AND execution_policy_revision=?`,
		record.ExecutionFingerprint, record.DeclarationDigest, record.RequestedTag, record.TargetIdentityDigest,
		record.PlatformOS, record.PlatformArch, nullableBindingValue(record.PlatformVariant), record.ImageManifestDigest,
		record.DriverMode, record.DriverDigest, nullableBindingValue(record.DriverArtifactRef), record.BundleManifestDigest,
		record.EffectiveConfigDigest, record.PolicyRevision).Scan(&equal)
	if err != nil || equal != 1 {
		return ErrLiquibaseBinding
	}
	result, err := tx.ExecContext(ctx, `UPDATE liquibase_job_bindings
SET execution_fingerprint=?,bound_at=? WHERE job_id=? AND execution_fingerprint IS NULL`,
		record.ExecutionFingerprint, record.CreatedAt, jobID)
	if err != nil || oneRowChanged(result) != nil {
		return ErrLiquibaseBinding
	}
	signatureValue := liquibaseBoundSignature(record.ExecutionFingerprint, planOnly != 0)
	result, err = tx.ExecContext(ctx, `UPDATE prepare_jobs SET signature=? WHERE job_id=? AND signature IS NULL`, signatureValue, jobID)
	if err != nil || oneRowChanged(result) != nil {
		return ErrLiquibaseBinding
	}
	if record.DriverMode == "mounted" {
		leaseTable, err := artifactLeaseTable(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+leaseTable+`(
lease_id,artifact_kind,artifact_digest,owner_kind,owner_id,created_at)
VALUES(?,'driver',?,'job',?,?)`, jobID+":driver", record.DriverArtifactRef, jobID, record.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validExecutionRecord(record LiquibaseExecutionRecord) bool {
	for _, value := range []string{
		record.ExecutionFingerprint, record.DeclarationDigest, record.TargetIdentityDigest,
		record.ImageManifestDigest, record.DriverDigest, record.BundleManifestDigest,
		record.EffectiveConfigDigest,
	} {
		if !bindingDigest.MatchString(value) {
			return false
		}
	}
	if record.ImageIndexDigest != "" && !bindingDigest.MatchString(record.ImageIndexDigest) {
		return false
	}
	if !bindingTag.MatchString(record.RequestedTag) || record.PlatformOS == "" || record.PlatformArch == "" || record.PolicyRevision == "" || record.CreatedAt == "" {
		return false
	}
	if record.DriverMode == "embedded" {
		return record.DriverArtifactRef == ""
	}
	return record.DriverMode == "mounted" && strings.TrimSpace(record.DriverArtifactRef) != ""
}

func nullableBindingValue(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func oneRowChanged(result sql.Result) error {
	if result == nil {
		return ErrLiquibaseBinding
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return ErrLiquibaseBinding
	}
	return nil
}

func liquibaseBoundSignature(fingerprint string, planOnly bool) string {
	flag := "execute"
	if planOnly {
		flag = "plan"
	}
	sum := sha256.Sum256([]byte("lb-container-job-v1\n" + fingerprint + "\n" + flag))
	return hex.EncodeToString(sum[:])
}
