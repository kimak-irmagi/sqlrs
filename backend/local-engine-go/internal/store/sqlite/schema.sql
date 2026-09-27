-- sqlrs local engine schema

CREATE TABLE IF NOT EXISTS states (
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
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_states_fingerprint ON states(state_fingerprint);
CREATE INDEX IF NOT EXISTS idx_states_parent ON states(parent_state_id);
CREATE INDEX IF NOT EXISTS idx_states_image ON states(image_id);
CREATE INDEX IF NOT EXISTS idx_states_kind ON states(prepare_kind);

CREATE TABLE IF NOT EXISTS instances (
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
CREATE INDEX IF NOT EXISTS idx_instances_state ON instances(state_id);
CREATE INDEX IF NOT EXISTS idx_instances_image ON instances(image_id);
CREATE INDEX IF NOT EXISTS idx_instances_expires ON instances(expires_at);

CREATE TABLE IF NOT EXISTS names (
  name TEXT PRIMARY KEY,
  instance_id TEXT,
  state_id TEXT,
  state_fingerprint TEXT NOT NULL,
  image_id TEXT NOT NULL,
  last_used_at TEXT,
  is_primary INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_names_instance ON names(instance_id);
CREATE INDEX IF NOT EXISTS idx_names_state ON names(state_id);
CREATE INDEX IF NOT EXISTS idx_names_image ON names(image_id);
CREATE INDEX IF NOT EXISTS idx_names_primary ON names(instance_id, is_primary);

-- BEGIN SQLRS RUNTIME V2 SCHEMA
CREATE TABLE runtime_v2_store_format (
  slot INTEGER PRIMARY KEY CHECK (slot = 1),
  format_version TEXT NOT NULL CHECK (format_version = 'sqlrs.runtime-persistence.v1'),
  semantic_version TEXT NOT NULL CHECK (semantic_version = 'sqlrs.runtime.v2')
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
CREATE INDEX runtime_v2_states_parent ON runtime_v2_states(parent_state_id);

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
CREATE INDEX runtime_v2_provenance_state_page
  ON runtime_v2_provenance_observations(state_id, insertion_seq ASC);

CREATE TABLE runtime_v2_resolutions (
  cache_key TEXT PRIMARY KEY CHECK (length(cache_key) = 64),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  cache_record_json BLOB NOT NULL CHECK (length(cache_record_json) <= 4194304),
  stored_at TEXT NOT NULL
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
CREATE INDEX runtime_v2_materializations_state_page
  ON runtime_v2_materializations(state_id, insertion_seq DESC);

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

CREATE TRIGGER runtime_v2_states_immutable BEFORE UPDATE ON runtime_v2_states
BEGIN SELECT RAISE(ABORT, 'runtime v2 state is immutable'); END;
CREATE TRIGGER runtime_v2_provenance_immutable
BEFORE UPDATE ON runtime_v2_provenance_observations
BEGIN SELECT RAISE(ABORT, 'runtime v2 provenance is immutable'); END;
CREATE TRIGGER runtime_v2_materializations_immutable
BEFORE UPDATE ON runtime_v2_materializations
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization is immutable'); END;
CREATE TRIGGER runtime_v2_materialization_components_immutable
BEFORE UPDATE ON runtime_v2_materialization_components
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization component is immutable'); END;
CREATE TRIGGER runtime_v2_states_no_delete BEFORE DELETE ON runtime_v2_states
BEGIN SELECT RAISE(ABORT, 'runtime v2 state deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_provenance_no_delete
BEFORE DELETE ON runtime_v2_provenance_observations
BEGIN SELECT RAISE(ABORT, 'runtime v2 provenance deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_materializations_no_delete
BEFORE DELETE ON runtime_v2_materializations
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_materialization_components_no_delete
BEFORE DELETE ON runtime_v2_materialization_components
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization component deletion is not enabled'); END;
-- END SQLRS RUNTIME V2 SCHEMA
