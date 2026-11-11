import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { spawn, execFileSync } from 'node:child_process';
import { costCampaignSelection, costCampaignIdentity, costCampaignSchedule, costCampaignEnvironment, costBeforeRevision, costBeforeEngineSHA } from './cost-campaign.mjs';
import { verifyBytes } from './control.mjs';
import { parseCollectorResult, failureKind, cleanupBoundary } from './collector-output.mjs';
import { parseCostOutput } from './cost-output.mjs';
import { fixtureSpec } from './inputs.mjs';
import { installJdbcDriver } from '../jdbc-driver-bundle.mjs';
import { warm } from './warm.mjs';

// CT13 integration orchestration, not a pure helper coverage claim. Execute
// only in explicit hosted Actions; child output remains bounded/private.
const repo = path.resolve(process.env.GITHUB_WORKSPACE || '.');
const root = path.join(repo, '.tmp/performance');
const beforeRoot = path.join(repo, '.tmp/performance-before');
const readJSON = file => JSON.parse(fs.readFileSync(file, 'utf8'));
const sha256 = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const git = args => execFileSync('git', args, { cwd: repo, encoding: 'utf8', stdio: ['ignore','pipe','pipe'], timeout: 10000 }).trim();
let active, report, reportFile, cancelled = false;
const persist = () => fs.writeFileSync(reportFile, JSON.stringify(report, null, 2)+'\n', { mode: 0o600 });
const cancel = () => { cancelled = true; active?.kill('SIGTERM'); };
process.on('SIGTERM', cancel); process.on('SIGINT', cancel);
try {
  if (process.platform !== 'linux' || process.arch !== 'x64' || process.env.GITHUB_ACTIONS !== 'true') throw new Error('Unsupported cost host');
  const selected = costCampaignSelection(process.env.GITHUB_EVENT_NAME, readJSON(process.env.GITHUB_EVENT_PATH), process.env.GITHUB_REPOSITORY);
  if (!selected || git(['status','--porcelain','--untracked-files=no'])) throw new Error('Unqualified cost checkout');
  const prepared = readJSON(path.join(root, 'prepared/artifacts.json'));
  if (process.argv[2] === 'preserve-before') {
    if (git(['rev-parse','HEAD']) !== costBeforeRevision || prepared.revision !== costBeforeRevision || prepared.engineHash !== costBeforeEngineSHA ||
        path.resolve(prepared.engine) !== path.join(root,'prepared/candidate/sqlrs-engine')) throw new Error('Old artifact identity differs');
    verifyBytes(fs.readFileSync(prepared.engine), costBeforeEngineSHA);
    fs.mkdirSync(beforeRoot, { mode: 0o700 });
    const retained = path.join(beforeRoot, 'sqlrs-engine');
    fs.copyFileSync(prepared.engine, retained, fs.constants.COPYFILE_EXCL);
    installJdbcDriver(repo, beforeRoot);
    verifyBytes(fs.readFileSync(retained), costBeforeEngineSHA);
    fs.writeFileSync(path.join(beforeRoot,'artifacts.json'), JSON.stringify({ ...prepared, engine: retained }), { mode: 0o600 });
    console.log('Verified old executable retained outside timing');
  } else {
    if (process.argv.length !== 2 || git(['rev-parse','HEAD']) !== selected) throw new Error('New checkout differs');
    const before = readJSON(path.join(beforeRoot,'artifacts.json'));
    const identity = costCampaignIdentity(before, prepared, selected);
    const reportDir = path.join(root,'cost-reports');
    fs.mkdirSync(reportDir, { recursive: true, mode: 0o700 });
    reportFile = path.join(reportDir,'jhipster-cost.json');
    report = { status: 'incomplete', kind: 'cost_attribution_not_acceptance', host: crypto.randomUUID(),
      run: process.env.GITHUB_RUN_ID || 'unqualified', platform: 'linux/amd64', runnerImage: process.env.ImageVersion || 'unknown',
      cpu: os.cpus()[0]?.model, ...identity, CollectorSHA256: sha256(path.join(root,'prepared/collector.test')), records: [] };
    persist();
    const started = Number(process.env.PERFORMANCE_JOB_STARTED_MS);
    if (!Number.isFinite(started) || started <= 0 || started > Date.now()) throw new Error('Invalid cost budget');
    const deadline = started + 50 * 60 * 1000;
    report.stage = 'warmup'; persist();
    warm(repo, ['jhipster']);
    const spec = fixtureSpec('jhipster');
    report.stage = 'image_prerequisite'; persist();
    const prerequisite = execFileSync('go', ['test','./internal/runtime','-run','^TestCostJHipsterImagePrerequisitesDocker$','-v','-count=1','-timeout=2m'],
      { cwd: path.join(repo,'backend/local-engine-go'), env: { ...process.env, SQLRS_COST_NATIVE_PROBE: '1' }, stdio: ['ignore','pipe','pipe'], encoding: 'utf8', timeout: 150000 });
    if (!prerequisite.includes('--- PASS: TestCostJHipsterImagePrerequisitesDocker (')) throw new Error('Image prerequisite did not run');
    report.ImagePrerequisiteOK = true; persist();
    for (const item of costCampaignSchedule()) {
      report.activeMember = item; persist();
      report.stage = 'collect'; persist();
      if (cancelled || Date.now() >= deadline) throw new Error('Cost budget exhausted');
      const artifact = item.Variant === 'old_v2' ? before : prepared;
      const engine = item.Route === 'v1' ? prepared.baseline : artifact.engine;
      const engineHash = item.Route === 'v1' ? prepared.baselineHash : artifact.engineHash;
      const revision = item.Route === 'v1' ? prepared.baselineRevision : artifact.revision;
      verifyBytes(fs.readFileSync(engine), engineHash);
      const output = await new Promise((resolve, reject) => {
        let output = '', overflow = false, launchFailed = false;
        active = spawn(path.join(root,'prepared/collector.test'), ['-test.run=^TestReleasedEngine$','-test.v','-test.timeout=12m'], {
          cwd: path.join(repo,'test/performance/liquibase'), stdio: ['ignore','pipe','pipe'],
          env: { ...costCampaignEnvironment(process.env, item), SQLRS_LIQUIBASE_RELEASED_BENCH: '1',
            SQLRS_BENCH_REPO: repo, SQLRS_BENCH_FIXTURE: 'jhipster', SQLRS_BENCH_ROUTE: item.Route,
            SQLRS_BENCH_ENGINE: engine, SQLRS_BENCH_ENGINE_SHA256: engineHash, SQLRS_BENCH_REVISION: revision,
            SQLRS_BENCH_DISTRIBUTIONS: path.join(root,'distributions'), SQLRS_BENCH_INPUTS: path.join(root,'inputs'), SQLRS_BENCH_DB_IMAGE: spec.database }
        });
        const collect = bytes => { if (Buffer.byteLength(output)+bytes.length > 4 * 1024 * 1024) { overflow = true; active?.kill('SIGTERM'); } else output += bytes.toString(); };
        active.stdout.on('data',collect); active.stderr.on('data',collect);
        const timer = setTimeout(() => active?.kill('SIGTERM'), Math.max(1,Math.min(10*60*1000, deadline-Date.now())));
        active.on('error', () => { launchFailed = true; });
        active.on('close', code => {
          clearTimeout(timer); active = undefined;
          if (code !== 0 || overflow || launchFailed) {
            report.failureKind = failureKind(output); report.cleanupBoundary = cleanupBoundary(output); persist();
            reject(new Error('Cost collector failed'));
          } else resolve(output);
        });
      });
      const expected = { route: item.Route, fixture: 'jhipster', revision, engineHash };
      report.stage = 'qualify_samples'; persist();
      const samples = parseCollectorResult(0, output, expected);
      report.stage = 'qualify_costs'; persist();
      const costs = item.Record ? parseCostOutput(0, output, expected) : null;
      if (!item.Record && output.includes('P01B_COST ')) throw new Error('Disabled member emitted measurements');
      report.records.push({ ...item, samples, costs }); persist();
    }
    const newHelperDigests = new Set(report.records.filter(row => row.Variant === 'new_off' || row.Variant === 'new_on')
      .flatMap(row => row.samples.map(sample => sample.HelperDigest)));
    if (newHelperDigests.size !== 1) throw new Error('New off/on helpers differ');
    if (cancelled || Date.now() >= deadline) throw new Error('Cost completion interrupted');
    delete report.stage;
    report.status = 'cost_attribution_complete'; delete report.activeMember; persist();
    console.log('Eight cost members complete; full P01b acceptance remains separate');
  }
} catch {
  if (report) { report.status = 'incomplete'; report.reason = 'cost_collection_failed'; persist(); }
  console.error('Cost campaign incomplete; raw/private diagnostics withheld'); process.exitCode = 1;
} finally {
  process.off('SIGTERM',cancel); process.off('SIGINT',cancel);
}
