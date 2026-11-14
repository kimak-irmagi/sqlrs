import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { costCatalog, costDefinition } from './cost-catalog.mjs';

test('CT01: report catalog agrees with every emitted source key, without trusting child metadata', () => {
  const source = fs.readFileSync('backend/local-engine-go/internal/preparecost/catalog.go', 'utf8');
  const keys = [...source.matchAll(/\bOperation\s*=\s*"([a-z_.]+)"/g)].map(match => match[1]).sort();
  const catalog = costCatalog();
  assert.deepEqual(catalog.map(item => item.Operation).sort(), keys);
  assert.equal(new Set(keys).size, keys.length);
  for (const entry of catalog) {
    assert.deepEqual(costDefinition(entry.Operation), entry);
    assert.match(entry.Current.File, /^internal\/[a-z_/]+\.go$/);
    const current = fs.readFileSync(`backend/local-engine-go/${entry.Current.File}`, 'utf8');
    assert.ok(current.includes(entry.Current.Symbol.split('.').at(-1) + '('), 'source symbol must exist');
    if (entry.Relation === 'additional') assert.equal(entry.Released, null);
    else {
      assert.match(entry.Released.File, /^internal\/[a-z_/]+\.go$/);
      assert.ok(entry.Released.Symbol);
      assert.equal(entry.Released.Revision, 'cab99c4c7ced5b18f4dc43a65b002d84d8fef02d');
    }
  }
});

test('CT01: shared/shifted DB work is not counted as wholly additional v2 work', () => {
  for (const operation of ['docker.command', 'db.prepare_snapshot', 'db.verify.after_apply', 'state.snapshot', 'state.size', 'state.seal']) {
    assert.equal(costDefinition(operation).Relation, 'shared');
  }
  for (const operation of ['db.resume.step', 'db.resume.publication']) assert.equal(costDefinition(operation).Relation, 'shifted');
  for (const operation of ['capture.read', 'capture.prune', 'runner.inventory', 'db.verify.after_resume', 'journal.invocation_plan']) {
    assert.equal(costDefinition(operation).Relation, 'additional');
  }
  assert.equal(costDefinition('liquibase.invoke').Relation, 'changed');
  assert.equal(costDefinition('liquibase.invoke').Released.Symbol, 'taskExecutor.executeLiquibaseStep');
});

test('CT01/CT10: catalog copies cannot be mutated or extended with private text', () => {
  const before = costCatalog();
  const changed = costCatalog();
  changed[0].Current.Symbol = 'private-marker';
  changed.splice(1);
  const single = costDefinition('db.prepare_snapshot');
  single.Released.Revision = 'private-marker';
  assert.deepEqual(costCatalog(), before);
  for (const key of ['', null, 'private-marker', 'capture.read\nprivate-marker']) {
    assert.throws(() => costDefinition(key), error => error.message === 'Unknown cost source key');
  }
});
