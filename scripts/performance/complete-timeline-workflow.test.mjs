import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { parse } from 'yaml';

test('TL12–TL14: Linux probe runs actual timeline and independent collector before the unchanged campaign', () => {
  const probe = parse(fs.readFileSync('.github/workflows/liquibase-cost-probe.yml', 'utf8'));
  const helpers = probe.jobs.probe.steps.find(s => s.name === 'Verify workflow and ordinary helpers');
  assert.match(helpers.run, /GOWORK=off go test \.\/\.\.\./);
  const native = probe.jobs.probe.steps.find(s => s.name === 'Qualify native host attribution and unchanged prepare');
  assert.match(native.run, /\^TestCompleteTimelineMainPathDocker\$/);
  assert.match(native.run, /TestCostMainPathNamedAttributionDocker\|TestMainPathLiquibasePrepareDocker/);
  const campaign = fs.readFileSync('scripts/performance/run-cost.mjs', 'utf8');
  assert.match(campaign, /item.Record \? parseCompleteCostOutput/);
  assert.match(campaign, /costCampaignSchedule\(\)/);
  assert.doesNotMatch(native.run, /run-cost|run-cohort|strace/);
});
