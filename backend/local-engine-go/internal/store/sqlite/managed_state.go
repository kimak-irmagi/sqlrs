package sqlite

import (
	"context"
	"database/sql"
)

// InstallManagedStateSchema installs state provenance after successful preflight,
// inside the transaction owning the new store format. It is deliberately a
// one-time installation: an existing lineage table is not silently repaired.
// See docs/architecture/managed-database-identity-internals.md, persistence.
func InstallManagedStateSchema(ctx context.Context, tx *sql.Tx) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx == nil {
		return ErrManagedPreflightUnavailable
	}
	for _, ddl := range []string{ManagedLineageSchemaSQL(), SchemaSQL(), managedStateConstraints} {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return managedPreflightError(ctx, err)
		}
	}
	return nil
}

// A local store owns exactly one domain. The unique lineage index allows an
// additive SQLite foreign key without rebuilding or deleting old empty tables.
// Triggers also protect duplicate INSERT OR IGNORE calls and NULL comparisons.
const managedStateConstraints = `
CREATE UNIQUE INDEX idx_managed_local_lineage ON managed_base_lineages(lineage_ref);
ALTER TABLE states ADD COLUMN lineage_ref TEXT REFERENCES managed_base_lineages(lineage_ref);
ALTER TABLE states ADD COLUMN identity_digest TEXT;
CREATE TRIGGER managed_lineage_domain BEFORE INSERT ON managed_base_lineages
WHEN NOT EXISTS (SELECT 1 FROM managed_store_format WHERE slot=1 AND domain_ref=NEW.domain_ref)
BEGIN SELECT RAISE(ABORT,'invalid managed lineage domain'); END;
CREATE TRIGGER managed_lineage_immutable BEFORE UPDATE ON managed_base_lineages
BEGIN SELECT RAISE(ABORT,'managed lineage is immutable'); END;
CREATE TRIGGER managed_state_insert BEFORE INSERT ON states
BEGIN
 SELECT CASE WHEN NOT EXISTS (
  SELECT 1 FROM managed_base_lineages l JOIN managed_store_format f ON f.domain_ref=l.domain_ref
  WHERE f.slot=1 AND l.lineage_ref=NEW.lineage_ref AND l.identity_digest=NEW.identity_digest
   AND (NEW.image_id=l.image_digest OR substr(NEW.image_id,-72)='@'||l.image_digest)
 ) THEN RAISE(ABORT,'invalid managed state binding') END;
 SELECT CASE WHEN NEW.parent_state_id IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM states p WHERE p.state_id=NEW.parent_state_id
   AND p.lineage_ref=NEW.lineage_ref AND p.identity_digest=NEW.identity_digest
 ) THEN RAISE(ABORT,'invalid managed parent binding') END;
 SELECT CASE WHEN EXISTS (
  SELECT 1 FROM states s WHERE s.state_id=NEW.state_id AND (
   s.lineage_ref IS NOT NEW.lineage_ref OR s.identity_digest IS NOT NEW.identity_digest
   OR s.parent_state_id IS NOT NEW.parent_state_id OR s.image_id IS NOT NEW.image_id
   OR s.prepare_kind IS NOT NEW.prepare_kind OR s.prepare_args_normalized IS NOT NEW.prepare_args_normalized
  )
 ) THEN RAISE(ABORT,'conflicting managed state') END;
 SELECT CASE WHEN EXISTS (
  SELECT 1 FROM states s WHERE s.state_fingerprint=NEW.state_fingerprint AND s.state_id IS NOT NEW.state_id
 ) THEN RAISE(ABORT,'conflicting managed state fingerprint') END;
END;
CREATE TRIGGER managed_state_immutable BEFORE UPDATE OF state_id,parent_state_id,image_id,prepare_kind,prepare_args_normalized,lineage_ref,identity_digest ON states
WHEN OLD.state_id IS NOT NEW.state_id OR OLD.parent_state_id IS NOT NEW.parent_state_id
 OR OLD.image_id IS NOT NEW.image_id OR OLD.prepare_kind IS NOT NEW.prepare_kind
 OR OLD.prepare_args_normalized IS NOT NEW.prepare_args_normalized
 OR OLD.lineage_ref IS NOT NEW.lineage_ref OR OLD.identity_digest IS NOT NEW.identity_digest
BEGIN SELECT RAISE(ABORT,'managed state binding is immutable'); END;
`
