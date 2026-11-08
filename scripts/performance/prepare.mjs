import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { installJdbcDriver } from '../jdbc-driver-bundle.mjs';
import { baselineRevision, baselineArchiveSHA, verifyBytes, safeArchiveNames } from './control.mjs';
import { validateMetadata } from './inputs.mjs';

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const root = path.join(repo, '.tmp/performance');
const prepared = path.join(root, 'prepared');
fs.mkdirSync(prepared, { recursive: true, mode: 0o700 });
const run = (cmd, args, cwd = repo, env = {}) => execFileSync(cmd, args, { cwd, env: { ...process.env, ...env }, encoding: 'utf8', stdio: ['ignore','pipe','pipe'], timeout: 15 * 60 * 1000 });
const sha = process.env.PERFORMANCE_CANDIDATE;
if (!/^[a-f0-9]{40}$/.test(sha || '') || run('git',['rev-parse','HEAD']).trim() !== sha || run('git',['status','--porcelain','--untracked-files=no']).trim()) throw new Error('Candidate checkout is not the fixed clean source');
const started = Number(process.env.PERFORMANCE_JOB_STARTED_MS || Date.now());
if(!Number.isFinite(started)||started<=0||started>Date.now())throw new Error('Invalid hosted budget start');
run('node',['scripts/fetch-sql-scripts.mjs','--lock','--source-prefix','jdbc-postgresql-']);
const candidate = path.join(prepared,'candidate');
fs.mkdirSync(candidate,{recursive:true});
// Identical fixed source/compiler/trimmed paths produce the same artifact on
// every VM. A hash from the initial preparation VM is mandatory thereafter.
run('go',['build','-trimpath','-buildvcs=true','-o',path.join(candidate,'sqlrs-engine'),'./cmd/sqlrs-engine'],path.join(repo,'backend/local-engine-go'),{CGO_ENABLED:'0'});
installJdbcDriver(repo,candidate);
const engineHash=crypto.createHash('sha256').update(fs.readFileSync(path.join(candidate,'sqlrs-engine'))).digest('hex');
if (process.env.PERFORMANCE_EXPECTED_ENGINE_SHA && engineHash!==process.env.PERFORMANCE_EXPECTED_ENGINE_SHA) throw new Error('Candidate artifact differs between VMs');
validateMetadata(run('go',['version','-m',path.join(candidate,'sqlrs-engine')]),sha);
run('go',['test','-c','-o',path.join(prepared,'collector.test'),'.'],path.join(repo,'test/performance/liquibase'),{GOWORK:'off',CGO_ENABLED:'0'});

const baseline = path.join(prepared,'baseline');
fs.mkdirSync(baseline,{recursive:true});
const response = await fetch('https://github.com/kimak-irmagi/sqlrs/releases/download/v0.1.1-rc.7/sqlrs_v0.1.1_linux_amd64.tar.gz');
if (!response.ok) throw new Error('Pinned release download failed');
const bytes=Buffer.from(await response.arrayBuffer());
verifyBytes(bytes,baselineArchiveSHA);
const archive=path.join(root,'baseline.tar.gz');
fs.writeFileSync(archive,bytes,{mode:0o600});
safeArchiveNames(run('tar',['-tzf',archive]).trim().split('\n'));
// Only regular files/directories are admitted; no symlink or hardlink escape.
if (run('tar',['-tvzf',archive]).trim().split('\n').some(line=>!/^[-d]/.test(line))) throw new Error('Unsafe release archive type');
run('tar',['-xzf',archive,'--no-same-owner','-C',baseline]);
const findEngine = dir => fs.readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?findEngine(path.join(dir,e.name)):e.name==='sqlrs-engine'?[path.join(dir,e.name)]:[]);
const engines=findEngine(baseline);
if(engines.length!==1) throw new Error('Release engine inventory differs');
validateMetadata(run('go',['version','-m',engines[0]]),baselineRevision);
const baselineHash=crypto.createHash('sha256').update(fs.readFileSync(engines[0])).digest('hex');
const releaseTag = run('git',['ls-remote','https://github.com/kimak-irmagi/sqlrs.git','refs/tags/v0.1.1-rc.7','refs/tags/v0.1.1-rc.7^{}']);
if (!releaseTag.split('\n').some(line=>line.startsWith(baselineRevision+'\t'))) throw new Error('Release tag provenance differs');
fs.writeFileSync(path.join(prepared,'artifacts.json'),JSON.stringify({revision:sha,engine:path.join(candidate,'sqlrs-engine'),engineHash,
  baseline:engines[0],baselineHash,baselineRevision,preparationMS:Date.now()-started}),{mode:0o600});
if(process.env.GITHUB_OUTPUT)fs.appendFileSync(process.env.GITHUB_OUTPUT,`engine_sha=${engineHash}\n`);
console.log('Verified immutable Linux benchmark artifacts; no measurements performed');
