import assert from "node:assert/strict";
import test from "node:test";

import { parseArgs, selectSources } from "./fetch-sql-scripts.mjs";

test("parseArgs collects repeated source prefixes", () => {
  const args = parseArgs([
    "node",
    "fetch-sql-scripts.mjs",
    "--lock",
    "--source-prefix",
    "chinook-",
    "--source-prefix",
    "sakila-",
  ]);

  assert.equal(args.lock, true);
  assert.deepEqual(args.sourcePrefixes, ["chinook-", "sakila-"]);
});

test("selectSources keeps the union of matching prefixes", () => {
  const sources = {
    "chinook-postgres": { id: 1 },
    "sakila-schema": { id: 2 },
    "sakila-data": { id: 3 },
    "flights-postgres": { id: 4 },
  };

  assert.deepEqual(selectSources(sources, ["chinook-", "sakila-"]), [
    ["chinook-postgres", { id: 1 }],
    ["sakila-schema", { id: 2 }],
    ["sakila-data", { id: 3 }],
  ]);
});

test("selectSources rejects a filter that matches nothing", () => {
  assert.throws(
    () => selectSources({ "chinook-postgres": {} }, ["missing-"]),
    /No manifest sources match prefixes: missing-/
  );
});
