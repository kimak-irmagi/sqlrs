import assert from "node:assert/strict";
import test from "node:test";
import { evaluateExecutionCoverage } from "./check-runtime-execution-coverage.mjs";

// ES28: exact statement-weighted gates, independently of displayed rounding.
const modulePath = "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/";
const added = "schemas/sqlrs/execution.go";
const block = (file, line, statements, count) => `${modulePath}${file}:${line}.1,${line}.20 ${statements} ${count}`;
const profile = (...lines) => `mode: count\n${lines.join("\n")}\n`;

test("execution coverage accepts exactly 95%, not a rounded approximation", () => {
  const passing = evaluateExecutionCoverage(profile(block(added, 1, 19, 1), block(added, 2, 1, 0)), [added]);
  assert.equal(passing.ok, true);
  assert.deepEqual(passing.module, { covered: 19, total: 20 });
  assert.deepEqual(passing.newCode, { covered: 19, total: 20 });
  const rounded = evaluateExecutionCoverage(profile(block(added, 1, 9496, 1), block(added, 2, 504, 0)), [added]);
  assert.equal((100 * rounded.module.covered / rounded.module.total).toFixed(1), "95.0");
  assert.equal(rounded.ok, false);
});

test("old code and other new files cannot mask a failing new file", () => {
  const report = evaluateExecutionCoverage(profile(
    block(added, 1, 18, 1), block(added, 2, 2, 0),
    block("canonical_value_inspect_v1.go", 1, 1000, 1), block("canonical.go", 1, 1000, 1),
  ), [added, "canonical_value_inspect_v1.go"]);
  assert.ok(report.module.covered / report.module.total >= 0.95);
  assert.ok(report.newCode.covered / report.newCode.total >= 0.95);
  assert.equal(report.ok, false);
  assert.ok(report.failures.some(value => value.includes(added)));
});

test("missing and zero-statement new files cannot silently pass", () => {
  for (const text of ["mode: count\n", profile(block("canonical.go", 1, 100, 1)), profile(block(added, 1, 0, 0))]) {
    assert.equal(evaluateExecutionCoverage(text, [added]).ok, false);
  }
});

test("block counts are statement-weighted, deduplicated and merged", () => {
  const report = evaluateExecutionCoverage(profile(block(added, 1, 19, 0), block(added, 1, 19, 1), block(added, 2, 1, 0)), [added]);
  assert.deepEqual(report.module, { covered: 19, total: 20 });
  assert.equal(report.ok, true);
});

test("coverage accepts Windows paths and CRLF without losing file identity", () => {
  const text = "mode: count\r\nC:\\checkout\\backend\\libs\\runtime-go\\schemas\\sqlrs\\execution.go:1.1,1.20 20 1\r\n";
  const report = evaluateExecutionCoverage(text, [added]);
  assert.equal(report.ok, true);
  assert.deepEqual(report.newCode, { covered: 20, total: 20 });
});

test("malformed, conflicting and unmeasurable data cannot become green", () => {
  for (const text of [
    "", "mode: unknown\n", "mode: count\nnot a block\n",
    profile(block(added, 1, -1, 1)), profile(block(added, 1, 20, -1)),
    profile(block(added, 1, 19, 1), block(added, 1, 20, 1)),
  ]) {
    assert.throws(() => evaluateExecutionCoverage(text, [added]));
  }
  assert.throws(() => evaluateExecutionCoverage(profile(block(added, 1, 20, 1)), []));
});
