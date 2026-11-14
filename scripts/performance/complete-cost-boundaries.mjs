const common = ['prepare.request','prepare.admission','prepare.worker','admission.inputs','admission.image','admission.identity','admission.commit',
  'input.publish','binding.prepare','input.materialize','runner.resolve','runner.ensure','runner.inventory_lookup','binding.commit',
  'prepare.planning','liquibase.plan_invoke','plan.launch_prepare','plan.input_prepare','plan.capture_reconcile','plan.retain','plan.release','plan.suffix',
  'prepare.publication','prepare.final_cleanup','projection.cleanup','finalization.confirm','finalization.commit'];
const database = ['db.startup','db.lock_wait','db.source_prepare','db.assign','db.clone','db.start','db.attach','db.verify.startup'];
const working = ['prepare.session_startup','session.plan','session.runner_start','session.readiness','runner.stop','publication.activate','publication.commit','private.dispose'];
const applying = ['prepare.step','liquibase.invoke','capture.result_decode','journal.invocation_plan','journal.invocation_admit','journal.invocation_complete',
  'db.prepare_snapshot','db.verify.after_apply','state.snapshot','state.size','state.seal','journal.state_publish'];
const invalid = () => { throw new Error('Incomplete timeline source boundaries'); };

/** TL04–TL07/TL11: gate the actual campaign after full artifact/tree/timer
 * validation, independently of exact ledger arithmetic. A balanced report
 * with a missing required call cannot qualify as the complete experiment.
 * Native tests separately prove these keys surround real effect boundaries.
 */
export function qualifyCompleteCostBoundaries(report, changes) {
  try {
    if (!report || !['uncached','cached'].includes(report.Pass) || !Array.isArray(report.SourceSpans) ||
      !Number.isSafeInteger(changes) || changes <= 0) invalid();
    const cached = report.Pass === 'cached';
    const count = (operation, phase, step) => report.SourceSpans.filter(s => s.Operation === operation &&
      (phase === undefined || s.Phase === phase) && (step === undefined || s.Step === step)).length;
    for (const operation of common) if (count(operation) !== 1) invalid();
    if (count('runner.inventory') !== (cached ? 0 : 1)) invalid();
    if (count('plan.dispose') !== (cached ? 1 : 0) || count('plan.db_transfer') !== (cached ? 0 : 1)) invalid();
    for (const operation of database) {
      if (count(operation) !== (cached ? 2 : 1) || count(operation, 'planning', 0) !== 1 || count(operation, cached ? 'publication' : 'startup', 0) !== (cached ? 1 : 0)) invalid();
    }
    for (const operation of working) if (count(operation) !== (cached ? 0 : 1)) invalid();
    if (count('session.input_prepare') !== (cached ? 0 : 2)) invalid();
    for (const operation of ['step.bind','step.status',...applying]) {
      const expected = ['step.bind','step.status'].includes(operation) || !cached ? 1 : 0;
      if (count(operation) !== changes*expected) invalid();
      for (let step = 1; step <= changes; step++) if (count(operation, 'step', step) !== expected) invalid();
    }
    return true;
  } catch { invalid(); }
}
