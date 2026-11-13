import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { parseCompleteCostOutput } from './complete-cost-output.mjs';
import { fixtureSpec } from './inputs.mjs';
import { summarizeHostCosts } from './cost-accounting.mjs';
import { costDefinition } from './cost-catalog.mjs';

// IR06/IR10: independent full receiver fixture, with exact sample identities and
// unchanged 100ms API ledger. No product encoder supplies this oracle.
test('IR10: complete receiver selects extended steps without changing the wall-time ledger',()=>{
  const revision='a'.repeat(40), engineHash='b'.repeat(64), ms=1e6;
  const expected={route:'v2',fixture:'jhipster',revision,engineHash,inventoryPolicy:'per-process-v1',inventoryVariant:'new_on',
    resultPolicy:'same-exec-v1',resultVariant:'new_on'};
  const spec=fixtureSpec('jhipster');
  const samples=['uncached','cached'].map((Pass,index)=>({Route:'v2',Fixture:'jhipster',Pass,SourceRevision:revision,EngineSHA256:engineHash,
    Platform:'linux/amd64',ElapsedMS:100,Changesets:8,CacheHits:index?8:0,LiquibaseCLIStarts:index?1:10,MigrationCLIStarts:index?1:9,
    ImageInspectionContainers:index?0:1,ContainerCreates:4,LiquibaseVersion:spec.version,
    DriverSHA256:'6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93',ImageDigest:spec.image.split('@')[1],
    CoreSHA256:spec.core,HelperDigest:'sha256:'+'c'.repeat(64),LauncherSHA256:'d'.repeat(64),JVMStartsFromPinnedLauncher:index?2:21,
    JavaDetails:'Java 21',OracleOK:true,CleanupOK:true,SnapshotBackend:'copy'}));
  const costs=samples.map((sample,index)=>{
    const span=(ID,Parent,Operation,Phase,Step,StartNS,EndNS)=>({ID,Parent,Operation,Phase,Step,StartNS,EndNS,Outcome:'succeeded'});
    const Spans=[span(1,0,'prepare.request','worker',0,0,80*ms),span(2,1,'prepare.admission','admission',0,0,ms),
      span(3,1,'prepare.worker','worker',0,ms,80*ms),span(4,3,'liquibase.plan_invoke','planning',0,ms,2*ms)];
    if(!index) for(let step=1;step<=8;step++) {
      const owner=Spans.length+1;
      Spans.push(span(owner,3,'liquibase.invoke','step',step,step*ms+ms,step*ms+2*ms));
      Spans.push(span(owner+1,owner,'capture.result_decode','step',step,step*ms+ms,step*ms+2*ms));
    }
    const Helpers=Spans.filter(s=>s.Operation.startsWith('liquibase.')).map(s=>{
      const phases=['preflight','cli','drain','stream_flush','hash_rename','renamed_flush','manifest_flush','final_flush',
        ...(s.Operation==='liquibase.invoke'?['result_prepare','result_emit']:[])];
      return {Owner:s.ID,HostElapsedNS:0,Available:true,Detail:{ResolutionNS:10*ms,TotalNS:0,CumulativeUncertaintyNS:phases.length*10*ms,
        Regions:phases.map(Phase=>({Phase,StartCS:0,EndCS:0,DurationNS:0}))}};
    });
    return {Pass:sample.Pass,PrepareElapsedNS:100*ms,ObserverWaitNS:1,Root:{Root:index+1,ElapsedNS:80*ms,Declared:Spans.length,
      Complete:true,Ended:true,Spans,Helpers,Observer:{EncodeNS:1,LockWaitNS:2,WriteNS:3},Clock:{Domain:'linux-monotonic-relative-v1',OriginNS:101*ms},
      ObserverWindow:{StartNS:181*ms,EndNS:182*ms}},Timeline:{Timer:{Window:{StartNS:100*ms,EndNS:200*ms},
        StartBracket:{StartNS:100*ms-1,EndNS:100*ms+1},EndBracket:{StartNS:200*ms-1,EndNS:200*ms+1},APIElapsedNS:100*ms},ClockDeltaNS:0,
        Client:[{Kind:'client.post',StartNS:100*ms,EndNS:110*ms},{Kind:'client.status',StartNS:110*ms,EndNS:200*ms}]}};
  });
  const output=()=>samples.map(s=>`P01B_SAMPLE ${JSON.stringify(s)}`).concat(costs.map(c=>`P01B_COST ${JSON.stringify(c)}`)).join('\n');
  const reports=parseCompleteCostOutput(0,output(),expected);
  assert.deepEqual(reports.map(r=>r.Helpers.map(h=>h.Regions.length)),[[8,...Array(8).fill(10)],[8]]);
  for(const report of reports) assert.equal(report.Additive.reduce((sum,row)=>sum+row.DurationNS,0),100*ms);
  assert.equal(reports[0].HostRows.filter(row=>row.Operation==='capture.result_decode').reduce((sum,row)=>sum+row.Count,0),8);
  assert.throws(()=>parseCompleteCostOutput(0,output(),{...expected,resultPolicy:'',resultVariant:''}));
  costs[0].Root.Helpers[1].Detail.Regions.pop();
  assert.throws(()=>parseCompleteCostOutput(0,output(),expected));
});

test('IR10: both real receivers choose the same policy before engine launch and evidence parsing',()=>{
  const collector=fs.readFileSync('test/performance/liquibase/collector_linux_test.go','utf8');
  assert.ok(collector.indexOf('stepResultCostSelection(')<collector.indexOf('child.Start()'));
  assert.match(collector,/if stepResult \{\s*costReader = newStepResultCostStream\(\)/);
  const campaign=fs.readFileSync('scripts/performance/run-cost.mjs','utf8');
  assert.match(campaign,/resultPolicy: environment.SQLRS_BENCH_RESULT_POLICY/);
  assert.match(campaign,/resultVariant: environment.SQLRS_BENCH_RESULT_VARIANT/);
  assert.ok(campaign.indexOf('const expected =')<campaign.indexOf('active = spawn('));
});

test('IR10/IC07: actual new host keys qualify independently without admitting arbitrary names',()=>{
  for(const Operation of ['capture.result_decode','runner.inventory_lookup']) {
    const report=summarizeHostCosts([{ID:1,Parent:0,Operation,Phase:'step',Step:1,StartNS:0,EndNS:10,Outcome:'succeeded'}],10);
    assert.equal(report.Rows.length,1);
    assert.equal(report.Rows[0].Operation,Operation);
    assert.equal(report.Rows[0].Count,1);
    assert.ok(costDefinition(Operation).Current.Symbol);
  }
  assert.throws(()=>summarizeHostCosts([{ID:1,Parent:0,Operation:'private-marker',Phase:'step',Step:1,StartNS:0,EndNS:10,Outcome:'succeeded'}],10),
    e=>e.message==='Invalid host cost evidence');
});
