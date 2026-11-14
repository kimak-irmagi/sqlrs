import assert from 'node:assert/strict';
import test from 'node:test';
import { parseCollectorResult, failureKind, failureHTTPStatus, cleanupBoundary } from './collector-output.mjs';
import { fixtureSpec } from './inputs.mjs';

test('PA10: failure diagnostics expose only reviewed fixed classes',()=>{
 assert.equal(failureKind('private log\nP01B_FAILURE_KIND permission_denied\n'),'permission_denied');
 assert.equal(failureKind('P01B_FAILURE_KIND postgres://SECRET'),'unknown');
 assert.equal(failureKind('PRIVATE_SENTINEL'),'unknown');
});

test('PA09/PA10: cleanup boundary is an enum, never a private path',()=>{
 assert.equal(cleanupBoundary('P01B_CLEANUP_FAILURE files'),'files');
 assert.equal(cleanupBoundary('P01B_CLEANUP_FAILURE containers'),'containers');
 assert.equal(cleanupBoundary('P01B_CLEANUP_FAILURE /PRIVATE_SENTINEL'),'unknown');
});

test('PA04/PA10: rejected API request is distinguishable without exposing its body',()=>{
 const output='released v1 prepare rejected: HTTP 400\nPRIVATE_SENTINEL';
 assert.equal(failureKind(output),'api_http_failure');
 assert.equal(failureHTTPStatus(output),400);
 assert.equal(failureHTTPStatus('PRIVATE_SENTINEL'),undefined);
 assert.equal(failureHTTPStatus('released status HTTP 503'),503);
});

test('PA05/PA10: fixed collector verification failures are distinct and disclose no rows',()=>{
 for(const [text,kind] of [
  ['released engine result differs from independent public oracle','oracle_mismatch'],
  ['benchmark oracle query failed','oracle_query'],['cannot verify benchmark result database','oracle_connection'],
  ['benchmark cache pass is not the declared workload','cache_pass'],['benchmark Liquibase CLI count=2 expected=1','cli_count'],
  ['benchmark Docker create stream is not complete','docker_count'],['offline image probes=0 expected=1','probe_count'],
  ['TempDir RemoveAll cleanup: PRIVATE_SENTINEL','private_cleanup']]){
  assert.equal(failureKind('P01B_FAILURE_KIND unknown\n'+text+'\nPRIVATE_SENTINEL'),kind);
 }
});

test('PA05/PA06/PA10: process success and exact independently verified workload are both mandatory',()=>{
 const spec=fixtureSpec('jhipster');const sha='a'.repeat(40), hash='b'.repeat(64);
 const rows=['uncached','cached'].map((pass,index)=>({Route:'v2',Fixture:'jhipster',Pass:pass,EngineSHA256:hash,SourceRevision:sha,
  Platform:'linux/amd64',ElapsedMS:1,Changesets:8,CacheHits:index?8:0,LiquibaseCLIStarts:index?2:10,MigrationCLIStarts:index?1:9,
  ImageInspectionContainers:1,ContainerCreates:4,LiquibaseVersion:spec.version,DriverSHA256:'6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93',
  ImageDigest:spec.image.split('@')[1],CoreSHA256:spec.core,HelperDigest:'sha256:'+'c'.repeat(64),LauncherSHA256:'d'.repeat(64),
  JVMStartsFromPinnedLauncher:index?5:21,JavaDetails:'Java 21',OracleOK:true,CleanupOK:true,SnapshotBackend:'copy'}));
 const expected={route:'v2',fixture:'jhipster',revision:sha,engineHash:hash};
 const output=items=>'PRIVATE_LOG_SENTINEL\n'+items.map(row=>'P01B_SAMPLE '+JSON.stringify(row)).join('\n');
 assert.equal(parseCollectorResult(0,output(rows),expected).length,2);
 assert.throws(()=>parseCollectorResult(1,output(rows),expected));
 assert.throws(()=>parseCollectorResult(0,output(rows.slice(0,1)),expected));
 for(const patch of [{Route:'v1'},{Pass:'cached'},{CoreSHA256:'0'.repeat(64)},{MigrationCLIStarts:0},{ImageInspectionContainers:0},
  {JVMStartsFromPinnedLauncher:1},{DriverSHA256:'0'.repeat(64)},{SourceRevision:'c'.repeat(40)}, {OracleOK:false}, {CleanupOK:false}]){
  assert.throws(()=>parseCollectorResult(0,output([{...rows[0],...patch},rows[1]]),expected));
 }
 assert.doesNotMatch(JSON.stringify(parseCollectorResult(0,output(rows),expected)),/PRIVATE_LOG/);
});
