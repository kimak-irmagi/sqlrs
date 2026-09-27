//go:build ignore

// Command generate creates the historical rc.6 compatibility fixture from the
// schema at commit 1752abbb44e72e5d4eaf6d310e70e5777a1bc9b0
// (tag v0.1.1-rc.6). It intentionally does not import current store code.
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite"
)

const legacySchema = `
CREATE TABLE states (state_id TEXT PRIMARY KEY,parent_state_id TEXT,state_fingerprint TEXT,image_id TEXT NOT NULL,prepare_kind TEXT NOT NULL,prepare_args_normalized TEXT NOT NULL,created_at TEXT NOT NULL,size_bytes INTEGER,last_used_at TEXT,use_count INTEGER,min_retention_until TEXT,evicted_at TEXT,eviction_reason TEXT,status TEXT);
CREATE UNIQUE INDEX idx_states_fingerprint ON states(state_fingerprint); CREATE INDEX idx_states_parent ON states(parent_state_id); CREATE INDEX idx_states_image ON states(image_id); CREATE INDEX idx_states_kind ON states(prepare_kind);
CREATE TABLE instances (instance_id TEXT PRIMARY KEY,state_id TEXT NOT NULL,image_id TEXT NOT NULL,created_at TEXT NOT NULL,expires_at TEXT,runtime_id TEXT,runtime_dir TEXT,status TEXT,FOREIGN KEY(state_id) REFERENCES states(state_id));
CREATE INDEX idx_instances_state ON instances(state_id); CREATE INDEX idx_instances_image ON instances(image_id); CREATE INDEX idx_instances_expires ON instances(expires_at);
CREATE TABLE names (name TEXT PRIMARY KEY,instance_id TEXT,state_id TEXT,state_fingerprint TEXT NOT NULL,image_id TEXT NOT NULL,last_used_at TEXT,is_primary INTEGER NOT NULL DEFAULT 0);
CREATE INDEX idx_names_instance ON names(instance_id); CREATE INDEX idx_names_state ON names(state_id); CREATE INDEX idx_names_image ON names(image_id); CREATE INDEX idx_names_primary ON names(instance_id,is_primary);
CREATE TABLE prepare_jobs (job_id TEXT PRIMARY KEY,status TEXT NOT NULL,prepare_kind TEXT NOT NULL,image_id TEXT NOT NULL,plan_only INTEGER NOT NULL DEFAULT 0,snapshot_mode TEXT NOT NULL DEFAULT 'always',prepare_args_normalized TEXT,signature TEXT,request_json TEXT,created_at TEXT NOT NULL,started_at TEXT,finished_at TEXT,result_json TEXT,error_json TEXT);
CREATE INDEX idx_prepare_jobs_status ON prepare_jobs(status);
CREATE TABLE prepare_tasks (job_id TEXT NOT NULL,task_id TEXT NOT NULL,position INTEGER NOT NULL,type TEXT NOT NULL,status TEXT NOT NULL,planner_kind TEXT,input_kind TEXT,input_id TEXT,image_id TEXT,resolved_image_id TEXT,task_hash TEXT,output_state_id TEXT,cached INTEGER,instance_mode TEXT,changeset_id TEXT,changeset_author TEXT,changeset_path TEXT,started_at TEXT,finished_at TEXT,error_json TEXT,PRIMARY KEY(job_id,task_id),FOREIGN KEY(job_id) REFERENCES prepare_jobs(job_id) ON DELETE CASCADE);
CREATE INDEX idx_prepare_tasks_job ON prepare_tasks(job_id); CREATE INDEX idx_prepare_tasks_status ON prepare_tasks(status); CREATE INDEX idx_prepare_tasks_position ON prepare_tasks(job_id,position);
CREATE TABLE prepare_events (seq INTEGER PRIMARY KEY AUTOINCREMENT,job_id TEXT NOT NULL,type TEXT NOT NULL,ts TEXT NOT NULL,status TEXT,task_id TEXT,message TEXT,result_json TEXT,error_json TEXT,FOREIGN KEY(job_id) REFERENCES prepare_jobs(job_id) ON DELETE CASCADE);
CREATE INDEX idx_prepare_events_job_seq ON prepare_events(job_id,seq);
CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE VIEW unrelated_state_summary AS SELECT state_id,image_id FROM states;
`

func main() {
	_, source, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(source), "populated-rc6.db")
	_ = os.Remove(path)
	db, err := sql.Open("sqlite", path)
	check(err)
	_, err = db.Exec(`PRAGMA foreign_keys=ON;` + legacySchema + `
INSERT INTO states(state_id,parent_state_id,state_fingerprint,image_id,prepare_kind,prepare_args_normalized,created_at,size_bytes,status) VALUES
 ('rc6-root',NULL,'rc6-root-fingerprint','postgres:16','psql','{"files":["001.sql"]}','2026-01-01T00:00:00Z',1024,'ready'),
 ('rc6-child','rc6-root','rc6-child-fingerprint','postgres:16','liquibase','{"changelog":"db.xml"}','2026-01-02T00:00:00Z',2048,'ready');
INSERT INTO instances(instance_id,state_id,image_id,created_at,runtime_id,status) VALUES('rc6-instance','rc6-child','postgres:16','2026-01-03T00:00:00Z','container-rc6','active');
INSERT INTO names(name,instance_id,state_id,state_fingerprint,image_id,is_primary) VALUES('rc6-main','rc6-instance','rc6-child','rc6-child-fingerprint','postgres:16',1);
INSERT INTO prepare_jobs(job_id,status,prepare_kind,image_id,request_json,created_at) VALUES('rc6-job','succeeded','psql','postgres:16','{}','2026-01-04T00:00:00Z');
INSERT INTO prepare_tasks(job_id,task_id,position,type,status,output_state_id) VALUES('rc6-job','prepare',0,'prepare','succeeded','rc6-child');
INSERT INTO prepare_events(job_id,type,ts,status,message) VALUES('rc6-job','completed','2026-01-04T00:00:01Z','succeeded','kept');
INSERT INTO settings(key,value) VALUES('fixture_revision','1752abbb44e72e5d4eaf6d310e70e5777a1bc9b0');`)
	check(err)
	check(db.Close())
	raw, err := os.ReadFile(path)
	check(err)
	digest := sha256.Sum256(raw)
	fmt.Println("sha256:" + hex.EncodeToString(digest[:]))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
