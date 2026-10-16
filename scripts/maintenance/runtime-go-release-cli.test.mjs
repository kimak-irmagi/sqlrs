import assert from "node:assert/strict";
import test from "node:test";
import { resolve, join } from "node:path";
import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { runCommand } from "./runtime-go-release-cli.mjs";
import { releaseBody, prepareRelease, validateRequest } from "./runtime-go-release.mjs";

const sha = "b".repeat(40);
const env = { INPUT_OPERATION:"publish", INPUT_VERSION:"v0.6.0-rc.1", INPUT_COMMIT:sha, INPUT_NOTES:"Notes",
  GITHUB_REF:"refs/heads/main", GITHUB_EVENT_NAME:"workflow_dispatch", GITHUB_SHA:"c".repeat(40),
  GITHUB_REPOSITORY:"kimak-irmagi/sqlrs", GITHUB_RUN_ID:"17", GITHUB_RUN_ATTEMPT:"2", GH_TOKEN:"fake",
  SOURCE_ROOT:"/checkout/source", REPORT_FILE:"/reports/metadata.json", GITHUB_OUTPUT:"/outputs", DOWNLOAD_FILE:"/download.json" };
const manifest = `sha256:${"a".repeat(64)}`;

test("CLI validates inputs before I/O and emits only bounded safe outputs", async () => {
  const outputs=[];
  const deps = { appendFile:async (...args)=>outputs.push(args), api:{ request:async()=>{throw Error("unexpected API");} } };
  await runCommand("inputs",env,deps);
  assert.equal(outputs.length,1);
  assert.match(outputs[0][1],/is_rc=true\n/);
  assert.ok(!outputs[0][1].includes("Notes"));
  await assert.rejects(runCommand("unknown",env,deps));
  await assert.rejects(runCommand("inputs",{...env,INPUT_COMMIT:"main"},deps));
});

test("CLI metadata and public report use only the explicit S manifest and exact download", async () => {
  const verifyEnv = {...env,INPUT_OPERATION:"verify",INPUT_NOTES:""};
  const req = validateRequest({ operation:"verify",version:env.INPUT_VERSION,commit:sha,notes:"",ref:env.GITHUB_REF,event:env.GITHUB_EVENT_NAME });
  const body = releaseBody("Notes",sha), plan=prepareRelease(req,manifest,body);
  const reads=[], writes=[], outputs=[];
  const assets=plan.assets.map((a,i)=>({id:i+10,name:a.name,size:a.bytes.length,state:"uploaded",bytes:a.bytes}));
  const release={id:1,tag_name:req.tag,name:plan.payload.name,prerelease:true,draft:false,body};
  const api={ async request(method,route) {
    assert.equal(method,"GET");
    if (route.startsWith("/releases/tags/")) return release;
    if (route === "/releases/1") return release;
    if (route.startsWith("/releases?")) return [release];
    if (route.startsWith("/git/ref/")) return {ref:`refs/tags/${req.tag}`,object:{type:"commit",sha}};
    if (route.startsWith("/rulesets?")) return [{id:1}];
    if (route === "/rulesets/1") return {target:"tag",enforcement:"active",conditions:{ref_name:{include:["~ALL"],exclude:[]}},rules:[{type:"update"},{type:"deletion"}]};
    if (route.startsWith("/releases/1/assets?")) return assets;
    return assets.find(a=>route.endsWith(`/${a.id}`)).bytes;
  }};
  const download={Path:"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go",Version:req.version,Sum:`h1:${Buffer.alloc(32,1).toString("base64")}`,GoModSum:`h1:${Buffer.alloc(32,2).toString("base64")}`,Zip:"module.zip",GoMod:"go.mod"};
  const deps={ api, readFile:async p=>{reads.push(p);return p===env.DOWNLOAD_FILE?JSON.stringify(download):manifest;},
    writeFile:async (...args)=>writes.push(args),appendFile:async (...args)=>outputs.push(args) };
  await runCommand("metadata",verifyEnv,deps);
  await runCommand("download",verifyEnv,deps);
  await runCommand("publish",env,deps);
  assert.ok(reads.includes(resolve(env.SOURCE_ROOT,"backend/libs/runtime-go/conformance/canonical-v1/manifest.sha256")));
  assert.equal(writes.length,3); assert.equal(outputs.length,3);
  const report=JSON.parse(writes[1][1]);
  assert.equal(report.gates.public_consumer,"success"); assert.equal(report.module_sum,download.Sum);
  assert.equal(report.release_body,undefined);
  assert.equal(JSON.parse(writes[2][1]).operation,"publish");
});

test("CLI API failures write a static failure report without notes or token", async () => {
  const writes=[];
  const deps={ api:{request:async()=>{throw Error("api failed");}},writeFile:async(...args)=>writes.push(args) };
  await assert.rejects(runCommand("source",env,deps));
  assert.equal(JSON.parse(writes[0][1]).gates.source,"failure");
  assert.ok(!writes[0][1].includes("fake") && !writes[0][1].includes("Notes"));
});

test("MP02 CLI source gate requires both exact post-merge CI and protected tags", async () => {
  const calls=[];
  await runCommand("source",env,{api:{async request(method,route) {
    assert.equal(method,"GET"); calls.push(route);
    if (route.includes("/actions/")) return {workflow_runs:[{id:1,head_sha:sha,head_branch:"main",event:"push",path:".github/workflows/ci.yml",status:"completed",conclusion:"success"}]};
    if (route.startsWith("/rulesets?")) return [{id:1}];
    return {target:"tag",enforcement:"active",conditions:{ref_name:{include:["~ALL"],exclude:[]}},rules:[{type:"update"},{type:"deletion"}]};
  }}});
  assert.ok(calls.some(p=>p.includes("head_sha="+sha)));
  assert.ok(calls.includes("/rulesets/1"));
});

test("MP01/02 actual CLI entry uses H from a source directory lacking release tooling/notes", async () => {
  const root=await mkdtemp(join(tmpdir(),"sqlrs-release-entry-"));
  try {
    const source=join(root,"source"), output=join(root,"outputs");
    await mkdir(join(source,"scripts","maintenance"),{recursive:true});
    await writeFile(join(source,"scripts","maintenance","runtime-go-release-cli.mjs"),'throw Error("wrong source tooling executed");\n');
    const script=fileURLToPath(new URL("./runtime-go-release-cli.mjs",import.meta.url));
    const childEnv={...process.env,...env,SOURCE_ROOT:source,REPORT_FILE:join(root,"report.json"),GITHUB_OUTPUT:output};
    const valid=spawnSync(process.execPath,[script,"inputs"],{cwd:source,env:childEnv,encoding:"utf8"});
    assert.equal(valid.status,0,valid.stderr);
    assert.match(await readFile(output,"utf8"),/commit=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n/);
    const invalid=spawnSync(process.execPath,[script,"inputs"],{cwd:source,env:{...childEnv,INPUT_COMMIT:"main"},encoding:"utf8"});
    assert.notEqual(invalid.status,0);
    assert.match(invalid.stderr,/exact source SHA required/);
    assert.doesNotMatch(invalid.stderr,/Notes|fake|wrong source tooling/);
  } finally { await rm(root,{recursive:true,force:true}); }
});
