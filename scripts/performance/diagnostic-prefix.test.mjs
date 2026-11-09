import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {diagnosticPrefixContext,diagnosticFailureContext} from './diagnostic.mjs';

test('PX03: prefix metadata is additive, closed and compatible with old context',()=>{
 const before=diagnosticFailureContext('P01B_DIAGNOSTIC_FAILURE trace_8\nP01B_DIAGNOSTIC_CONTEXT pending_entry other\nP01B_FAILURE_STAGE oracle');
 const pairs=[...['empty','numeric_header','pid_change','exit_notice','signal_notice','bracket_notice','other','none'].map(p=>[p,'none']),['unknown_syscall','other'],...['syscall_entry','syscall_resume'].flatMap(p=>['execve','execveat','exit','exit_group','restart_syscall','other'].map(c=>[p,c]))];
 for(const [tracePrefix,traceKnownCall] of pairs){
  const raw=`go:2: P01B_DIAGNOSTIC_PREFIX ${tracePrefix} ${traceKnownCall}\nPRIVATE_SENTINEL`;
  assert.deepEqual(diagnosticPrefixContext(raw),{tracePrefix,traceKnownCall});
  assert.ok(!JSON.stringify(diagnosticPrefixContext(raw)).includes('PRIVATE_SENTINEL'));
 }
 for(const raw of ['', 'P01B_DIAGNOSTIC_PREFIX PRIVATE_SENTINEL other','P01B_DIAGNOSTIC_PREFIX empty execve','P01B_DIAGNOSTIC_PREFIX unknown_syscall execve','P01B_DIAGNOSTIC_PREFIX syscall_entry PRIVATE_SENTINEL','P01B_DIAGNOSTIC_PREFIX syscall_entry execve_PRIVATE_SENTINEL','P01B_DIAGNOSTIC_PREFIX numeric_header_PRIVATE_SENTINEL none']){
  assert.deepEqual(diagnosticPrefixContext(raw),{tracePrefix:'none',traceKnownCall:'none'});
 }
 assert.deepEqual(diagnosticFailureContext('P01B_DIAGNOSTIC_FAILURE trace_8\nP01B_DIAGNOSTIC_CONTEXT pending_entry other\nP01B_FAILURE_STAGE oracle\nP01B_DIAGNOSTIC_PREFIX empty none'),before);
});

test('PX04: reporter adds prefix separately only for other-shaped failure',()=>{
 const source=fs.readFileSync('scripts/performance/run-diagnostic.mjs','utf8');
 const start=source.indexOf('if(code!==0||overflow||launchFailed)');
 const end=source.indexOf('else resolve(output)',start);
 assert.ok(start>=0&&end>start);
 const branch=source.slice(start,end);
 assert.match(branch,/Object\.assign\(report,diagnosticFailureContext\(output\)\)/);
 assert.match(branch,/if\(report\.traceShape==='other'\)Object\.assign\(report,diagnosticPrefixContext\(output\)\)/);
 assert.ok(branch.indexOf('diagnosticPrefixContext(output)')<branch.indexOf('persist()'));
 assert.equal(source.indexOf('diagnosticPrefixContext(output)'),start+branch.indexOf('diagnosticPrefixContext(output)'));
 assert.doesNotMatch(source.slice(end),/diagnosticPrefixContext\(output\)/);
});
