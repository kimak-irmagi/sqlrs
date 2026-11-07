package queue

import (
	"context"
	"database/sql"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
)

// ContainerJobView reads only current admitted Liquibase rows. The base queue
// reader remains usable independently of the optional component schema.
// See liquibase-planning-recovery-design.md: saved decision vs prepare result.
type ContainerJobView struct {
	Purpose              string
	LiquibaseImageDigest string
	Explanation          *planningcodec.Decision
}

func (s *SQLiteStore) LoadContainerJobView(ctx context.Context, jobID string) (ContainerJobView, error) {
	var view ContainerJobView
	var status string
	var image, encoded sql.NullString
	if s == nil || s.db == nil || jobID == "" {
		return view, ErrPlanningSchema
	}
	if err := s.db.QueryRowContext(ctx, `SELECT j.purpose,j.status,j.cache_explain_result_json,e.image_manifest_digest FROM prepare_jobs j JOIN liquibase_job_bindings b ON b.job_id=j.job_id LEFT JOIN liquibase_execution_bindings e ON e.execution_fingerprint=b.execution_fingerprint WHERE j.job_id=? AND j.prepare_kind='lb'`, jobID).Scan(&view.Purpose, &status, &encoded, &image); err != nil {
		return view, err
	}
	view.LiquibaseImageDigest = image.String
	if encoded.Valid {
		if view.Purpose != "cache_explain" || status != "succeeded" {
			return ContainerJobView{}, ErrPlanningSchema
		}
		decision, found, err := s.LoadPlanningDecision(ctx, jobID)
		if err != nil || !found {
			return ContainerJobView{}, ErrPlanningSchema
		}
		canonical, _, err := planningcodec.CanonicalDecision(decision)
		if err != nil || canonical != encoded.String {
			return ContainerJobView{}, ErrPlanningSchema
		}
		view.Explanation = &decision
	}
	return view, nil
}
