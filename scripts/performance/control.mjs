import crypto from 'node:crypto';
import path from 'node:path';

export const fixtures = ['jhipster', 'mobilitydata-user', 'mobilitydata-main'];
export const baselineRevision = 'cab99c4c7ced5b18f4dc43a65b002d84d8fef02d';
export const baselineArchiveSHA = 'da6132c95185f2a11561628458d34b22e7cb928c574d8ecec9f06d44fc8d0f01';
const sha40 = /^[a-f0-9]{40}$/;
const sha64 = /^[a-f0-9]{64}$/;
const digest = /^sha256:[a-f0-9]{64}$/;

/** PA01: event text selects fixed modes, never an executable shell fragment. */
export function selectRun(name, event, repository) {
  let sha, phase;
  if (name === 'pull_request') {
    if (event.action !== 'labeled' || event.pull_request?.head?.repo?.full_name !== repository) return null;
    phase = { 'performance:pilot': 'pilot', 'performance:full': 'full' }[event.label?.name];
    if (!phase) return null;
    sha = event.pull_request.head.sha;
  } else if (name === 'workflow_dispatch') {
    ({ candidate_sha: sha, phase } = event.inputs || {});
  } else return null;
  if (!sha40.test(sha || '') || !['pilot', 'full'].includes(phase)) throw new Error('Invalid performance selection');
  return { sha, phase };
}

/** PA02: bytes must match a declared pin, before extraction or execution. */
export function verifyBytes(bytes, expected) {
  const actual = crypto.createHash('sha256').update(bytes).digest('hex');
  if (!sha64.test(expected) || actual !== expected) throw new Error('Artifact identity mismatch');
  return actual;
}

/** Validate member names; extraction separately rejects links and special files. */
export function safeArchiveNames(names) {
  for (const name of names) {
    if (!name || /[\\\x00-\x1f:]/.test(name) || path.posix.isAbsolute(name) || name.split('/').includes('..')) throw new Error('Unsafe archive member');
  }
  return names;
}

/** PA07: a whole cohort and its controls stay on one machine, serially. */
export function schedule(fixture, phase) {
  if (!fixtures.includes(fixture) || !['comparison', 'confirmation'].includes(phase)) throw new Error('Invalid cohort');
  const result = [];
  for (const kind of ['comparison', 'control']) {
    for (let pair = 1; pair <= 5; pair++) {
      const order = kind === 'control' ? ['v1', 'v1'] : pair % 2 ? ['v1', 'v2'] : ['v2', 'v1'];
      order.forEach((route, side) => result.push({ id: `${phase}-${kind}-${pair}-${side}`, kind, pair, side, route, fixture, phase }));
    }
  }
  return result;
}

/** PA08: estimate wall cost, including untimed work; this is not a guarantee. */
export function checkBudget({ v1MS, v2MS, preparationMS }) {
  if (![v1MS, v2MS].every(x => Number.isFinite(x) && x > 0) || !Number.isFinite(preparationMS) || preparationMS < 0) throw new Error('Invalid budget evidence');
  const reserveMS = 15 * 60 * 1000;
  const estimatedMS = preparationMS + 1.2 * (15 * v1MS + 5 * v2MS) + reserveMS;
  return { estimatedMS, reserveMS, fits: estimatedMS < 6 * 60 * 60 * 1000 };
}

/** PA05/PA10: serialize a closed typed schema, never arbitrary child diagnostics. */
export function sanitizeSample(raw) {
  const steps = { jhipster: 8, 'mobilitydata-user': 12, 'mobilitydata-main': 102 }[raw.Fixture];
  const version = raw.Fixture === 'jhipster' ? '5.0.3' : '5.0.4';
  if (!steps || !['v1', 'v2'].includes(raw.Route) || !['uncached', 'cached'].includes(raw.Pass)
      || raw.Platform !== 'linux/amd64' || raw.SnapshotBackend !== 'copy' || !raw.OracleOK || !raw.CleanupOK
      || raw.Changesets !== steps || raw.CacheHits !== (raw.Pass === 'cached' ? steps : 0)
      || raw.LiquibaseVersion !== version || !Number.isFinite(raw.ElapsedMS) || raw.ElapsedMS <= 0
      || !sha40.test(raw.SourceRevision) || !sha64.test(raw.EngineSHA256)
      || !sha64.test(raw.DriverSHA256) || !sha64.test(raw.CoreSHA256) || !sha64.test(raw.LauncherSHA256)
      || !digest.test(raw.ImageDigest) || (raw.Route === 'v2' && !digest.test(raw.HelperDigest))
      || typeof raw.JavaDetails !== 'string' || raw.JavaDetails.length > 1024 || !raw.JavaDetails.length
      || !/^[\w .+()'",:\-\n=/]+$/.test(raw.JavaDetails) || /:\/\/|password|secret|token|dsn/i.test(raw.JavaDetails)) throw new Error('Invalid benchmark evidence');
  for (const key of ['LiquibaseCLIStarts', 'MigrationCLIStarts', 'ImageInspectionContainers', 'ContainerCreates', 'JVMStartsFromPinnedLauncher']) {
    if (!Number.isSafeInteger(raw[key]) || raw[key] < 0) throw new Error('Invalid benchmark counter');
  }
  const keys = ['Route', 'Fixture', 'Pass', 'EngineSHA256', 'SourceRevision', 'Platform', 'ElapsedMS', 'Changesets',
    'CacheHits', 'LiquibaseCLIStarts', 'MigrationCLIStarts', 'ImageInspectionContainers', 'ContainerCreates',
    'LiquibaseVersion', 'DriverSHA256', 'ImageDigest', 'CoreSHA256', 'LauncherSHA256', 'JVMStartsFromPinnedLauncher',
    'JavaDetails', 'OracleOK', 'CleanupOK', 'SnapshotBackend'];
  const clean = Object.fromEntries(keys.map(key => [key, raw[key]]));
  clean.HelperDigest = raw.Route === 'v2' ? raw.HelperDigest : '';
  return clean;
}

/** Completeness only: never turn successful acquisition into a speed verdict. */
export function validateCohort(records, fixture, phase) {
  const plan = schedule(fixture, phase);
  if (records.length !== plan.length) throw new Error('Incomplete cohort');
  const identities = new Map();
  records.forEach((record, index) => {
    for (const key of ['id', 'kind', 'pair', 'side', 'route', 'fixture', 'phase']) {
      if (record[key] !== plan[index][key]) throw new Error('Cohort order differs');
    }
    if (!record.host || record.host !== records[0].host || !sha40.test(record.revision) || record.revision !== records[0].revision
      || record.samples.length !== 2) throw new Error('Mixed or incomplete cohort');
    record.samples.forEach((raw, passIndex) => {
      const sample = sanitizeSample(raw);
      if (sample.Route !== record.route || sample.Fixture !== fixture || sample.Pass !== ['uncached', 'cached'][passIndex]) throw new Error('Wrong workload');
      const identity = JSON.stringify([sample.EngineSHA256, sample.SourceRevision, sample.ImageDigest, sample.DriverSHA256,
        sample.CoreSHA256, sample.LauncherSHA256, sample.JavaDetails, sample.HelperDigest]);
      if (identities.has(record.route) && identities.get(record.route) !== identity) throw new Error('Artifact changed inside cohort');
      identities.set(record.route, identity);
    });
  });
  return { status: 'collected', fixture, phase, pairs: 5, controls: 5 };
}
