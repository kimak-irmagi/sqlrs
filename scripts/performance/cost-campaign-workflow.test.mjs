import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { parse } from 'yaml';

test('CT13: explicit own-repository campaign keeps all eight members on one bounded VM', () => {
  const workflow = parse(fs.readFileSync('.github/workflows/liquibase-cost-attribution.yml', 'utf8'));
  assert.deepEqual(workflow.on.pull_request.types, ['labeled']);
  assert.deepEqual(workflow.permissions, { contents: 'read' });
  assert.equal(workflow.concurrency['cancel-in-progress'], false);
  assert.deepEqual(Object.keys(workflow.jobs), ['campaign']);
  const job = workflow.jobs.campaign;
  assert.equal(job['runs-on'], 'ubuntu-24.04');
  assert.equal(job['timeout-minutes'], 60);
  assert.match(job.if, /head.repo.full_name == github.repository/);
  assert.match(job.if, /github.event.label.name == 'performance:cost-attribution'/);
  assert.equal(job.strategy, undefined);
  assert.match(job.steps[0].run, /PERFORMANCE_JOB_STARTED_MS/);
  const checkouts = job.steps.filter(step => step.uses === 'actions/checkout@v7');
  assert.equal(checkouts.length, 3);
  assert.equal(checkouts[0].with.ref, '668d5050ad154c1967bea4c6e40b91f3a703d5ba');
  assert.equal(checkouts[1].with.ref, '${{ github.event.pull_request.head.sha }}');
  assert.equal(checkouts[1].with.path, '.tmp/performance-observer');
  assert.equal(checkouts[2].with.ref, '${{ github.event.pull_request.head.sha }}');
  assert.equal(checkouts[2].with.clean, false);
  for (const checkout of checkouts) assert.equal(checkout.with['persist-credentials'], false);
  const old = job.steps.find(step => step.name === 'Build exact old v2');
  assert.equal(old.with.candidate_sha, checkouts[0].with.ref);
  assert.equal(old.with.expected_engine_sha, '1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0');
  assert.equal(old.with.source_root, '${{ github.workspace }}');
  const preserve = job.steps.find(step => step.name === 'Retain verified old artifact');
  assert.match(preserve.run, /run-cost.mjs preserve-before/);
  assert.match(preserve.run, /git diff --exit-code/);
  assert.ok(job.steps.indexOf(preserve) < job.steps.indexOf(checkouts[2]));
  assert.equal(job.steps.find(step => step.name === 'Build selected new v2').with.candidate_sha, checkouts[2].with.ref);
  assert.equal(job.steps.find(step => step.name === 'Eight serial before after off on members').run, 'node scripts/performance/run-cost.mjs');
  const artifact = job.steps.find(step => step.uses === 'actions/upload-artifact@v7');
  assert.equal(artifact.if, 'always()');
  assert.equal(artifact.with.path, '.tmp/performance/cost-reports/*.json');
  assert.equal(artifact.with['retention-days'], 14);
});

test('CT13: actual parent verifies identities, owns bounded private output and uses unchanged collector', () => {
  const source = fs.readFileSync('scripts/performance/run-cost.mjs', 'utf8');
  for (const symbol of ['costCampaignIdentity', 'costCampaignSelection', 'costCampaignSchedule', 'costCampaignEnvironment', 'parseCostOutput', 'parseCollectorResult', 'verifyBytes']) assert.ok(source.includes(symbol+'('));
  assert.match(source, /warm\(repo, \['jhipster'\]\)/);
  assert.match(source, /-test.run=\^TestReleasedEngine\$/);
  assert.match(source, /50 \* 60 \* 1000/);
  assert.match(source, /4 \* 1024 \* 1024/);
  assert.match(source, /active\?\.kill\('SIGTERM'\)/);
  assert.doesNotMatch(source, /console\.(?:log|error)\([^\n]*(?:output|error\.message|err\.message)/);
  assert.doesNotMatch(source, /strace|rm -rf|reset --hard|shell:\s*true/);
});

test('CT13: completion keeps same-helper off/on proof and cannot turn cancellation into success', () => {
  const source = fs.readFileSync('scripts/performance/run-cost.mjs', 'utf8');
  const complete = source.indexOf("report.status = 'cost_attribution_complete'");
  const proof = source.indexOf('newHelperDigests.size !== 1');
  assert.ok(proof > 0 && proof < complete);
  assert.match(source.slice(proof, complete), /cancelled \|\| Date.now\(\) >= deadline/);
  assert.match(source, /report.stage = 'image_prerequisite'/);
});

test('CT05/CT13: the actual JHipster image must pass its native prerequisite, not skip or match zero tests', () => {
  const source = fs.readFileSync('scripts/performance/run-cost.mjs', 'utf8');
  const native = fs.readFileSync('backend/local-engine-go/internal/runtime/cost_jhipster_image_docker_requirements_test.go', 'utf8');
  assert.match(source, /--- PASS: TestCostJHipsterImagePrerequisitesDocker/);
  assert.match(native, /func TestCostJHipsterImagePrerequisitesDocker\(/);
  assert.match(native, /liquibase\/liquibase@sha256:17f98b85e6d4c33b84ee409d055f1526227c77b55dd30e3879ec5088716a32eb/);
  assert.match(native, /sync -f stdout.partial stderr.partial/);
  assert.match(native, /sync -f \./);
  assert.match(native, /\/proc\/uptime/);
});
