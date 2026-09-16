package deletion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sqlrs/engine-local/internal/conntrack"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/statefs"
	"github.com/sqlrs/engine-local/internal/store"
)

const (
	OutcomeDeleted     = "deleted"
	OutcomeWouldDelete = "would_delete"
	OutcomeBlocked     = "blocked"

	BlockActiveConnections = "active_connections"
	BlockActiveTasks       = "active_tasks"
	BlockHasDescendants    = "has_descendants"
	BlockBlockedDescendant = "blocked_by_descendant"
)

type Options struct {
	Access         *instanceaccess.Service
	Store          store.Store
	Conn           conntrack.Tracker
	Runtime        runtime.Runtime
	StateFS        statefs.StateFS
	StateStoreRoot string
}

type Manager struct {
	access         *instanceaccess.Service
	store          store.Store
	conn           conntrack.Tracker
	runtime        runtime.Runtime
	statefs        statefs.StateFS
	stateStoreRoot string
}

var (
	removeRuntimeDirAll = os.RemoveAll
	renameRuntimeDir    = os.Rename
	nowUnixNano         = func() int64 { return time.Now().UnixNano() }
)

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
	RuntimeDir  *string      `json:"-"`
	ImageID     *string      `json:"-"`
	Children    []DeleteNode `json:"children,omitempty"`
}

func NewManager(opts Options) (*Manager, error) {
	if opts.Store == nil {
		return nil, storeError("store is required")
	}
	tracker := opts.Conn
	if tracker == nil {
		tracker = conntrack.Noop{}
	}
	return &Manager{
		access:         opts.Access,
		store:          opts.Store,
		conn:           tracker,
		runtime:        opts.Runtime,
		statefs:        opts.StateFS,
		stateStoreRoot: strings.TrimSpace(opts.StateStoreRoot),
	}, nil
}

func (m *Manager) DeleteInstance(ctx context.Context, instanceID string, opts DeleteOptions) (DeleteResult, bool, error) {
	entry, ok, err := m.store.GetInstance(ctx, instanceID)
	if err != nil {
		return DeleteResult{}, false, err
	}
	if !ok {
		return DeleteResult{}, false, nil
	}

	connections, err := m.conn.ActiveConnections(ctx, instanceID)
	if err != nil {
		return DeleteResult{}, true, err
	}
	node := DeleteNode{
		Kind:        "instance",
		ID:          instanceID,
		Connections: &connections,
		RuntimeID:   entry.RuntimeID,
		RuntimeDir:  entry.RuntimeDir,
	}
	blocked := false
	if connections > 0 && !opts.Force {
		node.Blocked = BlockActiveConnections
		blocked = true
	}

	result := DeleteResult{
		DryRun:  opts.DryRun,
		Outcome: outcomeFor(blocked, opts.DryRun),
		Root:    node,
	}
	if blocked || opts.DryRun {
		return result, true, nil
	}
	if m.access != nil {
		err := m.access.RetireBound(ctx, instanceID, func(binding instanceaccess.AccessBinding) error {
			if err := m.stopManagedRuntime(ctx, entry.RuntimeID, binding); err != nil {
				return err
			}
			if err := m.removeRuntimeDir(entry.RuntimeDir); err != nil {
				return err
			}
			return m.store.DeleteInstance(ctx, instanceID)
		})
		return result, true, err
	}
	if err := m.stopRuntime(ctx, entry.RuntimeID); err != nil {
		return DeleteResult{}, true, err
	}
	if err := m.removeRuntimeDir(entry.RuntimeDir); err != nil {
		return DeleteResult{}, true, err
	}
	if err := m.store.DeleteInstance(ctx, instanceID); err != nil {
		return DeleteResult{}, true, err
	}
	return result, true, nil
}

func (m *Manager) DeleteState(ctx context.Context, stateID string, opts DeleteOptions) (DeleteResult, bool, error) {
	entry, ok, err := m.store.GetState(ctx, stateID)
	if err != nil {
		return DeleteResult{}, false, err
	}
	if !ok {
		return DeleteResult{}, false, nil
	}

	if !opts.Recurse {
		hasDescendants, err := m.hasDescendants(ctx, stateID)
		if err != nil {
			return DeleteResult{}, true, err
		}
		node := DeleteNode{
			Kind:    "state",
			ID:      stateID,
			ImageID: strPtr(entry.ImageID),
		}
		if hasDescendants {
			node.Blocked = BlockHasDescendants
			return DeleteResult{
				DryRun:  opts.DryRun,
				Outcome: OutcomeBlocked,
				Root:    node,
			}, true, nil
		}

		result := DeleteResult{
			DryRun:  opts.DryRun,
			Outcome: outcomeFor(false, opts.DryRun),
			Root:    node,
		}
		if opts.DryRun {
			return result, true, nil
		}
		if err := m.removeStateDir(node.ImageID, node.ID); err != nil {
			return DeleteResult{}, true, err
		}
		if err := m.store.DeleteState(ctx, stateID); err != nil {
			return DeleteResult{}, true, err
		}
		return result, true, nil
	}

	tree, blocked, err := m.buildStateNode(ctx, stateID, opts)
	if err != nil {
		return DeleteResult{}, true, err
	}
	result := DeleteResult{
		DryRun:  opts.DryRun,
		Outcome: outcomeFor(blocked, opts.DryRun),
		Root:    tree,
	}
	if blocked || opts.DryRun {
		return result, true, nil
	}
	if err := m.deleteTree(ctx, tree); err != nil {
		return DeleteResult{}, true, err
	}
	return result, true, nil
}

func (m *Manager) hasDescendants(ctx context.Context, stateID string) (bool, error) {
	instances, err := m.store.ListInstances(ctx, store.InstanceFilters{StateID: stateID})
	if err != nil {
		return false, err
	}
	if len(instances) > 0 {
		return true, nil
	}
	children, err := m.store.ListStates(ctx, store.StateFilters{ParentID: stateID})
	if err != nil {
		return false, err
	}
	return len(children) > 0, nil
}

func (m *Manager) buildStateNode(ctx context.Context, stateID string, opts DeleteOptions) (DeleteNode, bool, error) {
	entry, ok, err := m.store.GetState(ctx, stateID)
	if err != nil {
		return DeleteNode{}, false, err
	}
	if !ok {
		return DeleteNode{}, false, storeError("state not found")
	}
	node := DeleteNode{
		Kind:    "state",
		ID:      stateID,
		ImageID: strPtr(entry.ImageID),
	}

	blocked := false
	instances, err := m.store.ListInstances(ctx, store.InstanceFilters{StateID: stateID})
	if err != nil {
		return DeleteNode{}, false, err
	}
	for _, entry := range instances {
		connections, err := m.conn.ActiveConnections(ctx, entry.InstanceID)
		if err != nil {
			return DeleteNode{}, false, err
		}
		child := DeleteNode{
			Kind:        "instance",
			ID:          entry.InstanceID,
			Connections: &connections,
			RuntimeID:   entry.RuntimeID,
			RuntimeDir:  entry.RuntimeDir,
		}
		if connections > 0 && !opts.Force {
			child.Blocked = BlockActiveConnections
			blocked = true
		}
		node.Children = append(node.Children, child)
	}

	children, err := m.store.ListStates(ctx, store.StateFilters{ParentID: stateID})
	if err != nil {
		return DeleteNode{}, false, err
	}
	for _, entry := range children {
		childNode, childBlocked, err := m.buildStateNode(ctx, entry.StateID, opts)
		if err != nil {
			return DeleteNode{}, false, err
		}
		if childBlocked {
			blocked = true
		}
		node.Children = append(node.Children, childNode)
	}

	if blocked {
		node.Blocked = BlockBlockedDescendant
	}
	return node, blocked, nil
}

func (m *Manager) deleteTree(ctx context.Context, node DeleteNode) error {
	for _, child := range node.Children {
		if err := m.deleteTree(ctx, child); err != nil {
			return err
		}
	}
	switch node.Kind {
	case "instance":
		if m.access != nil {
			return m.access.RetireBound(ctx, node.ID, func(binding instanceaccess.AccessBinding) error {
				if err := m.stopManagedRuntime(ctx, node.RuntimeID, binding); err != nil {
					return err
				}
				if err := m.removeRuntimeDir(node.RuntimeDir); err != nil {
					return err
				}
				return m.store.DeleteInstance(ctx, node.ID)
			})
		}
		if err := m.stopRuntime(ctx, node.RuntimeID); err != nil {
			return err
		}
		if err := m.removeRuntimeDir(node.RuntimeDir); err != nil {
			return err
		}
		return m.store.DeleteInstance(ctx, node.ID)
	case "state":
		if err := m.removeStateDir(node.ImageID, node.ID); err != nil {
			return err
		}
		return m.store.DeleteState(ctx, node.ID)
	default:
		return nil
	}
}

func outcomeFor(blocked bool, dryRun bool) string {
	if blocked {
		return OutcomeBlocked
	}
	if dryRun {
		return OutcomeWouldDelete
	}
	return OutcomeDeleted
}

func (m *Manager) stopRuntime(ctx context.Context, runtimeID *string) error {
	if m.runtime == nil || runtimeID == nil {
		return nil
	}
	id := strings.TrimSpace(*runtimeID)
	if id == "" {
		return nil
	}
	if err := m.runtime.Stop(ctx, id); err != nil {
		var unavailable runtime.DockerUnavailableError
		if errors.As(err, &unavailable) {
			return nil
		}
		return err
	}
	return nil
}

func (m *Manager) stopManagedRuntime(ctx context.Context, id *string, b instanceaccess.AccessBinding) error {
	if id == nil || *id != b.RuntimeRef {
		return instanceaccess.ErrConflict
	}
	stopper, ok := m.runtime.(interface {
		StopManaged(context.Context, managedidentity.RuntimeBinding) error
	})
	if !ok {
		return instanceaccess.ErrUnavailable
	}
	return stopper.StopManaged(ctx, b.RuntimeBinding)
}

func (m *Manager) removeRuntimeDir(runtimeDir *string) error {
	if runtimeDir == nil {
		return nil
	}
	dir := strings.TrimSpace(*runtimeDir)
	if dir == "" {
		return nil
	}
	var removeErr error
	if m.statefs != nil {
		if err := m.statefs.RemovePath(context.Background(), dir); err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		} else {
			removeErr = err
		}
	}
	if err := removeRuntimeDirAll(dir); err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	} else if removeErr == nil {
		removeErr = err
	}

	staleDir := fmt.Sprintf("%s.stale-%d", dir, nowUnixNano())
	if err := renameRuntimeDir(dir, staleDir); err == nil {
		_ = removeRuntimeDirAll(staleDir)
		return nil
	} else if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return removeErr
}

func (m *Manager) removeStateDir(imageID *string, stateID string) error {
	if strings.TrimSpace(m.stateStoreRoot) == "" {
		return nil
	}
	if strings.TrimSpace(stateID) == "" {
		return nil
	}
	if m.statefs == nil {
		return nil
	}
	img := ""
	if imageID != nil {
		img = *imageID
	}
	path, err := m.statefs.StateDir(m.stateStoreRoot, img, stateID)
	if err != nil {
		return err
	}
	return m.statefs.RemovePath(context.Background(), path)
}

func strPtr(value string) *string {
	return &value
}

type storeError string

func (e storeError) Error() string {
	return string(e)
}
