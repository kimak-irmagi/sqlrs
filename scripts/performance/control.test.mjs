import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import crypto from 'node:crypto';
import os from 'node:os';
import path from 'node:path';
import { selectRun, schedule, checkBudget, validateCohort, sanitizeSample, verifyBytes, safeArchiveNames } from './control.mjs';

const sha = 'a'.repeat(40);
const repo = 'kimak-irmagi/sqlrs';
const pr = { action: 'labeled', label: { name: 'performance:pilot' }, pull_request: { head: { sha, repo: { full_name: repo } } } };

test('PA01: only explicit same-repository labels or validated dispatch start work', () => {
  assert.deepEqual(selectRun('pull_request', pr, repo), { sha, phase: 'pilot' });
  assert.deepEqual(selectRun('workflow_dispatch', { inputs: { candidate_sha: sha, phase: 'full' } }, repo), { sha, phase: 'full' });
  for (const event of [ { ...pr, action: 'synchronize' }, { ...pr, label: { name: 'other' } }, { ...pr, pull_request: { head: { sha, repo: { full_name: 'fork/repo' } } } } ]) {
    assert.equal(selectRun('pull_request', event, repo), null);
  }
  assert.equal(selectRun('push', {}, repo), null);
  for (const bad of ['main', 'a'.repeat(39), '$(touch /tmp/x)', sha + '\n']) {
    assert.throws(() => selectRun('workflow_dispatch', { inputs: { candidate_sha: bad, phase: 'full' } }, repo));
  }
  assert.throws(() => selectRun('workflow_dispatch', { inputs: { candidate_sha: sha, phase: 'cheap' } }, repo));
});

test('PA02: exact bytes and archive paths must be safe before extraction/execution', () => {
  const bytes = Buffer.from('verified archive');
  const digest = crypto.createHash('sha256').update(bytes).digest('hex');
  assert.equal(verifyBytes(bytes, digest), digest);
  for (const bad of ['0'.repeat(64), '', digest.toUpperCase()]) assert.throws(() => verifyBytes(bytes, bad));
  assert.deepEqual(safeArchiveNames(['sqlrs-engine', './lib/driver.jar']), ['sqlrs-engine', './lib/driver.jar']);
  for (const name of ['../escape', '/tmp/escape', 'lib/../../escape', 'C:/escape', 'lib\\escape', 'bad\0name']) assert.throws(() => safeArchiveNames([name]));
});

test('PA07: comparison and controls are serial, alternating and never reuse a sample', () => {
  const plan = schedule('jhipster', 'comparison');
  assert.equal(plan.length, 20);
  assert.equal(new Set(plan.map(x => x.id)).size, 20);
  assert.deepEqual(plan.slice(0, 4).map(x => x.route), ['v1', 'v2', 'v2', 'v1']);
  assert.equal(plan.filter(x => x.kind === 'control').length, 10);
  assert.ok(plan.filter(x => x.kind === 'control').every(x => x.route === 'v1'));
  assert.throws(() => schedule('private', 'comparison'));
  assert.throws(() => schedule('jhipster', 'pilot'));
});

const sample = () => ({ Route: 'v2', Fixture: 'jhipster', Pass: 'uncached', EngineSHA256: 'b'.repeat(64), SourceRevision: sha,
  Platform: 'linux/amd64', ElapsedMS: 20, Changesets: 8, CacheHits: 0, LiquibaseCLIStarts: 10, MigrationCLIStarts: 9,
  ImageInspectionContainers: 1, ContainerCreates: 8, LiquibaseVersion: '5.0.3', DriverSHA256: 'c'.repeat(64),
  ImageDigest: 'sha256:' + 'd'.repeat(64), HelperDigest: 'sha256:' + 'e'.repeat(64), CoreSHA256: 'f'.repeat(64),
  LauncherSHA256: '0'.repeat(64), JVMStartsFromPinnedLauncher: 21, JavaDetails: 'pinned Java 21', OracleOK: true,
  CleanupOK: true, SnapshotBackend: 'copy' });

test('PA05/PA10: typed allowlist excludes nested private diagnostics on success and failure', () => {
  const raw = { ...sample(), secret: 'PASSWORD_SENTINEL', nested: { DSN: 'postgres://SECRET' }, Error: 'SQL_SENTINEL' };
  const clean = sanitizeSample(raw);
  assert.equal(clean.ElapsedMS, 20);
  assert.doesNotMatch(JSON.stringify(clean), /SENTINEL|SECRET|postgres:\/\//);
  for (const patch of [{ JavaDetails: 'postgres://PASSWORD_SENTINEL' }, { OracleOK: false }, { CleanupOK: false },
    { Changesets: 7 }, { CacheHits: 1 }, { ElapsedMS: NaN }, { ElapsedMS: -1 }, { Platform: 'windows/amd64' },
    { EngineSHA256: '' }, { SnapshotBackend: 'btrfs' }, { Fixture: 'izess' }, { Pass: 'cached', CacheHits: 0 }]) {
    assert.throws(() => sanitizeSample({ ...sample(), ...patch }));
  }
  assert.equal(sanitizeSample({ ...sample(), Pass: 'cached', CacheHits: 8 }).CacheHits, 8);
});

test('PA08: full cost plus preparation and reserve, not just timed API intervals', () => {
  assert.equal(checkBudget({ v1MS: 1000, v2MS: 2000, preparationMS: 60000 }).fits, true);
  const result = checkBudget({ v1MS: 900000, v2MS: 1800000, preparationMS: 60000 });
  assert.equal(result.fits, false);
  assert.ok(result.estimatedMS > 6 * 60 * 60 * 1000);
  for (const v1MS of [0, -1, Infinity, NaN]) assert.throws(() => checkBudget({ v1MS, v2MS: 1, preparationMS: 0 }));
});

function complete() {
  return schedule('jhipster', 'comparison').map(item => ({ ...item, host: 'vm-A', revision: sha,
    samples: ['uncached', 'cached'].map(pass => ({ ...sample(), Route: item.route, Pass: pass, CacheHits: pass === 'cached' ? 8 : 0 })) }));
}

test('PA07/PA10: elapsed spread is recorded separately from any acceptance verdict',()=>{
  const rows=complete();let index=0;
  for(const row of rows)if(row.route==='v2')row.samples[0].ElapsedMS=10*(++index);
  const report=validateCohort(rows,'jhipster','comparison');
  assert.deepEqual(report.distributions['comparison:v2:uncached'],{min:10,median:30,max:50});
  assert.equal(report.status,'collected');
});

test('PA07/PA10: complete collection is not a speed verdict; every pair/pass/identity is required', () => {
  assert.equal(validateCohort(complete(), 'jhipster', 'comparison').status, 'collected');
  for (const mutate of [ rows => rows.pop(), rows => rows.push(rows[0]), rows => rows[1].host = 'vm-B',
    rows => rows[1].samples.pop(), rows => rows[1].revision = 'b'.repeat(40), rows => rows[1].samples[0].EngineSHA256 = '1'.repeat(64),
    rows => rows[1].samples[0].OracleOK = false, rows => rows[1].id = rows[0].id ]) {
    const rows = complete(); mutate(rows); assert.throws(() => validateCohort(rows, 'jhipster', 'comparison'));
  }
});
