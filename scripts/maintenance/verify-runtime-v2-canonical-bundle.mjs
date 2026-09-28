import { createHash } from "node:crypto";
import { lstat, readFile, readdir } from "node:fs/promises";
import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";

const BUNDLE_SCHEMA = "sqlrs.runtime.conformance.bundle-schema.v1";
const BUNDLE_VERSION = "runtime-v2-canonical-v1.1";
const SEMANTIC_SCHEMA = "sqlrs.runtime.v2.canonical.v1";
const VECTOR_SCHEMA = "sqlrs.runtime.v2.canonical.conformance-vector.v1";
const DIGEST_DOMAIN = "sqlrs.runtime.conformance.bundle.v1";
const VALUE_DOMAIN = "sqlrs.runtime.v2/identity-value";
const SCHEMA = "sqlrs.runtime.v2.canonical.v1";
const ENVELOPE = "sqlrs.runtime.fingerprint-envelope.v1";
const DOMAINS = {
  "canonical-value": VALUE_DOMAIN,
  "factory-state": `${SCHEMA}/factory-state`,
  transform: `${SCHEMA}/transform`,
  "resolved-extension": `${SCHEMA}/resolved-extension`,
  "derived-state": `${SCHEMA}/state`,
};
const portablePath = /^[a-z0-9][a-z0-9._-]*(\/[a-z0-9][a-z0-9._-]*)+$/;
const vectorID = /^[a-z][a-z0-9._/-]{0,127}$/;
const requiredTags = new Set(["legacy-non-reinterpretation", "canonical-value", "factory", "transform", "resolved-extension", "root-state", "derived-state", "extension-composition", "ordered-recipe", "relative-lineage", "mutable-reference-same-resolution", "identity-changing", "diagnostics-only", "secret-reference", "safe-redaction", "internal-disclosure", "integrity-tampering", "supported-builder-operational-metadata", "limits", "fuzz-seed"]);

function u32(value) {
  const result = Buffer.alloc(4);
  result.writeUInt32BE(value);
  return result;
}

function u16(value) {
  const result = Buffer.alloc(2);
  result.writeUInt16BE(value);
  return result;
}

function u64(value) {
  const result = Buffer.alloc(8);
  result.writeBigUInt64BE(BigInt(value));
  return result;
}

function framed(value) {
  const bytes = Buffer.isBuffer(value) ? value : Buffer.from(value, "utf8");
  return Buffer.concat([u64(bytes.length), bytes]);
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest();
}

function digest(bytes) {
  return `sha256:${sha256(bytes).toString("hex")}`;
}

function digestBytes(value) {
  if (!/^sha256:[0-9a-f]{64}$/.test(value)) throw new Error("invalid digest");
  return Buffer.from(value.slice(7), "hex");
}

function record(domain, fields) {
  const ordered = [...fields].sort((left, right) => left[0] - right[0]);
  return Buffer.concat([framed(domain), u32(ordered.length), ...ordered.flatMap(([tag, payload]) => [u16(tag), framed(payload)])]);
}

function validation(code, path, message = code) {
  return Object.assign(new Error(message), { code, path });
}

function canonicalValue(node, depth = 1, budget = { nodes: 0 }) {
  budget.nodes += 1;
  if (depth > 32 || budget.nodes > 4096) throw validation("limit_exceeded", "members");
  let tag;
  let payload;
  switch (node.type) {
    case "null": tag = 0; payload = Buffer.alloc(0); break;
    case "string": {
      tag = 1;
      payload = Buffer.from(node.value, "utf8");
      if (payload.length > 4096) throw validation("limit_exceeded", "value");
      break;
    }
    case "list":
    case "set": {
      tag = node.type === "list" ? 2 : 4;
      if (node.members.length > 256) throw validation("limit_exceeded", "members");
      let members = node.members.map((member) => canonicalValue(member, depth + 1, budget));
      if (tag === 4) {
        members = members.sort(Buffer.compare);
        if (members.some((member, index) => index > 0 && member.equals(members[index - 1]))) {
          throw validation("non_canonical", `members[${members.findIndex((member, index) => index > 0 && member.equals(members[index - 1]))}]`);
        }
      }
      payload = Buffer.concat([u32(members.length), ...members.map(framed)]);
      break;
    }
    case "map": {
      tag = 3;
      if (node.entries.length > 256) throw validation("limit_exceeded", "entries");
      const seen = new Set();
      const entries = node.entries.map(({ key, value }, index) => {
        if (seen.has(key)) throw validation("non_canonical", `entries[${index}].key`);
        seen.add(key);
        const keyBytes = Buffer.from(key, "utf8");
        if (keyBytes.length > 1024) throw validation("limit_exceeded", `entries[${index}].key`);
        return [canonicalValue({ type: "string", value: key }, depth + 1, budget), canonicalValue(value, depth + 1, budget)];
      }).sort((left, right) => Buffer.compare(left[0], right[0]));
      payload = Buffer.concat([u32(entries.length), ...entries.flatMap(([key, value]) => [framed(key), framed(value)])]);
      break;
    }
    case "depth-over": {
      let value = { type: "null" };
      for (let index = 1; index < node.depth; index += 1) value = { type: "list", members: [value] };
      return canonicalValue(value, depth, budget);
    }
    case "duplicate-map": return canonicalValue({ type: "map", entries: [{ key: "a", value: { type: "null" } }, { key: "a", value: { type: "null" } }] }, depth, budget);
    case "duplicate-set": return canonicalValue({ type: "set", members: [{ type: "null" }, { type: "null" }] }, depth, budget);
    case "invalid-utf8": throw validation("value_invalid", "value");
    case "members-over": throw validation("limit_exceeded", "members");
    case "nodes-over": throw validation("limit_exceeded", "members");
    case "string-over": throw validation("limit_exceeded", "value");
    case "key-over": throw validation("limit_exceeded", "entries[0].key");
    case "bytes-over": throw validation("limit_exceeded", "members");
    default: throw new Error(`unknown canonical value ${node.type}`);
  }
  const result = Buffer.concat([u16(tag), framed(payload)]);
  if (result.length > 1 << 20) throw validation("limit_exceeded", node.type === "map" ? "entries" : "members");
  return result;
}

function parseCanonicalEnvelope(raw) {
  if (raw.length > 1 << 20) throw validation("limit_exceeded", "value");

  function parseNode(offset, depth) {
    if (depth > 32 || raw.length - offset < 10) throw validation("shape_invalid", "value");
    const start = offset;
    const tag = raw.readUInt16BE(offset);
    const length = Number(raw.readBigUInt64BE(offset + 2));
    offset += 10;
    if (!Number.isSafeInteger(length) || length > raw.length - offset) throw validation("shape_invalid", "value");
    const end = offset + length;
    let nodes = 1;
    let treeDepth = depth;

    if (tag === 0) {
      if (length !== 0) throw validation("shape_invalid", "value");
    } else if (tag === 1) {
      const value = raw.subarray(offset, end);
      if (value.length > 4096) throw validation("limit_exceeded", "value");
      try {
        new TextDecoder("utf-8", { fatal: true }).decode(value);
      } catch {
        throw validation("value_invalid", "value");
      }
    } else if (tag === 2 || tag === 4) {
      if (end - offset < 4) throw validation("shape_invalid", "members");
      const count = raw.readUInt32BE(offset);
      offset += 4;
      if (count > 256) throw validation("limit_exceeded", "members");
      if (count * 8 > end - offset) throw validation("shape_invalid", "members");
      let previous;
      for (let index = 0; index < count; index += 1) {
        if (end - offset < 8) throw validation("shape_invalid", "value");
        const framedLength = Number(raw.readBigUInt64BE(offset));
        offset += 8;
        if (!Number.isSafeInteger(framedLength) || framedLength > end - offset) throw validation("shape_invalid", "value");
        const childEnd = offset + framedLength;
        const child = parseNode(offset, depth + 1);
        if (child.end !== childEnd) throw validation("shape_invalid", "members");
        const encoded = raw.subarray(offset, childEnd);
        if (tag === 4 && previous && Buffer.compare(previous, encoded) >= 0) throw validation("non_canonical", `members[${index}]`);
        previous = encoded;
        nodes += child.nodes;
        treeDepth = Math.max(treeDepth, child.depth);
        offset = childEnd;
      }
      if (offset !== end) throw validation("shape_invalid", "members");
    } else if (tag === 3) {
      if (end - offset < 4) throw validation("shape_invalid", "entries");
      const count = raw.readUInt32BE(offset);
      offset += 4;
      if (count > 256) throw validation("limit_exceeded", "entries");
      if (count * 16 > end - offset) throw validation("shape_invalid", "entries");
      let previous;
      for (let index = 0; index < count; index += 1) {
        if (end - offset < 8) throw validation("shape_invalid", "value");
        const keyLength = Number(raw.readBigUInt64BE(offset));
        offset += 8;
        if (!Number.isSafeInteger(keyLength) || keyLength > end - offset) throw validation("shape_invalid", "value");
        const keyEnd = offset + keyLength;
        const key = parseNode(offset, depth + 1);
        if (key.end !== keyEnd || raw.readUInt16BE(offset) !== 1) throw validation("shape_invalid", `entries[${index}].key`);
        if (keyLength - 10 > 1024) throw validation("limit_exceeded", `entries[${index}].key`);
        const encodedKey = raw.subarray(offset, keyEnd);
        if (previous && Buffer.compare(previous, encodedKey) >= 0) throw validation("non_canonical", `entries[${index}].key`);
        previous = encodedKey;
        nodes += key.nodes;
        treeDepth = Math.max(treeDepth, key.depth);
        offset = keyEnd;
        if (end - offset < 8) throw validation("shape_invalid", "value");
        const valueLength = Number(raw.readBigUInt64BE(offset));
        offset += 8;
        if (!Number.isSafeInteger(valueLength) || valueLength > end - offset) throw validation("shape_invalid", "value");
        const valueEnd = offset + valueLength;
        const value = parseNode(offset, depth + 1);
        if (value.end !== valueEnd) throw validation("shape_invalid", `entries[${index}].value`);
        nodes += value.nodes;
        treeDepth = Math.max(treeDepth, value.depth);
        offset = valueEnd;
      }
      if (offset !== end) throw validation("shape_invalid", "entries");
    } else {
      throw validation("value_invalid", "value");
    }
    if (nodes > 4096 || treeDepth > 32) throw validation("limit_exceeded", "value");
    return { end, nodes, depth: treeDepth, bytes: raw.subarray(start, end) };
  }

  const parsed = parseNode(0, 1);
  if (parsed.end !== raw.length) throw validation("shape_invalid", "value");
  return parsed.bytes;
}

function valueOutput(input) {
  const bytes = canonicalValue(input);
  const preimage = Buffer.concat([framed(VALUE_DOMAIN), framed(bytes)]);
  return { "canonical-bytes": bytes.toString("hex"), preimage_hex: preimage.toString("hex"), "canonical-value-token": `civ1:sha256:${sha256(preimage).toString("hex")}` };
}

function fieldPayload(field, path = "subject.fields") {
  if (!/^[a-z][a-z0-9._-]{0,127}$/.test(field.name) || !["public", "protected"].includes(field.disclosure)) throw validation("shape_invalid", path);
  if (field.kind === "text") {
    const bytes = Buffer.from(field.text, "utf8");
    if (bytes.length > 4096) throw validation("limit_exceeded", `${path}.text`);
    return [1, bytes];
  }
  if (field.kind === "canonical-value") {
    if (!/^(?:[0-9a-f]{2})+$/.test(field.canonical_hex)) throw validation("shape_invalid", `${path}.canonical_hex`);
    const encoded = Buffer.from(field.canonical_hex, "hex");
    parseCanonicalEnvelope(encoded);
    const token = `civ1:sha256:${sha256(Buffer.concat([framed(VALUE_DOMAIN), framed(encoded)])).toString("hex")}`;
    if (field.token !== token) throw validation("commitment_mismatch", `${path}.token`);
    return [2, Buffer.from(token.slice("civ1:sha256:".length), "hex")];
  }
  if (field.kind === "secret-reference") {
    const secret = field.secret_reference;
    if (!secret || !/^[a-z][a-z0-9._-]{0,127}$/.test(secret.provider) || !secret.identifier || !secret.version || Buffer.byteLength(secret.identifier) > 4096 || Buffer.byteLength(secret.version) > 4096) throw validation("value_invalid", `${path}.secret_reference`);
    return [3, Buffer.concat([framed(secret.provider), framed(secret.identifier), framed(secret.version)])];
  }
  throw new Error(`unknown field kind ${field.kind}`);
}

function verifyEnvelope(envelope) {
  const descriptor = envelope.descriptor;
  if (descriptor.envelope_version !== ENVELOPE || descriptor.schema_version !== SCHEMA) throw Object.assign(new Error("revision mismatch"), { code: "revision_mismatch", path: "descriptor.schema_version" });
  if (descriptor.algorithm !== "sha256" || descriptor.domain !== DOMAINS[descriptor.kind]) throw Object.assign(new Error("descriptor mismatch"), { code: "descriptor_invalid", path: "descriptor" });
  let preimage;
  if (descriptor.kind === "derived-state") {
    const transformDigest = verifyEnvelope(envelope.subject.transform);
    if (envelope.subject.transform.descriptor.kind !== "transform") throw validation("descriptor_invalid", "subject.transform.descriptor.kind");
    preimage = record(descriptor.domain, [[1, digestBytes(envelope.subject.parent)], [2, digestBytes(transformDigest)]]);
  } else if (descriptor.kind === "canonical-value") {
    if (!/^(?:[0-9a-f]{2})+$/.test(envelope.subject.canonical_hex)) throw validation("shape_invalid", "subject.canonical_hex");
    const bytes = Buffer.from(envelope.subject.canonical_hex, "hex");
    parseCanonicalEnvelope(bytes);
    const token = `civ1:sha256:${sha256(Buffer.concat([framed(VALUE_DOMAIN), framed(bytes)])).toString("hex")}`;
    if (token !== envelope.subject.token) throw Object.assign(new Error("token mismatch"), { code: "commitment_mismatch", path: "subject.token" });
    const actual = `sha256:${token.slice("civ1:sha256:".length)}`;
    if (descriptor.digest !== actual) throw Object.assign(new Error("digest mismatch"), { code: "digest_mismatch", path: "descriptor.digest" });
    return actual;
  } else {
    const fields = envelope.subject.fields;
    const fieldSet = [u32(fields.length)];
    let previousName = "";
    for (const [index, field] of fields.entries()) {
      const fieldPath = `subject.fields[${index}]`;
      if (index > 0 && previousName >= field.name) throw validation("non_canonical", `${fieldPath}.name`);
      const [kind, payload] = fieldPayload(field, fieldPath);
      const commitment = digest(Buffer.concat([framed(field.name), u16(kind), framed(payload)]));
      if (field.commitment !== commitment) throw validation("commitment_mismatch", `${fieldPath}.commitment`);
      fieldSet.push(framed(field.name), u16(kind), digestBytes(commitment));
      previousName = field.name;
    }
    preimage = record(descriptor.domain, [[1, Buffer.from(SCHEMA)], [2, Buffer.from(descriptor.provider)], [3, Buffer.from(descriptor.semantic_kind)], [4, Buffer.from(descriptor.identity_schema)], [5, Buffer.concat(fieldSet)]]);
  }
  const actual = digest(preimage);
  if (descriptor.digest !== actual) throw Object.assign(new Error("digest mismatch"), { code: "digest_mismatch", path: "descriptor.digest" });
  return actual;
}

function envelopeOutput(envelope) {
  return { digest: verifyEnvelope(envelope), descriptor: envelope.descriptor };
}

function lineageOutput(envelope) {
  let parent = verifyEnvelope(envelope.anchor);
  const ids = [];
  for (const [index, step] of envelope.steps.entries()) {
    const actual = verifyEnvelope(step);
    if (step.subject.parent !== parent) throw validation("lineage_mismatch", `steps[${index}]`);
    ids.push(actual);
    parent = actual;
  }
  if (envelope.endpoint !== parent) throw Object.assign(new Error("endpoint mismatch"), { code: "endpoint_mismatch", path: "endpoint" });
  return { endpoint: parent, "state-ids": ids };
}

function explain(envelope, internal) {
  verifyEnvelope(envelope);
  const result = { profile: internal ? "internal" : "safe", descriptor: envelope.descriptor };
  if (envelope.subject.fields) result.fields = envelope.subject.fields.map((field) => {
    if (!internal && (field.disclosure === "protected" || field.kind === "secret-reference")) return { name: field.name, kind: field.kind, redacted: true };
    const output = { name: field.name, kind: field.kind, redacted: false, commitment: field.commitment };
    if (field.kind === "text") output.text = field.text;
    if (field.kind === "canonical-value") output.canonical_hex = field.canonical_hex;
    if (field.kind === "secret-reference") output.secret_reference = field.secret_reference;
    return output;
  });
  return result;
}

function evaluateCase(item) {
  switch (item.operation) {
    case "canonical-value": return valueOutput(item.input);
    case "canonical-token": {
      if (!/^civ1:sha256:[0-9a-f]{64}$/.test(item.input.value)) throw validation("value_invalid", "token");
      return { token: item.input.value };
    }
    case "canonical-value-envelope": {
      if (!/^(?:[0-9a-f]{2})*$/.test(item.input.hex)) throw validation("shape_invalid", "value");
      const unit = Buffer.from(item.input.hex, "hex");
      const repeat = item.input.repeat ?? 1;
      if (!Number.isInteger(repeat) || repeat < 0 || unit.length * repeat > (1 << 20) + 1) throw validation("limit_exceeded", "value");
      const raw = Buffer.alloc(unit.length * repeat);
      for (let index = 0; index < repeat; index += 1) unit.copy(raw, index * unit.length);
      const bytes = parseCanonicalEnvelope(raw);
      const preimage = Buffer.concat([framed(VALUE_DOMAIN), framed(bytes)]);
      return { "canonical-bytes": bytes.toString("hex"), preimage_hex: preimage.toString("hex"), "canonical-value-token": `civ1:sha256:${sha256(preimage).toString("hex")}` };
    }
    case "identity-field": {
      const field = { ...item.input };
      if (field.kind === "canonical-value") {
        const output = valueOutput(field.canonical_value);
        field.canonical_hex = output["canonical-bytes"];
        field.token = output["canonical-value-token"];
      }
      const [kind, payload] = fieldPayload(field);
      return { "field-commitment": digest(Buffer.concat([framed(field.name), u16(kind), framed(payload)])), kind: field.kind };
    }
    case "factory": case "transform": case "resolved-extension": case "root-state": case "derived-state": return envelopeOutput(item.input.envelope);
    case "compose-factory": case "compose-transform": {
      const extensionDigest = verifyEnvelope(item.input.extension);
      const result = item.input.result;
      verifyEnvelope(result);
      if (item.input.extension.descriptor.kind !== "resolved-extension" || !result.subject.fields) throw validation("relation_mismatch", "input");
      const binding = result.subject.fields.find((field) => field.name === "extension.input.0");
      if (!binding || binding.text !== extensionDigest) throw Object.assign(new Error("composition mismatch"), { code: "relation_mismatch", path: "input.result.subject.fields" });
      return envelopeOutput(result);
    }
    case "recipe": case "relative-lineage": return lineageOutput(item.input.envelope);
    case "decode-envelope": return envelopeOutput(item.input.envelope);
    case "explain-safe": return { explanation: explain(item.input.envelope, false), digest: item.input.envelope.descriptor.digest };
    case "explain-internal": return { explanation: explain(item.input.envelope, true), digest: item.input.envelope.descriptor.digest };
    case "legacy-decode": throw Object.assign(new Error("legacy is not canonical"), { code: "revision_mismatch", path: "schema_version" });
    default: throw new Error(`unsupported operation ${item.operation}`);
  }
}

function verifyCase(item) {
  try {
    const actual = evaluateCase(item);
    if (item.expected.status !== "ok") throw new Error(`${item.id}: expected error but succeeded`);
    for (const [name, value] of Object.entries(item.expected)) {
      if (name !== "status" && stableJSON(actual[name]) !== stableJSON(value)) throw new Error(`${item.id}: ${name} mismatch`);
    }
  } catch (error) {
    if (item.expected.status !== "error" || error.code !== item.expected.error.code || error.path !== item.expected.error.path) throw error;
  }
}

function stableJSON(value) {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(",")}]`;
  if (value && typeof value === "object") return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableJSON(value[key])}`).join(",")}}`;
  return JSON.stringify(value);
}

function caseTagMatches(tag, item) {
  switch (tag) {
    case "legacy-non-reinterpretation": return item.operation === "legacy-decode";
    case "canonical-value": case "fuzz-seed": return item.operation === "canonical-value";
    case "factory": case "root-state": return ["factory", "root-state"].includes(item.operation);
    case "transform": return item.operation === "transform";
    case "resolved-extension": return item.operation === "resolved-extension";
    case "derived-state": return item.operation === "derived-state";
    case "extension-composition": return ["compose-factory", "compose-transform"].includes(item.operation);
    case "ordered-recipe": return item.operation === "recipe";
    case "relative-lineage": return item.operation === "relative-lineage";
    case "secret-reference": return ["identity-field", "explain-safe", "explain-internal"].includes(item.operation);
    case "safe-redaction": return item.operation === "explain-safe";
    case "internal-disclosure": return item.operation === "explain-internal";
    case "integrity-tampering": return item.operation === "decode-envelope" && item.expected?.status === "error";
    case "limits": return ["canonical-value", "canonical-value-envelope"].includes(item.operation) && item.expected?.error?.code === "limit_exceeded";
    default: return false;
  }
}

function relationTagMatches(tag, relation) {
  if (["mutable-reference-same-resolution", "diagnostics-only", "supported-builder-operational-metadata"].includes(tag)) {
    return relation.projection === "digest" && relation.comparison === "equal";
  }
  return tag === "identity-changing" && relation.projection === "digest" && relation.comparison === "not-equal";
}

function parseTextJSON(bytes, name) {
  if (bytes.length === 0 || bytes.at(-1) !== 0x0a || (bytes.length > 1 && bytes.at(-2) === 0x0a) || bytes.includes(0x0d)) {
    throw new Error(`${name}: expected UTF-8 LF with one terminal newline`);
  }
  const text = bytes.toString("utf8");
  if (!Buffer.from(text, "utf8").equals(bytes)) throw new Error(`${name}: invalid UTF-8`);
  return JSON.parse(text);
}

async function regularFile(filename) {
  const status = await lstat(filename);
  if (!status.isFile() || status.isSymbolicLink()) throw new Error(`${filename}: non-regular file`);
}

async function listedFiles(root) {
  const result = [];
  async function walk(directory, prefix = "") {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      const relative = prefix ? `${prefix}/${entry.name}` : entry.name;
      const absolute = path.join(directory, entry.name);
      const status = await lstat(absolute);
      if (status.isSymbolicLink()) throw new Error(`${relative}: symbolic link`);
      if (status.isDirectory()) await walk(absolute, relative);
      else if (status.isFile()) result.push(relative);
      else throw new Error(`${relative}: non-regular object`);
    }
  }
  await walk(root);
  return result.sort();
}

export async function verifyCanonicalBundle(root) {
  await regularFile(path.join(root, "manifest.json"));
  await regularFile(path.join(root, "manifest.sha256"));
  const manifestBytes = await readFile(path.join(root, "manifest.json"));
  const manifest = parseTextJSON(manifestBytes, "manifest.json");
  if (manifest.bundle_schema_version !== BUNDLE_SCHEMA || manifest.bundle_version !== BUNDLE_VERSION || manifest.semantic_schema !== SEMANTIC_SCHEMA) {
    throw new Error("manifest metadata mismatch");
  }
  if (!Array.isArray(manifest.entries)) throw new Error("manifest entries missing");

  const actual = await listedFiles(root);
  const expectedFiles = ["manifest.json", "manifest.sha256", ...manifest.entries.map((entry) => entry.path)].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expectedFiles)) throw new Error("missing or unlisted bundle file");

  const digestParts = [framed(DIGEST_DOMAIN), framed(BUNDLE_SCHEMA), u32(manifest.entries.length)];
  const coverage = new Set();
  const ids = new Set();
  const cases = new Map();
  const relations = [];
  let previousPath = "";
  for (const entry of manifest.entries) {
    if (!portablePath.test(entry.path) || entry.path <= previousPath || entry.path === "manifest.json" || entry.path === "manifest.sha256") throw new Error(`invalid path ${entry.path}`);
    const absolute = path.join(root, ...entry.path.split("/"));
    await regularFile(absolute);
    const bytes = await readFile(absolute);
    const digest = sha256(bytes);
    if (entry.size !== bytes.length) throw new Error(`${entry.path}: size mismatch`);
    if (entry.sha256 !== digest.toString("hex")) throw new Error(`${entry.path}: digest mismatch`);
    digestParts.push(framed(entry.path), u64(entry.size), digest);

    const vector = parseTextJSON(bytes, entry.path);
    if (vector.vector_schema !== VECTOR_SCHEMA || !Array.isArray(vector.cases) || !Array.isArray(vector.relations)) throw new Error(`${entry.path}: vector shape`);
    let previousID = "";
    for (const item of vector.cases) {
      if (!vectorID.test(item.id) || item.id <= previousID || ids.has(item.id)) throw new Error(`${entry.path}: invalid case id`);
      if (!Array.isArray(item.tags) || JSON.stringify(item.tags) !== JSON.stringify([...new Set(item.tags)].sort())) throw new Error(`${entry.path}: invalid tags`);
      if (!item.operation || item.input === undefined || item.expected === undefined) throw new Error(`${entry.path}: incomplete case`);
      verifyCase(item);
      ids.add(item.id);
      cases.set(item.id, item);
      item.tags.forEach((tag) => {
        if (requiredTags.has(tag) && !caseTagMatches(tag, item)) throw new Error(`${entry.path}: tag ${tag} does not match ${item.operation}`);
        coverage.add(tag);
      });
      previousID = item.id;
    }
    previousID = "";
    for (const relation of vector.relations) {
      if (!vectorID.test(relation.id) || relation.id <= previousID || ids.has(relation.id)) throw new Error(`${entry.path}: invalid relation id`);
      if (!Array.isArray(relation.tags) || JSON.stringify(relation.tags) !== JSON.stringify([...new Set(relation.tags)].sort())) throw new Error(`${entry.path}: invalid relation tags`);
      if (!Array.isArray(relation.cases) || relation.cases.length !== 2 || !["equal", "not-equal"].includes(relation.comparison)) throw new Error(`${entry.path}: invalid relation`);
      ids.add(relation.id);
      relation.tags.forEach((tag) => {
        if (requiredTags.has(tag) && !relationTagMatches(tag, relation)) throw new Error(`${entry.path}: relation tag ${tag} does not match ${relation.projection}/${relation.comparison}`);
        coverage.add(tag);
      });
      relations.push(relation);
      previousID = relation.id;
    }
    previousPath = entry.path;
  }

  for (const relation of relations) {
    const [left, right] = relation.cases.map((id) => cases.get(id));
    if (!left || !right) throw new Error(`${relation.id}: missing case`);
    const leftValue = left.expected[relation.projection];
    const rightValue = right.expected[relation.projection];
    const equal = JSON.stringify(leftValue) === JSON.stringify(rightValue);
    if (equal !== (relation.comparison === "equal")) throw new Error(`${relation.id}: relation mismatch`);
  }

  const detached = `sha256:${sha256(Buffer.concat(digestParts)).toString("hex")}\n`;
  if ((await readFile(path.join(root, "manifest.sha256"), "utf8")) !== detached) throw new Error("manifest digest mismatch");

  for (const tag of requiredTags) if (!coverage.has(tag)) throw new Error(`missing coverage ${tag}`);

  return detached.trim();
}

const invoked = process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url;
if (invoked) {
  const defaultRoot = fileURLToPath(new URL("../../backend/libs/runtime-go/conformance/canonical-v1", import.meta.url));
  const digest = await verifyCanonicalBundle(path.resolve(process.argv[2] ?? defaultRoot));
  process.stdout.write(`${digest}\n`);
}
