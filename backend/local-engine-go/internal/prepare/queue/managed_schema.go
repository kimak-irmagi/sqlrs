package queue

import (
	"context"
	"database/sql"
	"errors"
)

// InstallManagedSchema is the queue portion of the caller-owned cutover
// transaction. State/lineage schema must already exist and admission stay closed.
// Existing new-format databases are opened without rerunning this installation.
// See docs/architecture/managed-database-identity-internals.md.
func InstallManagedSchema(ctx context.Context, tx *sql.Tx) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx == nil {
		return errManagedSchema
	}
	for _, ddl := range []string{SchemaSQL(), managedQueueConstraints} {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errManagedSchema
		}
	}
	return nil
}

var errManagedSchema = errors.New("managed prepare queue schema unavailable")

const managedQueueConstraints = `
ALTER TABLE prepare_jobs ADD COLUMN resolved_image_id TEXT;
ALTER TABLE prepare_jobs ADD COLUMN lineage_ref TEXT REFERENCES managed_base_lineages(lineage_ref);
ALTER TABLE prepare_jobs ADD COLUMN identity_digest TEXT;
CREATE TRIGGER managed_job_insert BEFORE INSERT ON prepare_jobs
WHEN NOT EXISTS (
 SELECT 1 FROM managed_base_lineages l JOIN managed_store_format f ON f.domain_ref=l.domain_ref
 WHERE f.slot=1 AND l.lineage_ref=NEW.lineage_ref AND l.identity_digest=NEW.identity_digest
 AND substr(NEW.resolved_image_id, -71)=l.image_digest
)
BEGIN SELECT RAISE(ABORT,'invalid managed job binding'); END;
CREATE TRIGGER managed_job_immutable BEFORE UPDATE OF resolved_image_id,lineage_ref,identity_digest ON prepare_jobs
WHEN OLD.resolved_image_id IS NOT NEW.resolved_image_id
 OR OLD.lineage_ref IS NOT NEW.lineage_ref OR OLD.identity_digest IS NOT NEW.identity_digest
BEGIN SELECT RAISE(ABORT,'managed job binding is immutable'); END;
`
