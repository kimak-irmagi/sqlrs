import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {diagnosticFailureContext} from './diagnostic.mjs';

// CX03/CX04: independent Go-style log markers, no raw trace or process access.
test('CX03: closed context retains code, cleanup and last-reached stage independently',()=>{
 for(const location of ['record','pending_entry','none'])for(const shape of ['superseded_exec','exec_entry','exec_resume','other','none']){
  const metadata=diagnosticFailureContext(`collector.go:2: P01B_DIAGNOSTIC_FAILURE trace_8\ncollector.go:3: P01B_DIAGNOSTIC_CONTEXT ${location} ${shape}\ncollector.go:4: P01B_FAILURE_STAGE oracle\nP01B_CLEANUP_FAILURE files\nPRIVATE_SENTINEL`);
  assert.deepEqual(metadata,{diagnosticFailure:'trace_8',traceLocation:location,traceShape:shape,failureStage:'oracle',cleanupBoundary:'files'});
  assert.ok(!JSON.stringify(metadata).includes('PRIVATE_SENTINEL'));
 }
 for(const failureStage of ['identity','engine_start','configuration','uncached','cached','oracle']){
  assert.equal(diagnosticFailureContext(`P01B_FAILURE_STAGE ${failureStage}\n`).failureStage,failureStage);
 }
 for(const text of ['', 'P01B_FAILURE_STAGE PRIVATE_SENTINEL','P01B_FAILURE_STAGE oracle_PRIVATE_SENTINEL','P01B_DIAGNOSTIC_CONTEXT record_PRIVATE_SENTINEL other','P01B_DIAGNOSTIC_CONTEXT record exec_entry_PRIVATE_SENTINEL']){
  const metadata=diagnosticFailureContext(text);
  assert.deepEqual(metadata,{diagnosticFailure:'unknown',traceLocation:'none',traceShape:'none',failureStage:'collector_exit',cleanupBoundary:'unknown'});
 }
});

test('CX04: actual failure reporter persists closed metadata before incomplete exit',()=>{
 const source=fs.readFileSync('scripts/performance/run-diagnostic.mjs','utf8');
 assert.match(source,/import\s*\{[^}]*diagnosticFailureContext[^}]*\}\s*from '\.\/diagnostic\.mjs'/);
 const start=source.indexOf('if(code!==0||overflow||launchFailed)');
 const end=source.indexOf('else resolve(output)',start);
 assert.ok(start>=0&&end>start);
 const branch=source.slice(start,end);
 assert.match(branch,/Object\.assign\(report,diagnosticFailureContext\(output\)\)/);
 assert.ok(branch.indexOf('persist()')>branch.indexOf('diagnosticFailureContext(output)'));
 assert.ok(branch.indexOf('reject(')>branch.indexOf('persist()'));
 assert.doesNotMatch(branch,/report\.(?:raw|output|trace)\s*=\s*output/);
});
