package prepare

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

type mainPathImageResolver struct {
	calls int
	image engineRuntime.RunnerImageBinding
}

func (r *mainPathImageResolver) Resolve(context.Context, string, engineRuntime.RunnerPlatform) (engineRuntime.RunnerImageBinding, error) {
	r.calls++
	return r.image, nil
}

type mainPathImageRuntime struct{ ensures int }

func (*mainPathImageRuntime) RunnerPlatform(context.Context) (engineRuntime.RunnerPlatform, error) {
	return engineRuntime.RunnerPlatform{OS: "linux", Architecture: "amd64"}, nil
}
func (r *mainPathImageRuntime) EnsureRunnerImage(_ context.Context, b engineRuntime.RunnerImageBinding) (string, error) {
	r.ensures++
	return b.DigestRef, nil
}
func (*mainPathImageRuntime) InspectRunnerImage(context.Context, engineRuntime.RunnerImageBinding) (engineRuntime.RunnerImageInspection, error) {
	return engineRuntime.RunnerImageInspection{LiquibaseVersion: "5.0.4", JavaMajor: 21, PostgreSQLJars: []engineRuntime.RunnerJar{{Path: "/liquibase/lib/postgresql.jar", Digest: "sha256:" + strings.Repeat("b", 64)}}}, nil
}

// MP07: actual admission/binding must retain captured bytes and a pinned runner
// before the worker can plan. This is not a Docker or restart/replay claim.
func TestMainPathContainerBindingOwnsInputsAndImage(t *testing.T) {
	m, base, _, _, _ := managedExecutionFixture(t)
	project := filepath.Join(m.stateStoreRoot, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "changelog.sql")
	before := []byte("--liquibase formatted sql\n--changeset sqlrs:1\nSELECT 1;\n")
	if err := os.WriteFile(file, before, 0600); err != nil {
		t.Fatal(err)
	}
	resolver := &mainPathImageResolver{image: engineRuntime.RunnerImageBinding{RequestedTag: "latest", Platform: engineRuntime.RunnerPlatform{OS: "linux", Architecture: "amd64"}, ManifestDigest: "sha256:" + strings.Repeat("c", 64), DigestRef: "liquibase/liquibase@sha256:" + strings.Repeat("c", 64)}}
	m.containerLiquibase = &LiquibaseContainerDeps{Images: resolver, Runtime: &mainPathImageRuntime{}, Sources: inputbundle.Store{Root: filepath.Join(m.stateStoreRoot, "bundles")}, MaterializedRoot: filepath.Join(m.stateStoreRoot, "materialized")}
	p, err := m.prepareRequest(Request{PrepareKind: "lb", ImageID: base.request.ImageID, LiquibaseVersion: "latest", WorkDir: project, LiquibaseArgs: []string{"update", "--changelog-file", "changelog.sql"}})
	if err != nil {
		t.Fatal(err)
	}
	p.managed, p.resolvedImageID = base.managed, base.resolvedImageID
	job := queue.JobRecord{JobID: "main-bind", Status: StatusQueued, PrepareKind: "lb", ImageID: p.request.ImageID, ResolvedImageID: p.resolvedImageID, LineageRef: p.managed.LineageRef, IdentityDigest: p.managed.IdentityDigest, CreatedAt: "2026-11-07T12:00:00Z"}
	if err := m.admitContainerLiquibase(t.Context(), job, p); err != nil {
		t.Fatal(err)
	}
	saved, found, err := m.queue.GetJob(t.Context(), job.JobID)
	if err != nil || !found || saved.RequestJSON == nil {
		t.Fatal("admitted request missing", err)
	}
	var admitted map[string]any
	if err := json.Unmarshal([]byte(*saved.RequestJSON), &admitted); err != nil {
		t.Fatal(err)
	}
	if admitted["_sqlrs_container_admission"] == nil {
		t.Fatal("missing immutable container coordinates")
	}
	if err := os.WriteFile(file, []byte("changed after admission"), 0600); err != nil {
		t.Fatal(err)
	}
	bound, cleanup, err := m.bindContainerLiquibase(t.Context(), job.JobID, p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	}()
	if resolver.calls != 1 || bound.liquibaseImageID != resolver.image.DigestRef || !canonicalSHA256.MatchString(bound.liquibaseFingerprint) {
		t.Fatal("runner was not pinned")
	}
	if len(bound.liquibaseMounts) != 1 || bound.liquibaseMounts[0].HostPath == project {
		t.Fatal("mutable source was not replaced")
	}
	if bytes, err := os.ReadFile(filepath.Join(bound.liquibaseMounts[0].HostPath, "changelog.sql")); err != nil || string(bytes) != string(before) {
		t.Fatal("admitted bytes changed", err)
	}
	q := m.queue.(*queue.SQLiteStore)
	record, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID)
	if err != nil || !found || record.ExecutionFingerprint != bound.liquibaseFingerprint || record.ImageManifestDigest != resolver.image.ManifestDigest {
		t.Fatal("pinned journal differs", err)
	}
}
