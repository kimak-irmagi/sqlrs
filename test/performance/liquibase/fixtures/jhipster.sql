\c postgres
select 'change', orderexecuted, id, author, filename, md5sum,
       coalesce(contexts, ''), coalesce(labels, '')
from public.databasechangelog order by orderexecuted;
select 'column', table_name, ordinal_position, column_name, data_type,
       coalesce(character_maximum_length::text, ''),
       coalesce(numeric_precision::text, ''),
       coalesce(numeric_scale::text, ''), is_nullable,
       coalesce(column_default, '')
from information_schema.columns
where table_schema = 'public' and table_name not like 'databasechange%'
order by table_name, ordinal_position;
select 'constraint', c.conrelid::regclass::text, c.conname, c.contype,
       pg_get_constraintdef(c.oid)
from pg_constraint c join pg_namespace n on n.oid = c.connamespace
where n.nspname = 'public'
  and c.conrelid::regclass::text not like 'databasechange%'
order by c.conrelid::regclass::text, c.conname;
select 'index', tablename, indexname, indexdef
from pg_indexes
where schemaname = 'public' and tablename not like 'databasechange%'
order by tablename, indexname;
select 'sequence', sequencename, data_type, start_value, min_value,
       max_value, increment_by, cycle
from pg_sequences where schemaname = 'public' order by sequencename;
