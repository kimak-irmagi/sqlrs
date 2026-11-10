import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {diagnosticRawFailureContext,diagnosticFailureContext,diagnosticPrefixContext} from './diagnostic.mjs';

const unavailable={rawNameFamily:'none',rawArgumentCount:'none',rawArgumentKind:'none',rawRejectStage:'none'};
const marker=(f,c,k,s)=>`P01B_DIAGNOSTIC_RAW_CONTEXT ${f} ${c} ${k} ${s}`;

// RD04: independent literal enum lists/pairs, not imported implementation sets.
test('RD04: closed raw detail validates whole tuples and preserves old exact helpers',()=>{
 const before='P01B_DIAGNOSTIC_FAILURE trace_8\nP01B_DIAGNOSTIC_CONTEXT pending_entry other\nP01B_DIAGNOSTIC_PREFIX unknown_syscall other\nP01B_FAILURE_STAGE oracle';
 const oldContext=diagnosticFailureContext(before),oldPrefix=diagnosticPrefixContext(before);
 const pairs=[['empty','none'],['none','none'],['none','unavailable'],...['short','six','extra'].flatMap(c=>['numeric','other'].map(k=>[c,k]))];
 for(const f of ['numeric','fallback','other'])for(const s of ['name','framing','arity','argument','result','duration','other'])for(const [c,k] of pairs){
  const text=`${before}\n    collector_linux_test.go:418: ${marker(f,c,k,s)}\nPRIVATE_SENTINEL`;
  assert.deepEqual(diagnosticRawFailureContext(text),{rawNameFamily:f,rawArgumentCount:c,rawArgumentKind:k,rawRejectStage:s});
  assert.deepEqual(diagnosticFailureContext(text),oldContext);assert.deepEqual(diagnosticPrefixContext(text),oldPrefix);
 }
 assert.deepEqual(diagnosticRawFailureContext('P01B_DIAGNOSTIC_RAW_CONTEXT fallback empty none arity'),{rawNameFamily:'fallback',rawArgumentCount:'empty',rawArgumentKind:'none',rawRejectStage:'arity'});
 for(const text of ['',marker('none','none','none','none'),marker('PRIVATE_SENTINEL','six','numeric','argument'),marker('numeric','PRIVATE_SENTINEL','numeric','argument'),marker('numeric','six','PRIVATE_SENTINEL','argument'),marker('numeric','six','numeric','PRIVATE_SENTINEL'),marker('numeric','empty','numeric','arity'),marker('numeric','six','none','argument'),marker('numeric','none','numeric','framing'),marker('numeric','short','unavailable','argument'),marker('none','six','numeric','argument'),marker('numeric','six','numeric','none'),marker('numeric','six','numeric','argument_PRIVATE_SENTINEL'),'P01B_DIAGNOSTIC_RAW_CONTEXT numeric six numeric',`"${marker('numeric','six','numeric','argument')}"`,`prefix ${marker('numeric','six','numeric','argument')}`,marker('numeric','six','numeric','argument')+' PRIVATE_SENTINEL']){
  const result=diagnosticRawFailureContext(text);assert.deepEqual(result,unavailable);assert.ok(!JSON.stringify(result).includes('PRIVATE_SENTINEL'));
 }
 assert.deepEqual(diagnosticRawFailureContext(marker('numeric','short','numeric','arity')+'\n'+marker('fallback','empty','none','arity')),unavailable);
 assert.deepEqual(diagnosticRawFailureContext(marker('fallback','empty','none','arity')+'\n'+marker('fallback','empty','none','arity')),{rawNameFamily:'fallback',rawArgumentCount:'empty',rawArgumentKind:'none',rawRejectStage:'arity'});
 // Existing DG09 reconstruction and private-field dropping stay in old tests;
 // the new helper must not carry anything from surrounding arbitrary JSON.
 const clean=diagnosticRawFailureContext(marker('fallback','empty','none','arity')+'\n{"private":"PRIVATE_SENTINEL","pid":123456789}');
 assert.equal(Object.keys(clean).length,4);assert.ok(!JSON.stringify(clean).includes('PRIVATE_SENTINEL'));assert.ok(!JSON.stringify(clean).includes('123456789'));
});

test('RD06: report adds separate raw detail only on a failed unknown raw record',()=>{
 const source=fs.readFileSync('scripts/performance/run-diagnostic.mjs','utf8');
 const start=source.indexOf('if(code!==0||overflow||launchFailed)'),end=source.indexOf('else resolve(output)',start);
 assert.ok(start>=0&&end>start);const branch=source.slice(start,end);
 assert.match(branch,/if\(report\.traceShape==='other'\)Object\.assign\(report,diagnosticPrefixContext\(output\)\)/);
 assert.match(branch,/if\(report\.traceShape==='other'&&report\.tracePrefix==='unknown_syscall'\)Object\.assign\(report,diagnosticRawFailureContext\(output\)\)/);
 assert.ok(branch.indexOf('diagnosticRawFailureContext(output)')<branch.indexOf('persist()'));
 assert.equal(source.indexOf('diagnosticRawFailureContext(output)'),start+branch.indexOf('diagnosticRawFailureContext(output)'));
 assert.doesNotMatch(source.slice(end),/diagnosticRawFailureContext\(output\)/);
 assert.doesNotMatch(branch,/report\.(?:raw|output|trace)\s*=\s*output/);
});
