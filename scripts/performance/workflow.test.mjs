import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import YAML from 'yaml';

test('PA03: Temurin uses the provider full build selector without floating to another Java',()=>{
  const action=YAML.parse(fs.readFileSync(new URL('../../.github/actions/liquibase-perf-setup/action.yml',import.meta.url),'utf8'));
  const java=action.runs.steps.find(step=>step.uses?.startsWith('actions/setup-java'));
  assert.equal(java.with.distribution,'temurin');
  assert.equal(java.with['java-version'],'21.0.2+13.0.LTS');
});

test('PA01/PA07/PA08/PA10: separate opt-in read-only workflow keeps cohorts on one bounded VM', () => {
  const text = fs.readFileSync(new URL('../../.github/workflows/liquibase-performance.yml', import.meta.url), 'utf8');
  const flow = YAML.parse(text);
  assert.deepEqual(Object.keys(flow.on).sort(), ['pull_request', 'workflow_dispatch']);
  assert.deepEqual(flow.on.pull_request.types, ['labeled']);
  assert.deepEqual(flow.permissions, { contents: 'read' });
  assert.equal(flow.concurrency['cancel-in-progress'], false);
  assert.doesNotMatch(text, /pull_request_target|secrets\.|self-hosted|ubuntu-latest|docker system prune/);
  for (const job of Object.values(flow.jobs)) {
    assert.equal(job['runs-on'], 'ubuntu-24.04');
    assert.ok(job['timeout-minutes'] <= 360);
    for (const step of job.steps || []) {
      if (step.uses?.startsWith('actions/checkout')) {
        assert.equal(step.with['persist-credentials'], false);
        assert.match(step.with.ref, /candidate|outputs.sha/);
      }
      if (step.uses?.startsWith('actions/upload-artifact')) assert.match(step.with.path, /^\.tmp\/performance\/reports/);
    }
  }
  assert.equal(flow.jobs.collect.strategy.matrix.fixture.length, 3);
  assert.deepEqual(flow.jobs.collect.strategy.matrix.phase, ['comparison', 'confirmation']);
  assert.ok(flow.jobs.collect.needs.includes('pilot'));
  assert.match(flow.jobs.collect.if, /full/);
  assert.ok(flow.jobs.collect.steps.some(s => /run-cohort\.mjs/.test(s.run || '')));
});
