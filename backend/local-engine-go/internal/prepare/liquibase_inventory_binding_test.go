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

	"github.com/sqlrs/engine-local/internal/prepare/driverbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// IC06 uses real admission, queue, CAS and projections. Only image discovery
// and the packaged-driver capability are injected; native IC08 checks Docker.
func TestLiquibaseInventoryBindingKeepsPerJobWork(t *testing.T) {
	for _, mode := range []string{"embedded", "mounted", "publish-failure", "binding-failure"} {
		t.Run(mode, func(t *testing.T) {
			m, p, job, resolver, base := costTrimBindingFixture(t)
			rt := &inventoryBindingRuntime{costTrimImageRuntime: base, mounted: mode == "mounted" || mode == "publish-failure"}
			drivers := &inventoryBindingDrivers{}
			m.containerLiquibase.Runtime, m.containerLiquibase.Drivers = rt, drivers
			// Separate simulated requests must not share the binder's mutable
			// projection mounts. Snapshot the second request before the first bind.
			secondRequest := p
			secondRequest.liquibaseMounts = append([]engineRuntime.Mount(nil), p.liquibaseMounts...)
			recorder := preparecost.NewRecorder(true, 1)
			ctx := preparecost.WithRecorder(t.Context(), recorder)
			first, cleanup, err := m.bindContainerLiquibase(ctx, job.JobID, p)
			costTrimBindingCleanup(t, cleanup)
			if err != nil {
				t.Fatal(err)
			}
			q := m.queue.(*queue.SQLiteStore)
			saved, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID)
			if err != nil || !found {
				t.Fatal("initial execution missing", err)
			}
			db, err := sql.Open("sqlite", filepath.Join(m.stateStoreRoot, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			denied := errors.New("inventory-driver-publication-denied")
			if mode == "publish-failure" {
				drivers.err = denied
			}
			if mode == "binding-failure" {
				if _, err := db.Exec(`CREATE TRIGGER inventory_second_binding_denied BEFORE UPDATE ON liquibase_job_bindings
WHEN NEW.job_id='inventory-second-job'
BEGIN SELECT RAISE(ABORT,'inventory_binding_denied'); END`); err != nil {
					t.Fatal(err)
				}
			}
			job.JobID = "inventory-second-job"
			if err := m.admitContainerLiquibase(t.Context(), job, secondRequest); err != nil {
				t.Fatal(err)
			}
			second, cleanup, err := m.bindContainerLiquibase(ctx, job.JobID, secondRequest)
			costTrimBindingCleanup(t, cleanup)
			if resolver.calls != 2 || !reflect.DeepEqual(base.actions, []string{"ensure", "inventory", "ensure"}) {
				t.Fatal("hit skipped per-job image work or repeated inventory", resolver.calls, base.actions)
			}
			if mode == "publish-failure" || mode == "binding-failure" {
				if (mode == "publish-failure" && !errors.Is(err, denied)) || (mode == "binding-failure" && !errors.Is(err, queue.ErrLiquibaseBinding)) {
					t.Fatal("did not reach the intended failure", err)
				}
				if _, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID); err != nil || found {
					t.Fatal("failed bind published an execution", found, err)
				}
				var tasks, signed int
				if err := db.QueryRow(`SELECT count(*) FROM prepare_tasks`).Scan(&tasks); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(`SELECT count(*) FROM prepare_jobs WHERE job_id=? AND signature IS NOT NULL`, job.JobID).Scan(&signed); err != nil {
					t.Fatal(err)
				}
				if tasks != 0 || signed != 0 {
					t.Fatal("binding failure manufactured execution progress", tasks, signed)
				}
			} else {
				if err != nil || first.liquibaseFingerprint != second.liquibaseFingerprint || first.liquibaseMounts[0].HostPath == second.liquibaseMounts[0].HostPath {
					t.Fatal("job-specific projection/fingerprint lost", err)
				}
				next, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID)
				if err != nil || !found || !reflect.DeepEqual(next, saved) {
					t.Fatal("hit changed immutable execution", err)
				}
				bytes, err := os.ReadFile(filepath.Join(second.liquibaseMounts[0].HostPath, "changelog.sql"))
				if err != nil || string(bytes) != "--liquibase formatted sql\n--changeset sqlrs:1\nSELECT 1;\n" {
					t.Fatal("hit skipped real input materialisation", err)
				}
			}
			if rt.mounted && (drivers.publishes != 2 || drivers.materializes != map[bool]int{true: 1, false: 2}[mode == "publish-failure"]) {
				t.Fatal("artifact work was cached with inventory", drivers.publishes, drivers.materializes)
			}
			previous, found, err := q.GetLiquibaseExecutionForJob(t.Context(), "cost-binding")
			if err != nil || !found || !reflect.DeepEqual(previous, saved) {
				t.Fatal("failed/new binding changed prior job", err)
			}
			report := recorder.Finish()
			counts := map[preparecost.Operation]int{}
			for _, span := range report.Spans {
				counts[span.Operation]++
			}
			if !report.Complete || counts[preparecost.InventoryLookup] != 2 || counts[preparecost.ImageInventory] != 1 {
				t.Fatal("actual lookup/probe boundaries differ", counts)
			}
			if mode == "binding-failure" {
				// Only the test trigger changes. The same real metadata writer and
				// exact binding must now succeed; SQL Liquibase is never invoked.
				if _, err := db.Exec(`DROP TRIGGER inventory_second_binding_denied`); err != nil {
					t.Fatal(err)
				}
				if err := q.BindLiquibaseExecution(t.Context(), job.JobID, saved); err != nil {
					t.Fatal("same-boundary control failed without trigger", err)
				}
				next, found, err := q.GetLiquibaseExecutionForJob(t.Context(), job.JobID)
				if err != nil || !found || !reflect.DeepEqual(next, saved) {
					t.Fatal("same-boundary control changed exact binding", err)
				}
				previous, found, err := q.GetLiquibaseExecutionForJob(t.Context(), "cost-binding")
				if err != nil || !found || !reflect.DeepEqual(previous, saved) {
					t.Fatal("control changed first job", err)
				}
			}
		})
	}
}

type inventoryBindingRuntime struct {
	*costTrimImageRuntime
	mounted bool
}

func (r *inventoryBindingRuntime) InspectRunnerImage(ctx context.Context, image engineRuntime.RunnerImageBinding) (engineRuntime.RunnerImageInspection, error) {
	return preparecost.Observe(ctx, preparecost.ImageInventory, func(child context.Context) (engineRuntime.RunnerImageInspection, error) {
		value, err := r.costTrimImageRuntime.InspectRunnerImage(child, image)
		if r.mounted {
			value.PostgreSQLJars = nil
		}
		return value, err
	})
}

type inventoryBindingDrivers struct {
	LiquibaseDriverStore
	publishes, materializes int
	err                     error
}

func (d *inventoryBindingDrivers) Publish(context.Context) (driverbundle.Binding, error) {
	d.publishes++
	return driverbundle.Binding{Mode: "mounted", Digest: driverbundle.DriverDigest, ArtifactRef: "sha256:" + strings.Repeat("a", 64)}, d.err
}
func (d *inventoryBindingDrivers) Materialize(_ context.Context, binding driverbundle.Binding, destination string) (*driverbundle.Materialized, error) {
	d.materializes++
	if binding.Mode != "mounted" || binding.Digest != driverbundle.DriverDigest || binding.ArtifactRef != "sha256:"+strings.Repeat("a", 64) {
		return nil, errors.New("driver identity changed")
	}
	if err := os.MkdirAll(destination, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(destination, driverbundle.DriverFileName)
	if err := os.WriteFile(path, []byte("capability fixture; native tests verify actual JDBC"), 0600); err != nil {
		return nil, err
	}
	return &driverbundle.Materialized{HostPath: path, ContainerPath: "/liquibase/lib/" + driverbundle.DriverFileName}, nil
}
