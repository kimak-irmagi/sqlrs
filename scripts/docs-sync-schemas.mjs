import fs from "node:fs";
import path from "node:path";

const rootDir = process.cwd();
const docsDir = path.join(rootDir, "docs");

const refStart = "<!--ref:";
const refEnd = "<!--ref:end-->";
const refBody = "<!--ref:body-->";

function collectMarkdownFiles(dir) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...collectMarkdownFiles(fullPath));
      continue;
    }
    if (entry.isFile() && entry.name.toLowerCase().endsWith(".md")) {
      files.push(fullPath);
    }
  }
  return files;
}

function parseLinkTarget(prefix, filePath) {
  const match = prefix.match(/\]\(([^)]+)\)/);
  if (!match) {
    throw new Error(`Missing markdown link in ref block: ${filePath}`);
  }
  const target = match[1];
  const [linkPath, hash] = target.split("#");
  const resolved = path.resolve(path.dirname(filePath), linkPath);
  return { resolved, hash };
}

function readSnippet(resolvedPath, hash) {
  const raw = fs.readFileSync(resolvedPath, "utf8").replace(/\r\n/g, "\n");
  let lines = raw.split("\n");
  if (lines.length && lines[lines.length - 1] === "") {
    lines = lines.slice(0, -1);
  }

  let start = 1;
  let end = lines.length;
  if (hash) {
	if (hash.startsWith("region=")) {
	  const region = decodeURIComponent(hash.slice("region=".length));
	  const beginMarker = `-- BEGIN ${region}`;
	  const endMarker = `-- END ${region}`;
	  const begin = lines.indexOf(beginMarker);
	  const finish = lines.indexOf(endMarker, begin + 1);
	  if (begin === -1 || finish === -1 || finish <= begin) {
		throw new Error(`Invalid region "${region}" in ${resolvedPath}`);
	  }
	  return lines.slice(begin + 1, finish).join("\n").trim();
	}
    const match = hash.match(/L(\d+)(?:-L(\d+))?/i);
    if (!match) {
      throw new Error(`Invalid line range "${hash}" in ${resolvedPath}`);
    }
    start = Number(match[1]);
    end = match[2] ? Number(match[2]) : start;
  }

  if (start < 1 || start > lines.length) {
    throw new Error(`Start line out of range in ${resolvedPath}`);
  }
  if (end < start) {
    end = start;
  }
  if (end > lines.length) {
    end = lines.length;
  }
  return lines.slice(start - 1, end).join("\n");
}

function processBlock(block, filePath) {
  const startMatch = block.match(/^<!--ref:([a-zA-Z0-9_-]+)\s*-->/);
  if (!startMatch) {
    return block;
  }
  const lang = startMatch[1];
  const startIndex = block.indexOf(startMatch[0]) + startMatch[0].length;
  const bodyIndex = block.indexOf(refBody, startIndex);
  const endIndex = block.lastIndexOf(refEnd);
  const prefix = block.slice(startIndex, bodyIndex === -1 ? endIndex : bodyIndex);

  const { resolved, hash } = parseLinkTarget(prefix, filePath);
  const snippet = readSnippet(resolved, hash);

  const linkText = prefix.trim();
  if (linkText === "") {
    throw new Error(`Missing link text in ref block: ${filePath}`);
  }
  const codeBlock = `\`\`\`${lang}\n${snippet}\n\`\`\``;

  return `${refStart}${lang} -->\n${linkText}\n${refBody}\n${codeBlock}\n${refEnd}`;
}

function findRefStart(content, fromIndex) {
  let idx = content.indexOf(refStart, fromIndex);
  while (idx !== -1) {
    if (idx === 0 || content[idx - 1] === "\n") {
      return idx;
    }
    idx = content.indexOf(refStart, idx + refStart.length);
  }
  return -1;
}

function processContent(content, filePath) {
  let cursor = 0;
  let out = "";
  let changed = false;

  while (true) {
    const start = findRefStart(content, cursor);
    if (start === -1) {
      out += content.slice(cursor);
      break;
    }
    out += content.slice(cursor, start);
    const end = content.indexOf(refEnd, start);
    if (end === -1) {
      throw new Error(`Unclosed ref block in ${filePath}`);
    }
    const block = content.slice(start, end + refEnd.length);
    const next = processBlock(block, filePath);
    if (next !== block) {
      changed = true;
    }
    out += next;
    cursor = end + refEnd.length;
  }

  return { content: out, changed };
}

function main() {
  const files = collectMarkdownFiles(docsDir);
  let updated = 0;
  for (const filePath of files) {
    const content = fs.readFileSync(filePath, "utf8");
    if (!content.includes(refStart)) {
      continue;
    }
    const result = processContent(content, filePath);
    if (result.changed) {
      fs.writeFileSync(filePath, result.content);
      updated += 1;
    }
  }
  console.log(`docs-sync-schemas: updated ${updated} file(s)`);
}

main();
