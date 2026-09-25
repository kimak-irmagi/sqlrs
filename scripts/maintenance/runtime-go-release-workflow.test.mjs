import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";

const workflow = await readFile(new URL("../../.github/workflows/release-runtime-go.yml", import.meta.url), "utf8");
const product = await readFile(new URL("../../.github/workflows/release-local.yml", import.meta.url), "utf8");
const ci = await readFile(new URL("../../.github/workflows/ci.yml", import.meta.url), "utf8");
const fuzz = await readFile(new URL("../../.github/workflows/runtime-v2-fuzz.yml", import.meta.url), "utf8");

test("nested and product tag triggers are isolated", () => {
  assert.match(workflow, /backend\/libs\/runtime-go\/v\*/);
  assert.doesNotMatch(workflow, /tags:\s*\n\s*- ["']?v\*/);
  assert.doesNotMatch(product, /backend\/libs\/runtime-go\/v\*/);
});

test("nested release uses clean public consumption gates", () => {
  for (const required of ["GOWORK=off", "proxy.golang.org", "sum.golang.org", "go mod download", "retract v0.1.0", "sqlrs.runtime.v2"]) {
    assert.ok(workflow.includes(required), `missing ${required}`);
  }
  assert.doesNotMatch(workflow, /\breplace\b/);
});

test("manual validation checks out the requested commit and gates each package", () => {
  assert.match(workflow, /ref:.*inputs\.commit/);
  assert.match(workflow, /coverage-core\.out/);
  assert.match(workflow, /coverage-resolver\.out/);
  assert.match(workflow, /GoModSum/);
  assert.match(workflow, /\.Zip \| length > 0/);
  assert.match(workflow, /Verify protected nested-tag policy before publication/);
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
  assert.match(workflow, /go-version-file: backend\/libs\/runtime-go\/go\.mod\s+cache: false/);
  assert.match(fuzz, /go-version-file: backend\/libs\/runtime-go\/go\.mod\s+cache: false/);
});

test("engine coverage includes native managed access and enforces the project floor", () => {
  assert.match(ci, /coverage-engine-managed:/);
  assert.match(ci, /docker pull postgres:17/);
  assert.match(ci, /-tags managedintegration/);
  assert.match(ci, /engine_value="\$\{engine%\\%\}"/);
  assert.match(ci, /if \(value \+ 0 < 95\) exit 1/);
});
