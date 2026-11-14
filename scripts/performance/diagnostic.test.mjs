import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { parse } from 'yaml';
import { diagnosticSelection, diagnosticSchedule, diagnosticIdentity, parseDiagnosticOutput, diagnosticFailureKind } from './diagnostic.mjs';

test('DG01: diagnostic opt-in rejects ordinary/fork/unvalidated events',()=>{
  const event={action:'labeled',label:{name:'performance:diagnostic'},pull_request:{head:{sha:'a'.repeat(40),repo:{full_name:'owner/repo'}}}};
  assert.equal(diagnosticSelection('pull_request',event,'owner/repo'),'a'.repeat(40));
  for(const mutate of [e=>e.action='synchronize',e=>e.label.name='performance:full',e=>e.pull_request.head.repo.full_name='fork/repo']){
    const copy=structuredClone(event);mutate(copy);assert.equal(diagnosticSelection('pull_request',copy,'owner/repo'),null);
  }
  assert.equal(diagnosticSelection('push',event,'owner/repo'),null);
  event.pull_request.head.sha='shell; PRIVATE_SENTINEL';assert.throws(()=>diagnosticSelection('pull_request',event,'owner/repo'),/Invalid diagnostic selection/);
});

test('DG01: separate read-only bounded workflow cannot replace active qualification',()=>{
  const w=parse(fs.readFileSync('.github/workflows/liquibase-performance-diagnostic.yml','utf8'));
  assert.deepEqual(w.on,{pull_request:{types:['labeled']}});
  assert.deepEqual(w.permissions,{contents:'read'});
  assert.match(w.concurrency.group,/diagnostic/);assert.equal(w.concurrency['cancel-in-progress'],false);
  const jobs=Object.values(w.jobs);assert.equal(jobs.length,1);
  assert.equal(jobs[0]['runs-on'],'ubuntu-24.04');assert.equal(jobs[0]['timeout-minutes'],60);
  const checkouts=jobs[0].steps.filter(s=>s.uses?.startsWith('actions/checkout@'));
  assert.equal(checkouts[0].with.ref,'668d5050ad154c1967bea4c6e40b91f3a703d5ba');
  assert.equal(checkouts[1].with.ref,'${{ github.event.pull_request.head.sha }}');
  assert.equal(checkouts[1].with.path,'.tmp/performance-observer');
  for(const c of checkouts)assert.equal(c.with['persist-credentials'],false);
  const upload=jobs[0].steps.find(s=>s.uses?.startsWith('actions/upload-artifact@'));
  assert.equal(upload.with.path,'.tmp/performance/diagnostic-reports/*.json');
});

test('DG02: only exact fixed baseline and candidate binaries qualify diagnostics',()=>{
  const expected={revision:'668d5050ad154c1967bea4c6e40b91f3a703d5ba',engineHash:'1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0',baselineHash:'b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca'};
  assert.doesNotThrow(()=>diagnosticIdentity(expected));
  for(const key of Object.keys(expected))assert.throws(()=>diagnosticIdentity({...expected,[key]:'wrong'}),/Diagnostic executable identity differs/);
});

test('DG10: eight serial members alternate within traced and untraced pairs',()=>{
  assert.deepEqual(diagnosticSchedule().map(x=>[x.mode,x.route]),[
    ['plain','v1'],['plain','v2'],['trace','v1'],['trace','v2'],
    ['trace','v2'],['trace','v1'],['plain','v2'],['plain','v1'],
  ]);
  assert.equal(new Set(diagnosticSchedule().map(x=>x.id)).size,8);
});

function diagnostic(pass='uncached'){
  return {Pass:pass,Traced:true,ElapsedMS:1000,Phases:[{Category:'job',Ordinal:0,Available:true,StartMS:0,EndMS:900}],
    Trace:{CoveredMS:700,ResidualMS:300,Commands:1,Intervals:[{Category:'docker.copy',StartMS:200,EndMS:900}]}};
}

test('DG09: failure checkpoints are closed enums, never copied trace bytes',()=>{
 for(const kind of ['process_tree','transport','journal','endpoint','clock',...Array.from({length:18},(_,i)=>`trace_${i}`)]){
  assert.equal(diagnosticFailureKind(`P01B_DIAGNOSTIC_FAILURE ${kind}\nPRIVATE_SENTINEL`),kind);
 }
 for(const kind of ['trace_18','trace_99','PRIVATE_SENTINEL','journal_PRIVATE_SENTINEL','trace_2_PRIVATE_SENTINEL']){
  assert.equal(diagnosticFailureKind(`P01B_DIAGNOSTIC_FAILURE ${kind}`),'unknown');
 }
 assert.equal(diagnosticFailureKind('unclassified PRIVATE_SENTINEL'),'unknown');
});

test('DG09: closed diagnostic evidence never leaks raw/private fields',()=>{
  const input=[diagnostic(),diagnostic('cached')];
  for(const row of input){row.private='PRIVATE_SENTINEL';row.Trace.private='PRIVATE_SENTINEL';row.Phases[0].secret='PRIVATE_SENTINEL';row.Trace.Intervals[0].argv='PRIVATE_SENTINEL';}
  const raw=input.map(x=>'P01B_DIAGNOSTIC '+JSON.stringify(x)).join('\n');
  const out=parseDiagnosticOutput(raw,true,[{ElapsedMS:1000},{ElapsedMS:1000}]);
  assert.equal(out.length,2);assert.ok(!JSON.stringify(out).includes('PRIVATE_SENTINEL'));
  for(const mutate of [r=>r.Pass='wrong',r=>r.ElapsedMS=-1,r=>r.Traced=false,r=>r.Phases[0].Category='PRIVATE_SENTINEL',r=>r.Trace.Intervals[0].Category='PRIVATE_SENTINEL',r=>r.Trace.ResidualMS=NaN,r=>r.Trace.Commands=99]){
    const broken=structuredClone(input);mutate(broken[0]);
    assert.throws(()=>parseDiagnosticOutput(broken.map(x=>'P01B_DIAGNOSTIC '+JSON.stringify(x)).join('\n'),true,[{ElapsedMS:1000},{ElapsedMS:1000}]),/Invalid diagnostic evidence/);
  }
  assert.throws(()=>parseDiagnosticOutput('P01B_DIAGNOSTIC PRIVATE_SENTINEL',true,[]),/Invalid diagnostic evidence/);
  assert.throws(()=>parseDiagnosticOutput('',true,[]),/Incomplete diagnostic output/);
});
