-- Frozen managed baseline: v0.2.0-rc.1, commit 36e164a.
-- Exported from its actual SQLite catalog; not the current installer.
PRAGMA foreign_keys=ON;
BEGIN;
CREATE TABLE bootstrap_access (lineage_ref TEXT PRIMARY KEY NOT NULL, binding_json TEXT NOT NULL, secret_ref TEXT NOT NULL UNIQUE);

CREATE TABLE instance_access (
 instance_ref TEXT PRIMARY KEY NOT NULL,
 binding_json TEXT NOT NULL,
 secret_ref TEXT NOT NULL UNIQUE,
 stage TEXT NOT NULL CHECK(stage IN ('reserved','applying','verified','retiring','retired','quarantined'))
);

CREATE TABLE instances (
  instance_id TEXT PRIMARY KEY,
  state_id TEXT NOT NULL,
  image_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT,
  runtime_id TEXT,
  runtime_dir TEXT,
  status TEXT,
  FOREIGN KEY(state_id) REFERENCES states(state_id)
);

CREATE TABLE managed_base_lineages (
 domain_ref TEXT NOT NULL CHECK(length(domain_ref) BETWEEN 1 AND 128),
 lineage_ref TEXT NOT NULL CHECK(length(lineage_ref) BETWEEN 1 AND 128),
 engine_kind TEXT NOT NULL CHECK(engine_kind = 'postgres'),
 image_digest TEXT NOT NULL CHECK(length(image_digest) = 71),
 init_spec_digest TEXT NOT NULL CHECK(length(init_spec_digest) = 64),
 policy_version TEXT NOT NULL CHECK(length(policy_version) > 0),
 username TEXT NOT NULL CHECK(length(username) = 44 AND substr(username,1,12) = 'sqlrs_admin_' AND substr(username,13) NOT GLOB '*[^0-9a-f]*'),
 identity_digest TEXT NOT NULL CHECK(length(identity_digest) = 64),
 created_at TEXT NOT NULL CHECK(length(created_at) > 0),
 PRIMARY KEY(domain_ref,lineage_ref),
 UNIQUE(domain_ref,engine_kind,image_digest,init_spec_digest,policy_version)
);

CREATE TABLE managed_base_targets (
 base_key TEXT PRIMARY KEY NOT NULL,
 operation_ref TEXT NOT NULL REFERENCES managed_runtime_operations(operation_ref)
);

CREATE TABLE managed_runtime_operations (
 operation_ref TEXT PRIMARY KEY NOT NULL, target_path TEXT NOT NULL,
 source_ref TEXT NOT NULL, identity_json TEXT NOT NULL,
 physical_identity TEXT NOT NULL UNIQUE, runtime_ref TEXT NOT NULL DEFAULT '',
 stage TEXT NOT NULL CHECK(stage IN ('assigned','ready','running','retired'))
);

CREATE TABLE managed_state_seals (
 state_id TEXT PRIMARY KEY NOT NULL, operation_ref TEXT NOT NULL,
 binding_json TEXT NOT NULL
);

CREATE TABLE managed_store_format (
 slot INTEGER PRIMARY KEY CHECK(slot = 1), format_version TEXT NOT NULL, domain_ref TEXT NOT NULL
 );

CREATE TABLE names (
  name TEXT PRIMARY KEY,
  instance_id TEXT,
  state_id TEXT,
  state_fingerprint TEXT NOT NULL,
  image_id TEXT NOT NULL,
  last_used_at TEXT,
  is_primary INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE prepare_events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id TEXT NOT NULL,
  type TEXT NOT NULL,
  ts TEXT NOT NULL,
  status TEXT,
  task_id TEXT,
  message TEXT,
  result_json TEXT,
  error_json TEXT,
  FOREIGN KEY(job_id) REFERENCES prepare_jobs(job_id) ON DELETE CASCADE
);

CREATE TABLE prepare_jobs (
  job_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  prepare_kind TEXT NOT NULL,
  image_id TEXT NOT NULL,
  plan_only INTEGER NOT NULL DEFAULT 0,
  snapshot_mode TEXT NOT NULL DEFAULT 'always',
  prepare_args_normalized TEXT,
  signature TEXT,
  request_json TEXT,
  created_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT,
  result_json TEXT,
  error_json TEXT
, resolved_image_id TEXT, lineage_ref TEXT REFERENCES managed_base_lineages(lineage_ref), identity_digest TEXT);

CREATE TABLE prepare_tasks (
  job_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  planner_kind TEXT,
  input_kind TEXT,
  input_id TEXT,
  image_id TEXT,
  resolved_image_id TEXT,
  task_hash TEXT,
  output_state_id TEXT,
  cached INTEGER,
  instance_mode TEXT,
  changeset_id TEXT,
  changeset_author TEXT,
  changeset_path TEXT,
  started_at TEXT,
  finished_at TEXT,
  error_json TEXT,
  PRIMARY KEY (job_id, task_id),
  FOREIGN KEY(job_id) REFERENCES prepare_jobs(job_id) ON DELETE CASCADE
);

CREATE TABLE runtime_v2_materialization_components (
  materialization_id TEXT NOT NULL,
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  name TEXT NOT NULL CHECK (length(CAST(name AS BLOB)) BETWEEN 1 AND 128),
  kind TEXT NOT NULL CHECK (length(CAST(kind AS BLOB)) BETWEEN 1 AND 128),
  locator TEXT NOT NULL CHECK (length(CAST(locator AS BLOB)) BETWEEN 1 AND 4096),
  size_bytes INTEGER CHECK (size_bytes IS NULL OR size_bytes >= 0),
  metadata_json BLOB NOT NULL CHECK (length(metadata_json) BETWEEN 2 AND 65536),
  PRIMARY KEY (materialization_id, name),
  FOREIGN KEY (materialization_id)
    REFERENCES runtime_v2_materializations(materialization_id) ON DELETE CASCADE
);

CREATE TABLE runtime_v2_materializations (
  insertion_seq INTEGER PRIMARY KEY AUTOINCREMENT,
  materialization_id TEXT NOT NULL UNIQUE
    CHECK (length(CAST(materialization_id AS BLOB)) BETWEEN 1 AND 256),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  state_id TEXT NOT NULL,
  backend TEXT NOT NULL CHECK (length(CAST(backend AS BLOB)) BETWEEN 1 AND 128),
  runtime_id TEXT CHECK (runtime_id IS NULL OR length(CAST(runtime_id AS BLOB)) BETWEEN 1 AND 256),
  job_id TEXT CHECK (job_id IS NULL OR length(CAST(job_id AS BLOB)) BETWEEN 1 AND 256),
  created_at TEXT NOT NULL,
  size_bytes INTEGER CHECK (size_bytes IS NULL OR size_bytes >= 0),
  metadata_json BLOB NOT NULL CHECK (length(metadata_json) BETWEEN 2 AND 65536),
  FOREIGN KEY (state_id) REFERENCES runtime_v2_states(state_id)
);

CREATE TABLE runtime_v2_provenance_observations (
  insertion_seq INTEGER PRIMARY KEY AUTOINCREMENT,
  state_id TEXT NOT NULL,
  observation_digest TEXT NOT NULL CHECK (length(observation_digest) = 71),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  provenance_json BLOB NOT NULL CHECK (length(provenance_json) <= 4194304),
  observed_at TEXT NOT NULL,
  UNIQUE (state_id, observation_digest),
  FOREIGN KEY (state_id) REFERENCES runtime_v2_states(state_id) ON DELETE CASCADE
);

CREATE TABLE runtime_v2_resolutions (
  cache_key TEXT PRIMARY KEY CHECK (length(cache_key) = 64),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  cache_record_json BLOB NOT NULL CHECK (length(cache_record_json) <= 4194304),
  stored_at TEXT NOT NULL
);

CREATE TABLE runtime_v2_states (
  state_id TEXT PRIMARY KEY CHECK (length(state_id) = 71),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  state_kind TEXT NOT NULL CHECK (state_kind IN ('factory', 'derived')),
  parent_state_id TEXT CHECK (parent_state_id IS NULL OR length(parent_state_id) = 71),
  state_json BLOB NOT NULL CHECK (length(state_json) <= 4194304),
  resolved_identity_json BLOB NOT NULL CHECK (length(resolved_identity_json) <= 4194304),
  stored_at TEXT NOT NULL,
  FOREIGN KEY (parent_state_id) REFERENCES runtime_v2_states(state_id),
  CHECK ((state_kind = 'factory' AND parent_state_id IS NULL) OR
         (state_kind = 'derived' AND parent_state_id IS NOT NULL))
);

CREATE TABLE runtime_v2_store_format (
  slot INTEGER PRIMARY KEY CHECK (slot = 1),
  format_version TEXT NOT NULL CHECK (format_version = 'sqlrs.runtime-persistence.v1'),
  semantic_version TEXT NOT NULL CHECK (semantic_version = 'sqlrs.runtime.v2')
);

CREATE TABLE states (
  state_id TEXT PRIMARY KEY,
  parent_state_id TEXT,
  state_fingerprint TEXT,
  image_id TEXT NOT NULL,
  prepare_kind TEXT NOT NULL,
  prepare_args_normalized TEXT NOT NULL,
  created_at TEXT NOT NULL,
  size_bytes INTEGER,
  last_used_at TEXT,
  use_count INTEGER,
  min_retention_until TEXT,
  evicted_at TEXT,
  eviction_reason TEXT,
  status TEXT
, lineage_ref TEXT REFERENCES managed_base_lineages(lineage_ref), identity_digest TEXT);

CREATE INDEX idx_instances_expires ON instances(expires_at);

CREATE INDEX idx_instances_image ON instances(image_id);

CREATE INDEX idx_instances_state ON instances(state_id);

CREATE UNIQUE INDEX idx_managed_local_lineage ON managed_base_lineages(lineage_ref);

CREATE INDEX idx_names_image ON names(image_id);

CREATE INDEX idx_names_instance ON names(instance_id);

CREATE INDEX idx_names_primary ON names(instance_id, is_primary);

CREATE INDEX idx_names_state ON names(state_id);

CREATE INDEX idx_prepare_events_job_seq ON prepare_events(job_id, seq);

CREATE INDEX idx_prepare_jobs_status ON prepare_jobs(status);

CREATE INDEX idx_prepare_tasks_job ON prepare_tasks(job_id);

CREATE INDEX idx_prepare_tasks_position ON prepare_tasks(job_id, position);

CREATE INDEX idx_prepare_tasks_status ON prepare_tasks(status);

CREATE UNIQUE INDEX idx_states_fingerprint ON states(state_fingerprint);

CREATE INDEX idx_states_image ON states(image_id);

CREATE INDEX idx_states_kind ON states(prepare_kind);

CREATE INDEX idx_states_parent ON states(parent_state_id);

CREATE INDEX runtime_v2_materializations_state_page
  ON runtime_v2_materializations(state_id, insertion_seq DESC);

CREATE INDEX runtime_v2_provenance_state_page
  ON runtime_v2_provenance_observations(state_id, insertion_seq ASC);

CREATE INDEX runtime_v2_states_parent ON runtime_v2_states(parent_state_id);

CREATE TRIGGER bootstrap_access_immutable BEFORE UPDATE ON bootstrap_access
BEGIN SELECT RAISE(ABORT,'immutable bootstrap access binding'); END;

CREATE TRIGGER instance_access_immutable BEFORE UPDATE OF instance_ref,binding_json,secret_ref ON instance_access
BEGIN SELECT RAISE(ABORT,'immutable managed access binding'); END;

CREATE TRIGGER instance_access_lifecycle BEFORE UPDATE OF stage ON instance_access
WHEN NOT (NEW.stage=OLD.stage OR
 (OLD.stage='reserved' AND NEW.stage IN ('applying','retiring','quarantined')) OR
 (OLD.stage='applying' AND NEW.stage IN ('verified','retiring','quarantined')) OR
 (OLD.stage='verified' AND NEW.stage IN ('retiring','quarantined')) OR
 (OLD.stage='quarantined' AND NEW.stage='retiring') OR
 (OLD.stage='retiring' AND NEW.stage='retired'))
BEGIN SELECT RAISE(ABORT,'invalid managed access transition'); END;

CREATE TRIGGER managed_job_immutable BEFORE UPDATE OF resolved_image_id,lineage_ref,identity_digest ON prepare_jobs
WHEN OLD.resolved_image_id IS NOT NEW.resolved_image_id
 OR OLD.lineage_ref IS NOT NEW.lineage_ref OR OLD.identity_digest IS NOT NEW.identity_digest
BEGIN SELECT RAISE(ABORT,'managed job binding is immutable'); END;

CREATE TRIGGER managed_job_insert BEFORE INSERT ON prepare_jobs
WHEN NOT EXISTS (
 SELECT 1 FROM managed_base_lineages l JOIN managed_store_format f ON f.domain_ref=l.domain_ref
 WHERE f.slot=1 AND l.lineage_ref=NEW.lineage_ref AND l.identity_digest=NEW.identity_digest
 AND (NEW.resolved_image_id=l.image_digest OR substr(NEW.resolved_image_id,-72)='@'||l.image_digest)
)
BEGIN SELECT RAISE(ABORT,'invalid managed job binding'); END;

CREATE TRIGGER managed_lineage_domain BEFORE INSERT ON managed_base_lineages
WHEN NOT EXISTS (SELECT 1 FROM managed_store_format WHERE slot=1 AND domain_ref=NEW.domain_ref)
BEGIN SELECT RAISE(ABORT,'invalid managed lineage domain'); END;

CREATE TRIGGER managed_lineage_immutable BEFORE UPDATE ON managed_base_lineages
BEGIN SELECT RAISE(ABORT,'managed lineage is immutable'); END;

CREATE TRIGGER managed_state_immutable BEFORE UPDATE OF state_id,parent_state_id,image_id,prepare_kind,prepare_args_normalized,lineage_ref,identity_digest ON states
WHEN OLD.state_id IS NOT NEW.state_id OR OLD.parent_state_id IS NOT NEW.parent_state_id
 OR OLD.image_id IS NOT NEW.image_id OR OLD.prepare_kind IS NOT NEW.prepare_kind
 OR OLD.prepare_args_normalized IS NOT NEW.prepare_args_normalized
 OR OLD.lineage_ref IS NOT NEW.lineage_ref OR OLD.identity_digest IS NOT NEW.identity_digest
BEGIN SELECT RAISE(ABORT,'managed state binding is immutable'); END;

CREATE TRIGGER managed_state_insert BEFORE INSERT ON states
BEGIN
 SELECT CASE WHEN NOT EXISTS (
  SELECT 1 FROM managed_base_lineages l JOIN managed_store_format f ON f.domain_ref=l.domain_ref
  WHERE f.slot=1 AND l.lineage_ref=NEW.lineage_ref AND l.identity_digest=NEW.identity_digest
   AND (NEW.image_id=l.image_digest OR substr(NEW.image_id,-72)='@'||l.image_digest)
 ) THEN RAISE(ABORT,'invalid managed state binding') END;
 SELECT CASE WHEN NEW.parent_state_id IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM states p WHERE p.state_id=NEW.parent_state_id
   AND p.lineage_ref=NEW.lineage_ref AND p.identity_digest=NEW.identity_digest
 ) THEN RAISE(ABORT,'invalid managed parent binding') END;
 SELECT CASE WHEN EXISTS (
  SELECT 1 FROM states s WHERE s.state_id=NEW.state_id AND (
   s.lineage_ref IS NOT NEW.lineage_ref OR s.identity_digest IS NOT NEW.identity_digest
   OR s.parent_state_id IS NOT NEW.parent_state_id OR s.image_id IS NOT NEW.image_id
   OR s.prepare_kind IS NOT NEW.prepare_kind OR s.prepare_args_normalized IS NOT NEW.prepare_args_normalized
  )
 ) THEN RAISE(ABORT,'conflicting managed state') END;
 SELECT CASE WHEN EXISTS (
  SELECT 1 FROM states s WHERE s.state_fingerprint=NEW.state_fingerprint AND s.state_id IS NOT NEW.state_id
 ) THEN RAISE(ABORT,'conflicting managed state fingerprint') END;
END;

CREATE TRIGGER managed_state_seal_eviction AFTER DELETE ON states BEGIN DELETE FROM managed_state_seals WHERE state_id=OLD.state_id; END;

CREATE TRIGGER runtime_v2_materialization_components_immutable
BEFORE UPDATE ON runtime_v2_materialization_components
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization component is immutable'); END;

CREATE TRIGGER runtime_v2_materialization_components_no_delete
BEFORE DELETE ON runtime_v2_materialization_components
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization component deletion is not enabled'); END;

CREATE TRIGGER runtime_v2_materializations_immutable
BEFORE UPDATE ON runtime_v2_materializations
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization is immutable'); END;

CREATE TRIGGER runtime_v2_materializations_no_delete
BEFORE DELETE ON runtime_v2_materializations
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization deletion is not enabled'); END;

CREATE TRIGGER runtime_v2_provenance_immutable
BEFORE UPDATE ON runtime_v2_provenance_observations
BEGIN SELECT RAISE(ABORT, 'runtime v2 provenance is immutable'); END;

CREATE TRIGGER runtime_v2_provenance_no_delete
BEFORE DELETE ON runtime_v2_provenance_observations
BEGIN SELECT RAISE(ABORT, 'runtime v2 provenance deletion is not enabled'); END;

CREATE TRIGGER runtime_v2_states_immutable BEFORE UPDATE ON runtime_v2_states
BEGIN SELECT RAISE(ABORT, 'runtime v2 state is immutable'); END;

CREATE TRIGGER runtime_v2_states_no_delete BEFORE DELETE ON runtime_v2_states
BEGIN SELECT RAISE(ABORT, 'runtime v2 state deletion is not enabled'); END;

INSERT INTO managed_store_format(slot,format_version,domain_ref) VALUES(1,'sqlrs-managed-store.v1','store_461870f5f36e8f74bad476d6b8b8ac7a');
INSERT INTO runtime_v2_store_format(slot,format_version,semantic_version) VALUES(1,'sqlrs.runtime-persistence.v1','sqlrs.runtime.v2');
COMMIT;
