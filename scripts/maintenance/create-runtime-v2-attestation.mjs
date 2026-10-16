import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const supportedVersion = /^v0\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-rc\.[1-9][0-9]*)?$/;

/** Deterministic existing source-attestation format; publication tooling in H
 * supplies the manifest digest read from S. See runtime-go-module-publication.md.
 */
export function buildRuntimeAttestation(version, sourceSHA, manifestDigest) {
  if (!supportedVersion.test(version ?? "") || !/^[0-9a-f]{40}$/.test(sourceSHA ?? "")) throw new Error("invalid attestation coordinates");
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
  return [{ name: basename, bytes }, { name: `${basename}.sha256`, bytes: Buffer.from(`sha256:${digest}\n`) }];
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [outputDirectory, version, sourceSHA, manifestPath] = process.argv.slice(2);
  if (!outputDirectory || !manifestPath) throw new Error("usage: create-runtime-v2-attestation OUTPUT VERSION SOURCE_SHA MANIFEST_SHA256");
  const assets = buildRuntimeAttestation(version, sourceSHA, (await readFile(manifestPath, "utf8")).trim());
  await mkdir(outputDirectory, { recursive: true });
  for (const asset of assets) await writeFile(path.join(outputDirectory, asset.name), asset.bytes, { flag: "wx" });
  process.stdout.write(`${assets[0].name}\n`);
}
