import assert from 'node:assert/strict';
import test from 'node:test';
import { parseCollectorResult } from './collector-output.mjs';
import { fixtureSpec } from './inputs.mjs';

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
