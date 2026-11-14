import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {diagnosticClockFailureContext,diagnosticFailureContext,diagnosticPrefixContext,diagnosticRawFailureContext} from './diagnostic.mjs';

const unavailable={clockFailure:'none',clockWallDurationNS:null,clockElapsedDurationNS:null};
const marker=(reason,wall='none',elapsed='none')=>`P01B_DIAGNOSTIC_CLOCK ${reason} ${wall} ${elapsed}`;

// CW04: expected tuples and ceilings are independent literals, not shared sets.
test('CW04: exact closed clock context retains bounded coherent relative spans',()=>{
 const before='P01B_DIAGNOSTIC_FAILURE clock\nP01B_FAILURE_STAGE oracle';
 const old=[diagnosticFailureContext(before),diagnosticPrefixContext(before),diagnosticRawFailureContext(before)];
 for(const [reason,wall,elapsed] of [
  ['wall_order',null,null],['elapsed_order',null,null],
  ['duration_mismatch',null,null],['interval_order',null,null],
  ['duration_mismatch',1,2],['duration_mismatch',2,1],
  ['duration_mismatch',720000000000,719999999999],['interval_order',720000000000,720000000000],
 ]){
  const text=`${before}\n collector_linux_test.go:422: ${marker(reason,wall??'none',elapsed??'none')}\nPRIVATE_SENTINEL`;
  assert.deepEqual(diagnosticClockFailureContext(text),{clockFailure:reason,clockWallDurationNS:wall,clockElapsedDurationNS:elapsed});
  assert.deepEqual([diagnosticFailureContext(text),diagnosticPrefixContext(text),diagnosticRawFailureContext(text)],old);
 }
 for(const text of [
  '',marker('none'),marker('PRIVATE_SENTINEL'),marker('duration_mismatch_PRIVATE_SENTINEL'),
  'P01B_DIAGNOSTIC_CLOCK duration_mismatch 1',marker('duration_mismatch','none',1),
  marker('duration_mismatch',1,'none'),marker('duration_mismatch',1,1),marker('interval_order',1,2),
  marker('wall_order',1,1),marker('elapsed_order',1,1),marker('duration_mismatch',0,1),
  marker('duration_mismatch',-1,1),marker('duration_mismatch',1,-1),marker('duration_mismatch',1.5,2),
  marker('duration_mismatch','NaN',2),marker('duration_mismatch','Infinity',2),
  marker('duration_mismatch',720000000001,1),marker('duration_mismatch',1,'9223372036854775808'),
  `"${marker('duration_mismatch',1,2)}"`,`prefix ${marker('duration_mismatch',1,2)}`,
  marker('duration_mismatch',1,2)+' PRIVATE_SENTINEL',
 ]){
  const result=diagnosticClockFailureContext(text);
  assert.deepEqual(result,unavailable);assert.ok(!JSON.stringify(result).includes('PRIVATE_SENTINEL'));
 }
 const good=marker('duration_mismatch',1,2);
 assert.deepEqual(diagnosticClockFailureContext(good+'\n'+good),{clockFailure:'duration_mismatch',clockWallDurationNS:1,clockElapsedDurationNS:2});
 assert.deepEqual(diagnosticClockFailureContext(good+'\n'+marker('duration_mismatch',2,1)),unavailable);
 assert.deepEqual(diagnosticClockFailureContext(good+'\n'+marker('none')),unavailable);
});

// CW05: additive failure-only reconstruction; never change old exact contexts.
test('CW05: runner publishes clock detail only before failed clock persist/reject',()=>{
 const source=fs.readFileSync('scripts/performance/run-diagnostic.mjs','utf8');
 const start=source.indexOf('if(code!==0||overflow||launchFailed)'),end=source.indexOf('else resolve(output)',start);
 assert.ok(start>=0&&end>start);const branch=source.slice(start,end);
 assert.match(branch,/if\(report\.diagnosticFailure==='clock'\)Object\.assign\(report,diagnosticClockFailureContext\(output\)\)/);
 assert.ok(branch.indexOf('diagnosticClockFailureContext(output)')<branch.indexOf('persist()'));
 assert.ok(branch.indexOf('persist()')<branch.indexOf('reject('));
 assert.equal(source.indexOf('diagnosticClockFailureContext(output)'),start+branch.indexOf('diagnosticClockFailureContext(output)'));
 assert.doesNotMatch(source.slice(end),/diagnosticClockFailureContext\(output\)/);
 assert.doesNotMatch(branch,/report\.(?:raw|output|trace)\s*=\s*output/);
});
