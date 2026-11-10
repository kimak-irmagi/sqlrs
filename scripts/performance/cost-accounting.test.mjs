import test from 'node:test';
import assert from 'node:assert/strict';
import { summarizeHostCosts } from './cost-accounting.mjs';

const ms = 1_000_000;
const span = (ID, Parent, Operation, StartNS, EndNS, Phase = 'step', Step = 1, Outcome = 'succeeded') =>
  ({ ID, Parent, Operation, StartNS, EndNS, Phase, Step, Outcome });

test('CT11: independent Node 100 ms union/self/shared oracle is not 110 ms', () => {
  const input = [span(1, 0, 'prepare.worker', 0, 80 * ms, 'worker', 0),
    span(2, 1, 'capture.read', 10 * ms, 30 * ms), span(3, 1, 'capture.read', 20 * ms, 40 * ms)];
  const before = structuredClone(input);
  const report = summarizeHostCosts(input, 100 * ms);
  assert.deepEqual(input, before);
  assert.equal(report.AssignedNS, 80 * ms);
  assert.equal(report.SharedNS, 10 * ms);
  assert.equal(report.UnassignedNS, 20 * ms);
  const child = report.Rows.find(row => row.Operation === 'capture.read');
  assert.deepEqual(child.CallNS, [20 * ms, 20 * ms]);
  assert.equal(child.Count, 2);
  assert.equal(child.SumNS, 40 * ms);
  assert.equal(child.UnionNS, 30 * ms);
  assert.equal(child.SelfNS, 30 * ms);
  assert.equal(child.ExclusiveNS, 20 * ms);
  assert.equal(child.MeanPerCallMS, 20);
  assert.equal(child.ZTotalMS, 30);
  assert.equal(child.Zratio, 30);
  const parent = report.Rows.find(row => row.Operation === 'prepare.worker');
  assert.equal(parent.SelfNS, 50 * ms);
  assert.equal(parent.ExclusiveNS, 50 * ms);
  assert.equal(parent.ExclusiveNS + child.ExclusiveNS + report.SharedNS + report.UnassignedNS, 100 * ms);
  assert.equal(report.RowsNonAdditive, true);
});

test('CT11: different steps, once-per-job work and measured zero stay distinct', () => {
  const input = [span(1, 0, 'prepare.worker', 0, 100, 'worker', 0),
    span(2, 1, 'runner.inventory', 0, 10, 'startup', 0),
    span(3, 1, 'capture.read', 10, 30), span(4, 1, 'capture.read', 30, 50, 'step', 2),
    span(5, 1, 'capture.prune', 50, 50, 'step', 2)];
  const report = summarizeHostCosts(input, 120);
  assert.equal(report.Rows.length, 5);
  assert.equal(report.SharedNS, 0);
  assert.equal(report.UnassignedNS, 20);
  assert.equal(report.Rows.find(row => row.Operation === 'runner.inventory').Step, 0);
  assert.deepEqual(report.Rows.filter(row => row.Operation === 'capture.read').map(row => row.Step), [1, 2]);
  const zero = report.Rows.find(row => row.Operation === 'capture.prune');
  assert.equal(zero.Count, 1);
  assert.equal(zero.UnionNS, 0);
  assert.equal(zero.MeanPerCallMS, 0);
  assert.deepEqual(summarizeHostCosts([], 100).Rows, []);
  assert.equal(summarizeHostCosts([], 100).UnassignedNS, 100);
});

test('CT11: nested repeated keys exclude descendants; adjacent and unordered spans agree', () => {
  const input = [span(1, 0, 'prepare.worker', 0, 100), span(2, 1, 'capture.read', 10, 80),
    span(3, 2, 'capture.read', 20, 30, 'step', 1, 'failed'),
    span(4, 2, 'capture.prune', 30, 40, 'step', 1, 'cancelled')];
  const report = summarizeHostCosts(input, 100);
  assert.deepEqual(summarizeHostCosts([...input].reverse(), 100), report);
  assert.equal(report.SharedNS, 0);
  assert.equal(report.UnassignedNS, 0);
  assert.equal(report.Rows.reduce((sum, row) => sum + row.ExclusiveNS, 0), 100);
  const read = report.Rows.find(row => row.Operation === 'capture.read');
  assert.equal(read.SumNS, 80);
  assert.equal(read.UnionNS, 70);
  assert.equal(read.SelfNS, 60);
  const root = report.Rows.find(row => row.Operation === 'prepare.worker');
  assert.equal(root.SelfNS, 30);
});

test('CT11: concurrent roots/leaves belong to one shared bucket, not a chosen source', () => {
  const report = summarizeHostCosts([span(1, 0, 'capture.read', 0, 50), span(2, 0, 'capture.prune', 20, 80)], 100);
  assert.equal(report.SharedNS, 30);
  assert.equal(report.AssignedNS, 80);
  assert.equal(report.UnassignedNS, 20);
  assert.equal(report.Rows.find(row => row.Operation === 'capture.read').ExclusiveNS, 20);
  assert.equal(report.Rows.find(row => row.Operation === 'capture.prune').ExclusiveNS, 30);
});

test('CT11/CT10: malformed private evidence fails closed without echoing it', () => {
  const base = [span(1, 0, 'prepare.worker', 0, 100, 'worker', 0), span(2, 1, 'capture.read', 10, 30)];
  const mutations = [
    a => a.push({ ...a[1] }), a => { a[1].ID = 0; }, a => { a[1].Parent = 17; },
    a => { a[0].Parent = 2; }, a => { a[1].Parent = 2; }, a => { a[1].Operation = 'private-marker'; },
    a => { a[1].Phase = 'private-marker'; }, a => { a[1].Outcome = 'private-marker'; },
    a => { a[1].Step = 0; }, a => { a[1].Step = 1.5; }, a => { a[0].Step = 1; },
    a => { a[1].StartNS = -1; }, a => { a[1].EndNS = 9; }, a => { a[1].EndNS = 101; },
    a => { a[1].StartNS = NaN; }, a => { a[1].EndNS = Infinity; },
    a => { a[1].EndNS = Number.MAX_SAFE_INTEGER + 1; },
    a => { a[0].Phase = 'step'; a[0].Step = 2; }, a => { a[1].Parent = -1; },
    a => { a[1].StartNS = 0.5; }, a => { a[0].StartNS = 20; },
  ];
  for (const mutate of mutations) {
    const value = structuredClone(base); mutate(value);
    assert.throws(() => summarizeHostCosts(value, 100), error => error.message === 'Invalid host cost evidence');
  }
  for (const elapsed of [0, -1, NaN, Infinity, 0.5, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => summarizeHostCosts([], elapsed), /Invalid host cost evidence/);
  }
  assert.throws(() => summarizeHostCosts(null, 100), /Invalid host cost evidence/);
  assert.throws(() => summarizeHostCosts([null], 100), /Invalid host cost evidence/);
});

test('CT11: exact span quota and safe sums are bounded independently', () => {
  const full = Array.from({ length: 32768 }, (_, i) => span(i + 1, 0, 'capture.read', 0, 1));
  const report = summarizeHostCosts(full, 10);
  assert.equal(report.Rows[0].Count, 32768);
  assert.equal(report.Rows[0].SumNS, 32768);
  assert.equal(report.Rows[0].UnionNS, 1);
  assert.equal(report.SharedNS, 1);
  assert.throws(() => summarizeHostCosts([...full, span(32769, 0, 'capture.read', 0, 1)], 10), /Invalid host cost evidence/);
  assert.throws(() => summarizeHostCosts([span(1, 0, 'capture.read', 0, Number.MAX_SAFE_INTEGER),
    span(2, 0, 'capture.read', 0, Number.MAX_SAFE_INTEGER)], Number.MAX_SAFE_INTEGER), /Invalid host cost evidence/);
});
