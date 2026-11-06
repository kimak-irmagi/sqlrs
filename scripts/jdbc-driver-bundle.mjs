import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";

import YAML from "yaml";

const sourceName = "jdbc-postgresql-42.7.13";
const fileName = "postgresql-42.7.13.jar";
const mavenURL = `https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.13/${fileName}`;

function sha256File(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

// Read the same locked source entry used by the repository's external-file
// resolver. A release never chooses an unpinned alternative artifact.
export function jdbcDriverSpec(repoRoot) {
  const manifest = YAML.parse(fs.readFileSync(path.join(repoRoot, "scripts", "external", "manifest.yaml"), "utf8"));
  const source = manifest?.sources?.[sourceName];
  if (source?.kind !== "single-file" || source.url !== mavenURL ||
      !/^[0-9a-f]{64}$/.test(source.sha256 || "") ||
      source.out?.path !== `dist/deps/${fileName}`) {
    throw new Error("PostgreSQL JDBC dependency is not locked to the approved Maven artifact");
  }
  return {
    path: path.join(repoRoot, "dist", "deps", fileName),
    sha256: source.sha256,
    fileName
  };
}

// Install a verified JAR next to the released engine. The local and WSL
// engines use the same copy; job execution does not resolve dependencies.
export function installJdbcDriver(repoRoot, releaseRoot) {
  const spec = jdbcDriverSpec(repoRoot);
  if (!fs.existsSync(spec.path) || sha256File(spec.path) !== spec.sha256) {
    throw new Error("PostgreSQL JDBC dependency checksum mismatch or artifact missing");
  }
  const destination = path.join(releaseRoot, "lib", spec.fileName);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  fs.copyFileSync(spec.path, destination);
  if (sha256File(destination) !== spec.sha256) {
    throw new Error("PostgreSQL JDBC release copy checksum mismatch");
  }
  return destination;
}
