#!/usr/bin/env node
/**
 * fetch-sql-scripts.mjs
 *
 * Downloads third-party SQL assets described in scripts/external/manifest.yaml,
 * verifies sha256 (if provided), and places results directly into the manifest's
 * output paths (relative to repo/workspace root), e.g. ./examples/...
 *
 * Cache is stored under scripts/external/cache/ (gitignored).
 *
 * Usage:
 *   node scripts/fetch-sql-scripts.mjs
 *   node scripts/fetch-sql-scripts.mjs --print-sha
 *   node scripts/fetch-sql-scripts.mjs --write-sha
 *   node scripts/fetch-sql-scripts.mjs --lock
 *   node scripts/fetch-sql-scripts.mjs --lock --source-prefix chinook-
 *
 * Flags:
 *   --print-sha   Print computed sha256 for each downloaded artifact.
 *   --write-sha   If sha256 in manifest is empty/missing, write computed sha256 back to manifest.
 *   --lock        Fail if any sha256 is missing/empty OR mismatch occurs.
 *   --source-prefix <prefix>
 *                 Download only manifest sources whose names start with the prefix.
 *                 May be repeated; matching sources are combined.
 */

import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import https from "node:https";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import YAML from "yaml";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const repoRoot = path.resolve(__dirname, "..");
const baseDir = path.join(repoRoot, "scripts", "external");
const cacheDir = path.join(baseDir, "cache");
const manifestPath = path.join(baseDir, "manifest.yaml");

export function parseArgs(argv) {
  const args = argv.slice(2);
  const sourcePrefixes = [];
  for (let i = 0; i < args.length; i += 1) {
    if (args[i] !== "--source-prefix") continue;
    const prefix = args[i + 1]?.trim();
    if (!prefix || prefix.startsWith("--")) {
      throw new Error("--source-prefix requires a non-empty value");
    }
    sourcePrefixes.push(prefix);
    i += 1;
  }
  const argSet = new Set(args);
  return {
    printSha: argSet.has("--print-sha"),
    writeSha: argSet.has("--write-sha"),
    lock: argSet.has("--lock"),
    sourcePrefixes,
  };
}

export function selectSources(sources, prefixes) {
  const entries = Object.entries(sources || {});
  if (!Array.isArray(prefixes) || prefixes.length === 0) return entries;
  const selected = entries.filter(([name]) => prefixes.some((prefix) => name.startsWith(prefix)));
  if (selected.length === 0) {
    throw new Error(`No manifest sources match prefixes: ${prefixes.join(", ")}`);
  }
  return selected;
}

function ensureDir(p) {
  fs.mkdirSync(p, { recursive: true });
}

function isNonEmptyString(x) {
  return typeof x === "string" && x.trim().length > 0;
}

function normalizeRel(p) {
  // Prevent accidental absolute paths in manifest (keep output rooted at repoRoot).
  if (!isNonEmptyString(p)) return null;
  const rel = p.replace(/\\/g, "/");
  if (path.isAbsolute(rel)) {
    throw new Error(`Manifest output path must be relative (got absolute): ${p}`);
  }
  // Normalize .. segments
  const normalized = path.normalize(rel);
  if (normalized.startsWith("..")) {
    throw new Error(`Manifest output path must not escape repo root: ${p}`);
  }
  return normalized;
}

async function sha256File(filePath) {
  const h = crypto.createHash("sha256");
  const s = fs.createReadStream(filePath);
  return await new Promise((resolve, reject) => {
    s.on("data", (d) => h.update(d));
    s.on("end", () => resolve(h.digest("hex")));
    s.on("error", reject);
  });
}

function download(url, dst) {
  return new Promise((resolve, reject) => {
    ensureDir(path.dirname(dst));
    const f = fs.createWriteStream(dst);

    const request = (u) => {
      https
        .get(u, (res) => {
          // Follow redirects
          if (
            res.statusCode &&
            res.statusCode >= 300 &&
            res.statusCode < 400 &&
            res.headers.location
          ) {
            res.resume();
            return request(res.headers.location);
          }

          if (res.statusCode !== 200) {
            res.resume();
            return reject(new Error(`HTTP ${res.statusCode} for ${u}`));
          }

          res.pipe(f);
          f.on("finish", () => f.close(resolve));
        })
        .on("error", reject);
    };

    request(url);
  });
}

function unzipList(zipPath) {
  const out = execFileSync("unzip", ["-Z1", zipPath], { encoding: "utf8" });
  return out.split("\n").map((s) => s.trimEnd()).filter(Boolean);
}

function unzipExtract(zipPath, files, dstDir) {
  ensureDir(dstDir);
  execFileSync("unzip", ["-o", zipPath, ...files, "-d", dstDir], { stdio: "inherit" });
}

function safeCopyFile(src, dst) {
  ensureDir(path.dirname(dst));
  // copyFile is atomic enough for our use; you can swap to write temp + rename if needed.
  fs.copyFileSync(src, dst);
}

function loadManifest(p) {
  const text = fs.readFileSync(p, "utf8");
  return YAML.parse(text);
}

function saveManifest(p, obj) {
  // Keep it reasonably stable / readable
  const doc = new YAML.Document(obj);
  doc.contents && (doc.contents.flow = false);
  const out = String(doc);
  fs.writeFileSync(p, out, "utf8");
}

function getOutTargets(source) {
  // We support either:
  // - out.path (single destination file)
  // - out.dir  (directory to place extracted files / named files)
  const out = source.out || {};
  const outPath = normalizeRel(out.path || "");
  const outDir = normalizeRel(out.dir || "");
  return { outPath, outDir };
}

async function main() {
  const opts = parseArgs(process.argv);

  const manifest = loadManifest(manifestPath);
  const sources = selectSources(manifest.sources, opts.sourcePrefixes);

  ensureDir(cacheDir);

  let manifestChanged = false;

  for (const [name, s] of sources) {
    const url = s.url;
    if (!isNonEmptyString(url)) {
      throw new Error(`${name}: url is required`);
    }

    const kind = s.kind;
    if (!["single-file", "zip"].includes(kind)) {
      throw new Error(`${name}: unsupported kind=${kind}`);
    }

    const expected = (s.sha256 || "").toLowerCase().trim();
    const hasExpected = isNonEmptyString(expected);

    const cacheFile =
      kind === "single-file"
        ? path.join(cacheDir, `${name}.download`)
        : path.join(cacheDir, `${name}.zip`);

    // Download (or reuse cache)
    if (!fs.existsSync(cacheFile)) {
      console.log(`[fetch] ${name}: downloading ${url}`);
      await download(url, cacheFile);
    } else {
      console.log(`[fetch] ${name}: using cached ${path.relative(repoRoot, cacheFile)}`);
    }

    // Compute sha
    const actual = await sha256File(cacheFile);
    if (opts.printSha || !hasExpected) {
      console.log(`[sha256] ${name}: ${actual}`);
    }

    // Verify / handle missing sha
    if (hasExpected) {
      if (actual !== expected) {
        throw new Error(
          `[sha256 mismatch] ${name}\nexpected: ${expected}\nactual:   ${actual}\nfile: ${path.relative(repoRoot, cacheFile)}`
        );
      }
    } else {
      if (opts.writeSha) {
        s.sha256 = actual;
        manifestChanged = true;
      } else if (opts.lock) {
        throw new Error(
          `[sha256 missing] ${name}: sha256 is empty in manifest (use --write-sha to lock it in)`
        );
      } else {
        console.warn(
          `[warn] ${name}: sha256 is missing in manifest; continuing (use --write-sha to lock it in)`
        );
      }
    }

    const { outPath, outDir } = getOutTargets(s);

    if (kind === "single-file") {
      if (!outPath) {
        throw new Error(`${name}: out.path is required for kind=single-file`);
      }
      const dst = path.join(repoRoot, outPath);
      safeCopyFile(cacheFile, dst);
      console.log(`[ok] ${name} -> ${path.relative(repoRoot, dst)}`);
      continue;
    }

    // zip
    const includes = s.include || [];
    if (!Array.isArray(includes) || includes.length === 0) {
      throw new Error(`${name}: include[] is required for kind=zip`);
    }
    if (!outDir) {
      throw new Error(`${name}: out.dir is required for kind=zip`);
    }

    const available = new Set(unzipList(cacheFile));
    console.log(`[info] ${name}: zip contains ${available.size} files:`);
    console.log([...available.keys()].map(e => `[${e}]`).join("\n"));
    for (const f of includes) {
      if (!available.has(f)) {
        throw new Error(`"${name}": not found in zip: "${f}"`);
      }
    }

    const tmpDir = path.join(cacheDir, ".tmp", name);
    ensureDir(tmpDir);
    unzipExtract(cacheFile, includes, tmpDir);

    const dstDir = path.join(repoRoot, outDir);
    ensureDir(dstDir);

    for (const f of includes) {
      const src = path.join(tmpDir, f);
      const dst = path.join(dstDir, path.basename(f));
      safeCopyFile(src, dst);
      console.log(`[ok] ${name} -> ${path.relative(repoRoot, dst)}`);
    }
  }

  if (manifestChanged) {
    saveManifest(manifestPath, manifest);
    console.log(`[manifest] updated sha256 values in ${path.relative(repoRoot, manifestPath)}`);
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === __filename) {
  main().catch((e) => {
    console.error(e?.stack || String(e));
    process.exit(1);
  });
}
