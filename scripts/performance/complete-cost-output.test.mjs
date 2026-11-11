import test from 'node:test';
import assert from 'node:assert/strict';
import { parseCompleteCostOutput } from './complete-cost-output.mjs';
import { costCampaignEnvironment, costCampaignSchedule } from './cost-campaign.mjs';
import { fixtureSpec } from './inputs.mjs';

const revision = 'a'.repeat(40), engineHash = 'b'.repeat(64), ms = 1e6;
const expected = { route: 'v2', fixture: 'jhipster', revision, engineHash };
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
    const span = (ID, Parent, Operation, Phase, Step, StartNS, EndNS) => ({ ID, Parent, Operation, Phase, Step, StartNS, EndNS, Outcome: 'succeeded' });
    const Spans = [span(1, 0, 'prepare.request', 'worker', 0, 0, 80*ms), span(2, 1, 'prepare.admission', 'admission', 0, 0, ms),
      span(3, 1, 'prepare.worker', 'worker', 0, ms, 80*ms), span(4, 3, 'liquibase.plan_invoke', 'planning', 0, ms, 2*ms)];
    if (!index) for (let step = 1; step <= 8; step++) Spans.push(span(Spans.length+1, 3, 'liquibase.invoke', 'step', step, step*ms+ms, step*ms+2*ms));
    const Helpers = Spans.filter(s => s.Operation.startsWith('liquibase.')).map(s => ({ Owner: s.ID, HostElapsedNS: 0, Available: true,
      Detail: { ResolutionNS: 10*ms, CumulativeUncertaintyNS: 80*ms, TotalNS: 0,
        Regions: ['preflight','cli','drain','stream_flush','hash_rename','renamed_flush','manifest_flush','final_flush'].map(Phase => ({ Phase, StartCS: 0, EndCS: 0, DurationNS: 0 })) } }));
    return { Pass: sample.Pass, PrepareElapsedNS: 100*ms, ObserverWaitNS: 1,
      Root: { Root: index+1, ElapsedNS: 80*ms, Declared: Spans.length, Complete: true, Ended: true, Spans, Helpers,
        Observer: { EncodeNS: 1, LockWaitNS: 2, WriteNS: 3 }, Clock: { Domain: 'linux-monotonic-relative-v1', OriginNS: 101*ms },
        ObserverWindow: { StartNS: 181*ms, EndNS: 182*ms } },
      Timeline: { Timer: { Window: { StartNS: 100*ms, EndNS: 200*ms }, StartBracket: { StartNS: 100*ms-1, EndNS: 100*ms+1 },
        EndBracket: { StartNS: 200*ms-1, EndNS: 200*ms+1 }, APIElapsedNS: 100*ms }, ClockDeltaNS: 0,
        Client: [{ Kind: 'client.post', StartNS: 100*ms, EndNS: 110*ms }, { Kind: 'client.status', StartNS: 110*ms, EndNS: 200*ms }] } };
  });
  return { samples, costs };
}
const output = f => f.samples.map(s => `P01B_SAMPLE ${JSON.stringify(s)}`).concat(f.costs.map(c => `P01B_COST ${JSON.stringify(c)}`)).join('\n');

test('TL08–TL11: actual window is additive, source-local unknown remains explicit, helper stays separate', () => {
  const f = fixture();
  f.costs[0].Private = f.costs[0].Root.Spans[0].Private = 'PRIVATE_SENTINEL';
  const reports = parseCompleteCostOutput(0, output(f), expected);
  for (const report of reports) {
    const segments = report.Timeline.Segments;
    assert.equal(segments[0].StartNS, 0);
    assert.equal(segments.at(-1).EndNS, 100*ms);
    assert.equal(segments.reduce((s, x) => s + x.DurationNS, 0), 100*ms);
    assert.equal(report.Additive.reduce((s, x) => s + x.DurationNS, 0), 100*ms);
    assert.ok(report.Timeline.UnresolvedNS > 0);
    assert.equal(report.ClockDeltaNS, 0);
    assert.equal(report.PrepareElapsedNS, 100*ms);
    assert.equal(report.RequestElapsedNS, 80*ms);
    assert.equal(report.WorkerElapsedNS, 79*ms);
    assert.equal(report.HelperNonAdditive, true);
    assert.equal(report.Steps.length, 8);
    assert.ok(report.Timeline.Segments.some(s => s.Owners.some(owner => owner.Source.Current.Symbol)));
  }
  assert.deepEqual(reports[0].Steps.map(s => s.ApplyCalls), Array(8).fill(1));
  assert.deepEqual(reports[1].Steps.map(s => s.ApplyCalls), Array(8).fill(0));
  assert.doesNotMatch(JSON.stringify(reports), /PRIVATE_SENTINEL|"Private"/);
});

test('TL02/TL08: no partial, foreign-root, unknown-clock or unbracketed evidence qualifies', () => {
  for (const mutate of [
    f => { f.costs[0].Timeline = undefined; }, f => { f.costs[0].Timeline.ClockDeltaNS++; },
    f => { f.costs[0].Root.Clock.Domain = 'private'; }, f => { f.costs[0].Root.Clock.OriginNS = 99*ms; },
    f => { f.costs[0].Root.ObserverWindow = undefined; }, f => { f.costs[0].Root.Spans[0].Operation = 'prepare.worker'; },
    f => { f.costs[0].Root.Spans[1].Operation = 'prepare.worker'; }, f => { f.costs[0].Root.Spans[2].Outcome = 'failed'; },
    f => { f.costs[0].Timeline.Client[1].StartNS = 109*ms; }, f => { f.costs[0].Timeline.Client[1].Kind = 'private'; },
    f => { f.costs[0].Timeline.Timer.APIElapsedNS--; }, f => { f.costs[0].Root.Complete = false; },
    f => { f.samples[0].OracleOK = false; }, f => { f.costs.reverse(); },
  ]) {
    const f = fixture(); mutate(f);
    assert.throws(() => parseCompleteCostOutput(0, output(f), expected), e => e.message === 'Invalid complete cost evidence');
  }
});

test('TL14: unchanged schedule selects timeline only for the two measured new members', () => {
  const env = { SQLRS_BENCH_TIMELINE: '1', SQLRS_PREPARE_TIMELINE: '1', SQLRS_PREPARE_TIMELINE_REFERENCE: 'private' };
  for (const member of costCampaignSchedule()) {
    const got = costCampaignEnvironment(env, member);
    assert.equal(got.SQLRS_BENCH_TIMELINE, member.Record ? '1' : '0');
    assert.equal(got.SQLRS_PREPARE_TIMELINE, '');
    assert.equal(got.SQLRS_PREPARE_TIMELINE_REFERENCE, '');
  }
  assert.equal(env.SQLRS_PREPARE_TIMELINE_REFERENCE, 'private');
});

test('TL01/TL03: real recorder setup need not coincide with the request start', () => {
  const f = fixture();
  for (const cost of f.costs) {
    cost.Root.Spans[0].StartNS = 1;
    cost.Root.Spans[1].StartNS = 1;
  }
  const reports = parseCompleteCostOutput(0, output(f), expected);
  assert.equal(reports[0].RequestElapsedNS, 80*ms-1);
  assert.equal(reports[0].SourceSpans[0].StartNS, ms+1);
});
