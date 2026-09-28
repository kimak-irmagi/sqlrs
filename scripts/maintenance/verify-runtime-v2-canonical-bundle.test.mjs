import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { cp, mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { verifyCanonicalBundle } from "./verify-runtime-v2-canonical-bundle.mjs";

const source = fileURLToPath(new URL("../../backend/libs/runtime-go/conformance/canonical-v1", import.meta.url));

function framed(value) {
  const bytes = Buffer.from(value);
  const size = Buffer.alloc(8);
  size.writeBigUInt64BE(BigInt(bytes.length));
  return Buffer.concat([size, bytes]);
}

async function reseal(root) {
  const manifestPath = path.join(root, "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  const count = Buffer.alloc(4);
  count.writeUInt32BE(manifest.entries.length);
  const parts = [framed("sqlrs.runtime.conformance.bundle.v1"), framed(manifest.bundle_schema_version), count];
  for (const entry of manifest.entries) {
    const bytes = await readFile(path.join(root, ...entry.path.split("/")));
    const digest = createHash("sha256").update(bytes).digest();
    entry.size = bytes.length;
    entry.sha256 = digest.toString("hex");
    const size = Buffer.alloc(8);
    size.writeBigUInt64BE(BigInt(bytes.length));
    parts.push(framed(entry.path), size, digest);
  }
  await writeFile(manifestPath, `${JSON.stringify(manifest)}\n`);
  const detached = createHash("sha256").update(Buffer.concat(parts)).digest("hex");
  await writeFile(path.join(root, "manifest.sha256"), `sha256:${detached}\n`);
}

test("independent verifier accepts the locked bundle", async () => {
  assert.equal(await verifyCanonicalBundle(source), "sha256:4009154c578097c86d42aa6f457208a2c84f7d70a86c696ad4ae37b55a78edb4");
});

test("independent verifier rejects changed vector bytes", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "sqlrs-canonical-bundle-"));
  try {
    await cp(source, root, { recursive: true });
    const target = path.join(root, "vectors", "limits.json");
    const bytes = await readFile(target);
    bytes[0] ^= 1;
    await writeFile(target, bytes);
    await assert.rejects(() => verifyCanonicalBundle(root), /digest mismatch/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("independent verifier recomputes sealed semantic known answers", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "sqlrs-canonical-bundle-"));
  try {
    await cp(source, root, { recursive: true });
    const target = path.join(root, "vectors", "canonical-values.json");
    const vector = JSON.parse(await readFile(target, "utf8"));
    vector.cases[0].expected["canonical-value-token"] = `civ1:sha256:${"0".repeat(64)}`;
    await writeFile(target, `${JSON.stringify(vector)}\n`);
    await reseal(root);
    await assert.rejects(() => verifyCanonicalBundle(root), /mismatch/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("independent verifier rejects symbolic-link bundle entries", async (t) => {
  const root = await mkdtemp(path.join(tmpdir(), "sqlrs-canonical-bundle-"));
  try {
    await cp(source, root, { recursive: true });
    const target = path.join(root, "vectors", "limits.json");
    const linkTarget = path.join(root, "vectors", "canonical-values.json");
    await rm(target);
    try {
      await symlink(linkTarget, target, "file");
    } catch (error) {
      if (error.code === "EPERM") return t.skip("symbol creation is unavailable on this host");
      throw error;
    }
    await assert.rejects(() => verifyCanonicalBundle(root), /symbolic link|non-regular/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("independent verifier rejects a required tag on the wrong operation", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "sqlrs-canonical-bundle-"));
  try {
    await cp(source, root, { recursive: true });
    const secretsPath = path.join(root, "vectors", "secrets-and-disclosure.json");
    const canonicalPath = path.join(root, "vectors", "canonical-values.json");
    const secrets = JSON.parse(await readFile(secretsPath, "utf8"));
    const canonical = JSON.parse(await readFile(canonicalPath, "utf8"));
    secrets.cases.find((item) => item.id === "secrets/safe").tags = [];
    canonical.cases.find((item) => item.id === "canonical-value/null").tags.push("safe-redaction");
    canonical.cases.find((item) => item.id === "canonical-value/null").tags.sort();
    await writeFile(secretsPath, `${JSON.stringify(secrets)}\n`);
    await writeFile(canonicalPath, `${JSON.stringify(canonical)}\n`);
    await reseal(root);
    await assert.rejects(() => verifyCanonicalBundle(root), /tag safe-redaction|missing coverage safe-redaction/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
