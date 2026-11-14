import test from 'node:test';
import assert from 'node:assert/strict';
import { qualifyCompleteCostBoundaries } from './complete-cost-boundaries.mjs';

// IC07 isolates inventory counts from the older complete-workload fixture.
// Every other operation remains mandatory and is checked by the existing test.
test('IC07: a cached prepare cannot claim a probe and a cold prepare cannot omit one',()=>{
  const common=['prepare.request','prepare.admission','prepare.worker','admission.inputs','admission.image','admission.identity','admission.commit',
    'input.publish','binding.prepare','input.materialize','runner.resolve','runner.ensure','runner.inventory_lookup','binding.commit',
    'prepare.planning','liquibase.plan_invoke','plan.launch_prepare','plan.input_prepare','plan.capture_reconcile','plan.retain','plan.dispose','plan.release','plan.suffix',
    'prepare.publication','prepare.final_cleanup','projection.cleanup','finalization.confirm','finalization.commit'];
  const SourceSpans=common.map(Operation=>({Operation,Phase:'worker',Step:0}));
  for(const Operation of ['db.startup','db.lock_wait','db.source_prepare','db.assign','db.clone','db.start','db.attach','db.verify.startup']) {
    for(const Phase of ['planning','publication']) SourceSpans.push({Operation,Phase,Step:0});
  }
  for(const Operation of ['step.bind','step.status']) SourceSpans.push({Operation,Phase:'step',Step:1});
  const report={Pass:'cached',SourceSpans};
  assert.equal(qualifyCompleteCostBoundaries(report,1),true);
  report.SourceSpans.push({Operation:'runner.inventory',Phase:'worker',Step:0});
  assert.throws(()=>qualifyCompleteCostBoundaries(report,1));
});
