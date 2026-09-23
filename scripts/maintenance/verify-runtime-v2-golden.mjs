import { createHash } from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import { resolve } from "node:path";
import { isDeepStrictEqual } from "node:util";

const target = resolve(process.argv[2] ?? "backend/libs/runtime-go/testdata/golden");
const files = (await readdir(target)).filter((name) => name.endsWith(".json")).sort();
let failed = false;

for (const file of files) {
  const source = await readFile(resolve(target, file), "utf8");
  assertNoDuplicateMembers(source);
  const fixture = JSON.parse(source);
  rejectUnknown(fixture, ["case", "comparison_group", "input", "expected"], "$fixture");
  const actual = evaluate(fixture.input);
  if (!isDeepStrictEqual(actual, fixture.expected)) {
    failed = true;
    process.stderr.write(`${file}: expected values differ\n${JSON.stringify(actual, null, 2)}\n`);
  }
}

if (failed) process.exitCode = 1;

function evaluate(input) {
  rejectUnknown(input, ["type", "value"], "input");
  if (input.type === "recipe") return evaluateRecipe(input.value);
  if (input.type === "relative") return evaluateRelative(input.value);
  throw new Error("input.type must be recipe or relative");
}

function evaluateRecipe(recipe) {
  rejectUnknown(recipe, ["schema_version", "factory", "transforms"], "input.value");
  assertVersion(recipe.schema_version);
  const factoryBytes = identityRecord("sqlrs.runtime.v2/factory-state", recipe.factory.identity);
  const rootID = digest(factoryBytes);
  const root = factoryState(rootID);
  return evaluateSteps(rootID, recipe.transforms, {
    factory: factoryBytes.toString("hex"),
    initialStates: [root],
  });
}

function evaluateRelative(value) {
  rejectUnknown(value, ["schema_version", "anchor", "transforms"], "input.value");
  assertVersion(value.schema_version);
  parseDigest(value.anchor);
  return evaluateSteps(value.anchor, value.transforms, { initialStates: [] });
}

function evaluateSteps(anchor, transforms, options) {
  let parent = anchor;
  const transformBytes = [];
  const stateBytes = [];
  const fingerprints = [];
  const states = [...options.initialStates];
  for (const provenance of transforms) {
    rejectUnknown(provenance, ["identity", "declaration", "resolver"], "provenance");
    const bytes = identityRecord("sqlrs.runtime.v2/transform", provenance.identity);
    const fingerprint = digest(bytes);
    const derivedBytes = record("sqlrs.runtime.v2/state", [
      [1, parseDigest(parent)],
      [2, parseDigest(fingerprint)],
    ]);
    const id = digest(derivedBytes);
    transformBytes.push(bytes.toString("hex"));
    stateBytes.push(derivedBytes.toString("hex"));
    fingerprints.push(fingerprint);
    states.push(derivedState(id, parent, fingerprint));
    parent = id;
  }
  const canonical = { transforms: transformBytes, derived_states: stateBytes };
  if (options.factory !== undefined) canonical.factory = options.factory;
  return { canonical, transform_fingerprints: fingerprints, states, endpoint: parent };
}

function identityRecord(domain, identity) {
  rejectUnknown(identity, ["schema_version", "provider", "kind", "identity_schema", "fields"], "identity");
  assertVersion(identity.schema_version);
  const fields = [...identity.fields].sort((a, b) => Buffer.from(a.name).compare(Buffer.from(b.name)));
  const fieldSet = Buffer.concat([
    u32(fields.length),
    ...fields.flatMap((field) => [string(field.name), string(field.value)]),
  ]);
  return record(domain, [
    [1, Buffer.from(identity.schema_version)],
    [2, Buffer.from(identity.provider)],
    [3, Buffer.from(identity.kind)],
    [4, Buffer.from(identity.identity_schema)],
    [5, fieldSet],
  ]);
}

function record(domain, fields) {
  const ordered = [...fields].sort((a, b) => a[0] - b[0]);
  return Buffer.concat([
    string(domain),
    u32(ordered.length),
    ...ordered.flatMap(([tag, payload]) => [u16(tag), bytes(payload)]),
  ]);
}

function factoryState(id) {
  return { schema_version: "sqlrs.runtime.v2", state_kind: "factory", id, factory_fingerprint: id };
}

function derivedState(id, parent, fingerprint) {
  return { schema_version: "sqlrs.runtime.v2", state_kind: "derived", id, parent_id: parent, transform_fingerprint: fingerprint };
}

function digest(value) {
  return `sha256:${createHash("sha256").update(value).digest("hex")}`;
}

function parseDigest(value) {
  if (!/^sha256:[0-9a-f]{64}$/.test(value)) throw new Error("invalid digest");
  return Buffer.from(value.slice(7), "hex");
}

function string(value) { return bytes(Buffer.from(value, "utf8")); }
function bytes(value) { return Buffer.concat([u64(value.length), value]); }
function u16(value) { const result = Buffer.alloc(2); result.writeUInt16BE(value); return result; }
function u32(value) { const result = Buffer.alloc(4); result.writeUInt32BE(value); return result; }
function u64(value) { const result = Buffer.alloc(8); result.writeBigUInt64BE(BigInt(value)); return result; }

function assertVersion(value) {
  if (value !== "sqlrs.runtime.v2") throw new Error("unsupported schema version");
}

function rejectUnknown(value, allowed, path) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error(`${path} must be an object`);
  for (const key of Object.keys(value)) if (!allowed.includes(key)) throw new Error(`${path}.${key} is unknown`);
}

// JSON.parse silently accepts duplicate object members. This small lexical
// pass makes the test envelope obey the same no-duplicates rule as Runtime v2.
function assertNoDuplicateMembers(source) {
  let offset = 0;
  parseValue("$");
  whitespace();
  if (offset !== source.length) throw new Error("trailing JSON data");

  function parseValue(path) {
    whitespace();
    if (source[offset] === "{") return parseObject(path);
    if (source[offset] === "[") return parseArray(path);
    if (source[offset] === '"') return parseString();
    const match = /^(?:-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null)/.exec(source.slice(offset));
    if (!match) throw new Error(`invalid JSON at ${path}`);
    offset += match[0].length;
  }

  function parseObject(path) {
    offset++;
    whitespace();
    const seen = new Set();
    if (source[offset] === "}") { offset++; return; }
    while (true) {
      if (source[offset] !== '"') throw new Error(`invalid object key at ${path}`);
      const key = parseString();
      if (seen.has(key)) throw new Error(`duplicate JSON member ${path}.${key}`);
      seen.add(key);
      whitespace();
      if (source[offset++] !== ":") throw new Error(`missing colon at ${path}.${key}`);
      parseValue(`${path}.${key}`);
      whitespace();
      if (source[offset] === "}") { offset++; return; }
      if (source[offset++] !== ",") throw new Error(`invalid object at ${path}`);
      whitespace();
    }
  }

  function parseArray(path) {
    offset++;
    whitespace();
    if (source[offset] === "]") { offset++; return; }
    let index = 0;
    while (true) {
      parseValue(`${path}[${index++}]`);
      whitespace();
      if (source[offset] === "]") { offset++; return; }
      if (source[offset++] !== ",") throw new Error(`invalid array at ${path}`);
    }
  }

  function parseString() {
    const start = offset++;
    while (offset < source.length) {
      if (source[offset] === "\\") { offset += 2; continue; }
      if (source[offset++] === '"') return JSON.parse(source.slice(start, offset));
    }
    throw new Error("unterminated JSON string");
  }

  function whitespace() {
    while (/\s/.test(source[offset] ?? "")) offset++;
  }
}
