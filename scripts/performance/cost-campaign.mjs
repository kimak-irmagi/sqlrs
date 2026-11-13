// CT13: fixed, untraced before/after/off/on campaign. This is not the P01b
// acceptance verdict; no timings or source pins are inferred from labels.
export const costBeforeRevision = '668d5050ad154c1967bea4c6e40b91f3a703d5ba';
export const costBeforeEngineSHA = '1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0';
const baselineRevision = 'cab99c4c7ced5b18f4dc43a65b002d84d8fef02d';
const baselineHash = 'b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca';
const sha40 = value => typeof value === 'string' && /^[a-f0-9]{40}$/.test(value);
const sha64 = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);

export function costCampaignSelection(name, event, repository) {
  if (name !== 'pull_request' || event?.action !== 'labeled' || event?.label?.name !== 'performance:cost-attribution' ||
      event?.pull_request?.head?.repo?.full_name !== repository) return null;
  const sha = event.pull_request.head.sha;
  if (!sha40(sha)) throw new Error('Invalid cost campaign selection');
  return sha;
}

/** Check both preparations and export identities only, never private paths. */
export function costCampaignIdentity(before, after, selected) {
  if (!before || !after || before.revision !== costBeforeRevision || before.engineHash !== costBeforeEngineSHA ||
      before.baselineRevision !== baselineRevision || before.baselineHash !== baselineHash ||
      !sha40(selected) || after.revision !== selected || after.revision === costBeforeRevision || after.revision === baselineRevision ||
      !sha64(after.engineHash) || [costBeforeEngineSHA, baselineHash].includes(after.engineHash) ||
      after.baselineRevision !== baselineRevision || after.baselineHash !== baselineHash) throw new Error('Invalid cost campaign identity');
  return { BeforeRevision: before.revision, BeforeEngineSHA256: before.engineHash,
    Revision: after.revision, EngineSHA256: after.engineHash, BaselineRevision: baselineRevision, BaselineSHA256: baselineHash };
}

export function costCampaignSchedule() {
  return ['released_v1', 'old_v2', 'new_off', 'new_on', 'new_on', 'new_off', 'old_v2', 'released_v1']
    .map((Variant, index) => ({ ID: index + 1, Variant, Route: Variant === 'released_v1' ? 'v1' : 'v2', Record: Variant === 'new_on' }));
}

/** Clear inherited selectors; only the exact scheduled new-on member records. */
export function costCampaignEnvironment(input, member) {
  const expected = costCampaignSchedule()[member?.ID - 1];
  if (!expected || Object.keys(expected).some(key => member[key] !== expected[key])) throw new Error('Invalid cost campaign member');
  return { ...input, SQLRS_PREPARE_COST: '', SQLRS_PREPARE_TIMELINE: '', SQLRS_PREPARE_TIMELINE_REFERENCE: '',
    SQLRS_BENCH_IMAGE_PROBE_POLICY: ['new_off','new_on'].includes(member.Variant) ? 'per-process-v1' : '',
    SQLRS_BENCH_IMAGE_PROBE_VARIANT: ['new_off','new_on'].includes(member.Variant) ? member.Variant : '',
    SQLRS_BENCH_RESULT_POLICY: ['new_off','new_on'].includes(member.Variant) ? 'same-exec-v1' : '',
    SQLRS_BENCH_RESULT_VARIANT: ['new_off','new_on'].includes(member.Variant) ? member.Variant : '',
    SQLRS_BENCH_TIMELINE: member.Record ? '1' : '0', SQLRS_BENCH_DIAGNOSTIC: '', SQLRS_BENCH_TRACE: '',
    SQLRS_BENCH_COST: member.Record ? '1' : '0', SQLRS_BENCH_PROBE: '', SQLRS_BENCH_RETAIN_PROBE: '' };
}
