import assert from "node:assert/strict";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import YAML from "yaml";

import { installJdbcDriver } from "./jdbc-driver-bundle.mjs";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

test("engine checksum and Maven dependency lock agree", () => {
  const manifest = YAML.parse(fs.readFileSync(path.join(repoRoot, "scripts", "external", "manifest.yaml"), "utf8"));
  const locked = manifest.sources["jdbc-postgresql-42.7.13"];
  const code = fs.readFileSync(path.join(repoRoot, "backend", "local-engine-go", "internal", "prepare", "driverbundle", "driver.go"), "utf8");
  const cliCode = fs.readFileSync(path.join(repoRoot, "frontend", "cli-go", "internal", "enginebin", "jdbc.go"), "utf8");
  assert.match(code, new RegExp(`DriverDigest\\s*=\\s*"sha256:${locked.sha256}"`));
  assert.match(cliCode, new RegExp(`JDBCDriverDigest\\s*=\\s*"sha256:${locked.sha256}"`));
  assert.equal(locked.url, "https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.13/postgresql-42.7.13.jar");
});

test("locked JDBC artifact is verified and copied into a release bundle", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "sqlrs-jdbc-bundle-"));
  try {
    const bytes = Buffer.from("verified jar fixture");
    const digest = crypto.createHash("sha256").update(bytes).digest("hex");
    const artifact = path.join(root, "dist", "deps", "postgresql-42.7.13.jar");
    fs.mkdirSync(path.dirname(artifact), { recursive: true });
    fs.writeFileSync(artifact, bytes);
    fs.mkdirSync(path.join(root, "scripts", "external"), { recursive: true });
    fs.writeFileSync(path.join(root, "scripts", "external", "manifest.yaml"), [
      "version: 1",
      "sources:",
      "  jdbc-postgresql-42.7.13:",
      "    kind: single-file",
      "    url: https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.13/postgresql-42.7.13.jar",
      `    sha256: ${digest}`,
      "    out:",
      "      path: dist/deps/postgresql-42.7.13.jar",
      ""
    ].join("\n"));
    const target = path.join(root, "release");
    const installed = installJdbcDriver(root, target);
    assert.equal(installed, path.join(target, "lib", "postgresql-42.7.13.jar"));
    assert.deepEqual(fs.readFileSync(installed), bytes);
    fs.writeFileSync(artifact, "tampered");
    assert.throws(() => installJdbcDriver(root, path.join(root, "bad-release")), /checksum/i);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
