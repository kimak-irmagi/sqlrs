import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { expectedInventoryProbes } from './inventory-policy.mjs';
import { parseCollectorResult } from './collector-output.mjs';
import { costCampaignEnvironment, costCampaignSchedule } from './cost-campaign.mjs';
import { fixtureSpec } from './inputs.mjs';

const revision='a'.repeat(40), engineHash='b'.repeat(64);
const selected={route:'v2',revision,engineHash,inventoryPolicy:'per-process-v1',inventoryVariant:'new_on'};

test('IC07: exact opt-in changes only new artifact cold/hit expectations',()=>{
  assert.deepEqual(expectedInventoryProbes({route:'v1'}),[0,0]);
  assert.deepEqual(expectedInventoryProbes({route:'v2'}),[1,1]);
  for(const inventoryVariant of ['new_on','new_off']) assert.deepEqual(expectedInventoryProbes({...selected,inventoryVariant}),[1,0]);
  for(const patch of [{route:'v1'},{inventoryPolicy:'unknown'},{inventoryVariant:'old_v2'},{inventoryVariant:''},{inventoryPolicy:''},
    {revision:'main'},{engineHash:'invalid'}, {revision:'668d5050ad154c1967bea4c6e40b91f3a703d5ba'},
    {revision:'cab99c4c7ced5b18f4dc43a65b002d84d8fef02d'},
    {engineHash:'1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0'},
    {engineHash:'b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca'}]) {
    assert.throws(()=>expectedInventoryProbes({...selected,...patch}),/Invalid inventory probe policy/);
  }
  assert.throws(()=>expectedInventoryProbes({route:'unknown'}));
});

test('IC07: independent parser refuses wrong probes even with internally consistent JVM totals',()=>{
  const spec=fixtureSpec('jhipster');
  const rows=['uncached','cached'].map((Pass,index)=>({Route:'v2',Fixture:'jhipster',Pass,EngineSHA256:engineHash,SourceRevision:revision,
    Platform:'linux/amd64',ElapsedMS:1,Changesets:8,CacheHits:index?8:0,MigrationCLIStarts:index?1:9,
    ImageInspectionContainers:index?0:1,LiquibaseCLIStarts:index?1:10,JVMStartsFromPinnedLauncher:index?2:21,
    ContainerCreates:4,LiquibaseVersion:spec.version,DriverSHA256:'6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93',
    ImageDigest:spec.image.split('@')[1],CoreSHA256:spec.core,HelperDigest:'sha256:'+'c'.repeat(64),LauncherSHA256:'d'.repeat(64),
    JavaDetails:'Java 21',OracleOK:true,CleanupOK:true,SnapshotBackend:'copy'}));
  const output=values=>values.map(value=>'P01B_SAMPLE '+JSON.stringify(value)).join('\n');
  const expected={...selected,fixture:'jhipster'};
  assert.equal(parseCollectorResult(0,output(rows),expected).length,2);
  assert.throws(()=>parseCollectorResult(0,output(rows),{...expected,inventoryPolicy:'',inventoryVariant:''}));
  for(const index of [0,1]) {
    const wrong=structuredClone(rows), row=wrong[index]; row.ImageInspectionContainers=1-row.ImageInspectionContainers;
    row.LiquibaseCLIStarts=row.MigrationCLIStarts+row.ImageInspectionContainers;
    row.JVMStartsFromPinnedLauncher=2*row.LiquibaseCLIStarts+row.ImageInspectionContainers;
    assert.throws(()=>parseCollectorResult(0,output(wrong),expected));
  }
});

test('IC07: campaign strips inheritance and sends the same predeclared policy to both qualifiers',()=>{
  for(const member of costCampaignSchedule()) {
    const environment=costCampaignEnvironment({SQLRS_BENCH_IMAGE_PROBE_POLICY:'poison',SQLRS_BENCH_IMAGE_PROBE_VARIANT:'poison'},member);
    const isNew=['new_off','new_on'].includes(member.Variant);
    assert.equal(environment.SQLRS_BENCH_IMAGE_PROBE_POLICY,isNew?'per-process-v1':'');
    assert.equal(environment.SQLRS_BENCH_IMAGE_PROBE_VARIANT,isNew?member.Variant:'');
  }
  const source=fs.readFileSync('scripts/performance/run-cost.mjs','utf8');
  assert.match(source,/inventoryPolicy: environment.SQLRS_BENCH_IMAGE_PROBE_POLICY/);
  assert.match(source,/inventoryVariant: environment.SQLRS_BENCH_IMAGE_PROBE_VARIANT/);
  assert.ok(source.indexOf('const expected =')<source.indexOf('const output = await'));
  assert.match(source,/\.\.\.environment, SQLRS_LIQUIBASE_RELEASED_BENCH/);
  const collector=fs.readFileSync('test/performance/liquibase/collector_linux_test.go','utf8');
  assert.ok(collector.indexOf('inventoryProbeExpectations(')<collector.indexOf('child := exec.CommandContext'));
});
