package prepare

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// CT03: a real refusal at the binding write must not publish its preceding
// resolution/inventory as a committed execution, or retry either image action.
func TestCostTrimLiquibaseBindingWriteRefusal(t *testing.T) {
	m, p, job, resolver, rt := costTrimBindingFixture(t)
	db, err := sql.Open("sqlite", filepath.Join(m.stateStoreRoot, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TRIGGER cost_trim_binding_refusal BEFORE INSERT ON liquibase_execution_bindings
BEGIN SELECT RAISE(ABORT,'cost_trim_binding_write_denied'); END`); err != nil {
		t.Fatal(err)
	}
	q := m.queue.(*queue.SQLiteStore)
	before, found, err := q.GetLiquibaseJobBinding(t.Context(), job.JobID)
	if err != nil || !found {
		t.Fatal("admitted source missing before write fault", err)
	}
	_, cleanup, err := m.bindContainerLiquibase(t.Context(), job.JobID, p)
	costTrimBindingCleanup(t, cleanup)
	if err == nil || !strings.Contains(err.Error(), "cost_trim_binding_write_denied") ||
		resolver.calls != 1 || !reflect.DeepEqual(rt.actions, []string{"ensure", "inventory"}) {
		t.Fatal("binding failure did not reach its exact write without retry", err, rt.actions)
	}
	after, found, err := q.GetLiquibaseJobBinding(t.Context(), job.JobID)
	if err != nil || !found || !reflect.DeepEqual(before, after) {
		t.Fatal("binding write fault changed the admitted source", err)
	}
	if _, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID); err != nil || found {
		t.Fatal("failed binding returned a persisted execution", found, err)
	}
	var executions, signed, tasks int
	for query, target := range map[string]*int{
		`SELECT count(*) FROM liquibase_execution_bindings`:             &executions,
		`SELECT count(*) FROM prepare_jobs WHERE signature IS NOT NULL`: &signed,
		`SELECT count(*) FROM prepare_tasks`:                            &tasks,
	} {
		if err := db.QueryRow(query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if executions != 0 || signed != 0 || tasks != 0 {
		t.Fatal("refused bind created execution/signature/tasks", executions, signed, tasks)
	}
}

// CT03/O2: duplicate image availability checks are not part of the binding
// contract. Fresh resolution and saved-binding entry still each ensure once.
// See docs/architecture/issue89-cost-attribution-and-trimming.md §2.
func TestCostTrimLiquibaseBindingEnsuresOnce(t *testing.T) {
	for _, variant := range []string{"fresh", "saved", "ensure-refused", "inventory-refused", "next-job-moved-tag"} {
		t.Run(variant, func(t *testing.T) {
			m, p, job, resolver, rt := costTrimBindingFixture(t)
			denied := errors.New("cost-trim-binding-denied")
			if variant == "saved" {
				_, cleanup, err := m.bindContainerLiquibase(t.Context(), job.JobID, p)
				costTrimBindingCleanup(t, cleanup)
				if err != nil {
					t.Fatal(err)
				}
				rt.actions = nil
			}
			if variant == "ensure-refused" {
				rt.ensureErr = denied
			}
			if variant == "inventory-refused" {
				rt.inventoryErr = denied
			}
			bound, cleanup, err := m.bindContainerLiquibase(t.Context(), job.JobID, p)
			costTrimBindingCleanup(t, cleanup)
			q := m.queue.(*queue.SQLiteStore)
			saved, found, readErr := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			wantActions := []string{"ensure", "inventory"}
			if variant == "saved" || variant == "ensure-refused" {
				wantActions = []string{"ensure"}
			}
			if !reflect.DeepEqual(rt.actions, wantActions) {
				t.Fatalf("availability/inventory calls=%v want=%v", rt.actions, wantActions)
			}
			if resolver.calls != 1 {
				t.Fatalf("resolution calls=%d want=1", resolver.calls)
			}
			if variant == "ensure-refused" || variant == "inventory-refused" {
				if !errors.Is(err, denied) || found {
					t.Fatalf("failed binding published an execution: found=%t err=%v", found, err)
				}
				return
			}
			if err != nil || !found || bound.liquibaseImageID != resolver.image.DigestRef ||
				saved.ImageManifestDigest != resolver.image.ManifestDigest || saved.ExecutionFingerprint != bound.liquibaseFingerprint {
				t.Fatalf("pinned execution differs: found=%t err=%v", found, err)
			}
			data, err := os.ReadFile(filepath.Join(bound.liquibaseMounts[0].HostPath, "changelog.sql"))
			if err != nil || string(data) != "--liquibase formatted sql\n--changeset sqlrs:1\nSELECT 1;\n" {
				t.Fatal("immutable admitted input changed", err)
			}
			if variant != "next-job-moved-tag" {
				return
			}
			job.JobID = "cost-binding-next"
			resolver.image.ManifestDigest = "sha256:" + strings.Repeat("e", 64)
			resolver.image.DigestRef = "liquibase/liquibase@" + resolver.image.ManifestDigest
			if err := m.admitContainerLiquibase(t.Context(), job, p); err != nil {
				t.Fatal(err)
			}
			rt.actions = nil
			next, nextCleanup, err := m.bindContainerLiquibase(t.Context(), job.JobID, p)
			costTrimBindingCleanup(t, nextCleanup)
			if err != nil || resolver.calls != 2 || !reflect.DeepEqual(rt.actions, []string{"ensure", "inventory"}) ||
				next.liquibaseImageID != resolver.image.DigestRef || next.liquibaseFingerprint == saved.ExecutionFingerprint {
				t.Fatalf("new job reused old tag/binding: resolve=%d actions=%v err=%v", resolver.calls, rt.actions, err)
			}
			previous, found, err := q.GetLiquibaseExecutionForJob(t.Context(), "cost-binding")
			if err != nil || !found || !reflect.DeepEqual(previous, saved) {
				t.Fatal("new tag changed the prior immutable binding", err)
			}
		})
	}
}

func costTrimBindingFixture(t *testing.T) (*PrepareService, preparedRequest, queue.JobRecord, *mainPathImageResolver, *costTrimImageRuntime) {
	t.Helper()
	m, base, _, _, _ := managedExecutionFixture(t)
	project := filepath.Join(m.stateStoreRoot, "cost-project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "changelog.sql"), []byte("--liquibase formatted sql\n--changeset sqlrs:1\nSELECT 1;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	resolver := &mainPathImageResolver{image: engineRuntime.RunnerImageBinding{RequestedTag: "latest",
		Platform:       engineRuntime.RunnerPlatform{OS: "linux", Architecture: "amd64"},
		ManifestDigest: "sha256:" + strings.Repeat("c", 64), DigestRef: "liquibase/liquibase@sha256:" + strings.Repeat("c", 64)}}
	rt := &costTrimImageRuntime{}
	m.containerLiquibase = &LiquibaseContainerDeps{Images: resolver, Runtime: rt, Sources: inputbundle.Store{Root: filepath.Join(m.stateStoreRoot, "cost-bundles")}, MaterializedRoot: filepath.Join(m.stateStoreRoot, "cost-materialized")}
	p, err := m.prepareRequest(Request{PrepareKind: "lb", ImageID: base.request.ImageID, LiquibaseVersion: "latest", WorkDir: project,
		LiquibaseArgs: []string{"update", "--changelog-file", "changelog.sql"}})
	if err != nil {
		t.Fatal(err)
	}
	p.managed, p.resolvedImageID = base.managed, base.resolvedImageID
	job := queue.JobRecord{JobID: "cost-binding", Status: StatusQueued, PrepareKind: "lb", ImageID: p.request.ImageID,
		ResolvedImageID: p.resolvedImageID, LineageRef: p.managed.LineageRef, IdentityDigest: p.managed.IdentityDigest, CreatedAt: "2026-11-10T00:00:00Z"}
	if err := m.admitContainerLiquibase(t.Context(), job, p); err != nil {
		t.Fatal(err)
	}
	return m, p, job, resolver, rt
}

func costTrimBindingCleanup(t *testing.T, cleanup func() error) {
	t.Helper()
	if cleanup != nil {
		t.Cleanup(func() {
			if err := cleanup(); err != nil {
				t.Error(err)
			}
		})
	}
}

type costTrimImageRuntime struct {
	mainPathImageRuntime
	actions                 []string
	ensureErr, inventoryErr error
}

func (r *costTrimImageRuntime) EnsureRunnerImage(_ context.Context, binding engineRuntime.RunnerImageBinding) (string, error) {
	r.actions = append(r.actions, "ensure")
	return binding.DigestRef, r.ensureErr
}

func (r *costTrimImageRuntime) InspectRunnerImage(ctx context.Context, binding engineRuntime.RunnerImageBinding) (engineRuntime.RunnerImageInspection, error) {
	r.actions = append(r.actions, "inventory")
	if r.inventoryErr != nil {
		return engineRuntime.RunnerImageInspection{}, r.inventoryErr
	}
	return r.mainPathImageRuntime.InspectRunnerImage(ctx, binding)
}
