import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import YAML from "yaml";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const workflowPath = path.join(repoRoot, ".github", "workflows", "release-local.yml");

function run(name, fn) {
  try {
    fn();
    process.stdout.write(`ok - ${name}\n`);
  } catch (err) {
    process.stderr.write(`not ok - ${name}\n`);
    throw err;
  }
}

function loadWorkflow() {
  const raw = fs.readFileSync(workflowPath, "utf8");
  return YAML.parse(raw);
}

run("happy e2e matrix includes platform and snapshot backend axes", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  assert.ok(job, "missing e2e-happy job");
  assert.deepEqual(job.strategy?.matrix?.platform, ["linux", "windows"]);
  assert.deepEqual(job.strategy?.matrix?.snapshot_backend, ["copy", "btrfs"]);
});

run("happy e2e keeps the Windows copy backend covered", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const excluded = job.strategy?.matrix?.exclude || [];
  assert.ok(
    excluded.some((entry) => entry.platform === "windows" && entry.scenario === "hp-psql-sakila"),
    "missing windows/sakila exclusion"
  );
  assert.ok(!excluded.some((entry) => entry.platform === "windows" && entry.snapshot_backend === "copy"));
});

run("linux e2e cell passes snapshot backend to run-scenario", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const runStep = (job.steps || []).find((step) => step.name === "Run happy-path scenario (linux)");
  assert.ok(runStep, "missing run step");
  assert.match(String(runStep.run || ""), /--snapshot-backend "\$\{\{ matrix\.snapshot_backend \}\}"/);
  assert.match(String(runStep.run || ""), /--flow-runs "2"/);
});

run("linux e2e cells fetch only their scenario datasets", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const fetchStep = (job.steps || []).find((step) => step.name === "Fetch example SQL datasets (locked)");
  assert.ok(fetchStep, "missing SQL dataset fetch step");
  const script = String(fetchStep.run || "");
  assert.match(script, /hp-psql-chinook\|cache-pressure-chinook[\s\S]*--source-prefix chinook-postgres/);
  assert.match(script, /hp-psql-sakila[\s\S]*--source-prefix sakila-postgres-/);
  assert.match(script, /hp-lb-jhipster[\s\S]*--source-prefix liquibase-jhipster-/);
  assert.doesNotMatch(script, /pnpm fetch:sql --lock\s*$/m);
});

run("e2e diagnostics artifacts are backend and platform specific", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const uploadStep = (job.steps || []).find((step) => step.name === "Upload Linux E2E diagnostics");
  assert.ok(uploadStep, "missing upload step");
  assert.match(String(uploadStep.with?.name || ""), /\$\{\{ matrix\.platform \}\}/);
  assert.match(String(uploadStep.with?.name || ""), /\$\{\{ matrix\.snapshot_backend \}\}/);
});

run("linux btrfs profile installs required tooling", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const step = (job.steps || []).find((item) => item.name === "Install btrfs tooling");
  assert.ok(step, "missing btrfs tooling step");
  assert.equal(String(step.if || "").trim(), "matrix.platform == 'linux' && matrix.snapshot_backend == 'btrfs'");
  assert.match(String(step.run || ""), /apt-get install -y btrfs-progs/);
});

run("windows e2e cell provisions WSL and docker prerequisites", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const wslStep = (job.steps || []).find((item) => item.name === "Set up WSL");
  const dockerStep = (job.steps || []).find((item) => item.name === "Set up Docker on host");
  const runStep = (job.steps || []).find((item) => item.name === "Run happy-path scenario (windows host)");
  assert.ok(wslStep, "missing WSL setup step");
  assert.ok(dockerStep, "missing docker setup step");
  assert.ok(runStep, "missing windows run step");
  assert.equal(wslStep.uses, "Vampire/setup-wsl@v7");
  assert.equal(String(wslStep.if || "").trim(), "matrix.platform == 'windows'");
  assert.equal(dockerStep.uses, "docker/setup-docker-action@v5");
  assert.match(String(runStep.run || ""), /sqlrs_bin/);
  assert.doesNotMatch(String(runStep.run || ""), /engine_windows_bin|engine_linux_bin/);
  assert.doesNotMatch(String(runStep.run || ""), /--engine|--wsl-engine/);
  assert.match(String(runStep.run || ""), /\$isBtrfs/);
  assert.match(String(runStep.run || ""), /DOCKER_HOST/);
  assert.match(String(runStep.run || ""), /SQLRS_DOCKER_HOST_PATH_STYLE = "linux"/);
  assert.match(String(runStep.run || ""), /hostname -I/);
  assert.match(String(runStep.run || ""), /\\\\wsl\.localhost/);
  assert.match(String(runStep.run || ""), /\/var\/tmp\/sqlrs-release-e2e/);
  assert.match(String(runStep.run || ""), /SQLRS_STATE_DB = Join-Path \$outDir "state\.db"/);
  assert.match(String(runStep.run || ""), /"--store", "dir", \$storeRoot/);
  assert.match(String(runStep.run || ""), /"--store", "image", \$storeImage/);
  assert.match(String(runStep.run || ""), /chinook\.prep\.s9s\.yaml/);
  assert.match(String(runStep.run || ""), /"prepare",\s*"chinook"/);
  assert.match(String(runStep.run || ""), /raw-stdout-run2\.log/);
  assert.match(String(runStep.run || ""), /second pass failed/);
});

run("release workflow uses Node 24-backed action majors", () => {
  const workflow = loadWorkflow();
  const uses = Object.values(workflow.jobs || {}).flatMap((job) =>
    (job.steps || []).map((step) => step.uses).filter(Boolean)
  );
  const expectedMajors = new Map([
    ["actions/checkout", "v7"],
    ["actions/setup-go", "v7"],
    ["actions/setup-node", "v7"],
    ["actions/upload-artifact", "v7"],
    ["actions/download-artifact", "v8"],
    ["docker/setup-docker-action", "v5"],
    ["Vampire/setup-wsl", "v7"],
    ["liquibase/setup-liquibase", "v3"],
    ["softprops/action-gh-release", "v3"],
  ]);

  for (const [action, major] of expectedMajors) {
    const references = uses.filter((value) => value.startsWith(`${action}@`));
    assert.ok(references.length > 0, `missing ${action}`);
    assert.ok(
      references.every((value) => value === `${action}@${major}`),
      `${action} must consistently use ${major}: ${references.join(", ")}`
    );
  }
});

run("windows release archive is self-contained for native and WSL runtimes", () => {
  const workflow = loadWorkflow();
  const build = workflow.jobs?.["build-rc"];
  const windowsBuild = (build.steps || []).find((step) => step.name === "Build engine (windows)");
  const windowsRelease = (build.steps || []).find((step) => step.name === "Build local release (windows)");
  assert.match(String(windowsBuild?.run || ""), /GOOS = "linux"/);
  assert.match(String(windowsRelease?.run || ""), /--wsl-engine-bin/);

  const e2e = workflow.jobs?.["e2e-happy"];
  assert.equal(
    (e2e.steps || []).find((step) => step.name === "Download linux rc artifact for WSL engine"),
    undefined
  );
  const extract = (e2e.steps || []).find((step) => step.name === "Extract windows bundle");
  assert.match(String(extract?.run || ""), /libexec/);
  assert.match(String(extract?.run || ""), /linux-amd64/);
});

run("publish RC waits for unified e2e-happy job", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["publish-rc"];
  assert.ok(job, "missing publish-rc job");
  assert.ok(Array.isArray(job.needs), "publish-rc needs must be an array");
  assert.ok(job.needs.includes("e2e-happy"), "publish-rc must depend on e2e-happy");
  assert.ok(job.needs.includes("e2e-podman-macos"), "publish-rc must depend on e2e-podman-macos");
});

run("smoke matrix keeps darwin-only cells after windows happy-path integration", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-smoke"];
  assert.ok(job, "missing e2e-smoke job");
  const include = job.strategy?.matrix?.include || [];
  assert.equal(include.length, 1);
  assert.equal(include[0]?.os_family, "darwin");
});

run("release workflow includes macos podman probe with double flow run", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-podman-macos"];
  assert.ok(job, "missing e2e-podman-macos job");
  assert.equal(job["runs-on"], "macos-15-intel");
  const fetchStep = (job.steps || []).find((step) => step.name === "Fetch Chinook SQL asset (locked)");
  const installStep = (job.steps || []).find((step) => step.name === "Install Podman");
  const startStep = (job.steps || []).find((step) => step.name === "Start Podman machine");
  const skipStep = (job.steps || []).find((step) => step.name === "Skip note (Podman unavailable)");
  const runStep = (job.steps || []).find((step) => step.name === "Run macOS release e2e with podman runtime");
  assert.ok(fetchStep, "missing chinook asset fetch step for macos podman probe");
  assert.match(String(fetchStep.run || ""), /Chinook_PostgreSql\.sql/);
  assert.match(String(fetchStep.run || ""), /e3fde5c1a5b51a2a91429a702c9ca6e69ba56e6c7f5e112724d70c3d03db695e/);
  assert.ok(installStep, "missing podman install step");
  assert.match(String(installStep.run || ""), /podman-installer-macos-amd64\.pkg/);
  assert.match(String(installStep.run || ""), /v5\.8\.5/);
  assert.match(String(installStep.run || ""), /2677be9fa3bf75f7dbd4dfe5f5039cf105806f102af15ee6c6d174c70dcda3b8/);
  assert.match(String(installStep.run || ""), /shasum -a 256 -c/);
  assert.match(String(installStep.run || ""), /sudo installer -pkg/);
  assert.match(String(installStep.run || ""), /\/etc\/paths\.d\/podman-pkg/);
  assert.match(String(installStep.run || ""), /GITHUB_PATH/);
  assert.match(String(installStep.run || ""), /export PATH=/);
  assert.doesNotMatch(String(installStep.run || ""), /brew install podman/);
  assert.ok(startStep, "missing podman machine startup step");
  assert.equal(skipStep, undefined, "skip step must be removed: podman probe is release-blocking");
  assert.match(String(startStep.run || ""), /exit 1/);
  assert.ok(runStep, "missing podman release e2e run step");
  assert.match(String(runStep.run || ""), /--container-runtime "podman"/);
  assert.match(String(runStep.run || ""), /--flow-runs "2"/);
  assert.match(String(runStep.run || ""), /hp-psql-chinook/);
});

run("happy e2e matrix includes linux-only cache-pressure cell", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const scenarios = job.strategy?.matrix?.scenario || [];
  const excluded = job.strategy?.matrix?.exclude || [];
  assert.ok(scenarios.includes("cache-pressure-chinook"), "missing cache-pressure scenario in matrix");
  assert.ok(
    excluded.some((entry) => entry.platform === "windows" && entry.scenario === "cache-pressure-chinook"),
    "missing windows/cache-pressure exclusion"
  );
  assert.ok(
    excluded.some(
      (entry) =>
        entry.platform === "linux" &&
        entry.scenario === "cache-pressure-chinook" &&
        entry.snapshot_backend === "btrfs"
    ),
    "missing linux btrfs/cache-pressure exclusion"
  );
});

run("linux e2e step routes cache-pressure through dedicated runner", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const runStep = (job.steps || []).find((step) => step.name === "Run happy-path scenario (linux)");
  assert.ok(runStep, "missing run step");
  assert.match(String(runStep.run || ""), /cache-pressure-chinook\) scenario_timeout="30m"/);
  assert.match(String(runStep.run || ""), /run-cache-pressure-scenario\.mjs/);
  assert.match(String(runStep.run || ""), /cache-pressure-chinook\)/);
});

run("linux e2e diagnostics upload includes cache-pressure artifacts", () => {
  const workflow = loadWorkflow();
  const job = workflow.jobs?.["e2e-happy"];
  const uploadStep = (job.steps || []).find((step) => step.name === "Upload Linux E2E diagnostics");
  assert.ok(uploadStep, "missing upload step");
  const pathSpec = String(uploadStep.with?.path || "");
  assert.match(pathSpec, /command-config\*\.txt/);
  assert.match(pathSpec, /status\*\.json/);
  assert.match(pathSpec, /cache-pressure-summary\.json/);
});
