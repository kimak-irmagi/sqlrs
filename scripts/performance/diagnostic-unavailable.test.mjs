import test from 'node:test';
import assert from 'node:assert/strict';
import {parseDiagnosticOutput} from './diagnostic.mjs';

// UE04: literal terminal-only summaries use the existing category/report shape.
test('UE04: unavailable process time does not imply CLI launches or expose input',()=>{
 const samples=[{ElapsedMS:1000,LiquibaseCLIStarts:7},{ElapsedMS:1000,LiquibaseCLIStarts:1}];
 const original=structuredClone(samples);
 const clean=['uncached','cached'].map(Pass=>({Pass,Traced:true,ElapsedMS:1000,Phases:[],Trace:{CoveredMS:500,ResidualMS:500,Commands:3,Intervals:[
  {Category:'host.java',StartMS:0,EndMS:100},
  {Category:'process.unattributed',StartMS:100,EndMS:300},
  {Category:'process.unattributed',StartMS:200,EndMS:500},
 ]}}));
 const encode=rows=>rows.map(r=>'P01B_DIAGNOSTIC '+JSON.stringify(r)).join('\n');
 const dirty=structuredClone(clean);
 for(const row of dirty)Object.assign(row.Trace.Intervals[1],{pid:123456789,raw:'???( PRIVATE_SENTINEL',args:['PRIVATE_SENTINEL']});
 assert.deepEqual(parseDiagnosticOutput(encode(dirty),true,samples),clean);
 assert.deepEqual(samples,original);
 assert.ok(!JSON.stringify(parseDiagnosticOutput(encode(dirty),true,samples)).includes('PRIVATE_SENTINEL'));
 for(const mutate of [
  r=>r.Trace.Intervals[1].Category='exec.successful',
  r=>r.Trace.Intervals[1].StartMS=-1,r=>r.Trace.Intervals[1].EndMS=1001,
  r=>r.Trace.Intervals[1].EndMS=50,r=>r.Trace.Commands=2,
  r=>{r.Trace.CoveredMS=600;r.Trace.ResidualMS=400;},
 ]){
  const bad=structuredClone(clean);mutate(bad[0]);
  assert.throws(()=>parseDiagnosticOutput(encode(bad),true,samples),/Invalid diagnostic evidence/);
 }
});
