// CT11: independent observer arithmetic. Host rows are non-additive; only
// exclusive leaf ownership + shared overlap + unassigned conserve API elapsed.
// Requirements: issue89-cost-attribution-and-trimming.md §4.
const operations = new Set([
  'prepare.worker', 'runner.inventory', 'capture.read', 'capture.prune', 'docker.command', 'docker.capture_copy',
  'prepare.planning', 'prepare.session_startup', 'prepare.step', 'liquibase.invoke', 'liquibase.plan_invoke',
  'db.prepare_snapshot', 'db.resume.step', 'db.resume.publication', 'db.verify.after_apply', 'db.verify.after_resume',
  'state.snapshot', 'state.size', 'state.seal', 'journal.state_publish',
  'journal.invocation_plan', 'journal.invocation_admit', 'journal.invocation_complete',
  'prepare.publication', 'prepare.final_cleanup',
]);
const phases = new Set(['worker', 'startup', 'planning', 'step', 'publication', 'cleanup']);
const outcomes = new Set(['succeeded', 'failed', 'cancelled', 'panicked']);
const invalid = () => { throw new Error('Invalid host cost evidence'); };
const natural = value => Number.isSafeInteger(value) && value >= 0;
const plus = (a, b) => { const sum = a + b; if (!natural(sum)) invalid(); return sum; };

// Return disjoint caller-owned intervals; no per-nanosecond or pairwise scan.
function merge(intervals) {
  const ordered = intervals.map(pair => [...pair]).sort((a, b) => a[0] - b[0] || a[1] - b[1]);
  const result = [];
  for (const [start, end] of ordered) {
    if (start === end) continue;
    const last = result.at(-1);
    if (!last || last[1] < start) result.push([start, end]);
    else last[1] = Math.max(last[1], end);
  }
  return result;
}
const union = intervals => merge(intervals).reduce((sum, [start, end]) => plus(sum, end - start), 0);

/** Validate complete host spans and retain only closed numeric/enum fields.
 * This helper never invents timestamps, Java internals or a SQLite residual.
 * It accepts failed intermediate calls; their parent prepare may still succeed.
 */
export function summarizeHostCosts(input, elapsedNS) {
  if (!Array.isArray(input) || input.length > 32768 || !natural(elapsedNS) || elapsedNS === 0) invalid();
  const spans = input.map(span => {
    if (!span || !natural(span.ID) || span.ID === 0 || !natural(span.Parent) || !operations.has(span.Operation) ||
        !phases.has(span.Phase) || !outcomes.has(span.Outcome) || !natural(span.Step) ||
        (span.Phase === 'step' ? span.Step === 0 : span.Step !== 0) ||
        !natural(span.StartNS) || !natural(span.EndNS) || span.EndNS < span.StartNS || span.EndNS > elapsedNS) invalid();
    return { ID: span.ID, Parent: span.Parent, Operation: span.Operation, Phase: span.Phase,
      Step: span.Step, StartNS: span.StartNS, EndNS: span.EndNS };
  }).sort((a, b) => a.ID - b.ID);
  const byID = new Map(), children = new Map();
  for (const span of spans) {
    if (byID.has(span.ID)) invalid();
    byID.set(span.ID, span);
  }
  for (const span of spans) {
    if (!span.Parent) continue;
    const parent = byID.get(span.Parent);
    if (!parent || span.StartNS < parent.StartNS || span.EndNS > parent.EndNS ||
        parent.Step > 0 && (span.Step !== parent.Step || span.Phase !== parent.Phase)) invalid();
    if (!children.has(span.Parent)) children.set(span.Parent, []);
    children.get(span.Parent).push([span.StartNS, span.EndNS]);
  }
  // Iterative cycle proof also supports maximum-depth trees without JS stack
  // recursion. A previously proven path can be shared by many later children.
  const proven = new Set();
  for (const span of spans) {
    const path = new Set();
    let id = span.ID;
    while (id && !proven.has(id)) {
      if (path.has(id)) invalid();
      path.add(id);
      id = byID.get(id).Parent;
    }
    for (const id of path) proven.add(id);
  }
  const groups = new Map(), events = [];
  for (const span of spans) {
    const key = `${span.Operation}|${span.Phase}|${span.Step}`;
    if (!groups.has(key)) groups.set(key, { Operation: span.Operation, Phase: span.Phase, Step: span.Step,
      Count: 0, SumNS: 0, CallNS: [], ExclusiveNS: 0, inclusive: [], own: [] });
    const row = groups.get(key);
    row.Count++;
    const duration = span.EndNS - span.StartNS;
    row.SumNS = plus(row.SumNS, duration);
    row.CallNS.push(duration);
    row.inclusive.push([span.StartNS, span.EndNS]);
    let cursor = span.StartNS;
    const own = [];
    for (const [start, end] of merge(children.get(span.ID) ?? [])) {
      if (cursor < start) own.push([cursor, start]);
      cursor = Math.max(cursor, end);
    }
    if (cursor < span.EndNS) own.push([cursor, span.EndNS]);
    for (const pair of own) {
      row.own.push(pair);
      events.push({ at: pair[0], id: span.ID, row, start: true }, { at: pair[1], id: span.ID, row, start: false });
    }
  }
  // Own segments already exclude descendants. Concurrent leaf identities,
  // including two calls of the same key, must go to the shared bucket.
  events.sort((a, b) => a.at - b.at || Number(a.start) - Number(b.start));
  const active = new Map();
  let previous = 0, assigned = 0, shared = 0;
  for (let i = 0; i < events.length;) {
    const at = events[i].at, duration = at - previous;
    if (active.size) {
      assigned = plus(assigned, duration);
      if (active.size > 1) shared = plus(shared, duration);
      else { const row = active.values().next().value; row.ExclusiveNS = plus(row.ExclusiveNS, duration); }
    }
    while (i < events.length && events[i].at === at) {
      const event = events[i++];
      if (event.start) active.set(event.id, event.row);
      else active.delete(event.id);
    }
    previous = at;
  }
  const rows = [...groups.values()].map(({ inclusive, own, ...row }) => {
    const unionNS = union(inclusive), selfNS = union(own);
    return { ...row, UnionNS: unionNS, SelfNS: selfNS, MeanPerCallMS: row.SumNS / row.Count / 1e6,
      ZTotalMS: unionNS / 1e6, Zratio: unionNS / elapsedNS * 100 };
  }).sort((a, b) => (a.Operation < b.Operation ? -1 : a.Operation > b.Operation ? 1 : 0) ||
    (a.Phase < b.Phase ? -1 : a.Phase > b.Phase ? 1 : 0) || a.Step - b.Step);
  return { PrepareElapsedNS: elapsedNS, AssignedNS: assigned, SharedNS: shared,
    UnassignedNS: elapsedNS - assigned, RowsNonAdditive: true, Rows: rows };
}
