import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("./create-runtime-v2-attestation.mjs", import.meta.url));

test("attestation has exact order, content address, newline, and companion digest", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "sqlrs-attestation-"));
  try {
    const manifest = path.join(root, "manifest.sha256");
    await writeFile(manifest, `sha256:${"a".repeat(64)}\n`);
    const child = spawn(process.execPath, [script, root, "v0.3.0-rc.1", "b".repeat(40), manifest]);
    let stdout = "";
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    const [code] = await once(child, "exit");
    assert.equal(code, 0);
    const basename = stdout.trim();
    assert.match(basename, /^runtime-go-v0\.3\.0-rc\.1-[0-9a-f]{64}\.attestation\.json$/);
    const bytes = await readFile(path.join(root, basename));
    assert.equal(bytes.at(-1), 0x0a);
    assert.deepEqual(Object.keys(JSON.parse(bytes)), ["module", "tag", "source_sha", "semantic_schema", "bundle_schema_version", "bundle_version", "manifest_digest"]);
    const companion = await readFile(path.join(root, `${basename}.sha256`), "utf8");
    assert.equal(companion, `sha256:${basename.split("-").at(-1).replace(".attestation.json", "")}\n`);
    const duplicate = spawn(process.execPath, [script, root, "v0.3.0-rc.1", "b".repeat(40), manifest]);
    const [duplicateCode] = await once(duplicate, "exit");
    assert.notEqual(duplicateCode, 0);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
