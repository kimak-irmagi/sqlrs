package client

type HealthResponse struct {
	Ok         bool   `json:"ok"`
	Version    string `json:"version"`
	InstanceID string `json:"instanceId"`
	PID        int    `json:"pid"`
}

type CacheEvictionSummary struct {
	CompletedAt      string `json:"completed_at"`
	Trigger          string `json:"trigger"`
	EvictedCount     int    `json:"evicted_count"`
	FreedBytes       int64  `json:"freed_bytes"`
	BlockedCount     int    `json:"blocked_count"`
	ReclaimableBytes int64  `json:"reclaimable_bytes"`
	UsageBytesBefore int64  `json:"usage_bytes_before"`
	UsageBytesAfter  int64  `json:"usage_bytes_after"`
	FreeBytesBefore  int64  `json:"free_bytes_before"`
	FreeBytesAfter   int64  `json:"free_bytes_after"`
}

type CacheStatus struct {
	UsageBytes         int64                 `json:"usage_bytes"`
	ConfiguredMaxBytes *int64                `json:"configured_max_bytes,omitempty"`
	EffectiveMaxBytes  int64                 `json:"effective_max_bytes"`
	ReserveBytes       int64                 `json:"reserve_bytes"`
	HighWatermark      float64               `json:"high_watermark"`
	LowWatermark       float64               `json:"low_watermark"`
	MinStateAge        string                `json:"min_state_age"`
	StoreTotalBytes    int64                 `json:"store_total_bytes"`
	StoreFreeBytes     int64                 `json:"store_free_bytes"`
	StateCount         int                   `json:"state_count"`
	ReclaimableBytes   int64                 `json:"reclaimable_bytes"`
	BlockedCount       int                   `json:"blocked_count"`
	PressureReasons    []string              `json:"pressure_reasons"`
	LastEviction       *CacheEvictionSummary `json:"last_eviction,omitempty"`
}

type CacheExplainPrepareResponse struct {
	Decision        string `json:"decision"`
	ReasonCode      string `json:"reason_code,omitempty"`
	Signature       string `json:"signature,omitempty"`
	MatchedStateID  string `json:"matched_state_id,omitempty"`
	ResolvedImageID string `json:"resolved_image_id,omitempty"`
}

type ListFilters struct {
	Name     string
	Instance string
	State    string
	Kind     string
	Image    string
	IDPrefix string
}

type NameEntry struct {
	Name             string  `json:"name"`
	InstanceID       *string `json:"instance_id,omitempty"`
	ImageID          string  `json:"image_id"`
	StateID          string  `json:"state_id"`
	StateFingerprint string  `json:"state_fingerprint,omitempty"`
	Status           string  `json:"status"`
	LastUsedAt       *string `json:"last_used_at,omitempty"`
}

type InstanceEntry struct {
	InstanceID string  `json:"instance_id"`
	ImageID    string  `json:"image_id"`
	StateID    string  `json:"state_id"`
	Name       *string `json:"name,omitempty"`
	CreatedAt  string  `json:"created_at"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
	Status     string  `json:"status"`
}

type StateEntry struct {
	StateID           string  `json:"state_id"`
	ParentStateID     *string `json:"parent_state_id,omitempty"`
	ImageID           string  `json:"image_id"`
	PrepareKind       string  `json:"prepare_kind"`
	PrepareArgs       string  `json:"prepare_args_normalized"`
	CreatedAt         string  `json:"created_at"`
	SizeBytes         *int64  `json:"size_bytes,omitempty"`
	LastUsedAt        *string `json:"last_used_at,omitempty"`
	UseCount          *int64  `json:"use_count,omitempty"`
	MinRetentionUntil *string `json:"min_retention_until,omitempty"`
	RefCount          int     `json:"refcount"`
}

type PrepareJobRequest struct {
	PrepareKind       string            `json:"prepare_kind"`
	ImageID           string            `json:"image_id"`
	PsqlArgs          []string          `json:"psql_args"`
	LiquibaseArgs     []string          `json:"liquibase_args,omitempty"`
	LiquibaseExec     string            `json:"liquibase_exec,omitempty"`
	LiquibaseExecMode string            `json:"liquibase_exec_mode,omitempty"`
	LiquibaseEnv      map[string]string `json:"liquibase_env,omitempty"`
	WorkDir           string            `json:"work_dir,omitempty"`
	Stdin             *string           `json:"stdin,omitempty"`
	SourceManifest    *SourceManifest   `json:"source_manifest,omitempty"`
	PlanOnly          bool              `json:"plan_only,omitempty"`
}

// SourceManifest is the CLI-side representation of the remote source-sync
// contract in docs/architecture/remote-source-input-sync-flow.md.
type SourceManifest struct {
	WorkspaceRef *SourceWorkspaceRef               `json:"workspace_ref,omitempty"`
	Files        map[string]string                 `json:"files,omitempty"`
	Directories  map[string]SourceDirectoryListing `json:"directories,omitempty"`
}

type SourceWorkspaceRef struct {
	RootID   string `json:"root_id,omitempty"`
	RootPath string `json:"root_path"`
	WorkDir  string `json:"work_dir"`
}

type SourceDirectoryListing struct {
	Entries  []SourceDirectoryEntry `json:"entries"`
	Complete bool                   `json:"complete"`
}

type SourceDirectoryEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type SourceMissingManifestEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Reason string `json:"reason,omitempty"`
}

type SourceMissingBlob struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type SourceInputsMissingErrorResponse struct {
	Code                   string                       `json:"code"`
	Message                string                       `json:"message"`
	MissingManifestEntries []SourceMissingManifestEntry `json:"missing_manifest_entries,omitempty"`
	MissingBlobs           []SourceMissingBlob          `json:"missing_blobs,omitempty"`
}

type ConfigValue struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

type PrepareJobAccepted struct {
	JobID     string `json:"job_id"`
	StatusURL string `json:"status_url"`
	EventsURL string `json:"events_url,omitempty"`
	Status    string `json:"status,omitempty"`
}

type PrepareJobStatus struct {
	JobID                 string            `json:"job_id"`
	Status                string            `json:"status"`
	PrepareKind           string            `json:"prepare_kind"`
	ImageID               string            `json:"image_id"`
	PlanOnly              bool              `json:"plan_only,omitempty"`
	PrepareArgsNormalized string            `json:"prepare_args_normalized,omitempty"`
	CreatedAt             *string           `json:"created_at,omitempty"`
	StartedAt             *string           `json:"started_at,omitempty"`
	FinishedAt            *string           `json:"finished_at,omitempty"`
	Tasks                 []PlanTask        `json:"tasks,omitempty"`
	Result                *PrepareJobResult `json:"result,omitempty"`
	Error                 *ErrorResponse    `json:"error,omitempty"`
}

type PrepareJobEntry struct {
	JobID                 string  `json:"job_id"`
	Status                string  `json:"status"`
	PrepareKind           string  `json:"prepare_kind"`
	ImageID               string  `json:"image_id"`
	ResolvedImageID       string  `json:"resolved_image_id,omitempty"`
	PrepareArgsNormalized string  `json:"prepare_args_normalized,omitempty"`
	Signature             string  `json:"signature,omitempty"`
	PlanOnly              bool    `json:"plan_only,omitempty"`
	CreatedAt             *string `json:"created_at,omitempty"`
	StartedAt             *string `json:"started_at,omitempty"`
	FinishedAt            *string `json:"finished_at,omitempty"`
}

type PrepareJobEvent struct {
	Type    string            `json:"type"`
	Ts      string            `json:"ts"`
	Status  string            `json:"status,omitempty"`
	TaskID  string            `json:"task_id,omitempty"`
	Message string            `json:"message,omitempty"`
	Result  *PrepareJobResult `json:"result,omitempty"`
	Error   *ErrorResponse    `json:"error,omitempty"`
}

type PrepareJobResult struct {
	DSN                   string `json:"dsn"`
	InstanceID            string `json:"instance_id"`
	StateID               string `json:"state_id"`
	ImageID               string `json:"image_id"`
	PrepareKind           string `json:"prepare_kind"`
	PrepareArgsNormalized string `json:"prepare_args_normalized"`
}

type RunRequest struct {
	InstanceRef string    `json:"instance_ref"`
	Kind        string    `json:"kind"`
	Command     *string   `json:"command,omitempty"`
	Args        []string  `json:"args"`
	Stdin       *string   `json:"stdin,omitempty"`
	Steps       []RunStep `json:"steps,omitempty"`
}

type RunStep struct {
	Args  []string `json:"args"`
	Stdin *string  `json:"stdin,omitempty"`
}

type RunEvent struct {
	Type       string         `json:"type"`
	Ts         string         `json:"ts"`
	InstanceID string         `json:"instance_id,omitempty"`
	Data       string         `json:"data,omitempty"`
	ExitCode   *int           `json:"exit_code,omitempty"`
	Error      *ErrorResponse `json:"error,omitempty"`
}

type TaskInput struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type PlanTask struct {
	TaskID        string     `json:"task_id"`
	Type          string     `json:"type"`
	PlannerKind   string     `json:"planner_kind,omitempty"`
	Input         *TaskInput `json:"input,omitempty"`
	TaskHash      string     `json:"task_hash,omitempty"`
	OutputStateID string     `json:"output_state_id,omitempty"`
	Cached        *bool      `json:"cached,omitempty"`
	InstanceMode  string     `json:"instance_mode,omitempty"`
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

// UserProfileWriteRequest carries editable user profile fields for the
// remote-only user management API described in docs/architecture/user-org-component-structure.md.
type UserProfileWriteRequest struct {
	DisplayName string  `json:"display_name,omitempty"`
	Email       *string `json:"email,omitempty"`
}

// IdentityKey is the natural idempotency key for externally-authenticated users.
type IdentityKey struct {
	Provider string `json:"provider"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
}

// UserProfileResult is returned by current-user and identity-keyed user endpoints.
type UserProfileResult struct {
	Status      string                       `json:"status,omitempty"`
	User        UserProfile                  `json:"user"`
	Identities  []ExternalIdentity           `json:"identities"`
	Memberships []OrganizationMembershipView `json:"memberships"`
	ETag        string                       `json:"-"`
	Location    string                       `json:"-"`
}

type UserProfile struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name,omitempty"`
	Email       *string `json:"email,omitempty"`
	CreatedAt   string  `json:"created_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
}

type ExternalIdentity struct {
	Provider string `json:"provider"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
}

type OrganizationCreateRequest struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name,omitempty"`
}

type OrganizationCreateResponse struct {
	Organization Organization           `json:"organization"`
	Membership   OrganizationMembership `json:"membership"`
}

type OrganizationMembershipView struct {
	Organization Organization           `json:"organization"`
	Membership   OrganizationMembership `json:"membership"`
}

type Organization struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Endpoint    string `json:"endpoint"`
	DisplayName string `json:"display_name,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type OrganizationMembership struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
	CreatedAt      string `json:"created_at,omitempty"`
}

type DeleteOptions struct {
	Recurse bool
	Force   bool
	DryRun  bool
}

type DeleteResult struct {
	DryRun  bool       `json:"dry_run"`
	Outcome string     `json:"outcome"`
	Root    DeleteNode `json:"root"`
}

type DeleteNode struct {
	Kind        string       `json:"kind"`
	ID          string       `json:"id"`
	Connections *int         `json:"connections,omitempty"`
	Blocked     string       `json:"blocked,omitempty"`
	RuntimeID   *string      `json:"runtime_id,omitempty"`
	Children    []DeleteNode `json:"children,omitempty"`
}
