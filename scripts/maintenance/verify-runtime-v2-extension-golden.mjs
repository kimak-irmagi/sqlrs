import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

const target = resolve(process.argv[2] ?? "backend/libs/runtime-go/testdata/extension-golden.json");
const fixture = JSON.parse(await readFile(target, "utf8"));
const identity = fixture.identity;
const fields = [...identity.fields].sort((a, b) => Buffer.from(a.name).compare(Buffer.from(b.name)));
const fieldSet = Buffer.concat([u32(fields.length), ...fields.flatMap((field) => [string(field.name), string(field.value)])]);
const canonical = record("sqlrs.runtime.v2/resolved-extension", [
  [1, Buffer.from(identity.schema_version)],
  [2, Buffer.from(identity.owner)],
  [3, Buffer.from(identity.kind)],
  [4, Buffer.from(identity.identity_schema)],
  [5, fieldSet],
]);
const fingerprint = `sha256:${createHash("sha256").update(canonical).digest("hex")}`;
if (canonical.toString("hex") !== fixture.expected.canonical || fingerprint !== fixture.expected.fingerprint) {
  throw new Error(`extension golden differs: ${fingerprint}`);
}

function record(domain, fields) {
  const ordered = [...fields].sort((a, b) => a[0] - b[0]);
  return Buffer.concat([string(domain), u32(ordered.length), ...ordered.flatMap(([tag, payload]) => [u16(tag), bytes(payload)])]);
}
function string(value) { return bytes(Buffer.from(value, "utf8")); }
function bytes(value) { return Buffer.concat([u64(value.length), value]); }
function u16(value) { const result = Buffer.alloc(2); result.writeUInt16BE(value); return result; }
function u32(value) { const result = Buffer.alloc(4); result.writeUInt32BE(value); return result; }
function u64(value) { const result = Buffer.alloc(8); result.writeBigUInt64BE(BigInt(value)); return result; }
