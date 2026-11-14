import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

// IC07: reject wrong real event counts through the independently tested Go
// predicate, rather than leave that predicate disconnected from collection.
test('IC07: actual collector checks observed probe count against predeclared policy',()=>{
  const source=fs.readFileSync('test/performance/liquibase/collector_linux_test.go','utf8');
  assert.match(source,/count, inspections, err := creates\.count\(/);
  assert.match(source,/if !inventoryProbeCountMatches\(inspections, probeExpectations, pass\)/);
  assert.ok(source.indexOf('inventoryProbeExpectations(')<source.indexOf('for _, pass := range'));
  assert.ok(source.indexOf('inventoryProbeCountMatches(')<source.indexOf('sample := releasedBenchmarkSample'));
});
