import { readFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const moduleDirectory = "backend/libs/runtime-go/";
const requiredFiles = ["canonical_value_inspect_v1.go", "schemas/sqlrs/execution.go", "schemas/sqlrs/execution_validation.go", "schemas/sqlrs/execution_value.go"];
const normalize = value => value.replaceAll("\\", "/");
const measured = (covered = 0, total = 0) => ({ covered, total });
const add = (left, right) => {
  const result = measured(left.covered + right.covered, left.total + right.total);
  if (!Number.isSafeInteger(result.total * 100)) throw new Error("coverage statement count overflow");
  return result;
};
const passes = value => value.total > 0 && value.covered * 100 >= value.total * 95;

/** ES28: parse statement-weighted Go profiles without rounding or double counting.
 * Unknown/conflicting data throws; valid but insufficient/missing coverage fails.
 * Requirements: docs/architecture/runtime-v2-execution-schema-tests.md.
 */
export function evaluateExecutionCoverage(profile, newFiles) {
  if (!Array.isArray(newFiles) || newFiles.length === 0) throw new Error("new implementation file list is empty");
  const lines = profile.split(/\r?\n/);
  if (!/^mode: (count|set|atomic)$/.test(lines.shift())) throw new Error("invalid coverage mode");
  const blocks = new Map();
  for (const line of lines) {
    if (line === "") continue;
    const match = /^(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)$/.exec(line);
    if (!match) throw new Error("invalid coverage block");
    const [, rawFile, sl, sc, el, ec, statements, count] = match;
    const coordinates = [sl, sc, el, ec].map(Number);
    const size = Number(statements);
    if (coordinates.some(value => !Number.isSafeInteger(value) || value < 1) ||
        +el < +sl || +el === +sl && +ec < +sc || !Number.isSafeInteger(size)) throw new Error("invalid coverage metrics");
    const file = normalize(rawFile);
    const key = `${file}:${sl}.${sc},${el}.${ec}`;
    const previous = blocks.get(key);
    if (previous && previous.statements !== size) throw new Error("conflicting coverage block");
    blocks.set(key, { file, location: `${sl}.${sc},${el}.${ec}`, statements: size, covered: Boolean(previous?.covered || BigInt(count) > 0n) });
  }
  const totals = new Map();
  let module = measured();
  const uncovered = [];
  for (const block of blocks.values()) {
    const value = measured(block.covered ? block.statements : 0, block.statements);
    module = add(module, value);
    totals.set(block.file, add(totals.get(block.file) ?? measured(), value));
    if (!block.covered && block.statements > 0) uncovered.push({ file: block.file, location: block.location, statements: block.statements });
  }
  const failures = [];
  if (!passes(module)) failures.push("module coverage is absent or below 95%");
  let newCode = measured();
  const files = [];
  for (const name of [...new Set(newFiles.map(normalize))].sort()) {
    const matches = [...totals].filter(([file]) => file === name || file.endsWith(`/${name}`));
    if (matches.length > 1) throw new Error("ambiguous implementation file in coverage");
    const value = matches[0]?.[1] ?? measured();
    files.push({ file: name, ...value });
    newCode = add(newCode, value);
    if (!passes(value)) failures.push(`${name}: missing, unmeasurable or below 95%`);
  }
  if (!passes(newCode)) failures.push("new-code coverage is absent or below 95%");
  return { ok: failures.length === 0, module, newCode, files, failures, uncovered };
}

/** Include all added production Go files, including untracked local work. A CI
 * caller supplies the review base; release checks retain the mandatory facade
 * list even if that exact release commit contains only metadata changes.
 */
function discoverNewFiles(base) {
  const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
  const git = args => execFileSync("git", args, { cwd: repositoryRoot, encoding: "utf8" }).trim().split(/\r?\n/).filter(Boolean);
  const changed = base ? git(["diff", "--name-only", "--diff-filter=A", base, "--", moduleDirectory]) : [];
  const untracked = git(["ls-files", "--others", "--exclude-standard", "--", moduleDirectory]);
  return [...new Set([...requiredFiles, ...changed, ...untracked].map(normalize).filter(name => name.endsWith(".go") && !name.endsWith("_test.go")).map(name => name.startsWith(moduleDirectory) ? name.slice(moduleDirectory.length) : name))];
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const profile = await readFile(process.argv[2] ?? `${moduleDirectory}coverage-runtime-all.out`, "utf8");
    const base = process.env.BASELINE_SHA || undefined;
    const report = evaluateExecutionCoverage(profile, discoverNewFiles(base));
    console.log(JSON.stringify(report, null, 2));
    if (!report.ok) process.exitCode = 1;
  } catch (error) {
    console.error(`execution coverage check failed: ${error.message}`);
    process.exitCode = 1;
  }
}
