import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { spawn, execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { diagnosticSelection, diagnosticIdentity, diagnosticSchedule, parseDiagnosticOutput } from './diagnostic.mjs';
import { parseCollectorResult, failureKind, cleanupBoundary } from './collector-output.mjs';
import { fixtureSpec } from './inputs.mjs';
import { verifyBytes, baselineArchiveSHA, baselineReleaseAPI } from './control.mjs';
import { downloadPinned } from './download.mjs';
import { warm } from './warm.mjs';

// docs/architecture/issue89-performance-diagnostic-design.md DG10/DG11:
// imperative orchestration is exercised by the actual Actions job,
// not counted as covered by the independently tested pure decoding helpers.
const repo=path.resolve(process.env.GITHUB_WORKSPACE||'.');
const root=path.join(repo,'.tmp/performance');
const observer=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const reportDir=path.join(root,'diagnostic-reports');
fs.mkdirSync(reportDir,{recursive:true,mode:0o700});
const report={status:'incomplete',kind:'diagnostic_not_acceptance',host:crypto.randomUUID(),
  run:process.env.GITHUB_RUN_ID||'unqualified',platform:'linux/amd64',runnerImage:process.env.ImageVersion||'unknown',
  cpu:os.cpus()[0]?.model,records:[]};
const persist=()=>fs.writeFileSync(path.join(reportDir,'jhipster-diagnostic.json'),JSON.stringify(report,null,2)+'\n',{mode:0o600});
persist();
let active,cancelled=false;
const cancel=()=>{cancelled=true;active?.kill('SIGTERM');};
process.on('SIGTERM',cancel);process.on('SIGINT',cancel);
try{
  if(process.platform!=='linux'||process.arch!=='x64'||process.env.GITHUB_ACTIONS!=='true')throw new Error('Unsupported diagnostic host');
  const event=JSON.parse(fs.readFileSync(process.env.GITHUB_EVENT_PATH,'utf8'));
  const selected=diagnosticSelection(process.env.GITHUB_EVENT_NAME,event,process.env.GITHUB_REPOSITORY);
  if(!selected||selected!==process.env.OBSERVER_REVISION||execFileSync('git',['-C',observer,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==selected
    ||execFileSync('git',['-C',observer,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw new Error('Observer provenance differs');
  report.observerRevision=selected;
  const prepared=JSON.parse(fs.readFileSync(path.join(root,'prepared/artifacts.json'),'utf8'));
  diagnosticIdentity(prepared);
  report.revision=prepared.revision;report.engineHash=prepared.engineHash;report.baselineHash=prepared.baselineHash;
  report.observerSHA256=crypto.createHash('sha256').update(fs.readFileSync(path.join(root,'prepared/collector.test'))).digest('hex');
  persist();
  const jobStarted=Number(process.env.PERFORMANCE_JOB_STARTED_MS);
  if(!Number.isFinite(jobStarted)||jobStarted>Date.now())throw new Error('Invalid diagnostic budget');
  const deadline=jobStarted+50*60*1000;
  // TA06: independently prove the official API bytes even if the browser URL
  // has recovered. This is outside timing; it never substitutes another v1.
  await downloadPinned(baselineReleaseAPI,baselineArchiveSHA,{fetchImpl:(url,options)=>fetch(url,{...options,headers:{Accept:'application/octet-stream'}})});
  report.baselineAPIArchiveSHA256=baselineArchiveSHA;persist();
  warm(repo,['jhipster']);
  const spec=fixtureSpec('jhipster');
  for(const item of diagnosticSchedule()){
    report.activeMember=item;persist();
    if(cancelled||Date.now()>=deadline)throw new Error('Diagnostic budget exhausted');
    const engine=item.route==='v1'?prepared.baseline:prepared.engine;
    const engineHash=item.route==='v1'?prepared.baselineHash:prepared.engineHash;
    const revision=item.route==='v1'?prepared.baselineRevision:prepared.revision;
    verifyBytes(fs.readFileSync(engine),engineHash);
    const output=await new Promise((resolve,reject)=>{
      let output='',overflow=false,launchFailed=false;
      active=spawn(path.join(root,'prepared/collector.test'),['-test.run=^TestReleasedEngine$','-test.v','-test.timeout=12m'],{
        cwd:path.join(observer,'test/performance/liquibase'),stdio:['ignore','pipe','pipe'],env:{...process.env,
          SQLRS_LIQUIBASE_RELEASED_BENCH:'1',SQLRS_BENCH_DIAGNOSTIC:'1',SQLRS_BENCH_TRACE:item.mode==='trace'?'1':'',
          SQLRS_BENCH_REPO:repo,SQLRS_BENCH_FIXTURE:'jhipster',SQLRS_BENCH_ROUTE:item.route,
          SQLRS_BENCH_ENGINE:engine,SQLRS_BENCH_ENGINE_SHA256:engineHash,SQLRS_BENCH_REVISION:revision,
          SQLRS_BENCH_DISTRIBUTIONS:path.join(root,'distributions'),SQLRS_BENCH_INPUTS:path.join(root,'inputs'),SQLRS_BENCH_DB_IMAGE:spec.database,
          SQLRS_BENCH_PROBE:'',SQLRS_BENCH_RETAIN_PROBE:''}});
      const collect=b=>{if(Buffer.byteLength(output)+b.length>4*1024*1024){overflow=true;active?.kill('SIGTERM');}else output+=b.toString();};
      active.stdout.on('data',collect);active.stderr.on('data',collect);
      const timer=setTimeout(()=>active?.kill('SIGTERM'),Math.max(1,Math.min(10*60*1000,deadline-Date.now())));
      active.on('error',()=>{launchFailed=true;});
      active.on('close',code=>{
        clearTimeout(timer);active=undefined;
        if(code!==0||overflow||launchFailed){report.failureKind=failureKind(output);report.cleanupBoundary=cleanupBoundary(output);persist();reject(new Error('Diagnostic member failed'));}
        else resolve(output);
      });
    });
    const samples=parseCollectorResult(0,output,{route:item.route,fixture:'jhipster',revision,engineHash});
    const diagnostics=parseDiagnosticOutput(output,item.mode==='trace',samples);
    report.records.push({...item,samples,diagnostics});persist();
  }
  report.status='diagnostic_complete';delete report.activeMember;persist();
  console.log('Eight diagnostic members complete; this is not speed acceptance');
}catch{
  report.status='incomplete';report.reason='diagnostic_collection_failed';persist();
  console.error('Diagnostic incomplete; raw/private diagnostics withheld');process.exitCode=1;
}finally{
  process.off('SIGTERM',cancel);process.off('SIGINT',cancel);
}
