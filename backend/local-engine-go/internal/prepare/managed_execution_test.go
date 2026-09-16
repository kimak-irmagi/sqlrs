package prepare

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/managedstore"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
)

type managedFailureRuntime struct {
	fakeRuntime
	initErr    error
	inspectErr error
}

func (r *managedFailureRuntime) InspectManaged(_ context.Context, b managedidentity.RuntimeBinding) (engineRuntime.Instance, error) {
	return engineRuntime.Instance{ID: b.RuntimeRef, Binding: b, Host: "127.0.0.1", Port: 5432}, r.inspectErr
}

func (r *managedFailureRuntime) InitManagedBase(context.Context, engineRuntime.ManagedInitRequest) error {
	return r.initErr
}

func TestManagedPublicationReusesOnlyExactVerifiedInstance(t *testing.T) {
	for _, scenario := range []string{"retry", "state-conflict", "missing-access", "missing-secret", "missing-adapter", "inspect", "retired", "metadata", "empty-state"} {
		t.Run(scenario, func(t *testing.T) {
			m, p, db, _, rt := managedExecutionFixture(t)
			ctx := context.Background()
			stateID := strings.Repeat("d", 64)
			ref := managedOperationRef("instance", "job")[:32]
			if err := m.store.CreateState(ctx, store.StateCreate{StateID: stateID, StateFingerprint: stateID, ImageID: p.effectiveImageID(), PrepareKind: "psql", CreatedAt: "2026-09-16", LineageRef: p.managed.LineageRef, IdentityDigest: p.managed.IdentityDigest}); err != nil {
				t.Fatal(err)
			}
			b := managedidentity.RuntimeBinding{IdentityBinding: p.managed.IdentityBinding, RuntimeRef: "container", PhysicalIdentity: "physical"}
			if err := m.store.CreateInstance(ctx, store.InstanceCreate{InstanceID: ref, StateID: stateID, ImageID: p.effectiveImageID(), CreatedAt: "2026-09-16", RuntimeID: &b.RuntimeRef}); err != nil {
				t.Fatal(err)
			}
			if scenario != "missing-access" {
				if err := m.access.Activate(ctx, ref, b, func(instanceaccess.Secret) error { return nil }, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "state-conflict":
				stateID = strings.Repeat("e", 64)
			case "missing-adapter":
				m.runtime = &fakeRuntime{}
			case "missing-secret":
				binding, err := m.access.Lookup(ctx, ref)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(m.stateStoreRoot, "secrets", binding.SecretRef+".json")); err != nil {
					t.Fatal(err)
				}
			case "inspect":
				rt.inspectErr = instanceaccess.ErrUnavailable
			case "retired":
				if err := m.access.Retire(ctx, ref, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
			case "metadata":
				db.Close()
			case "empty-state":
				stateID = ""
			}
			result, resp := m.executor.createInstance(ctx, "job", p, stateID)
			if scenario == "retry" {
				if resp != nil || result == nil || result.InstanceID != ref || !strings.Contains(result.DSN, p.managed.Username) {
					t.Fatal("verified publication retry", resp)
				}
			} else if resp == nil || result != nil {
				t.Fatal("invalid publication admitted", scenario)
			}
		})
	}
}

func TestManagedActivationFailureDoesNotPublish(t *testing.T) {
	for _, scenario := range []string{"missing-bootstrap", "native-proof", "cleanup-fence", "cleanup-files", "retired-intent", "foreign-target", "missing-inspector", "lost-runtime", "missing-final-state"} {
		t.Run(scenario, func(t *testing.T) {
			m, p, _, _, runtime := managedExecutionFixture(t)
			ctx := context.Background()
			stateID := strings.Repeat("d", 64)
			ref := managedOperationRef("instance", "job")[:32]
			target := filepath.Join(m.stateStoreRoot, "jobs", "job", "operation")
			if scenario == "foreign-target" {
				target = filepath.Join(m.stateStoreRoot, "other-job", "operation")
			}
			op, err := m.access.Assign(ctx, "operation", target, stateID, p.managed.IdentityBinding)
			if err != nil {
				t.Fatal(err)
			}
			b := managedidentity.RuntimeBinding{IdentityBinding: p.managed.IdentityBinding, RuntimeRef: "container", PhysicalIdentity: op.PhysicalIdentity}
			if err := m.access.Attach(ctx, op, b); err != nil {
				t.Fatal(err)
			}
			runner := m.registerRunner("job", func() {})
			defer func() { close(runner.done); m.unregisterRunner("job") }()
			rt := &jobRuntime{stateID: stateID, operation: op, instance: engineRuntime.Instance{ID: b.RuntimeRef, Binding: b, Host: "invalid", Port: 1}, runtimeDir: target}
			if scenario != "missing-bootstrap" {
				if _, _, err := m.access.Bootstrap(ctx, p.managed.IdentityBinding, true); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "missing-final-state":
				// A lost final snapshot must fail before allocating instance access.
			case "retired-intent", "foreign-target", "missing-inspector", "lost-runtime":
				if err := m.access.Activate(ctx, ref, b, func(instanceaccess.Secret) error { return nil }, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
				if scenario == "retired-intent" {
					if err := m.access.Retire(ctx, ref, func() error { return nil }); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "missing-inspector" {
					m.runtime = &fakeRuntime{}
				}
				if scenario == "lost-runtime" {
					runtime.inspectErr = instanceaccess.ErrUnavailable
				}
			default:
				if scenario == "cleanup-fence" {
					rt.stateID = "other"
					rt.operation.Ref = "missing"
				}
				if scenario == "cleanup-files" {
					rt.stateID = "other"
					rt.cleanup = func() error { return errors.New("storage unavailable") }
				}
				runner.setRuntime(rt)
			}
			if result, resp := m.executor.createInstance(ctx, "job", p, stateID); result != nil || resp == nil {
				t.Fatal("failed activation published", scenario)
			}
			instances, err := m.store.ListInstances(ctx, store.InstanceFilters{})
			if err != nil || len(instances) != 0 {
				t.Fatal("public instance appeared", err)
			}
			if scenario == "missing-bootstrap" {
				if err := m.verifyManagedRuntime(ctx, rt); err == nil {
					t.Fatal("missing bootstrap verified")
				}
			}
		})
	}
}

type stateSizeFailureStore struct{ *fakeStore }

func (s stateSizeFailureStore) UpdateStateSize(context.Context, string, int64) error {
	return errors.New("metadata unavailable")
}

func TestCachedSizeBackfillFailureDoesNotInvalidateSnapshot(t *testing.T) {
	for _, scenario := range []string{"metadata", "missing", "root", "measurement", "write"} {
		t.Run(scenario, func(t *testing.T) {
			st := &fakeStore{statesByID: map[string]store.StateEntry{"state": {StateID: "state", ImageID: "postgres:17"}}}
			m := newManager(t, st)
			original := storeUsageFn
			defer func() { storeUsageFn = original }()
			storeUsageFn = func(string) (int64, error) { return 123, nil }
			switch scenario {
			case "metadata":
				st.getStateErr = errors.New("metadata unavailable")
			case "missing":
				st.statesByID = nil
			case "root":
				m.stateStoreRoot = ""
			case "measurement":
				storeUsageFn = func(string) (int64, error) { return 0, errors.New("storage unavailable") }
			case "write":
				m.store = stateSizeFailureStore{st}
			}
			m.executor.(*taskExecutor).backfillCachedStateSizeIfMissing(context.Background(), "job", "state")
			if len(st.deletedStates) != 0 || len(st.states) != 0 {
				t.Fatal("accounting failure changed snapshot ownership")
			}
		})
	}
}
func (r *managedFailureRuntime) StartManaged(_ context.Context, req engineRuntime.ManagedStartRequest) (engineRuntime.Instance, error) {
	if r.startErr != nil {
		return engineRuntime.Instance{}, r.startErr
	}
	b := managedidentity.RuntimeBinding{IdentityBinding: req.Identity, PhysicalIdentity: req.PhysicalIdentity, RuntimeRef: "container"}
	// An absent endpoint cannot satisfy native proof; successful publication is
	// covered by the actual PostgreSQL suite, never by an invented SQL success.
	return engineRuntime.Instance{ID: b.RuntimeRef, Binding: b, Host: "invalid", Port: 1}, nil
}

func managedExecutionFixture(t *testing.T) (*PrepareService, preparedRequest, *sql.DB, *fakeStateFS, *managedFailureRuntime) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
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
	t.Cleanup(func() { secrets.Close() })
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
	fs := &fakeStateFS{}
	rt := &managedFailureRuntime{}
	m := newManagerWithDeps(t, st, q, &testDeps{stateRoot: root, statefs: fs})
	m.access = access
	m.identity = owner
	m.runtime = rt
	p, err := m.prepareRequest(Request{PrepareKind: "psql", ImageID: "postgres@sha256:" + strings.Repeat("a", 64), PsqlArgs: []string{"-c", "SELECT 1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.bindManagedRequest(ctx, &p, nil); err != nil {
		t.Fatal(err)
	}
	return m, p, db, fs, rt
}

func TestManagedPrepareFailuresKeepPublicationClosed(t *testing.T) {
	for _, scenario := range []string{"cancelled", "nil-input", "invalid-binding", "unsupported-adapter", "invalid-kind", "wrong-base", "base-file", "partial-base", "init", "base-directory", "metadata", "secret", "base-commit", "clone", "target-exists", "target-parent", "start", "attach", "native-proof", "missing-state", "invalid-root"} {
		t.Run(scenario, func(t *testing.T) {
			m, p, db, fs, rt := managedExecutionFixture(t)
			ctx := context.Background()
			input := &TaskInput{Kind: "image", ID: p.managed.Key}
			paths, err := resolveStatePaths(m.managedRoot, p.effectiveImageID(), "", fs)
			if err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(paths.baseDir, p.managed.Key)
			opRef := "runtime_" + managedOperationRef(m.stateStoreRoot, "job", input.Kind, input.ID)
			target := filepath.Join(m.stateStoreRoot, "jobs", "job", opRef)
			switch scenario {
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "nil-input":
				input = nil
			case "invalid-binding":
				p.managed.IdentityDigest = "broken"
			case "unsupported-adapter":
				m.runtime = &fakeRuntime{}
			case "invalid-kind":
				input.Kind = "foreign"
			case "wrong-base":
				input.ID = "foreign"
			case "base-file":
				err = os.MkdirAll(filepath.Dir(base), 0700)
				if err == nil {
					err = os.WriteFile(base, []byte("preserve"), 0600)
				}
			case "partial-base":
				_, err = m.access.BaseOperation(ctx, p.managed.Key, base, p.managed.IdentityBinding, false)
				if err == nil {
					err = os.MkdirAll(base, 0700)
				}
			case "init":
				rt.initErr = instanceaccess.ErrUnavailable
			case "base-directory":
				fs.ensureBaseErr = errors.New("storage unavailable")
			case "metadata":
				err = db.Close()
			case "secret":
				var ref instanceaccess.SecretBinding
				ref, _, err = m.access.Bootstrap(ctx, p.managed.IdentityBinding, true)
				if err == nil {
					err = os.Remove(filepath.Join(m.stateStoreRoot, "secrets", ref.Ref()+".json"))
				}
			case "base-commit":
				_, err = db.Exec(`CREATE TRIGGER refuse BEFORE UPDATE OF stage ON managed_runtime_operations WHEN NEW.stage='ready' BEGIN SELECT RAISE(ABORT,'failure'); END`)
			case "clone":
				fs.cloneErr = errors.New("storage unavailable")
			case "target-exists":
				err = os.MkdirAll(target, 0700)
			case "target-parent":
				err = os.WriteFile(filepath.Join(m.stateStoreRoot, "jobs"), []byte("preserve"), 0600)
			case "start":
				rt.startErr = instanceaccess.ErrUnavailable
			case "attach":
				_, err = db.Exec(`CREATE TRIGGER refuse BEFORE UPDATE OF stage ON managed_runtime_operations WHEN NEW.stage='running' BEGIN SELECT RAISE(ABORT,'failure'); END`)
			case "missing-state":
				input.Kind = "state"
				input.ID = "missing"
			case "invalid-root":
				m.managedRoot = ""
			}
			if err != nil {
				t.Fatal(err)
			}
			if runtime, resp := m.executor.startRuntime(ctx, "job", p, input); resp == nil || runtime != nil {
				t.Fatal("invalid runtime admitted", scenario)
			}
			if scenario != "metadata" {
				instances, err := m.store.ListInstances(context.Background(), store.InstanceFilters{})
				if err != nil || len(instances) != 0 {
					t.Fatal("failed runtime published", err)
				}
			}
		})
	}
}
