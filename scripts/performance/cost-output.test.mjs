import test from 'node:test';
import assert from 'node:assert/strict';
import { parseCostOutput } from './cost-output.mjs';
import { fixtureSpec } from './inputs.mjs';

const revision = 'a'.repeat(40), engineHash = 'b'.repeat(64), ms = 1e6;
const expected = { route: 'v2', fixture: 'jhipster', revision, engineHash };
function helperObservation(Owner) {
  return { Owner, HostElapsedNS: 0, Available: true, Detail: { ResolutionNS: 10 * ms,
    CumulativeUncertaintyNS: 80 * ms, TotalNS: 0,
    Regions: ['preflight', 'cli', 'drain', 'stream_flush', 'hash_rename', 'renamed_flush', 'manifest_flush', 'final_flush']
      .map(Phase => ({ Phase, StartCS: 0, EndCS: 0, DurationNS: 0 })) } };
}
function fixture() {
  const spec = fixtureSpec('jhipster');
  const samples = ['uncached', 'cached'].map((Pass, index) => ({ Route: 'v2', Fixture: 'jhipster', Pass,
    SourceRevision: revision, EngineSHA256: engineHash, Platform: 'linux/amd64', ElapsedMS: 100,
    Changesets: 8, CacheHits: index ? 8 : 0, LiquibaseCLIStarts: index ? 2 : 10, MigrationCLIStarts: index ? 1 : 9,
    ImageInspectionContainers: 1, ContainerCreates: 4, LiquibaseVersion: spec.version,
    DriverSHA256: '6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93',
    ImageDigest: spec.image.split('@')[1], CoreSHA256: spec.core, HelperDigest: 'sha256:' + 'c'.repeat(64),
    LauncherSHA256: 'd'.repeat(64), JVMStartsFromPinnedLauncher: index ? 5 : 21, JavaDetails: 'Java 21',
    OracleOK: true, CleanupOK: true, SnapshotBackend: 'copy' }));
  const costs = samples.map((sample, index) => {
    const Spans = [{ ID: 1, Parent: 0, Operation: 'prepare.worker', Phase: 'worker', Step: 0,
      StartNS: 0, EndNS: 80 * ms, Outcome: 'succeeded' }];
    if (!index) for (let step = 1; step <= 8; step++) {
      const parent = Spans.length + 1;
      Spans.push({ ID: parent, Parent: 1, Operation: 'prepare.step', Phase: 'step', Step: step,
        StartNS: (step - 1) * 10 * ms, EndNS: step * 10 * ms, Outcome: 'succeeded' });
      Spans.push({ ID: parent + 1, Parent: parent, Operation: 'liquibase.invoke', Phase: 'step', Step: step,
        StartNS: (step - 1) * 10 * ms, EndNS: step * 10 * ms, Outcome: 'succeeded' });
    }
    Spans.push({ ID: Spans.length + 1, Parent: 1, Operation: 'liquibase.plan_invoke', Phase: 'planning', Step: 0,
      StartNS: 0, EndNS: ms, Outcome: 'succeeded' });
    const Helpers = Spans.filter(span => ['liquibase.invoke', 'liquibase.plan_invoke'].includes(span.Operation)).map(span => helperObservation(span.ID));
    return { Pass: sample.Pass, PrepareElapsedNS: 100 * ms, ObserverWaitNS: 1,
      Root: { Root: index + 1, ElapsedNS: 80 * ms, Declared: Spans.length, Complete: true, Ended: true, Spans,
        Helpers, Observer: { EncodeNS: 1, LockWaitNS: 2, WriteNS: 3 } } };
  });
  return { samples, costs };
}
const output = ({ samples, costs }) => 'PRIVATE_LOG_MARKER\n' +
  samples.map(sample => 'P01B_SAMPLE ' + JSON.stringify(sample)).join('\n') + '\n' +
  costs.map(cost => 'P01B_COST ' + JSON.stringify(cost)).join('\n');

test('CT10/CT11: qualified cost report links source, original API time and every zero/cache step', () => {
  const report = parseCostOutput(0, output(fixture()), expected);
  assert.equal(report.length, 2);
  const [fresh, cached] = report;
  assert.equal(fresh.SourceRevision, revision);
  assert.equal(fresh.EngineSHA256, engineHash);
  assert.equal(fresh.PrepareElapsedNS, 100 * ms);
  assert.equal(fresh.ObserverWaitNS, 1);
  assert.equal(fresh.Accounting.UnassignedNS, 20 * ms);
  assert.deepEqual(fresh.Steps.map(step => step.ApplyCalls), Array(8).fill(1));
  assert.deepEqual(cached.Steps.map(step => step.ApplyCalls), Array(8).fill(0));
  assert.ok(cached.Steps.every(step => step.Rows.length === 0));
  const apply = fresh.Overall.find(row => row.Operation === 'liquibase.invoke');
  assert.equal(apply.Count, 8);
  assert.equal(apply.ZTotalMS, 80);
  assert.equal(apply.Zratio, 80);
  assert.equal(apply.MeanPerCallMS, 10);
  assert.equal(apply.Source.Relation, 'changed');
  assert.equal(apply.Source.Released.Symbol, 'taskExecutor.executeLiquibaseStep');
  assert.equal(apply.Phase, undefined);
});

test('CT10: process/oracle failure, old/v1 selection, missing/duplicate/reordered roots never qualify', () => {
  assert.throws(() => parseCostOutput(1, output(fixture()), expected), /Invalid cost collector evidence/);
  for (const mutate of [
    f => { f.samples[0].OracleOK = false; }, f => { f.samples[1].CleanupOK = false; },
    f => { f.costs.pop(); }, f => { f.costs.push(f.costs[0]); }, f => { f.costs.reverse(); },
    f => { f.costs[0].Root.Root = 2; }, f => { f.costs[0].Pass = 'cached'; },
    f => { f.costs[0].Root.Complete = false; }, f => { f.costs[0].Root.Ended = false; },
    f => { f.costs[0].Root.Declared--; }, f => { f.costs[0].Root.Spans = []; },
    f => { f.costs[0].Root.ElapsedNS = 101 * ms; }, f => { f.costs[0].Root.ElapsedNS--; },
    f => { f.costs[0].PrepareElapsedNS--; }, f => { f.costs[0].ObserverWaitNS = -1; },
    f => { f.costs[0].Root.Spans[0].Outcome = 'failed'; },
    f => { f.costs[0].Root.Spans[1].Parent = 0; }, f => { f.costs[0].Root.Spans[1].ID = 17; },
    f => { f.costs[0].Root.Spans[2].Step = 9; }, f => { f.costs[0].Root.Spans[2].Parent = 17; },
    f => { f.costs[0].Root.Spans[2].Operation = 'private-marker'; },
  ]) {
    const f = fixture(); mutate(f);
    assert.throws(() => parseCostOutput(0, output(f), expected), error => error.message === 'Invalid cost collector evidence');
  }
  for (const patch of [{ route: 'v1' }, { revision: '668d5050ad154c1967bea4c6e40b91f3a703d5ba' }]) {
    assert.throws(() => parseCostOutput(0, output(fixture()), { ...expected, ...patch }), /Invalid cost collector evidence/);
  }
  assert.throws(() => parseCostOutput(0, output(fixture()) + '\nP01B_COST {PRIVATE_MARKER', expected), /Invalid cost collector evidence/);
});

test('CT11/CT10: private extras are dropped; actual intermediate failures remain measured, not erased', () => {
  const f = fixture(), root = f.costs[0].Root;
  root.Spans.push({ ID: root.Spans.length + 1, Parent: 1, Operation: 'docker.command', Phase: 'startup', Step: 0,
    StartNS: 0, EndNS: ms, Outcome: 'failed', Arguments: 'PRIVATE_MARKER' });
  root.Declared++;
  f.costs[0].Private = 'PRIVATE_MARKER';
  root.Private = 'PRIVATE_MARKER';
  const report = parseCostOutput(0, output(f), expected);
  assert.equal(report[0].Accounting.Rows.find(row => row.Operation === 'docker.command').Count, 1);
  assert.doesNotMatch(JSON.stringify(report), /PRIVATE_MARKER|PRIVATE_LOG/);
  report[0].Overall[0].Source.Current.Symbol = 'caller-change';
  assert.doesNotMatch(JSON.stringify(parseCostOutput(0, output(f), expected)), /caller-change/);
});

test('CT08/CT11: per-step apply intervals must agree with independent CLI/cache counts', () => {
  for (const operation of ['capture.read', 'prepare.step']) {
    const f = fixture();
    f.costs[0].Root.Spans[2].Operation = operation;
    assert.throws(() => parseCostOutput(0, output(f), expected), /Invalid cost collector evidence/);
  }
  const f = fixture();
  f.costs[1].Root.Spans.push({ ID: f.costs[1].Root.Spans.length + 1, Parent: 1, Operation: 'liquibase.invoke', Phase: 'step', Step: 1,
    StartNS: 1, EndNS: 2, Outcome: 'succeeded' });
  f.costs[1].Root.Declared++;
  assert.throws(() => parseCostOutput(0, output(f), expected), /Invalid cost collector evidence/);
});

test('CT07/CT11: closed outcome counts and non-additive totals remain explicit in the report', () => {
  const f = fixture(), root = f.costs[0].Root;
  for (const Outcome of ['failed', 'cancelled', 'panicked', 'succeeded']) {
    root.Spans.push({ ID: root.Spans.length + 1, Parent: 1, Operation: 'docker.command', Phase: 'startup', Step: 0,
      StartNS: 0, EndNS: ms, Outcome });
    root.Declared++;
  }
  const report = parseCostOutput(0, output(f), expected)[0];
  const command = report.Accounting.Rows.find(row => row.Operation === 'docker.command');
  assert.deepEqual(command.Outcomes, { succeeded: 1, failed: 1, cancelled: 1, panicked: 1 });
  assert.deepEqual(report.Overall.find(row => row.Operation === 'docker.command').Outcomes, command.Outcomes);
  assert.equal(report.OverallNonAdditive, true);
});
