//go:build linux

package benchmark

import (
	"context"
	"database/sql"
	"time"
)

// diagnosticPhase implements docs/architecture/issue89-performance-diagnostic-design.md:
// expose only relative numeric boundaries and a fixed label;
// unavailable version-specific facts are null, never zero-duration evidence.
type diagnosticPhase struct {
	Category       string
	Ordinal        int
	Available      bool
	StartMS, EndMS *float64
}

// diagnosticPass is the private collector's closed output, validated again by
// the parent before upload. Trace is null for untraced members (DG08/DG09).
type diagnosticPass struct {
	Pass       string
	Traced     bool
	ElapsedMS  float64
	Phases     []diagnosticPhase
	Trace      *diagnosticTraceSummary
	start, end time.Time
}

func diagnosticPhaseTimes(category string, ordinal int, start, end sql.NullString, origin time.Time) (diagnosticPhase, error) {
	p := diagnosticPhase{Category: category, Ordinal: ordinal}
	if !start.Valid || !end.Valid {
		return p, nil
	}
	a, e1 := time.Parse(time.RFC3339Nano, start.String)
	b, e2 := time.Parse(time.RFC3339Nano, end.String)
	_, z1 := a.Zone()
	_, z2 := b.Zone()
	if e1 != nil || e2 != nil || z1 != 0 || z2 != 0 || b.Before(a) {
		return p, errDiagnosticTrace
	}
	x, y := float64(a.Sub(origin))/1e6, float64(b.Sub(origin))/1e6
	if x < 0 || y < 0 {
		return p, errDiagnosticTrace
	}
	p.Available = true
	p.StartMS = &x
	p.EndMS = &y
	return p, nil
}

// readDiagnosticJournal executes fixed read-only timestamp queries, never
// request/event/SQL/capture text. Optional tables are explicit unavailable facts
// on the released version, not silently swallowed query errors. DG08.
func readDiagnosticJournal(ctx context.Context, db *sql.DB, job string, origin time.Time) ([]diagnosticPhase, error) {
	var created, started, finished sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT created_at,started_at,finished_at FROM prepare_jobs WHERE job_id=?`, job).Scan(&created, &started, &finished); err != nil || !created.Valid || !started.Valid || !finished.Valid {
		return nil, errDiagnosticTrace
	}
	root, err := diagnosticPhaseTimes("job", 0, created, finished, origin)
	if err != nil {
		return nil, errDiagnosticTrace
	}
	if _, err := diagnosticPhaseTimes("queued", 0, created, started, origin); err != nil {
		return nil, errDiagnosticTrace
	}
	out := []diagnosticPhase{root}
	rows, err := db.QueryContext(ctx, `SELECT position,type,started_at,finished_at FROM prepare_tasks WHERE job_id=? ORDER BY position`, job)
	if err != nil {
		return nil, errDiagnosticTrace
	}
	for rows.Next() {
		var ordinal int
		var kind string
		var a, b sql.NullString
		if err := rows.Scan(&ordinal, &kind, &a, &b); err != nil {
			rows.Close()
			return nil, errDiagnosticTrace
		}
		category := "other_task"
		switch kind {
		case "state_execute":
			category = "step"
		case "plan":
			category = "plan_task"
		case "resolve_image":
			category = "resolve_task"
		case "instance_create":
			category = "instance_task"
		}
		p, err := diagnosticPhaseTimes(category, ordinal, a, b, origin)
		if err != nil {
			rows.Close()
			return nil, errDiagnosticTrace
		}
		out = append(out, p)
	}
	rowErr := rows.Err()
	closeErr := rows.Close()
	if rowErr != nil || closeErr != nil {
		return nil, errDiagnosticTrace
	}
	for _, item := range []struct{ table, category, query string }{
		{"liquibase_job_bindings", "image_binding", `SELECT j.started_at,b.bound_at FROM prepare_jobs j JOIN liquibase_job_bindings b USING(job_id) WHERE j.job_id=?`},
		{"prepare_planning_intents", "planning", `SELECT created_at,plan_saved_at FROM prepare_planning_intents WHERE job_id=?`},
		{"prepare_liquibase_sessions", "session_start", `SELECT s.created_at,r.verified_at FROM prepare_liquibase_sessions s LEFT JOIN prepare_liquibase_session_readiness r USING(session_id) WHERE s.job_id=? ORDER BY s.session_no`},
	} {
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, item.table).Scan(&exists); err != nil {
			return nil, errDiagnosticTrace
		}
		p := diagnosticPhase{Category: item.category}
		if exists != 0 {
			var a, b sql.NullString
			err := db.QueryRowContext(ctx, item.query, job).Scan(&a, &b)
			if err != nil && err != sql.ErrNoRows {
				return nil, errDiagnosticTrace
			}
			if err == nil {
				p, err = diagnosticPhaseTimes(item.category, 0, a, b, origin)
				if err != nil {
					return nil, errDiagnosticTrace
				}
			}
		}
		out = append(out, p)
	}
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='prepare_liquibase_invocations'`).Scan(&exists); err != nil {
		return nil, errDiagnosticTrace
	}
	if exists != 0 {
		rows, err := db.QueryContext(ctx, `SELECT t.position,i.start_requested_at,i.complete_at FROM prepare_liquibase_invocations i JOIN prepare_tasks t USING(job_id,task_id) WHERE i.job_id=? ORDER BY t.position,i.attempt_no`, job)
		if err != nil {
			return nil, errDiagnosticTrace
		}
		defer rows.Close()
		for rows.Next() {
			var n int
			var a, b sql.NullString
			if rows.Scan(&n, &a, &b) != nil {
				return nil, errDiagnosticTrace
			}
			p, err := diagnosticPhaseTimes("invocation", n, a, b, origin)
			if err != nil {
				return nil, errDiagnosticTrace
			}
			out = append(out, p)
		}
		if rows.Err() != nil {
			return nil, errDiagnosticTrace
		}
	}
	return out, nil
}
