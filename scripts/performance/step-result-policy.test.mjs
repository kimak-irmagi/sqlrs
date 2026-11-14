import test from 'node:test';
import assert from 'node:assert/strict';
import { stepResultPolicy } from './step-result-policy.mjs';
import { qualifyCostDetails } from './cost-detail.mjs';
import { costCampaignEnvironment, costCampaignSchedule } from './cost-campaign.mjs';

test('IR06/IR10: exact artifact policy is selected before observation', () => {
  const identity = { route:'v2', revision:'a'.repeat(40), engineHash:'b'.repeat(64) };
  for (const member of costCampaignSchedule()) {
    const environment = costCampaignEnvironment({SQLRS_BENCH_RESULT_POLICY:'inherited'}, member);
    const selected = ['new_off','new_on'].includes(member.Variant);
    assert.equal(environment.SQLRS_BENCH_RESULT_POLICY, selected?'same-exec-v1':'');
    assert.equal(stepResultPolicy({...identity, resultPolicy:environment.SQLRS_BENCH_RESULT_POLICY,
      resultVariant:environment.SQLRS_BENCH_RESULT_VARIANT}), selected);
  }
  for (const patch of [{resultPolicy:'unknown'}, {route:'v1'}, {resultVariant:'old_v2'}, {revision:''}, {engineHash:''}, {resultPolicy:''}]) {
    assert.throws(() => stepResultPolicy({...identity,resultPolicy:'same-exec-v1',resultVariant:'new_on',...patch}));
  }
});

test('IR06: result helper ten regions are explicit; planner and legacy remain eight', () => {
  const phases=['preflight','cli','drain','stream_flush','hash_rename','renamed_flush','manifest_flush','final_flush'];
  const root=(operation, extended)=>({Spans:[{ID:1,Operation:operation,StartNS:0,EndNS:1,Phase:'step',Step:1}],
    Helpers:[{Owner:1,HostElapsedNS:0,Available:true,Detail:{ResolutionNS:10_000_000,TotalNS:0,
      CumulativeUncertaintyNS:(extended?10:8)*10_000_000,
      Regions:[...phases,...(extended?['result_prepare','result_emit']:[])].map(Phase=>({Phase,StartCS:0,EndCS:0,DurationNS:0}))}}],
    Observer:{EncodeNS:0,LockWaitNS:0,WriteNS:0}});
  const detail=qualifyCostDetails(root('liquibase.invoke',true),{stepResult:true});
  assert.equal(detail.Helpers[0].Regions.length,10);
  assert.equal(detail.Helpers[0].CumulativeUncertaintyNS,100_000_000);
  assert.throws(()=>qualifyCostDetails(root('liquibase.invoke',true)));
  assert.throws(()=>qualifyCostDetails(root('liquibase.invoke',false),{stepResult:true}));
  assert.throws(()=>qualifyCostDetails(root('liquibase.plan_invoke',true),{stepResult:true}));
  assert.equal(qualifyCostDetails(root('liquibase.plan_invoke',false),{stepResult:true}).Helpers[0].Regions.length,8);
  const invalid=root('liquibase.invoke',true); invalid.Helpers[0].Detail.Regions[9].Phase='private-marker';
  assert.throws(()=>qualifyCostDetails(invalid,{stepResult:true}), e=>!e.message.includes('private-marker'));
});
