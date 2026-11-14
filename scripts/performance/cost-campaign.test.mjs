import test from 'node:test';
import assert from 'node:assert/strict';
import { costCampaignSelection, costCampaignIdentity, costCampaignSchedule, costCampaignEnvironment } from './cost-campaign.mjs';

const oldRevision = '668d5050ad154c1967bea4c6e40b91f3a703d5ba';
const oldHash = '1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0';
const baselineRevision = 'cab99c4c7ced5b18f4dc43a65b002d84d8fef02d';
const baselineHash = 'b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca';
const revision = 'a'.repeat(40), hash = 'b'.repeat(64);
const artifacts = () => ({ before: { revision: oldRevision, engineHash: oldHash, baselineRevision, baselineHash },
  after: { revision, engineHash: hash, baselineRevision, baselineHash } });

test('CT13: only the explicit same-repository attribution label selects an immutable head', () => {
  const event = { action: 'labeled', label: { name: 'performance:cost-attribution' },
    pull_request: { head: { sha: revision, repo: { full_name: 'owner/sqlrs' } } } };
  assert.equal(costCampaignSelection('pull_request', event, 'owner/sqlrs'), revision);
  assert.equal(costCampaignSelection('push', event, 'owner/sqlrs'), null);
  assert.equal(costCampaignSelection('pull_request', event, 'foreign/sqlrs'), null);
  assert.equal(costCampaignSelection('pull_request', {}, 'owner/sqlrs'), null);
  for (const patch of [{ action: 'synchronize' }, { label: { name: 'performance:diagnostic' } }]) {
    assert.equal(costCampaignSelection('pull_request', { ...event, ...patch }, 'owner/sqlrs'), null);
  }
  for (const sha of ['', 'main', 'a'.repeat(39), 'A'.repeat(40), null]) {
    const invalid = structuredClone(event); invalid.pull_request.head.sha = sha;
    assert.throws(() => costCampaignSelection('pull_request', invalid, 'owner/sqlrs'), /Invalid cost campaign selection/);
  }
});

test('CT13: before/after are distinct exact artifacts and retain the actual released v1', () => {
  const { before, after } = artifacts();
  const result = costCampaignIdentity(before, after, revision);
  assert.deepEqual(result, { BeforeRevision: oldRevision, BeforeEngineSHA256: oldHash,
    Revision: revision, EngineSHA256: hash, BaselineRevision: baselineRevision, BaselineSHA256: baselineHash });
  for (const field of ['revision', 'engineHash', 'baselineRevision', 'baselineHash']) {
    for (const which of ['before', 'after']) {
      const pair = artifacts(); pair[which][field] = 'private-marker';
      assert.throws(() => costCampaignIdentity(pair.before, pair.after, revision), /Invalid cost campaign identity/);
    }
  }
  for (const change of [ { revision: oldRevision }, { revision: baselineRevision }, { engineHash: oldHash }, { engineHash: baselineHash } ]) {
    assert.throws(() => costCampaignIdentity(before, { ...after, ...change }, change.revision ?? revision));
  }
  assert.throws(() => costCampaignIdentity(undefined, after, revision));
  assert.throws(() => costCampaignIdentity(before, undefined, revision));
  before.Private = after.Private = 'private-marker';
  assert.doesNotMatch(JSON.stringify(costCampaignIdentity(before, after, revision)), /private-marker|Private/);
});

test('CT13: two reversed repetitions preserve all four routes and reuse the same new binary', () => {
  const schedule = costCampaignSchedule();
  assert.deepEqual(schedule.map(item => item.Variant), ['released_v1', 'old_v2', 'new_off', 'new_on', 'new_on', 'new_off', 'old_v2', 'released_v1']);
  assert.deepEqual(schedule.map(item => item.ID), [1,2,3,4,5,6,7,8]);
  assert.deepEqual(schedule.filter(item => item.Record).map(item => item.ID), [4,5]);
  assert.equal(schedule.filter(item => item.Route === 'v1').length, 2);
  schedule[0].Variant = 'caller-mutation';
  assert.equal(costCampaignSchedule()[0].Variant, 'released_v1');
  const environment = { PATH: 'original', SQLRS_PREPARE_COST: '1', SQLRS_BENCH_COST: '1', SQLRS_BENCH_TRACE: '1', SQLRS_BENCH_DIAGNOSTIC: '1' };
  for (const item of costCampaignSchedule()) {
    const got = costCampaignEnvironment(environment, item);
    assert.equal(got.PATH, 'original');
    assert.equal(got.SQLRS_BENCH_COST, item.Record ? '1' : '0');
    assert.equal(got.SQLRS_PREPARE_COST, '');
    assert.equal(got.SQLRS_BENCH_TRACE, '');
    assert.equal(got.SQLRS_BENCH_DIAGNOSTIC, '');
  }
  assert.equal(environment.SQLRS_PREPARE_COST, '1');
  for (const item of [{ ID: 0 }, { ...costCampaignSchedule()[0], Record: true }, { ...costCampaignSchedule()[3], Route: 'v1' }]) {
    assert.throws(() => costCampaignEnvironment(environment, item), /Invalid cost campaign member/);
  }
});
