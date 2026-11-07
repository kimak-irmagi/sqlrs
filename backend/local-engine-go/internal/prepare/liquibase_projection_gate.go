package prepare

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// liquibaseProjectionGate serializes one job's short allocation/finalization
// work, never its migration chain. Cancellation admission does not acquire it.
// Lock order: job gate, instance-access fence, SQL (cleanup design, LC20).
type liquibaseProjectionGate struct {
	turn chan struct{}
	refs int
}

// projectionServiceAuthority resolves the current queue at each boundary. The
// owner itself remains immutable across disjoint job gates; rewriting a shared
// interface field on every lookup would race another job's filesystem work.
type projectionServiceAuthority struct{ service *PrepareService }

func (a projectionServiceAuthority) journal() (projectionAuthority, error) {
	journal, ok := a.service.queue.(projectionAuthority)
	if !ok {
		return nil, errLiquibaseProjection
	}
	return journal, nil
}
func (a projectionServiceAuthority) CheckLiquibaseProjectionAllocation(ctx context.Context, id string) error {
	journal, err := a.journal()
	if err != nil {
		return err
	}
	return journal.CheckLiquibaseProjectionAllocation(ctx, id)
}
func (a projectionServiceAuthority) CheckLiquibaseProjectionResume(ctx context.Context, id string) error {
	journal, err := a.journal()
	if err != nil {
		return err
	}
	return journal.CheckLiquibaseProjectionResume(ctx, id)
}
func (a projectionServiceAuthority) CheckLiquibaseProjectionCleanup(ctx context.Context, intent queue.FinalizationSnapshot) error {
	journal, err := a.journal()
	if err != nil {
		return err
	}
	return journal.CheckLiquibaseProjectionCleanup(ctx, intent)
}

func (m *PrepareService) withLiquibaseProjectionGate(ctx context.Context, jobID string, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.projectionMu.Lock()
	if m.projectionGates == nil {
		m.projectionGates = make(map[string]*liquibaseProjectionGate)
	}
	gate := m.projectionGates[jobID]
	if gate == nil {
		gate = &liquibaseProjectionGate{turn: make(chan struct{}, 1)}
		m.projectionGates[jobID] = gate
	}
	gate.refs++
	m.projectionMu.Unlock()
	defer func() {
		m.projectionMu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(m.projectionGates, jobID)
		}
		m.projectionMu.Unlock()
	}()
	select {
	case gate.turn <- struct{}{}:
		defer func() { <-gate.turn }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

// localLiquibaseProjectionOwner binds a service to one installation root.
// Changing configuration must not abandon locators owned by an earlier call.
func (m *PrepareService) localLiquibaseProjectionOwner(allocate bool) (*liquibaseProjectionOwner, error) {
	m.projectionMu.Lock()
	defer m.projectionMu.Unlock()
	if m.containerLiquibase == nil {
		return nil, errLiquibaseProjection
	}
	root := m.containerLiquibase.MaterializedRoot
	if !filepath.IsAbs(root) {
		return nil, errLiquibaseProjection
	}
	root = filepath.Clean(root)
	_, ok := m.queue.(projectionAuthority)
	if !ok {
		return nil, errLiquibaseProjection
	}
	if m.projectionOwner != nil {
		if m.projectionOwner.root != root {
			return nil, errLiquibaseProjection
		}
		return m.projectionOwner, nil
	}
	if err := projectionDirectory(root, allocate); err != nil && (allocate || !errors.Is(err, os.ErrNotExist)) {
		return nil, err
	}
	owner := &liquibaseProjectionOwner{root: root, authority: projectionServiceAuthority{service: m}}
	m.projectionOwner = owner
	return owner, nil
}
