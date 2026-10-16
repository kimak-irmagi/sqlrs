import { createHash } from "node:crypto";
import { buildRuntimeAttestation } from "./create-runtime-v2-attestation.mjs";

// Policy and narrow API adapter for docs/architecture/runtime-go-module-publication.md.
export const modulePath = "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go";
const tagPrefix = "backend/libs/runtime-go/";
const versionPattern = /^v0\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$/;
const marker = "<!-- sqlrs-runtime-go-provenance:v1 -->";
const hash = value => createHash("sha256").update(value).digest("hex");
const requireValue = (condition, context) => { if (!condition) throw new Error(context); };
const bounded = (value, maximum, context) => requireValue(typeof value === "string" && value.isWellFormed() && Buffer.byteLength(value) <= maximum, context);

/** Parse only explicit main dispatches. Untrusted notes are never shell syntax. */
export function validateRequest({ operation = "validate", version, commit, notes = "", candidate_version = "", ref, event }) {
  requireValue(ref === "refs/heads/main" && event === "workflow_dispatch", "main workflow dispatch required");
  requireValue(["validate", "publish", "verify"].includes(operation), "invalid operation");
  requireValue(typeof version === "string" && versionPattern.test(version), "invalid version");
  requireValue(typeof commit === "string" && /^[0-9a-f]{40}$/.test(commit), "exact source SHA required");
  bounded(notes, 16 * 1024, "invalid or oversized notes");
  requireValue(!notes.includes("sqlrs-runtime-go-provenance:v1"), "reserved provenance marker in notes");
  const isRC = version.includes("-rc.");
  if (isRC) requireValue(candidate_version === "", "RC cannot name another candidate");
  else requireValue(typeof candidate_version === "string" && versionPattern.test(candidate_version) &&
    candidate_version.startsWith(`${version}-rc.`), "same-family candidate required");
  requireValue(isRC && operation !== "verify" ? notes.trim().length > 0 : notes === "", "notes not permitted or required notes absent");
  return { operation, version, sourceSHA: commit, notes, isRC, candidateVersion: candidate_version, tag: `${tagPrefix}${version}` };
}

/** Version-independent body lets GA inherit RC bytes without rewriting history. */
export function releaseBody(notes, sourceSHA) {
  const footer = `${marker}\nSource: ${sourceSHA}\n\n` +
    "Compatibility: published `sqlrs.runtime.v2` v0.2.0 contract (historical); " +
    "current identity sqlrs.runtime.v2.canonical.v1. Historical aliases: " +
    "sqlrs.runtime.v2.aliases.v1, sqlrs.runtime.v2.alias-expansion-trace.v1. " +
    "Historical cache sqlrs.resolution-cache.v1; current cache sqlrs.resolution-cache.canonical.v1. " +
    "Conformance: sqlrs.runtime.conformance.bundle-schema.v1, runtime-v2-canonical-v1.1. " +
    "Legacy bridge: v0.1.1-rc.6; v0.1.0 remains retracted.\n<!-- /sqlrs-runtime-go-provenance:v1 -->";
  return `${notes}\n\n${footer}`;
}

/** Construct every bounded payload/asset before making a public tag visible. */
export function prepareRelease(request, manifestDigest, existingBody) {
  let body;
  if (request.isRC && request.operation !== "verify") {
    body = releaseBody(request.notes, request.sourceSHA);
    requireValue(existingBody === undefined || existingBody === body, "conflicting supplied body");
  } else {
    bounded(existingBody, 20 * 1024, "missing or oversized published body");
    const split = existingBody.indexOf(`\n\n${marker}`);
    requireValue(split >= 0, "missing provenance footer");
    const notes = existingBody.slice(0, split);
    bounded(notes, 16 * 1024, "invalid published notes");
    requireValue(notes.trim().length > 0 && !notes.includes("sqlrs-runtime-go-provenance:v1") &&
      releaseBody(notes, request.sourceSHA) === existingBody, "published provenance differs");
    body = existingBody;
  }
  bounded(body, 20 * 1024, "oversized release body");
  const payload = { tag_name: request.tag, name: `Runtime Go module ${request.version}`, body,
    draft: true, prerelease: request.isRC, make_latest: "false" };
  bounded(JSON.stringify(payload), 128 * 1024, "oversized release payload");
  return { body, payload, manifestDigest, assets: buildRuntimeAttestation(request.version, request.sourceSHA, manifestDigest) };
}

/** Enumerate complete pages; malformed/failed reads must never mean absence. */
async function list(api, route, key) {
  const results = [];
  for (let page = 1; page <= 1000; page++) {
    const response = await api.request("GET", `${route}${route.includes("?") ? "&" : "?"}per_page=100&page=${page}`);
    const values = key ? response[key] : response;
    requireValue(Array.isArray(values), "invalid paginated API response");
    results.push(...values);
    if (values.length < 100) return results;
  }
  throw new Error("API pagination limit exceeded");
}

/** Exact successful post-merge ci, not a PR check or another SHA's green run. */
export async function qualifySource(api, request) {
  const runs = await list(api, `/actions/workflows/ci.yml/runs?event=push&branch=main&head_sha=${request.sourceSHA}`, "workflow_runs");
  const matching = runs.filter(run => run.head_sha === request.sourceSHA && run.head_branch === "main" &&
    run.event === "push" && run.path === ".github/workflows/ci.yml").sort((a,b) => b.id - a.id);
  requireValue(matching.length > 0 && matching[0].status === "completed" && matching[0].conclusion === "success", "source post-merge CI is not successful");
}

/** Require active update+deletion protection for the whole nested tag namespace. */
export async function requireTagProtection(api) {
  for (const entry of await list(api, "/rulesets")) {
    requireValue(Number.isSafeInteger(entry.id) && entry.id > 0, "invalid ruleset id");
    const rule = await api.request("GET", `/rulesets/${entry.id}`);
    const names = rule.conditions?.ref_name;
    if (rule.target === "tag" && rule.enforcement === "active" &&
      names?.include?.some(value => value === `refs/tags/${tagPrefix}v*` || value === "~ALL") &&
      Array.isArray(names.exclude) && names.exclude.length === 0 &&
      rule.rules?.some(value => value.type === "update") && rule.rules?.some(value => value.type === "deletion")) return;
  }
  throw new Error("protected module tag policy required");
}

/** Read draft-visible state before writes. Get-by-tag alone cannot discover drafts. */
async function readState(api, request, plan) {
  const matching = (await list(api, "/releases")).filter(release => release.tag_name === request.tag);
  requireValue(matching.length <= 1, "duplicate release coordinate");
  const release = matching[0];
  let reference;
  try { reference = await api.request("GET", `/git/ref/tags/${request.tag}`); }
  catch (error) { if (error.status !== 404) throw error; }
  if (reference) requireValue(reference.ref === `refs/tags/${request.tag}` && reference.object?.type === "commit" &&
    reference.object.sha === request.sourceSHA, "conflicting tag source");
  const present = new Set();
  if (release) {
    requireValue(Number.isSafeInteger(release.id) && release.id > 0 && typeof release.draft === "boolean" &&
      release.prerelease === request.isRC && release.name === plan.payload.name && release.body === plan.body,
    "conflicting release metadata");
    if (!reference) requireValue(release.draft && release.target_commitish === request.sourceSHA, "untagged draft must explicitly target source SHA");
    for (const asset of await list(api, `/releases/${release.id}/assets`)) {
      const expected = plan.assets.find(value => value.name === asset.name);
      requireValue(expected && !present.has(asset.name) && Number.isSafeInteger(asset.id) && asset.id > 0 &&
        asset.state === "uploaded" && asset.size === expected.bytes.length, "conflicting release asset");
      const bytes = await api.request("GET", `/releases/assets/${asset.id}`, { binary: true });
      requireValue(Buffer.isBuffer(bytes) && bytes.equals(expected.bytes), "release asset bytes differ");
      present.add(asset.name);
    }
    requireValue(release.draft || present.size === plan.assets.length, "published release assets incomplete");
  }
  return { release, reference, present };
}

/** Published output is immutable: verify bytes/metadata without write authority. */
export async function verifyRelease(api, request, plan) {
  await requireTagProtection(api);
  const state = await readState(api, request, plan);
  requireValue(state.reference && state.release && !state.release.draft, "complete published release required");
  return { body: plan.body, body_digest: hash(plan.body) };
}

/** Read only a published release body; drafts cannot authorize promotion. */
export async function readReleaseBody(api, tag) {
  const release = await api.request("GET", `/releases/tags/${tag}`);
  requireValue(release.tag_name === tag && release.draft === false && typeof release.body === "string", "published release body required");
  return release.body;
}

/** Monotonic state machine. Only matching drafts can be completed, never repaired.
 * Current-run consumer success is enforced by workflow needs; its body digest
 * binds the RC bytes re-read here. No source code executes under write authority.
 */
export async function publishRelease(api, request, plan, candidateDigest) {
  requireValue(request.operation === "publish", "publication mode required");
  requireValue(plan.payload.tag_name === request.tag && plan.payload.body === plan.body, "invalid publication plan");
  await requireTagProtection(api);
  if (!request.isRC) {
    requireValue(candidateDigest === hash(plan.body), "current candidate verification required");
    const candidate = { ...request, version: request.candidateVersion, tag: `${tagPrefix}${request.candidateVersion}`, isRC: true, operation: "verify" };
    await verifyRelease(api, candidate, prepareRelease(candidate, plan.manifestDigest, plan.body));
  }
  // A conflict can mean a concurrent matching write; reread the complete state.
  for (let attempt = 0; attempt < 8; attempt++) {
    const { reference, release, present } = await readState(api, request, plan);
    if (release && !release.draft) return;
    let stage;
    try {
      if (!reference) {
        stage = "create-tag";
        await api.request("POST", "/git/refs", { body: { ref: `refs/tags/${request.tag}`, sha: request.sourceSHA } });
      } else if (!release) {
        stage = "create-draft";
        await api.request("POST", "/releases", { body: plan.payload });
      }
      else {
        const missing = plan.assets.find(asset => !present.has(asset.name));
        stage = missing ? "upload-asset" : "publish-draft";
        if (missing) await api.request("POST", `/releases/${release.id}/assets?name=${encodeURIComponent(missing.name)}`, { body: missing.bytes, upload: true });
        else await api.request("PATCH", `/releases/${release.id}`, { body: { draft: false, make_latest: "false" } });
      }
    } catch (error) {
      if (error.status !== 409 && error.status !== 422) {
        // A failed/lost HTTP response is not proof that the attempted write
        // did not commit. Record only confirmed pre-write state, not guesses.
        error.publicationState = { attempted_write:stage, outcome:"unknown", last_confirmed:{
          tag_present:Boolean(reference), release_id:release?.id ?? null,
          draft:release?.draft ?? null, asset_count:present.size,
        } };
        throw error;
      }
      // Read validates conflicts; a repeat with no state progress is bounded.
      await readState(api, request, plan);
    }
  }
  throw new Error("publication did not converge; inspect partial state");
}

/** Exact public Go download metadata; a staged proxy is not publication proof. */
export function validateDownload(data, request) {
  const sum = value => typeof value === "string" && /^h1:[A-Za-z0-9+/]{43}=$/.test(value);
  requireValue(data.Path === modulePath && data.Version === request.version && !data.Replace && !data.Error &&
    sum(data.Sum) && sum(data.GoModSum) && typeof data.Zip === "string" && data.Zip.length > 0 &&
    typeof data.GoMod === "string" && data.GoMod.length > 0 && (!data.Origin || data.Origin.Hash === request.sourceSHA), "public module download differs");
  return { module_sum: data.Sum, go_mod_sum: data.GoModSum };
}

/** Diagnostic only: no cross-run report can authorize promotion. */
export function publicationReport(request, body, { automationSHA, runId, attempt, gates, sums = {} }) {
  return { module: modulePath, version: request.version, tag: request.tag, source_sha: request.sourceSHA,
    automation_sha: automationSHA, run_id: runId, run_attempt: attempt, operation: request.operation,
    release_body_sha256: hash(body), ...sums, gates };
}

/** Fixed-repository GitHub adapter. API errors never log token, body or server text. */
export class GitHubAPI {
  constructor(token, repository, fetcher = fetch) {
    requireValue(Boolean(token) && repository === "kimak-irmagi/sqlrs", "publication repository/token required");
    this.token = token; this.repository = repository; this.fetch = fetcher;
  }

  async request(method, route, { body, binary = false, upload = false } = {}) {
    requireValue(/^\/[A-Za-z0-9_/?=&.%+-]*$/.test(route) && !route.includes("..") && !route.startsWith("//"), "invalid API route");
    const bytes = body === undefined ? undefined : Buffer.isBuffer(body) ? body : JSON.stringify(body);
    if (bytes !== undefined) requireValue(Buffer.byteLength(bytes) <= 128 * 1024, "oversized API payload");
    const headers = { Authorization: `Bearer ${this.token}`, Accept: binary ? "application/octet-stream" : "application/vnd.github+json",
      "X-GitHub-Api-Version": "2022-11-28", "Content-Type": upload ? "application/octet-stream" : "application/json" };
    const url = `https://${upload ? "uploads" : "api"}.github.com/repos/${this.repository}${route}`;
    let response;
    try {
      response = await this.fetch(url, { method, headers, body: bytes, redirect: "manual", signal: AbortSignal.timeout(30000) });
      if (binary && response.status === 302) {
        const location = new URL(response.headers.get("location"));
        requireValue(location.protocol === "https:" && ["release-assets.githubusercontent.com", "objects.githubusercontent.com"].includes(location.hostname) && !location.username && !location.password, "unsafe asset redirect");
        response = await this.fetch(location.href, { method:"GET", headers: { Accept:"application/octet-stream" }, redirect:"error", signal: AbortSignal.timeout(30000) });
      }
    } catch { throw new Error("GitHub request failed"); }
    if (!response.ok) throw Object.assign(new Error(`GitHub API status ${response.status}`), { status: response.status });
    requireValue(Number(response.headers.get("content-length") ?? 0) <= 8 * 1024 * 1024, "oversized GitHub response");
    const content = Buffer.from(await response.arrayBuffer());
    requireValue(content.length <= 8 * 1024 * 1024, "oversized GitHub response");
    if (binary) return content;
    try { return JSON.parse(content.toString("utf8")); }
    catch { throw new Error("invalid GitHub JSON response"); }
  }
}
