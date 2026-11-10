import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { parse } from 'yaml';

test('CT05/CT12: cost prerequisite is explicit, own-repository and bounded', () => {
  const workflow = parse(fs.readFileSync('.github/workflows/liquibase-cost-probe.yml', 'utf8'));
  assert.deepEqual(workflow.on, { pull_request: { types: ['labeled'] } });
  assert.deepEqual(workflow.permissions, { contents: 'read' });
  assert.equal(workflow.concurrency['cancel-in-progress'], false);
  assert.match(workflow.concurrency.group, /cost-probe/);
  const jobs = Object.values(workflow.jobs);
  assert.equal(jobs.length, 1);
  const job = jobs[0];
  assert.match(job.if, /head.repo.full_name == github.repository/);
  assert.match(job.if, /github.event.label.name == 'performance:cost-probe'/);
  assert.equal(job['runs-on'], 'ubuntu-24.04');
  assert.equal(job['timeout-minutes'], 20);
  const checkout = job.steps.find(step => step.uses?.startsWith('actions/checkout@'));
  assert.equal(checkout.with.ref, '${{ github.event.pull_request.head.sha }}');
  assert.equal(checkout.with['persist-credentials'], false);
});

test('CT05/CT12: prerequisites do not run the acceptance cohort or trace the workload', () => {
  const workflow = parse(fs.readFileSync('.github/workflows/liquibase-cost-probe.yml', 'utf8'));
  const steps = workflow.jobs.probe.steps;
  const prepare = steps.find(step => step.name === 'Acquire public native fixture');
  assert.match(prepare.run, /docker pull liquibase\/liquibase:5\.0\.4/);
  const native = steps.find(step => step.name === 'Qualify sync and actual session acquisition');
  assert.equal(native['working-directory'], 'backend/local-engine-go');
  assert.equal(native.env.SQLRS_COST_NATIVE_PROBE, '1');
  assert.equal(native.env.SQLRS_LIQUIBASE_DOCKER_E2E, '1');
  assert.match(native.run, /TestCostSyncImplementationDocker/);
  assert.match(native.run, /TestLiquibaseStepCaptureReusableRunnerDocker/);
  assert.match(native.run, /-count=1 -timeout=8m/);
  for (const step of steps) {
    assert.doesNotMatch(step.run ?? '', /run-diagnostic|run-cohort|strace|SQLRS_LIQUIBASE_RELEASED_BENCH/);
    assert.ok(!step.uses?.startsWith('actions/upload-artifact@'), 'no raw probe log artifact');
  }
});
