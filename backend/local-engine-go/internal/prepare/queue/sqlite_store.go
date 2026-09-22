package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

var sqlOpenFn = sql.Open

func Open(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("sqlite path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sqlOpenFn("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := initDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func New(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	if err := initDB(db); err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) CreateJob(ctx context.Context, job JobRecord) error {
	query := `
INSERT INTO prepare_jobs (job_id, status, prepare_kind, image_id, plan_only, snapshot_mode, prepare_args_normalized, signature, request_json, created_at, started_at, finished_at, result_json, error_json, resolved_image_id, lineage_ref, identity_digest)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query,
		job.JobID,
		job.Status,
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
		nullString(job.ErrorJSON),
		job.ResolvedImageID,
		job.LineageRef,
		job.IdentityDigest,
	)
	return err
}

func (s *SQLiteStore) UpdateJob(ctx context.Context, jobID string, update JobUpdate) error {
	if jobID == "" {
		return fmt.Errorf("job id is required")
	}
	sets := []string{}
	args := []any{}
	if update.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *update.Status)
	}
	if update.SnapshotMode != nil {
		sets = append(sets, "snapshot_mode = ?")
		args = append(args, *update.SnapshotMode)
	}
	if update.PrepareArgsNormalized != nil {
		sets = append(sets, "prepare_args_normalized = ?")
		args = append(args, *update.PrepareArgsNormalized)
	}
	if update.Signature != nil {
		sets = append(sets, "signature = ?")
		args = append(args, *update.Signature)
	}
	if update.RequestJSON != nil {
		sets = append(sets, "request_json = ?")
		args = append(args, *update.RequestJSON)
	}
	if update.StartedAt != nil {
		sets = append(sets, "started_at = ?")
		args = append(args, *update.StartedAt)
	}
	if update.FinishedAt != nil {
		sets = append(sets, "finished_at = ?")
		args = append(args, *update.FinishedAt)
	}
	if update.ResultJSON != nil {
		sets = append(sets, "result_json = ?")
		args = append(args, *update.ResultJSON)
	}
	if update.ErrorJSON != nil {
		sets = append(sets, "error_json = ?")
		args = append(args, *update.ErrorJSON)
	}
	if len(sets) == 0 {
		return nil
	}
	query := "UPDATE prepare_jobs SET " + strings.Join(sets, ", ") + " WHERE job_id = ?"
	args = append(args, jobID)
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *SQLiteStore) GetJob(ctx context.Context, jobID string) (JobRecord, bool, error) {
	query := `
SELECT job_id, status, prepare_kind, image_id, plan_only, snapshot_mode, prepare_args_normalized, signature, request_json,
       created_at, started_at, finished_at, result_json, error_json, coalesce(resolved_image_id,''), coalesce(lineage_ref,''), coalesce(identity_digest,'')
FROM prepare_jobs
WHERE job_id = ?`
	row := s.db.QueryRowContext(ctx, query, jobID)
	record, err := scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobRecord{}, false, nil
		}
		return JobRecord{}, false, err
	}
	return record, true, nil
}

func (s *SQLiteStore) ListJobs(ctx context.Context, jobID string) ([]JobRecord, error) {
	query := strings.Builder{}
	query.WriteString(`
SELECT job_id, status, prepare_kind, image_id, plan_only, snapshot_mode, prepare_args_normalized, signature, request_json,
       created_at, started_at, finished_at, result_json, error_json, coalesce(resolved_image_id,''), coalesce(lineage_ref,''), coalesce(identity_digest,'')
FROM prepare_jobs
WHERE 1=1`)
	args := []any{}
	addPrefixFilter(&query, &args, "job_id", jobID)
	query.WriteString(" ORDER BY created_at")
	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []JobRecord
	for rows.Next() {
		record, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) ListJobsByStatus(ctx context.Context, statuses []string) ([]JobRecord, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	query := strings.Builder{}
	query.WriteString(`
SELECT job_id, status, prepare_kind, image_id, plan_only, snapshot_mode, prepare_args_normalized, signature, request_json,
       created_at, started_at, finished_at, result_json, error_json, coalesce(resolved_image_id,''), coalesce(lineage_ref,''), coalesce(identity_digest,'')
FROM prepare_jobs
WHERE status IN (`)
	args := []any{}
	for i, status := range statuses {
		if i > 0 {
			query.WriteString(",")
		}
		query.WriteString("?")
		args = append(args, status)
	}
	query.WriteString(") ORDER BY created_at")
	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []JobRecord
	for rows.Next() {
		record, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) ListJobsBySignature(ctx context.Context, signature string, statuses []string) ([]JobRecord, error) {
	if strings.TrimSpace(signature) == "" {
		return nil, nil
	}
	query := strings.Builder{}
	query.WriteString(`
SELECT job_id, status, prepare_kind, image_id, plan_only, snapshot_mode, prepare_args_normalized, signature, request_json,
       created_at, started_at, finished_at, result_json, error_json, coalesce(resolved_image_id,''), coalesce(lineage_ref,''), coalesce(identity_digest,'')
FROM prepare_jobs
WHERE signature = ?`)
	args := []any{signature}
	if len(statuses) > 0 {
		query.WriteString(" AND status IN (")
		for i, status := range statuses {
			if i > 0 {
				query.WriteString(",")
			}
			query.WriteString("?")
			args = append(args, status)
		}
		query.WriteString(")")
	}
	query.WriteString(" ORDER BY COALESCE(finished_at, created_at) DESC")
	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []JobRecord
	for rows.Next() {
		record, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) DeleteJob(ctx context.Context, jobID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM prepare_jobs WHERE job_id = ?`, jobID)
	return err
}

func (s *SQLiteStore) ReplaceTasks(ctx context.Context, jobID string, tasks []TaskRecord) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM prepare_tasks WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}
	query := `
INSERT INTO prepare_tasks (job_id, task_id, position, type, status, planner_kind, input_kind, input_id, image_id, resolved_image_id, task_hash, output_state_id, cached, instance_mode, changeset_id, changeset_author, changeset_path, started_at, finished_at, error_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for _, task := range tasks {
		_, err := s.db.ExecContext(ctx, query,
			task.JobID,
			task.TaskID,
			task.Position,
			task.Type,
			task.Status,
			nullString(task.PlannerKind),
			nullString(task.InputKind),
			nullString(task.InputID),
			nullString(task.ImageID),
			nullString(task.ResolvedImageID),
			nullString(task.TaskHash),
			nullString(task.OutputStateID),
			nullBool(task.Cached),
			nullString(task.InstanceMode),
			nullString(task.ChangesetID),
			nullString(task.ChangesetAuthor),
			nullString(task.ChangesetPath),
			nullString(task.StartedAt),
			nullString(task.FinishedAt),
			nullString(task.ErrorJSON),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) ListTasks(ctx context.Context, jobID string) ([]TaskRecord, error) {
	query := strings.Builder{}
	query.WriteString(`
SELECT job_id, task_id, position, type, status, planner_kind, input_kind, input_id, image_id, resolved_image_id, task_hash, output_state_id, cached, instance_mode, changeset_id, changeset_author, changeset_path, started_at, finished_at, error_json
FROM prepare_tasks
WHERE 1=1`)
	args := []any{}
	addFilter(&query, &args, "job_id", jobID)
	query.WriteString(" ORDER BY job_id, position")
	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TaskRecord
	for rows.Next() {
		record, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) UpdateTask(ctx context.Context, jobID string, taskID string, update TaskUpdate) error {
	if jobID == "" || taskID == "" {
		return fmt.Errorf("job_id and task_id are required")
	}
	sets := []string{}
	args := []any{}
	if update.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *update.Status)
	}
	if update.StartedAt != nil {
		sets = append(sets, "started_at = ?")
		args = append(args, *update.StartedAt)
	}
	if update.FinishedAt != nil {
		sets = append(sets, "finished_at = ?")
		args = append(args, *update.FinishedAt)
	}
	if update.ErrorJSON != nil {
		sets = append(sets, "error_json = ?")
		args = append(args, *update.ErrorJSON)
	}
	if update.TaskHash != nil {
		sets = append(sets, "task_hash = ?")
		args = append(args, *update.TaskHash)
	}
	if update.OutputStateID != nil {
		sets = append(sets, "output_state_id = ?")
		args = append(args, *update.OutputStateID)
	}
	if update.Cached != nil {
		sets = append(sets, "cached = ?")
		if *update.Cached {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if len(sets) == 0 {
		return nil
	}
	query := "UPDATE prepare_tasks SET " + strings.Join(sets, ", ") + " WHERE job_id = ? AND task_id = ?"
	args = append(args, jobID, taskID)
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *SQLiteStore) AppendEvent(ctx context.Context, event EventRecord) (int64, error) {
	query := `
INSERT INTO prepare_events (job_id, type, ts, status, task_id, message, result_json, error_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := s.db.ExecContext(ctx, query,
		event.JobID,
		event.Type,
		event.Ts,
		nullString(event.Status),
		nullString(event.TaskID),
		nullString(event.Message),
		nullString(event.ResultJSON),
		nullString(event.ErrorJSON),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) ListEventsSince(ctx context.Context, jobID string, offset int) ([]EventRecord, error) {
	query := `
SELECT seq, job_id, type, ts, status, task_id, message, result_json, error_json
FROM prepare_events
WHERE job_id = ?
ORDER BY seq
LIMIT -1 OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, jobID, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventRecord
	for rows.Next() {
		record, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) CountEvents(ctx context.Context, jobID string) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM prepare_events WHERE job_id = ?`, jobID)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func initDB(db *sql.DB) error {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	if err := ensureTaskImageColumns(db); err != nil {
		return err
	}
	if err := ensureTaskChangesetColumns(db); err != nil {
		return err
	}
	if err := ensureJobSignatureColumn(db); err != nil {
		return err
	}
	if _, err := db.Exec(SchemaSQL()); err != nil {
		return err
	}
	for _, column := range []string{"resolved_image_id", "lineage_ref", "identity_digest"} {
		if _, err := db.Exec("ALTER TABLE prepare_jobs ADD COLUMN " + column + " TEXT"); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}

func ensureTaskImageColumns(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE prepare_tasks ADD COLUMN image_id TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	if _, err := db.Exec("ALTER TABLE prepare_tasks ADD COLUMN resolved_image_id TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func ensureTaskChangesetColumns(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE prepare_tasks ADD COLUMN changeset_id TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	if _, err := db.Exec("ALTER TABLE prepare_tasks ADD COLUMN changeset_author TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	if _, err := db.Exec("ALTER TABLE prepare_tasks ADD COLUMN changeset_path TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func ensureJobSignatureColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE prepare_jobs ADD COLUMN signature TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	return nil
}

func scanJob(scanner interface {
	Scan(dest ...any) error
}) (JobRecord, error) {
	var record JobRecord
	var planOnly int
	var snapshotMode string
	var argsNormalized sql.NullString
	var signature sql.NullString
	var requestJSON sql.NullString
	var startedAt sql.NullString
	var finishedAt sql.NullString
	var resultJSON sql.NullString
	var errorJSON sql.NullString
	if err := scanner.Scan(
		&record.JobID,
		&record.Status,
		&record.PrepareKind,
		&record.ImageID,
		&planOnly,
		&snapshotMode,
		&argsNormalized,
		&signature,
		&requestJSON,
		&record.CreatedAt,
		&startedAt,
		&finishedAt,
		&resultJSON,
		&errorJSON,
		&record.ResolvedImageID,
		&record.LineageRef,
		&record.IdentityDigest,
	); err != nil {
		return JobRecord{}, err
	}
	record.PlanOnly = planOnly != 0
	record.SnapshotMode = snapshotMode
	record.PrepareArgsNormalized = strPtr(argsNormalized)
	record.Signature = strPtr(signature)
	record.RequestJSON = strPtr(requestJSON)
	record.StartedAt = strPtr(startedAt)
	record.FinishedAt = strPtr(finishedAt)
	record.ResultJSON = strPtr(resultJSON)
	record.ErrorJSON = strPtr(errorJSON)
	return record, nil
}

func scanTask(scanner interface {
	Scan(dest ...any) error
}) (TaskRecord, error) {
	var record TaskRecord
	var plannerKind sql.NullString
	var inputKind sql.NullString
	var inputID sql.NullString
	var imageID sql.NullString
	var resolvedImageID sql.NullString
	var taskHash sql.NullString
	var outputStateID sql.NullString
	var cached sql.NullInt64
	var instanceMode sql.NullString
	var changesetID sql.NullString
	var changesetAuthor sql.NullString
	var changesetPath sql.NullString
	var startedAt sql.NullString
	var finishedAt sql.NullString
	var errorJSON sql.NullString
	if err := scanner.Scan(
		&record.JobID,
		&record.TaskID,
		&record.Position,
		&record.Type,
		&record.Status,
		&plannerKind,
		&inputKind,
		&inputID,
		&imageID,
		&resolvedImageID,
		&taskHash,
		&outputStateID,
		&cached,
		&instanceMode,
		&changesetID,
		&changesetAuthor,
		&changesetPath,
		&startedAt,
		&finishedAt,
		&errorJSON,
	); err != nil {
		return TaskRecord{}, err
	}
	record.PlannerKind = strPtr(plannerKind)
	record.InputKind = strPtr(inputKind)
	record.InputID = strPtr(inputID)
	record.ImageID = strPtr(imageID)
	record.ResolvedImageID = strPtr(resolvedImageID)
	record.TaskHash = strPtr(taskHash)
	record.OutputStateID = strPtr(outputStateID)
	record.Cached = boolPtr(cached)
	record.InstanceMode = strPtr(instanceMode)
	record.ChangesetID = strPtr(changesetID)
	record.ChangesetAuthor = strPtr(changesetAuthor)
	record.ChangesetPath = strPtr(changesetPath)
	record.StartedAt = strPtr(startedAt)
	record.FinishedAt = strPtr(finishedAt)
	record.ErrorJSON = strPtr(errorJSON)
	return record, nil
}

func scanEvent(scanner interface {
	Scan(dest ...any) error
}) (EventRecord, error) {
	var record EventRecord
	var status sql.NullString
	var taskID sql.NullString
	var message sql.NullString
	var resultJSON sql.NullString
	var errorJSON sql.NullString
	if err := scanner.Scan(
		&record.Seq,
		&record.JobID,
		&record.Type,
		&record.Ts,
		&status,
		&taskID,
		&message,
		&resultJSON,
		&errorJSON,
	); err != nil {
		return EventRecord{}, err
	}
	record.Status = strPtr(status)
	record.TaskID = strPtr(taskID)
	record.Message = strPtr(message)
	record.ResultJSON = strPtr(resultJSON)
	record.ErrorJSON = strPtr(errorJSON)
	return record, nil
}

func addFilter(query *strings.Builder, args *[]any, column, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	query.WriteString(" AND ")
	query.WriteString(column)
	query.WriteString(" = ?")
	*args = append(*args, value)
}

func addPrefixFilter(query *strings.Builder, args *[]any, column, prefix string) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return
	}
	query.WriteString(" AND ")
	query.WriteString(column)
	query.WriteString(" LIKE ? ESCAPE '\\'")
	*args = append(*args, escapeLike(prefix)+"%")
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func boolPtr(value sql.NullInt64) *bool {
	if !value.Valid {
		return nil
	}
	cached := value.Int64 != 0
	return &cached
}

func nullBool(value *bool) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	if *value {
		return sql.NullInt64{Int64: 1, Valid: true}
	}
	return sql.NullInt64{Int64: 0, Valid: true}
}

func nullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func strPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func jobSnapshotMode(value string) string {
	if strings.TrimSpace(value) == "" {
		return "always"
	}
	return value
}
