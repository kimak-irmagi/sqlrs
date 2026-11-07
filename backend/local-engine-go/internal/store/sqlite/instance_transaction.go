package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlrs/engine-local/internal/store"
)

// InsertInstanceTx performs the existing publication/use-count writes in the
// caller's exact transaction. It does not acquire another connection or commit.
// See liquibase-container-component-structure.md, journal ownership.
func InsertInstanceTx(ctx context.Context, tx *sql.Tx, entry store.InstanceCreate) error {
	if tx == nil {
		return fmt.Errorf("instance publication transaction is required")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO instances (instance_id,state_id,image_id,created_at,expires_at,runtime_id,runtime_dir,status) VALUES(?,?,?,?,?,?,?,?)`, entry.InstanceID, entry.StateID, entry.ImageID, entry.CreatedAt, entry.ExpiresAt, entry.RuntimeID, entry.RuntimeDir, entry.Status); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE states SET last_used_at=?,use_count=COALESCE(use_count,0)+1 WHERE state_id=?`, entry.CreatedAt, entry.StateID)
	return err
}
