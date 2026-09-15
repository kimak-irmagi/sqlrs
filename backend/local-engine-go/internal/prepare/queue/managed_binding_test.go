package queue

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/managedidentity"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
)

// Recovery reads the original image and lineage even if no task was persisted.
func TestManagedJobBindingSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "queue.db")
	db, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	format, err := storesqlite.EnsureManagedStoreFormat(ctx, tx, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := storesqlite.InstallManagedStateSchema(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := InstallManagedSchema(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	identity, err := managedidentity.Generate(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	digest := identity.IdentityDigest
	image := "postgres@sha256:" + strings.Repeat("a", 64)
	if _, err := db.Exec(`INSERT INTO managed_base_lineages VALUES (?,?,?,?,?,?,?,?,?)`, format.DomainRef, identity.LineageRef, "postgres", strings.TrimPrefix(image, "postgres@"), strings.Repeat("b", 64), identity.PolicyVersion, identity.Username, digest, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	st := &SQLiteStore{db: db}
	signature := "signature"
	job := JobRecord{JobID: "job", Status: "queued", PrepareKind: "psql", ImageID: "postgres:17", CreatedAt: "now", Signature: &signature, ResolvedImageID: image, LineageRef: identity.LineageRef, IdentityDigest: digest}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st = &SQLiteStore{db: db}
	got, ok, err := st.GetJob(ctx, job.JobID)
	if err != nil || !ok || got.LineageRef != job.LineageRef || got.IdentityDigest != digest || got.ResolvedImageID != image {
		t.Fatalf("lost recovery binding: %+v %v", got, err)
	}
	for _, list := range []func() ([]JobRecord, error){
		func() ([]JobRecord, error) { return st.ListJobs(ctx, job.JobID) },
		func() ([]JobRecord, error) { return st.ListJobsByStatus(ctx, []string{"queued"}) },
		func() ([]JobRecord, error) { return st.ListJobsBySignature(ctx, signature, []string{"queued"}) },
	} {
		rows, err := list()
		if err != nil || len(rows) != 1 || rows[0].IdentityDigest != digest || rows[0].LineageRef != identity.LineageRef || rows[0].ResolvedImageID != image {
			t.Fatalf("list lost binding: %+v %v", rows, err)
		}
	}
	if _, err := db.Exec("UPDATE prepare_jobs SET identity_digest='changed' WHERE job_id='job'"); err == nil {
		t.Fatal("job binding changed")
	}
	bad := job
	bad.JobID = "invalid"
	bad.LineageRef = "missing"
	if err := st.CreateJob(ctx, bad); err == nil {
		t.Fatal("missing lineage accepted")
	}
	if _, ok, err := st.GetJob(ctx, bad.JobID); err != nil || ok {
		t.Fatal("partial job binding")
	}
}

func TestManagedQueueSchemaRejectsUnavailableTransaction(t *testing.T) {
	ctx := context.Background()
	if err := InstallManagedSchema(ctx, nil); err != errManagedSchema {
		t.Fatalf("nil transaction: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := InstallManagedSchema(canceled, nil); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := InstallManagedSchema(ctx, tx); err != errManagedSchema {
		t.Fatalf("closed transaction: %v", err)
	}
}
