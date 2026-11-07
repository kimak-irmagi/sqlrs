package prepare

type Request struct {
	PrepareKind       string            `json:"prepare_kind"`
	ImageID           string            `json:"image_id"`
	PsqlArgs          []string          `json:"psql_args"`
	LiquibaseArgs     []string          `json:"liquibase_args,omitempty"`
	LiquibaseVersion  string            `json:"liquibase_version,omitempty"`
	LiquibaseExec     string            `json:"liquibase_exec,omitempty"`
	LiquibaseExecMode string            `json:"liquibase_exec_mode,omitempty"`
	LiquibaseEnv      map[string]string `json:"liquibase_env,omitempty"`
	WorkDir           string            `json:"work_dir,omitempty"`
	Stdin             *string           `json:"stdin,omitempty"`
	PlanOnly          bool              `json:"plan_only,omitempty"`
}

type Accepted struct {
	JobID     string `json:"job_id"`
	StatusURL string `json:"status_url"`
	EventsURL string `json:"events_url,omitempty"`
	Status    string `json:"status,omitempty"`
}

type CacheExplainPrepareResult struct {
	// Accepted is used only for queued Liquibase explanations. The HTTP
	// boundary returns its job reference, never a provisional decision.
	Accepted             *Accepted `json:"-"`
	Decision             string    `json:"decision,omitempty"`
	ReasonCode           string    `json:"reason_code,omitempty"`
	Signature            string    `json:"signature,omitempty"`
	MatchedStateID       string    `json:"matched_state_id,omitempty"`
	ResolvedImageID      string    `json:"resolved_image_id,omitempty"`
	LiquibaseImageDigest string    `json:"liquibase_image_digest,omitempty"`
}

type Status struct {
	Purpose               string                     `json:"purpose"`
	LiquibaseImageDigest  string                     `json:"liquibase_image_digest,omitempty"`
	CacheExplainResult    *CacheExplainPrepareResult `json:"cache_explain_result,omitempty"`
	JobID                 string                     `json:"job_id"`
	Status                string                     `json:"status"`
	PrepareKind           string                     `json:"prepare_kind"`
	ImageID               string                     `json:"image_id"`
	PlanOnly              bool                       `json:"plan_only,omitempty"`
	PrepareArgsNormalized string                     `json:"prepare_args_normalized,omitempty"`
	CreatedAt             *string                    `json:"created_at,omitempty"`
	StartedAt             *string                    `json:"started_at,omitempty"`
	FinishedAt            *string                    `json:"finished_at,omitempty"`
	Tasks                 []PlanTask                 `json:"tasks,omitempty"`
	Result                *Result                    `json:"result,omitempty"`
	Error                 *ErrorResponse             `json:"error,omitempty"`
}

type JobEntry struct {
	Purpose               string                     `json:"purpose"`
	LiquibaseImageDigest  string                     `json:"liquibase_image_digest,omitempty"`
	CacheExplainResult    *CacheExplainPrepareResult `json:"cache_explain_result,omitempty"`
	JobID                 string                     `json:"job_id"`
	Status                string                     `json:"status"`
	PrepareKind           string                     `json:"prepare_kind"`
	ImageID               string                     `json:"image_id"`
	ResolvedImageID       string                     `json:"resolved_image_id,omitempty"`
	PrepareArgsNormalized string                     `json:"prepare_args_normalized,omitempty"`
	Signature             string                     `json:"signature,omitempty"`
	PlanOnly              bool                       `json:"plan_only,omitempty"`
	CreatedAt             *string                    `json:"created_at,omitempty"`
	StartedAt             *string                    `json:"started_at,omitempty"`
	FinishedAt            *string                    `json:"finished_at,omitempty"`
}

type Event struct {
	CacheExplainResult *CacheExplainPrepareResult `json:"cache_explain_result,omitempty"`
	Type               string                     `json:"type"`
	Ts                 string                     `json:"ts"`
	Status             string                     `json:"status,omitempty"`
	TaskID             string                     `json:"task_id,omitempty"`
	Message            string                     `json:"message,omitempty"`
	Result             *Result                    `json:"result,omitempty"`
	Error              *ErrorResponse             `json:"error,omitempty"`
}

type Result struct {
	DSN                   string `json:"dsn"`
	InstanceID            string `json:"instance_id"`
	StateID               string `json:"state_id"`
	ImageID               string `json:"image_id"`
	PrepareKind           string `json:"prepare_kind"`
	PrepareArgsNormalized string `json:"prepare_args_normalized"`
}

type TaskInput struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type PlanTask struct {
	TaskID          string     `json:"task_id"`
	Type            string     `json:"type"`
	PlannerKind     string     `json:"planner_kind,omitempty"`
	Input           *TaskInput `json:"input,omitempty"`
	ImageID         string     `json:"image_id,omitempty"`
	ResolvedImageID string     `json:"resolved_image_id,omitempty"`
	TaskHash        string     `json:"task_hash,omitempty"`
	OutputStateID   string     `json:"output_state_id,omitempty"`
	Cached          *bool      `json:"cached,omitempty"`
	InstanceMode    string     `json:"instance_mode,omitempty"`
	ChangesetID     string     `json:"changeset_id,omitempty"`
	ChangesetAuthor string     `json:"changeset_author,omitempty"`
	ChangesetPath   string     `json:"changeset_path,omitempty"`
}

type TaskEntry struct {
	TaskID          string     `json:"task_id"`
	JobID           string     `json:"job_id"`
	Type            string     `json:"type"`
	Status          string     `json:"status"`
	PlannerKind     string     `json:"planner_kind,omitempty"`
	Input           *TaskInput `json:"input,omitempty"`
	ImageID         string     `json:"image_id,omitempty"`
	ResolvedImageID string     `json:"resolved_image_id,omitempty"`
	ArgsSummary     string     `json:"args_summary,omitempty"`
	TaskHash        string     `json:"task_hash,omitempty"`
	OutputStateID   string     `json:"output_state_id,omitempty"`
	Cached          *bool      `json:"cached,omitempty"`
	InstanceMode    string     `json:"instance_mode,omitempty"`
	ChangesetID     string     `json:"changeset_id,omitempty"`
	ChangesetAuthor string     `json:"changeset_author,omitempty"`
	ChangesetPath   string     `json:"changeset_path,omitempty"`
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}
