package prepare

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/store"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
)

// Uses the actual planner and persistent adapters: utility-only key tests do not
// prove that cache explanation and queued preparation select the same identity.
func TestManagedPreparePlanningAndRecovery(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
	if err := queue.InstallManagedSchema(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	st, err := storesqlite.New(db)
	if err != nil {
		t.Fatal(err)
	}
	qs, err := queue.New(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := managedidentity.NewService(st)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := managedidentity.NewOwner(service, format.DomainRef, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	rt := &fakeRuntime{}
	m := newManagerWithDeps(t, st, qs, &testDeps{runtime: rt})
	m.identity = owner
	req := Request{PrepareKind: "psql", ImageID: "postgres@sha256:" + strings.Repeat("a", 64), PsqlArgs: []string{"-c", "SELECT 1"}}
	first, err := m.CacheExplain(ctx, req)
	if err != nil || first.Decision != "miss" {
		t.Fatalf("first plan: %+v %v", first, err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM managed_base_lineages").Scan(&count); err != nil || count != 1 {
		t.Fatal("plan did not reserve one lineage", err)
	}
	for _, table := range []string{"states", "instances", "prepare_jobs"} {
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("plan wrote %s", table)
		}
	}
	if len(rt.initCalls) != 0 || len(rt.startCalls) != 0 {
		t.Fatal("metadata reservation initialized a database")
	}
	req.PlanOnly = true
	accepted, err := m.Submit(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	job, ok, err := qs.GetJob(ctx, accepted.JobID)
	if err != nil || !ok || job.Status != StatusSucceeded || job.LineageRef == "" || job.ResolvedImageID != req.ImageID {
		t.Fatalf("job binding: %+v %v", job, err)
	}
	restored, err := m.prepareFromJob(job)
	if err != nil || restored.managed.LineageRef != job.LineageRef || restored.managed.IdentityDigest != job.IdentityDigest {
		t.Fatal("job recovery lost binding", err)
	}
	tasks, err := qs.ListTasks(ctx, job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	var stateID string
	for _, task := range tasks {
		if task.Type == "state_execute" {
			stateID = valueOrEmpty(task.OutputStateID)
			if valueOrEmpty(task.InputID) != restored.managed.Key {
				t.Fatal("planner did not start from identity-bound base key")
			}
		}
	}
	if stateID == "" {
		t.Fatal("plan has no output")
	}
	if err := m.validateManagedTasks(restored, tasks); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"input", "output", "hash", "image", "empty"} {
		t.Run("reject recovered "+field, func(t *testing.T) {
			broken := append([]queue.TaskRecord(nil), tasks...)
			if field == "image" {
				broken = append(broken, queue.TaskRecord{Type: "resolve_image", ResolvedImageID: strPtr("postgres@sha256:" + strings.Repeat("f", 64))})
			}
			for i := range broken {
				if broken[i].Type == "resolve_image" && field == "image" {
					broken[i].ResolvedImageID = strPtr("postgres@sha256:" + strings.Repeat("f", 64))
				}
				if broken[i].Type != "state_execute" {
					continue
				}
				switch field {
				case "input":
					broken[i].InputID = strPtr("foreign-base")
				case "output":
					broken[i].OutputStateID = strPtr("foreign-state")
				case "hash":
					broken[i].TaskHash = nil
				case "empty":
					broken[i].Type = "plan"
				}
			}
			if err := m.validateManagedTasks(restored, broken); err == nil {
				t.Fatal("accepted corrupted task chain")
			}
		})
	}
	if err := m.bindManagedRequest(ctx, &restored, nil); err != nil {
		t.Fatal(err)
	}
	bareDigest := restored
	bareDigest.resolvedImageID = "sha256:" + strings.Repeat("a", 64)
	if err := m.bindManagedRequest(ctx, &bareDigest, &job); err != nil || bareDigest.managed != restored.managed {
		t.Fatal("bare resolved digest changed recovered binding", err)
	}
	if err := m.bindManagedRequest(ctx, nil, nil); err == nil {
		t.Fatal("accepted nil request")
	}
	invalid := restored
	invalid.managed.IdentityDigest = "invalid"
	if _, err := m.isManagedStateCached(stateID, invalid); err == nil {
		t.Fatal("accepted invalid binding")
	}
	if err := st.CreateState(ctx, store.StateCreate{StateID: stateID, StateFingerprint: stateID, ImageID: req.ImageID, PrepareKind: "psql", PrepareArgsNormalized: restored.argsNormalized, CreatedAt: job.CreatedAt, LineageRef: job.LineageRef, IdentityDigest: job.IdentityDigest}); err != nil {
		t.Fatal(err)
	}
	req.PlanOnly = false
	second, err := m.CacheExplain(ctx, req)
	if err != nil || second.Decision != "hit" || second.MatchedStateID != stateID || second.Signature != first.Signature {
		t.Fatalf("plan/prepare cache parity: %+v %v", second, err)
	}
	for _, image := range []string{"docker.io/library/" + req.ImageID, "mirror.example/renamed@sha256:" + strings.Repeat("a", 64)} {
		t.Run("cached image alias "+image, func(t *testing.T) {
			alias := req
			alias.ImageID = image
			cached, err := m.CacheExplain(ctx, alias)
			if err != nil || cached.Decision != "hit" || cached.MatchedStateID != stateID {
				t.Fatalf("same digest did not reuse cached state: %+v %v", cached, err)
			}
		})
	}
	foreign := restored
	foreign.resolvedImageID = "postgres@sha256:" + strings.Repeat("f", 64)
	if _, err := m.isManagedStateCached(stateID, foreign); err == nil {
		t.Fatal("accepted foreign cached state")
	}
	bad := job
	bad.LineageRef = "missing"
	if _, err := m.prepareFromJob(bad); err == nil {
		t.Fatal("recovery selected a replacement for missing lineage")
	}
	bad = job
	bad.IdentityDigest = strings.Repeat("f", 64)
	if _, err := m.prepareFromJob(bad); err == nil {
		t.Fatal("recovery accepted a changed digest")
	}
	other, err := managedidentity.NewOwner(service, format.DomainRef, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	m.identity = other
	third, err := m.CacheExplain(ctx, req)
	if err != nil || third.Decision != "miss" || third.Signature == first.Signature {
		t.Fatalf("changed initialization reused cache: %+v %v", third, err)
	}
	if len(rt.initCalls) != 0 || len(rt.startCalls) != 0 {
		t.Fatal("planning created a runtime")
	}
}
