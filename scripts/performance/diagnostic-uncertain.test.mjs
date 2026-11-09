import test from 'node:test';
import assert from 'node:assert/strict';
import {parseDiagnosticOutput} from './diagnostic.mjs';

// EU03/EU04: intervals count uncertainty, never successful migration/CLI starts.
// Original samples stay independent and must not be mutated by report decoding.
test('EU04: explicitly unconfirmed intervals keep time without changing CLI counts',()=>{
 const samples=[{ElapsedMS:1000,LiquibaseCLIStarts:9,JVMStartsFromPinnedLauncher:18},{ElapsedMS:1000,LiquibaseCLIStarts:1,JVMStartsFromPinnedLauncher:2}];
 const original=structuredClone(samples);
 const rows=['uncached','cached'].map(Pass=>({Pass,Traced:true,ElapsedMS:1000,Phases:[],Trace:{CoveredMS:500,ResidualMS:500,Commands:2,Intervals:[{Category:'host.java',StartMS:0,EndMS:200},{Category:'exec.unconfirmed',StartMS:200,EndMS:500}]}}));
 const encode=value=>value.map(r=>`P01B_DIAGNOSTIC ${JSON.stringify(r)}`).join('\n');
 const result=parseDiagnosticOutput(encode(rows),true,samples);
 assert.deepEqual(result,rows);assert.deepEqual(samples,original);
 for(const mutate of [
  r=>r.Trace.Intervals[1].Category='exec.unconfirmed_PRIVATE_SENTINEL',
  r=>r.Trace.Intervals[1].StartMS=-1,
  r=>r.Trace.Intervals[1].EndMS=1001,
  r=>r.Trace.Commands=3,
  r=>{r.Trace.CoveredMS=400;r.Trace.ResidualMS=600;},
 ]){
  const broken=structuredClone(rows);mutate(broken[0]);
  assert.throws(()=>parseDiagnosticOutput(encode(broken),true,samples),error=>error.message==='Invalid diagnostic evidence');
 }
 const plain=rows.map(r=>({...r,Traced:false,Trace:null}));
 assert.deepEqual(parseDiagnosticOutput(encode(plain),false,samples),plain);
});
