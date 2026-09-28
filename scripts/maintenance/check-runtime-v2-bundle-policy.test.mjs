import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

const checker = path.resolve(new URL("check-runtime-v2-bundle-policy.mjs", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1"));
const bundle = "backend/libs/runtime-go/conformance/canonical-v1";
const manifest = (version) => `${JSON.stringify({ bundle_version: version })}\n`;

function git(cwd, ...args) {
  return execFileSync("git", args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
}

async function repository() {
  const root = await mkdtemp(path.join(os.tmpdir(), "runtime-bundle-policy-"));
  git(root, "init", "-q");
  git(root, "config", "user.name", "test");
  git(root, "config", "user.email", "test@example.invalid");
  return root;
}

test("initial unpublished bundle is accepted", async () => {
  const root = await repository();
  try {
    await writeFile(path.join(root, "README"), "baseline\n");
    git(root, "add", "."); git(root, "commit", "-qm", "baseline");
    await mkdir(path.join(root, bundle), { recursive: true });
    await writeFile(path.join(root, bundle, "manifest.json"), manifest("v1"));
    const result = spawnSync(process.execPath, [checker, "HEAD"], { cwd: root, encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("existing bundle bytes are immutable even if its manifest version is edited", async () => {
  const root = await repository();
  try {
    await mkdir(path.join(root, bundle), { recursive: true });
    await writeFile(path.join(root, bundle, "manifest.json"), manifest("v1"));
    await writeFile(path.join(root, bundle, "vector.json"), "one\n");
    git(root, "add", "."); git(root, "commit", "-qm", "baseline");
    await writeFile(path.join(root, bundle, "manifest.json"), manifest("v2"));
    await writeFile(path.join(root, bundle, "vector.json"), "two\n");
    const result = spawnSync(process.execPath, [checker, "HEAD"], { cwd: root, encoding: "utf8" });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /immutable conformance bundle changed/);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("new directory requires a new bundle version and preserves old bytes", async () => {
  const root = await repository();
  try {
    await mkdir(path.join(root, bundle), { recursive: true });
    await writeFile(path.join(root, bundle, "manifest.json"), manifest("v1"));
    git(root, "add", "."); git(root, "commit", "-qm", "baseline");
    const next = "backend/libs/runtime-go/conformance/canonical-v2";
    await mkdir(path.join(root, next), { recursive: true });
    await writeFile(path.join(root, next, "manifest.json"), manifest("v2"));
    let result = spawnSync(process.execPath, [checker, "HEAD", next], { cwd: root, encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr);
    await writeFile(path.join(root, next, "manifest.json"), manifest("v1"));
    result = spawnSync(process.execPath, [checker, "HEAD", next], { cwd: root, encoding: "utf8" });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /bundle version reused/);
  } finally { await rm(root, { recursive: true, force: true }); }
});
