package store

import "context"

const (
	NameStatusActive  = "active"
	NameStatusMissing = "missing"
	NameStatusExpired = "expired"

	InstanceStatusActive   = "active"
	InstanceStatusExpired  = "expired"
	InstanceStatusOrphaned = "orphaned"
)

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
	RuntimeID  *string `json:"runtime_id,omitempty"`
	RuntimeDir *string `json:"-"`
	Status     string  `json:"status"`
}

type StateEntry struct {
	// Private provenance; see managed-database-identity-internals.md.
	LineageRef        string  `json:"-"`
	IdentityDigest    string  `json:"-"`
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

type StateCreate struct {
	// Lineage and digest are written atomically with the state.
	LineageRef            string
	IdentityDigest        string
	StateID               string
	ParentStateID         *string
	StateFingerprint      string
	ImageID               string
	PrepareKind           string
	PrepareArgsNormalized string
	CreatedAt             string
	SizeBytes             *int64
	Status                *string
}

type InstanceCreate struct {
	InstanceID string
	StateID    string
	ImageID    string
	CreatedAt  string
	ExpiresAt  *string
	Status     *string
	RuntimeID  *string
	RuntimeDir *string
}

type NameFilters struct {
	InstanceID string
	StateID    string
	ImageID    string
}

type InstanceFilters struct {
	StateID  string
	ImageID  string
	IDPrefix string
}

type StateFilters struct {
	Kind     string
	ImageID  string
	IDPrefix string
	ParentID string
}

type Store interface {
	ListNames(ctx context.Context, filters NameFilters) ([]NameEntry, error)
	GetName(ctx context.Context, name string) (NameEntry, bool, error)
	ListInstances(ctx context.Context, filters InstanceFilters) ([]InstanceEntry, error)
	GetInstance(ctx context.Context, instanceID string) (InstanceEntry, bool, error)
	ListStates(ctx context.Context, filters StateFilters) ([]StateEntry, error)
	GetState(ctx context.Context, stateID string) (StateEntry, bool, error)
	CreateState(ctx context.Context, entry StateCreate) error
	UpdateStateSize(ctx context.Context, stateID string, sizeBytes int64) error
	CreateInstance(ctx context.Context, entry InstanceCreate) error
	DeleteInstance(ctx context.Context, instanceID string) error
	DeleteState(ctx context.Context, stateID string) error
	Close() error
}
