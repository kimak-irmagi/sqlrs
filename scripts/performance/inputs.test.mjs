import assert from 'node:assert/strict';
import test from 'node:test';
import { fixtureSpec, validateSourceHead, validateMetadata } from './inputs.mjs';

test('PA02/PA03: only clean pinned native builds and public fixture commits', () => {
  const sha = 'a'.repeat(40);
  const metadata = `\tbuild\tGOOS=linux\n\tbuild\tGOARCH=amd64\n\tbuild\tvcs.revision=${sha}\n\tbuild\tvcs.modified=false\n`;
  assert.doesNotThrow(() => validateMetadata(metadata, sha));
  for (const bad of [metadata.replace('linux','windows'), metadata.replace('false','true'), metadata.replace(sha,'b'.repeat(40)), '']) assert.throws(() => validateMetadata(bad,sha));
  for (const name of ['jhipster','mobilitydata-user','mobilitydata-main']) {
    const spec=fixtureSpec(name);
    assert.match(spec.repository,/^https:\/\/github.com\/(jhipster|MobilityData)\//);
    assert.equal(validateSourceHead(spec.commit+'\n',name),spec.commit);
    assert.throws(()=>validateSourceHead('b'.repeat(40),name));
  }
  assert.throws(()=>fixtureSpec('izess'));
});
