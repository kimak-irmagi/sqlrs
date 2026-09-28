import { execFileSync, spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import path from "node:path";

const baseline = process.argv[2];
const currentRoot = (process.argv[3] ?? "backend/libs/runtime-go/conformance/canonical-v1").replaceAll("\\", "/").replace(/\/$/, "");
const conformanceRoot = "backend/libs/runtime-go/conformance";
if (!baseline || !/^[A-Za-z0-9._/-]+$/.test(baseline) || !currentRoot.startsWith(`${conformanceRoot}/`)) {
  throw new Error("usage: check-runtime-v2-bundle-policy BASELINE_REF [CURRENT_BUNDLE_ROOT]");
}

function git(...args) {
  return execFileSync("git", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
}

git("rev-parse", "--verify", `${baseline}^{commit}`);
const listed = git("ls-tree", "-r", "--name-only", baseline, "--", conformanceRoot).trim().split(/\r?\n/).filter(Boolean);
const baselineManifests = listed.filter((name) => name.endsWith("/manifest.json"));
const currentManifest = JSON.parse(await readFile(path.join(currentRoot, "manifest.json"), "utf8"));
const baselineVersions = new Set();

for (const manifestPath of baselineManifests) {
  const bundleRoot = path.posix.dirname(manifestPath);
  const manifest = JSON.parse(git("show", `${baseline}:${manifestPath}`));
  baselineVersions.add(manifest.bundle_version);
  const diff = spawnSync("git", ["diff", "--quiet", baseline, "--", bundleRoot], { stdio: "ignore" });
  if (diff.status === 1) throw new Error(`immutable conformance bundle changed: ${bundleRoot}`);
  if (diff.status !== 0) throw new Error(`cannot compare conformance bundle: ${bundleRoot}`);
}

if (!baselineManifests.some((name) => path.posix.dirname(name) === currentRoot) && baselineVersions.has(currentManifest.bundle_version)) {
  throw new Error(`bundle version reused in a new directory: ${currentManifest.bundle_version}`);
}

process.stdout.write(`${currentManifest.bundle_version}\n`);
