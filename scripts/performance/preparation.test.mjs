import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import YAML from 'yaml';
import { downloadPinned } from './download.mjs';
import { preparationCheckout } from './inputs.mjs';
import { baselineReleaseAPI, baselineArchiveSHA } from './control.mjs';

const browser='https://example.invalid/fixture';
const api='https://api.github.com/repos/kimak-irmagi/sqlrs/releases/assets/583092383';
const bytes=Buffer.from('independent small transport fixture');
const sha=crypto.createHash('sha256').update(bytes).digest('hex');
const good=(body=bytes)=>({ok:true,status:200,arrayBuffer:async()=>Uint8Array.from(body).buffer});

// TA01–TA02: fixed expected calls are independent of the downloader result.
test('TA01: bounded Node/curl/API order uses identical independently pinned bytes',async()=>{
 for(const route of ['node','curl','api']){
  const calls=[];
  const result=await downloadPinned(browser,sha,{delayMS:0,apiURL:api,
   fetchImpl:async(url,options)=>{
    calls.push(url===browser?'node':'api');
    assert.ok(options.signal instanceof AbortSignal);
    if(url===api){assert.deepEqual(options.headers,{Accept:'application/octet-stream'});return good();}
    assert.equal(url,browser);return route==='node'?good():{ok:false,status:504};
   },
   fallback:async()=>{calls.push('curl');if(route==='api')throw new Error('PRIVATE_SENTINEL');return bytes;},
  });
  assert.deepEqual(result,bytes);
  assert.deepEqual(calls,route==='node'?['node']:route==='curl'?['node','node','node','curl']:['node','node','node','curl','api']);
 }
});

test('TA02: each successful-but-wrong transport is final, including JSON/HTML',async()=>{
 for(const route of ['node','curl','api'])for(const wrong of [Buffer.from('{}'),Buffer.from('<html>not an archive</html>')]){
  const calls=[];
  await assert.rejects(downloadPinned(browser,sha,{delayMS:0,apiURL:api,
   fetchImpl:async(url)=>{calls.push(url===browser?'node':'api');return (route==='node'||url===api)?good(wrong):{ok:false,status:504};},
   fallback:async()=>{calls.push('curl');if(route==='api')throw new Error('PRIVATE_SENTINEL');return wrong;},
  }),err=>!err.message.includes('PRIVATE_SENTINEL'));
  assert.deepEqual(calls,route==='node'?['node']:route==='curl'?['node','node','node','curl']:['node','node','node','curl','api']);
 }
});

test('TA02: API retries/body errors are bounded and ordinary 404 remains final',async()=>{
 for(const mode of ['exhausted','body_then_ok','api_404','browser_404']){
  let primary=0,alternate=0,curl=0;
  const request=()=>downloadPinned(browser,sha,{apiURL:api,delayMS:0,
   fetchImpl:async url=>{
    if(url===browser){primary++;return {ok:false,status:mode==='browser_404'?404:504};}
    alternate++;
    if(mode==='exhausted')throw new Error('PRIVATE_SENTINEL');
    if(mode==='api_404')return {ok:false,status:404};
    if(alternate===1)return {ok:true,status:200,arrayBuffer:async()=>{throw new Error('PRIVATE_SENTINEL');}};
    return good();
   },fallback:async()=>{curl++;throw new Error('PRIVATE_SENTINEL');},
  });
  if(mode==='body_then_ok')assert.deepEqual(await request(),bytes);
  else await assert.rejects(request(),err=>!err.message.includes('PRIVATE_SENTINEL'));
  assert.equal(primary,mode==='browser_404'?1:3);assert.equal(curl,mode==='browser_404'?0:1);
  assert.equal(alternate,mode==='browser_404'?0:mode==='exhausted'?3:mode==='api_404'?1:2);
 }
});

function gitFixture(t,name){
 const dir=fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(),'sqlrs-prep-test-')));
 t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 const git=args=>execFileSync('git',['-C',dir,'-c','core.autocrlf=false','-c',`core.hooksPath=${path.join(dir,'no-hooks')}`,'-c','commit.gpgsign=false',...args],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim();
 git(['init','--quiet','--template=']);fs.writeFileSync(path.join(dir,'fixture.txt'),name);
 git(['add','fixture.txt']);git(['-c','user.name=sqlrs test','-c','user.email=sqlrs-test@example.invalid','commit','--quiet','-m','fixture']);
 return {dir,git,head:git(['rev-parse','HEAD'])};
}

test('TA03: own checkout remains default, explicit source is not observer checkout',t=>{
 const source=gitFixture(t,'source'),observer=gitFixture(t,'observer');
 for(const sourceRoot of [undefined,''])assert.equal(preparationCheckout({ownRoot:observer.dir,sourceRoot,revision:observer.head}),observer.dir);
 assert.equal(preparationCheckout({ownRoot:observer.dir,sourceRoot:source.dir,workspace:source.dir,revision:source.head}),source.dir);
 for(const options of [
  {sourceRoot:'relative',workspace:source.dir},
  {sourceRoot:source.dir,workspace:''},
  {sourceRoot:source.dir,workspace:observer.dir},
  {sourceRoot:path.join(source.dir,'missing'),workspace:path.join(source.dir,'missing')},
  {sourceRoot:path.join(source.dir,'fixture.txt'),workspace:path.join(source.dir,'fixture.txt')},
 ])assert.throws(()=>preparationCheckout({ownRoot:observer.dir,revision:source.head,...options}),/Invalid preparation source root/);
 for(const revision of ['invalid','a'.repeat(40)])assert.throws(()=>preparationCheckout({ownRoot:source.dir,revision}),/Candidate checkout is not the fixed clean source/);
 fs.writeFileSync(path.join(source.dir,'fixture.txt'),'dirty');
 assert.throws(()=>preparationCheckout({ownRoot:observer.dir,sourceRoot:source.dir,workspace:source.dir,revision:source.head}),/Candidate checkout is not the fixed clean source/);
 assert.throws(()=>preparationCheckout({ownRoot:os.tmpdir(),revision:source.head}),/Candidate checkout is not the fixed clean source/);
});

test('TA03: actual prepare rejects wrong HEAD/dirty source before dependencies or outputs',t=>{
 const script=fileURLToPath(new URL('./prepare.mjs',import.meta.url));
 const source=gitFixture(t,'clean');
 for(const mode of ['wrong_head','dirty']){
  if(mode==='dirty')fs.writeFileSync(path.join(source.dir,'fixture.txt'),'PRIVATE_SENTINEL');
  const child=spawnSync(process.execPath,[script],{encoding:'utf8',env:{...process.env,PERFORMANCE_SOURCE_ROOT:source.dir,GITHUB_WORKSPACE:source.dir,PERFORMANCE_CANDIDATE:mode==='dirty'?source.head:'a'.repeat(40)}});
  assert.equal(child.status,1);assert.match(child.stderr,/Candidate checkout is not the fixed clean source/);
  assert.ok(!child.stderr.includes('PRIVATE_SENTINEL'));assert.ok(!fs.existsSync(path.join(source.dir,'.tmp')));
 }
});

test('TA04: observer preparation targets exact original sources and official asset',()=>{
 const workflow=YAML.parse(fs.readFileSync('.github/workflows/liquibase-performance-diagnostic.yml','utf8'));
 const setup=workflow.jobs.diagnostic.steps.find(s=>s.uses?.endsWith('/liquibase-perf-setup'));
 assert.equal(setup.uses,'./.tmp/performance-observer/.github/actions/liquibase-perf-setup');
 assert.equal(setup.with.source_root,'${{ github.workspace }}');
 assert.equal(setup.with.candidate_sha,'668d5050ad154c1967bea4c6e40b91f3a703d5ba');
 assert.equal(setup.with.expected_engine_sha,'1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0');
 const action=YAML.parse(fs.readFileSync('.github/actions/liquibase-perf-setup/action.yml','utf8'));
 const prepareStep=action.runs.steps.find(s=>s.id==='prepare');
 assert.equal(prepareStep.env.PERFORMANCE_SOURCE_ROOT,'${{ inputs.source_root }}');
 assert.equal(prepareStep.env.PERFORMANCE_PREPARE_SCRIPT,'${{ github.action_path }}/../../../scripts/performance/prepare.mjs');
 assert.match(prepareStep.run,/node "\$PERFORMANCE_PREPARE_SCRIPT"/);
 assert.equal(baselineReleaseAPI,api);
 assert.equal(baselineArchiveSHA,'da6132c95185f2a11561628458d34b22e7cb928c574d8ecec9f06d44fc8d0f01');
 const prepare=fs.readFileSync('scripts/performance/prepare.mjs','utf8');
 assert.match(prepare,/apiURL:\s*baselineReleaseAPI/);
 assert.ok(prepare.indexOf('preparationCheckout(')<prepare.indexOf('fs.mkdirSync('));
 for(const retained of ['-trimpath','-buildvcs=true','validateMetadata(','safeArchiveNames(','--no-same-owner','installJdbcDriver(','refs/tags/v0.1.1-rc.7','artifacts.json'])assert.ok(prepare.includes(retained));
 const runner=fs.readFileSync('scripts/performance/run-diagnostic.mjs','utf8');
 assert.match(runner,/downloadPinned\(baselineReleaseAPI,baselineArchiveSHA/);
 assert.ok(runner.indexOf('downloadPinned(baselineReleaseAPI,baselineArchiveSHA')<runner.indexOf("warm(repo,['jhipster'])"));
});
