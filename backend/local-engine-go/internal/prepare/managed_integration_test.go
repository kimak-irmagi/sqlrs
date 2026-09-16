//go:build managedintegration && !windows

package prepare

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sqlrs/engine-local/internal/dbms"
	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/managedstore"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/registry"
	managedRun "github.com/sqlrs/engine-local/internal/run"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/statefs"
	"github.com/sqlrs/engine-local/internal/store"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
)

type managedFixtureRunner struct{ t *testing.T }

func (r managedFixtureRunner) Run(ctx context.Context, name string, args []string, input *string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	password := ""
	if input != nil {
		cmd.Stdin = strings.NewReader(*input)
		password = strings.SplitN(*input, "\n", 2)[0]
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Logf("fixture command %s failed: %s", args[0], engineRuntime.RedactManagedOutput(string(out), password))
	}
	return string(out), err
}

// This suite exercises the production prepare path with an isolated store and
// official PostgreSQL 17. It requires Docker; no utility-only proof substitutes
// for successful recipe execution, immutable cache reuse and real credentials.
func TestManagedPreparePostgres17(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	root := t.TempDir()
	defer func() {
		// Only this fixture's Docker-owned data directories; secret root is never mounted.
		for _, name := range []string{"engines", "jobs"} {
			path := filepath.Join(root, name)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				continue
			}
			cmd := exec.Command("docker", "run", "--rm", "-v", path+":/cleanup", "postgres:17", "chown", "-R", strconv.Itoa(os.Getuid())+":"+strconv.Itoa(os.Getgid()), "/cleanup")
			if err := cmd.Run(); err != nil {
				t.Error("fixture data ownership cleanup", err)
			}
		}
	}()
	db, err := sql.Open("sqlite", filepath.Join(root, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	format, err := managedstore.Initialize(ctx, db, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	st, err := storesqlite.New(db)
	if err != nil {
		t.Fatal(err)
	}
	q, err := queue.New(db)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := instanceaccess.OpenSecrets(filepath.Join(root, "managed-secrets"))
	if err != nil {
		t.Fatal(err)
	}
	defer secrets.Close()
	access, err := instanceaccess.NewService(db, secrets, format.DomainRef)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := managedidentity.NewService(st)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := managedidentity.NewOwner(identities, format.DomainRef, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	rt := engineRuntime.NewDocker(engineRuntime.Options{ManagedSecrets: secrets, ProtectedRoot: filepath.Join(root, "managed-secrets"), Runner: managedFixtureRunner{t: t}})
	fs := statefs.NewManager(statefs.Options{Backend: "copy", StateStoreRoot: root})
	m, err := NewPrepareService(Options{Store: st, Queue: q, Runtime: rt, StateFS: fs, DBMS: dbms.NewPostgres(rt), StateStoreRoot: root, Identity: owner, Access: access})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		instances, _ := st.ListInstances(context.Background(), store.InstanceFilters{})
		for _, instance := range instances {
			if instance.RuntimeID != nil {
				_ = rt.Stop(context.Background(), *instance.RuntimeID)
			}
		}
	}()
	req := Request{PrepareKind: "psql", ImageID: "postgres:17", PsqlArgs: []string{"-v", "ON_ERROR_STOP=1", "-c", "CREATE ROLE postgres LOGIN; SET ROLE postgres; RESET ROLE; REASSIGN OWNED BY postgres TO CURRENT_USER; DROP OWNED BY postgres; DROP ROLE postgres; CREATE TABLE managed_acceptance(id integer); INSERT INTO managed_acceptance VALUES(17);"}}
	results := []Result{}
	var firstJob string
	for i := 0; i < 2; i++ {
		accepted, err := m.Submit(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstJob = accepted.JobID
		}
		status, ok := m.Get(accepted.JobID)
		if !ok || status.Status != StatusSucceeded || status.Result == nil {
			t.Fatalf("prepare failed: %+v", status.Error)
		}
		result := *status.Result
		results = append(results, result)
		config, err := pgconn.ParseConfig(result.DSN)
		if err != nil {
			t.Fatal("invalid DSN")
		}
		config.RequireAuth = "scram-sha-256"
		conn, err := pgconn.ConnectConfig(ctx, config)
		if err != nil {
			t.Fatal("published credential cannot connect")
		}
		rows, err := conn.Exec(ctx, "SELECT id FROM managed_acceptance").ReadAll()
		conn.Close(ctx)
		if err != nil || len(rows) != 1 || string(rows[0].Rows[0][0]) != "17" {
			t.Fatal("recipe data missing")
		}
		for _, table := range []string{"prepare_jobs", "prepare_events", "instance_access", "managed_runtime_operations"} {
			var leaked int
			column := "binding_json"
			if table == "prepare_jobs" || table == "prepare_events" {
				column = "result_json"
			}
			if table == "managed_runtime_operations" {
				column = "identity_json"
			}
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE instr("+column+",?)>0", config.Password).Scan(&leaked); err != nil || leaked != 0 {
				t.Fatal("credential leaked into control metadata", err)
			}
		}
	}
	if results[0].StateID != results[1].StateID || results[0].InstanceID == results[1].InstanceID {
		t.Fatal("cache/instance allocation mismatch")
	}
	first, _ := url.Parse(results[0].DSN)
	second, _ := url.Parse(results[1].DSN)
	a, _ := first.User.Password()
	b, _ := second.User.Password()
	if a == b || len(a) != 64 || first.User.Username() != second.User.Username() {
		t.Fatal("lineage or per-instance credentials mismatch")
	}
	// Recreate the durable crash window after native activation and before public
	// metadata commit. Recovery must reuse the exact running clone and password.
	if err := st.DeleteInstance(ctx, results[0].InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateJob(ctx, firstJob, queue.JobUpdate{Status: strPtr(StatusRunning)}); err != nil {
		t.Fatal(err)
	}
	reopenedAccess, err := instanceaccess.NewService(db, secrets, format.DomainRef)
	if err != nil {
		t.Fatal(err)
	}
	reopenedRuntime := engineRuntime.NewDocker(engineRuntime.Options{ManagedSecrets: secrets, ProtectedRoot: filepath.Join(root, "managed-secrets"), Runner: managedFixtureRunner{t: t}})
	reopened, err := NewPrepareService(Options{Store: st, Queue: q, Runtime: reopenedRuntime, StateFS: fs, DBMS: dbms.NewPostgres(reopenedRuntime), StateStoreRoot: root, Identity: owner, Access: reopenedAccess})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, _ := reopened.Get(firstJob)
	if recovered.Status != StatusSucceeded || recovered.Result == nil || recovered.Result.DSN != results[0].DSN {
		t.Fatalf("activation recovery changed access: %+v", recovered.Error)
	}
	runManager, err := managedRun.NewManager(managedRun.Options{Registry: registry.New(st), Runtime: reopenedRuntime, Access: reopenedAccess})
	if err != nil {
		t.Fatal(err)
	}
	runResult, err := runManager.Run(ctx, managedRun.Request{InstanceRef: results[0].InstanceID, Kind: "psql", Args: []string{"-At", "-c", "SELECT id FROM managed_acceptance"}})
	if err != nil || strings.TrimSpace(runResult.Stdout) != "17" {
		t.Fatal("authorized run after restart", err)
	}
	deleteManager, err := deletion.NewManager(deletion.Options{Store: st, Runtime: reopenedRuntime, Access: reopenedAccess, StateFS: fs, StateStoreRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := deleteManager.DeleteInstance(ctx, results[0].InstanceID, deletion.DeleteOptions{}); err != nil || !found {
		t.Fatal("managed deletion after restart", err)
	}
	if _, err := reopenedAccess.Lookup(ctx, results[0].InstanceID); err == nil {
		t.Fatal("retired access remained available")
	}
	job, found, err := q.GetJob(ctx, firstJob)
	if err != nil || !found {
		t.Fatal("missing durable job", err)
	}
	prepared, err := m.prepareFromJob(job)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := resolveStatePaths(root, prepared.effectiveImageID(), "", fs)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.RemovePath(ctx, filepath.Join(paths.baseDir, prepared.managed.Key)); err != nil {
		t.Fatal("physical base eviction", err)
	}
	evictionRequest := Request{PrepareKind: "psql", ImageID: "postgres:17", PsqlArgs: []string{"-v", "ON_ERROR_STOP=1", "-c", "CREATE ROLE sqlrs LOGIN; DROP ROLE sqlrs; SELECT 1;"}}
	evictionJob, err := m.Submit(ctx, evictionRequest)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, _ := m.Get(evictionJob.JobID)
	if rebuilt.Status != StatusSucceeded || rebuilt.Result == nil {
		t.Fatalf("base recreation failed: %+v", rebuilt.Error)
	}
	rebuiltURL, err := url.Parse(rebuilt.Result.DSN)
	if err != nil || rebuiltURL.User.Username() != first.User.Username() {
		t.Fatal("physical eviction changed managed identity")
	}
	req.PsqlArgs = []string{"-v", "ON_ERROR_STOP=1", "-c", "UPDATE pg_catalog.pg_authid SET rolsuper=false WHERE rolname=current_user;"}
	accepted, err := m.Submit(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	status, _ := m.Get(accepted.JobID)
	if status.Status != StatusFailed || status.Result != nil {
		t.Fatal("damaged role published")
	}
	workspace, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	// Cross-compiled WSL runs start in the Go module; normal go test starts in
	// the package. Locate the checked-in wrappers without changing their SQL.
	if _, err := os.Stat(filepath.Join(workspace, "examples")); err != nil {
		workspace, err = filepath.Abs("../../../..")
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, example := range []struct{ name, database, query, want string }{
		{"sakila", "sakila", "SELECT count(*) FROM film", "1000"},
		{"chinook", "chinook", "SELECT count(*) FROM track", "3503"},
		{"liquibase", "postgres", "SELECT count(*) > 0 FROM databasechangelog", "t"},
	} {
		t.Run(example.name, func(t *testing.T) {
			request := Request{PrepareKind: "psql", ImageID: "postgres:17", PsqlArgs: []string{"-q", "-v", "ON_ERROR_STOP=1", "-f", filepath.Join(workspace, "examples", example.name, "prepare.sql")}}
			if example.name == "liquibase" {
				base := filepath.Join(workspace, "examples", "liquibase", "jhipster-sample-app")
				request = Request{PrepareKind: "lb", ImageID: "postgres:17", LiquibaseArgs: []string{"update", "--searchPath", base, "--changelog-file", filepath.Join(base, "config", "liquibase", "master.xml")}, LiquibaseExec: "/usr/local/bin/liquibase", LiquibaseExecMode: "native", WorkDir: base}
			}
			var state string
			for attempt := 0; attempt < 2; attempt++ {
				accepted, err := m.Submit(ctx, request)
				if err != nil {
					t.Fatal(err)
				}
				status, ok := m.Get(accepted.JobID)
				if !ok || status.Status != StatusSucceeded || status.Result == nil {
					t.Fatalf("example failed: %+v", status.Error)
				}
				if attempt > 0 && state != status.Result.StateID {
					t.Fatal("example cache changed")
				}
				state = status.Result.StateID
				cfg, err := pgconn.ParseConfig(status.Result.DSN)
				if err != nil {
					t.Fatal("invalid example DSN")
				}
				cfg.Database = example.database
				cfg.RequireAuth = "scram-sha-256"
				conn, err := pgconn.ConnectConfig(ctx, cfg)
				if err != nil {
					t.Fatal("example access failed")
				}
				rows, err := conn.Exec(ctx, example.query).ReadAll()
				conn.Close(ctx)
				if err != nil || len(rows) != 1 || string(rows[0].Rows[0][0]) != example.want {
					t.Fatal("example dataset mismatch")
				}
			}
		})
	}
}
