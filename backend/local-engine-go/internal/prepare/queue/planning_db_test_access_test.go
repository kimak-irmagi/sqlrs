package queue

import (
	"context"
	"database/sql"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
)

// PlanningDBPublicationTargetForTest exposes only the pure metadata predicate
// to the independent real-journal tests; no product hook or effect is added.
func PlanningDBPublicationTargetForTest(ctx context.Context, db *sql.DB, job, instance string, op instanceaccess.Operation) bool {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false
	}
	defer tx.Rollback()
	return planningDBPublicationTarget(ctx, tx, job, instance, op)
}
