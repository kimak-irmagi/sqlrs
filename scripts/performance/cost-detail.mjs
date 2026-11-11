import { costDefinition } from './cost-catalog.mjs';

const phases = ['preflight', 'cli', 'drain', 'stream_flush', 'hash_rename', 'renamed_flush', 'manifest_flush', 'final_flush'];
const resolution = 10_000_000;
const natural = value => Number.isSafeInteger(value) && value >= 0;
const invalid = () => { throw new Error('Invalid cost collector evidence'); };

/** DT07: independent closed helper/observer qualification. Local coarse-clock
 * detail and observer components never increase/subtract host/API attribution.
 * Drop extras; return only copied public source coordinates and numbers/enums.
 */
export function qualifyCostDetails(root) {
  try {
    if (!root || !Array.isArray(root.Spans) || !Array.isArray(root.Helpers) || !root.Observer) invalid();
    const observer = root.Observer;
    if (![observer.EncodeNS, observer.LockWaitNS, observer.WriteNS].every(natural)) invalid();
    const owners = new Map(root.Spans.filter(span => ['liquibase.invoke', 'liquibase.plan_invoke'].includes(span.Operation))
      .map(span => [span.ID, span]));
    if (owners.size === 0 || root.Helpers.length !== owners.size) invalid();
    const Helpers = root.Helpers.map(helper => {
      const owner = owners.get(helper?.Owner);
      if (!owner || !natural(owner.StartNS) || !natural(owner.EndNS) || owner.EndNS < owner.StartNS ||
          !helper.Available || !natural(helper.HostElapsedNS) || helper.HostElapsedNS > owner.EndNS - owner.StartNS) invalid();
      owners.delete(helper.Owner);
      const detail = helper.Detail;
      if (!detail || detail.ResolutionNS !== resolution || detail.CumulativeUncertaintyNS !== 8 * resolution ||
          !Array.isArray(detail.Regions) || detail.Regions.length !== phases.length) invalid();
      let previous = 0;
      const Regions = detail.Regions.map((region, index) => {
        if (!region || region.Phase !== phases[index] || !natural(region.StartCS) || !natural(region.EndCS) ||
            region.StartCS !== previous || region.EndCS < region.StartCS) invalid();
        const duration = (region.EndCS - region.StartCS) * resolution;
        if (!natural(duration) || !natural(region.EndCS * resolution) || region.DurationNS !== duration) invalid();
        previous = region.EndCS;
        return { Phase: region.Phase, StartCS: region.StartCS, EndCS: region.EndCS, DurationNS: duration };
      });
      const total = previous * resolution;
      if (detail.TotalNS !== total || total > helper.HostElapsedNS && total - helper.HostElapsedNS > resolution) invalid();
      return { Owner: helper.Owner, Operation: owner.Operation, Phase: owner.Phase, Step: owner.Step,
        Source: costDefinition(owner.Operation), HostElapsedNS: helper.HostElapsedNS,
        ResolutionNS: resolution, CumulativeUncertaintyNS: 8 * resolution, TotalNS: total, Regions };
    });
    return { HelperNonAdditive: true, Helpers,
      Observer: { EncodeNS: observer.EncodeNS, LockWaitNS: observer.LockWaitNS, WriteNS: observer.WriteNS,
        Unmeasured: ['report_snapshot_copy', 'observer_marker_construction_and_write'] } };
  } catch {
    invalid();
  }
}
