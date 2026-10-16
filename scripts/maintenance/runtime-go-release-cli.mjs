import { readFile, writeFile, appendFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  validateRequest, prepareRelease, publishRelease, verifyRelease, readReleaseBody,
  qualifySource, requireTagProtection, validateDownload, publicationReport, GitHubAPI,
} from "./runtime-go-release.mjs";

/** Workflow entry point; H runs policy, S supplies only the manifest's data.
 * Dependency injection exercises the same adapter path without GitHub writes.
 * Requirements: docs/architecture/runtime-go-module-publication.md.
 */
export async function runCommand(command, env, dependencies = {}) {
  env = { ...env, REPORT_FILE: env.REPORT_FILE || resolve(env.RUNNER_TEMP,"runtime-publication-job.json") };
  const request = validateRequest({ operation:env.INPUT_OPERATION || "validate", version:env.INPUT_VERSION,
    commit:env.INPUT_COMMIT, notes:env.INPUT_NOTES || "", candidate_version:env.INPUT_CANDIDATE_VERSION || "",
    ref:env.GITHUB_REF, event:env.GITHUB_EVENT_NAME });
  if (!["inputs","source","metadata","publish","download"].includes(command)) throw new Error("unknown publication command");
  const io = { readFile, writeFile, appendFile, ...dependencies };
  const output = async value => { if (env.GITHUB_OUTPUT) await io.appendFile(env.GITHUB_OUTPUT,value); };
  if (command === "inputs") {
    await output(`version=${request.version}\ncommit=${request.sourceSHA}\nis_rc=${request.isRC}\n`);
    return;
  }
  try {
    const api = dependencies.api ?? new GitHubAPI(env.GH_TOKEN,env.GITHUB_REPOSITORY);
    if (command === "source") {
      await qualifySource(api,request); await requireTagProtection(api); return;
    }
    const manifest = (await io.readFile(resolve(env.SOURCE_ROOT,"backend/libs/runtime-go/conformance/canonical-v1/manifest.sha256"),"utf8")).trim();
    const body = request.isRC && request.operation !== "verify" ? undefined :
      await readReleaseBody(api, request.isRC ? request.tag : `backend/libs/runtime-go/${request.candidateVersion}`);
    const plan = prepareRelease(request,manifest,body);
    const result = command === "publish" ? await publishRelease(api,request,plan,env.CANDIDATE_DIGEST) :
      await verifyRelease(api,request,plan);
    let sums = {};
    if (command === "download") sums = validateDownload(JSON.parse(await io.readFile(env.DOWNLOAD_FILE,"utf8")),request);
    const gates = command === "download" ? {metadata:"success",public_consumer:"success"} : {metadata:"success"};
    const report = publicationReport(request,plan.body,{ automationSHA:env.GITHUB_SHA,runId:env.GITHUB_RUN_ID,attempt:env.GITHUB_RUN_ATTEMPT,gates,sums });
    await io.writeFile(env.REPORT_FILE,JSON.stringify(report,null,2)+"\n");
    await output(`body_digest=${result.body_digest}\n`);
  } catch (error) {
    await io.writeFile(env.REPORT_FILE,JSON.stringify({ version:request.version,source_sha:request.sourceSHA,
      automation_sha:env.GITHUB_SHA,run_id:env.GITHUB_RUN_ID,run_attempt:env.GITHUB_RUN_ATTEMPT,
      operation:request.operation,gates:{[command]:"failure"},
      ...(command === "publish" ? {publication_state:error.publicationState ?? {observation:"unknown"}} : {}) })+"\n");
    throw error;
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { await runCommand(process.argv[2],process.env); }
  catch (error) { console.error(`Runtime Go publication failed: ${error.message}`); process.exitCode=1; }
}
