import { summarizeHostCosts } from './cost-accounting.mjs';

export const maxTimelineAuxSpans = 8193;
const maxOwnerReferences = 4 * 32768;
const natural = value => Number.isSafeInteger(value) && value >= 0;
const interval = (start, end) => natural(start) && natural(end) && end >= start;
const auxiliaryKinds = new Set(['client.post', 'client.status', 'client.poll', 'observer']);
const structural = new Set(['prepare.request', 'prepare.admission', 'prepare.worker', 'binding.prepare', 'db.startup', 'prepare.planning', 'prepare.session_startup',
  'prepare.step', 'prepare.publication', 'prepare.final_cleanup']);
const invalid = () => { throw new Error('Invalid complete timeline evidence'); };

/** TL08: reconcile two real timer windows using their observed brackets, not
 * a fixed guessed tolerance. Never change the original performance sample.
 */
export function timelineTimerDelta(evidence) {
  try {
    if (!evidence || !natural(evidence.APIElapsedNS) || evidence.APIElapsedNS === 0) invalid();
    const { Window: w, StartBracket: s, EndBracket: e, APIElapsedNS: elapsed } = evidence;
    for (const value of [w, s, e]) if (!value || !interval(value.StartNS, value.EndNS)) invalid();
    if (w.StartNS === w.EndNS || s.EndNS > e.StartNS || w.StartNS < s.StartNS || w.StartNS > s.EndNS ||
      w.EndNS < e.StartNS || w.EndNS > e.EndNS || elapsed < e.StartNS - s.EndNS || elapsed > e.EndNS - s.StartNS) invalid();
    return w.EndNS - w.StartNS - elapsed;
  } catch { invalid(); }
}

/** TL09–TL11: independent closed, bounded chronological accounting. Shared
 * purpose is not measured equivalence; unresolved source self remains visible.
 * Inputs stay on their shared reference axis; segments are window-relative.
 * This arithmetic alone does not qualify a real workload or its probe coverage.
 */
export function buildTimeline(input, auxiliary, window) {
  try {
    if (!Array.isArray(input) || input.length > 32768 || !Array.isArray(auxiliary) ||
      auxiliary.length > maxTimelineAuxSpans || !window || !interval(window.StartNS, window.EndNS) || window.StartNS === window.EndNS) invalid();
    const spans = input.map(s => {
      if (!s || !interval(s.StartNS, s.EndNS)) invalid();
      return { ID: s.ID, Parent: s.Parent, Operation: s.Operation, Phase: s.Phase,
        Step: s.Step, StartNS: s.StartNS, EndNS: s.EndNS, Outcome: s.Outcome };
    });
    let last = window.EndNS;
    for (const s of spans) last = Math.max(last, s.EndNS);
    summarizeHostCosts(spans, last);
    const aux = auxiliary.map(s => {
      if (!s || !auxiliaryKinds.has(s.Kind) || !interval(s.StartNS, s.EndNS)) invalid();
      return { Kind: s.Kind, StartNS: s.StartNS, EndNS: s.EndNS };
    });
    for (const observer of [false, true]) {
      const ordered = aux.filter(s => (s.Kind === 'observer') === observer && s.EndNS > s.StartNS)
        .sort((a, b) => a.StartNS - b.StartNS);
      for (let i = 1; i < ordered.length; i++) if (ordered[i].StartNS < ordered[i - 1].EndNS) invalid();
    }
    const byID = new Map(spans.map(s => [s.ID, s]));
    const result = { ElapsedNS: window.EndNS - window.StartNS, UnresolvedNS: 0, OverlapNS: 0, Segments: [], Outside: [] };
    const events = [{ at: window.StartNS }, { at: window.EndNS }];
    const add = (start, end, id, parent, kind) => {
      if (start < window.StartNS && start < end) result.Outside.push({ Kind: kind, Owner: id, StartNS: start, EndNS: Math.min(end, window.StartNS) });
      if (end > window.EndNS && start < end) result.Outside.push({ Kind: kind, Owner: id, StartNS: Math.max(start, window.EndNS), EndNS: end });
      const from = Math.max(start, window.StartNS), to = Math.min(end, window.EndNS);
      if (from < to) events.push({ at: from, id, parent, kind, start: true }, { at: to, id, parent, kind, start: false });
    };
    for (const s of spans) add(s.StartNS, s.EndNS, s.ID, s.Parent, 'engine');
    for (const s of aux) add(s.StartNS, s.EndNS, 0, 0, s.Kind);
    events.sort((a, b) => a.at - b.at || Number(Boolean(a.start)) - Number(Boolean(b.start)));
    const active = new Set(), leaves = new Set(), children = new Map();
    let client = '', observer = false, previous = window.StartNS, references = 0;
    for (let i = 0; i < events.length;) {
      const at = events[i].at;
      if (at > previous) {
        references += leaves.size;
        if (references > maxOwnerReferences) invalid();
        const s = { StartNS: previous - window.StartNS, EndNS: at - window.StartNS,
          Kind: 'unresolved', Owners: [...leaves].sort((a, b) => a - b), Observer: observer };
        if (leaves.size > 1 || leaves.size === 1 && observer) { s.Kind = 'overlap'; result.OverlapNS += at - previous; }
        else if (leaves.size === 1) { if (!structural.has(byID.get(s.Owners[0]).Operation)) s.Kind = 'engine'; }
        else if (observer) s.Kind = 'observer';
        else if (client) s.Kind = client;
        if (s.Kind === 'unresolved') result.UnresolvedNS += at - previous;
        result.Segments.push(s);
      }
      while (i < events.length && events[i].at === at) {
        const e = events[i++];
        if (!e.kind) continue;
        if (e.kind === 'observer') { observer = e.start; continue; }
        if (e.kind !== 'engine') { client = e.start ? e.kind : ''; continue; }
        if (e.start) {
          active.add(e.id);
          if (!(children.get(e.id) ?? 0)) leaves.add(e.id);
          if (e.parent) { children.set(e.parent, (children.get(e.parent) ?? 0) + 1); leaves.delete(e.parent); }
        } else {
          active.delete(e.id); leaves.delete(e.id);
          if (e.parent) {
            children.set(e.parent, children.get(e.parent) - 1);
            if (!children.get(e.parent) && active.has(e.parent)) leaves.add(e.parent);
          }
        }
      }
      previous = at;
    }
    result.Outside.sort((a, b) => a.StartNS - b.StartNS || a.EndNS - b.EndNS ||
      (a.Kind < b.Kind ? -1 : a.Kind > b.Kind ? 1 : 0) || a.Owner - b.Owner);
    return result;
  } catch { invalid(); }
}
