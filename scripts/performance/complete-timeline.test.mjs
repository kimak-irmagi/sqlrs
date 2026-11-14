import test from 'node:test';
import assert from 'node:assert/strict';
import { buildTimeline, maxTimelineAuxSpans } from './complete-timeline.mjs';

// TL09–TL11: independent handwritten expected values, not Go output fixtures.
const span = (ID, Parent, Operation, StartNS, EndNS) => ({ ID, Parent, Operation,
  Phase: 'worker', Step: 0, StartNS, EndNS, Outcome: 'succeeded' });
const vector = () => ({ spans: [span(1, 0, 'prepare.worker', 110, 190),
  span(2, 1, 'capture.read', 120, 150), span(3, 1, 'runner.inventory', 150, 180)],
aux: [{ Kind: 'client.post', StartNS: 100, EndNS: 125 },
  { Kind: 'client.poll', StartNS: 125, EndNS: 195 }, { Kind: 'client.status', StartNS: 195, EndNS: 200 }],
window: { StartNS: 100, EndNS: 200 } });
const amounts = result => result.Segments.reduce((totals, s) => {
  totals[s.Kind] = (totals[s.Kind] ?? 0) + s.EndNS - s.StartNS;
  return totals;
}, {});
function conserve(result) {
  let cursor = 0, total = 0;
  for (const s of result.Segments) {
    assert.equal(s.StartNS, cursor);
    assert.ok(s.EndNS > s.StartNS);
    total += s.EndNS - s.StartNS;
    cursor = s.EndNS;
  }
  assert.equal(cursor, result.ElapsedNS);
  assert.equal(total, result.ElapsedNS);
}

test('TL09: handwritten exact partition excludes inclusive parent/client duplication', () => {
  const f = vector(), original = structuredClone(f);
  const result = buildTimeline(f.spans, f.aux, f.window);
  conserve(result);
  assert.equal(result.ElapsedNS, 100);
  assert.equal(result.UnresolvedNS, 20);
  assert.equal(result.OverlapNS, 0);
  assert.deepEqual(amounts(result), { 'client.post': 10, unresolved: 20, engine: 60, 'client.poll': 5, 'client.status': 5 });
  for (const id of [2, 3]) assert.equal(result.Segments.filter(s => s.Kind === 'engine' && s.Owners[0] === id)
    .reduce((sum, s) => sum + s.EndNS - s.StartNS, 0), 30);
  assert.deepEqual(f, original);
  assert.deepEqual(buildTimeline(f.spans.toReversed(), f.aux.toReversed(), f.window), result);
});

test('TL10: incomparable same-key work and observer cost get one overlap owner set', () => {
  const f = vector();
  f.spans.push(span(4, 1, 'capture.read', 135, 145));
  let result = buildTimeline(f.spans, f.aux, f.window);
  conserve(result);
  assert.equal(result.OverlapNS, 10);
  assert.equal(amounts(result).engine, 50);
  assert.deepEqual(result.Segments.find(s => s.Kind === 'overlap').Owners, [2, 4]);
  f.aux.push({ Kind: 'observer', StartNS: 175, EndNS: 205 });
  result = buildTimeline(f.spans, f.aux, f.window);
  conserve(result);
  assert.equal(result.OverlapNS, 25);
  assert.equal(amounts(result).observer, 10);
  assert.deepEqual(result.Outside, [{ Kind: 'observer', Owner: 0, StartNS: 200, EndNS: 205 }]);
  assert.ok(result.Segments.some(s => s.Kind === 'overlap' && s.Observer));
});

test('TL09/TL11: clip accounting only, preserve tails, unknown self and integer edges', () => {
  const f = vector();
  let result = buildTimeline(f.spans, f.aux, { StartNS: 125, EndNS: 175 });
  conserve(result);
  assert.equal(result.ElapsedNS, 50);
  assert.equal(amounts(result).engine, 50);
  assert.deepEqual(result.Outside, [
    { Kind: 'client.post', Owner: 0, StartNS: 100, EndNS: 125 },
    { Kind: 'engine', Owner: 1, StartNS: 110, EndNS: 125 },
    { Kind: 'engine', Owner: 2, StartNS: 120, EndNS: 125 },
    { Kind: 'engine', Owner: 3, StartNS: 175, EndNS: 180 },
    { Kind: 'engine', Owner: 1, StartNS: 175, EndNS: 190 },
    { Kind: 'client.poll', Owner: 0, StartNS: 175, EndNS: 195 },
    { Kind: 'client.status', Owner: 0, StartNS: 195, EndNS: 200 },
  ]);
  for (const window of [{ StartNS: 0, EndNS: 1 }, { StartNS: Number.MAX_SAFE_INTEGER - 1, EndNS: Number.MAX_SAFE_INTEGER }]) {
    result = buildTimeline([], [], window);
    conserve(result);
    assert.equal(result.UnresolvedNS, 1);
  }
  f.spans[0].Operation = 'prepare.planning';
  f.spans.push(span(4, 1, 'capture.read', 150, 150));
  result = buildTimeline(f.spans, [], f.window);
  conserve(result);
  assert.equal(result.UnresolvedNS, 40);
});

test('TL09–TL11: malformed/private evidence, quotas and partial results are refused', () => {
  const mutations = [
    f => { f.window.EndNS = 100; }, f => { f.window.StartNS = -1; },
    f => { f.window.EndNS = Number.MAX_SAFE_INTEGER + 1; },
    f => { f.spans[1].Operation = 'private-sentinel'; }, f => { f.spans[1].ID = 1; },
    f => { f.spans[1].Parent = 99; }, f => { f.spans[0].Parent = 2; },
    f => { f.spans[1].EndNS = 195; }, f => { f.spans[0].StartNS = -1; },
    f => { f.spans[0].EndNS = Infinity; }, f => { f.spans[1].Phase = 'private-sentinel'; },
    f => { f.spans[1].Outcome = 'private-sentinel'; }, f => { f.spans[1].Step = -1; },
    f => { f.aux[1].StartNS = 124; }, f => { f.aux[0].Kind = 'private-sentinel'; },
    f => { f.aux[0].StartNS = -1; }, f => { f.aux[0].EndNS = 99; },
    f => { f.aux[2].EndNS = Number.MAX_SAFE_INTEGER + 1; },
    f => { f.aux.push({ Kind: 'observer', StartNS: 180, EndNS: 195 }, { Kind: 'observer', StartNS: 190, EndNS: 200 }); },
    f => { f.spans = new Array(32769); }, f => { f.aux = new Array(maxTimelineAuxSpans + 1); },
    f => { f.spans = null; }, f => { f.aux = null; }, f => { f.window = null; },
  ];
  for (const mutate of mutations) {
    const f = vector(); mutate(f);
    assert.throws(() => buildTimeline(f.spans, f.aux, f.window), error => error.message === 'Invalid complete timeline evidence');
  }
});
