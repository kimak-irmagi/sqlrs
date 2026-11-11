import test from 'node:test';
import assert from 'node:assert/strict';
import { costDefinition } from './cost-catalog.mjs';

test('TL11: pinned v1 managed startup already owns identity, lock, assignment, clone and access checks', () => {
  for (const operation of ['db.source_prepare','db.lock_wait','db.base_init','db.assign','db.clone','db.attach','db.verify.startup']) {
    const definition = costDefinition(operation);
    assert.equal(definition.Relation, 'shared');
    assert.equal(definition.Released.File, 'internal/prepare/managed_execution.go');
    assert.equal(definition.Released.Symbol, 'taskExecutor.startManagedRuntime');
    assert.equal(definition.Released.Revision, 'cab99c4c7ced5b18f4dc43a65b002d84d8fef02d');
  }
  assert.equal(costDefinition('admission.identity').Relation, 'shared');
  assert.equal(costDefinition('db.startup').Relation, 'changed');
  assert.equal(costDefinition('db.start').Relation, 'changed');
  for (const operation of ['binding.commit','plan.release','session.plan','runner.stop','capture.read','runner.inventory']) {
    assert.equal(costDefinition(operation).Relation, 'additional');
    assert.equal(costDefinition(operation).Released, null);
  }
});
