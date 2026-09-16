package prepare

import (
	"context"
	"strings"

	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// bindManagedRequest selects metadata before hashing/publication and restores a
// queued job by exact reference without a reservation fallback. The owner is a
// required dependency of this managed path; startup wiring remains coordinated
// with runtime/access cutover. See managed-database-identity-internals.md.
func (m *PrepareService) bindManagedRequest(ctx context.Context, prepared *preparedRequest, job *queue.JobRecord) error {
	if m.identity == nil || prepared == nil {
		return managedidentity.ErrUnavailable
	}
	image := prepared.effectiveImageID()
	if at := strings.LastIndexByte(image, '@'); at >= 0 {
		image = image[at+1:]
	}
	var selected managedidentity.BaseSelection
	var err error
	if job != nil {
		selected, err = m.identity.RestoreStored(ctx, image, job.LineageRef, job.IdentityDigest)
	} else if prepared.managed.IdentityDigest != "" {
		selected, err = m.identity.RestoreBase(ctx, image, prepared.managed.IdentityBinding)
	} else {
		selected, err = m.identity.ResolveBase(ctx, image)
	}
	if err != nil {
		return err
	}
	prepared.managed = selected
	return nil
}

// isManagedStateCached checks identity as well as key existence. Corrupt cache
// metadata is an error; it cannot be treated as a miss and silently overwritten.
func (m *PrepareService) isManagedStateCached(stateID string, prepared preparedRequest) (bool, error) {
	if m.identity == nil {
		return m.isStateCached(stateID)
	}
	if prepared.managed.Validate() != nil {
		return false, managedidentity.ErrInvalid
	}
	entry, found, err := m.store.GetState(context.Background(), stateID)
	if err != nil || !found {
		return false, err
	}
	if entry.LineageRef != prepared.managed.LineageRef || entry.IdentityDigest != prepared.managed.IdentityDigest || entry.ImageID != prepared.effectiveImageID() {
		return false, managedidentity.ErrInvalid
	}
	if m.access != nil {
		if err := m.access.CheckSeal(context.Background(), stateID, prepared.managed.IdentityBinding); err != nil {
			return false, err
		}
	}
	return true, nil
}

// validateManagedTasks rejects corrupted recovery metadata before cache access or
// replanning can turn it into a different lineage. Each persisted hash must link
// from the selected base through the complete state chain (identity internals).
func (m *PrepareService) validateManagedTasks(prepared preparedRequest, tasks []queue.TaskRecord) error {
	if m.identity == nil {
		return nil
	}
	if prepared.managed.Validate() != nil || prepared.managed.Key == "" {
		return managedidentity.ErrInvalid
	}
	kind, input := "image", prepared.managed.Key
	count := 0
	for _, task := range tasks {
		if task.Type == "resolve_image" && valueOrEmpty(task.ResolvedImageID) != prepared.effectiveImageID() {
			return managedidentity.ErrInvalid
		}
		if task.Type != "state_execute" {
			continue
		}
		if valueOrEmpty(task.InputKind) != kind || valueOrEmpty(task.InputID) != input || valueOrEmpty(task.TaskHash) == "" {
			return managedidentity.ErrInvalid
		}
		output, err := m.computeOutputStateID(kind, input, valueOrEmpty(task.TaskHash))
		if err != nil || output != valueOrEmpty(task.OutputStateID) {
			return managedidentity.ErrInvalid
		}
		kind, input = "state", output
		count++
	}
	if count == 0 {
		return managedidentity.ErrInvalid
	}
	return nil
}
