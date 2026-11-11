import { parseCollectorResult } from './collector-output.mjs';
import { summarizeHostCosts } from './cost-accounting.mjs';
import { buildTimeline, timelineTimerDelta } from './complete-timeline.mjs';
import { qualifyCostDetails } from './cost-detail.mjs';
import { costDefinition } from './cost-catalog.mjs';

const natural = n => Number.isSafeInteger(n) && n >= 0;
const invalid = () => { throw new Error('Invalid complete cost evidence'); };
const clientKind = s => ['client.post', 'client.status', 'client.poll'].includes(s);

/** TL08–TL11: independent opt-in qualifier. Preserve the old API denominator,
 * shared-clock window, complete source tree and all tails separately. Only the
 * chronological ledger and its Additive grouping may be summed; inclusive
 * host/helper rows are explicitly non-additive. Drop all unrecognised fields.
 */
export function parseCompleteCostOutput(code, output, expected) {
  try {
    const samples = parseCollectorResult(code, output, expected);
    if (expected.route !== 'v2' || expected.revision === '668d5050ad154c1967bea4c6e40b91f3a703d5ba') invalid();
    const costs = output.split(/\r?\n/).filter(line => line.includes('P01B_COST '))
      .map(line => JSON.parse(line.slice(line.indexOf('P01B_COST ') + 10)));
    if (costs.length !== samples.length) invalid();
    return costs.map((cost, index) => {
      const sample = samples[index], root = cost.Root, evidence = cost.Timeline;
      if (cost.Pass !== sample.Pass || !natural(cost.PrepareElapsedNS) || cost.PrepareElapsedNS === 0 ||
        cost.PrepareElapsedNS !== Math.round(sample.ElapsedMS*1e6) || !natural(cost.ObserverWaitNS) ||
        !root || root.Root !== index+1 || root.Complete !== true || root.Ended !== true || !natural(root.ElapsedNS) || root.ElapsedNS === 0 ||
        !Array.isArray(root.Spans) || root.Spans.length < 3 || root.Declared !== root.Spans.length ||
        !evidence || evidence.Timer?.APIElapsedNS !== cost.PrepareElapsedNS) invalid();
      const delta = timelineTimerDelta(evidence.Timer), window = evidence.Timer.Window;
      if (evidence.ClockDeltaNS !== delta || root.Clock?.Domain !== 'linux-monotonic-relative-v1' ||
        !natural(root.Clock.OriginNS) || root.Clock.OriginNS < window.StartNS || root.Clock.OriginNS >= window.EndNS ||
        !natural(root.Clock.OriginNS+root.ElapsedNS) || !root.ObserverWindow ||
        !natural(root.ObserverWindow.StartNS) || !natural(root.ObserverWindow.EndNS) ||
        root.ObserverWindow.StartNS < root.Clock.OriginNS+root.ElapsedNS || root.ObserverWindow.EndNS < root.ObserverWindow.StartNS) invalid();
      const first = root.Spans[0];
      if (first.ID !== 1 || first.Parent !== 0 || first.Operation !== 'prepare.request' || first.Phase !== 'worker' || first.Step !== 0 ||
        first.Outcome !== 'succeeded' || first.EndNS !== root.ElapsedNS) invalid();
      for (let i = 1; i < root.Spans.length; i++) {
        const span = root.Spans[i];
        if (span.ID !== i+1 || span.Parent <= 0 || span.Parent >= span.ID || span.Step > sample.Changesets) invalid();
      }
      const participants = operation => {
        const rows = root.Spans.filter(s => s.Operation === operation);
        if (rows.length !== 1 || rows[0].Parent !== 1 || rows[0].Step !== 0 || rows[0].Outcome !== 'succeeded' ||
          rows[0].Phase !== (operation === 'prepare.admission' ? 'admission' : 'worker')) invalid();
        return rows[0];
      };
      participants('prepare.admission');
      const worker = participants('prepare.worker');
      const accounting = summarizeHostCosts(root.Spans, root.ElapsedNS);
      if (!Array.isArray(evidence.Client) || evidence.Client.length < 2 || evidence.Client.length > 8192) invalid();
      let previous = window.StartNS;
      const client = evidence.Client.map(s => {
        if (!s || !clientKind(s.Kind) || !natural(s.StartNS) || !natural(s.EndNS) || s.StartNS < previous ||
          s.EndNS < s.StartNS || s.EndNS > window.EndNS) invalid();
        previous = s.EndNS;
        return { Kind: s.Kind, StartNS: s.StartNS, EndNS: s.EndNS };
      });
      if (client[0].Kind !== 'client.post' || client.at(-1).Kind !== 'client.status' ||
        client.filter(s => s.Kind === 'client.post').length !== 1) invalid();
      const placed = root.Spans.map(s => ({ ...s, StartNS: root.Clock.OriginNS+s.StartNS, EndNS: root.Clock.OriginNS+s.EndNS }));
      const ledger = buildTimeline(placed, [...client, { Kind: 'observer', ...root.ObserverWindow }], window);
      const owner = id => {
        const s = root.Spans[id-1];
        return { ID: s.ID, Parent: s.Parent, Operation: s.Operation, Phase: s.Phase, Step: s.Step, Source: costDefinition(s.Operation) };
      };
      const additive = new Map();
      const segments = ledger.Segments.map(s => {
        const duration = s.EndNS-s.StartNS, owners = s.Owners.map(owner);
        const operation = owners.length === 1 && !s.Observer ? owners[0].Operation : '';
        const phase = owners.length === 1 ? owners[0].Phase : '', step = owners.length === 1 ? owners[0].Step : 0;
        const key = `${s.Kind}|${operation}|${phase}|${step}`;
        if (!additive.has(key)) additive.set(key, { Kind: s.Kind, Operation: operation, Phase: phase, Step: step, DurationNS: 0,
          ...(operation ? { Source: costDefinition(operation) } : {}) });
        additive.get(key).DurationNS += duration;
        return { StartNS: s.StartNS, EndNS: s.EndNS, DurationNS: duration, Kind: s.Kind, Owners: owners, Observer: s.Observer };
      });
      if (segments.reduce((sum, s) => sum+s.DurationNS, 0) !== ledger.ElapsedNS) invalid();
      const steps = Array.from({ length: sample.Changesets }, (_, i) => {
        const Step = i+1, applies = root.Spans.filter(s => s.Operation === 'liquibase.invoke' && s.Step === Step);
        if (applies.length !== (sample.Pass === 'uncached' ? 1 : 0)) invalid();
        return { Step, ApplyCalls: applies.length, Additive: [...additive.values()].filter(s => s.Step === Step) };
      });
      return { Pass: sample.Pass, Fixture: sample.Fixture, SourceRevision: sample.SourceRevision, EngineSHA256: sample.EngineSHA256,
        PrepareElapsedNS: cost.PrepareElapsedNS, RequestElapsedNS: first.EndNS-first.StartNS, WorkerElapsedNS: worker.EndNS-worker.StartNS,
        ObserverWaitNS: cost.ObserverWaitNS, ClockDomain: root.Clock.Domain, ClockDeltaNS: delta,
        Timer: { APIElapsedNS: evidence.Timer.APIElapsedNS,
          Window: { StartNS: window.StartNS, EndNS: window.EndNS },
          StartBracket: { StartNS: evidence.Timer.StartBracket.StartNS, EndNS: evidence.Timer.StartBracket.EndNS },
          EndBracket: { StartNS: evidence.Timer.EndBracket.StartNS, EndNS: evidence.Timer.EndBracket.EndNS } },
        SourceSpans: placed.map(s => ({ ...owner(s.ID), Outcome: s.Outcome, StartNS: s.StartNS-window.StartNS, EndNS: s.EndNS-window.StartNS })),
        Timeline: { ElapsedNS: ledger.ElapsedNS, UnresolvedNS: ledger.UnresolvedNS, OverlapNS: ledger.OverlapNS, Segments: segments,
          Outside: ledger.Outside.map(s => ({ Kind: s.Kind, StartNS: s.StartNS-window.StartNS, EndNS: s.EndNS-window.StartNS,
            Owner: s.Owner ? owner(s.Owner) : null })) },
        Additive: [...additive.values()].map(s => ({ ...s, ZTotalMS: s.DurationNS/1e6, Zratio: s.DurationNS/ledger.ElapsedNS*100 })),
        HostNonAdditive: true, HostRows: accounting.Rows.map(row => ({ ...row, Zratio: row.ZTotalMS*1e6/cost.PrepareElapsedNS*100, Source: costDefinition(row.Operation) })),
        Steps: steps, ...qualifyCostDetails(root) };
    });
  } catch { invalid(); }
}
