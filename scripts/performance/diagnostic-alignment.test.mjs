import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {parseDiagnosticOutput} from './diagnostic.mjs';

const encode=rows=>rows.map(r=>'P01B_DIAGNOSTIC '+JSON.stringify(r)).join('\n');
const samples=[{ElapsedMS:1000,LiquibaseCLIStarts:9},{ElapsedMS:1000,LiquibaseCLIStarts:1}];
const reports=(delta,residual)=>['uncached','cached'].map(Pass=>({Pass,Traced:true,ElapsedMS:1000,Phases:[],Trace:{CoveredMS:400,ResidualMS:residual,Commands:2,ClockDeltaNS:delta,Intervals:[{Category:'host.java',StartMS:0,EndMS:300},{Category:'docker.inspect',StartMS:200,EndMS:400}]}}));

// MC04: independent literals distinguish signed nanoseconds from milliseconds.
test('MC04: required alignment retains uncertainty on the calendar axis only',()=>{
 for(const [delta,residual] of [[0,600],[-9,599.999991],[9,600.000009],[-10000,599.99],[10000,600.01]]){
  const input=reports(delta,residual),before=structuredClone(samples);
  const output=parseDiagnosticOutput(encode(input),true,samples,true);
  assert.deepEqual(output,input);assert.deepEqual(samples,before);
  const old=structuredClone(input);for(const row of old)delete row.Trace.ClockDeltaNS;
  assert.deepEqual(parseDiagnosticOutput(encode(input),true,samples),old);
  const end=1000+delta/1e6;
  const full=['uncached','cached'].map(Pass=>({Pass,Traced:true,ElapsedMS:1000,Phases:[],Trace:{CoveredMS:end,ResidualMS:0,Commands:1,ClockDeltaNS:delta,Intervals:[{Category:'process.unattributed',StartMS:0,EndMS:end}]}}));
  assert.deepEqual(parseDiagnosticOutput(encode(full),true,samples,true),full);
  const empty=structuredClone(full);for(const row of empty)Object.assign(row.Trace,{CoveredMS:0,ResidualMS:end,Commands:0,Intervals:[]});
  assert.deepEqual(parseDiagnosticOutput(encode(empty),true,samples,true),empty);
 }
 const plain=['uncached','cached'].map(Pass=>({Pass,Traced:false,ElapsedMS:1000,Phases:[],Trace:null}));
 assert.deepEqual(parseDiagnosticOutput(encode(plain),false,samples,true),plain);
});

test('MC05: required mode rejects absent, out-of-bound or inconsistent alignment',()=>{
 for(const value of [undefined,null,'0',true,1.5,NaN,Infinity,-10001,10001,Number.MAX_SAFE_INTEGER+1]){
  const bad=reports(0,600);if(value===undefined)delete bad[0].Trace.ClockDeltaNS;else bad[0].Trace.ClockDeltaNS=value;
  assert.throws(()=>parseDiagnosticOutput(encode(bad),true,samples,true),/Invalid diagnostic evidence/);
 }
 for(const mutate of [
  r=>r.Trace.ClockDeltaNS=9, r=>r.Trace.ResidualMS=600.009,
  r=>{r.Trace.CoveredMS+=0.000002;r.Trace.ResidualMS-=0.000002;},
  r=>r.Trace.ResidualMS+=0.000002,
  r=>{r.Trace.CoveredMS=1000.000002;r.Trace.ResidualMS=0;r.Trace.Commands=1;r.Trace.Intervals=[{Category:'host.java',StartMS:0,EndMS:1000.000002}];},
 ]){
  const bad=reports(0,600);mutate(bad[0]);
  assert.throws(()=>parseDiagnosticOutput(encode(bad),true,samples,true),/Invalid diagnostic evidence/);
 }
 const negative=reports(-10000,0);for(const row of negative){row.ElapsedMS=0.000001;Object.assign(row.Trace,{CoveredMS:0,Commands:0,Intervals:[]});}
 assert.throws(()=>parseDiagnosticOutput(encode(negative),true,[{ElapsedMS:0.000001},{ElapsedMS:0.000001}],true),/Invalid diagnostic evidence/);
 const zero=reports(9,0.000009);for(const row of zero){row.ElapsedMS=0;Object.assign(row.Trace,{CoveredMS:0,Commands:0,Intervals:[]});}
 assert.throws(()=>parseDiagnosticOutput(encode(zero),true,[{ElapsedMS:0},{ElapsedMS:0}],true),/Invalid diagnostic evidence/);
 // Ordinary floating cancellation, not a wider alignment allowance.
 const rounded=reports(9,600.000009);for(const row of rounded)row.Trace.ResidualMS+=Number.EPSILON*1000;
 assert.equal(parseDiagnosticOutput(encode(rounded),true,samples,true).length,2);
 const dirty=reports(-9,599.999991),clean=structuredClone(dirty);
 for(const row of dirty){row.epoch='PRIVATE_SENTINEL';row.Trace.pid=123456789;row.Trace.Intervals[0].argv='PRIVATE_SENTINEL';}
 assert.deepEqual(parseDiagnosticOutput(encode(dirty),true,samples,true),clean);
 assert.throws(()=>parseDiagnosticOutput('P01B_DIAGNOSTIC PRIVATE_SENTINEL',true,samples,true),e=>e.message==='Invalid diagnostic evidence');
});

// MC06: missing evidence cannot fall back to the unchanged default parser.
test('MC06: native runner explicitly requires alignment before record acceptance',()=>{
 const source=fs.readFileSync('scripts/performance/run-diagnostic.mjs','utf8');
 assert.match(source,/parseDiagnosticOutput\(output,item\.mode==='trace',samples,true\)/);
 assert.equal((source.match(/parseDiagnosticOutput\(/g)||[]).length,1);
});
