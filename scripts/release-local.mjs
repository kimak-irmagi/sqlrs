import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { ensureDir, run } from "./_lib.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const repoRoot = path.resolve(__dirname, "..");

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg.startsWith("--")) {
      throw new Error(`Unknown argument: ${arg}`);
    }
    const key = arg.slice(2);
    if (i + 1 >= argv.length) {
      throw new Error(`Missing value for --${key}`);
    }
    out[key] = argv[i + 1];
    i += 1;
  }
  return out;
}

function rmrf(p) {
  if (!fs.existsSync(p)) return;
  fs.rmSync(p, { recursive: true, force: true });
}

function sha256File(pathname) {
  const data = fs.readFileSync(pathname);
  return crypto.createHash("sha256").update(data).digest("hex");
}

function isWindowsTarget(goos) {
  return goos === "windows";
}

function validateExecutable(pathname, goos, goarch, label) {
  const data = fs.readFileSync(pathname);
  let format = "unknown";
  let arch = "unknown";
  if (data.length >= 20 && data.subarray(0, 4).equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46]))) {
    format = "linux";
    const machine = data.readUInt16LE(18);
    arch = machine === 62 ? "amd64" : machine === 183 ? "arm64" : "unknown";
  } else if (data.length >= 64 && data.subarray(0, 2).toString("ascii") === "MZ") {
    const peOffset = data.readUInt32LE(0x3c);
    if (data.length >= peOffset + 6 && data.subarray(peOffset, peOffset + 4).equals(Buffer.from("PE\0\0"))) {
      format = "windows";
      const machine = data.readUInt16LE(peOffset + 4);
      arch = machine === 0x8664 ? "amd64" : machine === 0xaa64 ? "arm64" : "unknown";
    }
  } else if (data.length >= 8) {
    const magic = data.readUInt32LE(0);
    if (magic === 0xfeedface || magic === 0xfeedfacf) {
      format = "darwin";
      const cpu = data.readUInt32LE(4);
      arch = cpu === 0x01000007 ? "amd64" : cpu === 0x0100000c ? "arm64" : "unknown";
    }
  }
  if (format !== goos || arch !== goarch) {
    throw new Error(`${label} must be ${goos}/${goarch}, detected ${format}/${arch}: ${pathname}`);
  }
}

function copyDir(src, dest) {
  if (!fs.existsSync(src)) return;
  fs.cpSync(src, dest, { recursive: true });
}

const args = parseArgs(process.argv.slice(2));
const version = args.version || process.env.SQLRS_VERSION;
const buildVersion = args["build-version"] || process.env.SQLRS_BUILD_VERSION || version;
const goos = args.os || process.env.GOOS;
const goarch = args.arch || process.env.GOARCH;
const workspace = args.workspace ? path.resolve(args.workspace) : repoRoot;
const engineBin = args["engine-bin"] || process.env.SQLRS_ENGINE_BIN;
const wslEngineBin = args["wsl-engine-bin"] || process.env.SQLRS_WSL_ENGINE_BIN;

if (!version) {
  throw new Error("Missing version. Provide --version or SQLRS_VERSION.");
}
if (!goos || !goarch) {
  throw new Error("Missing target platform. Provide --os and --arch (or GOOS/GOARCH).");
}
if (!engineBin) {
  throw new Error("Missing engine binary. Provide --engine-bin or SQLRS_ENGINE_BIN.");
}
if (isWindowsTarget(goos) && !wslEngineBin) {
  throw new Error("Missing WSL engine binary. Provide --wsl-engine-bin or SQLRS_WSL_ENGINE_BIN.");
}
if (!isWindowsTarget(goos) && wslEngineBin) {
  throw new Error("--wsl-engine-bin is valid only for Windows release bundles.");
}

const exeSuffix = isWindowsTarget(goos) ? ".exe" : "";

const cliRoot = path.join(repoRoot, "frontend", "cli-go");
const distBin = path.resolve(workspace, "dist", "bin", `${goos}_${goarch}`);
const distRelease = path.resolve(workspace, "dist", "release");
const stagingRoot = path.join(distRelease, "staging");
const stagingDir = path.join(stagingRoot, `sqlrs_${version}_${goos}_${goarch}`);

ensureDir(distBin);
ensureDir(distRelease);
ensureDir(stagingRoot);
rmrf(stagingDir);
ensureDir(stagingDir);

const cliPath = path.join(distBin, `sqlrs${exeSuffix}`);
const engineOut = path.join(distBin, `sqlrs-engine${exeSuffix}`);
const enginePath = path.resolve(engineBin);
const wslEnginePath = wslEngineBin ? path.resolve(wslEngineBin) : "";

await run({
  cmd: [
    "go",
    "build",
    "-ldflags",
    `-X github.com/sqlrs/cli/internal/app.Version=${buildVersion}`,
    "-o",
    cliPath,
    "./cmd/sqlrs"
  ],
  cwd: cliRoot,
  env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: "0" }
});

if (!fs.existsSync(enginePath)) {
  throw new Error(`Engine binary not found: ${enginePath}`);
}
validateExecutable(enginePath, goos, goarch, "Engine binary");
fs.copyFileSync(enginePath, engineOut);

if (isWindowsTarget(goos)) {
  if (!fs.existsSync(wslEnginePath)) {
    throw new Error(`WSL engine binary not found: ${wslEnginePath}`);
  }
  validateExecutable(wslEnginePath, "linux", goarch, "WSL engine binary");
}

fs.copyFileSync(cliPath, path.join(stagingDir, `sqlrs${exeSuffix}`));
fs.copyFileSync(engineOut, path.join(stagingDir, `sqlrs-engine${exeSuffix}`));
if (isWindowsTarget(goos)) {
  const wslEngineOut = path.join(stagingDir, "libexec", `linux-${goarch}`, "sqlrs-engine");
  ensureDir(path.dirname(wslEngineOut));
  fs.copyFileSync(wslEnginePath, wslEngineOut);
}
fs.copyFileSync(path.join(repoRoot, "LICENSE"), path.join(stagingDir, "LICENSE"));
fs.copyFileSync(path.join(repoRoot, "README.md"), path.join(stagingDir, "README.md"));

const docsRoot = path.join(stagingDir, "docs");
ensureDir(docsRoot);
copyDir(path.join(repoRoot, "docs", "user-guides"), path.join(docsRoot, "user-guides"));
copyDir(path.join(repoRoot, "docs", "api-guides"), path.join(docsRoot, "api-guides"));

let archivePath = "";
if (isWindowsTarget(goos)) {
  if (process.platform !== "win32") {
    throw new Error("Windows target requires running on Windows for zip packaging.");
  }
  archivePath = path.join(distRelease, `sqlrs_${version}_${goos}_${goarch}.zip`);
  await run({
    cmd: [
      "powershell",
      "-NoProfile",
      "-Command",
      `Compress-Archive -Path "${stagingDir}\\*" -DestinationPath "${archivePath}" -Force`
    ],
    cwd: workspace
  });
} else {
  archivePath = path.join(distRelease, `sqlrs_${version}_${goos}_${goarch}.tar.gz`);
  await run({
    cmd: ["tar", "-czf", archivePath, "-C", stagingRoot, path.basename(stagingDir)],
    cwd: workspace
  });
}

const checksum = sha256File(archivePath);
const checksumPath = `${archivePath}.sha256`;
fs.writeFileSync(checksumPath, `${checksum}  ${path.basename(archivePath)}\n`, "utf8");

console.log(`Built sqlrs local release: ${archivePath}`);
