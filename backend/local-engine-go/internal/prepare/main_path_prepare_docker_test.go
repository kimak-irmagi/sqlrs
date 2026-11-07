package prepare

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sqlrs/engine-local/internal/dbms"
	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/managedstore"
	"github.com/sqlrs/engine-local/internal/prepare/driverbundle"
	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/sqliteconn"
	"github.com/sqlrs/engine-local/internal/statefs"
	storepkg "github.com/sqlrs/engine-local/internal/store"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
)

// mainPathNativeRuntime records exact allocations for the fallback only.
// Product cleanup is inspected independently before that fallback executes.
type mainPathNativeRuntime struct {
	*engineRuntime.DockerRuntime
	db                 *sql.DB
	allocated          []string
	runners, databases []string
	beforeInvoke       func(context.Context, engineRuntime.StepCaptureIdentity)
}

func (r *mainPathNativeRuntime) CreateContainer(ctx context.Context, request engineRuntime.RunRequest) (string, error) {
	id, err := r.DockerRuntime.CreateContainer(ctx, request)
	if id != "" {
		r.allocated = append(r.allocated, id)
	}
	return id, err
}

func (r *mainPathNativeRuntime) StartManaged(ctx context.Context, request engineRuntime.ManagedStartRequest) (engineRuntime.Instance, error) {
	instance, err := r.DockerRuntime.StartManaged(ctx, request)
	if instance.ID != "" {
		r.allocated = append(r.allocated, instance.ID)
	}
	return instance, err
}

func (r *mainPathNativeRuntime) InvokeTrackedLiquibaseStep(ctx context.Context, name, dir string, identity engineRuntime.StepCaptureIdentity, args []string, limit int64) (engineRuntime.CompleteStepCapture, error) {
	var id string
	if err := r.db.QueryRowContext(ctx, `SELECT c.db_runtime_id FROM prepare_clones c JOIN prepare_liquibase_sessions s ON s.clone_id=c.clone_id WHERE s.session_id=?`, identity.SessionID).Scan(&id); err != nil {
		return engineRuntime.CompleteStepCapture{}, err
	}
	r.runners = append(r.runners, identity.RunnerID)
	r.databases = append(r.databases, id)
	if len(r.runners) == 3 {
		var slots, old int
		if err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_liquibase_capture_slots WHERE session_id=?", identity.SessionID).Scan(&slots); err != nil {
			return engineRuntime.CompleteStepCapture{}, err
		}
		if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_capture_slots WHERE session_id=? AND task_id='execute-0'`, identity.SessionID).Scan(&old); err != nil {
			return engineRuntime.CompleteStepCapture{}, err
		}
		if slots != 2 || old != 0 {
			return engineRuntime.CompleteStepCapture{}, fmt.Errorf("third invocation did not reuse bounded capture slots")
		}
	}
	if r.beforeInvoke != nil {
		r.beforeInvoke(ctx, identity)
	}
	return r.DockerRuntime.InvokeTrackedLiquibaseStep(ctx, name, dir, identity, args, limit)
}

// MP05/MP06/MP07: a genuine complete production service, not an isolated
// coordinator or SQL writer. Explicit opt-in must fail, never skip, if broken.
type mainPathNativeFixture struct {
	ctx    context.Context
	root   string
	rt     *mainPathNativeRuntime
	m      *PrepareService
	st     *storesqlite.Store
	q      *queue.SQLiteStore
	fs     statefs.StateFS
	access *instanceaccess.Service
	req    Request
}

func newMainPathNativeFixture(t *testing.T) mainPathNativeFixture {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("select native Docker suite explicitly")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	t.Cleanup(cancel)
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	const changes = "--liquibase formatted sql\n--changeset sqlrs:1\nCREATE TABLE sqlrs_reuse_probe(id integer PRIMARY KEY);\n--changeset sqlrs:2\nINSERT INTO sqlrs_reuse_probe VALUES(1);\n--changeset sqlrs:3\nINSERT INTO sqlrs_reuse_probe VALUES(2);\n"
	if err := os.WriteFile(filepath.Join(project, "changelog.sql"), []byte(changes), 0600); err != nil {
		t.Fatal(err)
	}
	jar, err := os.ReadFile(os.Getenv("SQLRS_JDBC_JAR_PATH"))
	if err != nil {
		t.Fatal("verified JDBC fixture is required", err)
	}
	executable := filepath.Join(root, "engine", "sqlrs-engine")
	if err := os.MkdirAll(filepath.Join(root, "engine", "lib"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "engine", "lib", driverbundle.DriverFileName), jar, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sqliteconn.Open(filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
	secrets, err := instanceaccess.OpenSecrets(filepath.Join(root, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secrets.Close() })
	access, err := instanceaccess.NewServiceWithPublicationPins(db, secrets, format.DomainRef, q)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := managedidentity.NewService(st)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := managedidentity.NewOwner(ids, format.DomainRef, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	rt := &mainPathNativeRuntime{DockerRuntime: engineRuntime.NewDocker(engineRuntime.Options{ManagedSecrets: secrets, ProtectedRoot: filepath.Join(root, "secrets")}), db: db}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		for _, id := range rt.allocated {
			if len(id) != 64 {
				t.Error("invalid fallback ID")
				continue
			}
			if err := exec.CommandContext(cleanup, "docker", "inspect", id).Run(); err == nil {
				// Managed shutdown also restores host access to copied PGDATA.
				// A bare docker rm would strand postgres-owned test directories
				// when this suite runs as an ordinary Linux user.
				if err := rt.Stop(cleanup, id); err != nil {
					t.Error("owned fallback failed", err)
				}
			}
		}
	})
	fs := statefs.NewManager(statefs.Options{Backend: "copy", StateStoreRoot: root})
	m, err := NewPrepareService(Options{Store: st, Queue: q, Runtime: rt, StateFS: fs, DBMS: dbms.NewPostgres(rt), StateStoreRoot: root, Identity: owner, Access: access,
		ContainerLiquibase: &LiquibaseContainerDeps{Images: engineRuntime.NewRunnerImageResolver(nil), Runtime: rt, Sources: inputbundle.Store{Root: filepath.Join(root, "bundles")}, Drivers: driverbundle.New(filepath.Join(root, "drivers"), executable), MaterializedRoot: filepath.Join(root, "materialized")}})
	if err != nil {
		t.Fatal(err)
	}
	req := Request{PrepareKind: "lb", ImageID: "postgres:17", LiquibaseVersion: "5.0.4", WorkDir: project, LiquibaseArgs: []string{"--changelog-file", "changelog.sql", "update"}}
	return mainPathNativeFixture{ctx: ctx, root: root, rt: rt, m: m, st: st, q: q, fs: fs, access: access, req: req}
}

func TestMainPathLiquibasePrepareDocker(t *testing.T) {
	f := newMainPathNativeFixture(t)
	ctx, root, rt, m, st, q, fs, access, req := f.ctx, f.root, f.rt, f.m, f.st, f.q, f.fs, f.access, f.req

	accepted, err := m.Submit(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	status, found := m.Get(accepted.JobID)
	if !found || status.Status != StatusSucceeded || status.Result == nil {
		t.Fatalf("production prepare incomplete: status=%s error=%+v", status.Status, status.Error)
	}
	mainPathAssertRows(t, ctx, status.Result.DSN, 3, 2)
	if len(rt.runners) != 3 || len(rt.databases) != 3 {
		t.Fatalf("wrong native counts: runners=%d databases=%d", len(rt.runners), len(rt.databases))
	}
	for i := range rt.runners {
		if rt.runners[i] != rt.runners[0] || rt.databases[i] != rt.databases[0] {
			t.Fatal("normal workflow replaced a container")
		}
	}
	// Every exact temporary allocation must already be absent; only published
	// user instances are allowed to survive before the fallback cleanup.
	instances, err := st.ListInstances(ctx, storepkg.InstanceFilters{})
	if err != nil || len(instances) != 1 || instances[0].RuntimeID == nil {
		t.Fatal("published instance unavailable", err)
	}
	for _, id := range rt.allocated {
		if id != *instances[0].RuntimeID {
			if err := rt.VerifyContainerAbsent(ctx, id); err != nil {
				t.Fatal("temporary container leaked before fallback", id, err)
			}
		}
	}
	second, err := m.Submit(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	hit, found := m.Get(second.JobID)
	if !found || hit.Status != StatusSucceeded || hit.Result == nil || len(rt.runners) != 3 {
		t.Fatalf("exact cache hit failed: %+v", hit.Error)
	}
	mainPathAssertRows(t, ctx, hit.Result.DSN, 3, 2)
	planRequest := req
	planRequest.PlanOnly = true
	planned, err := m.Submit(ctx, planRequest)
	if err != nil {
		t.Fatal(err)
	}
	planStatus, found := m.Get(planned.JobID)
	storedTasks, taskErr := q.ListTasks(ctx, planned.JobID)
	if !found || planStatus.Status != StatusSucceeded || len(planStatus.Tasks) < 3 || len(storedTasks) != 1 || storedTasks[0].Type != "plan" || taskErr != nil || len(rt.runners) != 3 {
		t.Fatal("plan projection must not store or execute predicted SQL", planStatus.Error, taskErr)
	}
	explanation, err := m.CacheExplain(ctx, req)
	if err != nil || explanation.Accepted == nil {
		t.Fatal("cache explain was not admitted", err)
	}
	explained, found := m.Get(explanation.Accepted.JobID)
	if !found || explained.Status != StatusSucceeded || explained.Purpose != "cache_explain" || explained.Result != nil || explained.CacheExplainResult == nil || explained.CacheExplainResult.Decision != "hit" || explained.CacheExplainResult.MatchedStateID != status.Result.StateID || len(rt.runners) != 3 {
		t.Fatal("saved cache explanation is not the exact hit", explained.Error)
	}
	for _, event := range mustMainPathEvents(t, m, explanation.Accepted.JobID) {
		if event.Result != nil {
			t.Fatal("cache decision projected as a prepare result")
		}
	}
	// Restore every retained prefix independently and inspect real data. This
	// cannot be satisfied by reporting cached=true on an empty directory.
	job, ok, err := q.GetJob(ctx, accepted.JobID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	prepared, err := m.prepareFromJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for prefix, task := range status.Tasks {
		if task.Type != "state_execute" || task.OutputStateID == status.Result.StateID {
			continue
		}
		copy, problem := (&taskExecutor{m: m}).startManagedRuntime(ctx, fmt.Sprintf("%s-prefix-%d", accepted.JobID, prefix), prepared, &TaskInput{Kind: "state", ID: task.OutputStateID})
		if problem != nil {
			t.Fatal("checkpoint restore failed", problem.Code)
		}
		connection, err := m.managedConnection(ctx, copy)
		if err != nil {
			t.Fatal(err)
		}
		index := 0
		if task.TaskID == "execute-1" {
			index = 1
		}
		mainPathAssertRows(t, ctx, managedDSN(copy.instance, connection.Password), index+1, index)
		temporary := &jobRunner{}
		temporary.setRuntime(copy)
		if err := m.cleanupRuntime(ctx, temporary); err != nil {
			t.Fatal(err)
		}
	}
	// Use the public deletion workflow to remove only the final snapshot.
	// The original recipe/bundle stays identical, so exactly SQL3 must run.
	deletes, err := deletion.NewManager(deletion.Options{Store: st, Runtime: rt, StateFS: fs, Access: access, StateStoreRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range []string{status.Result.InstanceID, hit.Result.InstanceID} {
		result, found, err := deletes.DeleteInstance(ctx, instance, deletion.DeleteOptions{})
		if err != nil || !found || result.Outcome != deletion.OutcomeDeleted {
			t.Fatal("fixture public instance deletion failed", err)
		}
	}
	removed, found, err := deletes.DeleteState(ctx, status.Result.StateID, deletion.DeleteOptions{})
	if err != nil || !found || removed.Outcome != deletion.OutcomeDeleted {
		t.Fatal("fixture public final-state deletion failed", err)
	}
	partial, err := m.Submit(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	resumed, found := m.Get(partial.JobID)
	if !found || resumed.Status != StatusSucceeded || resumed.Result == nil || len(rt.runners) != 4 {
		t.Fatal("cached prefix did not execute exactly one remaining change", resumed.Error)
	}
	mainPathAssertRows(t, ctx, resumed.Result.DSN, 3, 2)
	deleted, found, err := deletes.DeleteInstance(ctx, resumed.Result.InstanceID, deletion.DeleteOptions{})
	if err != nil || !found || deleted.Outcome != deletion.OutcomeDeleted {
		t.Fatal("published instance disposal failed", err)
	}
}

func TestMainPathLiquibaseKnownFailureDocker(t *testing.T) {
	f := newMainPathNativeFixture(t)
	ctx, root, rt, m, st, req := f.ctx, f.root, f.rt, f.m, f.st, f.req

	badProject := filepath.Join(root, "known-failure")
	if err := os.Mkdir(badProject, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badProject, "changelog.sql"), []byte("--liquibase formatted sql\n--changeset sqlrs:bad\nINSERT INTO absent_sqlrs_table VALUES(1);\n"), 0600); err != nil {
		t.Fatal(err)
	}
	badRequest := req
	badRequest.WorkDir = badProject
	failed, err := m.Submit(ctx, badRequest)
	if err != nil {
		t.Fatal(err)
	}
	failedStatus, found := m.Get(failed.JobID)
	if !found || failedStatus.Status != StatusFailed || failedStatus.Result != nil || failedStatus.Error == nil {
		t.Fatal("known unsuccessful child must finish failed", failedStatus.Error)
	}
	active, err := st.ListInstances(ctx, storepkg.InstanceFilters{})
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string]bool{}
	for _, entry := range active {
		if entry.RuntimeID != nil {
			kept[*entry.RuntimeID] = true
		}
	}
	for _, id := range rt.allocated {
		if !kept[id] {
			if err := rt.VerifyContainerAbsent(ctx, id); err != nil {
				t.Fatal("known failure leaked temporary runtime", id)
			}
		}
	}
}

func TestMainPathLiquibaseCancellationDocker(t *testing.T) {
	f := newMainPathNativeFixture(t)
	ctx, root, rt, m, req := f.ctx, f.root, f.rt, f.m, f.req
	kept := map[string]bool{}

	cancelProject := filepath.Join(root, "accepted-cancel")
	if err := os.Mkdir(cancelProject, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cancelProject, "changelog.sql"), []byte("--liquibase formatted sql\n--changeset sqlrs:cancel\nCREATE TABLE must_not_run(id int);\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cancelRequest := req
	cancelRequest.WorkDir = cancelProject
	rt.beforeInvoke = func(_ context.Context, id engineRuntime.StepCaptureIdentity) {
		_, found, accepted, err := m.Cancel(id.JobID)
		if err != nil || !found || !accepted {
			t.Error("exact invocation cancellation was not accepted", err)
		}
	}
	cancelled, err := m.Submit(ctx, cancelRequest)
	if err != nil {
		t.Fatal(err)
	}
	rt.beforeInvoke = nil
	cancelStatus, found := m.Get(cancelled.JobID)
	if !found || cancelStatus.Status != StatusFailed || cancelStatus.Error == nil || cancelStatus.Error.Code != "cancelled" {
		t.Fatal("accepted cancellation did not finish without SQL", cancelStatus.Error)
	}
	for _, id := range rt.allocated {
		if !kept[id] {
			if err := rt.VerifyContainerAbsent(ctx, id); err != nil {
				t.Fatal("accepted cancellation leaked a temporary container", id)
			}
		}
	}
}

func mustMainPathEvents(t *testing.T, m *PrepareService, job string) []Event {
	t.Helper()
	events, found, done, err := m.EventsSince(job, 0)
	if err != nil || !found || !done {
		t.Fatal("completed event stream unavailable", err)
	}
	return events
}

func mainPathAssertRows(t *testing.T, ctx context.Context, dsn string, changes, rows int) {
	t.Helper()
	conn, err := pgconn.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("published connection unavailable", err)
	}
	defer conn.Close(context.Background())
	result, err := conn.Exec(ctx, "SELECT count(*) FROM databasechangelog; SELECT count(*) FROM sqlrs_reuse_probe;").ReadAll()
	if err != nil || len(result) != 2 || len(result[0].Rows) != 1 || len(result[1].Rows) != 1 || string(result[0].Rows[0][0]) != fmt.Sprint(changes) || string(result[1].Rows[0][0]) != fmt.Sprint(rows) {
		t.Fatal("actual database contents differ", err)
	}
}
