import assert from "node:assert/strict";
import test from "node:test";
import { readFile, readdir } from "node:fs/promises";
import { parse } from "yaml";

const workflow = await readFile(new URL("../../.github/workflows/release-runtime-go.yml", import.meta.url), "utf8");
const product = await readFile(new URL("../../.github/workflows/release-local.yml", import.meta.url), "utf8");
const ci = await readFile(new URL("../../.github/workflows/ci.yml", import.meta.url), "utf8");
const fuzz = await readFile(new URL("../../.github/workflows/runtime-v2-fuzz.yml", import.meta.url), "utf8");
const consumerFiles = await readdir(new URL("../../test/runtime-v2-consumer/", import.meta.url));
const historicalNotes = await readFile(new URL("../../backend/libs/runtime-go/RELEASE-v0.3.0.md", import.meta.url), "utf8");
const helper = await readFile(new URL("./runtime-go-release.mjs", import.meta.url), "utf8");
const cli = await readFile(new URL("./runtime-go-release-cli.mjs", import.meta.url), "utf8");
const publicConsumer = await readFile(new URL("../../.github/actions/runtime-go-public-consumer/action.yml", import.meta.url), "utf8");
const releaseSources = workflow + helper + cli + publicConsumer;

test("manual nested and product tag triggers are isolated", () => {
  assert.deepEqual(Object.keys(parse(workflow).on), ["workflow_dispatch"]);
  assert.doesNotMatch(product, /backend\/libs\/runtime-go\/v\*/);
});

test("product RC validates Runtime v2 with the CI-tested command and staged consumer", () => {
  const steps = parse(product).jobs["test-runtime-v2"].steps;
  const runtime = steps.find(step => step.name === "Run Runtime v2 tests");
  const consumer = steps.find(step => step.name === "Test standalone Runtime v2 consumer through staged proxy");
  assert.ok(runtime);
  assert.equal(runtime.env.SQLRS_REQUIRE_NATIVE_CONTINUITY, "1");
  assert.match(runtime.run, /go test -count=1 -timeout=10m -coverpkg=\.\/\.\.\. -covermode=count -coverprofile=coverage-runtime-all\.out \.\/\.\./);
  assert.ok(consumer);
  assert.equal(consumer.if, "matrix.runner == 'ubuntu-24.04'");
  assert.match(consumer.run, /stage-runtime-module\.go/);
  assert.match(consumer.run, /go mod init example\.invalid\/runtime-v2-consumer/);
  assert.match(consumer.run, /GOPROXY="file:\/\/\$temp\/proxy".*go test -count=1 \.\/\.\./);
});

test("macOS engine coverage does not restore the Go cache", () => {
  const steps = parse(ci).jobs["coverage-engine"].steps;
  const setup = steps.find(step => step.name === "Set up Go (engine)");
  assert.ok(setup);
  assert.equal(setup.with.cache, "${{ matrix.runner != 'macos-latest' }}");
});

test("nested release uses reusable clean public consumption gates", () => {
  for (const required of ["GOWORK=off", "proxy.golang.org", "sum.golang.org", "go mod download", "retract v0.1.0", "sqlrs.runtime.v2", "sqlrs.runtime.v2.canonical.v1", "sqlrs.runtime.v2.aliases.v1", "sqlrs.runtime.v2.alias-expansion-trace.v1", "sqlrs.resolution-cache.v1", "sqlrs.runtime.conformance.bundle-schema.v1", "runtime-v2-canonical-v1.1", "v0.1.1-rc.6", "create-runtime-v2-attestation.mjs", "manifest.sha256"]) {
    assert.ok(releaseSources.includes(required), `missing ${required}`);
  }
  assert.match(historicalNotes, /v0\.3\.0/);
  assert.doesNotMatch(workflow, /go mod edit[^\n]*-replace/);
  assert.match(workflow, /stage-runtime-module\.go/);
  assert.match(releaseSources, /test\/runtime-v2-consumer\/"\*_test\.go/);
  assert.doesNotMatch(workflow, /cat > runtime_test\.go/);
  assert.match(workflow, /staged_version=v0\.0\.0-pr\.0/);
  assert.match(ci, /staged_version=v0\.0\.0-pr\.0/);
  assert.match(workflow, /go mod init example\.invalid\/runtime-v2-consumer/);
  assert.match(ci, /go mod init example\.invalid\/runtime-v2-consumer/);
  assert.ok(!consumerFiles.includes("go.mod") && !consumerFiles.includes("go.sum"), "fixture sources must not pin a release version");
  assert.match(workflow, /verify-runtime-v2-canonical-bundle\.mjs/);
  assert.match(workflow, /check-runtime-v2-bundle-policy\.mjs/);
  assert.match(ci, /check-runtime-v2-bundle-policy\.mjs "\$BASELINE_SHA"/);
  assert.doesNotMatch(workflow, /gh release upload[^\n]*--clobber/);
  assert.match(helper, /target_commitish/);
  assert.match(helper, /make_latest: "false"/);
});

test("manual validation checks out the requested commit and gates each package", () => {
  assert.match(workflow, /ref:.*inputs\.commit/);
  assert.match(workflow, /ref:\s*\$\{\{ github\.sha \}\}/);
  const supportedVersion = /^v0\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$/;
  for (const version of ["v0.0.1", "v0.1.0", "v0.12.34", "v0.2.0-rc.1", "v0.20.300-rc.42"]) {
    assert.match(version, supportedVersion);
  }
  for (const version of ["v0.01.0", "v0.1.00", "v0.1.0-rc.0", "v0.1.0-rc.01", "v1.1.0"]) {
    assert.doesNotMatch(version, supportedVersion);
  }
  assert.doesNotMatch(workflow, /RELEASE-.*\.md|verify_published/);
  assert.match(workflow, /git -C source fetch --no-tags origin main:refs\/remotes\/origin\/main/);
  assert.match(workflow, /git -C source merge-base --is-ancestor "\$INPUT_COMMIT" origin\/main/);
  assert.match(workflow, /grep -Fxv "backend\/libs\/runtime-go\/\$VERSION"/);
  assert.match(workflow, /go test[^\n]*-coverpkg=\.\/\.\.\.[^\n]*\.\/\.\.\./);
  assert.match(workflow, /coverage-runtime-all\.out/);
  assert.match(helper, /GoModSum/);
  assert.match(helper, /\.Zip/);
  assert.match(workflow, /Verify protected nested-tag policy before publication/);
  assert.match(workflow, /candidate_version:/);
  assert.doesNotMatch(workflow, /\$\{VERSION\}-rc\.1/);
  assert.match(workflow, /inputs\.operation == 'publish'/);
  assert.match(helper, /workflow_dispatch/);
  assert.match(publicConsumer, /go list -m -retracted -json/);
  assert.match(publicConsumer, /\.Retracted \| length > 0/);
  assert.doesNotMatch(workflow, /\[retracted:/);
});

test("Runtime v2 workflows use Node 24 actions and explicit Go cache inputs", () => {
  const runtimeWorkflows = [ci, workflow, fuzz, product];
  const deprecatedActions = /actions\/(?:checkout@v4|setup-go@v5|setup-node@v4|upload-artifact@v4|download-artifact@v4)/;
  for (const source of runtimeWorkflows) {
    assert.doesNotMatch(source, deprecatedActions);
    assert.doesNotMatch(source, /ubuntu-latest/);
  }

  assert.match(ci, /go-version-file: backend\/libs\/runtime-go\/go\.mod\s+cache: false/);
  assert.match(ci, /cache-dependency-path: backend\/local-engine-go\/go\.sum/);
  assert.match(ci, /cache-dependency-path: frontend\/cli-go\/go\.sum/);
  assert.match(workflow, /go-version-file: source\/backend\/libs\/runtime-go\/go\.mod\s+cache: false/);
  assert.match(fuzz, /go-version-file: backend\/libs\/runtime-go\/go\.mod\s+cache: false/);
});

// MP12: inspect actual YAML structure and reject semantic wiring mutations.
function assertReleaseWiring(document) {
  assert.deepEqual(Object.keys(document.on), ["workflow_dispatch"]);
  assert.equal(document.permissions.contents, "read");
  assert.equal(document.concurrency["cancel-in-progress"], false);
  const jobs = document.jobs;
  assert.deepEqual(jobs.publish.needs, ["validate", "verify-candidate"]);
  assert.match(jobs.publish.if, /needs\.validate\.result == 'success'/);
  assert.match(jobs.publish.if, /needs\['verify-candidate'\]\.result == 'success'/);
  assert.match(jobs.publish.if, /needs\.validate\.outputs\.is_rc == 'true'/);
  assert.match(jobs.publish.if, /inputs\.operation == 'publish'/);
  assert.equal(jobs.publish.permissions.contents, "write");
  assert.equal(jobs["verify-candidate"].permissions.contents, "read");
  assert.equal(jobs["verify-target"].permissions.contents, "read");
  assert.deepEqual(jobs["verify-target"].needs, ["validate", "verify-candidate", "publish"]);
  assert.match(jobs["verify-target"].if, /needs\.publish\.result == 'success'/);
  for (const [name, job] of Object.entries(jobs)) {
    const checkouts = job.steps.filter(s => s.uses?.startsWith("actions/checkout@"));
    assert.equal(checkouts.find(s => s.with.path === "automation")?.with.ref, "${{ github.sha }}", name);
    for (const checkout of checkouts) assert.equal(checkout.with["persist-credentials"], false);
    if (name === "publish") {
      for (const step of job.steps) {
        assert.doesNotMatch(step.run ?? "", /(?:go (?:test|run)|node source\/)/);
        assert.notEqual(step.uses, "./source/.github/actions/runtime-go-public-consumer");
      }
    }
    if (name !== "publish") assert.notEqual(job.permissions?.contents, "write");
  }
  const publishStep = jobs.publish.steps.find(s => s.run?.includes("runtime-go-release-cli.mjs publish"));
  assert.ok(publishStep);
  assert.equal(publishStep.env.CANDIDATE_DIGEST, "${{ needs['verify-candidate'].outputs.body_digest }}");
  const sourceStep = jobs.validate.steps.find(s => s.name === "Unit, coverage, race, and fuzz smoke");
  assert.equal(sourceStep["working-directory"], "source/backend/libs/runtime-go");
}

test("MP12 publication wiring has explicit trust, permissions and current-run dependencies", () => {
  const document = parse(workflow);
  assertReleaseWiring(document);
  for (const mutate of [
    d => { d.on.push = { tags:["*"] }; },
    d => { d.permissions.contents = "write"; },
    d => { d.concurrency["cancel-in-progress"] = true; },
    d => { d.jobs.publish.needs = ["validate"]; },
    d => { d.jobs.publish.if = "always()"; },
    d => { d.jobs["verify-target"].permissions.contents = "write"; },
    d => { d.jobs["verify-target"].needs = ["publish"]; },
    d => { d.jobs.publish.steps.push({ run:"node source/scripts/unsafe.mjs" }); },
    d => { d.jobs.validate.steps.find(s => s.with?.path === "automation").with.ref = "${{ inputs.commit }}"; },
    d => { d.jobs.validate.steps.find(s => s.name === "Unit, coverage, race, and fuzz smoke")["working-directory"] = "automation/backend/libs/runtime-go"; },
    d => { d.jobs.publish.steps.find(s => s.run?.includes("runtime-go-release-cli.mjs publish")).env.CANDIDATE_DIGEST = "old-report"; },
  ]) {
    const broken = structuredClone(document); mutate(broken);
    assert.throws(() => assertReleaseWiring(broken));
  }
});

test("MP12 actual job conditions reject every failed, skipped or cancelled mandatory dependency", () => {
  const jobs = parse(workflow).jobs;
  const evaluate = (expression,needs,inputs) => new Function("needs","inputs","always",`return (${expression.trim().replace(/^\$\{\{/,"").replace(/\}\}$/,"")});`)(needs,inputs,()=>true);
  for (const operation of ["validate","publish","verify"]) for (const isRC of [true,false]) {
    for (const validation of ["success","failure","cancelled","skipped"]) for (const candidate of ["success","failure","cancelled","skipped"]) for (const published of ["success","failure","cancelled","skipped"]) {
      const needs={validate:{result:validation,outputs:{is_rc:String(isRC)}},"verify-candidate":{result:candidate},publish:{result:published}};
      const qualified=validation === "success" && (isRC ? candidate === "skipped" : candidate === "success");
      assert.equal(evaluate(jobs.publish.if,needs,{operation}),qualified && operation === "publish");
      assert.equal(evaluate(jobs["verify-target"].if,needs,{operation}),qualified &&
        (operation === "publish" && published === "success" || operation === "verify" && published === "skipped"));
    }
  }
  const action = parse(publicConsumer);
  const step=action.runs.steps.find(s=>s.name === "Public proxy, checksum database and clean consumer");
  assert.equal(step.env.GOWORK,"off");
  assert.equal(step.env.GOPROXY,"https://proxy.golang.org");
  assert.equal(step.env.GOSUMDB,"sum.golang.org");
  assert.ok(step.run.indexOf("go test -count=1") < step.run.indexOf("release-cli.mjs\" download"));
});

test("engine coverage includes native managed access and enforces the project floor", () => {
  assert.match(ci, /coverage-engine-managed:/);
  assert.match(ci, /docker pull postgres:17/);
  assert.match(ci, /-tags managedintegration/);
  assert.match(ci, /engine_value="\$\{engine%\\%\}"/);
  assert.match(ci, /if \(value \+ 0 < 95\) exit 1/);
  assert.doesNotMatch(ci, /peter-evans\/(?:find-comment@v3|create-or-update-comment@v4)/);
});

// ES27-ES28: no-match fuzz commands and rounded module totals are insufficient.
test("execution-schema fuzz targets and exact coverage gates run in CI and release", () => {
  for (const source of [ci, workflow]) {
    for (const name of ["FuzzExecutionEnvironmentInput", "FuzzExecutionEnvironmentIdentity", "FuzzCanonicalValueInspect"]) {
      assert.ok(source.includes(`-fuzz '^${name}$'`), `missing fuzz invocation ${name}`);
      assert.ok(source.includes(`-list '^${name}$'`), `missing existence check ${name}`);
    }
    assert.match(source, /-fuzztime=2s -timeout=30s -parallel=2/);
    assert.match(source, /check-runtime-execution-coverage\.mjs/);
    assert.match(source, /check-runtime-execution-coverage\.test\.mjs/);
    assert.match(source, /if:\s*\$?\{?\{?\s*always\(\)/);
    assert.match(source, /coverage-runtime-all\.out/);
  }
});
