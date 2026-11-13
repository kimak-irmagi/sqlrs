import test from 'node:test';
import assert from 'node:assert/strict';
import { qualifyCompleteCostBoundaries } from './complete-cost-boundaries.mjs';

const common = ['prepare.request','prepare.admission','prepare.worker','admission.inputs','admission.image','admission.identity','admission.commit',
  'input.publish','binding.prepare','input.materialize','runner.resolve','runner.ensure','runner.inventory_lookup','binding.commit',
  'prepare.planning','liquibase.plan_invoke','plan.launch_prepare','plan.input_prepare','plan.capture_reconcile','plan.retain','plan.release','plan.suffix',
  'prepare.publication','prepare.final_cleanup','projection.cleanup','finalization.confirm','finalization.commit'];
const database = ['db.startup','db.lock_wait','db.source_prepare','db.assign','db.clone','db.start','db.attach','db.verify.startup'];
const working = ['prepare.session_startup','session.plan','session.runner_start','session.readiness','runner.stop','publication.activate','publication.commit','private.dispose'];
const applying = ['prepare.step','liquibase.invoke','capture.result_decode','journal.invocation_plan','journal.invocation_admit','journal.invocation_complete',
  'db.prepare_snapshot','db.verify.after_apply','state.snapshot','state.size','state.seal','journal.state_publish'];
function fixture(cached = false) {
  const SourceSpans = common.map(Operation => ({ Operation, Phase: 'worker', Step: 0 }));
  if (!cached) SourceSpans.push({ Operation: 'runner.inventory', Phase: 'worker', Step: 0 });
  for (const Operation of database) for (const Phase of cached ? ['planning','publication'] : ['planning']) SourceSpans.push({ Operation, Phase, Step: 0 });
  SourceSpans.push({ Operation: cached ? 'plan.dispose' : 'plan.db_transfer', Phase: cached ? 'publication' : 'startup', Step: 0 });
  for (let Step = 1; Step <= 3; Step++) {
    for (const Operation of ['step.bind', 'step.status', ...(!cached ? applying : [])]) SourceSpans.push({ Operation, Phase: 'step', Step });
  }
  if (!cached) for (const Operation of [...working,'session.input_prepare','session.input_prepare']) SourceSpans.push({ Operation, Phase: 'worker', Step: 0 });
  return { Pass: cached ? 'cached' : 'uncached', SourceSpans };
}

test('TL04–TL07/TL11/PG10: completeness preserves physical DB counts, handoff, private cleanup and every cache ordinal', () => {
  for (const cached of [false, true]) {
    const report = fixture(cached), before = structuredClone(report);
    assert.equal(qualifyCompleteCostBoundaries(report, 3), true);
    assert.deepEqual(report, before);
    for (let i = 0; i < report.SourceSpans.length; i++) {
      const damaged = structuredClone(report);
      damaged.SourceSpans.splice(i, 1);
      assert.throws(() => qualifyCompleteCostBoundaries(damaged, 3), /Incomplete timeline source boundaries/);
    }
    for (const mutation of [
      r => { r.SourceSpans.find(s => s.Operation === 'db.clone' && s.Phase === 'planning').Phase = 'startup'; },
      r => { r.SourceSpans.find(s => s.Operation === 'step.bind').Step = 4; },
      r => { r.Pass = 'unknown'; }, r => { r.SourceSpans.push({ Operation: 'liquibase.invoke', Phase: 'step', Step: 1 }); },
    ]) {
      const damaged = structuredClone(report); mutation(damaged);
      assert.throws(() => qualifyCompleteCostBoundaries(damaged, 3), /Incomplete timeline source boundaries/);
    }
  }
  for (const count of [0, -1, NaN, 1.5]) assert.throws(() => qualifyCompleteCostBoundaries(fixture(), count));
});
