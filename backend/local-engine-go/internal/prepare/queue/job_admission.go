package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
)

type jobExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// insertJob is shared by ordinary queue admission and the Liquibase
// transaction that also binds its input bundle and retention lease.
func insertJob(ctx context.Context, writer jobExecer, job JobRecord) error {
	query := `
INSERT INTO prepare_jobs (job_id, status, purpose, prepare_kind, image_id, plan_only, snapshot_mode, prepare_args_normalized, signature, request_json, created_at, started_at, finished_at, result_json, cache_explain_result_json, error_json, resolved_image_id, lineage_ref, identity_digest)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := writer.ExecContext(ctx, query,
		job.JobID,
		job.Status,
		jobRecordPurpose(job.Purpose),
		job.PrepareKind,
		job.ImageID,
		boolToInt(job.PlanOnly),
		jobSnapshotMode(job.SnapshotMode),
		nullString(job.PrepareArgsNormalized),
		nullString(job.Signature),
		nullString(job.RequestJSON),
		job.CreatedAt,
		nullString(job.StartedAt),
		nullString(job.FinishedAt),
		nullString(job.ResultJSON),
		nullString(job.CacheExplainResultJSON),
		nullString(job.ErrorJSON),
		nullIfEmpty(job.ResolvedImageID),
		nullIfEmpty(job.LineageRef),
		nullIfEmpty(job.IdentityDigest),
	)
	return err
}

func jobRecordPurpose(purpose string) string {
	if purpose == "" {
		return "prepare"
	}
	return purpose
}
