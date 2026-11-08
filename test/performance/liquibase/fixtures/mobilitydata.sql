-- E02: canonical PostgreSQL projection for the public MobilityData fixture.
-- Run against both the repository's natural Compose route and sqlrs's result.
WITH change_rows AS (
  -- sqlrs prefixes mounted resources with pathN/; identity is relative to
  -- the public project's liquibase/ directory in both execution paths.
  SELECT orderexecuted, concat_ws('|', id, author,
    regexp_replace(filename, '^((path[0-9]+|mobilitydata)/)+', ''), md5sum,
    coalesce(contexts, '<null>'), coalesce(labels, '<null>')) AS value
  FROM databasechangelog
)
SELECT 'changelog|' || orderexecuted || '|' || value
FROM change_rows ORDER BY orderexecuted;

WITH relations AS (
  SELECT c.oid, c.relname, c.relkind
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'public' AND c.relkind IN ('r', 'm', 'v', 'S')
    AND c.relname NOT IN ('databasechangelog', 'databasechangeloglock')
    AND NOT EXISTS (
      SELECT 1 FROM pg_depend d JOIN pg_extension e ON e.oid = d.refobjid
      WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype = 'e'
    )
), schema_rows AS (
  SELECT 'relation|' || relname || '|' || relkind::text AS value FROM relations
  UNION ALL
  SELECT 'column|' || r.relname || '|' || a.attnum || '|' || a.attname || '|'
      || format_type(a.atttypid, a.atttypmod) || '|' || a.attnotnull || '|'
      || coalesce(pg_get_expr(ad.adbin, ad.adrelid), '<null>')
  FROM relations r JOIN pg_attribute a ON a.attrelid = r.oid
    AND a.attnum > 0 AND NOT a.attisdropped
  LEFT JOIN pg_attrdef ad ON ad.adrelid = r.oid AND ad.adnum = a.attnum
  UNION ALL
  SELECT 'constraint|' || r.relname || '|' || con.conname || '|'
      || pg_get_constraintdef(con.oid)
  FROM relations r JOIN pg_constraint con ON con.conrelid = r.oid
  UNION ALL
  SELECT 'index|' || r.relname || '|' || i.indexname || '|' || i.indexdef
  FROM relations r JOIN pg_indexes i ON i.schemaname = 'public' AND i.tablename = r.relname
  UNION ALL
  SELECT 'sequence|' || r.relname || '|' || s.data_type || '|' || s.start_value
      || '|' || s.min_value || '|' || s.max_value || '|' || s.increment_by
      || '|' || s.cycle || '|' || s.cache_size
  FROM relations r JOIN pg_sequences s ON s.schemaname = 'public'
    AND s.sequencename = r.relname AND r.relkind = 'S'
  UNION ALL
  SELECT 'view|' || r.relname || '|' || pg_get_viewdef(r.oid, false)
  FROM relations r WHERE r.relkind IN ('m', 'v')
)
SELECT 'schema|' || split_part(value, '|', 1) || '|' || count(*) || '|'
    || md5(string_agg(value, E'\n' ORDER BY value))
FROM schema_rows
GROUP BY split_part(value, '|', 1)
ORDER BY split_part(value, '|', 1);
