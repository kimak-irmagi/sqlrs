package prepare

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// buildPlanningLaunch derives one initial preview-clone and capture identity
// from the already saved adapter binding. Publishing the helper bytes is safe
// before the journal transaction; the transaction leases them before any
// external runtime action. See P01/C01a in the planning recovery design.
func (m *PrepareService) buildPlanningLaunch(ctx context.Context, jobID string,
	prepared preparedRequest, record queue.LiquibaseExecutionRecord,
	artifacts inputbundle.Store) (queue.PlanningLaunch, error) {
	if m == nil || jobID == "" || prepared.request.PrepareKind != "lb" ||
		prepared.liquibaseFingerprint != record.ExecutionFingerprint ||
		prepared.liquibaseImageID != "liquibase/liquibase@"+record.ImageManifestDigest ||
		record.PolicyRevision != liquibaseContainerFingerprintDomain {
		return queue.PlanningLaunch{}, fmt.Errorf("saved Liquibase planning binding is unavailable")
	}
	if prepared.liquibaseProjection == nil || prepared.liquibaseProjection.job.jobID != jobID {
		return queue.PlanningLaunch{}, errLiquibaseProjection
	}
	helperDigest, err := publishPlanningHelper(ctx, artifacts)
	if err != nil {
		return queue.PlanningLaunch{}, err
	}
	updateArgs := replaceLiquibaseCommand(prepared.normalizedArgs, "updateSQL")
	encodedArgs, err := json.Marshal(updateArgs)
	if err != nil {
		return queue.PlanningLaunch{}, err
	}
	binding, err := queue.ProjectLiquibasePlanningBinding(record, string(encodedArgs), helperDigest)
	if err != nil {
		return queue.PlanningLaunch{}, err
	}
	cloneID, err := randomHex(12)
	if err != nil {
		return queue.PlanningLaunch{}, err
	}
	protectedRef, err := prepared.liquibaseProjection.protectedRef(cloneID)
	if err != nil {
		return queue.PlanningLaunch{}, err
	}
	previewOwnerID := jobID + "-plan"
	sourceKind, sourceID := "image", prepared.effectiveImageID()
	if prepared.managed.Key != "" {
		sourceID = prepared.managed.Key
	}
	if sourceID == "" {
		return queue.PlanningLaunch{}, fmt.Errorf("resolved planning source is unavailable")
	}
	clone := queue.ClonePlan{
		ID: cloneID, Generation: 1, SourceKind: sourceKind, SourceImageID: sourceID,
		Ref: filepath.ToSlash(filepath.Join("jobs", previewOwnerID,
			"runtime_"+managedOperationRef(m.stateStoreRoot, previewOwnerID, sourceKind, sourceID))),
		PlannedRuntimeName: "sqlrs-db-" + cloneID,
	}
	now := time.Now
	if m.now != nil {
		now = m.now
	}
	return queue.PlanningLaunch{
		BoundLaunch: queue.BoundLaunch{
			Attempt: queue.AttemptPlan{JobID: jobID, TaskID: "plan", CloneID: cloneID,
				ExecutionDigest: binding.Digest, WorkerGeneration: 1,
				CreatedAt: now().UTC().Format(time.RFC3339Nano)},
			Binding: binding, Clone: clone,
			Leases: []queue.ArtifactLease{{ID: "capture-helper-" + cloneID,
				Kind: "capture_helper", Digest: helperDigest}},
		},
		Capture: queue.PlanningCaptureNames{
			ProtectedInput: protectedRef,
			Stdout:         "plan-" + cloneID + "-stdout",
			Stderr:         "plan-" + cloneID + "-stderr",
			Manifest:       "plan-" + cloneID + "-manifest",
			HelperDigest:   helperDigest, ProtocolRevision: engineRuntime.LiquibaseCaptureProtocol,
		},
	}, nil
}
