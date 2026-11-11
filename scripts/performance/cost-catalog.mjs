// CT01: reviewed public source coordinates for the new candidate and actual
// released v1. These are static observer data, never child-supplied paths/names.
// Artifact identity supplies Current's exact revision separately.
const definitions = [
  {"Operation":"prepare.request","Relation":"changed","Current":{"File":"internal/prepare/manager.go","Symbol":"PrepareService.submitForPurpose"},"Released":{"File":"internal/prepare/manager.go","Symbol":"PrepareService.Submit","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"prepare.admission","Relation":"changed","Current":{"File":"internal/prepare/manager.go","Symbol":"PrepareService.submitForPurpose"},"Released":{"File":"internal/prepare/manager.go","Symbol":"PrepareService.Submit","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"liquibase.plan_invoke","Relation":"changed","Current":{"File":"internal/prepare/tracked_executor.go","Symbol":"runTrackedExecutor"},"Released":{"File":"internal/prepare/components.go","Symbol":"PrepareService.runLiquibaseUpdateSQL","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"prepare.worker","Relation":"changed","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"jobCoordinator.runContainerLiquibase"},"Released":{"File":"internal/prepare/manager.go","Symbol":"jobCoordinator.runJob","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"runner.inventory","Relation":"additional","Current":{"File":"internal/runtime/runner_image_inspect.go","Symbol":"DockerRuntime.InspectRunnerImage"},"Released":null},
  {"Operation":"capture.read","Relation":"additional","Current":{"File":"internal/runtime/liquibase_step_capture.go","Symbol":"DockerRuntime.ReadCompleteLiquibaseStepOutput"},"Released":null},
  {"Operation":"capture.prune","Relation":"additional","Current":{"File":"internal/runtime/liquibase_prune.go","Symbol":"DockerRuntime.VerifyLiquibaseCapturePruned"},"Released":null},
  {"Operation":"docker.command","Relation":"shared","Current":{"File":"internal/runtime/docker.go","Symbol":"DockerRuntime.run"},"Released":{"File":"internal/runtime/docker.go","Symbol":"DockerRuntime.run","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"docker.capture_copy","Relation":"additional","Current":{"File":"internal/runtime/session_capture_bundle.go","Symbol":"DockerRuntime.readSessionCaptureBundle"},"Released":null},
  {"Operation":"prepare.planning","Relation":"changed","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.planNormalContainerLiquibase"},"Released":{"File":"internal/prepare/components.go","Symbol":"PrepareService.runLiquibaseUpdateSQL","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"prepare.session_startup","Relation":"changed","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.startNormalLiquibaseSession"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.ensureRuntime","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"prepare.step","Relation":"changed","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.executeNormalLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"liquibase.invoke","Relation":"changed","Current":{"File":"internal/runtime/liquibase_step_capture.go","Symbol":"DockerRuntime.InvokeTrackedLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeLiquibaseStep","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"db.prepare_snapshot","Relation":"shared","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.observeExecuteNormalLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"db.resume.step","Relation":"shifted","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.observeExecuteNormalLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"db.resume.publication","Relation":"shifted","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.publishNormalLiquibaseWorkingInstance"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"db.verify.after_apply","Relation":"shared","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.observeExecuteNormalLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"db.verify.after_resume","Relation":"additional","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.observeExecuteNormalLiquibaseStep"},"Released":null},
  {"Operation":"state.snapshot","Relation":"shared","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.observeExecuteNormalLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"state.size","Relation":"shared","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.observeExecuteNormalLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"state.seal","Relation":"shared","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.observeExecuteNormalLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"journal.state_publish","Relation":"changed","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.observeExecuteNormalLiquibaseStep"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.executeStateTask","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"journal.invocation_plan","Relation":"additional","Current":{"File":"internal/prepare/liquibase_session_coordinator.go","Symbol":"liquibaseSessionCoordinator.invokeStep"},"Released":null},
  {"Operation":"journal.invocation_admit","Relation":"additional","Current":{"File":"internal/prepare/liquibase_session_coordinator.go","Symbol":"liquibaseSessionCoordinator.invokeStep"},"Released":null},
  {"Operation":"journal.invocation_complete","Relation":"additional","Current":{"File":"internal/prepare/liquibase_session_coordinator.go","Symbol":"liquibaseSessionCoordinator.invokeStep"},"Released":null},
  {"Operation":"prepare.publication","Relation":"changed","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.publishNormalLiquibaseResult"},"Released":{"File":"internal/prepare/execution.go","Symbol":"taskExecutor.createInstance","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
  {"Operation":"prepare.final_cleanup","Relation":"changed","Current":{"File":"internal/prepare/liquibase_normal_worker.go","Symbol":"PrepareService.finishNormalLiquibase"},"Released":{"File":"internal/prepare/manager.go","Symbol":"jobCoordinator.runJob","Revision":"cab99c4c7ced5b18f4dc43a65b002d84d8fef02d"}},
];

// TL04–TL07: exact call-site correspondence. These rows identify code; their
// inclusive time is not added to child operations or asserted equal to v1 time.
const timelineSites = [
  ['plan.release', 'liquibase_normal_worker.go', 'PrepareService.observePlanNormalContainerLiquibase'],
  ['db.source_prepare', 'managed_execution.go', 'taskExecutor.observeStartManagedRuntimeWithPlan', 'shared', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['session.plan', 'liquibase_normal_worker.go', 'PrepareService.observeStartNormalLiquibaseSession'],
  ['session.input_prepare', 'liquibase_normal_worker.go', 'PrepareService.observeStartNormalLiquibaseSession'],
  ['step.status', 'liquibase_normal_worker.go', 'jobCoordinator.runObservedContainerLiquibase', 'changed', 'execution.go', 'taskExecutor.executeStateTask'],
  ['runner.stop', 'liquibase_session_coordinator.go', 'liquibaseSessionCoordinator.stopRunner'],
  ['admission.inputs', 'manager.go', 'PrepareService.submitObservedForPurpose', 'changed', 'manager.go', 'PrepareService.Submit'],
  ['admission.image', 'manager.go', 'PrepareService.submitObservedForPurpose', 'changed', 'manager.go', 'PrepareService.Submit'],
  ['admission.identity', 'manager.go', 'PrepareService.submitObservedForPurpose', 'shared', 'manager.go', 'PrepareService.Submit'],
  ['admission.commit', 'liquibase_container_binding.go', 'PrepareService.admitContainerLiquibase', 'changed', 'manager.go', 'PrepareService.Submit'],
  ['db.assign', 'managed_execution.go', 'taskExecutor.observeStartManagedRuntimeWithPlan', 'shared', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['publication.activate', 'liquibase_normal_worker.go', 'PrepareService.publishNormalLiquibaseWorkingInstance', 'changed', 'execution.go', 'taskExecutor.createInstance'],
  ['publication.commit', 'liquibase_normal_worker.go', 'PrepareService.publishNormalLiquibaseWorkingInstance', 'changed', 'execution.go', 'taskExecutor.createInstance'],
  ['private.dispose', 'liquibase_normal_worker.go', 'PrepareService.publishNormalLiquibaseWorkingInstance'],
  ['binding.prepare', 'liquibase_container_binding.go', 'PrepareService.bindContainerLiquibase'],
  ['projection.wait', 'liquibase_projection_gate.go', 'PrepareService.withLiquibaseProjectionGate'],
  ['input.publish', 'liquibase_container_binding.go', 'PrepareService.admitContainerLiquibase'],
  ['input.materialize', 'liquibase_container_binding.go', 'PrepareService.bindFreshContainerLiquibase'],
  ['runner.resolve', 'liquibase_container_binding.go', 'PrepareService.bindFreshContainerLiquibase'],
  ['runner.ensure', 'liquibase_container_binding.go', 'PrepareService.bindFreshContainerLiquibase'],
  ['driver.materialize', 'liquibase_container_binding.go', 'PrepareService.bindFreshContainerLiquibase'],
  ['binding.commit', 'liquibase_container_binding.go', 'PrepareService.bindFreshContainerLiquibase'],
  ['plan.launch_prepare', 'liquibase_normal_worker.go', 'PrepareService.observePlanNormalContainerLiquibase'],
  ['plan.input_prepare', 'planning_executor_request.go', 'PrepareService.buildPlanningExecutorRequest'],
  ['plan.capture_reconcile', 'liquibase_normal_worker.go', 'PrepareService.observePlanNormalContainerLiquibase'],
  ['plan.dispose', 'liquibase_normal_worker.go', 'PrepareService.observePlanNormalContainerLiquibase', 'changed', 'execution.go', 'PrepareService.cleanupRuntime'],
  ['plan.suffix', 'liquibase_normal_worker.go', 'jobCoordinator.runObservedContainerLiquibase', 'changed', 'manager.go', 'jobCoordinator.buildPlanLiquibase'],
  ['db.startup', 'managed_execution.go', 'taskExecutor.startManagedRuntimeWithPlan', 'changed', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['db.lock_wait', 'managed_execution.go', 'taskExecutor.observeStartManagedRuntimeWithPlan', 'shared', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['db.base_init', 'managed_execution.go', 'taskExecutor.observeStartManagedRuntimeWithPlan', 'shared', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['db.clone', 'managed_execution.go', 'taskExecutor.observeStartManagedRuntimeWithPlan', 'shared', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['db.start', 'managed_execution.go', 'taskExecutor.observeStartManagedRuntimeWithPlan', 'changed', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['db.attach', 'managed_execution.go', 'taskExecutor.observeStartManagedRuntimeWithPlan', 'shared', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['db.verify.startup', 'managed_execution.go', 'taskExecutor.observeStartManagedRuntimeWithPlan', 'shared', 'managed_execution.go', 'taskExecutor.startManagedRuntime'],
  ['session.runner_start', 'liquibase_normal_worker.go', 'PrepareService.observeStartNormalLiquibaseSession', 'changed', 'execution.go', 'taskExecutor.executeLiquibaseStep'],
  ['session.readiness', 'liquibase_normal_worker.go', 'PrepareService.observeStartNormalLiquibaseSession'],
  ['step.bind', 'liquibase_normal_worker.go', 'jobCoordinator.runObservedContainerLiquibase'],
  ['projection.cleanup', 'liquibase_normal_worker.go', 'PrepareService.finishObservedNormalLiquibase'],
  ['finalization.confirm', 'liquibase_normal_worker.go', 'PrepareService.finishObservedNormalLiquibase'],
  ['finalization.commit', 'liquibase_normal_worker.go', 'PrepareService.finishObservedNormalLiquibase', 'changed', 'manager.go', 'jobCoordinator.runJob'],
];
for (const [Operation, file, Symbol, relation, releasedFile, releasedSymbol] of timelineSites) {
  definitions.push({ Operation, Relation: relation ?? 'additional', Current: { File: `internal/prepare/${file}`, Symbol },
    Released: releasedFile ? { File: `internal/prepare/${releasedFile}`, Symbol: releasedSymbol, Revision: 'cab99c4c7ced5b18f4dc43a65b002d84d8fef02d' } : null });
}

/** Every caller owns its catalog, including nested source-coordinate objects. */
export function costCatalog() { return structuredClone(definitions); }

/** Unknown keys cannot acquire source identity or echo arbitrary/private text. */
export function costDefinition(operation) {
  const definition = definitions.find(item => item.Operation === operation);
  if (!definition) throw new Error('Unknown cost source key');
  return structuredClone(definition);
}
