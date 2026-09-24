import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";

const workflow = await readFile(new URL("../../.github/workflows/release-runtime-go.yml", import.meta.url), "utf8");
const product = await readFile(new URL("../../.github/workflows/release-local.yml", import.meta.url), "utf8");

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
