import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {parse} from 'yaml';
import {parseDiagnosticOutput,diagnosticSchedule} from './diagnostic.mjs';

// UR04: an unknown process is neither an inferred Java launch nor raw metadata.
test('UR04: unattributed intervals retain checked time and drop private fields',()=>{
 const samples=[{ElapsedMS:1000,LiquibaseCLIStarts:9},{ElapsedMS:1000,LiquibaseCLIStarts:1}];
 const original=structuredClone(samples);
 const clean=['uncached','cached'].map(Pass=>({Pass,Traced:true,ElapsedMS:1000,Phases:[],Trace:{CoveredMS:500,ResidualMS:500,Commands:3,Intervals:[
  {Category:'host.java',StartMS:0,EndMS:200},
  {Category:'process.unattributed',StartMS:200,EndMS:500},
  {Category:'exec.unconfirmed',StartMS:100,EndMS:400},
 ]}}));
 const encode=rows=>rows.map(r=>'P01B_DIAGNOSTIC '+JSON.stringify(r)).join('\n');
 const input=structuredClone(clean);
 for(const row of input)Object.assign(row.Trace.Intervals[1],{syscall:'PRIVATE_SENTINEL',args:['0xdeadbeefcafef00d'],pid:123456789,description:'PRIVATE_SENTINEL'});
 const out=parseDiagnosticOutput(encode(input),true,samples);
 assert.deepEqual(out,clean);assert.deepEqual(samples,original);
 assert.ok(!JSON.stringify(out).includes('PRIVATE_SENTINEL'));
 assert.ok(!JSON.stringify(out).includes('deadbeefcafef00d'));
 for(const mutate of [
  r=>r.Trace.Intervals[1].Category='process.unattributed_PRIVATE_SENTINEL',
  r=>r.Trace.Intervals[1].StartMS=-1,r=>r.Trace.Intervals[1].EndMS=1001,
  r=>r.Trace.Intervals[1].EndMS=100,r=>r.Trace.Intervals[1].EndMS=Infinity,
  r=>r.Trace.Commands=4,r=>{r.Trace.CoveredMS=400;r.Trace.ResidualMS=600;},
 ]){
  const broken=structuredClone(clean);mutate(broken[0]);
  assert.throws(()=>parseDiagnosticOutput(encode(broken),true,samples),/Invalid diagnostic evidence/);
 }
});

// UR06/UR07: preserve the old native smoke, pins and eight-member schedule.
test('UR06: native raw probe is opt-in and must pass before diagnostic members',()=>{
 const workflow=parse(fs.readFileSync('.github/workflows/liquibase-performance-diagnostic.yml','utf8'));
 const steps=workflow.jobs.diagnostic.steps;
 const prepare=steps.findIndex(s=>s.name==='Prepare external observer only');
 const measured=steps.findIndex(s=>s.name==='Eight serial diagnostic members');
 assert.ok(prepare>=0&&measured>prepare);
 assert.match(steps[prepare].run,/SQLRS_DIAGNOSTIC_SMOKE=1 GOWORK=off go test -run '\^TestDiagnosticStraceProcessSmoke\$'/);
 assert.match(steps[prepare].run,/SQLRS_DIAGNOSTIC_RAW_SMOKE=1 GOWORK=off go test -run '\^TestDiagnosticRawSyscallSmoke\$' -v -count=1 -timeout=30s/);
 assert.doesNotMatch(steps[prepare].run,/continue-on-error|\|\| true/);
 const source=fs.readFileSync('test/performance/liquibase/diagnostic_raw_smoke_linux_test.go','utf8');
 assert.match(source,/os\.Getenv\("SQLRS_DIAGNOSTIC_RAW_SMOKE"\) != "1"/);
 assert.match(source,/verifyDiagnosticEndpoint\("\/proc"/);
 assert.match(source,/stopDiagnosticGroup\(/);
 assert.match(source,/finishMeasured\(-1\)/);
 assert.deepEqual(diagnosticSchedule().map(s=>[s.mode,s.route]),[['plain','v1'],['plain','v2'],['trace','v1'],['trace','v2'],['trace','v2'],['trace','v1'],['plain','v2'],['plain','v1']]);
});
