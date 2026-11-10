// Package preparecost attributes observed prepare time to closed, source-linked
// operations. Measurements never grant execution, publication or cleanup rights.
// Requirements: docs/architecture/issue89-cost-attribution-and-trimming.md.
package preparecost

// Operation is an engine-owned source key, not a request/log-derived string.
type Operation string

const (
	Worker         Operation = "prepare.worker"
	ImageInventory Operation = "runner.inventory"
	CaptureRead    Operation = "capture.read"
	PruneObserve   Operation = "capture.prune"
	DockerCommand  Operation = "docker.command"
	CaptureCopy    Operation = "docker.capture_copy"
	Planning       Operation = "prepare.planning"
	SessionStartup Operation = "prepare.session_startup"
	StepExecution  Operation = "prepare.step"
)

// Relation describes code correspondence, not an inferred timing difference.
type Relation string

const (
	Shared     Relation = "shared"
	Changed    Relation = "changed"
	Shifted    Relation = "shifted"
	Additional Relation = "additional"
	Unresolved Relation = "unresolved"
)

// Location uses repository-relative source coordinates. It contains no runtime
// path/identity. Artifact provenance supplies the exact source revision.
type Location struct{ File, Symbol string }

// Definition connects a stable operation key to both implementations. An empty
// Released location is unavailable/not applicable, never a zero-duration claim.
type Definition struct {
	Operation Operation
	Relation  Relation
	Current   Location
	Released  Location
}

// Definitions returns caller-owned values. The catalog grows only alongside
// tested instrumentation sites; no caller can extend it with private data.
func Definitions() []Definition {
	return []Definition{
		{Worker, Changed, Location{"internal/prepare/liquibase_normal_worker.go", "jobCoordinator.runContainerLiquibase"}, Location{"internal/prepare/manager.go", "jobCoordinator.runJob"}},
		{ImageInventory, Additional, Location{"internal/runtime/runner_image_inspect.go", "DockerRuntime.InspectRunnerImage"}, Location{}},
		{CaptureRead, Additional, Location{"internal/runtime/liquibase_step_capture.go", "DockerRuntime.ReadCompleteLiquibaseStepOutput"}, Location{}},
		{PruneObserve, Additional, Location{"internal/runtime/liquibase_prune.go", "DockerRuntime.VerifyLiquibaseCapturePruned"}, Location{}},
		{DockerCommand, Shared, Location{"internal/runtime/docker.go", "DockerRuntime.run"}, Location{"internal/runtime/docker.go", "DockerRuntime.run"}},
		{CaptureCopy, Additional, Location{"internal/runtime/session_capture_bundle.go", "DockerRuntime.readSessionCaptureBundle"}, Location{}},
		{Planning, Changed, Location{"internal/prepare/liquibase_normal_worker.go", "PrepareService.planNormalContainerLiquibase"}, Location{"internal/prepare/components.go", "PrepareService.runLiquibaseUpdateSQL"}},
		{SessionStartup, Changed, Location{"internal/prepare/liquibase_normal_worker.go", "PrepareService.startNormalLiquibaseSession"}, Location{"internal/prepare/execution.go", "taskExecutor.ensureRuntime"}},
		{StepExecution, Changed, Location{"internal/prepare/liquibase_normal_worker.go", "PrepareService.executeNormalLiquibaseStep"}, Location{"internal/prepare/execution.go", "taskExecutor.executeStateTask"}},
	}
}

// Describe refuses unknown keys rather than echoing arbitrary input in a report.
func Describe(operation Operation) (Definition, bool) {
	for _, definition := range Definitions() {
		if definition.Operation == operation {
			return definition, true
		}
	}
	return Definition{}, false
}
