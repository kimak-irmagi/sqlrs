import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import test from "node:test";
import {
  validateRequest, releaseBody, prepareRelease, publishRelease, verifyRelease,
  qualifySource, requireTagProtection, validateDownload, publicationReport, readReleaseBody, GitHubAPI,
} from "./runtime-go-release.mjs";

// MP01-MP11: public requirements, not a real credential/publication test.
const sha = "b".repeat(40);
const manifest = `sha256:${"a".repeat(64)}`;
const digest = bytes => createHash("sha256").update(bytes).digest("hex");
const input = (extra = {}) => ({ operation: "publish", version: "v0.6.0-rc.1", commit: sha,
  notes: "Add the execution-environment facade.", ref: "refs/heads/main", event: "workflow_dispatch", ...extra });
const request = extra => validateRequest(input(extra));
const fail = status => Object.assign(new Error("injected API failure"), { status });

/** In-memory GitHub with committed-write/lost-response injection and paginated reads. */
function github() {
  const state = { ref: null, refs: new Map(), releases: [], writes: [], fault: 0, conflict: 0, calls: [],
    runs: [{ id: 7, head_sha: sha, head_branch: "main", event: "push", path: ".github/workflows/ci.yml", status: "completed", conclusion: "success" }],
    rules: [{ id: 1, target: "tag", enforcement: "active", conditions: { ref_name: { include: ["refs/tags/backend/libs/runtime-go/v*"], exclude: [] } }, rules: [{ type: "update" }, { type: "deletion" }] }] };
  const api = { state, async request(method, route, options = {}) {
    state.calls.push([method, route]);
    const url = new URL(route, "https://example.invalid");
    const p = url.pathname;
    let result;
    if (method === "GET") {
      if (p === "") return { default_branch: "main", visibility: "public" };
      if (p === "/rulesets") return state.rules.map(r => ({ id: r.id }));
      if (p === "/rulesets/1") return state.rules[0];
      if (p.includes("/actions/")) return { workflow_runs: state.runs };
      if (p.startsWith("/git/ref/")) { const ref = state.refs.get(`refs/${decodeURIComponent(p.slice(9))}`); if (!ref) throw fail(404); return ref; }
      if (p === "/releases") return state.releases;
      if (p.startsWith("/releases/tags/")) {
        const release = state.releases.find(r => !r.draft && r.tag_name === p.slice(15));
        if (!release) throw fail(404);
        return release;
      }
      if (/^\/releases\/\d+\/assets$/.test(p)) return state.releases.find(r => r.id === +p.split("/")[2]).assets;
      if (/^\/releases\/assets\/\d+$/.test(p)) return state.releases.flatMap(r => r.assets).find(a => a.id === +p.split("/").at(-1)).bytes;
      throw Error(`unhandled GET ${p}`);
    }
    const body = options.body;
    if (p === "/git/refs") {
      result = state.ref = { ref: body.ref, object: { type: "commit", sha: body.sha } };
      state.refs.set(body.ref, result);
    }
    else if (p === "/releases") {
      result = { id: state.releases.length + 1, ...body, assets: [] };
      state.releases.push(result);
    } else if (method === "POST" && /^\/releases\/\d+\/assets$/.test(p)) {
      const release = state.releases.find(r => r.id === +p.split("/")[2]);
      result = { id: 10 + state.releases.flatMap(r => r.assets).length, name: url.searchParams.get("name"), state: "uploaded", size: body.length, bytes: Buffer.from(body) };
      release.assets.push(result);
    } else if (method === "PATCH") {
      result = Object.assign(state.releases.find(r => r.id === +p.split("/")[2]), body);
    } else throw Error(`unhandled write ${p}`);
    state.writes.push([method, route, body]);
    if (state.fault === state.writes.length) { state.fault = 0; throw fail(503); }
    if (state.conflict === state.writes.length) { state.conflict = 0; throw fail(409); }
    return result;
  } };
  return api;
}

test("MP01 exact inputs, modes and inert bounded text", () => {
  for (const version of ["v0.0.1-rc.1", "v0.12.34-rc.42"]) assert.equal(request({ version }).version, version);
  assert.equal(request({ operation: undefined }).operation, "validate");
  const text = '` $(touch no) \\"\n::set-output name=x::bad\nEOF\nПривет';
  assert.equal(request({ notes: text }).notes, text);
  for (const extra of [
    { version: "v1.0.0" }, { version: "v0.01.0-rc.1" }, { version: "v0.1.0-rc.0" },
    { commit: "main" }, { commit: "B".repeat(40) }, { commit: sha.slice(1) },
    { ref: "refs/heads/other" }, { event: "push" }, { operation: "unknown" },
    { notes: "" }, { notes: "é".repeat(8193) }, { notes: "\ud800" },
    { notes: "<!-- sqlrs-runtime-go-provenance:v1 -->" }, { candidate_version: "v0.6.0-rc.2" },
    { operation: "verify", notes: "unwanted" }, { version: "v0.6.0", notes: "", candidate_version: "v0.5.0-rc.1" },
    { version: "v0.6.0", candidate_version: "v0.6.0-rc.1" }, { version: "v0.6.0", notes: "" },
  ]) assert.throws(() => request(extra), undefined, JSON.stringify(extra));
  assert.equal(Buffer.byteLength(request({ notes: "é".repeat(8192) }).notes), 16384);
  assert.equal(request({ operation: "verify", notes: "" }).operation, "verify");
  assert.equal(request({ version: "v0.6.0", candidate_version: "v0.6.0-rc.1", notes: "" }).isRC, false);
});

test("MP01/04 independent deterministic assets and body, size checks before writes", () => {
  const req = request();
  const plan = prepareRelease(req, manifest);
  const expected = Buffer.from(JSON.stringify({ module: "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go",
    tag: "backend/libs/runtime-go/v0.6.0-rc.1", source_sha: sha,
    semantic_schema: "sqlrs.runtime.v2.canonical.v1", bundle_schema_version: "sqlrs.runtime.conformance.bundle-schema.v1",
    bundle_version: "runtime-v2-canonical-v1.1", manifest_digest: manifest }) + "\n");
  assert.deepEqual(plan.assets[0].bytes, expected);
  assert.equal(plan.assets[0].name, `runtime-go-v0.6.0-rc.1-${digest(expected)}.attestation.json`);
  assert.equal(plan.assets[1].bytes.toString(), `sha256:${digest(expected)}\n`);
  assert.equal(plan.body, releaseBody(req.notes, sha));
  assert.ok(plan.body.endsWith("<!-- /sqlrs-runtime-go-provenance:v1 -->"));
  assert.ok(plan.body.includes(`Source: ${sha}`));
  for (const notes of ["\\".repeat(16384), "\u0001".repeat(16384), "é".repeat(8192)]) {
    const bounded = prepareRelease(request({ notes }), manifest);
    assert.ok(Buffer.byteLength(bounded.body) <= 20480);
    assert.ok(Buffer.byteLength(JSON.stringify(bounded.payload)) <= 131072);
  }
  assert.throws(() => prepareRelease(req, "bad digest"));
  assert.throws(() => prepareRelease(request({ operation: "verify", notes: "" }), manifest, "untrusted body"));
  assert.throws(() => prepareRelease(req, manifest, "different body"));
});

test("MP02 exact successful source CI, latest run and pagination", async () => {
  const api = github();
  await qualifySource(api, request());
  for (const change of [{ conclusion: "failure" }, { status: "in_progress" }, { head_sha: "c".repeat(40) },
    { head_branch: "other" }, { event: "pull_request" }, { path: "another.yml" }]) {
    const other = github(); other.state.runs = [{ ...other.state.runs[0], ...change }];
    await assert.rejects(qualifySource(other, request()));
  }
  api.state.runs.push({ ...api.state.runs[0], id: 8, conclusion: "cancelled" });
  await assert.rejects(qualifySource(api, request()));
  api.state.runs = [];
  await assert.rejects(qualifySource(api, request()));
  const paged = github();
  const original = paged.request;
  paged.request = async (m, p, o) => p.includes("/actions/") && new URL(p,"https://example.invalid").searchParams.get("page") === "1"
    ? { workflow_runs: Array.from({ length: 100 }, (_, i) => ({ id: i, head_sha: "c".repeat(40) })) }
    : original(m, p, o);
  await qualifySource(paged, request());
  assert.ok(paged.state.calls.some(([, p]) => p.includes("page=2")));
});

test("MP03 tag policy and read failures fail closed", async () => {
  await requireTagProtection(github());
  for (const change of [{ target: "branch" }, { enforcement: "disabled" }, { rules: [{ type: "update" }] },
    { conditions: { ref_name: { include: ["refs/tags/v*"], exclude: [] } } },
    { conditions: { ref_name: { include: ["~ALL"], exclude: ["refs/tags/backend/libs/runtime-go/v*"] } } }]) {
    const api = github(); Object.assign(api.state.rules[0], change);
    await assert.rejects(requireTagProtection(api)); assert.equal(api.state.writes.length, 0);
  }
  const absent = github(); absent.state.rules = [];
  await assert.rejects(requireTagProtection(absent));
  await assert.rejects(requireTagProtection({ request: async () => { throw fail(403); } }));
});

test("MP04 new publication orders exact tag, draft, assets and publication", async () => {
  const api = github(), req = request(), plan = prepareRelease(req, manifest);
  await publishRelease(api, req, plan);
  assert.deepEqual(api.state.writes.map(([m, p]) => [m, p.split("?")[0]]), [
    ["POST", "/git/refs"], ["POST", "/releases"], ["POST", "/releases/1/assets"],
    ["POST", "/releases/1/assets"], ["PATCH", "/releases/1"],
  ]);
  assert.deepEqual(api.state.writes[0][2], { ref: `refs/tags/${req.tag}`, sha });
  const payload = api.state.writes[1][2];
  assert.equal(payload.draft, true); assert.equal(payload.make_latest, "false");
  assert.equal(payload.prerelease, true); assert.equal(payload.target_commitish, undefined);
  assert.equal(api.state.writes.at(-1)[2].make_latest, "false");
  assert.equal(api.state.releases[0].draft, false);
});

test("MP05 conflicts and ambiguous read/write errors never overwrite output", async () => {
  const req = request(), plan = prepareRelease(req, manifest);
  for (const mutate of [
    api => { api.state.ref.object.sha = "c".repeat(40); },
    api => { api.state.releases[0].body += "tampered"; },
    api => { api.state.releases[0].prerelease = false; },
    api => { api.state.releases[0].name = "wrong"; },
    api => { api.state.releases[0].assets[0].bytes = Buffer.from("changed"); },
    api => { api.state.releases[0].assets.pop(); },
    api => { api.state.releases[0].assets.push({ id: 99, name: "unknown", bytes: Buffer.from("x") }); },
    api => { api.state.releases.push({ ...api.state.releases[0], id: 2 }); },
  ]) {
    const api = github(); await publishRelease(api, req, plan); mutate(api);
    const before = api.state.writes.length;
    await assert.rejects(publishRelease(api, req, plan)); assert.equal(api.state.writes.length, before);
  }
  for (const status of [401, 403, 404, 429, 500]) {
    const api = github(), original = api.request;
    api.request = async (m, p, o) => { if (p.startsWith("/releases")) throw fail(status); return original(m,p,o); };
    await assert.rejects(publishRelease(api, req, plan)); assert.equal(api.state.writes.length, 0);
  }
  for (const status of [403, 404, 422, 503]) {
    const api = github(), original = api.request;
    api.request = async (m, p, o) => { if (m !== "GET") throw fail(status); return original(m,p,o); };
    await assert.rejects(publishRelease(api, req, plan)); assert.equal(api.state.writes.length, 0);
  }
});

test("MP06 every committed-write/lost-response boundary resumes safely", async () => {
  const req = request(), plan = prepareRelease(req, manifest);
  for (let boundary = 1; boundary <= 5; boundary++) {
    const api = github(); api.state.fault = boundary;
    await assert.rejects(publishRelease(api, req, plan), error => {
      assert.equal(error.publicationState.outcome,"unknown");
      assert.equal(error.publicationState.last_confirmed.tag_present,boundary > 1);
      assert.ok(["create-tag","create-draft","upload-asset","publish-draft"].includes(error.publicationState.attempted_write));
      return true;
    });
    await publishRelease(api, req, plan);
    assert.equal(api.state.writes.length, 5, `boundary ${boundary}`);
    const before = api.state.writes.length;
    await publishRelease(api, req, plan); await verifyRelease(api, req, plan);
    assert.equal(api.state.writes.length, before);
  }
  const api = github();
  api.state.releases.push({ id: 1, ...plan.payload, target_commitish: sha, assets: [] });
  await publishRelease(api, req, plan);
  assert.equal(api.state.writes.filter(([, p]) => p === "/releases").length, 0);
  const wrong = github(); wrong.state.releases.push({ id: 1, ...plan.payload, target_commitish: "main", assets: [] });
  await assert.rejects(publishRelease(wrong, req, plan)); assert.equal(wrong.state.writes.length, 0);
});

test("MP06 concurrent matching writes are reread, conflicts cannot become blind retries", async () => {
  for (let boundary = 1; boundary <= 5; boundary++) {
    const api = github(); api.state.conflict = boundary;
    await publishRelease(api, request(), prepareRelease(request(), manifest));
    assert.equal(api.state.writes.length, 5);
  }
});

test("MP07/08 final body, same S and family, mandatory current candidate digest", async () => {
  const api = github(), rc = request(), rcPlan = prepareRelease(rc, manifest);
  await publishRelease(api, rc, rcPlan);
  const final = request({ version: "v0.6.0", candidate_version: rc.version, notes: "" });
  const plan = prepareRelease(final, manifest, rcPlan.body);
  for (const candidateDigest of [undefined, "0".repeat(64)]) {
    const before = api.state.writes.length;
    await assert.rejects(publishRelease(api, final, plan, candidateDigest));
    assert.equal(api.state.writes.length, before);
  }
  await publishRelease(api, final, plan, digest(rcPlan.body));
  assert.equal(api.state.releases[1].body, rcPlan.body);
  assert.equal(api.state.releases[1].prerelease, false);
  assert.equal(api.state.releases[1].tag_name, "backend/libs/runtime-go/v0.6.0");
});

test("MP09 read-only verification cannot accept draft, absent or partial output", async () => {
  const req = request(), plan = prepareRelease(req, manifest), api = github();
  await assert.rejects(verifyRelease(api, req, plan));
  await assert.rejects(publishRelease(api, request({ operation: "validate" }), plan));
  await assert.rejects(publishRelease(api, request({ operation: "verify", notes: "" }), plan));
  await publishRelease(api, req, plan);
  const before = api.state.writes.length;
  await verifyRelease(api, req, plan); assert.equal(api.state.writes.length, before);
  assert.equal(await readReleaseBody(api, req.tag), plan.body);
  api.state.releases[0].draft = true;
  await assert.rejects(verifyRelease(api, req, plan)); assert.equal(api.state.writes.length, before);
  await assert.rejects(readReleaseBody(api, req.tag));
});

test("MP10 exact public module metadata rejects replacements, missing sums and wrong origin", () => {
  const data = { Path: "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go", Version: "v0.6.0-rc.1",
    Sum: `h1:${Buffer.alloc(32,1).toString("base64")}`, GoModSum: `h1:${Buffer.alloc(32,2).toString("base64")}`,
    Zip: "module.zip", GoMod: "go.mod", Origin: { Hash: sha } };
  assert.deepEqual(validateDownload(data, request()), { module_sum: data.Sum, go_mod_sum: data.GoModSum });
  for (const extra of [{ Path:"wrong" }, { Version:"v0.6.0" }, { Sum:"h1:" }, { GoModSum:"" },
    { Zip:"" }, { GoMod:"" }, { Replace:{} }, { Error:"denied" }, { Origin:{ Hash:"c".repeat(40) } }]) {
    assert.throws(() => validateDownload({ ...data, ...extra }, request()));
  }
});

test("MP11 diagnostic evidence binds run/source without copying the release body", () => {
  const req = request(), plan = prepareRelease(req, manifest);
  const report = publicationReport(req, plan.body, { automationSHA:"c".repeat(40), runId:"123", attempt:"2", gates:{ metadata:"success" } });
  assert.equal(report.source_sha, sha); assert.equal(report.automation_sha, "c".repeat(40));
  assert.equal(report.release_body_sha256, digest(plan.body));
  assert.equal(report.run_attempt,"2"); assert.equal(report.release_body,undefined);
  assert.equal(report.gates.metadata,"success");
});

test("GitHub adapter constrains endpoints, masks failures and handles downloads without leaking auth", async () => {
  const calls = [];
  const api = new GitHubAPI("token-not-in-errors", "kimak-irmagi/sqlrs", async (url, options) => {
    calls.push([url,options]); return new Response(JSON.stringify({ id:1 }), {status:200});
  });
  assert.deepEqual(await api.request("GET","/releases"), {id:1});
  assert.equal(calls[0][1].headers.Authorization,"Bearer token-not-in-errors");
  await api.request("POST","/git/refs",{body:{ref:"refs/tags/x",sha}});
  assert.equal(JSON.parse(calls[1][1].body).sha,sha);
  await api.request("POST","/releases/1/assets?name=x",{body:Buffer.from("x"),upload:true});
  assert.ok(calls[2][0].startsWith("https://uploads.github.com/"));
  for (const route of ["https://evil.invalid/", "//evil.invalid/", "/../other", "/releases#fragment"]) {
    await assert.rejects(api.request("GET",route));
  }
  for (const status of [401,403,404,429,500]) {
    const bad = new GitHubAPI("secret", "kimak-irmagi/sqlrs", async () => new Response("private response",{status}));
    await assert.rejects(bad.request("GET","/releases"), error => error.status === status && !/secret|private response/.test(error.message));
  }
  const malformed = new GitHubAPI("secret", "kimak-irmagi/sqlrs", async () => new Response("not json"));
  await assert.rejects(malformed.request("GET","/releases"));
  const raw = new GitHubAPI("secret", "kimak-irmagi/sqlrs", async () => new Response("asset"));
  assert.equal((await raw.request("GET","/releases/assets/1",{binary:true})).toString(),"asset");
  const redirectCalls = [];
  const redirect = new GitHubAPI("secret", "kimak-irmagi/sqlrs", async (url,options) => {
    redirectCalls.push(options);
    return redirectCalls.length === 1 ? new Response(null,{status:302,headers:{location:"https://release-assets.githubusercontent.com/file"}}) : new Response("asset");
  });
  assert.equal((await redirect.request("GET","/releases/assets/1",{binary:true})).toString(),"asset");
  assert.equal(redirectCalls[1].headers.Authorization,undefined);
  const unsafe = new GitHubAPI("secret", "kimak-irmagi/sqlrs", async () => new Response(null,{status:302,headers:{location:"https://evil.invalid/file"}}));
  await assert.rejects(unsafe.request("GET","/releases/assets/1",{binary:true}));
});
