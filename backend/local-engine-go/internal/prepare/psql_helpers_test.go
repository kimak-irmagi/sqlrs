package prepare

import "testing"

func psqlOutputStateID(t *testing.T, mgr *PrepareService, prepared preparedRequest, input TaskInput) string {
	t.Helper()
	digest, err := computePsqlContentDigest(prepared.psqlInputs, prepared.psqlWorkDir)
	if err != nil {
		t.Fatalf("computePsqlContentDigest: %v", err)
	}
	taskHash := psqlTaskHash(prepared.request.PrepareKind, digest.hash, mgr.version)
	outputID, _ := mgr.computeOutputStateID(input.Kind, input.ID, taskHash)
	return outputID
}

func psqlOutputStateIDForStep(t *testing.T, mgr *PrepareService, prepared preparedRequest, input TaskInput, taskID string) string {
	t.Helper()
	step, err := psqlStepForTask(prepared.psqlSteps, taskID)
	if err != nil {
		t.Fatalf("psqlStepForTask: %v", err)
	}
	digest, err := computePsqlContentDigest(step.inputs, prepared.psqlWorkDir)
	if err != nil {
		t.Fatalf("computePsqlContentDigest: %v", err)
	}
	taskHash := psqlTaskHash(prepared.request.PrepareKind, digest.hash, mgr.version)
	outputID, _ := mgr.computeOutputStateID(input.Kind, input.ID, taskHash)
	return outputID
}
