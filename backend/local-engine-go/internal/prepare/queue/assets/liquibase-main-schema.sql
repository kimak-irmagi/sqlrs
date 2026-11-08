-- Mechanically extracted final approved catalog, 2026-11-07.
-- No intermediate prototype migration is executed by this artifact.

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

CREATE TABLE liquibase_execution_bindings (
  execution_fingerprint TEXT PRIMARY KEY,
  declaration_digest TEXT NOT NULL,
  requested_tag TEXT NOT NULL,
  target_identity_digest TEXT NOT NULL,
  platform_os TEXT NOT NULL,
  platform_arch TEXT NOT NULL,
  platform_variant TEXT,
  image_index_digest TEXT,
  image_manifest_digest TEXT NOT NULL,
  driver_mode TEXT NOT NULL CHECK (driver_mode IN ('embedded', 'mounted')),
  driver_digest TEXT NOT NULL,
  driver_artifact_ref TEXT,
  bundle_manifest_digest TEXT NOT NULL,
  effective_config_digest TEXT NOT NULL,
  execution_policy_revision TEXT NOT NULL,
  created_at TEXT NOT NULL,
  CHECK ((driver_mode = 'embedded' AND driver_artifact_ref IS NULL) OR
         (driver_mode = 'mounted' AND driver_artifact_ref IS NOT NULL))
);

CREATE TABLE liquibase_job_bindings (
  job_id TEXT PRIMARY KEY REFERENCES prepare_jobs(job_id) ON DELETE CASCADE,
  requested_tag TEXT NOT NULL,
  bundle_manifest_digest TEXT NOT NULL,
  execution_fingerprint TEXT REFERENCES liquibase_execution_bindings(execution_fingerprint),
  bound_at TEXT,
  CHECK ((execution_fingerprint IS NULL AND bound_at IS NULL) OR
         (execution_fingerprint IS NOT NULL AND bound_at IS NOT NULL))
);

CREATE TABLE liquibase_state_bindings (
  state_id TEXT PRIMARY KEY REFERENCES states(state_id) ON DELETE CASCADE,
  execution_fingerprint TEXT NOT NULL REFERENCES liquibase_execution_bindings(execution_fingerprint)
);

CREATE TABLE liquibase_store_format (
  slot INTEGER PRIMARY KEY CHECK(slot=1),
  format_version INTEGER NOT NULL CHECK(format_version=2)
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

CREATE TABLE "prepare_artifact_leases" (
  lease_id TEXT PRIMARY KEY,
  artifact_kind TEXT NOT NULL CHECK(artifact_kind IN ('bundle','command','driver','capture_helper')),
  artifact_digest TEXT NOT NULL,
  owner_kind TEXT NOT NULL CHECK(owner_kind IN ('admission','job','explain','retention')),
  owner_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT,
  CHECK((owner_kind='job' AND expires_at IS NULL)
    OR (owner_kind<>'job' AND expires_at IS NOT NULL)),
  CHECK(length(artifact_digest)=71 AND substr(artifact_digest,1,7)='sha256:'
    AND substr(artifact_digest,8) NOT GLOB '*[^0-9a-f]*'),
  UNIQUE(artifact_kind,artifact_digest,owner_kind,owner_id)
);

CREATE TABLE "prepare_attempt_decisions" (
  decision_id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  attempt_no INTEGER NOT NULL,
  decision_no INTEGER NOT NULL CHECK(decision_no>0),
  choice TEXT NOT NULL CHECK(choice IN ('publish_original','retry_original','replay_fresh','require_recovery','complete_planning')),
  target_clone_id TEXT REFERENCES prepare_clones(clone_id) DEFERRABLE INITIALLY DEFERRED,
  next_attempt_no INTEGER,
  worker_generation INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(job_id,task_id,attempt_no,decision_no),
  FOREIGN KEY(job_id,task_id,attempt_no)
    REFERENCES prepare_step_attempts(job_id,task_id,attempt_no) ON DELETE CASCADE
);

CREATE TABLE prepare_attempt_evidence (
  evidence_id INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  attempt_no INTEGER NOT NULL,
  kind TEXT NOT NULL,
  scope_digest TEXT NOT NULL,
  facts_json TEXT NOT NULL CHECK(length(CAST(facts_json AS BLOB))<=8192),
  facts_digest TEXT NOT NULL,
  worker_generation INTEGER NOT NULL,
  observed_at TEXT NOT NULL,
  FOREIGN KEY(job_id,task_id,attempt_no)
    REFERENCES prepare_step_attempts(job_id,task_id,attempt_no) ON DELETE CASCADE,
  CHECK(length(scope_digest)=71 AND substr(scope_digest,1,7)='sha256:'
    AND substr(scope_digest,8) NOT GLOB '*[^0-9a-f]*'),
  CHECK(length(facts_digest)=71 AND substr(facts_digest,1,7)='sha256:'
    AND substr(facts_digest,8) NOT GLOB '*[^0-9a-f]*'),
  CHECK(json_valid(facts_json))
);

CREATE TABLE "prepare_attempt_store_format" (
  slot INTEGER PRIMARY KEY CHECK(slot=1),
  format_version INTEGER NOT NULL CHECK(format_version=3)
);

CREATE TABLE prepare_clones (
  clone_id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES prepare_jobs(job_id) ON DELETE CASCADE,
  clone_generation INTEGER NOT NULL CHECK(clone_generation>0),
  source_kind TEXT NOT NULL CHECK(source_kind IN ('image','state')),
  source_image_id TEXT,
  source_state_id TEXT REFERENCES states(state_id),
  clone_ref TEXT NOT NULL UNIQUE,
  planned_db_runtime_name TEXT NOT NULL UNIQUE,
  db_runtime_id TEXT,
  phase TEXT NOT NULL CHECK(phase IN ('allocated','active','quiescent','retained','released')),
  created_at TEXT NOT NULL,
  UNIQUE(job_id,clone_generation),
  CHECK((source_kind='image' AND source_image_id IS NOT NULL AND source_state_id IS NULL)
     OR (source_kind='state' AND source_image_id IS NULL AND source_state_id IS NOT NULL))
);

CREATE TABLE prepare_decision_evidence (
  decision_id TEXT NOT NULL REFERENCES prepare_attempt_decisions(decision_id) ON DELETE CASCADE,
  evidence_id INTEGER NOT NULL REFERENCES prepare_attempt_evidence(evidence_id) ON DELETE CASCADE,
  PRIMARY KEY(decision_id,evidence_id)
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

CREATE TABLE prepare_execution_bindings (
  execution_digest TEXT PRIMARY KEY,
  prepare_kind TEXT NOT NULL CHECK(prepare_kind<>''),
  command_digest TEXT NOT NULL,
  input_manifest_digest TEXT NOT NULL,
  executor_image_digest TEXT NOT NULL,
  target_identity_digest TEXT NOT NULL,
  policy_revision TEXT NOT NULL CHECK(policy_revision<>''),
  effect_scope TEXT NOT NULL CHECK(effect_scope IN ('unknown','clone_confined')),
  partial_prefix_safe INTEGER NOT NULL DEFAULT 0 CHECK(partial_prefix_safe IN (0,1)),
  enforcement_digest TEXT,
  adapter_binding_digest TEXT REFERENCES liquibase_execution_bindings(execution_fingerprint),
  created_at TEXT NOT NULL,
  CHECK(length(execution_digest)=71 AND substr(execution_digest,1,7)='sha256:'
    AND substr(execution_digest,8) NOT GLOB '*[^0-9a-f]*'),
  CHECK(length(command_digest)=71 AND substr(command_digest,1,7)='sha256:'
    AND substr(command_digest,8) NOT GLOB '*[^0-9a-f]*'),
  CHECK(length(input_manifest_digest)=71 AND substr(input_manifest_digest,1,7)='sha256:'
    AND substr(input_manifest_digest,8) NOT GLOB '*[^0-9a-f]*'),
  CHECK(length(executor_image_digest)=71 AND substr(executor_image_digest,1,7)='sha256:'
    AND substr(executor_image_digest,8) NOT GLOB '*[^0-9a-f]*'),
  CHECK(length(target_identity_digest)=71 AND substr(target_identity_digest,1,7)='sha256:'
    AND substr(target_identity_digest,8) NOT GLOB '*[^0-9a-f]*'),
  CHECK((effect_scope='unknown' AND partial_prefix_safe=0) OR enforcement_digest IS NOT NULL),
  CHECK(enforcement_digest IS NULL OR
    (length(enforcement_digest)=71 AND substr(enforcement_digest,1,7)='sha256:'
     AND substr(enforcement_digest,8) NOT GLOB '*[^0-9a-f]*')),
  CHECK((prepare_kind='lb' AND adapter_binding_digest IS NOT NULL)
    OR (prepare_kind<>'lb' AND adapter_binding_digest IS NULL))
);

CREATE TABLE prepare_jobs (
  job_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  purpose TEXT NOT NULL DEFAULT 'prepare' CHECK(purpose IN ('prepare','cache_explain')),
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
  cache_explain_result_json TEXT,
  error_json TEXT, resolved_image_id TEXT, lineage_ref TEXT REFERENCES managed_base_lineages(lineage_ref), identity_digest TEXT,
  CHECK(purpose <> 'cache_explain' OR (prepare_kind='lb' AND plan_only=1 AND result_json IS NULL)),
  CHECK(purpose <> 'prepare' OR cache_explain_result_json IS NULL),
  CHECK(purpose <> 'cache_explain' OR status <> 'succeeded' OR cache_explain_result_json IS NOT NULL),
  CHECK(cache_explain_result_json IS NULL OR (purpose='cache_explain' AND status='succeeded')),
  CHECK(cache_explain_result_json IS NULL OR json_valid(cache_explain_result_json))
);

CREATE TABLE prepare_liquibase_cache_restores (
  job_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  session_id TEXT NOT NULL REFERENCES prepare_liquibase_sessions(session_id),
  state_id TEXT NOT NULL REFERENCES states(state_id),
  restore_phase TEXT NOT NULL CHECK(restore_phase IN ('requested','stopped','restored','published','abandoned')),
  requested_at TEXT NOT NULL,
  completed_at TEXT,
  PRIMARY KEY(job_id,task_id),
  FOREIGN KEY(job_id,task_id) REFERENCES prepare_tasks(job_id,task_id) ON DELETE CASCADE
);

CREATE TABLE prepare_liquibase_cache_selection_store_format (
 slot INTEGER PRIMARY KEY CHECK(slot=1),
 format_version INTEGER NOT NULL CHECK(format_version=1)
);

CREATE TABLE prepare_liquibase_cache_selections (
 job_id TEXT NOT NULL,
 task_id TEXT NOT NULL,
 attempt_no INTEGER NOT NULL CHECK(attempt_no>0),
 session_id TEXT NOT NULL REFERENCES prepare_liquibase_sessions(session_id),
 clone_id TEXT NOT NULL REFERENCES prepare_clones(clone_id),
 worker_generation INTEGER NOT NULL CHECK(worker_generation>0),
 execution_digest TEXT NOT NULL REFERENCES prepare_execution_bindings(execution_digest),
 target_state_id TEXT NOT NULL CHECK(length(target_state_id)>0),
 selected_at TEXT NOT NULL CHECK(coalesce(((CASE WHEN substr(selected_at,-6)='+00:00' THEN substr(selected_at,1,length(selected_at)-6)||'Z' ELSE selected_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(selected_at,-6)='+00:00' THEN substr(selected_at,1,length(selected_at)-6)||'Z' ELSE selected_at END))=20 OR (length((CASE WHEN substr(selected_at,-6)='+00:00' THEN substr(selected_at,1,length(selected_at)-6)||'Z' ELSE selected_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(selected_at,-6)='+00:00' THEN substr(selected_at,1,length(selected_at)-6)||'Z' ELSE selected_at END),20,1)='.'
AND substr((CASE WHEN substr(selected_at,-6)='+00:00' THEN substr(selected_at,1,length(selected_at)-6)||'Z' ELSE selected_at END),21,length((CASE WHEN substr(selected_at,-6)='+00:00' THEN substr(selected_at,1,length(selected_at)-6)||'Z' ELSE selected_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(selected_at,-6)='+00:00' THEN substr(selected_at,1,length(selected_at)-6)||'Z' ELSE selected_at END),1,19)||'Z'))=substr((CASE WHEN substr(selected_at,-6)='+00:00' THEN substr(selected_at,1,length(selected_at)-6)||'Z' ELSE selected_at END),1,19)||'Z'),0)),
 transferred_at TEXT CHECK(transferred_at IS NULL OR coalesce(((CASE WHEN substr(transferred_at,-6)='+00:00' THEN substr(transferred_at,1,length(transferred_at)-6)||'Z' ELSE transferred_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(transferred_at,-6)='+00:00' THEN substr(transferred_at,1,length(transferred_at)-6)||'Z' ELSE transferred_at END))=20 OR (length((CASE WHEN substr(transferred_at,-6)='+00:00' THEN substr(transferred_at,1,length(transferred_at)-6)||'Z' ELSE transferred_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(transferred_at,-6)='+00:00' THEN substr(transferred_at,1,length(transferred_at)-6)||'Z' ELSE transferred_at END),20,1)='.'
AND substr((CASE WHEN substr(transferred_at,-6)='+00:00' THEN substr(transferred_at,1,length(transferred_at)-6)||'Z' ELSE transferred_at END),21,length((CASE WHEN substr(transferred_at,-6)='+00:00' THEN substr(transferred_at,1,length(transferred_at)-6)||'Z' ELSE transferred_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(transferred_at,-6)='+00:00' THEN substr(transferred_at,1,length(transferred_at)-6)||'Z' ELSE transferred_at END),1,19)||'Z'))=substr((CASE WHEN substr(transferred_at,-6)='+00:00' THEN substr(transferred_at,1,length(transferred_at)-6)||'Z' ELSE transferred_at END),1,19)||'Z'),0)),
 successor_session_id TEXT REFERENCES prepare_liquibase_sessions(session_id),
 released_at TEXT CHECK(released_at IS NULL OR coalesce(((CASE WHEN substr(released_at,-6)='+00:00' THEN substr(released_at,1,length(released_at)-6)||'Z' ELSE released_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(released_at,-6)='+00:00' THEN substr(released_at,1,length(released_at)-6)||'Z' ELSE released_at END))=20 OR (length((CASE WHEN substr(released_at,-6)='+00:00' THEN substr(released_at,1,length(released_at)-6)||'Z' ELSE released_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(released_at,-6)='+00:00' THEN substr(released_at,1,length(released_at)-6)||'Z' ELSE released_at END),20,1)='.'
AND substr((CASE WHEN substr(released_at,-6)='+00:00' THEN substr(released_at,1,length(released_at)-6)||'Z' ELSE released_at END),21,length((CASE WHEN substr(released_at,-6)='+00:00' THEN substr(released_at,1,length(released_at)-6)||'Z' ELSE released_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(released_at,-6)='+00:00' THEN substr(released_at,1,length(released_at)-6)||'Z' ELSE released_at END),1,19)||'Z'))=substr((CASE WHEN substr(released_at,-6)='+00:00' THEN substr(released_at,1,length(released_at)-6)||'Z' ELSE released_at END),1,19)||'Z'),0)),
 release_outcome_digest TEXT,
 PRIMARY KEY(job_id,task_id,attempt_no),
 FOREIGN KEY(job_id,task_id,attempt_no) REFERENCES prepare_liquibase_invocations(job_id,task_id,attempt_no) ON DELETE CASCADE,
 CHECK(NOT(transferred_at IS NOT NULL AND released_at IS NOT NULL)),
 CHECK(successor_session_id IS NULL OR (transferred_at IS NOT NULL AND released_at IS NULL)),
 CHECK((released_at IS NULL AND release_outcome_digest IS NULL) OR
 (released_at IS NOT NULL AND release_outcome_digest IS NOT NULL
 AND length(release_outcome_digest)=71 AND substr(release_outcome_digest,1,7)='sha256:'
 AND substr(release_outcome_digest,8) NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE prepare_liquibase_capture_retirements (
 retirement_ref TEXT PRIMARY KEY NOT NULL CHECK(length(retirement_ref) BETWEEN 1 AND 128 AND substr(retirement_ref,1,1) GLOB '[a-zA-Z0-9]' AND retirement_ref NOT GLOB '*[^a-zA-Z0-9_.-]*'),
 job_id TEXT NOT NULL,
 task_id TEXT NOT NULL,
 attempt_no INTEGER NOT NULL CHECK(attempt_no>0 AND typeof(attempt_no)='integer'),
 session_id TEXT NOT NULL REFERENCES prepare_liquibase_sessions(session_id) ON DELETE CASCADE,
 reason TEXT NOT NULL CHECK(reason IN ('published_prune','session_disposal')),
 requested_at TEXT NOT NULL CHECK(coalesce(((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END))=20 OR (length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),20,1)='.'
AND substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),21,length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),1,19)||'Z'))=substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),1,19)||'Z'),0)),
 verified_at TEXT CHECK(verified_at IS NULL OR coalesce(((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END))=20 OR (length((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END),20,1)='.'
AND substr((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END),21,length((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END),1,19)||'Z'))=substr((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END),1,19)||'Z'),0)),
 absence_kind TEXT CHECK(absence_kind IN ('path_absent','container_absent')),
 UNIQUE(job_id,task_id,attempt_no),
 FOREIGN KEY(job_id,task_id,attempt_no) REFERENCES prepare_liquibase_invocations(job_id,task_id,attempt_no) ON DELETE CASCADE,
 CHECK((verified_at IS NULL AND absence_kind IS NULL) OR (verified_at IS NOT NULL AND absence_kind IS NOT NULL))
);

CREATE TABLE prepare_liquibase_capture_slots (
 session_id TEXT NOT NULL REFERENCES prepare_liquibase_sessions(session_id) ON DELETE CASCADE,
 slot_no INTEGER NOT NULL CHECK(slot_no IN (1,2) AND typeof(slot_no)='integer'),
 job_id TEXT NOT NULL,
 task_id TEXT NOT NULL,
 attempt_no INTEGER NOT NULL CHECK(attempt_no>0 AND typeof(attempt_no)='integer'),
 reserved_at TEXT NOT NULL CHECK(coalesce(((CASE WHEN substr(reserved_at,-6)='+00:00' THEN substr(reserved_at,1,length(reserved_at)-6)||'Z' ELSE reserved_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(reserved_at,-6)='+00:00' THEN substr(reserved_at,1,length(reserved_at)-6)||'Z' ELSE reserved_at END))=20 OR (length((CASE WHEN substr(reserved_at,-6)='+00:00' THEN substr(reserved_at,1,length(reserved_at)-6)||'Z' ELSE reserved_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(reserved_at,-6)='+00:00' THEN substr(reserved_at,1,length(reserved_at)-6)||'Z' ELSE reserved_at END),20,1)='.'
AND substr((CASE WHEN substr(reserved_at,-6)='+00:00' THEN substr(reserved_at,1,length(reserved_at)-6)||'Z' ELSE reserved_at END),21,length((CASE WHEN substr(reserved_at,-6)='+00:00' THEN substr(reserved_at,1,length(reserved_at)-6)||'Z' ELSE reserved_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(reserved_at,-6)='+00:00' THEN substr(reserved_at,1,length(reserved_at)-6)||'Z' ELSE reserved_at END),1,19)||'Z'))=substr((CASE WHEN substr(reserved_at,-6)='+00:00' THEN substr(reserved_at,1,length(reserved_at)-6)||'Z' ELSE reserved_at END),1,19)||'Z'),0)),
 prior_retirement_ref TEXT REFERENCES prepare_liquibase_capture_retirements(retirement_ref),
 PRIMARY KEY(session_id,slot_no),
 UNIQUE(job_id,task_id,attempt_no),
 FOREIGN KEY(job_id,task_id,attempt_no) REFERENCES prepare_liquibase_invocations(job_id,task_id,attempt_no)
 ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE prepare_liquibase_finalization_store_format (
  slot INTEGER PRIMARY KEY CHECK(slot=1),
  format_version INTEGER NOT NULL CHECK(format_version=2)
);

CREATE TABLE "prepare_liquibase_invocations" (
  job_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  attempt_no INTEGER NOT NULL CHECK(attempt_no>0),
  session_id TEXT NOT NULL REFERENCES prepare_liquibase_sessions(session_id) ON DELETE CASCADE,
  invocation_ref TEXT NOT NULL UNIQUE,
  stdout_ref TEXT NOT NULL UNIQUE,
  stderr_ref TEXT NOT NULL UNIQUE,
  manifest_ref TEXT NOT NULL UNIQUE,
  helper_digest TEXT NOT NULL,
  protocol_revision TEXT NOT NULL CHECK(protocol_revision IN ('lb-step-capture-v1','lb-step-capture-v2')),
  start_requested_at TEXT,
  exit_code INTEGER CHECK(exit_code BETWEEN 0 AND 255),
  completion_digest TEXT,
  completion_bytes INTEGER CHECK(completion_bytes>=0),
  complete_at TEXT,
  retired_at TEXT,
  PRIMARY KEY(job_id,task_id,attempt_no),
  FOREIGN KEY(job_id,task_id,attempt_no)
    REFERENCES prepare_step_attempts(job_id,task_id,attempt_no) ON DELETE CASCADE,
  CHECK(length(invocation_ref) BETWEEN 1 AND 128 AND invocation_ref NOT GLOB '*[^a-zA-Z0-9_.-]*'),
  CHECK(stdout_ref=invocation_ref||'-stdout' AND stderr_ref=invocation_ref||'-stderr'
    AND manifest_ref=invocation_ref||'-manifest'),
  CHECK((complete_at IS NULL AND exit_code IS NULL AND completion_digest IS NULL AND completion_bytes IS NULL)
     OR (complete_at IS NOT NULL AND exit_code IS NOT NULL AND completion_digest IS NOT NULL AND completion_bytes IS NOT NULL))
);

CREATE TABLE prepare_liquibase_job_cancellations (
  job_id TEXT PRIMARY KEY REFERENCES prepare_jobs(job_id) ON DELETE CASCADE,
  requested_at TEXT NOT NULL CHECK(coalesce(((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END))=20 OR (length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),20,1)='.'
AND substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),21,length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),1,19)||'Z'))=substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),1,19)||'Z'),0))
);

CREATE TABLE prepare_liquibase_job_finalizations (
  job_id TEXT PRIMARY KEY REFERENCES prepare_jobs(job_id) ON DELETE CASCADE,
  format_revision INTEGER NOT NULL CHECK(format_revision=1),
  outcome_kind TEXT NOT NULL CHECK(outcome_kind IN ('prepare_success','plan_success','cache_explain_success','failure','cancelled')),
  event_profile TEXT NOT NULL CHECK(
    (outcome_kind='prepare_success' AND event_profile='status_result') OR
    (outcome_kind='plan_success' AND event_profile='status') OR
    (outcome_kind='cache_explain_success' AND event_profile='result') OR
    (outcome_kind IN ('failure','cancelled') AND event_profile IN ('error','status_error'))),
  instance_id TEXT,
  instance_pin TEXT REFERENCES instances(instance_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  outcome_json TEXT NOT NULL CHECK(json_valid(outcome_json) AND json_type(outcome_json)='object'),
  outcome_digest TEXT NOT NULL CHECK(length(outcome_digest)=71 AND substr(outcome_digest,1,7)='sha256:' AND substr(outcome_digest,8) NOT GLOB '*[^0-9a-f]*'),
  final_state_id TEXT,
  created_at TEXT NOT NULL CHECK(coalesce(((CASE WHEN substr(created_at,-6)='+00:00' THEN substr(created_at,1,length(created_at)-6)||'Z' ELSE created_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(created_at,-6)='+00:00' THEN substr(created_at,1,length(created_at)-6)||'Z' ELSE created_at END))=20 OR (length((CASE WHEN substr(created_at,-6)='+00:00' THEN substr(created_at,1,length(created_at)-6)||'Z' ELSE created_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(created_at,-6)='+00:00' THEN substr(created_at,1,length(created_at)-6)||'Z' ELSE created_at END),20,1)='.'
AND substr((CASE WHEN substr(created_at,-6)='+00:00' THEN substr(created_at,1,length(created_at)-6)||'Z' ELSE created_at END),21,length((CASE WHEN substr(created_at,-6)='+00:00' THEN substr(created_at,1,length(created_at)-6)||'Z' ELSE created_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(created_at,-6)='+00:00' THEN substr(created_at,1,length(created_at)-6)||'Z' ELSE created_at END),1,19)||'Z'))=substr((CASE WHEN substr(created_at,-6)='+00:00' THEN substr(created_at,1,length(created_at)-6)||'Z' ELSE created_at END),1,19)||'Z'),0)),
  projections_removed_at TEXT CHECK(projections_removed_at IS NULL OR coalesce(((CASE WHEN substr(projections_removed_at,-6)='+00:00' THEN substr(projections_removed_at,1,length(projections_removed_at)-6)||'Z' ELSE projections_removed_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(projections_removed_at,-6)='+00:00' THEN substr(projections_removed_at,1,length(projections_removed_at)-6)||'Z' ELSE projections_removed_at END))=20 OR (length((CASE WHEN substr(projections_removed_at,-6)='+00:00' THEN substr(projections_removed_at,1,length(projections_removed_at)-6)||'Z' ELSE projections_removed_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(projections_removed_at,-6)='+00:00' THEN substr(projections_removed_at,1,length(projections_removed_at)-6)||'Z' ELSE projections_removed_at END),20,1)='.'
AND substr((CASE WHEN substr(projections_removed_at,-6)='+00:00' THEN substr(projections_removed_at,1,length(projections_removed_at)-6)||'Z' ELSE projections_removed_at END),21,length((CASE WHEN substr(projections_removed_at,-6)='+00:00' THEN substr(projections_removed_at,1,length(projections_removed_at)-6)||'Z' ELSE projections_removed_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(projections_removed_at,-6)='+00:00' THEN substr(projections_removed_at,1,length(projections_removed_at)-6)||'Z' ELSE projections_removed_at END),1,19)||'Z'))=substr((CASE WHEN substr(projections_removed_at,-6)='+00:00' THEN substr(projections_removed_at,1,length(projections_removed_at)-6)||'Z' ELSE projections_removed_at END),1,19)||'Z'),0)),
  CHECK((outcome_kind='prepare_success' AND instance_id IS NOT NULL AND instance_id<>''
    AND final_state_id IS NOT NULL AND final_state_id<>''
    AND (instance_pin IS NULL OR instance_pin=instance_id)) OR
    (outcome_kind<>'prepare_success' AND instance_id IS NULL AND instance_pin IS NULL AND final_state_id IS NULL))
);

CREATE TABLE prepare_liquibase_planning_capture_disposals (
 job_id TEXT NOT NULL,
 task_id TEXT NOT NULL CHECK(task_id='plan'),
 attempt_no INTEGER NOT NULL CHECK(attempt_no>0 AND typeof(attempt_no)='integer'),
 reason TEXT NOT NULL CHECK(reason IN ('plan_saved','selected_failure','accepted_cancel','common_discard')),
 authority_ref TEXT NOT NULL CHECK(length(authority_ref)>0),
 requested_at TEXT NOT NULL CHECK(coalesce(((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END))=20 OR (length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),20,1)='.'
AND substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),21,length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),1,19)||'Z'))=substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),1,19)||'Z'),0)),
 PRIMARY KEY(job_id,task_id,attempt_no),
 FOREIGN KEY(job_id,task_id,attempt_no) REFERENCES prepare_planning_attempt_artifacts(job_id,task_id,attempt_no) ON DELETE CASCADE
);

CREATE TABLE prepare_liquibase_session_disposals (
 session_id TEXT PRIMARY KEY NOT NULL REFERENCES prepare_liquibase_sessions(session_id) ON DELETE CASCADE,
 reason TEXT NOT NULL CHECK(reason IN ('final_published','cache_selected','selected_failure','accepted_cancel','unknown_discard','proven_nonstart','instance_transferred')),
 authority_ref TEXT NOT NULL CHECK(length(authority_ref)>0),
 requested_at TEXT NOT NULL CHECK(coalesce(((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END))=20 OR (length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),20,1)='.'
AND substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),21,length((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),1,19)||'Z'))=substr((CASE WHEN substr(requested_at,-6)='+00:00' THEN substr(requested_at,1,length(requested_at)-6)||'Z' ELSE requested_at END),1,19)||'Z'),0)),
 runner_id TEXT CHECK(runner_id IS NULL OR (length(runner_id)=64 AND runner_id NOT GLOB '*[^0-9a-f]*')),
 db_runtime_id TEXT CHECK(db_runtime_id IS NULL OR (length(db_runtime_id)=64 AND db_runtime_id NOT GLOB '*[^0-9a-f]*')),
 db_absent_at TEXT CHECK(db_absent_at IS NULL OR coalesce(((CASE WHEN substr(db_absent_at,-6)='+00:00' THEN substr(db_absent_at,1,length(db_absent_at)-6)||'Z' ELSE db_absent_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(db_absent_at,-6)='+00:00' THEN substr(db_absent_at,1,length(db_absent_at)-6)||'Z' ELSE db_absent_at END))=20 OR (length((CASE WHEN substr(db_absent_at,-6)='+00:00' THEN substr(db_absent_at,1,length(db_absent_at)-6)||'Z' ELSE db_absent_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(db_absent_at,-6)='+00:00' THEN substr(db_absent_at,1,length(db_absent_at)-6)||'Z' ELSE db_absent_at END),20,1)='.'
AND substr((CASE WHEN substr(db_absent_at,-6)='+00:00' THEN substr(db_absent_at,1,length(db_absent_at)-6)||'Z' ELSE db_absent_at END),21,length((CASE WHEN substr(db_absent_at,-6)='+00:00' THEN substr(db_absent_at,1,length(db_absent_at)-6)||'Z' ELSE db_absent_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(db_absent_at,-6)='+00:00' THEN substr(db_absent_at,1,length(db_absent_at)-6)||'Z' ELSE db_absent_at END),1,19)||'Z'))=substr((CASE WHEN substr(db_absent_at,-6)='+00:00' THEN substr(db_absent_at,1,length(db_absent_at)-6)||'Z' ELSE db_absent_at END),1,19)||'Z'),0)),
 runner_absent_at TEXT CHECK(runner_absent_at IS NULL OR coalesce(((CASE WHEN substr(runner_absent_at,-6)='+00:00' THEN substr(runner_absent_at,1,length(runner_absent_at)-6)||'Z' ELSE runner_absent_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(runner_absent_at,-6)='+00:00' THEN substr(runner_absent_at,1,length(runner_absent_at)-6)||'Z' ELSE runner_absent_at END))=20 OR (length((CASE WHEN substr(runner_absent_at,-6)='+00:00' THEN substr(runner_absent_at,1,length(runner_absent_at)-6)||'Z' ELSE runner_absent_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(runner_absent_at,-6)='+00:00' THEN substr(runner_absent_at,1,length(runner_absent_at)-6)||'Z' ELSE runner_absent_at END),20,1)='.'
AND substr((CASE WHEN substr(runner_absent_at,-6)='+00:00' THEN substr(runner_absent_at,1,length(runner_absent_at)-6)||'Z' ELSE runner_absent_at END),21,length((CASE WHEN substr(runner_absent_at,-6)='+00:00' THEN substr(runner_absent_at,1,length(runner_absent_at)-6)||'Z' ELSE runner_absent_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(runner_absent_at,-6)='+00:00' THEN substr(runner_absent_at,1,length(runner_absent_at)-6)||'Z' ELSE runner_absent_at END),1,19)||'Z'))=substr((CASE WHEN substr(runner_absent_at,-6)='+00:00' THEN substr(runner_absent_at,1,length(runner_absent_at)-6)||'Z' ELSE runner_absent_at END),1,19)||'Z'),0)),
 nonstart_basis TEXT CHECK(nonstart_basis IN ('session_uninvoked','attempt')),
 nonstart_scope_digest TEXT,
 nonstart_observation_digest TEXT,
 nonstart_observed_at TEXT,
 handoff_job_id TEXT REFERENCES prepare_liquibase_job_finalizations(job_id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
 handoff_instance_id TEXT,
 handoff_state_id TEXT,
 handoff_operation_ref TEXT,
 handoff_runtime_dir TEXT,
 handoff_outcome_digest TEXT,
 CHECK((reason='instance_transferred' AND handoff_job_id IS NOT NULL AND length(handoff_job_id)>0
 AND handoff_instance_id IS NOT NULL AND length(handoff_instance_id)>0
 AND handoff_state_id IS NOT NULL AND length(handoff_state_id)>0
 AND handoff_operation_ref IS NOT NULL AND length(handoff_operation_ref)>0
 AND handoff_runtime_dir IS NOT NULL AND length(handoff_runtime_dir)>0
 AND handoff_outcome_digest IS NOT NULL AND length(handoff_outcome_digest)=71
 AND substr(handoff_outcome_digest,1,7)='sha256:' AND substr(handoff_outcome_digest,8) NOT GLOB '*[^0-9a-f]*'
 AND authority_ref=handoff_outcome_digest AND runner_id IS NOT NULL AND db_runtime_id IS NOT NULL AND db_absent_at IS NULL)
 OR (reason<>'instance_transferred' AND handoff_job_id IS NULL AND handoff_instance_id IS NULL
 AND handoff_state_id IS NULL AND handoff_operation_ref IS NULL AND handoff_runtime_dir IS NULL AND handoff_outcome_digest IS NULL)),
 CHECK((reason='proven_nonstart' AND nonstart_basis IS NOT NULL AND nonstart_scope_digest IS NOT NULL
   AND nonstart_observation_digest IS NOT NULL AND nonstart_observed_at IS NOT NULL
   AND length(nonstart_scope_digest)=71 AND substr(nonstart_scope_digest,1,7)='sha256:' AND substr(nonstart_scope_digest,8) NOT GLOB '*[^0-9a-f]*' AND length(nonstart_observation_digest)=71 AND substr(nonstart_observation_digest,1,7)='sha256:' AND substr(nonstart_observation_digest,8) NOT GLOB '*[^0-9a-f]*' AND coalesce(((CASE WHEN substr(nonstart_observed_at,-6)='+00:00' THEN substr(nonstart_observed_at,1,length(nonstart_observed_at)-6)||'Z' ELSE nonstart_observed_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(nonstart_observed_at,-6)='+00:00' THEN substr(nonstart_observed_at,1,length(nonstart_observed_at)-6)||'Z' ELSE nonstart_observed_at END))=20 OR (length((CASE WHEN substr(nonstart_observed_at,-6)='+00:00' THEN substr(nonstart_observed_at,1,length(nonstart_observed_at)-6)||'Z' ELSE nonstart_observed_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(nonstart_observed_at,-6)='+00:00' THEN substr(nonstart_observed_at,1,length(nonstart_observed_at)-6)||'Z' ELSE nonstart_observed_at END),20,1)='.'
AND substr((CASE WHEN substr(nonstart_observed_at,-6)='+00:00' THEN substr(nonstart_observed_at,1,length(nonstart_observed_at)-6)||'Z' ELSE nonstart_observed_at END),21,length((CASE WHEN substr(nonstart_observed_at,-6)='+00:00' THEN substr(nonstart_observed_at,1,length(nonstart_observed_at)-6)||'Z' ELSE nonstart_observed_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(nonstart_observed_at,-6)='+00:00' THEN substr(nonstart_observed_at,1,length(nonstart_observed_at)-6)||'Z' ELSE nonstart_observed_at END),1,19)||'Z'))=substr((CASE WHEN substr(nonstart_observed_at,-6)='+00:00' THEN substr(nonstart_observed_at,1,length(nonstart_observed_at)-6)||'Z' ELSE nonstart_observed_at END),1,19)||'Z'),0))
 OR (reason<>'proven_nonstart' AND nonstart_basis IS NULL AND nonstart_scope_digest IS NULL
   AND nonstart_observation_digest IS NULL AND nonstart_observed_at IS NULL)),
 CHECK(runner_absent_at IS NULL OR db_absent_at IS NOT NULL OR reason='instance_transferred')
);

CREATE TABLE prepare_liquibase_session_readiness (
 session_id TEXT PRIMARY KEY NOT NULL REFERENCES prepare_liquibase_sessions(session_id) ON DELETE CASCADE,
 runner_id TEXT NOT NULL CHECK(length(runner_id)=64 AND runner_id NOT GLOB '*[^0-9a-f]*'),
 ready_digest TEXT NOT NULL CHECK(length(ready_digest)=71 AND substr(ready_digest,1,7)='sha256:' AND substr(ready_digest,8) NOT GLOB '*[^0-9a-f]*'),
 ready_bytes INTEGER NOT NULL CHECK(ready_bytes BETWEEN 1 AND 8192 AND typeof(ready_bytes)='integer'),
 verified_at TEXT NOT NULL CHECK(coalesce(((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z'
AND (length((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END))=20 OR (length((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END)) BETWEEN 22 AND 30
AND substr((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END),20,1)='.'
AND substr((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END),21,length((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END))-21) NOT GLOB '*[^0-9]*'))
AND strftime('%Y-%m-%dT%H:%M:%SZ',julianday(substr((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END),1,19)||'Z'))=substr((CASE WHEN substr(verified_at,-6)='+00:00' THEN substr(verified_at,1,length(verified_at)-6)||'Z' ELSE verified_at END),1,19)||'Z'),0))
);

CREATE TABLE "prepare_liquibase_session_store_format" (
  slot INTEGER PRIMARY KEY CHECK(slot=1),
  format_version INTEGER NOT NULL CHECK(format_version=3)
);

CREATE TABLE prepare_liquibase_sessions (
  session_id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES prepare_jobs(job_id) ON DELETE CASCADE,
  session_no INTEGER NOT NULL CHECK(session_no>0),
  clone_id TEXT NOT NULL UNIQUE REFERENCES prepare_clones(clone_id),
  worker_generation INTEGER NOT NULL CHECK(worker_generation>0),
  execution_fingerprint TEXT NOT NULL REFERENCES liquibase_execution_bindings(execution_fingerprint),
  start_kind TEXT NOT NULL CHECK(start_kind IN ('image','state')),
  start_id TEXT NOT NULL,
  start_position INTEGER NOT NULL CHECK(start_position>=0),
  planned_runner_name TEXT NOT NULL UNIQUE,
  runner_config_digest TEXT NOT NULL,
  capture_helper_digest TEXT NOT NULL,
  protected_config_ref TEXT NOT NULL,
  runner_id TEXT UNIQUE CHECK(runner_id IS NULL OR
    (length(runner_id)=64 AND runner_id NOT GLOB '*[^0-9a-f]*')),
  runner_phase TEXT NOT NULL CHECK(runner_phase IN ('planned','create_requested','created','start_requested','running','stop_requested','stop_acknowledged','stopped','retired')),
  db_phase TEXT NOT NULL CHECK(db_phase IN ('running','stopped','restart_requested')),
  cursor_image_id TEXT,
  cursor_state_id TEXT REFERENCES states(state_id),
  cursor_position INTEGER NOT NULL CHECK(cursor_position>=0),
  created_at TEXT NOT NULL,
  retired_at TEXT, capture_location TEXT NOT NULL DEFAULT 'host_shared', capture_protocol_revision TEXT NOT NULL DEFAULT 'lb-step-capture-v1',
  UNIQUE(job_id,session_no),
  CHECK((retired_at IS NULL AND ((cursor_image_id IS NOT NULL AND cursor_state_id IS NULL) OR
        (cursor_image_id IS NULL AND cursor_state_id IS NOT NULL))) OR
        (retired_at IS NOT NULL AND cursor_image_id IS NULL AND cursor_state_id IS NULL))
);

CREATE TABLE prepare_planning_attempt_artifacts (
  job_id TEXT NOT NULL,
  task_id TEXT NOT NULL CHECK(task_id='plan'),
  attempt_no INTEGER NOT NULL CHECK(attempt_no>0),
  protected_input_ref TEXT NOT NULL UNIQUE,
  complete_output_ref TEXT NOT NULL UNIQUE,
  complete_stderr_ref TEXT NOT NULL UNIQUE,
  capture_manifest_ref TEXT NOT NULL UNIQUE,
  capture_helper_digest TEXT NOT NULL,
  capture_protocol_revision TEXT NOT NULL CHECK(capture_protocol_revision<>''),
  output_digest TEXT,
  output_bytes INTEGER,
  output_complete_at TEXT,
  executor_retired_at TEXT,
  protected_input_retired_at TEXT,
  output_retired_at TEXT,
  stderr_retired_at TEXT,
  manifest_retired_at TEXT, capture_location TEXT NOT NULL DEFAULT 'host_shared',
  PRIMARY KEY(job_id,task_id,attempt_no),
  FOREIGN KEY(job_id,task_id,attempt_no)
    REFERENCES prepare_step_attempts(job_id,task_id,attempt_no) ON DELETE CASCADE,
  CHECK((output_digest IS NULL AND output_bytes IS NULL AND output_complete_at IS NULL)
    OR (output_digest IS NOT NULL AND output_bytes IS NOT NULL
      AND output_bytes>=0 AND output_complete_at IS NOT NULL)),
	CHECK(protected_input_ref<>complete_output_ref AND protected_input_ref<>complete_stderr_ref
	  AND protected_input_ref<>capture_manifest_ref AND complete_output_ref<>complete_stderr_ref
	  AND complete_output_ref<>capture_manifest_ref AND complete_stderr_ref<>capture_manifest_ref),
  CHECK(length(capture_helper_digest)=71 AND substr(capture_helper_digest,1,7)='sha256:'
    AND substr(capture_helper_digest,8) NOT GLOB '*[^0-9a-f]*'),
  CHECK(output_digest IS NULL OR (length(output_digest)=71 AND substr(output_digest,1,7)='sha256:'
    AND substr(output_digest,8) NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE prepare_planning_changesets (
  job_id TEXT NOT NULL REFERENCES prepare_planning_intents(job_id) ON DELETE CASCADE,
  ordinal INTEGER NOT NULL CHECK(ordinal>=0),
  id TEXT NOT NULL,
  author TEXT NOT NULL,
  path TEXT NOT NULL,
  sql_hash TEXT NOT NULL CHECK(length(sql_hash)=64 AND sql_hash NOT GLOB '*[^0-9a-f]*'),
  checksum TEXT,
  PRIMARY KEY(job_id,ordinal)
);

CREATE TABLE prepare_planning_intents (
  job_id TEXT PRIMARY KEY REFERENCES prepare_jobs(job_id) ON DELETE CASCADE,
  task_id TEXT NOT NULL CHECK(task_id='plan'),
  execution_digest TEXT NOT NULL REFERENCES prepare_execution_bindings(execution_digest),
  preview_clone_id TEXT NOT NULL UNIQUE REFERENCES prepare_clones(clone_id),
  result_attempt_no INTEGER,
  plan_codec_revision TEXT NOT NULL CHECK(plan_codec_revision<>''),
  plan_digest TEXT,
  plan_count INTEGER,
  parsed_output_digest TEXT,
  plan_saved_at TEXT,
  cache_decision_json TEXT,
  cache_decision_digest TEXT,
  decision_saved_at TEXT,
  plan_projection_json TEXT,
  plan_projection_digest TEXT,
  plan_projection_saved_at TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(job_id,task_id) REFERENCES prepare_tasks(job_id,task_id),
  FOREIGN KEY(job_id,task_id,result_attempt_no)
    REFERENCES prepare_step_attempts(job_id,task_id,attempt_no),
  CHECK((result_attempt_no IS NULL AND plan_digest IS NULL AND plan_count IS NULL
      AND parsed_output_digest IS NULL AND plan_saved_at IS NULL)
    OR (result_attempt_no IS NOT NULL AND plan_digest IS NOT NULL
      AND plan_count IS NOT NULL AND plan_count>=0
      AND parsed_output_digest IS NOT NULL AND plan_saved_at IS NOT NULL)),
  CHECK((cache_decision_json IS NULL AND cache_decision_digest IS NULL AND decision_saved_at IS NULL)
    OR (cache_decision_json IS NOT NULL AND cache_decision_digest IS NOT NULL
      AND decision_saved_at IS NOT NULL AND plan_saved_at IS NOT NULL)),
  CHECK(cache_decision_json IS NULL OR json_valid(cache_decision_json)),
  CHECK((plan_projection_json IS NULL AND plan_projection_digest IS NULL AND plan_projection_saved_at IS NULL)
    OR (plan_projection_json IS NOT NULL AND plan_projection_digest IS NOT NULL
      AND plan_projection_saved_at IS NOT NULL AND plan_saved_at IS NOT NULL)),
  CHECK(plan_projection_json IS NULL OR json_valid(plan_projection_json)),
  CHECK(plan_digest IS NULL OR (length(plan_digest)=71 AND substr(plan_digest,1,7)='sha256:'
    AND substr(plan_digest,8) NOT GLOB '*[^0-9a-f]*')),
  CHECK(parsed_output_digest IS NULL OR
    (length(parsed_output_digest)=71 AND substr(parsed_output_digest,1,7)='sha256:'
    AND substr(parsed_output_digest,8) NOT GLOB '*[^0-9a-f]*')),
  CHECK(cache_decision_digest IS NULL OR
    (length(cache_decision_digest)=71 AND substr(cache_decision_digest,1,7)='sha256:'
    AND substr(cache_decision_digest,8) NOT GLOB '*[^0-9a-f]*')),
  CHECK(plan_projection_digest IS NULL OR
    (length(plan_projection_digest)=71 AND substr(plan_projection_digest,1,7)='sha256:'
    AND substr(plan_projection_digest,8) NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE prepare_state_bindings (
  state_id TEXT PRIMARY KEY REFERENCES states(state_id) ON DELETE CASCADE,
  execution_digest TEXT NOT NULL REFERENCES prepare_execution_bindings(execution_digest)
);

CREATE TABLE "prepare_step_attempts" (
  job_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  attempt_no INTEGER NOT NULL CHECK(attempt_no>0),
  clone_id TEXT REFERENCES prepare_clones(clone_id),
  execution_digest TEXT REFERENCES prepare_execution_bindings(execution_digest),
  worker_generation INTEGER NOT NULL CHECK(worker_generation>=0),
  executor_kind TEXT NOT NULL DEFAULT 'container' CHECK(executor_kind IN ('container','session_exec')),
  planned_executor_name TEXT UNIQUE,
  executor_id TEXT,
  phase TEXT NOT NULL CHECK(phase IN ('planned','create_requested','created','start_requested','observed_stopped','exec_requested','exec_completed','closed')),
  resolution TEXT CHECK(resolution IN ('never_started','published','no_effects','retry_safe_partial','discarded_clone','planning_result_saved')),
  legacy_unbound INTEGER NOT NULL DEFAULT 0 CHECK(legacy_unbound IN (0,1)),
  created_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT,
  PRIMARY KEY(job_id,task_id,attempt_no),
  FOREIGN KEY(job_id,task_id) REFERENCES prepare_tasks(job_id,task_id) ON DELETE CASCADE,
  CHECK(legacy_unbound=1 OR (clone_id IS NOT NULL AND execution_digest IS NOT NULL))
);

CREATE TABLE prepare_step_bindings (
  job_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  execution_digest TEXT NOT NULL REFERENCES prepare_execution_bindings(execution_digest),
  bound_at TEXT NOT NULL,
  PRIMARY KEY(job_id,task_id),
  FOREIGN KEY(job_id,task_id) REFERENCES prepare_tasks(job_id,task_id) ON DELETE CASCADE
);

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

CREATE INDEX idx_liquibase_job_fingerprint ON liquibase_job_bindings(execution_fingerprint);

CREATE INDEX idx_liquibase_state_fingerprint ON liquibase_state_bindings(execution_fingerprint);

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

CREATE INDEX prepare_artifact_lease_expiry ON prepare_artifact_leases(expires_at);

CREATE UNIQUE INDEX prepare_attempt_unresolved_unique
ON prepare_step_attempts(job_id,task_id)
WHERE resolution IS NULL AND legacy_unbound=0;

CREATE UNIQUE INDEX prepare_clone_active_unique ON prepare_clones(job_id)
WHERE phase IN ('allocated','active','quiescent');

CREATE INDEX prepare_lb_cache_selection_active_target ON prepare_liquibase_cache_selections(target_state_id)
WHERE transferred_at IS NULL AND released_at IS NULL;

CREATE INDEX prepare_lb_finalization_instance_pin ON prepare_liquibase_job_finalizations(instance_pin) WHERE instance_pin IS NOT NULL;

CREATE UNIQUE INDEX prepare_liquibase_one_live_session
ON prepare_liquibase_sessions(job_id) WHERE retired_at IS NULL;

CREATE UNIQUE INDEX prepare_liquibase_one_pending_restore ON prepare_liquibase_cache_restores(session_id) WHERE restore_phase NOT IN ('published','abandoned');

CREATE UNIQUE INDEX prepare_liquibase_one_unresolved_invocation
ON prepare_liquibase_invocations(session_id) WHERE complete_at IS NULL AND retired_at IS NULL;

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

CREATE TRIGGER liquibase_execution_immutable
BEFORE UPDATE ON liquibase_execution_bindings
BEGIN SELECT RAISE(ABORT, 'Liquibase execution binding is immutable'); END;

CREATE TRIGGER liquibase_job_bind_once
BEFORE UPDATE ON liquibase_job_bindings
WHEN OLD.execution_fingerprint IS NOT NULL
  OR NEW.execution_fingerprint IS NULL
  OR NEW.requested_tag IS NOT OLD.requested_tag
  OR NEW.bundle_manifest_digest IS NOT OLD.bundle_manifest_digest
  OR NOT EXISTS (
    SELECT 1 FROM liquibase_execution_bindings e
    WHERE e.execution_fingerprint = NEW.execution_fingerprint
      AND e.requested_tag = NEW.requested_tag
      AND e.bundle_manifest_digest = NEW.bundle_manifest_digest)
BEGIN SELECT RAISE(ABORT, 'invalid Liquibase binding transition'); END;

CREATE TRIGGER liquibase_job_initial_unbound
BEFORE INSERT ON liquibase_job_bindings
WHEN NEW.execution_fingerprint IS NOT NULL OR NEW.bound_at IS NOT NULL
BEGIN SELECT RAISE(ABORT, 'Liquibase job must be admitted unbound'); END;

CREATE TRIGGER liquibase_job_kind_guard
BEFORE INSERT ON liquibase_job_bindings
WHEN NOT EXISTS (SELECT 1 FROM prepare_jobs WHERE job_id = NEW.job_id AND prepare_kind = 'lb')
BEGIN SELECT RAISE(ABORT, 'Liquibase binding requires lb job'); END;

CREATE TRIGGER liquibase_state_kind_guard
BEFORE INSERT ON liquibase_state_bindings
WHEN NOT EXISTS (SELECT 1 FROM states WHERE state_id = NEW.state_id AND prepare_kind = 'lb')
BEGIN SELECT RAISE(ABORT, 'Liquibase state binding requires lb state'); END;

CREATE TRIGGER managed_job_immutable BEFORE UPDATE OF resolved_image_id,lineage_ref,identity_digest ON prepare_jobs
WHEN OLD.resolved_image_id IS NOT NEW.resolved_image_id
 OR OLD.lineage_ref IS NOT NEW.lineage_ref OR OLD.identity_digest IS NOT NEW.identity_digest
BEGIN
 SELECT CASE WHEN NOT (
   OLD.purpose='cache_explain' AND OLD.status IN ('queued','running')
   AND OLD.resolved_image_id IS NULL AND OLD.lineage_ref IS NULL AND OLD.identity_digest IS NULL
   AND NEW.resolved_image_id IS NOT NULL AND NEW.lineage_ref IS NOT NULL
   AND NEW.identity_digest IS NOT NULL
   AND EXISTS (SELECT 1 FROM managed_base_lineages l
     JOIN managed_store_format f ON f.domain_ref=l.domain_ref AND f.slot=1
     WHERE l.lineage_ref=NEW.lineage_ref AND l.identity_digest=NEW.identity_digest
       AND (NEW.resolved_image_id=l.image_digest
         OR substr(NEW.resolved_image_id,-72)='@'||l.image_digest))
 ) THEN RAISE(ABORT,'managed job binding is immutable') END;
END;

CREATE TRIGGER managed_job_insert BEFORE INSERT ON prepare_jobs
WHEN NOT (NEW.purpose='cache_explain' AND NEW.status='queued'
          AND NEW.resolved_image_id IS NULL AND NEW.lineage_ref IS NULL
          AND NEW.identity_digest IS NULL)
 AND NOT EXISTS (
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

CREATE TRIGGER prepare_artifact_job_lease_conversion_guard
BEFORE UPDATE OF owner_kind,owner_id,expires_at ON prepare_artifact_leases
WHEN OLD.owner_kind='job' AND EXISTS (
  SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.owner_id
    AND (j.status NOT IN ('succeeded','failed') OR EXISTS (
      SELECT 1 FROM prepare_step_attempts a
      WHERE a.job_id=j.job_id AND a.resolution IS NULL) OR EXISTS (
      SELECT 1 FROM prepare_clones c
      WHERE c.job_id=j.job_id AND c.phase<>'released')))
BEGIN SELECT RAISE(ABORT,'recoverable job artifact lease cannot expire'); END;

CREATE TRIGGER prepare_artifact_job_lease_delete_guard
BEFORE DELETE ON prepare_artifact_leases
WHEN OLD.owner_kind='job' AND EXISTS (
  SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.owner_id
    AND (j.status NOT IN ('succeeded','failed') OR EXISTS (
      SELECT 1 FROM prepare_step_attempts a
      WHERE a.job_id=j.job_id AND a.resolution IS NULL) OR EXISTS (
      SELECT 1 FROM prepare_clones c
      WHERE c.job_id=j.job_id AND c.phase<>'released')))
BEGIN SELECT RAISE(ABORT,'recoverable job artifact lease is retained'); END;

CREATE TRIGGER prepare_artifact_lease_identity_immutable
BEFORE UPDATE OF lease_id,artifact_kind,artifact_digest,created_at ON prepare_artifact_leases
BEGIN SELECT RAISE(ABORT,'artifact lease identity is immutable'); END;

CREATE TRIGGER prepare_attempt_decision_delete_guard BEFORE DELETE ON prepare_attempt_decisions
WHEN EXISTS (SELECT 1 FROM prepare_jobs WHERE job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'attempt decision is retained'); END;

CREATE TRIGGER prepare_attempt_decision_generation_guard BEFORE INSERT ON prepare_attempt_decisions
WHEN NOT EXISTS (SELECT 1 FROM prepare_step_attempts a
  WHERE a.job_id=NEW.job_id AND a.task_id=NEW.task_id
    AND a.attempt_no=NEW.attempt_no AND a.worker_generation=NEW.worker_generation)
BEGIN SELECT RAISE(ABORT,'stale attempt decision generation'); END;

CREATE TRIGGER prepare_attempt_decision_immutable BEFORE UPDATE ON prepare_attempt_decisions
BEGIN SELECT RAISE(ABORT,'attempt decision is append-only'); END;

CREATE TRIGGER prepare_attempt_delete_guard BEFORE DELETE ON prepare_step_attempts
WHEN EXISTS (SELECT 1 FROM prepare_jobs WHERE job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'attempt history is retained with its job'); END;

CREATE TRIGGER prepare_attempt_discarded_clone_guard
BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN NEW.executor_kind='container' AND NEW.resolution='discarded_clone' AND NOT (NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=NEW.job_id)
AND OLD.resolution IS NULL AND NEW.phase='closed'
AND EXISTS (SELECT 1 FROM prepare_clones c WHERE c.clone_id=NEW.clone_id AND c.job_id=NEW.job_id
 AND (
NEW.executor_kind='container' AND NEW.task_id='plan' AND NEW.legacy_unbound=0
AND NEW.attempt_no IN (1,2)
AND EXISTS (SELECT 1 FROM prepare_jobs j JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
 JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
 JOIN prepare_execution_bindings b ON b.execution_digest=NEW.execution_digest
 WHERE j.job_id=NEW.job_id AND j.prepare_kind='lb' AND j.purpose IN ('prepare','cache_explain')
 AND b.prepare_kind='lb' AND b.policy_revision='lb-container-v2'
 AND lb.execution_policy_revision=b.policy_revision AND lb.execution_fingerprint=b.adapter_binding_digest
 AND lb.bundle_manifest_digest=b.input_manifest_digest)
AND (SELECT count(*) FROM prepare_attempt_evidence unique_proof WHERE unique_proof.job_id=NEW.job_id
 AND unique_proof.task_id=NEW.task_id AND unique_proof.attempt_no=NEW.attempt_no
 AND unique_proof.kind='cancelled_planning_disposal')=1
AND EXISTS (SELECT 1 FROM prepare_attempt_evidence p
 JOIN prepare_liquibase_job_cancellations request ON request.job_id=p.job_id
 WHERE p.job_id=NEW.job_id AND p.task_id=NEW.task_id AND p.attempt_no=NEW.attempt_no
 AND p.worker_generation=NEW.worker_generation AND p.kind='cancelled_planning_disposal'
 AND json_extract(p.facts_json,'$.clone_id')=NEW.clone_id
 AND json_extract(p.facts_json,'$.execution_digest')=NEW.execution_digest
 AND json_extract(p.facts_json,'$.observation.clone_generation')=c.clone_generation
 AND json_extract(p.facts_json,'$.observation.executor_name')=NEW.planned_executor_name
 AND json_extract(p.facts_json,'$.observation.executor_id')=coalesce(NEW.executor_id,'')
 AND json_extract(p.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
 AND json_extract(p.facts_json,'$.observation.runtime_id')=coalesce(c.db_runtime_id,'')
 AND json_extract(p.facts_json,'$.observation.requested_at')=request.requested_at
 AND json_extract(p.facts_json,'$.observation.finished_at')=NEW.finished_at
 AND json_extract(p.facts_json,'$.observation.executor_absent')=1
 AND json_extract(p.facts_json,'$.observation.runtime_absent')=1
 AND ((json_extract(p.facts_json,'$.observation.phase')='planned' AND NEW.executor_id IS NULL AND NEW.started_at IS NULL
       AND ((json_extract(p.facts_json,'$.observation.clone_phase')='allocated' AND c.db_runtime_id IS NULL)
         OR (json_extract(p.facts_json,'$.observation.clone_phase')='active' AND c.db_runtime_id IS NOT NULL)))
   OR (json_extract(p.facts_json,'$.observation.phase')='created' AND NEW.executor_id IS NOT NULL AND NEW.started_at IS NULL
       AND c.db_runtime_id IS NOT NULL AND json_extract(p.facts_json,'$.observation.clone_phase')='active')
   OR (json_extract(p.facts_json,'$.observation.phase')='observed_stopped'
       AND NEW.executor_id IS NOT NULL AND c.db_runtime_id IS NOT NULL
       AND json_extract(p.facts_json,'$.observation.clone_phase') IN ('active','quiescent')
       AND EXISTS (SELECT 1 FROM prepare_attempt_evidence exit
        WHERE exit.job_id=NEW.job_id AND exit.task_id=NEW.task_id AND exit.attempt_no=NEW.attempt_no
        AND exit.worker_generation=NEW.worker_generation AND exit.kind='executor_inspection'
        AND exit.evidence_id<p.evidence_id
        AND json_extract(exit.facts_json,'$.clone_id')=NEW.clone_id
        AND json_extract(exit.facts_json,'$.execution_digest')=NEW.execution_digest
        AND json_extract(exit.facts_json,'$.observation.executor_id')=NEW.executor_id
        AND json_extract(exit.facts_json,'$.observation.exit_code') BETWEEN 0 AND 255
        AND (NOT EXISTS (SELECT 1 FROM prepare_attempt_evidence req
          WHERE req.job_id=NEW.job_id AND req.task_id=NEW.task_id AND req.attempt_no=NEW.attempt_no
          AND req.kind IN ('termination_requested','termination_acknowledged'))
         OR EXISTS (SELECT 1 FROM prepare_attempt_evidence req JOIN prepare_attempt_evidence ack
          ON ack.job_id=req.job_id AND ack.task_id=req.task_id AND ack.attempt_no=req.attempt_no
          WHERE req.job_id=NEW.job_id AND req.task_id=NEW.task_id AND req.attempt_no=NEW.attempt_no
          AND req.kind='termination_requested' AND ack.kind='termination_acknowledged'
          AND req.worker_generation=NEW.worker_generation AND ack.worker_generation=NEW.worker_generation
          AND req.evidence_id<ack.evidence_id AND ack.evidence_id<exit.evidence_id
          AND json_extract(req.facts_json,'$.observation.executor_id')=NEW.executor_id
          AND json_extract(ack.facts_json,'$.observation.executor_id')=NEW.executor_id)))))))
 AND EXISTS (SELECT 1 FROM prepare_attempt_evidence p WHERE p.job_id=NEW.job_id AND p.task_id=NEW.task_id
  AND p.attempt_no=NEW.attempt_no AND p.kind='cancelled_planning_disposal'
  AND json_extract(p.facts_json,'$.observation.phase')=OLD.phase
  AND json_extract(p.facts_json,'$.observation.clone_phase')=c.phase))) AND (
  OLD.resolution IS NOT NULL OR OLD.phase<>'observed_stopped' OR NEW.phase<>'closed'
  OR NOT EXISTS (SELECT 1 FROM prepare_attempt_decisions d
    JOIN prepare_execution_bindings b ON b.execution_digest=NEW.execution_digest
    JOIN prepare_clones c ON c.clone_id=NEW.clone_id AND c.job_id=NEW.job_id
    JOIN prepare_decision_evidence qlink ON qlink.decision_id=d.decision_id
    JOIN prepare_attempt_evidence q ON q.evidence_id=qlink.evidence_id
    WHERE d.job_id=NEW.job_id AND d.task_id=NEW.task_id
      AND d.attempt_no=NEW.attempt_no AND d.choice='replay_fresh'
      AND d.target_clone_id IS NOT NULL
      AND d.next_attempt_no=NEW.attempt_no+1
      AND d.worker_generation=NEW.worker_generation
      AND (
        (b.effect_scope='clone_confined' AND b.enforcement_digest IS NOT NULL)
        OR (b.prepare_kind='lb' AND b.policy_revision='lb-container-v2'
          AND b.effect_scope='unknown' AND b.enforcement_digest IS NULL
          AND EXISTS (SELECT 1 FROM prepare_jobs j
            JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
            JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
            WHERE j.job_id=NEW.job_id AND j.prepare_kind='lb'
              AND j.purpose IN ('prepare','cache_explain')
              AND lb.execution_policy_revision='lb-container-v2'
              AND lb.bundle_manifest_digest=b.input_manifest_digest
              AND lb.execution_fingerprint=b.adapter_binding_digest)
          AND EXISTS (SELECT 1 FROM prepare_decision_evidence stop_link
            JOIN prepare_attempt_evidence stop ON stop.evidence_id=stop_link.evidence_id
            WHERE stop_link.decision_id=d.decision_id
              AND stop.job_id=NEW.job_id AND stop.task_id=NEW.task_id
              AND stop.attempt_no=NEW.attempt_no
              AND stop.worker_generation=NEW.worker_generation
              AND stop.kind='executor_inspection'
              AND json_extract(stop.facts_json,'$.observation.executor_id')=NEW.executor_id
              AND (json_extract(stop.facts_json,'$.observation.exit_code')=0
                OR EXISTS (SELECT 1 FROM prepare_decision_evidence request_link
                  JOIN prepare_attempt_evidence request ON request.evidence_id=request_link.evidence_id
                  JOIN prepare_decision_evidence ack_link ON ack_link.decision_id=request_link.decision_id
                  JOIN prepare_attempt_evidence ack ON ack.evidence_id=ack_link.evidence_id
                  WHERE request_link.decision_id=d.decision_id
                    AND request.kind='termination_requested'
                    AND ack.kind='termination_acknowledged'
                    AND request.evidence_id<ack.evidence_id
                    AND ack.evidence_id<stop.evidence_id
                    AND request.job_id=NEW.job_id AND request.task_id=NEW.task_id
                    AND request.attempt_no=NEW.attempt_no
                    AND request.worker_generation=NEW.worker_generation
                    AND ack.job_id=NEW.job_id AND ack.task_id=NEW.task_id
                    AND ack.attempt_no=NEW.attempt_no
                    AND ack.worker_generation=NEW.worker_generation
                    AND json_extract(request.facts_json,'$.clone_id')=NEW.clone_id
                    AND json_extract(ack.facts_json,'$.clone_id')=NEW.clone_id
                    AND json_extract(request.facts_json,'$.execution_digest')=NEW.execution_digest
                    AND json_extract(ack.facts_json,'$.execution_digest')=NEW.execution_digest
                    AND json_extract(request.facts_json,'$.observation.executor_id')=NEW.executor_id
                    AND json_extract(ack.facts_json,'$.observation.executor_id')=NEW.executor_id)))))
      AND c.phase='quiescent' AND q.kind='db_quiescence'
      AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id
      AND q.attempt_no=NEW.attempt_no AND q.worker_generation=NEW.worker_generation
      AND json_extract(q.facts_json,'$.clone_id')=NEW.clone_id
      AND json_extract(q.facts_json,'$.execution_digest')=NEW.execution_digest
      AND json_extract(q.facts_json,'$.observation.runtime_absent')=1
      AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
      AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
      AND EXISTS (SELECT 1 FROM prepare_decision_evidence exit_link
        JOIN prepare_attempt_evidence e ON e.evidence_id=exit_link.evidence_id
        WHERE exit_link.decision_id=d.decision_id
          AND e.evidence_id<q.evidence_id
          AND e.kind='executor_inspection'
          AND e.job_id=NEW.job_id AND e.task_id=NEW.task_id
          AND e.attempt_no=NEW.attempt_no
          AND e.worker_generation=NEW.worker_generation
          AND json_extract(e.facts_json,'$.clone_id')=NEW.clone_id
          AND json_extract(e.facts_json,'$.execution_digest')=NEW.execution_digest
          AND json_extract(e.facts_json,'$.observation.executor_id')=NEW.executor_id)))
BEGIN SELECT RAISE(ABORT,'fresh replay lacks ordered bound proof'); END;

CREATE TRIGGER prepare_attempt_evidence_delete_guard BEFORE DELETE ON prepare_attempt_evidence
WHEN EXISTS (SELECT 1 FROM prepare_jobs WHERE job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'attempt evidence is retained'); END;

CREATE TRIGGER prepare_attempt_evidence_generation_guard BEFORE INSERT ON prepare_attempt_evidence
WHEN NOT EXISTS (SELECT 1 FROM prepare_step_attempts a
  WHERE a.job_id=NEW.job_id AND a.task_id=NEW.task_id
    AND a.attempt_no=NEW.attempt_no AND a.worker_generation=NEW.worker_generation)
BEGIN SELECT RAISE(ABORT,'stale attempt evidence generation'); END;

CREATE TRIGGER prepare_attempt_evidence_immutable BEFORE UPDATE ON prepare_attempt_evidence
BEGIN SELECT RAISE(ABORT,'attempt evidence is append-only'); END;

CREATE TRIGGER prepare_attempt_identity_immutable
BEFORE UPDATE OF job_id,task_id,attempt_no,clone_id,execution_digest,
  planned_executor_name,legacy_unbound,created_at ON prepare_step_attempts
BEGIN SELECT RAISE(ABORT,'attempt identity is immutable'); END;

CREATE TRIGGER prepare_attempt_insert_guard BEFORE INSERT ON prepare_step_attempts
WHEN (NEW.legacy_unbound=0 AND (
  NEW.phase<>'planned'
  OR NOT EXISTS (SELECT 1 FROM prepare_step_bindings b
              WHERE b.job_id=NEW.job_id AND b.task_id=NEW.task_id
                AND b.execution_digest=NEW.execution_digest)
  OR NOT EXISTS (SELECT 1 FROM prepare_clones c
                 WHERE c.clone_id=NEW.clone_id AND c.job_id=NEW.job_id
                   AND c.phase IN ('allocated','active'))
  OR EXISTS (SELECT 1 FROM prepare_step_attempts old
             WHERE old.job_id=NEW.job_id AND old.task_id=NEW.task_id
               AND old.legacy_unbound=1 AND old.resolution IS NULL)
  OR EXISTS (SELECT 1 FROM prepare_step_attempts other
             WHERE other.job_id=NEW.job_id AND other.task_id<>NEW.task_id
               AND other.resolution IS NULL)
  OR NEW.attempt_no != coalesce((SELECT max(old.attempt_no)+1
       FROM prepare_step_attempts old WHERE old.job_id=NEW.job_id
         AND old.task_id=NEW.task_id),1)
  OR (NEW.attempt_no>1 AND NOT EXISTS (
    SELECT 1 FROM prepare_step_attempts prior
    JOIN prepare_attempt_decisions d ON d.job_id=prior.job_id
      AND d.task_id=prior.task_id AND d.attempt_no=prior.attempt_no
    WHERE prior.job_id=NEW.job_id AND prior.task_id=NEW.task_id
      AND prior.attempt_no=NEW.attempt_no-1 AND prior.legacy_unbound=0
      AND prior.resolution IS NOT NULL
      AND d.next_attempt_no=NEW.attempt_no AND d.target_clone_id=NEW.clone_id
      AND d.worker_generation=NEW.worker_generation
      AND d.choice IN ('retry_original','replay_fresh')
      AND EXISTS (SELECT 1 FROM prepare_decision_evidence e
                  WHERE e.decision_id=d.decision_id)))))
BEGIN SELECT RAISE(ABORT,'attempt lacks exact clone/binding or legacy proof'); END;

CREATE TRIGGER prepare_attempt_job_delete_guard BEFORE DELETE ON prepare_jobs
WHEN EXISTS (SELECT 1 FROM prepare_step_attempts a
  WHERE a.job_id=OLD.job_id AND a.resolution IS NULL)
  OR EXISTS (SELECT 1 FROM prepare_clones c
  WHERE c.job_id=OLD.job_id AND c.phase<>'released')
  OR EXISTS (SELECT 1 FROM prepare_artifact_leases l
  WHERE l.owner_kind='job' AND l.owner_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'unresolved attempt or retained clone'); END;

CREATE TRIGGER prepare_attempt_job_terminal_guard BEFORE UPDATE OF status ON prepare_jobs
WHEN NEW.status IN ('succeeded','failed') AND (
  EXISTS (SELECT 1 FROM prepare_step_attempts a
    WHERE a.job_id=OLD.job_id AND a.resolution IS NULL)
  OR EXISTS (SELECT 1 FROM prepare_clones c
    WHERE c.job_id=OLD.job_id AND c.phase<>'released'))
BEGIN SELECT RAISE(ABORT,'unresolved attempt or retained clone'); END;

CREATE TRIGGER prepare_attempt_no_effects_guard
BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN NEW.resolution='no_effects' AND (
  OLD.resolution IS NOT NULL OR OLD.phase<>'observed_stopped' OR NEW.phase<>'closed'
  OR NOT EXISTS (SELECT 1 FROM prepare_attempt_decisions d
    JOIN prepare_decision_evidence qlink ON qlink.decision_id=d.decision_id
    JOIN prepare_attempt_evidence q ON q.evidence_id=qlink.evidence_id
    JOIN prepare_clones c ON c.clone_id=NEW.clone_id AND c.job_id=NEW.job_id
    WHERE d.job_id=NEW.job_id AND d.task_id=NEW.task_id
      AND d.attempt_no=NEW.attempt_no AND d.choice='retry_original'
      AND d.target_clone_id=NEW.clone_id AND d.next_attempt_no=NEW.attempt_no+1
      AND d.worker_generation=NEW.worker_generation
      AND c.phase='quiescent' AND q.kind='db_quiescence'
      AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id
      AND q.attempt_no=NEW.attempt_no AND q.worker_generation=NEW.worker_generation
      AND json_extract(q.facts_json,'$.observation.runtime_absent')=1
      AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
      AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
      AND EXISTS (SELECT 1 FROM prepare_decision_evidence exit_link
        JOIN prepare_attempt_evidence e ON e.evidence_id=exit_link.evidence_id
        WHERE exit_link.decision_id=d.decision_id
          AND e.evidence_id<q.evidence_id
          AND e.kind='executor_inspection'
          AND e.job_id=NEW.job_id AND e.task_id=NEW.task_id
          AND e.attempt_no=NEW.attempt_no
          AND e.worker_generation=NEW.worker_generation
          AND json_extract(e.facts_json,'$.observation.executor_id')=NEW.executor_id)
      AND EXISTS (SELECT 1 FROM prepare_decision_evidence probe_link
        JOIN prepare_attempt_evidence p ON p.evidence_id=probe_link.evidence_id
        WHERE probe_link.decision_id=d.decision_id
          AND p.evidence_id>q.evidence_id AND p.kind='probe'
          AND p.job_id=NEW.job_id AND p.task_id=NEW.task_id
          AND p.attempt_no=NEW.attempt_no
          AND p.worker_generation=NEW.worker_generation
          AND json_extract(p.facts_json,'$.observation.result')='proved_no_effects'
          AND json_extract(p.facts_json,'$.observation.scope_complete')=1)))
BEGIN SELECT RAISE(ABORT,'no-effects retry lacks ordered bound proof'); END;

CREATE TRIGGER prepare_attempt_partial_retry_guard
BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN NEW.resolution='retry_safe_partial' AND (
  OLD.resolution IS NOT NULL OR OLD.phase<>'observed_stopped' OR NEW.phase<>'closed'
  OR NOT EXISTS (SELECT 1 FROM prepare_execution_bindings b
    JOIN prepare_clones c ON c.clone_id=NEW.clone_id AND c.job_id=NEW.job_id
    WHERE b.execution_digest=NEW.execution_digest AND b.partial_prefix_safe=1
      AND b.enforcement_digest IS NOT NULL AND c.phase='quiescent')
  OR NOT EXISTS (SELECT 1 FROM prepare_attempt_decisions d
    JOIN prepare_decision_evidence de ON de.decision_id=d.decision_id
    JOIN prepare_attempt_evidence e ON e.evidence_id=de.evidence_id
    JOIN prepare_clones c ON c.clone_id=NEW.clone_id AND c.job_id=NEW.job_id
    WHERE d.job_id=NEW.job_id AND d.task_id=NEW.task_id
      AND d.attempt_no=NEW.attempt_no AND d.choice='retry_original'
      AND d.target_clone_id=NEW.clone_id AND d.next_attempt_no=NEW.attempt_no+1
      AND d.worker_generation=NEW.worker_generation
      AND e.kind='db_quiescence' AND e.worker_generation=NEW.worker_generation
      AND e.job_id=NEW.job_id AND e.task_id=NEW.task_id
      AND e.attempt_no=NEW.attempt_no
      AND json_extract(e.facts_json,'$.observation.runtime_absent')=1
      AND json_extract(e.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
      AND json_extract(e.facts_json,'$.observation.runtime_id')=c.db_runtime_id
      AND EXISTS (SELECT 1 FROM prepare_decision_evidence prior_link
        JOIN prepare_attempt_evidence exit_evidence
          ON exit_evidence.evidence_id=prior_link.evidence_id
        WHERE prior_link.decision_id=d.decision_id
          AND exit_evidence.evidence_id<e.evidence_id
          AND exit_evidence.kind='executor_inspection'
          AND exit_evidence.job_id=NEW.job_id
          AND exit_evidence.task_id=NEW.task_id
          AND exit_evidence.attempt_no=NEW.attempt_no
          AND exit_evidence.worker_generation=NEW.worker_generation
          AND json_extract(exit_evidence.facts_json,'$.observation.executor_id')=NEW.executor_id)))
BEGIN SELECT RAISE(ABORT,'partial retry lacks enforced bound quiescence'); END;

CREATE TRIGGER prepare_attempt_published_guard
BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN NEW.executor_kind='container' AND NEW.resolution='published' AND (
  OLD.resolution IS NOT NULL OR OLD.phase<>'observed_stopped' OR NEW.phase<>'closed'
  OR NOT EXISTS (
    SELECT 1 FROM prepare_tasks t
    JOIN prepare_state_bindings sb ON sb.state_id=t.output_state_id
      AND sb.execution_digest=NEW.execution_digest
    JOIN prepare_clones c ON c.clone_id=NEW.clone_id AND c.job_id=NEW.job_id
    JOIN prepare_attempt_decisions d ON d.job_id=NEW.job_id
      AND d.task_id=NEW.task_id AND d.attempt_no=NEW.attempt_no
      AND d.worker_generation=NEW.worker_generation AND d.choice='publish_original'
    JOIN prepare_decision_evidence exit_link ON exit_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence exit_e ON exit_e.evidence_id=exit_link.evidence_id
    JOIN prepare_decision_evidence q_link ON q_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence q ON q.evidence_id=q_link.evidence_id
    JOIN prepare_decision_evidence pub_link ON pub_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence pub ON pub.evidence_id=pub_link.evidence_id
    WHERE t.job_id=NEW.job_id AND t.task_id=NEW.task_id
      AND c.phase='quiescent' AND c.db_runtime_id IS NOT NULL
      AND exit_e.job_id=NEW.job_id AND exit_e.task_id=NEW.task_id
      AND exit_e.attempt_no=NEW.attempt_no
      AND exit_e.worker_generation=NEW.worker_generation
      AND exit_e.kind='executor_inspection'
      AND json_extract(exit_e.facts_json,'$.observation.executor_id')=NEW.executor_id
      AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id
      AND q.attempt_no=NEW.attempt_no AND q.worker_generation=NEW.worker_generation
      AND q.kind='db_quiescence' AND q.evidence_id>exit_e.evidence_id
      AND json_extract(q.facts_json,'$.observation.runtime_absent')=1
      AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
      AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
      AND pub.job_id=NEW.job_id AND pub.task_id=NEW.task_id
      AND pub.attempt_no=NEW.attempt_no AND pub.worker_generation=NEW.worker_generation
      AND pub.kind='publication' AND pub.evidence_id>q.evidence_id
      AND json_extract(pub.facts_json,'$.observation.state_id')=t.output_state_id
      AND (json_extract(exit_e.facts_json,'$.observation.exit_code')=0
        OR EXISTS (SELECT 1 FROM prepare_decision_evidence probe_link
          JOIN prepare_attempt_evidence probe ON probe.evidence_id=probe_link.evidence_id
          WHERE probe_link.decision_id=d.decision_id
            AND probe.job_id=NEW.job_id AND probe.task_id=NEW.task_id
            AND probe.attempt_no=NEW.attempt_no
            AND probe.worker_generation=NEW.worker_generation
            AND probe.kind='probe' AND probe.evidence_id>q.evidence_id
            AND probe.evidence_id<pub.evidence_id
            AND json_extract(probe.facts_json,'$.observation.result')='proved_success'
            AND json_extract(probe.facts_json,'$.observation.scope_complete')=1))))
BEGIN SELECT RAISE(ABORT,'published attempt lacks exact state and ordered proof'); END;

CREATE TRIGGER prepare_attempt_resolution_immutable
BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN OLD.resolution IS NOT NULL AND NEW.resolution IS NOT OLD.resolution
BEGIN SELECT RAISE(ABORT,'attempt resolution is immutable'); END;

CREATE TRIGGER prepare_attempt_task_delete_guard BEFORE DELETE ON prepare_tasks
WHEN EXISTS (SELECT 1 FROM prepare_step_attempts a
  WHERE a.job_id=OLD.job_id AND a.task_id=OLD.task_id AND a.resolution IS NULL)
BEGIN SELECT RAISE(ABORT,'unresolved task attempt'); END;

CREATE TRIGGER prepare_attempt_task_terminal_guard BEFORE UPDATE OF status ON prepare_tasks
WHEN NEW.status IN ('succeeded','failed') AND EXISTS (
  SELECT 1 FROM prepare_step_attempts a WHERE a.job_id=OLD.job_id
    AND a.task_id=OLD.task_id AND a.resolution IS NULL)
BEGIN SELECT RAISE(ABORT,'unresolved task attempt'); END;

CREATE TRIGGER prepare_clone_delete_guard BEFORE DELETE ON prepare_clones
WHEN EXISTS (SELECT 1 FROM prepare_jobs WHERE job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'clone is retained with its job'); END;

CREATE TRIGGER prepare_clone_replay_guard BEFORE INSERT ON prepare_clones
WHEN EXISTS (SELECT 1 FROM prepare_step_attempts prior WHERE prior.job_id=NEW.job_id)
 AND NOT EXISTS (
   SELECT 1 FROM prepare_attempt_decisions d
   JOIN prepare_step_attempts prior ON prior.job_id=d.job_id
     AND prior.task_id=d.task_id AND prior.attempt_no=d.attempt_no
   JOIN prepare_clones source ON source.clone_id=prior.clone_id
   WHERE d.job_id=NEW.job_id AND d.target_clone_id=NEW.clone_id
     AND d.choice='replay_fresh' AND prior.execution_digest IS NOT NULL
     AND source.source_kind=NEW.source_kind
     AND source.source_image_id IS NEW.source_image_id
     AND source.source_state_id IS NEW.source_state_id
     AND EXISTS (SELECT 1 FROM prepare_decision_evidence e
                 WHERE e.decision_id=d.decision_id))
 AND NOT EXISTS (
   SELECT 1 FROM prepare_step_attempts prior
   JOIN prepare_tasks completed ON completed.job_id=prior.job_id
     AND completed.task_id=prior.task_id
   JOIN prepare_tasks next ON next.job_id=prior.job_id
   JOIN prepare_jobs job ON job.job_id=prior.job_id
   JOIN prepare_clones old ON old.clone_id=prior.clone_id
     AND old.job_id=prior.job_id
   WHERE prior.job_id=NEW.job_id AND prior.resolution='published'
     AND prior.legacy_unbound=0 AND completed.status='succeeded'
     AND completed.type='state_execute'
     AND completed.output_state_id=NEW.source_state_id
     AND next.status='running' AND next.type='state_execute'
     AND next.input_kind='state' AND next.input_id=NEW.source_state_id
     AND next.position>completed.position AND job.status='running'
     AND NEW.source_kind='state' AND old.phase='released'
     AND NOT EXISTS (SELECT 1 FROM prepare_tasks between_task
       WHERE between_task.job_id=next.job_id
         AND between_task.type='state_execute'
         AND between_task.position>completed.position
         AND between_task.position<next.position)
     AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts unresolved
       WHERE unresolved.job_id=NEW.job_id AND unresolved.resolution IS NULL))
 AND NOT EXISTS (
   SELECT 1 FROM prepare_step_attempts result
   JOIN prepare_clones preview ON preview.clone_id=result.clone_id
     AND preview.job_id=result.job_id
   JOIN prepare_tasks plan ON plan.job_id=result.job_id
     AND plan.task_id=result.task_id
   JOIN prepare_tasks next ON next.job_id=result.job_id
   JOIN prepare_jobs job ON job.job_id=result.job_id
   WHERE result.job_id=NEW.job_id AND result.task_id='plan'
     AND result.phase='closed' AND result.resolution='planning_result_saved'
     AND result.legacy_unbound=0 AND plan.type='plan'
     AND plan.status='succeeded' AND plan.position=0
     AND job.prepare_kind='lb' AND job.plan_only=0
     AND job.purpose='prepare' AND job.status='running'
     AND next.type='state_execute' AND next.status='running'
     AND next.position>plan.position
     AND NOT EXISTS (SELECT 1 FROM prepare_tasks earlier
       WHERE earlier.job_id=next.job_id AND earlier.type='state_execute'
         AND earlier.position<next.position)
     AND next.input_kind=NEW.source_kind
     AND next.input_id=CASE NEW.source_kind WHEN 'image' THEN NEW.source_image_id
       ELSE NEW.source_state_id END
     AND preview.source_kind=NEW.source_kind
     AND preview.source_image_id IS NEW.source_image_id
     AND preview.source_state_id IS NEW.source_state_id
     AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts unresolved
       WHERE unresolved.job_id=NEW.job_id AND unresolved.resolution IS NULL)
     AND NOT EXISTS (SELECT 1 FROM prepare_clones old
       JOIN prepare_step_attempts attempt ON attempt.clone_id=old.clone_id
         AND attempt.job_id=old.job_id
       WHERE old.job_id=NEW.job_id AND attempt.task_id='plan'
         AND old.phase<>'released'))
 AND NOT EXISTS (
  SELECT 1 FROM prepare_jobs job
  JOIN prepare_tasks next ON next.job_id=job.job_id
  JOIN prepare_tasks prefix ON prefix.job_id=job.job_id AND prefix.output_state_id=NEW.source_state_id
  JOIN prepare_step_bindings step ON step.job_id=prefix.job_id AND step.task_id=prefix.task_id
  JOIN prepare_state_bindings state ON state.state_id=prefix.output_state_id AND state.execution_digest=step.execution_digest
  JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id
  JOIN liquibase_job_bindings binding ON binding.job_id=job.job_id AND binding.execution_fingerprint=lb.execution_fingerprint
  WHERE job.job_id=NEW.job_id AND job.prepare_kind='lb' AND job.purpose='prepare' AND job.status='running'
    AND NEW.source_kind='state' AND next.type='state_execute' AND next.status='running'
    AND next.input_kind='state' AND next.input_id=NEW.source_state_id
    AND prefix.type='state_execute' AND prefix.status='succeeded' AND prefix.cached=1
    AND prefix.position=next.position-1
    AND NOT EXISTS (SELECT 1 FROM prepare_tasks earlier WHERE earlier.job_id=job.job_id
      AND earlier.type='state_execute' AND earlier.position<next.position AND earlier.status<>'succeeded')
    AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts unresolved WHERE unresolved.job_id=job.job_id AND unresolved.resolution IS NULL)
    AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_sessions previous WHERE previous.job_id=job.job_id)
    AND EXISTS (SELECT 1 FROM prepare_step_attempts plan JOIN prepare_clones preview ON preview.clone_id=plan.clone_id
      JOIN prepare_tasks planned ON planned.job_id=plan.job_id AND planned.task_id=plan.task_id
      WHERE plan.job_id=job.job_id AND plan.task_id='plan' AND plan.phase='closed'
        AND plan.resolution='planning_result_saved' AND preview.phase='released' AND planned.status='succeeded'))
 AND NOT EXISTS (
 SELECT 1 FROM prepare_attempt_decisions d JOIN prepare_step_attempts a ON a.job_id=d.job_id AND a.task_id=d.task_id AND a.attempt_no=d.attempt_no
 JOIN prepare_liquibase_invocations i ON i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no
 JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id
 JOIN prepare_clones old ON old.clone_id=s.clone_id
 JOIN prepare_tasks t ON t.job_id=a.job_id AND t.task_id=a.task_id
 WHERE d.job_id=NEW.job_id AND d.target_clone_id=NEW.clone_id AND d.choice='replay_fresh'
 AND a.executor_kind='session_exec' AND a.phase='closed' AND a.resolution IN ('discarded_clone','never_started')
 AND old.phase='released' AND s.retired_at IS NULL AND s.runner_phase='stopped'
 AND t.position=s.cursor_position AND NEW.source_kind=t.input_kind
 AND ((NEW.source_kind='state' AND NEW.source_state_id=s.cursor_state_id AND t.input_id=s.cursor_state_id)
   OR (NEW.source_kind='image' AND NEW.source_image_id=s.cursor_image_id AND t.input_id=s.cursor_image_id))
 AND EXISTS (SELECT 1 FROM prepare_decision_evidence evidence WHERE evidence.decision_id=d.decision_id))
 AND NOT EXISTS (
SELECT 1 FROM prepare_liquibase_cache_restores r JOIN prepare_liquibase_sessions old ON old.session_id=r.session_id
JOIN prepare_clones prior ON prior.clone_id=old.clone_id JOIN prepare_tasks cached ON cached.job_id=r.job_id AND cached.task_id=r.task_id
JOIN prepare_tasks next ON next.job_id=r.job_id
WHERE r.job_id=NEW.job_id AND r.restore_phase='abandoned' AND old.retired_at IS NOT NULL AND prior.phase='released'
AND cached.status='succeeded' AND cached.cached=1 AND cached.output_state_id=r.state_id
AND NEW.source_kind='state' AND NEW.source_state_id=r.state_id AND next.input_kind='state' AND next.input_id=r.state_id
AND next.type='state_execute' AND next.position=cached.position+1 AND next.status IN ('queued','running')
AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.job_id=r.job_id AND a.resolution IS NULL))
BEGIN SELECT RAISE(ABORT,'fresh clone requires proved replay or published predecessor'); END;

CREATE TRIGGER prepare_clone_runtime_identity_guard BEFORE UPDATE OF planned_db_runtime_name,db_runtime_id ON prepare_clones
WHEN NEW.planned_db_runtime_name IS NOT OLD.planned_db_runtime_name
  OR (OLD.db_runtime_id IS NOT NULL AND NEW.db_runtime_id IS NOT OLD.db_runtime_id)
BEGIN SELECT RAISE(ABORT,'clone runtime identity is immutable'); END;

CREATE TRIGGER prepare_clone_source_guard BEFORE DELETE ON states
WHEN EXISTS (SELECT 1 FROM prepare_clones c
             WHERE c.source_state_id=OLD.state_id AND c.phase<>'released')
BEGIN SELECT RAISE(ABORT,'state is retained by a recovery clone'); END;

CREATE TRIGGER prepare_clone_source_immutable
BEFORE UPDATE OF clone_id,job_id,clone_generation,source_kind,source_image_id,
  source_state_id,clone_ref,created_at ON prepare_clones
BEGIN SELECT RAISE(ABORT,'clone source and logical identity are immutable'); END;

CREATE TRIGGER prepare_clone_unresolved_retention_guard
BEFORE UPDATE OF phase ON prepare_clones
WHEN NEW.phase IN ('retained','released') AND EXISTS (
  SELECT 1 FROM prepare_step_attempts a
  WHERE a.clone_id=OLD.clone_id AND a.job_id=OLD.job_id
    AND a.resolution IS NULL)
BEGIN SELECT RAISE(ABORT,'unresolved attempt retains its clone'); END;

CREATE TRIGGER prepare_decision_evidence_delete_guard BEFORE DELETE ON prepare_decision_evidence
WHEN EXISTS (SELECT 1 FROM prepare_attempt_decisions d JOIN prepare_jobs j ON j.job_id=d.job_id WHERE d.decision_id=OLD.decision_id)
BEGIN SELECT RAISE(ABORT,'decision evidence link is retained'); END;

CREATE TRIGGER prepare_decision_evidence_immutable BEFORE UPDATE ON prepare_decision_evidence
BEGIN SELECT RAISE(ABORT,'decision evidence link is immutable'); END;

CREATE TRIGGER prepare_decision_evidence_scope_guard BEFORE INSERT ON prepare_decision_evidence
WHEN NOT EXISTS (SELECT 1 FROM prepare_attempt_decisions d
  JOIN prepare_attempt_evidence e ON e.evidence_id=NEW.evidence_id
  WHERE d.decision_id=NEW.decision_id AND d.job_id=e.job_id
    AND d.task_id=e.task_id AND d.attempt_no=e.attempt_no)
BEGIN SELECT RAISE(ABORT,'decision evidence refers to another attempt'); END;

CREATE TRIGGER prepare_execution_delete_guard BEFORE DELETE ON prepare_execution_bindings
BEGIN SELECT RAISE(ABORT,'common execution binding is retained'); END;

CREATE TRIGGER prepare_execution_immutable BEFORE UPDATE ON prepare_execution_bindings
BEGIN SELECT RAISE(ABORT,'common execution binding is immutable'); END;

CREATE TRIGGER prepare_execution_liquibase_guard BEFORE INSERT ON prepare_execution_bindings
WHEN NEW.prepare_kind='lb' AND NOT EXISTS (
  SELECT 1 FROM liquibase_execution_bindings b
  WHERE b.execution_fingerprint=NEW.adapter_binding_digest
    AND b.target_identity_digest=NEW.target_identity_digest
    AND b.image_manifest_digest=NEW.executor_image_digest
    AND b.execution_policy_revision=NEW.policy_revision)
BEGIN SELECT RAISE(ABORT,'Liquibase adapter binding differs'); END;

CREATE TRIGGER prepare_job_purpose_insert_guard BEFORE INSERT ON prepare_jobs
WHEN NEW.purpose NOT IN ('prepare','cache_explain')
 OR (NEW.purpose='cache_explain' AND NEW.status<>'queued')
 OR (NEW.purpose='cache_explain' AND (NEW.prepare_kind<>'lb' OR NEW.plan_only<>1 OR NEW.result_json IS NOT NULL))
 OR (NEW.purpose='prepare' AND NEW.cache_explain_result_json IS NOT NULL)
 OR (NEW.purpose='cache_explain' AND NEW.status='succeeded' AND NEW.cache_explain_result_json IS NULL)
 OR (NEW.cache_explain_result_json IS NOT NULL AND NEW.status<>'succeeded')
 OR (NEW.cache_explain_result_json IS NOT NULL AND NOT json_valid(NEW.cache_explain_result_json))
BEGIN SELECT RAISE(ABORT,'invalid prepare job purpose or explanation result'); END;

CREATE TRIGGER prepare_job_purpose_update_guard BEFORE UPDATE ON prepare_jobs
WHEN NEW.purpose IS NOT OLD.purpose
 OR (OLD.cache_explain_result_json IS NOT NULL AND NEW.cache_explain_result_json IS NOT OLD.cache_explain_result_json)
 OR (NEW.purpose='cache_explain' AND (NEW.prepare_kind<>'lb' OR NEW.plan_only<>1 OR NEW.result_json IS NOT NULL))
 OR (NEW.purpose='prepare' AND NEW.cache_explain_result_json IS NOT NULL)
 OR (NEW.purpose='cache_explain' AND NEW.status='succeeded' AND NEW.cache_explain_result_json IS NULL)
 OR (NEW.cache_explain_result_json IS NOT NULL AND NEW.status<>'succeeded')
 OR (NEW.cache_explain_result_json IS NOT NULL AND NOT json_valid(NEW.cache_explain_result_json))
BEGIN SELECT RAISE(ABORT,'invalid prepare job purpose or explanation result'); END;

CREATE TRIGGER prepare_lb_cache_selection_admission BEFORE INSERT ON prepare_liquibase_cache_selections
WHEN NEW.transferred_at IS NOT NULL OR NEW.successor_session_id IS NOT NULL OR NEW.released_at IS NOT NULL
 OR NEW.release_outcome_digest IS NOT NULL OR NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_invocations i
 JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id
 JOIN prepare_clones c ON c.clone_id=s.clone_id
 JOIN prepare_tasks t ON t.job_id=a.job_id AND t.task_id=a.task_id
 JOIN prepare_jobs j ON j.job_id=a.job_id
 JOIN states state ON state.state_id=NEW.target_state_id
 JOIN prepare_state_bindings binding ON binding.state_id=state.state_id AND binding.execution_digest=a.execution_digest
 JOIN prepare_step_bindings step ON step.job_id=a.job_id AND step.task_id=a.task_id AND step.execution_digest=a.execution_digest
 JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id AND lb.execution_fingerprint=s.execution_fingerprint
 JOIN prepare_attempt_evidence request USING(job_id,task_id,attempt_no,worker_generation)
 JOIN prepare_attempt_evidence ack USING(job_id,task_id,attempt_no,worker_generation)
 JOIN prepare_attempt_evidence stopped USING(job_id,task_id,attempt_no,worker_generation)
 WHERE a.job_id=NEW.job_id AND a.task_id=NEW.task_id AND a.attempt_no=NEW.attempt_no
 AND a.clone_id=NEW.clone_id AND s.clone_id=NEW.clone_id AND s.session_id=NEW.session_id
 AND a.worker_generation=NEW.worker_generation AND s.worker_generation=NEW.worker_generation
 AND a.execution_digest=NEW.execution_digest AND a.executor_kind='session_exec'
 AND a.phase IN ('planned','exec_requested','exec_completed') AND a.resolution IS NULL
 AND i.retired_at IS NULL AND (i.complete_at IS NULL OR i.exit_code=0)
 AND s.retired_at IS NULL AND s.runner_phase='stopped' AND s.runner_id IS NOT NULL
 AND c.phase IN ('active','quiescent') AND c.db_runtime_id IS NOT NULL
 AND t.type='state_execute' AND t.status IN ('running','recovery_required') AND t.position=s.cursor_position
 AND t.output_state_id=state.state_id AND state.state_fingerprint=state.state_id AND state.prepare_kind='lb'
 AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0 AND j.status IN ('running','recovery_required')
 AND state.image_id=coalesce(nullif(j.resolved_image_id,''),j.image_id)
 AND coalesce(state.lineage_ref,'')=coalesce(j.lineage_ref,'') AND coalesce(state.identity_digest,'')=coalesce(j.identity_digest,'')
 AND ((t.input_kind='state' AND t.input_id=s.cursor_state_id AND state.parent_state_id=t.input_id)
 OR (t.input_kind='image' AND t.input_id=s.cursor_image_id AND state.parent_state_id IS NULL))
 AND request.kind='termination_requested' AND ack.kind='termination_acknowledged' AND stopped.kind='executor_inspection'
 AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id
 AND json_extract(request.facts_json,'$.clone_id')=s.clone_id
 AND json_extract(ack.facts_json,'$.clone_id')=s.clone_id
 AND json_extract(stopped.facts_json,'$.clone_id')=s.clone_id
 AND json_extract(request.facts_json,'$.execution_digest')=a.execution_digest
 AND json_extract(ack.facts_json,'$.execution_digest')=a.execution_digest
 AND json_extract(stopped.facts_json,'$.execution_digest')=a.execution_digest
 AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id
 AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id
 AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id
 AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id
 AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
 AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id
 AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
 AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts other WHERE other.job_id=a.job_id AND other.resolution IS NULL
   AND (other.task_id<>a.task_id OR other.attempt_no<>a.attempt_no))
 AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.job_id=a.job_id AND r.task_id=a.task_id)
 AND NOT EXISTS(SELECT 1 FROM prepare_attempt_evidence cancel WHERE cancel.job_id=a.job_id AND cancel.task_id=a.task_id
   AND cancel.attempt_no=a.attempt_no AND cancel.kind='cancellation_requested')
 AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_cancellations cancel WHERE cancel.job_id=a.job_id)
 AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=a.job_id))
BEGIN SELECT RAISE(ABORT,'invalid Liquibase cache selection admission'); END;

CREATE TRIGGER prepare_lb_cache_selection_immutable_identity
BEFORE UPDATE OF job_id,task_id,attempt_no,session_id,clone_id,worker_generation,execution_digest,target_state_id,selected_at
ON prepare_liquibase_cache_selections
BEGIN SELECT RAISE(ABORT,'Liquibase cache selection identity is immutable'); END;

CREATE TRIGGER prepare_lb_cache_selection_job_delete BEFORE DELETE ON prepare_jobs
WHEN EXISTS(SELECT 1 FROM prepare_liquibase_cache_selections selection WHERE selection.job_id=OLD.job_id
 AND selection.transferred_at IS NULL AND selection.released_at IS NULL)
BEGIN SELECT RAISE(ABORT,'active Liquibase cache selection prevents job deletion'); END;

CREATE TRIGGER prepare_lb_cache_selection_job_terminal BEFORE UPDATE OF status ON prepare_jobs
WHEN NEW.status IN ('failed','succeeded') AND EXISTS(SELECT 1 FROM prepare_liquibase_cache_selections selection
 WHERE selection.job_id=OLD.job_id AND selection.transferred_at IS NULL AND selection.released_at IS NULL)
BEGIN SELECT RAISE(ABORT,'active Liquibase cache selection prevents terminal publication'); END;

CREATE TRIGGER prepare_lb_cache_selection_selection_delete BEFORE DELETE ON prepare_liquibase_cache_selections
WHEN EXISTS(SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'Liquibase cache selection is retained with its job'); END;

CREATE TRIGGER prepare_lb_cache_selection_state_delete BEFORE DELETE ON states
WHEN EXISTS(SELECT 1 FROM prepare_liquibase_cache_selections selection WHERE selection.target_state_id=OLD.state_id
 AND selection.transferred_at IS NULL AND selection.released_at IS NULL)
BEGIN SELECT RAISE(ABORT,'active Liquibase cache selection pins target state'); END;

CREATE TRIGGER prepare_lb_cache_selection_terminal_release BEFORE UPDATE OF released_at,release_outcome_digest ON prepare_liquibase_cache_selections
WHEN OLD.released_at IS NOT NULL OR OLD.transferred_at IS NOT NULL OR NEW.released_at IS NULL
 OR NEW.transferred_at IS NOT NULL OR NEW.successor_session_id IS NOT NULL
 OR NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations f JOIN prepare_jobs j ON j.job_id=f.job_id
 JOIN prepare_liquibase_sessions s ON s.session_id=OLD.session_id
 JOIN prepare_clones c ON c.clone_id=OLD.clone_id
 JOIN prepare_liquibase_invocations i ON i.job_id=OLD.job_id AND i.task_id=OLD.task_id AND i.attempt_no=OLD.attempt_no
 WHERE f.job_id=OLD.job_id AND f.outcome_kind IN ('failure','cancelled') AND f.outcome_digest=NEW.release_outcome_digest
 AND f.projections_removed_at IS NOT NULL AND j.status IN ('queued','running','recovery_required')
 AND s.retired_at IS NOT NULL AND s.runner_phase='retired' AND c.phase='released' AND i.retired_at IS NOT NULL
 AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_sessions live WHERE live.job_id=OLD.job_id AND live.retired_at IS NULL))
BEGIN SELECT RAISE(ABORT,'Liquibase cache selection release requires terminal cleanup proof'); END;

CREATE TRIGGER prepare_lb_cache_selection_transfer BEFORE UPDATE OF transferred_at,successor_session_id ON prepare_liquibase_cache_selections
WHEN OLD.transferred_at IS NOT NULL OR OLD.released_at IS NOT NULL OR NEW.transferred_at IS NULL
 OR NEW.released_at IS NOT NULL OR NEW.release_outcome_digest IS NOT NULL
 OR NOT EXISTS(SELECT 1 FROM prepare_tasks t
 JOIN prepare_step_attempts a USING(job_id,task_id)
 JOIN prepare_liquibase_invocations i USING(job_id,task_id,attempt_no)
 JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id
 JOIN prepare_clones c ON c.clone_id=s.clone_id
 JOIN prepare_liquibase_cache_restores r ON r.job_id=t.job_id AND r.task_id=t.task_id AND r.session_id=s.session_id
 WHERE t.job_id=OLD.job_id AND t.task_id=OLD.task_id AND a.attempt_no=OLD.attempt_no
 AND a.execution_digest=OLD.execution_digest AND a.clone_id=OLD.clone_id AND a.worker_generation=OLD.worker_generation
 AND a.phase='closed' AND a.resolution='discarded_clone'
 AND t.status='succeeded' AND t.cached=1 AND t.output_state_id=OLD.target_state_id
 AND s.session_id=OLD.session_id AND s.runner_phase='retired' AND s.retired_at IS NOT NULL
 AND c.phase='released' AND i.retired_at IS NOT NULL AND r.state_id=OLD.target_state_id AND r.restore_phase='abandoned'
 AND ((NEW.successor_session_id IS NULL AND NOT EXISTS(SELECT 1 FROM prepare_tasks suffix WHERE suffix.job_id=t.job_id
   AND suffix.type='state_execute' AND suffix.position>t.position))
 OR EXISTS(SELECT 1 FROM prepare_liquibase_sessions successor JOIN prepare_clones source ON source.clone_id=successor.clone_id
   JOIN prepare_tasks suffix ON suffix.job_id=successor.job_id AND suffix.position=successor.start_position
   JOIN prepare_step_bindings b ON b.job_id=suffix.job_id AND b.task_id=suffix.task_id
   WHERE successor.session_id=NEW.successor_session_id AND successor.job_id=OLD.job_id
   AND successor.session_no=s.session_no+1 AND successor.worker_generation=OLD.worker_generation
   AND successor.execution_fingerprint=s.execution_fingerprint AND successor.start_kind='state'
   AND successor.start_id=OLD.target_state_id AND successor.start_position=t.position+1
   AND source.source_state_id=OLD.target_state_id AND source.source_kind='state'
   AND suffix.type='state_execute' AND suffix.input_kind='state' AND suffix.input_id=OLD.target_state_id))
 AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_cancellations cancel WHERE cancel.job_id=OLD.job_id)
 AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=OLD.job_id))
BEGIN SELECT RAISE(ABORT,'Liquibase cache selection transfer requires atomic ownership'); END;

CREATE TRIGGER prepare_lb_finalization_delete_guard BEFORE DELETE ON prepare_liquibase_job_finalizations
WHEN EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'Liquibase finalization is retained with job history'); END;

CREATE TRIGGER prepare_lb_finalization_disposal_evidence BEFORE INSERT ON prepare_attempt_evidence
WHEN NEW.kind='selected_failure_disposal' AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a
JOIN prepare_liquibase_invocations i USING(job_id,task_id,attempt_no)
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id
WHERE a.job_id=NEW.job_id AND a.task_id=NEW.task_id AND a.attempt_no=NEW.attempt_no
AND a.resolution IS NULL AND a.phase IN ('planned','exec_requested','exec_completed') AND s.runner_phase='stopped'
AND EXISTS (
SELECT 1 FROM prepare_liquibase_invocations i
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id AND s.job_id=i.job_id
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=s.job_id
JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN prepare_step_bindings sb ON sb.job_id=i.job_id AND sb.task_id=i.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
JOIN prepare_attempt_evidence request ON request.evidence_id=json_extract(NEW.facts_json,'$.observation.request_id')
JOIN prepare_attempt_evidence ack ON ack.evidence_id=json_extract(NEW.facts_json,'$.observation.ack_id')
JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=json_extract(NEW.facts_json,'$.observation.stop_id')
JOIN prepare_attempt_evidence q ON q.evidence_id=json_extract(NEW.facts_json,'$.observation.database_id')
WHERE i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no
AND a.executor_kind='session_exec' AND a.legacy_unbound=0
AND s.clone_id=a.clone_id AND s.worker_generation=a.worker_generation
AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND b.execution_digest=a.execution_digest AND b.prepare_kind='lb'
AND b.adapter_binding_digest=s.execution_fingerprint AND s.execution_fingerprint=lb.execution_fingerprint
AND b.policy_revision=lb.execution_policy_revision AND b.input_manifest_digest=lb.bundle_manifest_digest
AND b.target_identity_digest=lb.target_identity_digest AND b.executor_image_digest=lb.image_manifest_digest
AND NEW.job_id=a.job_id AND NEW.task_id=a.task_id AND NEW.attempt_no=a.attempt_no
AND NEW.worker_generation=a.worker_generation AND NEW.kind='selected_failure_disposal'
AND json_extract(NEW.facts_json,'$.clone_id')=a.clone_id
AND json_extract(NEW.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(NEW.facts_json,'$.observation.outcome_digest')=selected.outcome_digest
AND json_extract(NEW.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(NEW.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(NEW.facts_json,'$.observation.invocation_ref')=i.invocation_ref
AND json_extract(NEW.facts_json,'$.observation.clone_generation')=c.clone_generation
AND json_extract(NEW.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(NEW.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(NEW.facts_json,'$.observation.executor_absent')=1
AND json_extract(NEW.facts_json,'$.observation.runtime_absent')=1
AND json_extract(NEW.facts_json,'$.observation.original_phase') IN ('planned','exec_requested','exec_completed') AND json_extract(NEW.facts_json,'$.observation.original_phase')=a.phase
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged'
AND stopped.kind='executor_inspection' AND q.kind='db_quiescence'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id
AND request.job_id=a.job_id AND request.task_id=a.task_id AND request.attempt_no=a.attempt_no AND request.worker_generation=a.worker_generation
AND ack.job_id=a.job_id AND ack.task_id=a.task_id AND ack.attempt_no=a.attempt_no AND ack.worker_generation=a.worker_generation
AND stopped.job_id=a.job_id AND stopped.task_id=a.task_id AND stopped.attempt_no=a.attempt_no AND stopped.worker_generation=a.worker_generation
AND q.job_id=a.job_id AND q.task_id=a.task_id AND q.attempt_no=a.attempt_no AND q.worker_generation=a.worker_generation
AND json_extract(request.facts_json,'$.clone_id')=a.clone_id AND json_extract(request.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(ack.facts_json,'$.clone_id')=a.clone_id AND json_extract(ack.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(stopped.facts_json,'$.clone_id')=a.clone_id AND json_extract(stopped.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(q.facts_json,'$.clone_id')=a.clone_id AND json_extract(q.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.runtime_absent')=1))
BEGIN SELECT RAISE(ABORT,'selected failure disposal evidence lacks exact outcome and ordered stop proof'); END;

CREATE TRIGGER prepare_lb_finalization_effect_liquibase_job_bindings BEFORE INSERT ON liquibase_job_bindings
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_clones BEFORE INSERT ON prepare_clones
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_liquibase_cache_restores BEFORE INSERT ON prepare_liquibase_cache_restores
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_liquibase_invocations BEFORE INSERT ON prepare_liquibase_invocations
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_liquibase_sessions BEFORE INSERT ON prepare_liquibase_sessions
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_planning_attempt_artifacts BEFORE INSERT ON prepare_planning_attempt_artifacts
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_planning_changesets BEFORE INSERT ON prepare_planning_changesets
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_planning_intents BEFORE INSERT ON prepare_planning_intents
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_step_attempts BEFORE INSERT ON prepare_step_attempts
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_step_bindings BEFORE INSERT ON prepare_step_bindings
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_effect_prepare_tasks BEFORE INSERT ON prepare_tasks
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_failure_admission BEFORE INSERT ON prepare_liquibase_job_finalizations
WHEN NEW.outcome_kind='failure' AND NOT (NEW.event_profile='status_error' AND EXISTS(
SELECT 1 FROM prepare_jobs j JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
JOIN prepare_planning_intents p ON p.job_id=j.job_id
JOIN prepare_tasks t ON t.job_id=p.job_id AND t.task_id=p.task_id
JOIN prepare_step_attempts a ON a.job_id=p.job_id AND a.task_id='plan' AND a.attempt_no=p.result_attempt_no
JOIN prepare_clones c ON c.clone_id=p.preview_clone_id AND c.job_id=p.job_id
JOIN prepare_step_bindings b ON b.job_id=p.job_id AND b.task_id='plan'
JOIN prepare_execution_bindings e ON e.execution_digest=b.execution_digest
WHERE j.job_id=NEW.job_id AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND j.status IN ('queued','running','recovery_required') AND j.finished_at IS NULL AND j.result_json IS NULL AND j.cache_explain_result_json IS NULL
AND jb.requested_tag=lb.requested_tag AND jb.bundle_manifest_digest=lb.bundle_manifest_digest
AND NOT EXISTS(SELECT 1 FROM prepare_step_bindings owner_step
LEFT JOIN prepare_execution_bindings owner_execution ON owner_execution.execution_digest=owner_step.execution_digest
WHERE owner_step.job_id=j.job_id AND (owner_execution.execution_digest IS NULL
OR owner_execution.prepare_kind IS NOT 'lb'
OR owner_execution.adapter_binding_digest IS NOT lb.execution_fingerprint
OR owner_execution.input_manifest_digest IS NOT lb.bundle_manifest_digest
OR owner_execution.executor_image_digest IS NOT lb.image_manifest_digest
OR owner_execution.target_identity_digest IS NOT lb.target_identity_digest
OR owner_execution.policy_revision IS NOT lb.execution_policy_revision))
AND p.plan_saved_at IS NOT NULL AND p.plan_digest IS NOT NULL AND p.plan_count IS NOT NULL
AND p.plan_count=(SELECT count(*) FROM prepare_planning_changesets changes WHERE changes.job_id=p.job_id)
AND t.type='plan' AND t.status='succeeded' AND c.phase='released'
AND a.clone_id=c.clone_id AND a.execution_digest=p.execution_digest AND b.execution_digest=p.execution_digest
AND e.prepare_kind='lb' AND e.adapter_binding_digest=jb.execution_fingerprint AND e.input_manifest_digest=jb.bundle_manifest_digest
AND a.phase='closed' AND a.resolution='planning_result_saved'
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts pending WHERE pending.job_id=p.job_id AND pending.task_id='plan' AND (pending.phase<>'closed' OR pending.resolution IS NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_planning_attempt_artifacts x WHERE x.job_id=p.job_id AND (x.executor_retired_at IS NULL OR x.protected_input_retired_at IS NULL OR x.output_retired_at IS NULL OR x.stderr_retired_at IS NULL OR x.manifest_retired_at IS NULL))))
AND NOT (NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.job_id=NEW.job_id AND (a.phase<>'closed' OR a.resolution IS NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_clones c WHERE c.job_id=NEW.job_id AND c.phase<>'released')
AND NOT EXISTS(SELECT 1 FROM prepare_planning_attempt_artifacts x WHERE x.job_id=NEW.job_id AND (x.executor_retired_at IS NULL OR x.protected_input_retired_at IS NULL OR x.output_retired_at IS NULL OR x.stderr_retired_at IS NULL OR x.manifest_retired_at IS NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_sessions s WHERE s.job_id=NEW.job_id AND (s.retired_at IS NULL OR s.runner_phase<>'retired' OR s.cursor_image_id IS NOT NULL OR s.cursor_state_id IS NOT NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.job_id=NEW.job_id AND i.retired_at IS NULL)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.job_id=NEW.job_id AND r.restore_phase NOT IN ('published','abandoned'))
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND (substr(replace(o.target_path,char(92),'/'),-length('/jobs/'||NEW.job_id||'/'||o.operation_ref))='/jobs/'||NEW.job_id||'/'||o.operation_ref
OR EXISTS(SELECT 1 FROM prepare_clones owner_clone WHERE owner_clone.job_id=NEW.job_id
AND substr(replace(owner_clone.clone_ref,char(92),'/'),1,5)='jobs/'
AND substr(replace(owner_clone.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(owner_clone.clone_ref,char(92),'/')))='/'||replace(owner_clone.clone_ref,char(92),'/')))))
BEGIN SELECT RAISE(ABORT,'Liquibase failure requires completed planning or retired owners'); END;

CREATE TRIGGER prepare_lb_finalization_failure_proof BEFORE UPDATE OF projections_removed_at ON prepare_liquibase_job_finalizations
WHEN OLD.outcome_kind='failure' AND NEW.projections_removed_at IS NOT NULL
AND NOT (NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.job_id=OLD.job_id AND (a.phase<>'closed' OR a.resolution IS NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_clones c WHERE c.job_id=OLD.job_id AND c.phase<>'released')
AND NOT EXISTS(SELECT 1 FROM prepare_planning_attempt_artifacts x WHERE x.job_id=OLD.job_id AND (x.executor_retired_at IS NULL OR x.protected_input_retired_at IS NULL OR x.output_retired_at IS NULL OR x.stderr_retired_at IS NULL OR x.manifest_retired_at IS NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_sessions s WHERE s.job_id=OLD.job_id AND (s.retired_at IS NULL OR s.runner_phase<>'retired' OR s.cursor_image_id IS NOT NULL OR s.cursor_state_id IS NOT NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.job_id=OLD.job_id AND i.retired_at IS NULL)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.job_id=OLD.job_id AND r.restore_phase NOT IN ('published','abandoned'))
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND (substr(replace(o.target_path,char(92),'/'),-length('/jobs/'||OLD.job_id||'/'||o.operation_ref))='/jobs/'||OLD.job_id||'/'||o.operation_ref
OR EXISTS(SELECT 1 FROM prepare_clones owner_clone WHERE owner_clone.job_id=OLD.job_id
AND substr(replace(owner_clone.clone_ref,char(92),'/'),1,5)='jobs/'
AND substr(replace(owner_clone.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(owner_clone.clone_ref,char(92),'/')))='/'||replace(owner_clone.clone_ref,char(92),'/')))))
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure requires all owners retired before projection proof'); END;

CREATE TRIGGER prepare_lb_finalization_failure_task_completion BEFORE UPDATE OF status,finished_at,error_json ON prepare_tasks
WHEN EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure') AND (
 (OLD.status='queued' AND NEW.status<>'queued')
 OR (OLD.status IN ('failed','succeeded') AND (OLD.status IS NOT NEW.status OR OLD.finished_at IS NOT NEW.finished_at OR OLD.error_json IS NOT NEW.error_json))
 OR ((NEW.status IN ('failed','succeeded') OR NEW.finished_at IS NOT NULL OR (OLD.status='recovery_required' AND NEW.status='running'))
 AND (OLD.status IS NOT NEW.status OR OLD.finished_at IS NOT NEW.finished_at OR OLD.error_json IS NOT NEW.error_json)
 AND NOT (NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.job_id=OLD.job_id AND (a.phase<>'closed' OR a.resolution IS NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_clones c WHERE c.job_id=OLD.job_id AND c.phase<>'released')
AND NOT EXISTS(SELECT 1 FROM prepare_planning_attempt_artifacts x WHERE x.job_id=OLD.job_id AND (x.executor_retired_at IS NULL OR x.protected_input_retired_at IS NULL OR x.output_retired_at IS NULL OR x.stderr_retired_at IS NULL OR x.manifest_retired_at IS NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_sessions s WHERE s.job_id=OLD.job_id AND (s.retired_at IS NULL OR s.runner_phase<>'retired' OR s.cursor_image_id IS NOT NULL OR s.cursor_state_id IS NOT NULL))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.job_id=OLD.job_id AND i.retired_at IS NULL)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.job_id=OLD.job_id AND r.restore_phase NOT IN ('published','abandoned'))
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND (substr(replace(o.target_path,char(92),'/'),-length('/jobs/'||OLD.job_id||'/'||o.operation_ref))='/jobs/'||OLD.job_id||'/'||o.operation_ref
OR EXISTS(SELECT 1 FROM prepare_clones owner_clone WHERE owner_clone.job_id=OLD.job_id
AND substr(replace(owner_clone.clone_ref,char(92),'/'),1,5)='jobs/'
AND substr(replace(owner_clone.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(owner_clone.clone_ref,char(92),'/')))='/'||replace(owner_clone.clone_ref,char(92),'/')))))))
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains active tasks until resource retirement'); END;

CREATE TRIGGER prepare_lb_finalization_failure_task_outcome BEFORE UPDATE OF status,finished_at,error_json ON prepare_tasks
WHEN EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure') AND OLD.status NOT IN ('failed','succeeded')
AND (NEW.status IN ('failed','succeeded') OR NEW.finished_at IS NOT NULL)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected JOIN prepare_jobs j ON j.job_id=selected.job_id
WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure'
AND NEW.status='failed' AND NEW.finished_at IS NOT NULL AND NEW.error_json IS selected.outcome_json AND (
 (j.status='failed' AND selected.projections_removed_at IS NOT NULL AND j.error_json IS selected.outcome_json AND j.finished_at IS NEW.finished_at)
 OR (j.status IN ('queued','running','recovery_required') AND NEW.finished_at IS selected.created_at)))
BEGIN SELECT RAISE(ABORT,'Liquibase task completion must match its selected terminal outcome'); END;

CREATE TRIGGER prepare_lb_finalization_immutable
BEFORE UPDATE OF job_id,format_revision,outcome_kind,event_profile,instance_id,outcome_json,outcome_digest,final_state_id,created_at
ON prepare_liquibase_job_finalizations
BEGIN SELECT RAISE(ABORT,'Liquibase finalization outcome is immutable'); END;

CREATE TRIGGER prepare_lb_finalization_insert_guard BEFORE INSERT ON prepare_liquibase_job_finalizations
WHEN NEW.projections_removed_at IS NOT NULL
 OR (NEW.outcome_kind='prepare_success' AND NEW.instance_pin IS NOT NEW.instance_id)
 OR NOT EXISTS (SELECT 1 FROM prepare_jobs j JOIN liquibase_job_bindings b ON b.job_id=j.job_id WHERE j.job_id=NEW.job_id
   AND j.prepare_kind='lb' AND j.status IN ('queued','running','recovery_required')
   AND (NEW.outcome_kind IN ('failure','cancelled')
     OR (NEW.outcome_kind='prepare_success' AND j.purpose='prepare' AND j.plan_only=0)
     OR (NEW.outcome_kind='plan_success' AND j.purpose='prepare' AND j.plan_only=1)
     OR (NEW.outcome_kind='cache_explain_success' AND j.purpose='cache_explain')))
 OR ((NEW.outcome_kind='cancelled') <> EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=NEW.job_id))
BEGIN SELECT RAISE(ABORT,'invalid Liquibase finalization admission'); END;

CREATE TRIGGER prepare_lb_finalization_instance_pin_immutable
BEFORE UPDATE OF instance_id,state_id,image_id,created_at,expires_at,runtime_id,runtime_dir,status ON instances
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.instance_pin=OLD.instance_id)
AND (NEW.instance_id IS NOT OLD.instance_id OR NEW.state_id IS NOT OLD.state_id
 OR NEW.image_id IS NOT OLD.image_id OR NEW.created_at IS NOT OLD.created_at
 OR NEW.expires_at IS NOT OLD.expires_at OR NEW.runtime_id IS NOT OLD.runtime_id
 OR NEW.runtime_dir IS NOT OLD.runtime_dir OR NEW.status IS NOT OLD.status)
BEGIN SELECT RAISE(ABORT,'pinned Liquibase instance is immutable'); END;

CREATE TRIGGER prepare_lb_finalization_invocation_start BEFORE UPDATE OF start_requested_at ON prepare_liquibase_invocations
WHEN OLD.start_requested_at IS NOT NEW.start_requested_at AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure')
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure forbids a new execution permit'); END;

CREATE TRIGGER prepare_lb_finalization_job_definition BEFORE UPDATE OF job_id,purpose,prepare_kind,image_id,plan_only,snapshot_mode,prepare_args_normalized,signature,request_json,resolved_image_id,lineage_ref,identity_digest,created_at ON prepare_jobs
WHEN (EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure') OR EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=NEW.job_id AND selected.outcome_kind='failure')) AND (OLD.job_id IS NOT NEW.job_id OR OLD.purpose IS NOT NEW.purpose OR OLD.prepare_kind IS NOT NEW.prepare_kind OR OLD.image_id IS NOT NEW.image_id OR OLD.plan_only IS NOT NEW.plan_only OR OLD.snapshot_mode IS NOT NEW.snapshot_mode OR OLD.prepare_args_normalized IS NOT NEW.prepare_args_normalized OR OLD.signature IS NOT NEW.signature OR OLD.request_json IS NOT NEW.request_json OR OLD.resolved_image_id IS NOT NEW.resolved_image_id OR OLD.lineage_ref IS NOT NEW.lineage_ref OR OLD.identity_digest IS NOT NEW.identity_digest OR OLD.created_at IS NOT NEW.created_at)
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its job definition'); END;

CREATE TRIGGER prepare_lb_finalization_operation_delete BEFORE DELETE ON managed_runtime_operations
WHEN EXISTS(SELECT 1 FROM prepare_jobs owner JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=owner.job_id
WHERE selected.outcome_kind='failure' AND owner.prepare_kind='lb'
AND (substr(replace(OLD.target_path,char(92),'/'),-length('/jobs/'||owner.job_id||'/'||OLD.operation_ref))='/jobs/'||owner.job_id||'/'||OLD.operation_ref
OR EXISTS(SELECT 1 FROM prepare_clones owner_clone WHERE owner_clone.job_id=owner.job_id
AND substr(replace(owner_clone.clone_ref,char(92),'/'),1,5)='jobs/'
AND substr(replace(owner_clone.clone_ref,char(92),'/'),-length('/'||OLD.operation_ref))='/'||OLD.operation_ref
AND substr(replace(OLD.target_path,char(92),'/'),-length('/'||replace(owner_clone.clone_ref,char(92),'/')))='/'||replace(owner_clone.clone_ref,char(92),'/'))))
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its operation locator'); END;

CREATE TRIGGER prepare_lb_finalization_operation_identity BEFORE UPDATE ON managed_runtime_operations
WHEN (EXISTS(SELECT 1 FROM prepare_jobs owner JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=owner.job_id
WHERE selected.outcome_kind='failure' AND owner.prepare_kind='lb'
AND (substr(replace(OLD.target_path,char(92),'/'),-length('/jobs/'||owner.job_id||'/'||OLD.operation_ref))='/jobs/'||owner.job_id||'/'||OLD.operation_ref
OR EXISTS(SELECT 1 FROM prepare_clones owner_clone WHERE owner_clone.job_id=owner.job_id
AND substr(replace(owner_clone.clone_ref,char(92),'/'),1,5)='jobs/'
AND substr(replace(owner_clone.clone_ref,char(92),'/'),-length('/'||OLD.operation_ref))='/'||OLD.operation_ref
AND substr(replace(OLD.target_path,char(92),'/'),-length('/'||replace(owner_clone.clone_ref,char(92),'/')))='/'||replace(owner_clone.clone_ref,char(92),'/')))) OR EXISTS(SELECT 1 FROM prepare_jobs owner JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=owner.job_id
WHERE selected.outcome_kind='failure' AND owner.prepare_kind='lb'
AND (substr(replace(NEW.target_path,char(92),'/'),-length('/jobs/'||owner.job_id||'/'||NEW.operation_ref))='/jobs/'||owner.job_id||'/'||NEW.operation_ref
OR EXISTS(SELECT 1 FROM prepare_clones owner_clone WHERE owner_clone.job_id=owner.job_id
AND substr(replace(owner_clone.clone_ref,char(92),'/'),1,5)='jobs/'
AND substr(replace(owner_clone.clone_ref,char(92),'/'),-length('/'||NEW.operation_ref))='/'||NEW.operation_ref
AND substr(replace(NEW.target_path,char(92),'/'),-length('/'||replace(owner_clone.clone_ref,char(92),'/')))='/'||replace(owner_clone.clone_ref,char(92),'/'))))) AND (
OLD.operation_ref IS NOT NEW.operation_ref OR OLD.target_path IS NOT NEW.target_path OR OLD.source_ref IS NOT NEW.source_ref
OR OLD.identity_json IS NOT NEW.identity_json OR OLD.physical_identity IS NOT NEW.physical_identity
OR (OLD.runtime_ref IS NOT NEW.runtime_ref AND NOT (OLD.runtime_ref='' AND NEW.runtime_ref<>'' AND OLD.stage='assigned' AND NEW.stage='running'))
OR NOT (OLD.stage=NEW.stage OR (OLD.stage='assigned' AND NEW.stage IN ('ready','running','retired')) OR (OLD.stage='ready' AND NEW.stage IN ('running','retired')) OR (OLD.stage='running' AND NEW.stage='retired')))
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its exact operation'); END;

CREATE TRIGGER prepare_lb_finalization_operation_insert BEFORE INSERT ON managed_runtime_operations
WHEN EXISTS(SELECT 1 FROM prepare_jobs owner JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=owner.job_id
WHERE selected.outcome_kind='failure' AND owner.prepare_kind='lb'
AND (substr(replace(NEW.target_path,char(92),'/'),-length('/jobs/'||owner.job_id||'/'||NEW.operation_ref))='/jobs/'||owner.job_id||'/'||NEW.operation_ref
OR EXISTS(SELECT 1 FROM prepare_clones owner_clone WHERE owner_clone.job_id=owner.job_id
AND substr(replace(owner_clone.clone_ref,char(92),'/'),1,5)='jobs/'
AND substr(replace(owner_clone.clone_ref,char(92),'/'),-length('/'||NEW.operation_ref))='/'||NEW.operation_ref
AND substr(replace(NEW.target_path,char(92),'/'),-length('/'||replace(owner_clone.clone_ref,char(92),'/')))='/'||replace(owner_clone.clone_ref,char(92),'/'))))
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure forbids a new exact operation assignment'); END;

CREATE TRIGGER prepare_lb_finalization_operation_pin_guard BEFORE UPDATE OF stage ON managed_runtime_operations
WHEN NEW.stage='retired' AND OLD.stage<>'retired' AND EXISTS (
 SELECT 1 FROM prepare_liquibase_job_finalizations f
 JOIN instances i ON i.instance_id=f.instance_pin
 WHERE f.instance_pin IS NOT NULL AND i.runtime_id=OLD.runtime_ref AND i.runtime_dir=OLD.target_path)
BEGIN SELECT RAISE(ABORT,'pinned Liquibase publication retains its exact runtime operation'); END;

CREATE TRIGGER prepare_lb_finalization_permit_attempt BEFORE UPDATE OF phase ON prepare_step_attempts
WHEN NEW.phase<>OLD.phase AND NEW.phase IN ('create_requested','start_requested','exec_requested')
AND EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_permit_database BEFORE UPDATE OF db_phase ON prepare_liquibase_sessions
WHEN NEW.db_phase<>OLD.db_phase AND NEW.db_phase IN ('restart_requested')
AND EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_permit_runner BEFORE UPDATE OF runner_phase ON prepare_liquibase_sessions
WHEN NEW.runner_phase<>OLD.runner_phase AND NEW.runner_phase IN ('create_requested','start_requested')
AND EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_pin_guard BEFORE UPDATE OF instance_pin ON prepare_liquibase_job_finalizations
WHEN OLD.instance_pin IS NULL OR NEW.instance_pin IS NOT NULL OR OLD.projections_removed_at IS NULL
 OR NOT EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id AND j.status='succeeded' AND j.finished_at IS NOT NULL)
BEGIN SELECT RAISE(ABORT,'Liquibase instance pin clears only at successful completion'); END;

CREATE TRIGGER prepare_lb_finalization_plan_result BEFORE UPDATE OF
result_attempt_no,plan_digest,plan_count,parsed_output_digest,plan_saved_at ON prepare_planning_intents
WHEN (OLD.result_attempt_no IS NOT NEW.result_attempt_no
OR OLD.plan_digest IS NOT NEW.plan_digest OR OLD.plan_count IS NOT NEW.plan_count
OR OLD.parsed_output_digest IS NOT NEW.parsed_output_digest OR OLD.plan_saved_at IS NOT NEW.plan_saved_at)
AND EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_proof_guard BEFORE UPDATE OF projections_removed_at ON prepare_liquibase_job_finalizations
WHEN OLD.projections_removed_at IS NOT NULL OR NEW.projections_removed_at IS NULL
 OR NOT EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id AND j.prepare_kind='lb' AND j.status IN ('queued','running','recovery_required'))
BEGIN SELECT RAISE(ABORT,'Liquibase projection proof is set once before completion'); END;

CREATE TRIGGER prepare_lb_finalization_publish_decision BEFORE INSERT ON prepare_attempt_decisions
WHEN NEW.choice='publish_original' AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=NEW.job_id AND selected.outcome_kind='failure')
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure forbids publication'); END;

CREATE TRIGGER prepare_lb_finalization_published_resolution BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN NEW.resolution='published' AND OLD.resolution IS NOT NEW.resolution AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure')
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure forbids publication'); END;

CREATE TRIGGER prepare_lb_finalization_replay_permit BEFORE INSERT ON prepare_attempt_decisions
WHEN NEW.choice IN ('retry_original','replay_fresh') AND EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=NEW.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_request_delete_guard BEFORE DELETE ON prepare_liquibase_job_cancellations
WHEN EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'accepted Liquibase cancellation is retained with history'); END;

CREATE TRIGGER prepare_lb_finalization_request_immutable BEFORE UPDATE ON prepare_liquibase_job_cancellations
BEGIN SELECT RAISE(ABORT,'accepted Liquibase cancellation is immutable'); END;

CREATE TRIGGER prepare_lb_finalization_request_insert_guard BEFORE INSERT ON prepare_liquibase_job_cancellations
WHEN NOT EXISTS (SELECT 1 FROM prepare_jobs j JOIN liquibase_job_bindings b ON b.job_id=j.job_id
 WHERE j.job_id=NEW.job_id AND j.prepare_kind='lb' AND j.status IN ('queued','running'))
 OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=NEW.job_id)
BEGIN SELECT RAISE(ABORT,'Liquibase cancellation is no longer eligible'); END;

CREATE TRIGGER prepare_lb_finalization_restore_progress BEFORE UPDATE OF restore_phase,completed_at ON prepare_liquibase_cache_restores
WHEN EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure') AND (OLD.restore_phase IS NOT NEW.restore_phase OR OLD.completed_at IS NOT NEW.completed_at)
AND NOT (OLD.restore_phase IN ('requested','stopped','restored') AND OLD.completed_at IS NULL
AND NEW.restore_phase='abandoned' AND NEW.completed_at IS NOT NULL
AND EXISTS(SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=s.job_id
WHERE s.session_id=OLD.session_id AND s.job_id=OLD.job_id AND s.runner_phase='stopped' AND s.retired_at IS NULL
AND c.phase IN ('active','quiescent') AND selected.outcome_kind='failure' AND selected.projections_removed_at IS NULL
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=s.clone_id AND (a.phase<>'closed' OR a.resolution IS NULL))))
BEGIN SELECT RAISE(ABORT,'selected failure permits only exact stopped restore abandonment'); END;

CREATE TRIGGER prepare_lb_finalization_retain_liquibase_job_bindings BEFORE DELETE ON liquibase_job_bindings
WHEN EXISTS(SELECT 1 FROM prepare_jobs owner JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=owner.job_id
WHERE owner.job_id=OLD.job_id AND selected.outcome_kind='failure')
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its binding/source record'); END;

CREATE TRIGGER prepare_lb_finalization_retain_prepare_liquibase_cache_restores BEFORE DELETE ON prepare_liquibase_cache_restores
WHEN EXISTS(SELECT 1 FROM prepare_jobs owner JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=owner.job_id
WHERE owner.job_id=OLD.job_id AND selected.outcome_kind='failure')
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its binding/source record'); END;

CREATE TRIGGER prepare_lb_finalization_retain_prepare_liquibase_sessions BEFORE DELETE ON prepare_liquibase_sessions
WHEN EXISTS(SELECT 1 FROM prepare_jobs owner JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=owner.job_id
WHERE owner.job_id=OLD.job_id AND selected.outcome_kind='failure')
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its binding/source record'); END;

CREATE TRIGGER prepare_lb_finalization_retain_prepare_step_bindings BEFORE DELETE ON prepare_step_bindings
WHEN EXISTS(SELECT 1 FROM prepare_jobs owner JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=owner.job_id
WHERE owner.job_id=OLD.job_id AND selected.outcome_kind='failure')
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its binding/source record'); END;

CREATE TRIGGER prepare_lb_finalization_runtime_assignment BEFORE INSERT ON managed_runtime_operations
WHEN EXISTS (SELECT 1 FROM prepare_jobs j WHERE EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=j.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
AND substr(replace(NEW.target_path,char(92),'/'),-length('/jobs/'||j.job_id||'/'||NEW.operation_ref))='/jobs/'||j.job_id||'/'||NEW.operation_ref)
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_lb_finalization_session_key BEFORE UPDATE OF session_id,created_at ON prepare_liquibase_sessions
WHEN (OLD.session_id IS NOT NEW.session_id OR OLD.created_at IS NOT NEW.created_at) AND (EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure') OR EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=NEW.job_id AND selected.outcome_kind='failure'))
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its exact session'); END;

CREATE TRIGGER prepare_lb_finalization_task_definition BEFORE UPDATE OF job_id,task_id,position,type,planner_kind,input_kind,input_id,image_id,resolved_image_id,task_hash,output_state_id,cached,instance_mode,changeset_id,changeset_author,changeset_path ON prepare_tasks
WHEN (EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure') OR EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=NEW.job_id AND selected.outcome_kind='failure')) AND (
OLD.job_id IS NOT NEW.job_id OR OLD.task_id IS NOT NEW.task_id OR OLD.position IS NOT NEW.position OR OLD.type IS NOT NEW.type
OR OLD.planner_kind IS NOT NEW.planner_kind OR OLD.input_kind IS NOT NEW.input_kind OR OLD.input_id IS NOT NEW.input_id
OR OLD.image_id IS NOT NEW.image_id OR OLD.resolved_image_id IS NOT NEW.resolved_image_id OR OLD.task_hash IS NOT NEW.task_hash
OR OLD.output_state_id IS NOT NEW.output_state_id OR OLD.cached IS NOT NEW.cached OR OLD.instance_mode IS NOT NEW.instance_mode
OR OLD.changeset_id IS NOT NEW.changeset_id OR OLD.changeset_author IS NOT NEW.changeset_author OR OLD.changeset_path IS NOT NEW.changeset_path)
BEGIN SELECT RAISE(ABORT,'selected Liquibase failure retains its task definition'); END;

CREATE TRIGGER prepare_lb_finalization_task_delete BEFORE DELETE ON prepare_tasks
WHEN EXISTS (SELECT 1 FROM prepare_jobs authority_job WHERE authority_job.job_id=OLD.job_id AND authority_job.prepare_kind='lb' AND (
authority_job.status IN ('succeeded','failed')
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=authority_job.job_id)
OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=authority_job.job_id)))
BEGIN SELECT RAISE(ABORT,'Liquibase work is frozen'); END;

CREATE TRIGGER prepare_liquibase_invocation_completion_guard
BEFORE UPDATE OF exit_code,completion_digest,completion_bytes,complete_at ON prepare_liquibase_invocations
WHEN OLD.complete_at IS NOT NULL OR OLD.start_requested_at IS NULL
  OR NEW.complete_at IS NULL
  OR NOT EXISTS (SELECT 1 FROM prepare_step_attempts a
    WHERE a.job_id=OLD.job_id AND a.task_id=OLD.task_id AND a.attempt_no=OLD.attempt_no
      AND a.phase='exec_requested' AND a.resolution IS NULL)
BEGIN SELECT RAISE(ABORT,'invocation completion is immutable and requires a start request'); END;

CREATE TRIGGER prepare_liquibase_invocation_delete_guard
BEFORE DELETE ON prepare_liquibase_invocations
WHEN EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'invocation evidence is retained with its job'); END;

CREATE TRIGGER prepare_liquibase_invocation_identity_immutable
BEFORE UPDATE OF job_id,task_id,attempt_no,session_id,invocation_ref,stdout_ref,
  stderr_ref,manifest_ref,helper_digest,protocol_revision ON prepare_liquibase_invocations
BEGIN SELECT RAISE(ABORT,'Liquibase invocation identity is immutable'); END;

CREATE TRIGGER prepare_liquibase_invocation_insert_guard
BEFORE INSERT ON prepare_liquibase_invocations
WHEN NEW.start_requested_at IS NOT NULL OR NEW.complete_at IS NOT NULL OR NEW.retired_at IS NOT NULL
  OR NOT EXISTS (SELECT 1 FROM prepare_step_attempts a
  JOIN prepare_liquibase_sessions s ON s.session_id=NEW.session_id
  JOIN prepare_execution_bindings b ON b.execution_digest=a.execution_digest
  JOIN prepare_tasks t ON t.job_id=a.job_id AND t.task_id=a.task_id
  WHERE a.job_id=NEW.job_id AND a.task_id=NEW.task_id
    AND a.attempt_no=NEW.attempt_no AND a.executor_kind='session_exec'
    AND a.clone_id=s.clone_id AND a.worker_generation=s.worker_generation
    AND s.job_id=a.job_id AND s.retired_at IS NULL
    AND ((s.runner_phase='running' AND s.runner_id IS NOT NULL) OR
 (s.runner_phase='planned' AND s.runner_id IS NULL AND a.phase='planned' AND EXISTS (
 SELECT 1 FROM prepare_attempt_decisions d JOIN prepare_step_attempts prior ON prior.job_id=d.job_id AND prior.task_id=d.task_id AND prior.attempt_no=d.attempt_no
 WHERE d.job_id=a.job_id AND d.task_id=a.task_id AND d.target_clone_id=a.clone_id
 AND d.choice='replay_fresh' AND d.next_attempt_no=a.attempt_no AND d.worker_generation=a.worker_generation
 AND prior.executor_kind='session_exec' AND prior.phase='closed' AND prior.resolution IN ('discarded_clone','never_started'))))
    AND s.db_phase='running' AND s.capture_helper_digest=NEW.helper_digest
    AND b.adapter_binding_digest=s.execution_fingerprint
    AND t.position=s.cursor_position
    AND ((t.input_kind='image' AND t.input_id=s.cursor_image_id)
      OR (t.input_kind='state' AND t.input_id=s.cursor_state_id)))
BEGIN SELECT RAISE(ABORT,'invocation is not bound to the live session'); END;

CREATE TRIGGER prepare_liquibase_invocation_restore_guard BEFORE INSERT ON prepare_liquibase_invocations
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=NEW.session_id AND r.restore_phase NOT IN ('published','abandoned'))
BEGIN SELECT RAISE(ABORT,'invocation overlaps a pending restore'); END;

CREATE TRIGGER prepare_liquibase_invocation_retirement_guard
BEFORE UPDATE OF retired_at ON prepare_liquibase_invocations
WHEN OLD.retired_at IS NOT NULL OR NEW.retired_at IS NULL
  OR NOT EXISTS (SELECT 1 FROM prepare_liquibase_sessions s
    JOIN prepare_step_attempts a ON a.job_id=OLD.job_id AND a.task_id=OLD.task_id AND a.attempt_no=OLD.attempt_no
    WHERE s.session_id=OLD.session_id AND s.runner_phase='stopped' AND a.phase='closed' AND a.resolution IS NOT NULL)
BEGIN SELECT RAISE(ABORT,'unresolved invocation retains private capture'); END;

CREATE TRIGGER prepare_liquibase_invocation_start_guard
BEFORE UPDATE OF start_requested_at ON prepare_liquibase_invocations
WHEN OLD.start_requested_at IS NOT NULL OR NEW.start_requested_at IS NULL
  OR NOT EXISTS (SELECT 1 FROM prepare_step_attempts a
    JOIN prepare_liquibase_sessions s ON s.session_id=OLD.session_id
    WHERE a.job_id=OLD.job_id AND a.task_id=OLD.task_id AND a.attempt_no=OLD.attempt_no
      AND a.phase='planned' AND a.resolution IS NULL
      AND s.runner_phase='running' AND s.db_phase='running' AND s.retired_at IS NULL)
BEGIN SELECT RAISE(ABORT,'invalid invocation start boundary'); END;

CREATE TRIGGER prepare_liquibase_private_disposal_attempt BEFORE INSERT ON prepare_step_attempts
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_liquibase_session_disposals d ON d.session_id=s.session_id WHERE s.clone_id=NEW.clone_id)
BEGIN SELECT RAISE(ABORT,'disposal forbids new working attempts'); END;

CREATE TRIGGER prepare_liquibase_private_disposal_db_absence BEFORE UPDATE OF db_absent_at ON prepare_liquibase_session_disposals
WHEN OLD.db_absent_at IS NOT NULL OR NEW.db_absent_at IS NULL OR NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_sessions s WHERE s.session_id=OLD.session_id AND s.runner_id IS OLD.runner_id
 AND (s.runner_phase='stopped' OR (OLD.reason='proven_nonstart' AND OLD.nonstart_basis='session_uninvoked'
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=s.session_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=s.clone_id AND a.task_id<>'plan'))))
BEGIN SELECT RAISE(ABORT,'database absence requires exact stopped or uninvoked session'); END;

CREATE TRIGGER prepare_liquibase_private_disposal_db_attach BEFORE UPDATE OF db_runtime_id ON prepare_clones
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_liquibase_session_disposals d ON d.session_id=s.session_id WHERE s.clone_id=OLD.clone_id)
BEGIN SELECT RAISE(ABORT,'disposal freezes database attachment'); END;

CREATE TRIGGER prepare_liquibase_private_disposal_delete BEFORE DELETE ON prepare_liquibase_session_disposals
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_jobs j ON j.job_id=s.job_id WHERE s.session_id=OLD.session_id)
BEGIN SELECT RAISE(ABORT,'session disposal is retained with its job'); END;

CREATE TRIGGER prepare_liquibase_private_disposal_identity
BEFORE UPDATE OF session_id,reason,authority_ref,requested_at,runner_id,db_runtime_id,nonstart_basis,nonstart_scope_digest,nonstart_observation_digest,nonstart_observed_at,handoff_job_id,handoff_instance_id,handoff_state_id,handoff_operation_ref,handoff_runtime_dir,handoff_outcome_digest
ON prepare_liquibase_session_disposals
BEGIN SELECT RAISE(ABORT,'Liquibase session disposal identity is immutable'); END;

CREATE TRIGGER prepare_liquibase_private_disposal_insert BEFORE INSERT ON prepare_liquibase_session_disposals
WHEN NEW.db_absent_at IS NOT NULL OR NEW.runner_absent_at IS NOT NULL
 OR EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals old WHERE old.session_id=NEW.session_id)
 OR NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_clones clone ON clone.clone_id=s.clone_id AND clone.job_id=s.job_id
 JOIN prepare_jobs j ON j.job_id=s.job_id JOIN liquibase_job_bindings binding ON binding.job_id=j.job_id AND binding.execution_fingerprint=s.execution_fingerprint
 WHERE s.session_id=NEW.session_id AND s.retired_at IS NULL AND s.capture_location='container_private' AND s.capture_protocol_revision='lb-step-capture-v2'
 AND s.runner_id IS NEW.runner_id AND clone.db_runtime_id IS NEW.db_runtime_id
 AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0 AND j.status IN ('queued','running','recovery_required')
 AND (
 (NEW.reason='instance_transferred' AND NEW.handoff_job_id=j.job_id AND NEW.handoff_state_id=s.cursor_state_id
 AND s.runner_phase='stopped' AND s.db_phase='running' AND clone.phase='active'
 AND EXISTS (SELECT 1 FROM prepare_tasks t JOIN prepare_liquibase_invocations i USING(job_id,task_id)
 JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 WHERE t.job_id=j.job_id AND t.position=s.cursor_position-1 AND t.output_state_id=s.cursor_state_id
 AND t.type='state_execute' AND t.status='succeeded' AND i.session_id=s.session_id
 AND a.clone_id=s.clone_id AND a.worker_generation=s.worker_generation AND a.phase='closed' AND a.resolution='published')
 AND EXISTS (SELECT 1 FROM states state JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id
 WHERE state.state_id=s.cursor_state_id AND state.prepare_kind='lb' AND lb.execution_fingerprint=s.execution_fingerprint
 AND state.image_id=j.resolved_image_id AND state.lineage_ref=j.lineage_ref AND state.identity_digest=j.identity_digest)
 AND EXISTS (SELECT 1 FROM managed_runtime_operations o JOIN managed_state_seals seal ON seal.operation_ref=o.operation_ref
 JOIN instance_access access ON access.instance_ref=NEW.handoff_instance_id AND access.stage='verified'
 WHERE o.operation_ref=NEW.handoff_operation_ref AND o.target_path=NEW.handoff_runtime_dir
 AND o.source_ref=s.start_id AND o.runtime_ref=clone.db_runtime_id AND o.stage='running'
 AND seal.state_id=s.cursor_state_id
 AND json_extract(seal.binding_json,'$.RuntimeRef')=o.runtime_ref
 AND json_extract(seal.binding_json,'$.PhysicalIdentity')=o.physical_identity
 AND json_extract(access.binding_json,'$.RuntimeRef')=o.runtime_ref
 AND json_extract(access.binding_json,'$.PhysicalIdentity')=o.physical_identity)
 AND NOT EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=j.job_id AND t.type='state_execute' AND t.status<>'succeeded')
 AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.job_id=j.job_id AND (a.phase<>'closed' OR a.resolution IS NULL))
 AND NOT EXISTS (SELECT 1 FROM prepare_clones c WHERE c.job_id=j.job_id AND c.clone_id<>s.clone_id AND c.phase<>'released')
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_sessions other WHERE other.job_id=j.job_id AND other.session_id<>s.session_id AND other.retired_at IS NULL)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.job_id=j.job_id AND r.restore_phase NOT IN ('published','abandoned'))
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations cancel WHERE cancel.job_id=j.job_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=j.job_id))
 OR
 (NEW.reason='accepted_cancel' AND NEW.authority_ref=j.job_id AND EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=j.job_id))
 OR (NEW.reason='selected_failure' AND EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=j.job_id AND f.outcome_kind='failure' AND f.outcome_digest=NEW.authority_ref))
 OR (NEW.reason='final_published' AND s.cursor_state_id=NEW.authority_ref
 AND EXISTS (SELECT 1 FROM states state JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id AND lb.execution_fingerprint=s.execution_fingerprint WHERE state.state_id=s.cursor_state_id AND state.prepare_kind='lb')
 AND NOT EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=j.job_id AND t.type='state_execute' AND t.status<>'succeeded')
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no) WHERE i.session_id=s.session_id AND (a.phase<>'closed' OR a.resolution IS NOT 'published'))
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores restore WHERE restore.session_id=s.session_id AND restore.restore_phase NOT IN ('published','abandoned'))
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=j.job_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=j.job_id AND f.outcome_kind IN ('failure','cancelled')))
 OR (NEW.reason='cache_selected' AND EXISTS (SELECT 1 FROM prepare_liquibase_cache_selections selection
 WHERE selection.session_id=s.session_id AND selection.clone_id=s.clone_id AND selection.worker_generation=s.worker_generation
 AND json_array(selection.job_id,selection.task_id,selection.attempt_no)=NEW.authority_ref
 AND selection.transferred_at IS NULL AND selection.released_at IS NULL))
 OR (NEW.reason='proven_nonstart' AND NEW.nonstart_basis='session_uninvoked' AND NEW.authority_ref=s.session_id
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=s.session_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=s.clone_id AND a.task_id<>'plan'))
 OR (NEW.reason='proven_nonstart' AND NEW.nonstart_basis='attempt' AND EXISTS (
 SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 JOIN prepare_attempt_evidence e USING(job_id,task_id,attempt_no)
 WHERE i.session_id=s.session_id AND a.clone_id=s.clone_id AND a.worker_generation=s.worker_generation
 AND e.worker_generation=s.worker_generation AND e.kind='never_started' AND e.evidence_id>0 AND CAST(e.evidence_id AS TEXT)=NEW.authority_ref
 AND e.scope_digest=NEW.nonstart_scope_digest AND e.facts_digest=NEW.nonstart_observation_digest AND e.observed_at=NEW.nonstart_observed_at))
 OR (NEW.reason='unknown_discard' AND EXISTS (
 SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 JOIN prepare_attempt_evidence stopped USING(job_id,task_id,attempt_no)
 JOIN prepare_attempt_evidence request USING(job_id,task_id,attempt_no,worker_generation)
 JOIN prepare_attempt_evidence ack USING(job_id,task_id,attempt_no,worker_generation)
 WHERE i.session_id=s.session_id AND a.clone_id=s.clone_id AND a.worker_generation=s.worker_generation
 AND stopped.worker_generation=s.worker_generation AND stopped.kind='executor_inspection' AND stopped.evidence_id>0
 AND CAST(stopped.evidence_id AS TEXT)=NEW.authority_ref AND request.kind='termination_requested' AND ack.kind='termination_acknowledged'
 AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id
 AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id
 AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id
 AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
 AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1))
 ))
BEGIN SELECT RAISE(ABORT,'invalid exact Liquibase session disposal authority'); END;

CREATE TRIGGER prepare_liquibase_private_disposal_runner_absence BEFORE UPDATE OF runner_absent_at ON prepare_liquibase_session_disposals
WHEN OLD.runner_absent_at IS NOT NULL OR NEW.runner_absent_at IS NULL OR (OLD.db_absent_at IS NULL AND NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_job_finalizations f JOIN instances i ON i.instance_id=f.instance_pin
 JOIN managed_runtime_operations o ON o.runtime_ref=i.runtime_id AND o.target_path=i.runtime_dir
 JOIN prepare_liquibase_sessions s ON s.session_id=OLD.session_id
 JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
 WHERE OLD.reason='instance_transferred' AND f.job_id=OLD.handoff_job_id AND s.job_id=f.job_id
 AND f.outcome_kind='prepare_success' AND f.outcome_digest=OLD.handoff_outcome_digest AND f.projections_removed_at IS NULL
 AND f.instance_id=OLD.handoff_instance_id AND f.final_state_id=OLD.handoff_state_id
 AND i.state_id=OLD.handoff_state_id AND i.runtime_id=OLD.db_runtime_id AND i.runtime_dir=OLD.handoff_runtime_dir
 AND o.operation_ref=OLD.handoff_operation_ref AND o.source_ref=s.start_id AND o.stage='running'
 AND s.runner_id=OLD.runner_id AND s.runner_phase='stopped' AND c.db_runtime_id=OLD.db_runtime_id))
BEGIN SELECT RAISE(ABORT,'runner absence follows saved database absence'); END;

CREATE TRIGGER prepare_liquibase_private_disposal_runner_attach BEFORE UPDATE OF runner_id ON prepare_liquibase_sessions
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=OLD.session_id)
BEGIN SELECT RAISE(ABORT,'disposal freezes runner attachment'); END;

CREATE TRIGGER prepare_liquibase_private_invocation_insert BEFORE INSERT ON prepare_liquibase_invocations
WHEN NEW.protocol_revision<>'lb-step-capture-v2' OR NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_liquibase_session_readiness ready
 ON ready.session_id=s.session_id AND ready.runner_id=s.runner_id
 JOIN prepare_liquibase_capture_slots slot ON slot.session_id=s.session_id
 WHERE s.session_id=NEW.session_id AND s.capture_location='container_private' AND s.capture_protocol_revision=NEW.protocol_revision
 AND slot.job_id=NEW.job_id AND slot.task_id=NEW.task_id AND slot.attempt_no=NEW.attempt_no
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=s.session_id))
BEGIN SELECT RAISE(ABORT,'private invocation lacks readiness or exact slot'); END;

CREATE TRIGGER prepare_liquibase_private_invocation_start BEFORE UPDATE OF start_requested_at ON prepare_liquibase_invocations
WHEN NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_liquibase_session_readiness ready
 ON ready.session_id=s.session_id AND ready.runner_id=s.runner_id
 JOIN prepare_liquibase_capture_slots slot ON slot.session_id=s.session_id
 WHERE s.session_id=OLD.session_id AND s.capture_location='container_private' AND s.capture_protocol_revision=OLD.protocol_revision
 AND slot.job_id=OLD.job_id AND slot.task_id=OLD.task_id AND slot.attempt_no=OLD.attempt_no
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=s.session_id)
 AND (slot.prior_retirement_ref IS NULL OR EXISTS (
 SELECT 1 FROM prepare_liquibase_capture_retirements prior WHERE prior.retirement_ref=slot.prior_retirement_ref
 AND prior.session_id=s.session_id AND prior.reason='published_prune'
 AND ((prior.verified_at IS NULL AND EXISTS (SELECT 1 FROM prepare_liquibase_capture_slots old
 WHERE old.session_id=s.session_id AND old.job_id=prior.job_id AND old.task_id=prior.task_id AND old.attempt_no=prior.attempt_no))
 OR (prior.verified_at IS NOT NULL AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_capture_slots old
 WHERE old.session_id=s.session_id AND old.job_id=prior.job_id AND old.task_id=prior.task_id AND old.attempt_no=prior.attempt_no))))))
BEGIN SELECT RAISE(ABORT,'private invocation start lacks exact saved reservation'); END;

CREATE TRIGGER prepare_liquibase_private_planning_disposal_delete BEFORE DELETE ON prepare_liquibase_planning_capture_disposals
WHEN EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'planning capture disposal is retained with its job'); END;

CREATE TRIGGER prepare_liquibase_private_planning_disposal_insert BEFORE INSERT ON prepare_liquibase_planning_capture_disposals
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_planning_capture_disposals old WHERE old.job_id=NEW.job_id AND old.task_id=NEW.task_id AND old.attempt_no=NEW.attempt_no)
 OR NOT EXISTS (
 SELECT 1 FROM prepare_planning_attempt_artifacts artifact JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 JOIN prepare_planning_intents intent ON intent.job_id=a.job_id AND intent.task_id=a.task_id
 JOIN prepare_jobs j ON j.job_id=a.job_id JOIN liquibase_job_bindings binding ON binding.job_id=j.job_id
 WHERE artifact.job_id=NEW.job_id AND artifact.task_id=NEW.task_id AND artifact.attempt_no=NEW.attempt_no
 AND artifact.capture_location='container_private' AND artifact.capture_protocol_revision='lb-capture-v2'
 AND a.task_id='plan' AND a.execution_digest=intent.execution_digest
 AND ((a.attempt_no=1 AND a.clone_id=intent.preview_clone_id)
 OR (a.attempt_no=2 AND EXISTS (
 SELECT 1 FROM prepare_step_attempts prior
 JOIN prepare_attempt_decisions replay ON replay.job_id=prior.job_id AND replay.task_id=prior.task_id AND replay.attempt_no=prior.attempt_no
 JOIN prepare_clones original ON original.clone_id=prior.clone_id AND original.job_id=prior.job_id
 JOIN prepare_clones replacement ON replacement.clone_id=a.clone_id AND replacement.job_id=a.job_id
 WHERE prior.job_id=a.job_id AND prior.task_id=a.task_id AND prior.attempt_no=1
 AND prior.clone_id=intent.preview_clone_id AND prior.execution_digest=a.execution_digest
 AND prior.worker_generation=a.worker_generation AND replay.worker_generation=a.worker_generation
 AND prior.phase='closed' AND prior.resolution IN ('discarded_clone','never_started')
 AND replay.choice='replay_fresh' AND replay.next_attempt_no=2 AND replay.target_clone_id=a.clone_id
 AND replacement.source_kind=original.source_kind
 AND replacement.source_image_id IS original.source_image_id AND replacement.source_state_id IS original.source_state_id)))
 AND j.prepare_kind='lb' AND j.purpose IN ('prepare','cache_explain') AND j.status IN ('queued','running','recovery_required')
 AND ((NEW.reason='plan_saved' AND intent.result_attempt_no=a.attempt_no AND intent.plan_digest=NEW.authority_ref
 AND intent.plan_saved_at IS NOT NULL AND a.phase='closed' AND a.resolution='planning_result_saved')
 OR (NEW.reason='selected_failure' AND EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=j.job_id AND f.outcome_kind='failure' AND f.outcome_digest=NEW.authority_ref))
 OR (NEW.reason='accepted_cancel' AND NEW.authority_ref=j.job_id AND EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=j.job_id))
 OR (NEW.reason='common_discard' AND EXISTS (SELECT 1 FROM prepare_attempt_evidence e
 WHERE e.job_id=a.job_id AND e.task_id=a.task_id AND e.attempt_no=a.attempt_no AND e.worker_generation=a.worker_generation
 AND e.evidence_id>0 AND CAST(e.evidence_id AS TEXT)=NEW.authority_ref
 AND json_extract(e.facts_json,'$.clone_id')=a.clone_id AND json_extract(e.facts_json,'$.execution_digest')=a.execution_digest
 AND ((e.kind='never_started') OR (e.kind='executor_inspection'
 AND json_extract(e.facts_json,'$.observation.executor_id')=a.executor_id
 AND json_type(e.facts_json,'$.observation.exit_code')='integer'
 AND json_extract(e.facts_json,'$.observation.exit_code') BETWEEN 0 AND 255)))))
 )
BEGIN SELECT RAISE(ABORT,'invalid exact planning capture disposal authority'); END;

CREATE TRIGGER prepare_liquibase_private_planning_disposal_update BEFORE UPDATE ON prepare_liquibase_planning_capture_disposals
BEGIN SELECT RAISE(ABORT,'planning capture disposal is immutable'); END;

CREATE TRIGGER prepare_liquibase_private_planning_insert BEFORE INSERT ON prepare_planning_attempt_artifacts
WHEN NEW.capture_location<>'container_private' OR NEW.capture_protocol_revision<>'lb-capture-v2'
BEGIN SELECT RAISE(ABORT,'new planning capture must use private v2'); END;

CREATE TRIGGER prepare_liquibase_private_planning_location BEFORE UPDATE OF capture_location ON prepare_planning_attempt_artifacts
BEGIN SELECT RAISE(ABORT,'planning capture location is immutable'); END;

CREATE TRIGGER prepare_liquibase_private_planning_retirement
BEFORE UPDATE OF executor_retired_at,output_retired_at,stderr_retired_at,manifest_retired_at ON prepare_planning_attempt_artifacts
WHEN OLD.capture_location='container_private' AND (
 NEW.executor_retired_at IS NULL OR NEW.output_retired_at IS NULL OR NEW.stderr_retired_at IS NULL OR NEW.manifest_retired_at IS NULL
 OR NOT EXISTS (SELECT 1 FROM prepare_liquibase_planning_capture_disposals intent
 WHERE intent.job_id=OLD.job_id AND intent.task_id=OLD.task_id AND intent.attempt_no=OLD.attempt_no))
BEGIN SELECT RAISE(ABORT,'private planning retirement requires exact disposal and complete absence'); END;

CREATE TRIGGER prepare_liquibase_private_ready_delete BEFORE DELETE ON prepare_liquibase_session_readiness
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_jobs j ON j.job_id=s.job_id WHERE s.session_id=OLD.session_id)
BEGIN SELECT RAISE(ABORT,'Liquibase readiness is retained with its job'); END;

CREATE TRIGGER prepare_liquibase_private_ready_insert BEFORE INSERT ON prepare_liquibase_session_readiness
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_session_readiness r WHERE r.session_id=NEW.session_id)
 OR NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_jobs j ON j.job_id=s.job_id
 JOIN liquibase_job_bindings b ON b.job_id=j.job_id AND b.execution_fingerprint=s.execution_fingerprint
 WHERE s.session_id=NEW.session_id AND s.runner_id=NEW.runner_id AND s.runner_phase='running' AND s.retired_at IS NULL
 AND s.capture_location='container_private' AND s.capture_protocol_revision='lb-step-capture-v2'
 AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0 AND j.status IN ('queued','running','recovery_required')
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=s.session_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=j.job_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=j.job_id))
BEGIN SELECT RAISE(ABORT,'invalid exact Liquibase readiness'); END;

CREATE TRIGGER prepare_liquibase_private_ready_update BEFORE UPDATE ON prepare_liquibase_session_readiness
BEGIN SELECT RAISE(ABORT,'Liquibase readiness is immutable'); END;

CREATE TRIGGER prepare_liquibase_private_retirement_delete BEFORE DELETE ON prepare_liquibase_capture_retirements
WHEN EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'capture retirement history is retained with its job'); END;

CREATE TRIGGER prepare_liquibase_private_retirement_identity
BEFORE UPDATE OF retirement_ref,job_id,task_id,attempt_no,session_id,reason,requested_at ON prepare_liquibase_capture_retirements
BEGIN SELECT RAISE(ABORT,'capture retirement identity is immutable'); END;

CREATE TRIGGER prepare_liquibase_private_retirement_insert BEFORE INSERT ON prepare_liquibase_capture_retirements
WHEN NEW.verified_at IS NOT NULL OR NEW.absence_kind IS NOT NULL
 OR EXISTS (SELECT 1 FROM prepare_liquibase_capture_retirements r WHERE r.retirement_ref=NEW.retirement_ref
 OR (r.job_id=NEW.job_id AND r.task_id=NEW.task_id AND r.attempt_no=NEW.attempt_no))
 OR NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_liquibase_capture_slots slot
 ON slot.job_id=i.job_id AND slot.task_id=i.task_id AND slot.attempt_no=i.attempt_no AND slot.session_id=i.session_id
 JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id
 WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id AND i.attempt_no=NEW.attempt_no AND i.session_id=NEW.session_id
 AND s.capture_location='container_private' AND s.capture_protocol_revision='lb-step-capture-v2' AND i.protocol_revision=s.capture_protocol_revision
 AND ((NEW.reason='session_disposal' AND EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=s.session_id))
 OR (NEW.reason='published_prune' AND i.complete_at IS NOT NULL AND i.exit_code=0
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=s.session_id)
 AND EXISTS (
 SELECT 1 FROM prepare_step_attempts a JOIN prepare_tasks t ON t.job_id=a.job_id AND t.task_id=a.task_id
 JOIN states state ON state.state_id=t.output_state_id
 JOIN prepare_state_bindings b ON b.state_id=state.state_id AND b.execution_digest=a.execution_digest
 JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id AND lb.execution_fingerprint=s.execution_fingerprint
 JOIN prepare_attempt_decisions decision ON decision.job_id=a.job_id AND decision.task_id=a.task_id AND decision.attempt_no=a.attempt_no
 WHERE a.job_id=i.job_id AND a.task_id=i.task_id AND a.attempt_no=i.attempt_no AND a.clone_id=s.clone_id
 AND a.worker_generation=s.worker_generation AND a.executor_kind='session_exec' AND a.phase='closed' AND a.resolution='published'
 AND t.status='succeeded' AND t.type='state_execute' AND s.cursor_position=t.position+1 AND s.cursor_state_id=t.output_state_id
 AND state.prepare_kind='lb' AND decision.choice='publish_original')
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores restore WHERE restore.session_id=s.session_id AND restore.restore_phase NOT IN ('published','abandoned')))))
BEGIN SELECT RAISE(ABORT,'capture retirement lacks exact published or disposal authority'); END;

CREATE TRIGGER prepare_liquibase_private_retirement_observation
BEFORE UPDATE OF verified_at,absence_kind ON prepare_liquibase_capture_retirements
WHEN OLD.verified_at IS NOT NULL OR OLD.absence_kind IS NOT NULL OR NEW.verified_at IS NULL OR NEW.absence_kind IS NULL
 OR NOT EXISTS (SELECT 1 FROM prepare_liquibase_capture_slots slot WHERE slot.session_id=OLD.session_id
 AND slot.job_id=OLD.job_id AND slot.task_id=OLD.task_id AND slot.attempt_no=OLD.attempt_no)
 OR (NEW.absence_kind='path_absent' AND OLD.reason<>'published_prune')
 OR (NEW.absence_kind='container_absent' AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d
 WHERE d.session_id=OLD.session_id AND d.runner_absent_at IS NOT NULL
 AND (d.db_absent_at IS NOT NULL OR (d.reason='instance_transferred' AND EXISTS (
 SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=d.handoff_job_id
 AND f.instance_pin=d.handoff_instance_id AND f.outcome_digest=d.handoff_outcome_digest)))))
BEGIN SELECT RAISE(ABORT,'invalid exact capture retirement observation'); END;

CREATE TRIGGER prepare_liquibase_private_session_insert BEFORE INSERT ON prepare_liquibase_sessions
WHEN NEW.capture_location<>'container_private' OR NEW.capture_protocol_revision<>'lb-step-capture-v2'
BEGIN SELECT RAISE(ABORT,'new Liquibase session must use private v2'); END;

CREATE TRIGGER prepare_liquibase_private_session_location BEFORE UPDATE OF capture_location,capture_protocol_revision ON prepare_liquibase_sessions
BEGIN SELECT RAISE(ABORT,'Liquibase capture placement and protocol are immutable'); END;

CREATE TRIGGER prepare_liquibase_private_session_retired BEFORE UPDATE OF retired_at ON prepare_liquibase_sessions
WHEN NEW.retired_at IS NOT NULL AND OLD.capture_location='container_private'
 AND (EXISTS (SELECT 1 FROM prepare_liquibase_capture_slots slot WHERE slot.session_id=OLD.session_id)
 OR EXISTS (SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id AND NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_capture_retirements r WHERE r.job_id=i.job_id AND r.task_id=i.task_id AND r.attempt_no=i.attempt_no
 AND r.session_id=i.session_id AND r.verified_at IS NOT NULL AND r.absence_kind IS NOT NULL))
 OR NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=OLD.session_id
 AND d.runner_absent_at IS NOT NULL AND (d.db_absent_at IS NOT NULL OR (d.reason='instance_transferred' AND EXISTS (
 SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=d.handoff_job_id
 AND f.instance_pin=d.handoff_instance_id AND f.outcome_digest=d.handoff_outcome_digest)))))
BEGIN SELECT RAISE(ABORT,'private session retains capture or disposal ownership'); END;

CREATE TRIGGER prepare_liquibase_handoff_outcome BEFORE INSERT ON prepare_liquibase_job_finalizations
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.handoff_job_id=NEW.job_id
 AND (NEW.outcome_kind<>'prepare_success' OR NEW.instance_id IS NOT d.handoff_instance_id
 OR NEW.instance_pin IS NOT d.handoff_instance_id OR NEW.final_state_id IS NOT d.handoff_state_id
 OR NEW.outcome_digest IS NOT d.handoff_outcome_digest OR NEW.created_at IS NOT d.requested_at))
BEGIN SELECT RAISE(ABORT,'working transfer requires its exact future success outcome'); END;

CREATE TRIGGER prepare_liquibase_handoff_instance BEFORE INSERT ON instances
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.handoff_instance_id=NEW.instance_id
 AND (NEW.state_id IS NOT d.handoff_state_id OR NEW.runtime_id IS NOT d.db_runtime_id
 OR NEW.runtime_dir IS NOT d.handoff_runtime_dir OR NEW.created_at IS NOT d.requested_at OR NEW.status IS NOT 'active'))
BEGIN SELECT RAISE(ABORT,'working transfer requires its exact live instance'); END;

CREATE TRIGGER prepare_liquibase_handoff_resume BEFORE UPDATE OF db_phase ON prepare_liquibase_sessions
WHEN OLD.runner_phase='stopped' AND NEW.db_phase<>OLD.db_phase AND NEW.db_phase IN ('restart_requested','running')
AND (OLD.retired_at IS NOT NULL OR OLD.cursor_state_id IS NULL
 OR NOT EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=OLD.job_id AND t.position=OLD.cursor_position-1
 AND t.type='state_execute' AND t.status='succeeded' AND t.output_state_id=OLD.cursor_state_id)
 OR EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=OLD.job_id AND t.type='state_execute' AND t.status<>'succeeded')
 OR EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id AND (a.phase<>'closed' OR a.resolution IS NOT 'published'))
 OR EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=OLD.job_id)
 OR EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=OLD.job_id))
BEGIN SELECT RAISE(ABORT,'publication-only resume requires completed suffix'); END;

CREATE TRIGGER prepare_liquibase_private_slot_delete BEFORE DELETE ON prepare_liquibase_capture_slots
WHEN EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_capture_retirements r WHERE r.session_id=OLD.session_id
 AND r.job_id=OLD.job_id AND r.task_id=OLD.task_id AND r.attempt_no=OLD.attempt_no AND r.verified_at IS NOT NULL AND r.absence_kind IS NOT NULL)
BEGIN SELECT RAISE(ABORT,'unconfirmed capture retains its slot'); END;

CREATE TRIGGER prepare_liquibase_private_slot_insert BEFORE INSERT ON prepare_liquibase_capture_slots
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_capture_slots x WHERE x.session_id=NEW.session_id AND x.slot_no=NEW.slot_no)
 OR EXISTS (SELECT 1 FROM prepare_liquibase_capture_slots x WHERE x.job_id=NEW.job_id AND x.task_id=NEW.task_id AND x.attempt_no=NEW.attempt_no)
 OR NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_jobs j ON j.job_id=s.job_id
 JOIN prepare_liquibase_session_readiness r ON r.session_id=s.session_id AND r.runner_id=s.runner_id
 JOIN prepare_step_attempts a ON a.job_id=j.job_id AND a.task_id=NEW.task_id AND a.attempt_no=NEW.attempt_no
 WHERE s.session_id=NEW.session_id AND s.job_id=NEW.job_id AND s.retired_at IS NULL AND s.runner_phase='running' AND s.db_phase='running'
 AND s.capture_location='container_private' AND s.capture_protocol_revision='lb-step-capture-v2'
 AND a.clone_id=s.clone_id AND a.worker_generation=s.worker_generation AND a.executor_kind='session_exec' AND a.phase='planned' AND a.resolution IS NULL
 AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0 AND j.status IN ('queued','running','recovery_required')
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=s.session_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=j.job_id)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=j.job_id))
 OR ((SELECT count(*) FROM prepare_liquibase_capture_slots x WHERE x.session_id=NEW.session_id)=0 AND NEW.prior_retirement_ref IS NOT NULL)
 OR ((SELECT count(*) FROM prepare_liquibase_capture_slots x WHERE x.session_id=NEW.session_id)=1 AND NOT EXISTS (
 SELECT 1 FROM prepare_liquibase_capture_slots old JOIN prepare_liquibase_capture_retirements intent
 ON intent.job_id=old.job_id AND intent.task_id=old.task_id AND intent.attempt_no=old.attempt_no AND intent.session_id=old.session_id
 JOIN prepare_liquibase_invocations i ON i.job_id=old.job_id AND i.task_id=old.task_id AND i.attempt_no=old.attempt_no
 JOIN prepare_step_attempts a ON a.job_id=i.job_id AND a.task_id=i.task_id AND a.attempt_no=i.attempt_no
 WHERE old.session_id=NEW.session_id AND old.slot_no<>NEW.slot_no AND intent.retirement_ref=NEW.prior_retirement_ref
 AND intent.reason='published_prune' AND intent.verified_at IS NULL
 AND a.phase='closed' AND a.resolution='published' AND i.complete_at IS NOT NULL AND i.exit_code=0))
 OR (SELECT count(*) FROM prepare_liquibase_capture_slots x WHERE x.session_id=NEW.session_id)>=2
BEGIN SELECT RAISE(ABORT,'invalid exact Liquibase capture reservation'); END;

CREATE TRIGGER prepare_liquibase_private_slot_update BEFORE UPDATE ON prepare_liquibase_capture_slots
BEGIN SELECT RAISE(ABORT,'Liquibase capture reservation is immutable'); END;

CREATE TRIGGER prepare_liquibase_restore_identity_guard BEFORE UPDATE OF job_id,task_id,session_id,state_id,requested_at ON prepare_liquibase_cache_restores
BEGIN SELECT RAISE(ABORT,'restore source is immutable'); END;

CREATE TRIGGER prepare_liquibase_restore_insert_guard BEFORE INSERT ON prepare_liquibase_cache_restores
WHEN NEW.restore_phase<>'requested' OR NEW.completed_at IS NOT NULL OR NOT EXISTS (
SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_jobs j ON j.job_id=s.job_id
JOIN prepare_tasks t ON t.job_id=j.job_id AND t.task_id=NEW.task_id
JOIN states state ON state.state_id=t.output_state_id
JOIN prepare_step_bindings step ON step.job_id=t.job_id AND step.task_id=t.task_id
JOIN prepare_state_bindings binding ON binding.state_id=state.state_id AND binding.execution_digest=step.execution_digest
JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id AND lb.execution_fingerprint=s.execution_fingerprint
WHERE s.session_id=NEW.session_id AND s.job_id=NEW.job_id AND (s.runner_phase='running' OR (s.runner_phase='stopped' AND EXISTS (
SELECT 1 FROM prepare_step_attempts a JOIN prepare_liquibase_invocations i USING(job_id,task_id,attempt_no)
JOIN prepare_attempt_evidence selected ON selected.job_id=a.job_id AND selected.task_id=a.task_id AND selected.attempt_no=a.attempt_no
WHERE a.job_id=NEW.job_id AND a.task_id=NEW.task_id AND a.clone_id=s.clone_id AND i.session_id=s.session_id
AND a.executor_kind='session_exec' AND a.phase='closed' AND a.resolution='discarded_clone' AND a.worker_generation=s.worker_generation
AND selected.kind='cache_superseded' AND selected.worker_generation=a.worker_generation
AND json_extract(selected.facts_json,'$.observation.session_id')=s.session_id AND json_extract(selected.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(selected.facts_json,'$.observation.cached_state_id')=NEW.state_id))) AND s.retired_at IS NULL
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.status='running' AND t.type='state_execute' AND t.status='running'
AND t.position=s.cursor_position AND NEW.state_id=state.state_id AND state.state_fingerprint=state.state_id
AND ((t.input_kind='state' AND t.input_id=s.cursor_state_id AND state.parent_state_id=s.cursor_state_id)
OR (t.input_kind='image' AND t.input_id=s.cursor_image_id AND state.parent_state_id IS NULL)))
OR EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.job_id=NEW.job_id AND a.resolution IS NULL)
BEGIN SELECT RAISE(ABORT,'cache restore source, phase or writer differs'); END;

CREATE TRIGGER prepare_liquibase_restore_phase_guard BEFORE UPDATE OF restore_phase ON prepare_liquibase_cache_restores
WHEN NOT ((OLD.restore_phase='requested' AND NEW.restore_phase='stopped') OR (OLD.restore_phase='stopped' AND NEW.restore_phase='restored')
OR (OLD.restore_phase='restored' AND NEW.restore_phase='published' AND NEW.completed_at IS NOT NULL)
OR (OLD.restore_phase IN ('requested','stopped','restored') AND NEW.restore_phase='abandoned'))
BEGIN SELECT RAISE(ABORT,'invalid restore phase transition'); END;

CREATE TRIGGER prepare_liquibase_session_cursor_guard BEFORE UPDATE OF cursor_image_id,cursor_state_id,cursor_position ON prepare_liquibase_sessions WHEN (NOT (OLD.cursor_image_id IS NEW.cursor_image_id AND OLD.cursor_state_id IS NEW.cursor_state_id AND OLD.cursor_position=NEW.cursor_position) AND NOT (NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NOT NULL AND NEW.cursor_position=OLD.cursor_position+1 AND OLD.db_phase='stopped' AND EXISTS (SELECT 1 FROM prepare_tasks t JOIN prepare_step_attempts a ON a.job_id=t.job_id AND a.task_id=t.task_id JOIN prepare_liquibase_invocations i ON i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no JOIN states state ON state.state_id=t.output_state_id JOIN prepare_state_bindings binding ON binding.state_id=state.state_id AND binding.execution_digest=a.execution_digest WHERE t.job_id=OLD.job_id AND t.position=OLD.cursor_position AND t.type='state_execute' AND t.status='succeeded' AND t.output_state_id=NEW.cursor_state_id AND a.resolution='published' AND a.clone_id=OLD.clone_id AND i.session_id=OLD.session_id AND ((t.input_kind='image' AND t.input_id=OLD.cursor_image_id AND state.parent_state_id IS NULL) OR (t.input_kind='state' AND t.input_id=OLD.cursor_state_id AND state.parent_state_id=OLD.cursor_state_id)))) AND NOT (OLD.runner_phase='stopped' AND NEW.runner_phase='retired' AND NEW.retired_at IS NOT NULL AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL AND EXISTS (SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.phase='released')) AND NOT (OLD.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_position=OLD.cursor_position+1 AND EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r JOIN prepare_tasks t ON t.job_id=r.job_id AND t.task_id=r.task_id JOIN states state ON state.state_id=r.state_id JOIN prepare_state_bindings binding ON binding.state_id=state.state_id JOIN prepare_step_bindings step ON step.job_id=t.job_id AND step.task_id=t.task_id AND step.execution_digest=binding.execution_digest WHERE r.session_id=OLD.session_id AND r.restore_phase='published' AND t.status='succeeded' AND t.cached=1 AND t.position=OLD.cursor_position AND r.state_id=NEW.cursor_state_id AND ((t.input_kind='state' AND t.input_id=OLD.cursor_state_id AND state.parent_state_id=OLD.cursor_state_id) OR(t.input_kind='image' AND t.input_id=OLD.cursor_image_id AND state.parent_state_id IS NULL))))) AND NOT ((OLD.runner_phase='planned' AND OLD.runner_id IS NULL AND OLD.retired_at IS NULL
AND OLD.db_phase IN ('running','stopped') AND OLD.cursor_position=OLD.start_position
AND NEW.runner_phase='retired' AND NEW.runner_id IS NULL AND NEW.retired_at IS NOT NULL
AND NEW.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL
AND NEW.cursor_position=OLD.cursor_position AND NEW.created_at IS OLD.created_at
AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings binding ON binding.job_id=j.job_id
WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND selected.projections_removed_at IS NULL AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND j.status IN ('queued','running','recovery_required') AND binding.execution_fingerprint=OLD.execution_fingerprint)
AND EXISTS(SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.job_id=OLD.job_id AND c.phase='released'
AND c.source_kind=OLD.start_kind AND ((OLD.start_kind='image' AND c.source_image_id=OLD.start_id) OR (OLD.start_kind='state' AND c.source_state_id=OLD.start_id))
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(c.clone_ref,char(92),'/')))='/'||replace(c.clone_ref,char(92),'/')
AND substr(replace(c.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref))
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id)) OR (OLD.runner_phase='stopped' AND OLD.runner_id IS NOT NULL AND NEW.runner_id IS OLD.runner_id
AND OLD.retired_at IS NULL AND NEW.runner_phase='retired' AND NEW.retired_at IS NOT NULL
AND NEW.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL AND NEW.cursor_position=OLD.cursor_position
AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure' AND selected.projections_removed_at IS NULL)
AND EXISTS(SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.job_id=OLD.job_id AND c.phase='released'
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(c.clone_ref,char(92),'/')))='/'||replace(c.clone_ref,char(92),'/')
AND substr(replace(c.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref))
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id AND (a.phase<>'closed' OR a.resolution IS NULL
OR (a.resolution='discarded_clone' AND NOT (EXISTS(SELECT 1 FROM prepare_attempt_evidence p WHERE EXISTS (
SELECT 1 FROM prepare_liquibase_invocations i
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id AND s.job_id=i.job_id
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=s.job_id
JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN prepare_step_bindings sb ON sb.job_id=i.job_id AND sb.task_id=i.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
JOIN prepare_attempt_evidence request ON request.evidence_id=json_extract(p.facts_json,'$.observation.request_id')
JOIN prepare_attempt_evidence ack ON ack.evidence_id=json_extract(p.facts_json,'$.observation.ack_id')
JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=json_extract(p.facts_json,'$.observation.stop_id')
JOIN prepare_attempt_evidence q ON q.evidence_id=json_extract(p.facts_json,'$.observation.database_id')
WHERE i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no
AND a.executor_kind='session_exec' AND a.legacy_unbound=0
AND s.clone_id=a.clone_id AND s.worker_generation=a.worker_generation
AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND b.execution_digest=a.execution_digest AND b.prepare_kind='lb'
AND b.adapter_binding_digest=s.execution_fingerprint AND s.execution_fingerprint=lb.execution_fingerprint
AND b.policy_revision=lb.execution_policy_revision AND b.input_manifest_digest=lb.bundle_manifest_digest
AND b.target_identity_digest=lb.target_identity_digest AND b.executor_image_digest=lb.image_manifest_digest
AND p.job_id=a.job_id AND p.task_id=a.task_id AND p.attempt_no=a.attempt_no
AND p.worker_generation=a.worker_generation AND p.kind='selected_failure_disposal'
AND json_extract(p.facts_json,'$.clone_id')=a.clone_id
AND json_extract(p.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(p.facts_json,'$.observation.outcome_digest')=selected.outcome_digest
AND json_extract(p.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(p.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(p.facts_json,'$.observation.invocation_ref')=i.invocation_ref
AND json_extract(p.facts_json,'$.observation.clone_generation')=c.clone_generation
AND json_extract(p.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(p.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(p.facts_json,'$.observation.executor_absent')=1
AND json_extract(p.facts_json,'$.observation.runtime_absent')=1
AND json_extract(p.facts_json,'$.observation.original_phase') IN ('planned','exec_requested','exec_completed')
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged'
AND stopped.kind='executor_inspection' AND q.kind='db_quiescence'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id AND p.evidence_id>q.evidence_id
AND request.job_id=a.job_id AND request.task_id=a.task_id AND request.attempt_no=a.attempt_no AND request.worker_generation=a.worker_generation
AND ack.job_id=a.job_id AND ack.task_id=a.task_id AND ack.attempt_no=a.attempt_no AND ack.worker_generation=a.worker_generation
AND stopped.job_id=a.job_id AND stopped.task_id=a.task_id AND stopped.attempt_no=a.attempt_no AND stopped.worker_generation=a.worker_generation
AND q.job_id=a.job_id AND q.task_id=a.task_id AND q.attempt_no=a.attempt_no AND q.worker_generation=a.worker_generation
AND json_extract(request.facts_json,'$.clone_id')=a.clone_id AND json_extract(request.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(ack.facts_json,'$.clone_id')=a.clone_id AND json_extract(ack.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(stopped.facts_json,'$.clone_id')=a.clone_id AND json_extract(stopped.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(q.facts_json,'$.clone_id')=a.clone_id AND json_extract(q.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.runtime_absent')=1))
AND (SELECT count(*) FROM prepare_attempt_evidence single WHERE single.job_id=a.job_id AND single.task_id=a.task_id AND single.attempt_no=a.attempt_no AND single.kind='selected_failure_disposal')=1))))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id AND i.retired_at IS NULL)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id AND r.restore_phase NOT IN ('published','abandoned'))))
BEGIN SELECT RAISE(ABORT,'session cursor requires atomic publication'); END;

CREATE TRIGGER prepare_liquibase_session_db_phase_guard BEFORE UPDATE OF db_phase ON prepare_liquibase_sessions WHEN (NOT (OLD.db_phase=NEW.db_phase OR (OLD.db_phase='running' AND NEW.db_phase='stopped' AND EXISTS ( SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a ON a.job_id=i.job_id AND a.task_id=i.task_id AND a.attempt_no=i.attempt_no JOIN prepare_attempt_evidence q ON q.job_id=a.job_id AND q.task_id=a.task_id AND q.attempt_no=a.attempt_no JOIN prepare_clones c ON c.clone_id=OLD.clone_id WHERE i.session_id=OLD.session_id AND a.phase='exec_completed' AND a.resolution IS NULL AND q.kind='db_quiescence' AND q.worker_generation=OLD.worker_generation AND json_extract(q.facts_json,'$.observation.session_id')=OLD.session_id AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id AND json_extract(q.facts_json,'$.observation.postgres_stopped')=1 AND json_extract(q.facts_json,'$.observation.runtime_present')=1)) OR (OLD.db_phase='stopped' AND NEW.db_phase='restart_requested' AND NOT EXISTS ( SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id AND r.restore_phase NOT IN ('published','abandoned')) AND NOT EXISTS ( SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id AND a.resolution IS NULL)) OR (OLD.db_phase='restart_requested' AND NEW.db_phase='running' AND EXISTS ( SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.phase='active' AND c.db_runtime_id IS NOT NULL))) AND NOT (OLD.db_phase='running' AND NEW.db_phase='stopped' AND EXISTS ( SELECT 1 FROM prepare_liquibase_cache_restores r JOIN prepare_clones c ON c.clone_id=OLD.clone_id WHERE r.session_id=OLD.session_id AND r.restore_phase='stopped' AND c.phase='quiescent' AND c.db_runtime_id IS NOT NULL) AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id AND a.resolution IS NULL))) AND NOT ((OLD.runner_phase='planned' AND OLD.runner_id IS NULL AND OLD.retired_at IS NULL
AND OLD.db_phase IN ('running','stopped') AND OLD.cursor_position=OLD.start_position
AND NEW.runner_phase='retired' AND NEW.runner_id IS NULL AND NEW.retired_at IS NOT NULL
AND NEW.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL
AND NEW.cursor_position=OLD.cursor_position AND NEW.created_at IS OLD.created_at
AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings binding ON binding.job_id=j.job_id
WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND selected.projections_removed_at IS NULL AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND j.status IN ('queued','running','recovery_required') AND binding.execution_fingerprint=OLD.execution_fingerprint)
AND EXISTS(SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.job_id=OLD.job_id AND c.phase='released'
AND c.source_kind=OLD.start_kind AND ((OLD.start_kind='image' AND c.source_image_id=OLD.start_id) OR (OLD.start_kind='state' AND c.source_state_id=OLD.start_id))
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(c.clone_ref,char(92),'/')))='/'||replace(c.clone_ref,char(92),'/')
AND substr(replace(c.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref))
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id)) OR (OLD.runner_phase='stopped' AND OLD.runner_id IS NOT NULL AND NEW.runner_id IS OLD.runner_id
AND OLD.retired_at IS NULL AND NEW.runner_phase='retired' AND NEW.retired_at IS NOT NULL
AND NEW.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL AND NEW.cursor_position=OLD.cursor_position
AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure' AND selected.projections_removed_at IS NULL)
AND EXISTS(SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.job_id=OLD.job_id AND c.phase='released'
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(c.clone_ref,char(92),'/')))='/'||replace(c.clone_ref,char(92),'/')
AND substr(replace(c.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref))
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id AND (a.phase<>'closed' OR a.resolution IS NULL
OR (a.resolution='discarded_clone' AND NOT (EXISTS(SELECT 1 FROM prepare_attempt_evidence p WHERE EXISTS (
SELECT 1 FROM prepare_liquibase_invocations i
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id AND s.job_id=i.job_id
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=s.job_id
JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN prepare_step_bindings sb ON sb.job_id=i.job_id AND sb.task_id=i.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
JOIN prepare_attempt_evidence request ON request.evidence_id=json_extract(p.facts_json,'$.observation.request_id')
JOIN prepare_attempt_evidence ack ON ack.evidence_id=json_extract(p.facts_json,'$.observation.ack_id')
JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=json_extract(p.facts_json,'$.observation.stop_id')
JOIN prepare_attempt_evidence q ON q.evidence_id=json_extract(p.facts_json,'$.observation.database_id')
WHERE i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no
AND a.executor_kind='session_exec' AND a.legacy_unbound=0
AND s.clone_id=a.clone_id AND s.worker_generation=a.worker_generation
AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND b.execution_digest=a.execution_digest AND b.prepare_kind='lb'
AND b.adapter_binding_digest=s.execution_fingerprint AND s.execution_fingerprint=lb.execution_fingerprint
AND b.policy_revision=lb.execution_policy_revision AND b.input_manifest_digest=lb.bundle_manifest_digest
AND b.target_identity_digest=lb.target_identity_digest AND b.executor_image_digest=lb.image_manifest_digest
AND p.job_id=a.job_id AND p.task_id=a.task_id AND p.attempt_no=a.attempt_no
AND p.worker_generation=a.worker_generation AND p.kind='selected_failure_disposal'
AND json_extract(p.facts_json,'$.clone_id')=a.clone_id
AND json_extract(p.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(p.facts_json,'$.observation.outcome_digest')=selected.outcome_digest
AND json_extract(p.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(p.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(p.facts_json,'$.observation.invocation_ref')=i.invocation_ref
AND json_extract(p.facts_json,'$.observation.clone_generation')=c.clone_generation
AND json_extract(p.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(p.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(p.facts_json,'$.observation.executor_absent')=1
AND json_extract(p.facts_json,'$.observation.runtime_absent')=1
AND json_extract(p.facts_json,'$.observation.original_phase') IN ('planned','exec_requested','exec_completed')
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged'
AND stopped.kind='executor_inspection' AND q.kind='db_quiescence'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id AND p.evidence_id>q.evidence_id
AND request.job_id=a.job_id AND request.task_id=a.task_id AND request.attempt_no=a.attempt_no AND request.worker_generation=a.worker_generation
AND ack.job_id=a.job_id AND ack.task_id=a.task_id AND ack.attempt_no=a.attempt_no AND ack.worker_generation=a.worker_generation
AND stopped.job_id=a.job_id AND stopped.task_id=a.task_id AND stopped.attempt_no=a.attempt_no AND stopped.worker_generation=a.worker_generation
AND q.job_id=a.job_id AND q.task_id=a.task_id AND q.attempt_no=a.attempt_no AND q.worker_generation=a.worker_generation
AND json_extract(request.facts_json,'$.clone_id')=a.clone_id AND json_extract(request.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(ack.facts_json,'$.clone_id')=a.clone_id AND json_extract(ack.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(stopped.facts_json,'$.clone_id')=a.clone_id AND json_extract(stopped.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(q.facts_json,'$.clone_id')=a.clone_id AND json_extract(q.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.runtime_absent')=1))
AND (SELECT count(*) FROM prepare_attempt_evidence single WHERE single.job_id=a.job_id AND single.task_id=a.task_id AND single.attempt_no=a.attempt_no AND single.kind='selected_failure_disposal')=1))))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id AND i.retired_at IS NULL)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id AND r.restore_phase NOT IN ('published','abandoned'))))
BEGIN SELECT RAISE(ABORT,'invalid session database phase transition'); END;

CREATE TRIGGER prepare_liquibase_session_delete_guard
BEFORE DELETE ON prepare_liquibase_sessions
WHEN OLD.retired_at IS NULL
BEGIN SELECT RAISE(ABORT,'live Liquibase session is retained'); END;

CREATE TRIGGER prepare_liquibase_session_identity_immutable
BEFORE UPDATE OF job_id,session_no,clone_id,worker_generation,execution_fingerprint,
  start_kind,start_id,start_position,planned_runner_name,runner_config_digest,
  capture_helper_digest,protected_config_ref,runner_id ON prepare_liquibase_sessions
WHEN OLD.job_id IS NOT NEW.job_id OR OLD.session_no IS NOT NEW.session_no
  OR OLD.clone_id IS NOT NEW.clone_id
  OR OLD.worker_generation IS NOT NEW.worker_generation
  OR OLD.execution_fingerprint IS NOT NEW.execution_fingerprint
  OR OLD.start_kind IS NOT NEW.start_kind OR OLD.start_id IS NOT NEW.start_id
  OR OLD.start_position IS NOT NEW.start_position
  OR OLD.planned_runner_name IS NOT NEW.planned_runner_name
  OR OLD.runner_config_digest IS NOT NEW.runner_config_digest
  OR OLD.capture_helper_digest IS NOT NEW.capture_helper_digest
  OR OLD.protected_config_ref IS NOT NEW.protected_config_ref
  OR (OLD.runner_id IS NOT NULL AND OLD.runner_id IS NOT NEW.runner_id)
BEGIN SELECT RAISE(ABORT,'Liquibase session identity is immutable'); END;

CREATE TRIGGER prepare_liquibase_session_insert_guard
BEFORE INSERT ON prepare_liquibase_sessions
WHEN NEW.runner_phase<>'planned' OR NEW.runner_id IS NOT NULL OR NEW.retired_at IS NOT NULL
  OR NEW.cursor_position<>NEW.start_position
  OR (NEW.start_kind='image' AND (NEW.cursor_image_id IS NOT NEW.start_id OR NEW.cursor_state_id IS NOT NULL))
  OR (NEW.start_kind='state' AND (NEW.cursor_state_id IS NOT NEW.start_id OR NEW.cursor_image_id IS NOT NULL))
  OR NOT EXISTS (SELECT 1 FROM prepare_jobs j
  JOIN liquibase_job_bindings b ON b.job_id=j.job_id
  JOIN prepare_clones c ON c.job_id=j.job_id AND c.clone_id=NEW.clone_id
  WHERE j.job_id=NEW.job_id AND j.prepare_kind='lb' AND j.purpose='prepare'
    AND j.status IN ('queued','running','recovery_required')
    AND b.execution_fingerprint=NEW.execution_fingerprint
    AND c.source_kind=NEW.start_kind
    AND (c.source_image_id=NEW.start_id OR c.source_state_id=NEW.start_id))
BEGIN SELECT RAISE(ABORT,'invalid Liquibase session binding'); END;

CREATE TRIGGER prepare_liquibase_session_job_delete_guard
BEFORE DELETE ON prepare_jobs
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_sessions s WHERE s.job_id=OLD.job_id AND s.retired_at IS NULL)
BEGIN SELECT RAISE(ABORT,'live Liquibase session prevents job deletion'); END;

CREATE TRIGGER prepare_liquibase_session_job_terminal_guard
BEFORE UPDATE OF status ON prepare_jobs
WHEN NEW.status IN ('succeeded','failed') AND EXISTS (
  SELECT 1 FROM prepare_liquibase_sessions s WHERE s.job_id=OLD.job_id AND s.retired_at IS NULL)
BEGIN SELECT RAISE(ABORT,'live Liquibase session prevents terminal job status'); END;

CREATE TRIGGER prepare_liquibase_session_retirement_guard BEFORE UPDATE OF retired_at ON prepare_liquibase_sessions WHEN (OLD.retired_at IS NOT NULL OR NEW.retired_at IS NULL OR NEW.runner_phase<>'retired' OR OLD.runner_phase<>'stopped' OR EXISTS (SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id AND i.retired_at IS NULL) OR EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id AND a.resolution IS NULL) OR EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id AND r.restore_phase NOT IN ('published','abandoned')) OR NOT EXISTS (SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.phase='released')) AND NOT ((OLD.runner_phase='planned' AND OLD.runner_id IS NULL AND OLD.retired_at IS NULL
AND OLD.db_phase IN ('running','stopped') AND OLD.cursor_position=OLD.start_position
AND NEW.runner_phase='retired' AND NEW.runner_id IS NULL AND NEW.retired_at IS NOT NULL
AND NEW.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL
AND NEW.cursor_position=OLD.cursor_position AND NEW.created_at IS OLD.created_at
AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings binding ON binding.job_id=j.job_id
WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND selected.projections_removed_at IS NULL AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND j.status IN ('queued','running','recovery_required') AND binding.execution_fingerprint=OLD.execution_fingerprint)
AND EXISTS(SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.job_id=OLD.job_id AND c.phase='released'
AND c.source_kind=OLD.start_kind AND ((OLD.start_kind='image' AND c.source_image_id=OLD.start_id) OR (OLD.start_kind='state' AND c.source_state_id=OLD.start_id))
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(c.clone_ref,char(92),'/')))='/'||replace(c.clone_ref,char(92),'/')
AND substr(replace(c.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref))
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id)) OR (OLD.runner_phase='stopped' AND OLD.runner_id IS NOT NULL AND NEW.runner_id IS OLD.runner_id
AND OLD.retired_at IS NULL AND NEW.runner_phase='retired' AND NEW.retired_at IS NOT NULL
AND NEW.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL AND NEW.cursor_position=OLD.cursor_position
AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure' AND selected.projections_removed_at IS NULL)
AND EXISTS(SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.job_id=OLD.job_id AND c.phase='released'
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(c.clone_ref,char(92),'/')))='/'||replace(c.clone_ref,char(92),'/')
AND substr(replace(c.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref))
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id AND (a.phase<>'closed' OR a.resolution IS NULL
OR (a.resolution='discarded_clone' AND NOT (EXISTS(SELECT 1 FROM prepare_attempt_evidence p WHERE EXISTS (
SELECT 1 FROM prepare_liquibase_invocations i
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id AND s.job_id=i.job_id
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=s.job_id
JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN prepare_step_bindings sb ON sb.job_id=i.job_id AND sb.task_id=i.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
JOIN prepare_attempt_evidence request ON request.evidence_id=json_extract(p.facts_json,'$.observation.request_id')
JOIN prepare_attempt_evidence ack ON ack.evidence_id=json_extract(p.facts_json,'$.observation.ack_id')
JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=json_extract(p.facts_json,'$.observation.stop_id')
JOIN prepare_attempt_evidence q ON q.evidence_id=json_extract(p.facts_json,'$.observation.database_id')
WHERE i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no
AND a.executor_kind='session_exec' AND a.legacy_unbound=0
AND s.clone_id=a.clone_id AND s.worker_generation=a.worker_generation
AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND b.execution_digest=a.execution_digest AND b.prepare_kind='lb'
AND b.adapter_binding_digest=s.execution_fingerprint AND s.execution_fingerprint=lb.execution_fingerprint
AND b.policy_revision=lb.execution_policy_revision AND b.input_manifest_digest=lb.bundle_manifest_digest
AND b.target_identity_digest=lb.target_identity_digest AND b.executor_image_digest=lb.image_manifest_digest
AND p.job_id=a.job_id AND p.task_id=a.task_id AND p.attempt_no=a.attempt_no
AND p.worker_generation=a.worker_generation AND p.kind='selected_failure_disposal'
AND json_extract(p.facts_json,'$.clone_id')=a.clone_id
AND json_extract(p.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(p.facts_json,'$.observation.outcome_digest')=selected.outcome_digest
AND json_extract(p.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(p.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(p.facts_json,'$.observation.invocation_ref')=i.invocation_ref
AND json_extract(p.facts_json,'$.observation.clone_generation')=c.clone_generation
AND json_extract(p.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(p.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(p.facts_json,'$.observation.executor_absent')=1
AND json_extract(p.facts_json,'$.observation.runtime_absent')=1
AND json_extract(p.facts_json,'$.observation.original_phase') IN ('planned','exec_requested','exec_completed')
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged'
AND stopped.kind='executor_inspection' AND q.kind='db_quiescence'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id AND p.evidence_id>q.evidence_id
AND request.job_id=a.job_id AND request.task_id=a.task_id AND request.attempt_no=a.attempt_no AND request.worker_generation=a.worker_generation
AND ack.job_id=a.job_id AND ack.task_id=a.task_id AND ack.attempt_no=a.attempt_no AND ack.worker_generation=a.worker_generation
AND stopped.job_id=a.job_id AND stopped.task_id=a.task_id AND stopped.attempt_no=a.attempt_no AND stopped.worker_generation=a.worker_generation
AND q.job_id=a.job_id AND q.task_id=a.task_id AND q.attempt_no=a.attempt_no AND q.worker_generation=a.worker_generation
AND json_extract(request.facts_json,'$.clone_id')=a.clone_id AND json_extract(request.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(ack.facts_json,'$.clone_id')=a.clone_id AND json_extract(ack.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(stopped.facts_json,'$.clone_id')=a.clone_id AND json_extract(stopped.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(q.facts_json,'$.clone_id')=a.clone_id AND json_extract(q.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.runtime_absent')=1))
AND (SELECT count(*) FROM prepare_attempt_evidence single WHERE single.job_id=a.job_id AND single.task_id=a.task_id AND single.attempt_no=a.attempt_no AND single.kind='selected_failure_disposal')=1))))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id AND i.retired_at IS NULL)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id AND r.restore_phase NOT IN ('published','abandoned'))))
BEGIN SELECT RAISE(ABORT,'session retirement requires all physical resources and captures retired'); END;

CREATE TRIGGER prepare_liquibase_session_runner_attachment_guard
BEFORE UPDATE OF runner_id ON prepare_liquibase_sessions
WHEN OLD.runner_id IS NULL AND NEW.runner_id IS NOT NULL
  AND (OLD.runner_phase<>'create_requested' OR NEW.runner_phase<>'created')
BEGIN SELECT RAISE(ABORT,'runner attachment requires durable create intent'); END;

CREATE TRIGGER prepare_liquibase_session_runner_phase_guard BEFORE UPDATE OF runner_phase ON prepare_liquibase_sessions WHEN (NOT (OLD.runner_phase=NEW.runner_phase OR (OLD.runner_phase='planned' AND NEW.runner_phase='create_requested') OR (OLD.runner_phase='create_requested' AND NEW.runner_phase='created' AND NEW.runner_id IS NOT NULL) OR (OLD.runner_phase='created' AND NEW.runner_phase='start_requested' AND NEW.runner_id IS NOT NULL) OR (OLD.runner_phase='start_requested' AND NEW.runner_phase='running' AND NEW.runner_id IS NOT NULL) OR (OLD.runner_phase IN ('created','start_requested','running') AND NEW.runner_phase='stop_requested' AND NEW.runner_id IS NOT NULL) OR (OLD.runner_phase='stop_requested' AND NEW.runner_phase='stop_acknowledged') OR (OLD.runner_phase='stop_acknowledged' AND NEW.runner_phase='stopped') OR (OLD.runner_phase='stopped' AND NEW.runner_phase='retired' AND NEW.retired_at IS NOT NULL))) AND NOT ((OLD.runner_phase='planned' AND OLD.runner_id IS NULL AND OLD.retired_at IS NULL
AND OLD.db_phase IN ('running','stopped') AND OLD.cursor_position=OLD.start_position
AND NEW.runner_phase='retired' AND NEW.runner_id IS NULL AND NEW.retired_at IS NOT NULL
AND NEW.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL
AND NEW.cursor_position=OLD.cursor_position AND NEW.created_at IS OLD.created_at
AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings binding ON binding.job_id=j.job_id
WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND selected.projections_removed_at IS NULL AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND j.status IN ('queued','running','recovery_required') AND binding.execution_fingerprint=OLD.execution_fingerprint)
AND EXISTS(SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.job_id=OLD.job_id AND c.phase='released'
AND c.source_kind=OLD.start_kind AND ((OLD.start_kind='image' AND c.source_image_id=OLD.start_id) OR (OLD.start_kind='state' AND c.source_state_id=OLD.start_id))
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(c.clone_ref,char(92),'/')))='/'||replace(c.clone_ref,char(92),'/')
AND substr(replace(c.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref))
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id)) OR (OLD.runner_phase='stopped' AND OLD.runner_id IS NOT NULL AND NEW.runner_id IS OLD.runner_id
AND OLD.retired_at IS NULL AND NEW.runner_phase='retired' AND NEW.retired_at IS NOT NULL
AND NEW.db_phase='stopped' AND NEW.cursor_image_id IS NULL AND NEW.cursor_state_id IS NULL AND NEW.cursor_position=OLD.cursor_position
AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure' AND selected.projections_removed_at IS NULL)
AND EXISTS(SELECT 1 FROM prepare_clones c WHERE c.clone_id=OLD.clone_id AND c.job_id=OLD.job_id AND c.phase='released'
AND NOT EXISTS(SELECT 1 FROM managed_runtime_operations o WHERE o.stage<>'retired'
AND substr(replace(o.target_path,char(92),'/'),-length('/'||replace(c.clone_ref,char(92),'/')))='/'||replace(c.clone_ref,char(92),'/')
AND substr(replace(c.clone_ref,char(92),'/'),-length('/'||o.operation_ref))='/'||o.operation_ref))
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=OLD.clone_id AND (a.phase<>'closed' OR a.resolution IS NULL
OR (a.resolution='discarded_clone' AND NOT (EXISTS(SELECT 1 FROM prepare_attempt_evidence p WHERE EXISTS (
SELECT 1 FROM prepare_liquibase_invocations i
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id AND s.job_id=i.job_id
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=s.job_id
JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN prepare_step_bindings sb ON sb.job_id=i.job_id AND sb.task_id=i.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
JOIN prepare_attempt_evidence request ON request.evidence_id=json_extract(p.facts_json,'$.observation.request_id')
JOIN prepare_attempt_evidence ack ON ack.evidence_id=json_extract(p.facts_json,'$.observation.ack_id')
JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=json_extract(p.facts_json,'$.observation.stop_id')
JOIN prepare_attempt_evidence q ON q.evidence_id=json_extract(p.facts_json,'$.observation.database_id')
WHERE i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no
AND a.executor_kind='session_exec' AND a.legacy_unbound=0
AND s.clone_id=a.clone_id AND s.worker_generation=a.worker_generation
AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND b.execution_digest=a.execution_digest AND b.prepare_kind='lb'
AND b.adapter_binding_digest=s.execution_fingerprint AND s.execution_fingerprint=lb.execution_fingerprint
AND b.policy_revision=lb.execution_policy_revision AND b.input_manifest_digest=lb.bundle_manifest_digest
AND b.target_identity_digest=lb.target_identity_digest AND b.executor_image_digest=lb.image_manifest_digest
AND p.job_id=a.job_id AND p.task_id=a.task_id AND p.attempt_no=a.attempt_no
AND p.worker_generation=a.worker_generation AND p.kind='selected_failure_disposal'
AND json_extract(p.facts_json,'$.clone_id')=a.clone_id
AND json_extract(p.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(p.facts_json,'$.observation.outcome_digest')=selected.outcome_digest
AND json_extract(p.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(p.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(p.facts_json,'$.observation.invocation_ref')=i.invocation_ref
AND json_extract(p.facts_json,'$.observation.clone_generation')=c.clone_generation
AND json_extract(p.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(p.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(p.facts_json,'$.observation.executor_absent')=1
AND json_extract(p.facts_json,'$.observation.runtime_absent')=1
AND json_extract(p.facts_json,'$.observation.original_phase') IN ('planned','exec_requested','exec_completed')
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged'
AND stopped.kind='executor_inspection' AND q.kind='db_quiescence'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id AND p.evidence_id>q.evidence_id
AND request.job_id=a.job_id AND request.task_id=a.task_id AND request.attempt_no=a.attempt_no AND request.worker_generation=a.worker_generation
AND ack.job_id=a.job_id AND ack.task_id=a.task_id AND ack.attempt_no=a.attempt_no AND ack.worker_generation=a.worker_generation
AND stopped.job_id=a.job_id AND stopped.task_id=a.task_id AND stopped.attempt_no=a.attempt_no AND stopped.worker_generation=a.worker_generation
AND q.job_id=a.job_id AND q.task_id=a.task_id AND q.attempt_no=a.attempt_no AND q.worker_generation=a.worker_generation
AND json_extract(request.facts_json,'$.clone_id')=a.clone_id AND json_extract(request.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(ack.facts_json,'$.clone_id')=a.clone_id AND json_extract(ack.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(stopped.facts_json,'$.clone_id')=a.clone_id AND json_extract(stopped.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(q.facts_json,'$.clone_id')=a.clone_id AND json_extract(q.facts_json,'$.execution_digest')=a.execution_digest
AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.runtime_absent')=1))
AND (SELECT count(*) FROM prepare_attempt_evidence single WHERE single.job_id=a.job_id AND single.task_id=a.task_id AND single.attempt_no=a.attempt_no AND single.kind='selected_failure_disposal')=1))))
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=OLD.session_id AND i.retired_at IS NULL)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=OLD.session_id AND r.restore_phase NOT IN ('published','abandoned'))))
BEGIN SELECT RAISE(ABORT,'invalid Liquibase runner phase transition'); END;

CREATE TRIGGER prepare_planning_artifact_delete_guard
BEFORE DELETE ON prepare_planning_attempt_artifacts
WHEN EXISTS (SELECT 1 FROM prepare_jobs WHERE job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'planning capture is retained with its job'); END;

CREATE TRIGGER prepare_planning_artifact_identity_immutable
BEFORE UPDATE OF job_id,task_id,attempt_no,protected_input_ref,complete_output_ref,
  complete_stderr_ref,capture_manifest_ref,capture_helper_digest,capture_protocol_revision
ON prepare_planning_attempt_artifacts
BEGIN SELECT RAISE(ABORT,'planning artifact identity is immutable'); END;

CREATE TRIGGER prepare_planning_artifact_insert_guard
BEFORE INSERT ON prepare_planning_attempt_artifacts
WHEN NOT EXISTS (
  SELECT 1 FROM prepare_planning_intents i
  JOIN prepare_tasks t ON t.job_id=i.job_id AND t.task_id=i.task_id
  JOIN prepare_step_attempts a ON a.job_id=i.job_id AND a.task_id=i.task_id
  JOIN prepare_artifact_leases l ON l.owner_kind='job' AND l.owner_id=i.job_id
  WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id
    AND t.type='plan' AND a.attempt_no=NEW.attempt_no
    AND a.execution_digest=i.execution_digest
    AND l.artifact_kind='capture_helper' AND l.artifact_digest=NEW.capture_helper_digest
    AND l.expires_at IS NULL)
BEGIN SELECT RAISE(ABORT,'planning artifact lacks bound attempt or capture helper lease'); END;

CREATE TRIGGER prepare_planning_artifact_output_immutable
BEFORE UPDATE OF output_digest,output_bytes,output_complete_at
ON prepare_planning_attempt_artifacts
WHEN OLD.output_complete_at IS NOT NULL AND (
  NEW.output_digest IS NOT OLD.output_digest OR NEW.output_bytes IS NOT OLD.output_bytes
  OR NEW.output_complete_at IS NOT OLD.output_complete_at)
BEGIN SELECT RAISE(ABORT,'complete planning output is immutable'); END;

CREATE TRIGGER prepare_planning_artifact_retirement_immutable
BEFORE UPDATE OF executor_retired_at,protected_input_retired_at,output_retired_at,
  stderr_retired_at,manifest_retired_at
ON prepare_planning_attempt_artifacts
WHEN (OLD.executor_retired_at IS NOT NULL AND NEW.executor_retired_at IS NOT OLD.executor_retired_at)
 OR (OLD.protected_input_retired_at IS NOT NULL AND NEW.protected_input_retired_at IS NOT OLD.protected_input_retired_at)
 OR (OLD.output_retired_at IS NOT NULL AND NEW.output_retired_at IS NOT OLD.output_retired_at)
 OR (OLD.stderr_retired_at IS NOT NULL AND NEW.stderr_retired_at IS NOT OLD.stderr_retired_at)
 OR (OLD.manifest_retired_at IS NOT NULL AND NEW.manifest_retired_at IS NOT OLD.manifest_retired_at)
BEGIN SELECT RAISE(ABORT,'planning artifact retirement is monotonic'); END;

CREATE TRIGGER prepare_planning_changeset_delete_guard BEFORE DELETE ON prepare_planning_changesets
WHEN EXISTS (SELECT 1 FROM prepare_planning_intents i
             WHERE i.job_id=OLD.job_id AND i.plan_saved_at IS NOT NULL)
BEGIN SELECT RAISE(ABORT,'saved planning changeset is retained'); END;

CREATE TRIGGER prepare_planning_changeset_insert_guard BEFORE INSERT ON prepare_planning_changesets
WHEN NOT EXISTS (SELECT 1 FROM prepare_planning_intents i
                 WHERE i.job_id=NEW.job_id AND i.plan_saved_at IS NULL)
BEGIN SELECT RAISE(ABORT,'planning changeset cannot follow saved plan'); END;

CREATE TRIGGER prepare_planning_changeset_update_guard BEFORE UPDATE ON prepare_planning_changesets
WHEN EXISTS (SELECT 1 FROM prepare_planning_intents i
             WHERE i.job_id=OLD.job_id AND i.plan_saved_at IS NOT NULL)
BEGIN SELECT RAISE(ABORT,'saved planning changeset is immutable'); END;

CREATE TRIGGER prepare_planning_decision_guard
BEFORE INSERT ON prepare_attempt_decisions
WHEN NEW.choice='complete_planning' AND (
  NEW.target_clone_id IS NOT NULL OR NEW.next_attempt_no IS NOT NULL
  OR NOT EXISTS (
    SELECT 1 FROM prepare_planning_intents i
    JOIN prepare_step_attempts a ON a.job_id=i.job_id AND a.task_id=i.task_id
      AND a.attempt_no=i.result_attempt_no
    WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id
      AND i.result_attempt_no=NEW.attempt_no AND i.plan_saved_at IS NOT NULL
      AND i.execution_digest=a.execution_digest
      AND a.worker_generation=NEW.worker_generation))
BEGIN SELECT RAISE(ABORT,'planning completion decision lacks saved exact plan'); END;

CREATE TRIGGER prepare_planning_decision_immutable BEFORE UPDATE OF
  cache_decision_json,cache_decision_digest,decision_saved_at
ON prepare_planning_intents
WHEN OLD.decision_saved_at IS NOT NULL AND (
  NEW.cache_decision_json IS NOT OLD.cache_decision_json
  OR NEW.cache_decision_digest IS NOT OLD.cache_decision_digest
  OR NEW.decision_saved_at IS NOT OLD.decision_saved_at)
BEGIN SELECT RAISE(ABORT,'saved cache decision is immutable'); END;

CREATE TRIGGER prepare_planning_intent_delete_guard BEFORE DELETE ON prepare_planning_intents
WHEN EXISTS (SELECT 1 FROM prepare_jobs WHERE job_id=OLD.job_id)
BEGIN SELECT RAISE(ABORT,'planning intent is retained with its job'); END;

CREATE TRIGGER prepare_planning_intent_immutable BEFORE UPDATE OF
  job_id,task_id,execution_digest,preview_clone_id,plan_codec_revision,created_at
ON prepare_planning_intents
BEGIN SELECT RAISE(ABORT,'planning intent identity is immutable'); END;

CREATE TRIGGER prepare_planning_intent_insert_guard BEFORE INSERT ON prepare_planning_intents
WHEN NOT EXISTS (
  SELECT 1 FROM prepare_jobs j
  JOIN prepare_tasks t ON t.job_id=j.job_id AND t.task_id=NEW.task_id
  JOIN prepare_step_bindings b ON b.job_id=t.job_id AND b.task_id=t.task_id
  JOIN prepare_clones c ON c.clone_id=NEW.preview_clone_id AND c.job_id=j.job_id
  WHERE j.job_id=NEW.job_id AND j.prepare_kind='lb'
    AND j.purpose IN ('prepare','cache_explain')
    AND (j.purpose<>'cache_explain' OR j.plan_only=1)
    AND j.status IN ('queued','running')
    AND t.type='plan' AND t.task_id='plan'
    AND b.execution_digest=NEW.execution_digest
)
BEGIN SELECT RAISE(ABORT,'planning intent lacks exact job, task, clone, or binding'); END;

CREATE TRIGGER prepare_planning_job_delete_guard
BEFORE DELETE ON prepare_jobs
WHEN EXISTS (SELECT 1 FROM prepare_planning_intents i WHERE i.job_id=OLD.job_id)
  AND (OLD.status NOT IN ('succeeded','failed')
    OR EXISTS (SELECT 1 FROM prepare_planning_attempt_artifacts capture
               WHERE capture.job_id=OLD.job_id AND (
                 capture.executor_retired_at IS NULL
                 OR capture.protected_input_retired_at IS NULL
                 OR capture.output_retired_at IS NULL
                 OR capture.stderr_retired_at IS NULL
                 OR capture.manifest_retired_at IS NULL)))
BEGIN SELECT RAISE(ABORT,'planning job retains protected resources'); END;

CREATE TRIGGER prepare_planning_job_terminal_guard
BEFORE UPDATE OF status ON prepare_jobs
WHEN NEW.status IN ('succeeded','failed')
  AND EXISTS (SELECT 1 FROM prepare_planning_intents i WHERE i.job_id=OLD.job_id)
  AND (
    EXISTS (SELECT 1 FROM prepare_step_attempts a
            WHERE a.job_id=OLD.job_id AND a.resolution IS NULL)
    OR EXISTS (SELECT 1 FROM prepare_clones c
               WHERE c.job_id=OLD.job_id AND c.phase<>'released')
    OR EXISTS (SELECT 1 FROM prepare_planning_attempt_artifacts capture
               WHERE capture.job_id=OLD.job_id AND (
                 capture.executor_retired_at IS NULL
                 OR capture.protected_input_retired_at IS NULL
                 OR capture.output_retired_at IS NULL
                 OR capture.stderr_retired_at IS NULL
                 OR capture.manifest_retired_at IS NULL))
    OR (NEW.status='succeeded' AND NOT EXISTS (
      SELECT 1 FROM prepare_planning_intents i
      JOIN prepare_tasks t ON t.job_id=i.job_id AND t.task_id=i.task_id
      JOIN prepare_step_attempts a ON a.job_id=i.job_id AND a.task_id=i.task_id
        AND a.attempt_no=i.result_attempt_no
      WHERE i.job_id=OLD.job_id AND i.plan_saved_at IS NOT NULL
        AND t.status='succeeded' AND a.resolution='planning_result_saved'
        AND a.execution_digest=i.execution_digest))
    OR (NEW.status='succeeded' AND NEW.purpose='cache_explain' AND (
      EXISTS (SELECT 1 FROM prepare_tasks t
              WHERE t.job_id=OLD.job_id AND t.type='state_execute')
      OR NOT EXISTS (SELECT 1 FROM prepare_planning_intents i
                     WHERE i.job_id=OLD.job_id AND i.decision_saved_at IS NOT NULL
                       AND i.cache_decision_json=NEW.cache_explain_result_json)))
    OR (NEW.status='succeeded' AND NEW.purpose='prepare' AND NEW.plan_only=1
      AND NOT EXISTS (SELECT 1 FROM prepare_planning_intents i
                      WHERE i.job_id=OLD.job_id AND i.plan_projection_saved_at IS NOT NULL
                        AND i.plan_projection_json IS NOT NULL
                        AND i.plan_projection_digest IS NOT NULL))
    OR (NEW.status='succeeded' AND NEW.purpose='prepare' AND NEW.plan_only=1
      AND EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=OLD.job_id AND t.task_id<>'plan')))
BEGIN SELECT RAISE(ABORT,'planning job lacks result or safe cleanup'); END;

CREATE TRIGGER prepare_planning_plan_immutable BEFORE UPDATE OF
  result_attempt_no,plan_digest,plan_count,parsed_output_digest,plan_saved_at
ON prepare_planning_intents
WHEN OLD.plan_saved_at IS NOT NULL AND (
  NEW.result_attempt_no IS NOT OLD.result_attempt_no
  OR NEW.plan_digest IS NOT OLD.plan_digest OR NEW.plan_count IS NOT OLD.plan_count
  OR NEW.parsed_output_digest IS NOT OLD.parsed_output_digest
  OR NEW.plan_saved_at IS NOT OLD.plan_saved_at)
BEGIN SELECT RAISE(ABORT,'saved planning result is immutable'); END;

CREATE TRIGGER prepare_planning_plan_save_guard
BEFORE UPDATE OF result_attempt_no,plan_digest,plan_count,parsed_output_digest,plan_saved_at
ON prepare_planning_intents
WHEN OLD.plan_saved_at IS NULL AND NEW.plan_saved_at IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM prepare_step_attempts a
  JOIN prepare_planning_attempt_artifacts capture
    ON capture.job_id=a.job_id AND capture.task_id=a.task_id
    AND capture.attempt_no=a.attempt_no
  WHERE a.job_id=NEW.job_id AND a.task_id=NEW.task_id
    AND a.attempt_no=NEW.result_attempt_no
    AND a.execution_digest=NEW.execution_digest
    AND a.phase IN ('observed_stopped','closed') AND a.executor_id IS NOT NULL
    AND capture.output_complete_at IS NOT NULL
    AND capture.output_digest=NEW.parsed_output_digest
    AND EXISTS (SELECT 1 FROM prepare_attempt_evidence e
      WHERE e.job_id=a.job_id AND e.task_id=a.task_id AND e.attempt_no=a.attempt_no
        AND e.kind='executor_inspection'
        AND json_extract(e.facts_json,'$.observation.executor_id')=a.executor_id
        AND json_extract(e.facts_json,'$.observation.exit_code')=0)
    AND NEW.plan_count=(SELECT count(*) FROM prepare_planning_changesets cs
                        WHERE cs.job_id=NEW.job_id)
    AND coalesce((SELECT max(cs.ordinal) FROM prepare_planning_changesets cs
                  WHERE cs.job_id=NEW.job_id),-1)=NEW.plan_count-1
)
BEGIN SELECT RAISE(ABORT,'planning result lacks complete exact output and successful exit'); END;

CREATE TRIGGER prepare_planning_projection_guard BEFORE UPDATE OF
  plan_projection_json,plan_projection_digest,plan_projection_saved_at
ON prepare_planning_intents
WHEN NEW.plan_projection_saved_at IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM prepare_jobs j JOIN prepare_tasks t
    ON t.job_id=j.job_id AND t.task_id='plan'
  WHERE j.job_id=NEW.job_id AND j.purpose='prepare' AND j.prepare_kind='lb'
    AND j.plan_only=1 AND j.status='running' AND t.status='succeeded'
    AND NOT EXISTS (SELECT 1 FROM prepare_tasks other
                    WHERE other.job_id=j.job_id AND other.task_id<>'plan'))
BEGIN SELECT RAISE(ABORT,'public plan requires a completed plan-only task'); END;

CREATE TRIGGER prepare_planning_projection_immutable BEFORE UPDATE OF
  plan_projection_json,plan_projection_digest,plan_projection_saved_at
ON prepare_planning_intents
WHEN OLD.plan_projection_saved_at IS NOT NULL AND (
  NEW.plan_projection_json IS NOT OLD.plan_projection_json
  OR NEW.plan_projection_digest IS NOT OLD.plan_projection_digest
  OR NEW.plan_projection_saved_at IS NOT OLD.plan_projection_saved_at)
BEGIN SELECT RAISE(ABORT,'saved public plan projection is immutable'); END;

CREATE TRIGGER prepare_planning_resolution_guard
BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN NEW.resolution='planning_result_saved' AND (
  OLD.resolution IS NOT NULL OR OLD.phase<>'observed_stopped' OR NEW.phase<>'closed'
  OR NEW.executor_id IS NULL
  OR NOT EXISTS (
    SELECT 1 FROM prepare_planning_intents i
    JOIN prepare_planning_attempt_artifacts capture
      ON capture.job_id=NEW.job_id AND capture.task_id=NEW.task_id
      AND capture.attempt_no=NEW.attempt_no
    JOIN prepare_clones clone ON clone.clone_id=NEW.clone_id AND clone.job_id=NEW.job_id
    JOIN prepare_attempt_decisions d
      ON d.job_id=NEW.job_id AND d.task_id=NEW.task_id
      AND d.attempt_no=NEW.attempt_no AND d.worker_generation=NEW.worker_generation
      AND d.choice='complete_planning' AND d.target_clone_id IS NULL
      AND d.next_attempt_no IS NULL
    JOIN prepare_decision_evidence exit_link ON exit_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence exit_e ON exit_e.evidence_id=exit_link.evidence_id
    JOIN prepare_decision_evidence quiescence_link ON quiescence_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence q ON q.evidence_id=quiescence_link.evidence_id
    JOIN prepare_decision_evidence output_link ON output_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence output_e ON output_e.evidence_id=output_link.evidence_id
    WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id
      AND i.result_attempt_no=NEW.attempt_no AND i.plan_saved_at IS NOT NULL
      AND i.execution_digest=NEW.execution_digest
      AND i.parsed_output_digest=capture.output_digest
      AND capture.output_complete_at IS NOT NULL
      AND exit_e.job_id=NEW.job_id AND exit_e.task_id=NEW.task_id
      AND exit_e.attempt_no=NEW.attempt_no
      AND exit_e.worker_generation=NEW.worker_generation
      AND exit_e.kind='executor_inspection'
      AND json_extract(exit_e.facts_json,'$.observation.executor_id')=NEW.executor_id
      AND json_extract(exit_e.facts_json,'$.observation.exit_code')=0
      AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id
      AND q.attempt_no=NEW.attempt_no AND q.worker_generation=NEW.worker_generation
      AND q.kind='db_quiescence' AND q.evidence_id>exit_e.evidence_id
      AND json_extract(q.facts_json,'$.observation.runtime_absent')=1
      AND json_extract(q.facts_json,'$.observation.runtime_name')=clone.planned_db_runtime_name
      AND json_extract(q.facts_json,'$.observation.runtime_id')=clone.db_runtime_id
      AND output_e.job_id=NEW.job_id AND output_e.task_id=NEW.task_id
      AND output_e.attempt_no=NEW.attempt_no
      AND output_e.worker_generation=NEW.worker_generation
      AND output_e.kind='planning_output' AND output_e.evidence_id>q.evidence_id
      AND json_extract(output_e.facts_json,'$.observation.output_digest')=capture.output_digest
      AND json_extract(output_e.facts_json,'$.observation.output_bytes')=capture.output_bytes
      AND json_extract(output_e.facts_json,'$.observation.manifest_ref')=capture.capture_manifest_ref))
BEGIN SELECT RAISE(ABORT,'planning resolution lacks ordered exact evidence'); END;

CREATE TRIGGER prepare_planning_task_delete_guard
BEFORE DELETE ON prepare_tasks
WHEN OLD.type='plan' AND EXISTS (
  SELECT 1 FROM prepare_planning_intents i
  JOIN prepare_jobs j ON j.job_id=i.job_id
  WHERE i.job_id=OLD.job_id AND i.task_id=OLD.task_id)
BEGIN SELECT RAISE(ABORT,'durable plan task is retained with its job'); END;

CREATE TRIGGER prepare_planning_task_success_guard
BEFORE UPDATE OF status ON prepare_tasks
WHEN NEW.type='plan' AND NEW.status='succeeded' AND OLD.status<>'succeeded'
  AND EXISTS (SELECT 1 FROM prepare_planning_intents i
              WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id)
  AND NOT EXISTS (
    SELECT 1 FROM prepare_planning_intents i
    JOIN prepare_step_attempts a ON a.job_id=i.job_id AND a.task_id=i.task_id
      AND a.attempt_no=i.result_attempt_no
    WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id
      AND i.plan_saved_at IS NOT NULL AND a.resolution='planning_result_saved'
      AND a.execution_digest=i.execution_digest)
BEGIN SELECT RAISE(ABORT,'plan task lacks resolved saved result'); END;

CREATE TRIGGER prepare_recovery_job_delete_guard
BEFORE DELETE ON prepare_jobs
WHEN OLD.status='recovery_required'
BEGIN SELECT RAISE(ABORT,'recovery-required job cannot be deleted'); END;

CREATE TRIGGER prepare_recovery_job_terminal_guard
BEFORE UPDATE OF status ON prepare_jobs
WHEN OLD.status='recovery_required' AND NEW.status IN ('succeeded','failed')
BEGIN SELECT RAISE(ABORT,'recovery-required job cannot become terminal'); END;

CREATE TRIGGER prepare_recovery_task_delete_guard
BEFORE DELETE ON prepare_tasks
WHEN OLD.status='recovery_required' OR EXISTS (
  SELECT 1 FROM prepare_jobs j WHERE j.job_id=OLD.job_id AND j.status='recovery_required')
BEGIN SELECT RAISE(ABORT,'recovery-required task cannot be deleted'); END;

CREATE TRIGGER prepare_recovery_task_terminal_guard
BEFORE UPDATE OF status ON prepare_tasks
WHEN OLD.status='recovery_required' AND NEW.status IN ('succeeded','failed')
AND NOT (NEW.status='failed' AND EXISTS (
  SELECT 1 FROM prepare_step_attempts a
  JOIN prepare_liquibase_invocations i ON i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no
  JOIN prepare_liquibase_sessions session ON session.session_id=i.session_id AND session.clone_id=a.clone_id
  WHERE a.job_id=OLD.job_id AND a.task_id=OLD.task_id AND a.executor_kind='session_exec'
    AND a.phase='closed' AND a.resolution='discarded_clone'
    AND (i.complete_at IS NOT NULL AND i.exit_code<>0 OR EXISTS (
SELECT 1 FROM prepare_attempt_evidence cancel WHERE cancel.job_id=a.job_id AND cancel.task_id=a.task_id
AND cancel.attempt_no=a.attempt_no AND cancel.worker_generation=a.worker_generation AND cancel.kind='cancellation_requested'
AND json_extract(cancel.facts_json,'$.observation.session_id')=session.session_id
AND json_extract(cancel.facts_json,'$.observation.runner_id')=session.runner_id)) AND session.runner_phase='stopped'
    AND EXISTS (SELECT 1 FROM prepare_attempt_evidence e WHERE e.job_id=a.job_id AND e.task_id=a.task_id
      AND e.attempt_no=a.attempt_no AND e.worker_generation=a.worker_generation AND e.kind='db_quiescence'
      AND json_extract(e.facts_json,'$.observation.session_id')=session.session_id
      AND json_extract(e.facts_json,'$.observation.runtime_absent')=1)))
AND (NEW.status<>'succeeded' OR NOT EXISTS (
  SELECT 1 FROM prepare_step_attempts a
  JOIN prepare_state_bindings sb ON sb.state_id=OLD.output_state_id
  WHERE a.job_id=OLD.job_id AND a.task_id=OLD.task_id
    AND a.phase='closed' AND a.resolution='published'
    AND sb.execution_digest=a.execution_digest))
AND NOT (NEW.status='succeeded' AND EXISTS (
SELECT 1 FROM prepare_liquibase_cache_restores r JOIN prepare_state_bindings state ON state.state_id=r.state_id
JOIN prepare_step_bindings step ON step.job_id=r.job_id AND step.task_id=r.task_id AND step.execution_digest=state.execution_digest
WHERE r.job_id=OLD.job_id AND r.task_id=OLD.task_id AND r.state_id=OLD.output_state_id AND r.restore_phase IN ('published','abandoned')))
BEGIN SELECT RAISE(ABORT,'recovery-required task cannot become terminal'); END;

CREATE TRIGGER prepare_session_attempt_discarded_guard BEFORE UPDATE OF resolution ON prepare_step_attempts WHEN ((NEW.executor_kind='session_exec' AND NEW.resolution='discarded_clone' AND ( OLD.resolution IS NOT NULL OR OLD.phase<>'exec_completed' OR NEW.phase<>'closed' OR NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id JOIN prepare_clones c ON c.clone_id=s.clone_id JOIN prepare_attempt_evidence complete ON complete.job_id=i.job_id AND complete.task_id=i.task_id AND complete.attempt_no=i.attempt_no JOIN prepare_attempt_evidence q ON q.job_id=i.job_id AND q.task_id=i.task_id AND q.attempt_no=i.attempt_no WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id AND i.attempt_no=NEW.attempt_no AND i.complete_at IS NOT NULL AND i.exit_code<>0 AND s.runner_phase='stopped' AND s.clone_id=NEW.clone_id AND s.worker_generation=NEW.worker_generation AND complete.kind='exec_completion' AND complete.worker_generation=NEW.worker_generation AND json_extract(complete.facts_json,'$.observation.invocation_ref')=i.invocation_ref AND json_extract(complete.facts_json,'$.observation.manifest_digest')=i.completion_digest AND json_extract(complete.facts_json,'$.observation.exit_code')=i.exit_code AND q.kind='db_quiescence' AND q.worker_generation=NEW.worker_generation AND q.evidence_id>complete.evidence_id AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id AND json_extract(q.facts_json,'$.observation.runtime_absent')=1)) AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id JOIN prepare_clones c ON c.clone_id=s.clone_id JOIN prepare_tasks t ON t.job_id=i.job_id AND t.task_id=i.task_id JOIN prepare_execution_bindings b ON b.execution_digest=NEW.execution_digest JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=s.execution_fingerprint JOIN prepare_attempt_decisions d ON d.job_id=i.job_id AND d.task_id=i.task_id AND d.attempt_no=i.attempt_no JOIN prepare_decision_evidence request_link ON request_link.decision_id=d.decision_id JOIN prepare_attempt_evidence request ON request.evidence_id=request_link.evidence_id JOIN prepare_decision_evidence ack_link ON ack_link.decision_id=d.decision_id JOIN prepare_attempt_evidence ack ON ack.evidence_id=ack_link.evidence_id JOIN prepare_decision_evidence stop_link ON stop_link.decision_id=d.decision_id JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=stop_link.evidence_id JOIN prepare_decision_evidence q_link ON q_link.decision_id=d.decision_id JOIN prepare_attempt_evidence q ON q.evidence_id=q_link.evidence_id WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id AND i.attempt_no=NEW.attempt_no AND i.complete_at IS NULL AND s.clone_id=NEW.clone_id AND s.worker_generation=NEW.worker_generation AND s.runner_phase='stopped' AND c.phase='quiescent' AND NEW.phase='closed' AND ((NEW.resolution='never_started' AND i.start_requested_at IS NULL) OR (NEW.resolution='discarded_clone' AND i.start_requested_at IS NOT NULL AND EXISTS ( SELECT 1 FROM prepare_events warning WHERE warning.job_id=NEW.job_id AND warning.task_id=NEW.task_id AND warning.message LIKE 'WARNING [liquibase_external_effect_replay]%'))) AND b.prepare_kind='lb' AND b.policy_revision='lb-container-v2' AND b.effect_scope='unknown' AND b.enforcement_digest IS NULL AND b.adapter_binding_digest=lb.execution_fingerprint AND b.input_manifest_digest=lb.bundle_manifest_digest AND lb.execution_policy_revision=b.policy_revision AND d.choice='replay_fresh' AND d.target_clone_id IS NOT NULL AND d.next_attempt_no=NEW.attempt_no+1 AND d.worker_generation=NEW.worker_generation AND t.position=s.cursor_position AND ((t.input_kind='state' AND t.input_id=s.cursor_state_id) OR (t.input_kind='image' AND t.input_id=s.cursor_image_id)) AND request.kind='termination_requested' AND ack.kind='termination_acknowledged' AND stopped.kind='executor_inspection' AND q.kind='db_quiescence' AND request.job_id=NEW.job_id AND request.task_id=NEW.task_id AND request.attempt_no=NEW.attempt_no AND request.worker_generation=NEW.worker_generation AND ack.job_id=NEW.job_id AND ack.task_id=NEW.task_id AND ack.attempt_no=NEW.attempt_no AND ack.worker_generation=NEW.worker_generation AND stopped.job_id=NEW.job_id AND stopped.task_id=NEW.task_id AND stopped.attempt_no=NEW.attempt_no AND stopped.worker_generation=NEW.worker_generation AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id AND q.attempt_no=NEW.attempt_no AND q.worker_generation=NEW.worker_generation AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1 AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id AND json_extract(q.facts_json,'$.observation.runtime_absent')=1 AND NOT EXISTS (SELECT 1 FROM prepare_attempt_evidence cancel WHERE cancel.job_id=NEW.job_id AND cancel.task_id=NEW.task_id AND cancel.attempt_no=NEW.attempt_no AND cancel.kind='cancellation_requested' AND cancel.worker_generation=NEW.worker_generation) AND NOT EXISTS (SELECT 1 FROM prepare_attempt_decisions earlier JOIN prepare_liquibase_invocations old ON old.job_id=earlier.job_id AND old.task_id=earlier.task_id AND old.attempt_no=earlier.attempt_no WHERE earlier.job_id=NEW.job_id AND earlier.task_id=NEW.task_id AND earlier.choice='replay_fresh' AND earlier.attempt_no<>NEW.attempt_no AND old.start_requested_at IS NOT NULL)) AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id JOIN prepare_clones c ON c.clone_id=s.clone_id JOIN prepare_attempt_evidence cancel ON cancel.job_id=i.job_id AND cancel.task_id=i.task_id AND cancel.attempt_no=i.attempt_no JOIN prepare_attempt_evidence request ON request.job_id=i.job_id AND request.task_id=i.task_id AND request.attempt_no=i.attempt_no JOIN prepare_attempt_evidence ack ON ack.job_id=i.job_id AND ack.task_id=i.task_id AND ack.attempt_no=i.attempt_no JOIN prepare_attempt_evidence stopped ON stopped.job_id=i.job_id AND stopped.task_id=i.task_id AND stopped.attempt_no=i.attempt_no JOIN prepare_attempt_evidence q ON q.job_id=i.job_id AND q.task_id=i.task_id AND q.attempt_no=i.attempt_no WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id AND i.attempt_no=NEW.attempt_no AND OLD.resolution IS NULL AND OLD.phase IN ('planned','exec_requested','exec_completed') AND NEW.phase='closed' AND s.clone_id=NEW.clone_id AND s.worker_generation=NEW.worker_generation AND s.runner_phase='stopped' AND c.phase='quiescent' AND cancel.kind='cancellation_requested' AND request.kind='termination_requested' AND ack.kind='termination_acknowledged' AND stopped.kind='executor_inspection' AND q.kind='db_quiescence' AND cancel.worker_generation=NEW.worker_generation AND request.worker_generation=NEW.worker_generation AND ack.worker_generation=NEW.worker_generation AND stopped.worker_generation=NEW.worker_generation AND q.worker_generation=NEW.worker_generation AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id AND cancel.evidence_id<q.evidence_id AND json_extract(cancel.facts_json,'$.observation.session_id')=s.session_id AND json_extract(cancel.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1 AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id AND json_extract(q.facts_json,'$.observation.runtime_absent')=1) AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id JOIN prepare_clones c ON c.clone_id=s.clone_id JOIN prepare_tasks t ON t.job_id=i.job_id AND t.task_id=i.task_id JOIN prepare_jobs j ON j.job_id=t.job_id JOIN prepare_attempt_evidence selected ON selected.job_id=i.job_id AND selected.task_id=i.task_id AND selected.attempt_no=i.attempt_no JOIN states state ON state.state_id=json_extract(selected.facts_json,'$.observation.cached_state_id') JOIN prepare_state_bindings b ON b.state_id=state.state_id AND b.execution_digest=NEW.execution_digest JOIN prepare_step_bindings step ON step.job_id=t.job_id AND step.task_id=t.task_id AND step.execution_digest=b.execution_digest JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id AND lb.execution_fingerprint=s.execution_fingerprint JOIN prepare_attempt_evidence request ON request.job_id=i.job_id AND request.task_id=i.task_id AND request.attempt_no=i.attempt_no JOIN prepare_attempt_evidence ack ON ack.job_id=i.job_id AND ack.task_id=i.task_id AND ack.attempt_no=i.attempt_no JOIN prepare_attempt_evidence stopped ON stopped.job_id=i.job_id AND stopped.task_id=i.task_id AND stopped.attempt_no=i.attempt_no JOIN prepare_attempt_evidence q ON q.evidence_id=json_extract(selected.facts_json,'$.observation.quiescence_id') WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id AND i.attempt_no=NEW.attempt_no AND OLD.resolution IS NULL AND OLD.phase IN ('planned','exec_requested','exec_completed') AND NEW.phase='closed' AND (i.complete_at IS NULL OR i.exit_code=0) AND s.clone_id=NEW.clone_id AND s.worker_generation=NEW.worker_generation AND s.runner_phase='stopped' AND s.retired_at IS NULL AND c.phase='quiescent' AND t.position=s.cursor_position AND t.status IN ('running','recovery_required') AND t.output_state_id=state.state_id AND state.state_fingerprint=state.state_id AND state.prepare_kind='lb' AND state.image_id=coalesce(nullif(j.resolved_image_id,''),j.image_id) AND coalesce(state.lineage_ref,'')=coalesce(j.lineage_ref,'') AND coalesce(state.identity_digest,'')=coalesce(j.identity_digest,'') AND ((t.input_kind='state' AND t.input_id=s.cursor_state_id AND state.parent_state_id=s.cursor_state_id) OR (t.input_kind='image' AND t.input_id=s.cursor_image_id AND state.parent_state_id IS NULL)) AND selected.kind='cache_superseded' AND request.kind='termination_requested' AND ack.kind='termination_acknowledged' AND stopped.kind='executor_inspection' AND q.kind='db_quiescence' AND selected.worker_generation=NEW.worker_generation AND request.worker_generation=NEW.worker_generation AND ack.worker_generation=NEW.worker_generation AND stopped.worker_generation=NEW.worker_generation AND q.worker_generation=NEW.worker_generation AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id AND q.attempt_no=NEW.attempt_no AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id AND q.evidence_id<selected.evidence_id AND json_extract(selected.facts_json,'$.observation.session_id')=s.session_id AND json_extract(selected.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1 AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id AND json_extract(q.facts_json,'$.observation.runtime_absent')=1 AND NOT EXISTS (SELECT 1 FROM prepare_attempt_evidence cancel WHERE cancel.job_id=NEW.job_id AND cancel.task_id=NEW.task_id AND cancel.attempt_no=NEW.attempt_no AND cancel.kind='cancellation_requested'))) OR (NEW.executor_kind='session_exec' AND NEW.resolution='discarded_clone' AND EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=OLD.job_id AND selected.outcome_kind='failure'))) AND NOT (OLD.resolution IS NULL AND OLD.phase IN ('planned','exec_requested','exec_completed') AND NEW.phase='closed'
AND NEW.resolution='discarded_clone' AND NEW.executor_kind='session_exec'
AND EXISTS(SELECT 1 FROM prepare_liquibase_sessions s JOIN prepare_clones c ON c.clone_id=s.clone_id
WHERE s.clone_id=NEW.clone_id AND s.job_id=NEW.job_id AND s.worker_generation=NEW.worker_generation
AND s.runner_phase='stopped' AND c.phase IN ('active','quiescent')) AND (EXISTS(SELECT 1 FROM prepare_attempt_evidence p WHERE EXISTS (
SELECT 1 FROM prepare_liquibase_invocations i
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id AND s.job_id=i.job_id
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=s.job_id
JOIN prepare_jobs j ON j.job_id=selected.job_id
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN prepare_step_bindings sb ON sb.job_id=i.job_id AND sb.task_id=i.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
JOIN prepare_attempt_evidence request ON request.evidence_id=json_extract(p.facts_json,'$.observation.request_id')
JOIN prepare_attempt_evidence ack ON ack.evidence_id=json_extract(p.facts_json,'$.observation.ack_id')
JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=json_extract(p.facts_json,'$.observation.stop_id')
JOIN prepare_attempt_evidence q ON q.evidence_id=json_extract(p.facts_json,'$.observation.database_id')
WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id AND i.attempt_no=NEW.attempt_no
AND NEW.executor_kind='session_exec' AND NEW.legacy_unbound=0
AND s.clone_id=NEW.clone_id AND s.worker_generation=NEW.worker_generation
AND selected.outcome_kind='failure' AND selected.event_profile='status_error'
AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND b.execution_digest=NEW.execution_digest AND b.prepare_kind='lb'
AND b.adapter_binding_digest=s.execution_fingerprint AND s.execution_fingerprint=lb.execution_fingerprint
AND b.policy_revision=lb.execution_policy_revision AND b.input_manifest_digest=lb.bundle_manifest_digest
AND b.target_identity_digest=lb.target_identity_digest AND b.executor_image_digest=lb.image_manifest_digest
AND p.job_id=NEW.job_id AND p.task_id=NEW.task_id AND p.attempt_no=NEW.attempt_no
AND p.worker_generation=NEW.worker_generation AND p.kind='selected_failure_disposal'
AND json_extract(p.facts_json,'$.clone_id')=NEW.clone_id
AND json_extract(p.facts_json,'$.execution_digest')=NEW.execution_digest
AND json_extract(p.facts_json,'$.observation.outcome_digest')=selected.outcome_digest
AND json_extract(p.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(p.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(p.facts_json,'$.observation.invocation_ref')=i.invocation_ref
AND json_extract(p.facts_json,'$.observation.clone_generation')=c.clone_generation
AND json_extract(p.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(p.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(p.facts_json,'$.observation.executor_absent')=1
AND json_extract(p.facts_json,'$.observation.runtime_absent')=1
AND json_extract(p.facts_json,'$.observation.original_phase') IN ('planned','exec_requested','exec_completed') AND json_extract(p.facts_json,'$.observation.original_phase')=OLD.phase
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged'
AND stopped.kind='executor_inspection' AND q.kind='db_quiescence'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id AND p.evidence_id>q.evidence_id
AND request.job_id=NEW.job_id AND request.task_id=NEW.task_id AND request.attempt_no=NEW.attempt_no AND request.worker_generation=NEW.worker_generation
AND ack.job_id=NEW.job_id AND ack.task_id=NEW.task_id AND ack.attempt_no=NEW.attempt_no AND ack.worker_generation=NEW.worker_generation
AND stopped.job_id=NEW.job_id AND stopped.task_id=NEW.task_id AND stopped.attempt_no=NEW.attempt_no AND stopped.worker_generation=NEW.worker_generation
AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id AND q.attempt_no=NEW.attempt_no AND q.worker_generation=NEW.worker_generation
AND json_extract(request.facts_json,'$.clone_id')=NEW.clone_id AND json_extract(request.facts_json,'$.execution_digest')=NEW.execution_digest
AND json_extract(ack.facts_json,'$.clone_id')=NEW.clone_id AND json_extract(ack.facts_json,'$.execution_digest')=NEW.execution_digest
AND json_extract(stopped.facts_json,'$.clone_id')=NEW.clone_id AND json_extract(stopped.facts_json,'$.execution_digest')=NEW.execution_digest
AND json_extract(q.facts_json,'$.clone_id')=NEW.clone_id AND json_extract(q.facts_json,'$.execution_digest')=NEW.execution_digest
AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.runtime_absent')=1))
AND (SELECT count(*) FROM prepare_attempt_evidence single WHERE single.job_id=NEW.job_id AND single.task_id=NEW.task_id AND single.attempt_no=NEW.attempt_no AND single.kind='selected_failure_disposal')=1))
BEGIN SELECT RAISE(ABORT,'failed session discard lacks complete exit and physical absence'); END;

CREATE TRIGGER prepare_session_attempt_never_started_guard BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN NEW.executor_kind='session_exec' AND NEW.resolution='never_started'
AND (OLD.resolution IS NOT NULL OR OLD.phase<>'planned' OR NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i
JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id
JOIN prepare_clones c ON c.clone_id=s.clone_id
JOIN prepare_tasks t ON t.job_id=i.job_id AND t.task_id=i.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=NEW.execution_digest
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=s.execution_fingerprint
JOIN prepare_attempt_decisions d ON d.job_id=i.job_id AND d.task_id=i.task_id AND d.attempt_no=i.attempt_no
JOIN prepare_decision_evidence request_link ON request_link.decision_id=d.decision_id
JOIN prepare_attempt_evidence request ON request.evidence_id=request_link.evidence_id
JOIN prepare_decision_evidence ack_link ON ack_link.decision_id=d.decision_id
JOIN prepare_attempt_evidence ack ON ack.evidence_id=ack_link.evidence_id
JOIN prepare_decision_evidence stop_link ON stop_link.decision_id=d.decision_id
JOIN prepare_attempt_evidence stopped ON stopped.evidence_id=stop_link.evidence_id
JOIN prepare_decision_evidence q_link ON q_link.decision_id=d.decision_id
JOIN prepare_attempt_evidence q ON q.evidence_id=q_link.evidence_id
WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id AND i.attempt_no=NEW.attempt_no
AND i.complete_at IS NULL AND s.clone_id=NEW.clone_id AND s.worker_generation=NEW.worker_generation
AND s.runner_phase='stopped' AND c.phase='quiescent' AND NEW.phase='closed'
AND ((NEW.resolution='never_started' AND i.start_requested_at IS NULL)
  OR (NEW.resolution='discarded_clone' AND i.start_requested_at IS NOT NULL AND EXISTS (
    SELECT 1 FROM prepare_events warning WHERE warning.job_id=NEW.job_id AND warning.task_id=NEW.task_id
    AND warning.message LIKE 'WARNING [liquibase_external_effect_replay]%')))
AND b.prepare_kind='lb' AND b.policy_revision='lb-container-v2' AND b.effect_scope='unknown' AND b.enforcement_digest IS NULL
AND b.adapter_binding_digest=lb.execution_fingerprint AND b.input_manifest_digest=lb.bundle_manifest_digest AND lb.execution_policy_revision=b.policy_revision
AND d.choice='replay_fresh' AND d.target_clone_id IS NOT NULL AND d.next_attempt_no=NEW.attempt_no+1 AND d.worker_generation=NEW.worker_generation
AND t.position=s.cursor_position AND ((t.input_kind='state' AND t.input_id=s.cursor_state_id) OR (t.input_kind='image' AND t.input_id=s.cursor_image_id))
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged' AND stopped.kind='executor_inspection' AND q.kind='db_quiescence'
AND request.job_id=NEW.job_id AND request.task_id=NEW.task_id AND request.attempt_no=NEW.attempt_no AND request.worker_generation=NEW.worker_generation
AND ack.job_id=NEW.job_id AND ack.task_id=NEW.task_id AND ack.attempt_no=NEW.attempt_no AND ack.worker_generation=NEW.worker_generation
AND stopped.job_id=NEW.job_id AND stopped.task_id=NEW.task_id AND stopped.attempt_no=NEW.attempt_no AND stopped.worker_generation=NEW.worker_generation
AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id AND q.attempt_no=NEW.attempt_no AND q.worker_generation=NEW.worker_generation
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id AND stopped.evidence_id<q.evidence_id
AND json_extract(request.facts_json,'$.observation.session_id')=s.session_id AND json_extract(ack.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(stopped.facts_json,'$.observation.session_id')=s.session_id AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(request.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(ack.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(stopped.facts_json,'$.observation.runner_id')=s.runner_id AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.runtime_absent')=1
AND NOT EXISTS (SELECT 1 FROM prepare_attempt_evidence cancel WHERE cancel.job_id=NEW.job_id AND cancel.task_id=NEW.task_id
AND cancel.attempt_no=NEW.attempt_no AND cancel.kind='cancellation_requested' AND cancel.worker_generation=NEW.worker_generation)
AND NOT EXISTS (SELECT 1 FROM prepare_attempt_decisions earlier JOIN prepare_liquibase_invocations old
ON old.job_id=earlier.job_id AND old.task_id=earlier.task_id AND old.attempt_no=earlier.attempt_no
WHERE earlier.job_id=NEW.job_id AND earlier.task_id=NEW.task_id AND earlier.choice='replay_fresh'
AND earlier.attempt_no<>NEW.attempt_no AND old.start_requested_at IS NOT NULL)))
BEGIN SELECT RAISE(ABORT,'session non-start lacks ordered physical retirement proof'); END;

CREATE TRIGGER prepare_session_attempt_phase_guard
BEFORE UPDATE OF phase ON prepare_step_attempts
WHEN (OLD.executor_kind='container' AND NEW.phase IN ('exec_requested','exec_completed'))
  OR (OLD.executor_kind='session_exec' AND NOT (
    OLD.phase=NEW.phase
    OR (OLD.phase='planned' AND NEW.phase='exec_requested' AND EXISTS (
      SELECT 1 FROM prepare_liquibase_invocations i WHERE i.job_id=OLD.job_id
        AND i.task_id=OLD.task_id AND i.attempt_no=OLD.attempt_no AND i.start_requested_at IS NOT NULL))
    OR (OLD.phase='exec_requested' AND NEW.phase='exec_completed' AND EXISTS (
      SELECT 1 FROM prepare_liquibase_invocations i WHERE i.job_id=OLD.job_id
        AND i.task_id=OLD.task_id AND i.attempt_no=OLD.attempt_no AND i.complete_at IS NOT NULL))
    OR (NEW.phase='closed' AND NEW.resolution IS NOT NULL)))
BEGIN SELECT RAISE(ABORT,'invalid session attempt phase transition'); END;

CREATE TRIGGER prepare_session_attempt_published_guard
BEFORE UPDATE OF resolution ON prepare_step_attempts
WHEN NEW.executor_kind='session_exec' AND NEW.resolution='published' AND (
  OLD.resolution IS NOT NULL OR OLD.phase<>'exec_completed' OR NEW.phase<>'closed'
  OR NOT EXISTS (SELECT 1 FROM prepare_liquibase_invocations i
    JOIN prepare_liquibase_sessions s ON s.session_id=i.session_id
    JOIN prepare_clones c ON c.clone_id=s.clone_id
    JOIN prepare_tasks t ON t.job_id=NEW.job_id AND t.task_id=NEW.task_id
    JOIN prepare_state_bindings binding ON binding.state_id=t.output_state_id AND binding.execution_digest=NEW.execution_digest
    JOIN prepare_attempt_decisions d ON d.job_id=NEW.job_id AND d.task_id=NEW.task_id
      AND d.attempt_no=NEW.attempt_no AND d.worker_generation=NEW.worker_generation AND d.choice='publish_original'
    JOIN prepare_decision_evidence complete_link ON complete_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence complete ON complete.evidence_id=complete_link.evidence_id
    JOIN prepare_decision_evidence q_link ON q_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence q ON q.evidence_id=q_link.evidence_id
    JOIN prepare_decision_evidence pub_link ON pub_link.decision_id=d.decision_id
    JOIN prepare_attempt_evidence pub ON pub.evidence_id=pub_link.evidence_id
    WHERE i.job_id=NEW.job_id AND i.task_id=NEW.task_id AND i.attempt_no=NEW.attempt_no
      AND i.complete_at IS NOT NULL AND i.exit_code=0 AND s.clone_id=NEW.clone_id
      AND s.worker_generation=NEW.worker_generation AND s.db_phase='stopped' AND c.phase='quiescent'
      AND complete.kind='exec_completion' AND complete.job_id=NEW.job_id AND complete.task_id=NEW.task_id
      AND complete.attempt_no=NEW.attempt_no AND complete.worker_generation=NEW.worker_generation
      AND json_extract(complete.facts_json,'$.observation.session_id')=s.session_id
      AND json_extract(complete.facts_json,'$.observation.runner_id')=s.runner_id
      AND json_extract(complete.facts_json,'$.observation.invocation_ref')=i.invocation_ref
      AND json_extract(complete.facts_json,'$.observation.manifest_digest')=i.completion_digest
      AND json_extract(complete.facts_json,'$.observation.exit_code')=0
      AND json_extract(complete.facts_json,'$.observation.children_absent')=1
      AND q.kind='db_quiescence' AND q.job_id=NEW.job_id AND q.task_id=NEW.task_id
      AND q.attempt_no=NEW.attempt_no AND q.worker_generation=NEW.worker_generation AND q.evidence_id>complete.evidence_id
      AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
      AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
      AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
      AND json_extract(q.facts_json,'$.observation.postgres_stopped')=1
      AND json_extract(q.facts_json,'$.observation.runtime_present')=1
      AND pub.kind='publication' AND pub.job_id=NEW.job_id AND pub.task_id=NEW.task_id
      AND pub.attempt_no=NEW.attempt_no AND pub.worker_generation=NEW.worker_generation AND pub.evidence_id>q.evidence_id
      AND json_extract(pub.facts_json,'$.observation.state_id')=t.output_state_id))
BEGIN SELECT RAISE(ABORT,'session publication lacks ordered child and database proof'); END;

CREATE TRIGGER prepare_session_attempt_shape_insert
BEFORE INSERT ON prepare_step_attempts
WHEN (NEW.executor_kind='container' AND NEW.planned_executor_name IS NULL)
  OR (NEW.executor_kind='session_exec' AND
      (NEW.planned_executor_name IS NOT NULL OR NEW.executor_id IS NOT NULL
       OR NOT EXISTS (SELECT 1 FROM prepare_jobs j
         JOIN prepare_tasks t ON t.job_id=j.job_id
         WHERE j.job_id=NEW.job_id AND j.prepare_kind='lb'
           AND j.purpose='prepare' AND t.task_id=NEW.task_id
           AND t.type='state_execute')))
BEGIN SELECT RAISE(ABORT,'invalid attempt executor shape'); END;

CREATE TRIGGER prepare_session_attempt_shape_update
BEFORE UPDATE OF executor_kind,planned_executor_name,executor_id ON prepare_step_attempts
WHEN OLD.executor_kind IS NOT NEW.executor_kind
  OR (NEW.executor_kind='container' AND NEW.planned_executor_name IS NULL)
  OR (NEW.executor_kind='session_exec' AND
      (NEW.planned_executor_name IS NOT NULL OR NEW.executor_id IS NOT NULL))
BEGIN SELECT RAISE(ABORT,'attempt executor shape is immutable'); END;

CREATE TRIGGER prepare_session_boundary_cancel_invocation_guard BEFORE INSERT ON prepare_liquibase_invocations
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_sessions WHERE session_id=NEW.session_id AND EXISTS (SELECT 1 FROM prepare_attempt_evidence cancel
JOIN prepare_liquibase_invocations previous USING(job_id,task_id,attempt_no)
WHERE previous.session_id=prepare_liquibase_sessions.session_id
AND cancel.kind='cancellation_requested' AND cancel.worker_generation=prepare_liquibase_sessions.worker_generation
AND json_extract(cancel.facts_json,'$.observation.session_id')=prepare_liquibase_sessions.session_id
AND json_extract(cancel.facts_json,'$.observation.runner_id')=prepare_liquibase_sessions.runner_id
AND json_extract(cancel.facts_json,'$.observation.session_scope')=1))
BEGIN SELECT RAISE(ABORT,'cancelled session cannot start a suffix invocation'); END;

CREATE TRIGGER prepare_session_boundary_cancel_restore_guard BEFORE INSERT ON prepare_liquibase_cache_restores
WHEN EXISTS (SELECT 1 FROM prepare_liquibase_sessions WHERE session_id=NEW.session_id AND EXISTS (SELECT 1 FROM prepare_attempt_evidence cancel
JOIN prepare_liquibase_invocations previous USING(job_id,task_id,attempt_no)
WHERE previous.session_id=prepare_liquibase_sessions.session_id
AND cancel.kind='cancellation_requested' AND cancel.worker_generation=prepare_liquibase_sessions.worker_generation
AND json_extract(cancel.facts_json,'$.observation.session_id')=prepare_liquibase_sessions.session_id
AND json_extract(cancel.facts_json,'$.observation.runner_id')=prepare_liquibase_sessions.runner_id
AND json_extract(cancel.facts_json,'$.observation.session_scope')=1))
BEGIN SELECT RAISE(ABORT,'cancelled session cannot restore a suffix'); END;

CREATE TRIGGER prepare_session_cancellation_execution_guard BEFORE UPDATE OF phase ON prepare_step_attempts
WHEN NEW.executor_kind='session_exec' AND OLD.phase<>NEW.phase AND NEW.phase IN ('exec_requested','exec_completed')
AND EXISTS (SELECT 1 FROM prepare_attempt_evidence e WHERE e.job_id=OLD.job_id AND e.task_id=OLD.task_id
AND e.attempt_no=OLD.attempt_no AND e.worker_generation=OLD.worker_generation AND e.kind='cancellation_requested')
BEGIN SELECT RAISE(ABORT,'cancelled session invocation cannot execute or publish'); END;

CREATE TRIGGER prepare_state_binding_delete_guard BEFORE DELETE ON prepare_state_bindings
WHEN EXISTS (SELECT 1 FROM states WHERE state_id=OLD.state_id)
BEGIN SELECT RAISE(ABORT,'published state binding is retained'); END;

CREATE TRIGGER prepare_state_binding_guard BEFORE INSERT ON prepare_state_bindings
WHEN NOT EXISTS (
  SELECT 1 FROM states s JOIN prepare_execution_bindings b
    ON b.execution_digest=NEW.execution_digest
  WHERE s.state_id=NEW.state_id AND s.prepare_kind=b.prepare_kind
    AND (s.prepare_kind<>'lb' OR EXISTS (
      SELECT 1 FROM liquibase_state_bindings lb
      WHERE lb.state_id=s.state_id AND lb.execution_fingerprint=b.adapter_binding_digest)))
BEGIN SELECT RAISE(ABORT,'state execution binding differs'); END;

CREATE TRIGGER prepare_state_binding_immutable BEFORE UPDATE ON prepare_state_bindings
BEGIN SELECT RAISE(ABORT,'state binding is immutable'); END;

CREATE TRIGGER prepare_step_binding_immutable BEFORE UPDATE ON prepare_step_bindings
BEGIN SELECT RAISE(ABORT,'step binding is immutable'); END;

CREATE TRIGGER prepare_step_binding_kind_guard BEFORE INSERT ON prepare_step_bindings
WHEN NOT EXISTS (
  SELECT 1 FROM prepare_jobs j JOIN prepare_execution_bindings b
  ON b.execution_digest=NEW.execution_digest
  WHERE j.job_id=NEW.job_id AND j.prepare_kind=b.prepare_kind)
BEGIN SELECT RAISE(ABORT,'step execution kind differs from job'); END;

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
