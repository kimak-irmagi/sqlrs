import { parseCollectorResult } from './collector-output.mjs';
import { summarizeHostCosts } from './cost-accounting.mjs';
import { costDefinition } from './cost-catalog.mjs';

const natural = value => Number.isSafeInteger(value) && value >= 0;
const invalid = () => { throw new Error('Invalid cost collector evidence'); };
const sourced = (row, outcomes) => ({ ...row, Source: costDefinition(row.Operation), Outcomes: { ...outcomes } });

/** CT10/CT11: only a successful, independently qualified public workload may
 * publish source-linked costs. API timing stays the denominator. ObserverWaitNS
 * is post-timer wait only, not claimed serialization or recording overhead.
 * Extra/private child fields are dropped; invalid evidence is never partial.
 */
export function parseCostOutput(code, output, expected) {
  try {
    const samples = parseCollectorResult(code, output, expected);
    if (expected.route !== 'v2' || expected.revision === '668d5050ad154c1967bea4c6e40b91f3a703d5ba') invalid();
    const costs = output.split(/\r?\n/).filter(line => line.includes('P01B_COST '))
      .map(line => JSON.parse(line.slice(line.indexOf('P01B_COST ') + 10)));
    if (costs.length !== samples.length) invalid();
    return costs.map((cost, index) => {
      const sample = samples[index], root = cost.Root;
      if (cost.Pass !== sample.Pass || !natural(cost.PrepareElapsedNS) || cost.PrepareElapsedNS === 0 ||
          cost.PrepareElapsedNS !== Math.round(sample.ElapsedMS * 1e6) || !natural(cost.ObserverWaitNS) ||
          !root || root.Root !== index + 1 || root.Complete !== true || root.Ended !== true ||
          !natural(root.ElapsedNS) || root.ElapsedNS > cost.PrepareElapsedNS ||
          !Array.isArray(root.Spans) || root.Spans.length === 0 || root.Declared !== root.Spans.length) invalid();
      const worker = root.Spans[0];
      if (worker.ID !== 1 || worker.Parent !== 0 || worker.Operation !== 'prepare.worker' || worker.Phase !== 'worker' ||
          worker.Step !== 0 || worker.Outcome !== 'succeeded' || worker.EndNS !== root.ElapsedNS) invalid();
      for (let i = 1; i < root.Spans.length; i++) {
        const span = root.Spans[i];
        if (span.ID !== i + 1 || span.Parent <= 0 || span.Parent >= span.ID || span.Step > sample.Changesets) invalid();
      }
      const accounting = summarizeHostCosts(root.Spans, cost.PrepareElapsedNS);
      const scopedOutcomes = new Map(), overallOutcomes = new Map();
      const scopeKey = span => `${span.Operation}|${span.Phase}|${span.Step}`;
      for (const span of root.Spans) {
        for (const [map, key] of [[scopedOutcomes, scopeKey(span)], [overallOutcomes, span.Operation]]) {
          if (!map.has(key)) map.set(key, { succeeded: 0, failed: 0, cancelled: 0, panicked: 0 });
          map.get(key)[span.Outcome]++;
        }
      }
      const scoped = row => sourced(row, scopedOutcomes.get(scopeKey(row)));
      // Re-aggregate identical validated intervals by operation only. Flattening
      // phase/ordinal is a grouping device, not a claimed new measured phase.
      const overall = summarizeHostCosts(root.Spans.map(span => ({ ...span, Phase: 'worker', Step: 0 })), cost.PrepareElapsedNS)
        .Rows.map(({ Phase, Step, ...row }) => sourced(row, overallOutcomes.get(row.Operation)));
      const steps = Array.from({ length: sample.Changesets }, (_, index) => {
        const step = index + 1;
        const applies = root.Spans.filter(span => span.Operation === 'liquibase.invoke' && span.Step === step);
        if (applies.length !== (sample.Pass === 'uncached' ? 1 : 0)) invalid();
        return { Step: step, ApplyCalls: applies.length,
          Rows: accounting.Rows.filter(row => row.Phase === 'step' && row.Step === step).map(scoped) };
      });
      return { Pass: sample.Pass, Fixture: sample.Fixture, SourceRevision: sample.SourceRevision,
        EngineSHA256: sample.EngineSHA256, PrepareElapsedNS: cost.PrepareElapsedNS, WorkerElapsedNS: root.ElapsedNS,
        ObserverWaitNS: cost.ObserverWaitNS, Accounting: { ...accounting, Rows: accounting.Rows.map(scoped) },
        OverallNonAdditive: true, Overall: overall, Steps: steps };
    });
  } catch {
    // Even JSON/lookup/shape errors may contain private source text. Retain no
    // raw error/message/record, including reports preceding a later failure.
    invalid();
  }
}
