import path from "node:path";
import { fileURLToPath } from "node:url";

import { ensureDir, isWindows, run } from "./_lib.mjs";
import { installJdbcDriver } from "./jdbc-driver-bundle.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const repoRoot = path.resolve(__dirname, "..");
const workspace = process.argv[2] ? path.resolve(process.argv[2]) : repoRoot;
const engineRoot = path.join(repoRoot, "backend", "local-engine-go");

const distBin = path.resolve(workspace, "dist", "bin");
ensureDir(distBin);

const binaryName = isWindows() ? "sqlrs-engine.exe" : "sqlrs-engine";
const output = path.join(distBin, binaryName);

await run({
  cmd: ["node", "scripts/fetch-sql-scripts.mjs", "--lock", "--source-prefix", "jdbc-postgresql-"],
  cwd: repoRoot
});

await run({ cmd: ["go", "build", "-o", output, "./cmd/sqlrs-engine"], cwd: engineRoot });
console.log(`Built sqlrs-engine into: ${output}`);
console.log(`Bundled JDBC driver: ${installJdbcDriver(repoRoot, distBin)}`);

if (isWindows()) {
  const linuxBinary = "sqlrs-engine-linux-amd64";
  const linuxOutput = path.join(distBin, linuxBinary);
  const env = {
    ...process.env,
    GOOS: "linux",
    GOARCH: "amd64",
    CGO_ENABLED: "0"
  };
  await run({ cmd: ["go", "build", "-o", linuxOutput, "./cmd/sqlrs-engine"], cwd: engineRoot, env });
  console.log(`Built sqlrs-engine (linux/amd64) into: ${linuxOutput}`);
}
