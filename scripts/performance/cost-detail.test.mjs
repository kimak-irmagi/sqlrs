import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { qualifyCostDetails } from './cost-detail.mjs';
import { summarizeHostCosts } from './cost-accounting.mjs';

const ms = 1e6;
const phases = ['preflight', 'cli', 'drain', 'stream_flush', 'hash_rename', 'renamed_flush', 'manifest_flush', 'final_flush'];
function fixture() {
  const Regions = phases.map((Phase, index) => ({ Phase, StartCS: index < 2 ? 0 : 2,
    EndCS: index === 0 ? 0 : 2, DurationNS: index === 1 ? 20 * ms : 0 }));
  return { Spans: [
    { ID: 1, Parent: 0, Operation: 'prepare.worker', Phase: 'worker', Step: 0, StartNS: 0, EndNS: 80 * ms, Outcome: 'succeeded' },
    { ID: 2, Parent: 1, Operation: 'liquibase.invoke', Phase: 'step', Step: 1, StartNS: 0, EndNS: 30 * ms, Outcome: 'succeeded' },
  ], Helpers: [{ Owner: 2, HostElapsedNS: 30 * ms, Available: true,
    Detail: { ResolutionNS: 10 * ms, CumulativeUncertaintyNS: 80 * ms, TotalNS: 20 * ms, Regions } }],
  Observer: { EncodeNS: 2 * ms, LockWaitNS: 3 * ms, WriteNS: 4 * ms } };
}

test('DT07: helper and observer stay separate from independently conserved host/API time', () => {
  const root = fixture();
  const result = qualifyCostDetails(root);
  const host = summarizeHostCosts(root.Spans, 100 * ms);
  assert.equal(host.UnassignedNS, 20 * ms);
  assert.equal(result.HelperNonAdditive, true);
  assert.equal(result.Helpers[0].TotalNS, 20 * ms);
  assert.equal(result.Helpers[0].ResolutionNS, 10 * ms);
  assert.equal(result.Helpers[0].CumulativeUncertaintyNS, 80 * ms);
  assert.equal(result.Helpers[0].Step, 1);
  assert.equal(result.Helpers[0].Source.Released.Symbol, 'taskExecutor.executeLiquibaseStep');
  assert.deepEqual(result.Observer, { EncodeNS: 2 * ms, LockWaitNS: 3 * ms, WriteNS: 4 * ms,
    Unmeasured: ['report_snapshot_copy', 'observer_marker_construction_and_write'] });
  root.Observer.WriteNS = 1000 * ms;
  assert.equal(qualifyCostDetails(root).Observer.WriteNS, 1000 * ms, 'observer is not forced into API window');
  const zero = fixture();
  for (const region of zero.Helpers[0].Detail.Regions) region.StartCS = region.EndCS = region.DurationNS = 0;
  zero.Helpers[0].Detail.TotalNS = 0;
  assert.equal(qualifyCostDetails(zero).Helpers[0].TotalNS, 0, 'present zero remains measurable');
});

test('DT07: incomplete, wrong-owner, malformed or unsafe metadata cannot qualify', () => {
  for (const mutate of [
    r => { delete r.Helpers; }, r => { r.Helpers = []; }, r => { r.Helpers.push(structuredClone(r.Helpers[0])); },
    r => { r.Helpers[0].Owner = 1; }, r => { r.Helpers[0].Owner = 9; }, r => { r.Helpers[0].Available = false; },
    r => { r.Helpers[0].HostElapsedNS = -1; }, r => { r.Helpers[0].HostElapsedNS = 30 * ms + 1; },
    r => { r.Helpers[0].HostElapsedNS = 10 * ms - 1; }, r => { r.Helpers[0].Detail.Regions.pop(); },
    r => { r.Helpers[0].Detail.Regions[0].Phase = 'private-marker'; },
    r => { r.Helpers[0].Detail.Regions[1].StartCS = 1; },
    r => { r.Helpers[0].Detail.Regions[1].EndCS = -1; },
    r => { r.Helpers[0].Detail.Regions[1].DurationNS = 1; },
    r => { r.Helpers[0].Detail.Regions[1].EndCS = Number.MAX_SAFE_INTEGER; },
    r => { r.Helpers[0].Detail.ResolutionNS++; }, r => { r.Helpers[0].Detail.CumulativeUncertaintyNS++; },
    r => { r.Helpers[0].Detail.TotalNS++; }, r => { delete r.Observer; }, r => { r.Observer.EncodeNS = -1; },
    r => { r.Observer.LockWaitNS = Number.MAX_SAFE_INTEGER + 1; }, r => { r.Observer.WriteNS = 'private-marker'; },
    r => { r.Spans = null; }, r => { r.Helpers = null; }, r => { r.Helpers[0] = null; },
    r => { r.Helpers[0].Detail = null; }, r => { r.Helpers[0].Detail.Regions[1] = null; },
  ]) {
    const root = fixture(); mutate(root);
    assert.throws(() => qualifyCostDetails(root), error => error.message === 'Invalid cost collector evidence');
  }
});

test('DT07: private fields and caller mutations do not enter later reports', () => {
  const root = fixture();
  root.Observer.Private = root.Helpers[0].Private = root.Helpers[0].Detail.Regions[0].Private = 'private-marker';
  const result = qualifyCostDetails(root);
  assert.doesNotMatch(JSON.stringify(result), /private-marker|Private/);
  result.Helpers[0].Regions[0].Phase = 'caller-mutation';
  result.Helpers[0].Source.Current.Symbol = 'caller-mutation';
  assert.doesNotMatch(JSON.stringify(qualifyCostDetails(root)), /caller-mutation/);
});

test('DT05: collector retains original API timer and one deadline for both end markers', () => {
  const source = fs.readFileSync('test/performance/liquibase/collector_linux_test.go', 'utf8');
  const block = source.slice(source.indexOf('end := time.Now()'), source.indexOf('stage = "oracle"'));
  assert.equal((block.match(/context.WithTimeout/g) ?? []).length, 1);
  assert.ok(block.indexOf('costReader.await(') < block.indexOf('costReader.awaitMeasured('));
  assert.ok(block.indexOf('costReader.awaitMeasured(') < block.indexOf('stop()'));
  assert.ok(block.indexOf('stop()') < block.indexOf('qualifyCostPass('));
});
