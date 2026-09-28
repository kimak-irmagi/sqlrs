import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

const [outputDirectory, version, sourceSHA, manifestPath] = process.argv.slice(2);
if (!outputDirectory || !/^v0\.3\.0(?:-rc\.[1-9][0-9]*)?$/.test(version ?? "") || !/^[0-9a-f]{40}$/.test(sourceSHA ?? "") || !manifestPath) {
  throw new Error("usage: create-runtime-v2-attestation OUTPUT VERSION SOURCE_SHA MANIFEST_SHA256");
}
const manifestDigest = (await readFile(manifestPath, "utf8")).trim();
if (!/^sha256:[0-9a-f]{64}$/.test(manifestDigest)) throw new Error("invalid manifest digest");
const record = {
  module: "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go",
  tag: `backend/libs/runtime-go/${version}`,
  source_sha: sourceSHA,
  semantic_schema: "sqlrs.runtime.v2.canonical.v1",
  bundle_schema_version: "sqlrs.runtime.conformance.bundle-schema.v1",
  bundle_version: "runtime-v2-canonical-v1.1",
  manifest_digest: manifestDigest,
};
const bytes = Buffer.from(`${JSON.stringify(record)}\n`, "utf8");
const digest = createHash("sha256").update(bytes).digest("hex");
const basename = `runtime-go-${version}-${digest}.attestation.json`;
await mkdir(outputDirectory, { recursive: true });
await writeFile(path.join(outputDirectory, basename), bytes, { flag: "wx" });
await writeFile(path.join(outputDirectory, `${basename}.sha256`), `sha256:${digest}\n`, { flag: "wx" });
process.stdout.write(`${basename}\n`);
