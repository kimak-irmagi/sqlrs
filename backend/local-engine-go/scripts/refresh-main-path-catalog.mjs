// Mechanical regeneration only: canonical DDL is the source, not a migration.
// Requirements: issue89-working-instance-handoff-schema.md §4 / OT-WH02.
// Inspect the semantic catalog diff before accepting changed pinned hashes.
import { DatabaseSync } from 'node:sqlite';
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

const root = fileURLToPath(new URL('../', import.meta.url));
const asset = resolve(root, 'internal/prepare/queue/assets');
const fixture = resolve(root, 'internal/managedstore/testdata');
const ddl = readFileSync(resolve(asset, 'liquibase-main-schema.sql'), 'utf8').replaceAll('\r\n', '\n');
const prior = JSON.parse(readFileSync(resolve(asset, 'liquibase-main-catalog.json'), 'utf8'));
const digest = text => createHash('sha256').update(text.trim().replaceAll('\r\n', '\n')).digest('hex');
const starts = [...ddl.matchAll(/^CREATE (?:UNIQUE )?(?:TABLE|INDEX|TRIGGER) /gm)].map(m => m.index);
const db = new DatabaseSync(':memory:');
try {
  db.exec(ddl);
  const definitions = new Map(db.prepare("SELECT name,type,tbl_name,sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'").all().map(row => [row.name, row]));
  const objects = starts.map((offset, index) => {
    const statement = ddl.slice(offset, starts[index + 1] ?? ddl.length).trimEnd();
    const match = statement.match(/^CREATE (?:UNIQUE )?(?:TABLE|INDEX|TRIGGER) ["`]?([^\s"`(]+)/);
    if (!match || !statement.endsWith(';')) throw new Error(`Invalid canonical statement at ${offset}`);
    const row = definitions.get(match[1]);
    if (!row) throw new Error(`Missing SQLite definition: ${match[1]}`);
    const original = prior.objects.find(item => item.name === row.name);
    return { name: row.name, type: row.type, table: row.tbl_name, sha256: digest(row.sql), offset: Buffer.byteLength(ddl.slice(0, offset)), length: Buffer.byteLength(statement), ...(original?.released_sha256 ? { released_sha256: original.released_sha256 } : {}) };
  });
  if (objects.length !== definitions.size) throw new Error('Canonical statement/catalog mismatch');
  const markers = prior.markers.map(marker => marker.table === 'prepare_liquibase_session_store_format' ? { ...marker, sql: 'INSERT INTO prepare_liquibase_session_store_format(slot,format_version) VALUES(1,3);' } : marker);
  const expected = Object.fromEntries([...objects].sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0).map(row => [row.name, { type: row.type, table: row.table, sha256: row.sha256 }]));
  writeFileSync(resolve(asset, 'liquibase-main-catalog.json'), JSON.stringify({ objects, markers }, null, 2) + '\n');
  writeFileSync(resolve(fixture, 'issue89-target-catalog.json'), JSON.stringify(expected, null, 2) + '\n');
  for (const name of ['liquibase-main-schema.sql', 'liquibase-main-catalog.json']) {
    console.log(`${name} ${createHash('sha256').update(readFileSync(resolve(asset, name))).digest('hex')}`);
  }
  console.log(`issue89-target-catalog.json ${createHash('sha256').update(readFileSync(resolve(fixture, 'issue89-target-catalog.json'))).digest('hex')}`);
} finally {
  db.close();
}
