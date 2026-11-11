import test from 'node:test';
import assert from 'node:assert/strict';
import { timelineTimerDelta } from './complete-timeline.mjs';

const fixture = () => ({ Window: { StartNS: 100, EndNS: 200 },
  StartBracket: { StartNS: 99, EndNS: 101 }, EndBracket: { StartNS: 199, EndNS: 201 }, APIElapsedNS: 100 });

test('TL08: signed timer difference stays inside actual independently supplied brackets', () => {
  for (const elapsed of [98, 99, 100, 101, 102]) {
    const f = fixture(); f.APIElapsedNS = elapsed;
    assert.equal(timelineTimerDelta(f), 100 - elapsed);
    assert.equal(f.APIElapsedNS, elapsed);
  }
  for (const mutate of [
    f => { f.APIElapsedNS = 97; }, f => { f.APIElapsedNS = 103; }, f => { f.APIElapsedNS = 0; },
    f => { f.Window.StartNS = 98; }, f => { f.Window.EndNS = 202; },
    f => { f.StartBracket.StartNS = -1; }, f => { f.StartBracket.EndNS = 98; },
    f => { f.EndBracket.EndNS = 198; }, f => { f.EndBracket.EndNS = Number.MAX_SAFE_INTEGER + 1; },
    f => { f.EndBracket.StartNS = 100; }, f => { f.Window.EndNS = f.Window.StartNS; },
    f => { f.Window = null; }, f => { f.StartBracket = null; }, f => { f.EndBracket = null; },
    f => { f.APIElapsedNS = NaN; },
  ]) {
    const f = fixture(); mutate(f);
    assert.throws(() => timelineTimerDelta(f), error => error.message === 'Invalid complete timeline evidence');
  }
});
