import test from 'node:test';
import assert from 'node:assert/strict';
import { summarizeHostCosts } from './cost-accounting.mjs';

// CPR07: the same 60 ms of work must not become a speedup merely because the
// separate prune label moves inside capture.read. This oracle is not a sample.
test('CPR07: old read plus prune and combined read conserve equal work', () => {
  const ms = 1e6;
  const span = (ID, Parent, Operation, start, end, Step = 2) => ({ ID, Parent, Operation,
    Phase: Step ? 'step' : 'worker', Step, StartNS: start * ms, EndNS: end * ms, Outcome: 'succeeded' });
  const root = span(1, 0, 'prepare.worker', 0, 100, 0);
  const old = summarizeHostCosts([root, span(2, 1, 'capture.read', 10, 30), span(3, 1, 'capture.prune', 30, 70)], 100 * ms);
  const combined = summarizeHostCosts([root, span(2, 1, 'capture.read', 10, 70)], 100 * ms);
  const original = old.Rows.filter(row => row.Operation !== 'prepare.worker');
  const current = combined.Rows.find(row => row.Operation === 'capture.read');
  assert.equal(original.reduce((sum, row) => sum + row.UnionNS, 0), 60 * ms);
  assert.equal(current.UnionNS, 60 * ms);
  assert.equal(current.ZTotalMS, 60);
  assert.equal(current.Zratio, 60);
  assert.equal(combined.Rows.some(row => row.Operation === 'capture.prune'), false);
  for (const report of [old, combined]) {
    assert.equal(report.Rows.reduce((sum, row) => sum + row.ExclusiveNS, 0) + report.SharedNS + report.UnassignedNS, 100 * ms);
    assert.equal(report.PrepareElapsedNS, 100 * ms);
  }
});
