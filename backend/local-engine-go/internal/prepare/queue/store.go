package queue

import "context"

type JobRecord struct {
	// Immutable recovery provenance, selected before job creation; see
	// docs/architecture/managed-database-identity-internals.md.
	ResolvedImageID       string
	LineageRef            string
	IdentityDigest        string
	JobID                 string
	Status                string
	PrepareKind           string
	ImageID               string
	PlanOnly              bool
	SnapshotMode          string
	PrepareArgsNormalized *string
	Signature             *string
	RequestJSON           *string
	CreatedAt             string
	StartedAt             *string
	FinishedAt            *string
	ResultJSON            *string
	ErrorJSON             *string
}

type TaskRecord struct {
	JobID           string
	TaskID          string
	Position        int
	Type            string
	Status          string
	PlannerKind     *string
	InputKind       *string
	InputID         *string
	ImageID         *string
	ResolvedImageID *string
	TaskHash        *string
	OutputStateID   *string
	Cached          *bool
	InstanceMode    *string
	ChangesetID     *string
	ChangesetAuthor *string
	ChangesetPath   *string
	StartedAt       *string
	FinishedAt      *string
	ErrorJSON       *string
}

type EventRecord struct {
	Seq        int64
	JobID      string
	Type       string
	Ts         string
	Status     *string
	TaskID     *string
	Message    *string
	ResultJSON *string
	ErrorJSON  *string
}

type JobUpdate struct {
	Status                *string
	SnapshotMode          *string
	PrepareArgsNormalized *string
	Signature             *string
	RequestJSON           *string
	StartedAt             *string
	FinishedAt            *string
	ResultJSON            *string
	ErrorJSON             *string
}

type TaskUpdate struct {
	Status        *string
	StartedAt     *string
	FinishedAt    *string
	ErrorJSON     *string
	TaskHash      *string
	OutputStateID *string
	Cached        *bool
}

type Store interface {
	CreateJob(ctx context.Context, job JobRecord) error
	UpdateJob(ctx context.Context, jobID string, update JobUpdate) error
	GetJob(ctx context.Context, jobID string) (JobRecord, bool, error)
	ListJobs(ctx context.Context, jobID string) ([]JobRecord, error)
	ListJobsByStatus(ctx context.Context, statuses []string) ([]JobRecord, error)
	ListJobsBySignature(ctx context.Context, signature string, statuses []string) ([]JobRecord, error)
	DeleteJob(ctx context.Context, jobID string) error

	ReplaceTasks(ctx context.Context, jobID string, tasks []TaskRecord) error
	ListTasks(ctx context.Context, jobID string) ([]TaskRecord, error)
	UpdateTask(ctx context.Context, jobID string, taskID string, update TaskUpdate) error

	AppendEvent(ctx context.Context, event EventRecord) (int64, error)
	ListEventsSince(ctx context.Context, jobID string, offset int) ([]EventRecord, error)
	CountEvents(ctx context.Context, jobID string) (int, error)

	Close() error
}
