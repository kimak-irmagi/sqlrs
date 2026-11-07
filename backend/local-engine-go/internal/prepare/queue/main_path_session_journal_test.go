package queue_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/managedstore"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/statefs"
	enginestore "github.com/sqlrs/engine-local/internal/store"
)

const mainPathJournalUTC = "2026-11-07T12:00:00Z"

// mainPathJournalReady is a metadata-test observation, not a Docker claim.
// The actual writer must call it with the exact saved runner outside SQL.
type mainPathJournalReady struct {
	db   *sql.DB
	want queue.LiquibaseSessionRecord
}

func (v mainPathJournalReady) VerifyLiquibaseSessionReady(ctx context.Context, session queue.LiquibaseSessionRecord) (string, int64, error) {
	if !reflect.DeepEqual(session, v.want) {
		return "", 0, queue.ErrCommonAttempt
	}
	var n int
	if err := v.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_liquibase_sessions WHERE session_id=? AND runner_id=?", session.SessionID, session.RunnerID).Scan(&n); err != nil {
		return "", 0, err
	}
	if n != 1 {
		return "", 0, queue.ErrCommonAttempt
	}
	return "sha256:" + strings.Repeat("7", 64), 100, nil
}

// mainPathSessionJournal uses the actual coordinated initializer and normal
// admission/session/readiness writers. No prototype migration or disabled guard
// participates. MP01/MP02, issue89-main-path-tests and normal installation design.
func mainPathSessionJournal(t *testing.T) (*sql.DB, *queue.SQLiteStore, queue.LiquibaseSessionRecord, string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	format, err := managedstore.Initialize(t.Context(), db, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	q, err := queue.New(db)
	if err != nil {
		t.Fatal(err)
	}
	id, err := managedidentity.Generate(bytes.NewReader(bytes.Repeat([]byte{0x35}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	digest := func(char string) string { return "sha256:" + strings.Repeat(char, 64) }
	image := digest("1")
	if _, err := db.Exec("INSERT INTO managed_base_lineages VALUES(?,?,?,?,?,?,?,?,?)", format.DomainRef, id.LineageRef, "postgres", image, strings.Repeat("b", 64), id.PolicyVersion, id.Username, id.IdentityDigest, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	request := `{"prepare_kind":"lb","liquibase_version":"5.0.3","liquibase_args":["update","--changelog-file","/sqlrs/mnt/path1"],"_sqlrs_container_admission":{"normalized_args":["update","--changelog-file","/sqlrs/mnt/path1"],"mounts":["/sqlrs/mnt/path1"],"container_work_dir":"/sqlrs/mnt"}}`
	job := queue.JobRecord{JobID: "main-path-job", PrepareKind: "lb", Status: "queued", ImageID: "postgres:17", ResolvedImageID: image, LineageRef: id.LineageRef, IdentityDigest: id.IdentityDigest, CreatedAt: mainPathJournalUTC, RequestJSON: &request}
	record := queue.LiquibaseExecutionRecord{ExecutionFingerprint: digest("a"), DeclarationDigest: digest("b"), RequestedTag: "5.0.3", TargetIdentityDigest: digest("c"), PlatformOS: "linux", PlatformArch: "amd64", ImageManifestDigest: digest("d"), DriverMode: "embedded", DriverDigest: digest("e"), BundleManifestDigest: digest("f"), EffectiveConfigDigest: digest("0"), PolicyRevision: "lb-container-v2", CreatedAt: mainPathJournalUTC}
	if err := q.AdmitLiquibaseJob(t.Context(), job, record.RequestedTag, record.BundleManifestDigest); err != nil {
		t.Fatal(err)
	}
	if err := q.BindLiquibaseExecution(t.Context(), job.JobID, record); err != nil {
		t.Fatal(err)
	}
	inputKind := "image"
	stateKind, stateA, stateB, stateC := "state", strings.Repeat("2", 64), strings.Repeat("3", 64), strings.Repeat("4", 64)
	if err := q.ReplaceTasks(t.Context(), job.JobID, []queue.TaskRecord{
		{JobID: job.JobID, TaskID: "execute", Position: 0, Type: "state_execute", Status: "running", InputKind: &inputKind, InputID: &image, OutputStateID: &stateA},
		{JobID: job.JobID, TaskID: "execute-next", Position: 1, Type: "state_execute", Status: "queued", InputKind: &stateKind, InputID: &stateA, OutputStateID: &stateB},
		{JobID: job.JobID, TaskID: "execute-third", Position: 2, Type: "state_execute", Status: "queued", InputKind: &stateKind, InputID: &stateB, OutputStateID: &stateC},
	}); err != nil {
		t.Fatal(err)
	}
	running := "running"
	if err := q.UpdateJob(t.Context(), job.JobID, queue.JobUpdate{Status: &running}); err != nil {
		t.Fatal(err)
	}
	execution := digest("9")
	if err := q.BindStep(t.Context(), job.JobID, "execute", queue.ExecutionBinding{Digest: execution, PrepareKind: "lb", CommandDigest: execution, InputManifestDigest: record.BundleManifestDigest, ExecutorImageDigest: record.ImageManifestDigest, TargetIdentityDigest: record.TargetIdentityDigest, PolicyRevision: "lb-container-v2", EffectScope: "unknown", AdapterBindingDigest: record.ExecutionFingerprint}, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	session := queue.LiquibaseSessionRecord{SessionID: "main-path-session", JobID: job.JobID, CloneID: "main-path-clone", SessionNo: 1, WorkerGeneration: 1, ExecutionFingerprint: record.ExecutionFingerprint, StartKind: "image", StartID: image, PlannedRunnerName: "sqlrs-lb-main-path", RunnerConfigDigest: digest("8"), CaptureHelperDigest: digest("6"), ProtectedConfigRef: "main-path-config", CreatedAt: mainPathJournalUTC}
	session, err = q.PlanLiquibaseSession(t.Context(), queue.LiquibaseSessionLaunch{Session: session, Clone: queue.ClonePlan{ID: session.CloneID, Ref: "main-path-copy", PlannedRuntimeName: "sqlrs-db-main-path", Generation: 1, SourceKind: "image", SourceImageID: image}, FirstTaskID: "execute", ExecutionDigest: execution, HelperLeaseID: "main-path-helper"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.ActivateLiquibaseSessionClone(t.Context(), session, "sqlrs-db-main-path", strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	if err := q.RequestLiquibaseSessionRunnerCreate(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	session.RunnerID = strings.Repeat("a", 64)
	if err := q.AttachLiquibaseSessionRunner(t.Context(), session, session.RunnerID); err != nil {
		t.Fatal(err)
	}
	if err := q.RequestLiquibaseSessionRunnerStart(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkLiquibaseSessionRunnerRunning(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	session.RunnerPhase = "running"
	if _, err := q.RecordLiquibaseSessionReadiness(t.Context(), session, mainPathJournalReady{db: db, want: session}, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	return db, q, session, execution
}

func mainPathInvocationPlan(name string) queue.LiquibaseInvocationPlan {
	return queue.LiquibaseInvocationPlan{InvocationRef: name, StdoutRef: name + "-stdout", StderrRef: name + "-stderr", ManifestRef: name + "-manifest", ProtocolRevision: "lb-step-capture-v2", CreatedAt: mainPathJournalUTC}
}

// mainPathPublishStep advances the real SQL publication writer. The marker and
// DB/child observations are explicit fixture data, not native execution evidence.
func mainPathPublishStep(t *testing.T, db *sql.DB, q *queue.SQLiteStore, session queue.LiquibaseSessionRecord, invocation queue.LiquibaseInvocationRecord, state string) queue.LiquibaseSessionRecord {
	t.Helper()
	if err := q.RequestLiquibaseSessionExec(t.Context(), session, invocation, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	complete, err := q.CompleteLiquibaseSessionInvocation(t.Context(), session, invocation, queue.LiquibaseInvocationCompletion{ExitCode: 0, ManifestDigest: "sha256:" + strings.Repeat("e", 64), ManifestBytes: 500}, mainPathJournalUTC)
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := q.RecordLiquibaseSessionDBStopped(t.Context(), session, invocation, "sqlrs-db-main-path", strings.Repeat("d", 64), mainPathJournalUTC)
	if err != nil {
		t.Fatal(err)
	}
	job, found, err := q.GetJob(t.Context(), session.JobID)
	if err != nil || !found {
		t.Fatal("job binding", err)
	}
	entry := enginestore.StateCreate{StateID: state, StateFingerprint: state, ImageID: job.ResolvedImageID, PrepareKind: "lb", PrepareArgsNormalized: "fixture update-count", CreatedAt: mainPathJournalUTC, LineageRef: job.LineageRef, IdentityDigest: job.IdentityDigest, LiquibaseExecutionFingerprint: session.ExecutionFingerprint}
	if session.CursorKind == "state" {
		parent := session.CursorID
		entry.ParentStateID = &parent
	}
	dir := filepath.Join(t.TempDir(), state)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statefs.PublishedMarkerPath(dir, "copy"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := q.PublishLiquibaseSessionState(t.Context(), session, invocation, entry, dir, "copy", complete, quiet, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	session, found, err = q.ReadLiquibaseSession(t.Context(), session.SessionID)
	if err != nil || !found {
		t.Fatal(err)
	}
	if session.CursorID != state {
		t.Fatal("publication did not move exact cursor")
	}
	if err := q.RequestLiquibaseSessionDBRestart(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkLiquibaseSessionDBReady(t.Context(), session, "sqlrs-db-main-path", strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	session.DBPhase = "running"
	return session
}

func mainPathNextInvocation(t *testing.T, q *queue.SQLiteStore, session queue.LiquibaseSessionRecord, execution, task, name string) queue.LiquibaseInvocationRecord {
	t.Helper()
	running := "running"
	if err := q.UpdateTask(t.Context(), session.JobID, task, queue.TaskUpdate{Status: &running}); err != nil {
		t.Fatal(err)
	}
	record, found, err := q.GetLiquibaseExecutionForJob(t.Context(), session.JobID)
	if err != nil || !found {
		t.Fatal(err)
	}
	if err := q.BindStep(t.Context(), session.JobID, task, queue.ExecutionBinding{Digest: execution, PrepareKind: "lb", CommandDigest: execution, InputManifestDigest: record.BundleManifestDigest, ExecutorImageDigest: record.ImageManifestDigest, TargetIdentityDigest: record.TargetIdentityDigest, PolicyRevision: "lb-container-v2", EffectScope: "unknown", AdapterBindingDigest: record.ExecutionFingerprint}, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	invocation, err := q.PlanLiquibaseSessionInvocation(t.Context(), session, task, execution, mainPathInvocationPlan(name))
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}

func mainPathJournalRows(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	result := map[string]string{}
	for _, name := range names {
		rs, err := db.Query(`SELECT * FROM "` + strings.ReplaceAll(name, `"`, `""`) + `"`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rs.Columns()
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		for rs.Next() {
			data := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range dest {
				dest[i] = &data[i]
			}
			if err := rs.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			values = append(values, string(body))
		}
		if err := rs.Err(); err != nil {
			t.Fatal(err)
		}
		rs.Close()
		sort.Strings(values)
		result[name] = strings.Join(values, "\n")
	}
	return result
}

type mainPathPruneProbe struct {
	db    *sql.DB
	want  queue.CapturePrunePlan
	calls int
	err   error
}

func (v *mainPathPruneProbe) VerifyLiquibaseCapturePruned(ctx context.Context, plan queue.CapturePrunePlan) error {
	v.calls++
	if !reflect.DeepEqual(plan, v.want) {
		return queue.ErrCommonAttempt
	}
	var n int
	if err := v.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_liquibase_capture_slots WHERE session_id=?", plan.SessionID).Scan(&n); err != nil {
		return err
	}
	if n != 2 {
		return queue.ErrCommonAttempt
	}
	return v.err
}

// MP01/MP02/MP04: real publication A, exact reservation B, effect-free reading,
// refusal without changes, atomic confirmation, immutable history and room for C.
func TestMainPathCapturePruneConfirmation(t *testing.T) {
	db, q, session, execution := mainPathSessionJournal(t)
	a, err := q.PlanLiquibaseSessionInvocation(t.Context(), session, "execute", execution, mainPathInvocationPlan("main-first"))
	if err != nil {
		t.Fatal(err)
	}
	session = mainPathPublishStep(t, db, q, session, a, strings.Repeat("2", 64))
	b := mainPathNextInvocation(t, q, session, execution, "execute-next", "main-second")
	before := mainPathJournalRows(t, db)
	plan, err := q.ReadLiquibaseCapturePrunePlan(t.Context(), session, b)
	if err != nil || plan == nil {
		t.Fatal("published predecessor unreadable", err)
	}
	if plan.SessionID != "main-path-session" || plan.RunnerID != strings.Repeat("a", 64) || plan.WorkerGeneration != 1 || plan.PriorInvocationRef != "main-first" || plan.NextInvocationRef != "main-second" || plan.PriorManifestDigest != "sha256:"+strings.Repeat("e", 64) || plan.RetirementRef != *b.PriorRetirementRef {
		t.Fatalf("wrong saved authority: %+v", plan)
	}
	if !reflect.DeepEqual(before, mainPathJournalRows(t, db)) {
		t.Fatal("read changed journal")
	}
	denied := errors.New("probe denied")
	probe := &mainPathPruneProbe{db: db, want: *plan, err: denied}
	if err := q.ConfirmLiquibaseCapturePruned(t.Context(), session, b, probe, mainPathJournalUTC); !errors.Is(err, denied) || probe.calls != 1 {
		t.Fatal("probe refusal boundary", err)
	}
	if !reflect.DeepEqual(before, mainPathJournalRows(t, db)) {
		t.Fatal("refused observation changed rows")
	}
	probe.err = nil
	if err := q.ConfirmLiquibaseCapturePruned(t.Context(), session, b, probe, mainPathJournalUTC); err != nil {
		t.Fatal(err)
	}
	after := mainPathJournalRows(t, db)
	for table, rows := range before {
		if table != "prepare_liquibase_capture_slots" && table != "prepare_liquibase_capture_retirements" && after[table] != rows {
			t.Fatalf("confirmation changed unrelated table %s", table)
		}
	}
	if err := q.ConfirmLiquibaseCapturePruned(t.Context(), session, b, nil, "2026-11-07T12:00:01Z"); err != nil {
		t.Fatal("repeat confirmation", err)
	}
	if !reflect.DeepEqual(after, mainPathJournalRows(t, db)) {
		t.Fatal("repeat changed original history/time")
	}
	var path string
	if err := db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		t.Fatal(err)
	}
	peer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if !reflect.DeepEqual(after, mainPathJournalRows(t, peer)) {
		t.Fatal("independent reader cannot see committed confirmation")
	}
	session = mainPathPublishStep(t, db, q, session, b, strings.Repeat("3", 64))
	c := mainPathNextInvocation(t, q, session, execution, "execute-third", "main-third")
	third, err := q.ReadLiquibaseCapturePrunePlan(t.Context(), session, c)
	if err != nil || third == nil || third.PriorInvocationRef != "main-second" || c.CaptureSlot != a.CaptureSlot {
		t.Fatal("third invocation lost bounded predecessor history", err)
	}
	var slots int
	if err := db.QueryRow("SELECT count(*) FROM prepare_liquibase_capture_slots").Scan(&slots); err != nil || slots != 2 {
		t.Fatalf("unbounded slots: %d %v", slots, err)
	}
}

func TestMainPathCapturePruneFirstInvocation(t *testing.T) {
	db, q, session, execution := mainPathSessionJournal(t)
	invocation, err := q.PlanLiquibaseSessionInvocation(t.Context(), session, "execute", execution, queue.LiquibaseInvocationPlan{InvocationRef: "main-first", StdoutRef: "main-first-stdout", StderrRef: "main-first-stderr", ManifestRef: "main-first-manifest", ProtocolRevision: "lb-step-capture-v2", CreatedAt: mainPathJournalUTC})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := q.ReadLiquibaseCapturePrunePlan(t.Context(), session, invocation)
	if err != nil || plan != nil {
		t.Fatalf("first invocation invented predecessor: %+v %v", plan, err)
	}
	var attempts, slots int
	if err := db.QueryRow("SELECT count(*) FROM prepare_step_attempts").Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM prepare_liquibase_capture_slots").Scan(&slots); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || slots != 1 {
		t.Fatalf("read changed reservation: attempts=%d slots=%d", attempts, slots)
	}
	for _, variant := range []string{"runner", "generation", "invocation", "helper", "prior"} {
		t.Run(variant, func(t *testing.T) {
			badSession, badInvocation := session, invocation
			switch variant {
			case "runner":
				badSession.RunnerID = strings.Repeat("b", 64)
			case "generation":
				badSession.WorkerGeneration++
			case "invocation":
				badInvocation.InvocationRef = "foreign"
			case "helper":
				badInvocation.HelperDigest = "sha256:" + strings.Repeat("f", 64)
			case "prior":
				ref := "foreign"
				badInvocation.PriorRetirementRef = &ref
			}
			if plan, err := q.ReadLiquibaseCapturePrunePlan(t.Context(), badSession, badInvocation); err == nil || plan != nil {
				t.Fatalf("broken binding became first invocation: %+v %v", plan, err)
			}
		})
	}
}

// MP04: neither a failed second write nor a genuine SQLite COMMIT BUSY may
// expose a partial retirement as success. The same saved request remains usable.
func TestMainPathCapturePruneRollback(t *testing.T) {
	for _, fault := range []string{"second_write", "commit_busy"} {
		t.Run(fault, func(t *testing.T) {
			db, q, session, execution := mainPathSessionJournal(t)
			a, err := q.PlanLiquibaseSessionInvocation(t.Context(), session, "execute", execution, mainPathInvocationPlan("main-first"))
			if err != nil {
				t.Fatal(err)
			}
			session = mainPathPublishStep(t, db, q, session, a, strings.Repeat("2", 64))
			b := mainPathNextInvocation(t, q, session, execution, "execute-next", "main-second")
			plan, err := q.ReadLiquibaseCapturePrunePlan(t.Context(), session, b)
			if err != nil || plan == nil {
				t.Fatal(err)
			}
			probe := &mainPathPruneProbe{db: db, want: *plan}
			before := mainPathJournalRows(t, db)
			var release func()
			if fault == "second_write" {
				_, err = db.Exec(`CREATE TRIGGER test_refuse_slot_delete BEFORE DELETE ON prepare_liquibase_capture_slots BEGIN SELECT RAISE(ABORT,'test second write'); END`)
				if err != nil {
					t.Fatal(err)
				}
				release = func() {
					if _, err := db.Exec("DROP TRIGGER test_refuse_slot_delete"); err != nil {
						t.Error(err)
					}
				}
			} else {
				var file string
				if err := db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file); err != nil {
					t.Fatal(err)
				}
				peer, err := sql.Open("sqlite", file)
				if err != nil {
					t.Fatal(err)
				}
				defer peer.Close()
				tx, err := peer.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				var slots int
				if err := tx.QueryRow("SELECT count(*) FROM prepare_liquibase_capture_slots").Scan(&slots); err != nil || slots != 2 {
					t.Fatal("reader lock not established", err)
				}
				release = func() {
					if err := tx.Rollback(); err != nil {
						t.Error(err)
					}
				}
			}
			err = q.ConfirmLiquibaseCapturePruned(t.Context(), session, b, probe, mainPathJournalUTC)
			if err == nil || probe.calls != 1 {
				t.Fatalf("fault did not reach confirmation: calls=%d err=%v", probe.calls, err)
			}
			if fault == "second_write" && !strings.Contains(err.Error(), "test second write") {
				t.Fatal("wrong failure boundary", err)
			}
			if fault == "commit_busy" {
				var coded interface{ Code() int }
				if !errors.As(err, &coded) || coded.Code() != 5 {
					t.Fatal("not a real SQLITE_BUSY", err)
				}
			}
			if !reflect.DeepEqual(before, mainPathJournalRows(t, db)) {
				t.Fatal("failed confirmation leaked uncommitted changes")
			}
			release()
			if err := q.ConfirmLiquibaseCapturePruned(t.Context(), session, b, probe, mainPathJournalUTC); err != nil {
				t.Fatal("honest retry after removing fault", err)
			}
			if probe.calls != 2 {
				t.Fatal("retry skipped its observation")
			}
		})
	}
}
