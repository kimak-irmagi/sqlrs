import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { spawn, execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { fixtures, schedule, checkBudget, sanitizeSample, validateCohort, verifyBytes } from './control.mjs';
import { fixtureSpec } from './inputs.mjs';
import { warm } from './warm.mjs';
import { parseCollectorResult, failureKind, failureHTTPStatus } from './collector-output.mjs';

// PA07–PA10: the parent owns serial scheduling and a sanitized incomplete report.
// Raw collector stdout/stderr stays in memory and is never forwarded/uploaded.
const repo=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const root=path.join(repo,'.tmp/performance');
const prepared=JSON.parse(fs.readFileSync(path.join(root,'prepared/artifacts.json'),'utf8'));
const pilot=process.argv[2]==='pilot';
const fixture=process.env.PERFORMANCE_FIXTURE;
const phase=pilot?'pilot':process.env.PERFORMANCE_PHASE;
if(!pilot)schedule(fixture,phase);
const names=pilot?fixtures:[fixture];
const reports=path.join(root,'reports');
fs.mkdirSync(reports,{recursive:true,mode:0o700});
const reportPath=path.join(reports,pilot?'pilot.json':`${fixture}-${phase}.json`);
const host=crypto.randomUUID();
const report={status:'incomplete',phase,host,revision:prepared.revision,run:process.env.GITHUB_RUN_ID||'local-unqualified',
  platform:'linux/amd64',runner:process.env.ImageOS||'unknown',runnerImage:process.env.ImageVersion||'unknown',
  cpu:os.cpus()[0]?.model,cpus:os.cpus().length,memoryBytes:os.totalmem(),records:[],budgets:{}};
const persist=()=>fs.writeFileSync(reportPath,JSON.stringify(report,null,2)+'\n',{mode:0o600});
persist();
const started=Date.now();
// Leave 15 minutes before the hosted hard timeout, including setup already used.
const deadline=Date.now()+Math.max(0,345*60*1000-prepared.preparationMS);
let cancelled=false;
let active;
const cancel=()=>{cancelled=true;active?.kill('SIGTERM');};
process.on('SIGTERM',cancel);process.on('SIGINT',cancel);

async function member(route,name){

  report.activeMember={route,fixture:name};persist();
  if(cancelled || Date.now()>=deadline)throw new Error('Collection deadline or cancellation');
  const spec=fixtureSpec(name);
  const engine=route==='v1'?prepared.baseline:prepared.engine;
  const engineHash=route==='v1'?prepared.baselineHash:prepared.engineHash;
  verifyBytes(fs.readFileSync(engine),engineHash);
  const before=Date.now();
  const result=await new Promise((resolve,reject)=>{
    let output='',overflow=false;
    active=spawn(path.join(root,'prepared/collector.test'),['-test.run=^TestReleasedEngine$','-test.v','-test.timeout=155m'],{
      cwd:path.join(repo,'test/performance/liquibase'),stdio:['ignore','pipe','pipe'],env:{...process.env,
        SQLRS_LIQUIBASE_RELEASED_BENCH:'1',SQLRS_BENCH_REPO:repo,SQLRS_BENCH_FIXTURE:name,SQLRS_BENCH_ROUTE:route,
        SQLRS_BENCH_ENGINE:engine,SQLRS_BENCH_ENGINE_SHA256:engineHash,SQLRS_BENCH_REVISION:route==='v1'?prepared.baselineRevision:prepared.revision,
        SQLRS_BENCH_DISTRIBUTIONS:path.join(root,'distributions'),SQLRS_BENCH_INPUTS:path.join(root,'inputs'),SQLRS_BENCH_DB_IMAGE:spec.database,
        SQLRS_BENCH_PROBE:'',SQLRS_BENCH_RETAIN_PROBE:''}});
    const collect=bytes=>{if(output.length+bytes.length>4*1024*1024){overflow=true;active.kill('SIGTERM');}else output+=bytes.toString();};
    active.stdout.on('data',collect);active.stderr.on('data',collect);
    // TERM interrupts Go normally only with the collector's signal handler.
    // A later hard deadline cannot be mistaken for a completed run.
    const timer=setTimeout(()=>active?.kill('SIGTERM'),Math.min(145*60*1000,Math.max(1,deadline-Date.now())));
    active.on('error',()=>{clearTimeout(timer);active=undefined;reject(new Error('Collector launch failed'));});
    active.on('close',code=>{
      clearTimeout(timer);active=undefined;
      if(code!==0||overflow){
        const boundary=output.match(/P01B_FAILURE_STAGE (identity|engine_start|configuration|uncached|cached|oracle)\b/);
        report.failureStage=boundary?.[1]||'collector_exit';persist();
        report.failureKind=failureKind(output);persist();
        report.httpFailure=failureHTTPStatus(output);persist();
        report.oracleParts=[...new Set([...output.matchAll(/P01B_ORACLE_PART (row_count|row_shape|kind|order|id|author|path|checksum|contexts|labels|schema)\b/g)].map(match=>match[1]))];persist();
        reject(new Error('Collector failed; private diagnostics withheld'));
      }else resolve(output);
    });
  });
  const samples=parseCollectorResult(0,result,{route,fixture:name,engineHash,revision:route==='v1'?prepared.baselineRevision:prepared.revision});
  return {samples,totalMS:Date.now()-before};
}

try{
  if(process.platform!=='linux'||process.arch!=='x64'||process.env.GITHUB_ACTIONS!=='true')throw new Error('Real performance collection requires the approved Actions Linux VM');
  report.docker=execFileSync('docker',['version','--format','{{.Server.Version}}'],{encoding:'utf8',timeout:30000}).trim();
  warm(repo,names);
  report.preparationMS=prepared.preparationMS+Date.now()-started;
  if(pilot){
    for(const name of names){
      const runs={};
      for(const route of ['v1','v2']){
        runs[route]=await member(route,name);
        report.records.push({fixture:name,route,host,revision:prepared.revision,...runs[route]});persist();
      }
      report.budgets[name]=checkBudget({v1MS:runs.v1.totalMS,v2MS:runs.v2.totalMS,preparationMS:report.preparationMS});persist();
    }
    if(Object.values(report.budgets).some(b=>!b.fits))throw new Error('Full cohort does not fit the hosted budget');
    report.status='pilot_complete';
  }else{
    for(const item of schedule(fixture,phase)){
      const result=await member(item.route,fixture);
      report.records.push({...item,host,revision:prepared.revision,...result});persist();
    }
    Object.assign(report,validateCohort(report.records,fixture,phase));
  }
  persist();console.log(`${phase}: complete collection; speed acceptance requires separate P01b evaluation`);
}catch{
  report.status='incomplete';report.reason='collection_failed_or_budget_unavailable';persist();
  console.error('Performance collection incomplete; consult sanitized evidence only');process.exitCode=1;
}finally{
  process.off('SIGTERM',cancel);process.off('SIGINT',cancel);
}
