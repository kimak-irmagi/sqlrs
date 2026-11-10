import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const collector = () => fs.readFileSync('test/performance/liquibase/collector_linux_test.go', 'utf8');

test('CT10/CT13/OT-CT03: private cost reader waits after the unchanged API timer', () => {
  const source = collector();
  assert.match(source, /costEnabled := os\.Getenv\("SQLRS_BENCH_COST"\) == "1"/);
  assert.match(source, /validateCostSelection\(route, revision, costEnabled, diagnostic\)/);
  assert.match(source, /child\.Env = costEngineEnvironment\(child\.Env, costEnabled\)/);
  const loop = source.slice(source.indexOf('for _, pass := range []string{"uncached", "cached"}'));
  const start = loop.indexOf('start := time.Now()');
  const request = loop.indexOf('call(http.MethodPost, "/v1/prepare-jobs"');
  const status = loop.indexOf('status.Status == StatusSucceeded');
  const end = loop.indexOf('end := time.Now()');
  const awaitCost = loop.indexOf('costReader.await(');
  const oracle = loop.indexOf('stage = "oracle"');
  assert.ok(start >= 0 && start < request && request < status && status < end);
  assert.ok(end < awaitCost && awaitCost < oracle, 'observer cannot extend original API timing');
  assert.match(loop, /context\.WithTimeout\(ctx, 10\*time.Second\)/);
  assert.match(loop, /qualifyCostPass\(observation, costOrdinal, end.Sub\(start\).Nanoseconds\(\), expectedChanges\)/);
});

test('CT10/OT-CT03: reader belongs to existing exact process/file cleanup', () => {
  const source = collector();
  assert.match(source, /costPipe, costWriter, err = os.Pipe\(\)/);
  assert.match(source, /costDone <- costReader.consume\(costPipe\)/);
  const cleanup = source.slice(source.indexOf('enginePID := 0'), source.indexOf('var state struct'));
  const process = cleanup.indexOf('case <-wait:');
  const reader = cleanup.indexOf('case readErr := <-costDone:');
  const files = cleanup.indexOf('cleanupPrivateRoot(');
  assert.ok(process >= 0 && process < reader && reader < files);
  assert.match(cleanup, /case <-time.After\(10 \* time.Second\):/);
  assert.match(cleanup, /costPipe.Close\(\)/);
  assert.match(source, /P01B_COST %s/);
  assert.doesNotMatch(source, /Logf\([^\n]*(costPipe|readErr|observation\.Spans)/);
});
