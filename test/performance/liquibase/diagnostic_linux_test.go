package benchmark

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// DG03 uses independently specified microsecond timestamps, never decoder output
// to manufacture its own expected intervals.
func TestDiagnosticTraceRecords(t *testing.T) {
	trace := "1 100.000000 execve(\"/engine\", [\"/engine\"], 0x0) = 0\n" +
		"2 100.100000 execve(\"/usr/bin/docker\", [\"docker\", \"image\", \"inspect\", \"PRIVATE_SENTINEL\"], 0x0 <unfinished ...>\n" +
		"3 100.200000 execve(\"/missing\", [\"missing\"], 0x0) = -1 ENOENT (unavailable)\n" +
		"2 100.110000 <... execve resumed>) = 0\n" +
		"2 100.400000 +++ exited with 0 +++\n" +
		"3 100.300000 +++ exited with 127 +++\n" +
		"2 100.500000 execveat(AT_FDCWD, \"/bin/java\", [\"java\", \"quoted\\\"arg\"], 0x0, 0) = 0\n" +
		"2 100.600000 execve(\"/bin/true\", [\"true\"], 0x0) = 0\n" +
		"2 100.700000 +++ exited with 0 +++\n" +
		"4 100.800000 +++ exited with 0 +++\n1 101.000000 +++ exited with 0 +++\n"
	got, err := decodeDiagnosticTrace(strings.NewReader(trace), 1)
	want := []diagnosticInterval{{"docker.image_inspect", 100100000000, 100400000000}, {"host.java", 100500000000, 100600000000}, {"other", 100600000000, 100700000000}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded intervals differ: %v", err)
	}
	for _, invalid := range []string{
		"PRIVATE_SENTINEL\n", "2 not-time anything\n",
		"2 100.0 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n",
		"2 100.2 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n2 100.1 +++ exited with 0 +++\n",
		"2 100.0 <... execve resumed>) = 0\n",
		"2 100.0 execve(\"/bin/java\", [\"java\"], 0x0 <unfinished ...>\n",
		"2 100.0 execve(\"/bin/java\", [\"java\", ...], 0x0) = 0\n",
		strings.Repeat("x", 65537),
	} {
		if _, err := decodeDiagnosticTrace(strings.NewReader(invalid), 1); err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
			t.Fatal("invalid trace accepted or exposed raw diagnostics")
		}
	}
	if _, err := decodeDiagnosticTrace(helperFailingReader{errors.New("PRIVATE_SENTINEL")}, 1); err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
		t.Fatal("read failure accepted or exposed")
	}
}

// DG04 checks classifications at argument boundaries and never searches SQL text.
func TestDiagnosticOperationCategories(t *testing.T) {
	for _, row := range []struct {
		path string
		args []string
		want string
	}{
		{"/usr/bin/docker", []string{"docker", "inspect", "id"}, "docker.inspect"},
		{"/usr/bin/docker", []string{"docker", "container", "inspect", "id"}, "docker.inspect"},
		{"/usr/bin/docker", []string{"docker", "image", "inspect", "id"}, "docker.image_inspect"},
		{"/usr/bin/docker", []string{"docker", "create", "id"}, "docker.create"},
		{"/usr/bin/docker", []string{"docker", "start", "id"}, "docker.start"},
		{"/usr/bin/docker", []string{"docker", "stop", "id"}, "docker.stop"},
		{"/usr/bin/docker", []string{"docker", "container", "rm", "-f", "id"}, "docker.remove"},
		{"/usr/bin/docker", []string{"docker", "cp", "id:/private", "-"}, "docker.copy"},
		{"/usr/bin/docker", []string{"docker", "run", "--rm"}, "docker.run"},
		{"/usr/bin/docker", []string{"docker", "exec", "id", "sh", "PRIVATE_SENTINEL pg_ctl"}, "docker.exec"},
		{"/opt/liquibase/liquibase", []string{"liquibase", "update-count"}, "host.liquibase"},
		{"/opt/java/bin/java", []string{"java", "-version"}, "host.java"},
		{"/bin/pg_ctl", []string{"pg_ctl", "start"}, "host.pg_control"},
		{"/bin/psql", []string{"psql"}, "host.pg_identity"},
		{"/bin/echo", []string{"echo", "docker", "pg_ctl"}, "other"},
	} {
		if got := diagnosticCategory(row.path, row.args); got != row.want {
			t.Fatalf("category differs: %s", got)
		}
	}
}

// DG05 union coverage must not add overlapping parent/child command durations.
func TestDiagnosticIntervalCoverage(t *testing.T) {
	start, end := time.Unix(100, 0), time.Unix(101, 0)
	rows := []diagnosticInterval{{"host.liquibase", 99900000000, 100600000000}, {"host.java", 100200000000, 100500000000}, {"docker.copy", 100600000000, 100800000000}, {"other", 101100000000, 101200000000}}
	got, err := summarizeDiagnosticIntervals(rows, start, end)
	if err != nil || got.CoveredMS != 800 || got.ResidualMS != 200 || got.Commands != 3 || len(got.Intervals) != 3 {
		t.Fatal("overlap/window accounting differs")
	}
	if _, err := summarizeDiagnosticIntervals([]diagnosticInterval{{"other", 4, 3}}, start, end); err == nil {
		t.Fatal("negative interval accepted")
	}
	if _, err := summarizeDiagnosticIntervals(nil, end, start); err == nil {
		t.Fatal("backwards window accepted")
	}
	if got, err := summarizeDiagnosticIntervals(nil, start, end); err != nil || got.ResidualMS != 1000 {
		t.Fatal("empty work was hidden")
	}
}

type diagnosticFailWriter struct{}

func (diagnosticFailWriter) Write([]byte) (int, error) { return 0, errors.New("PRIVATE_SENTINEL") }

// DG06 errors cannot expose the private source, or retain more than the cap.
func TestDiagnosticTraceTransportLimits(t *testing.T) {
	for _, n := range []int{0, 8, 9} {
		var dst bytes.Buffer
		err := copyDiagnosticTrace(&dst, strings.NewReader(strings.Repeat("x", n)), 8)
		if (err != nil) != (n > 8) || dst.Len() > 8 {
			t.Fatal("trace cap differs")
		}
	}
	for _, err := range []error{
		copyDiagnosticTrace(io.Discard, helperFailingReader{errors.New("PRIVATE_SENTINEL")}, 8),
		copyDiagnosticTrace(diagnosticFailWriter{}, strings.NewReader("x"), 8),
	} {
		if err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
			t.Fatal("transport failure exposed or ignored")
		}
	}
}

// DG07 proof uses all three independent process facts, not a reported PID alone.
func TestDiagnosticEndpointOwnership(t *testing.T) {
	root := t.TempDir()
	makeProc := func(pid string, parent, group int, executable string) {
		dir := filepath.Join(root, pid)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		stat := pid + " (name with ) parens) S " + strconv.Itoa(parent) + " " + strconv.Itoa(group) + " 0"
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(executable, filepath.Join(dir, "exe")); err != nil {
			t.Fatal(err)
		}
	}
	makeProc("12", 10, 10, "/exact-engine")
	if verifyDiagnosticEndpoint(root, 12, 10, "/exact-engine") != nil {
		t.Fatal("exact child rejected")
	}
	for _, row := range []struct {
		pid, launcher int
		exe           string
	}{{12, 11, "/exact-engine"}, {12, 10, "/other"}, {13, 10, "/exact-engine"}, {0, 10, "/exact-engine"}} {
		if verifyDiagnosticEndpoint(root, row.pid, row.launcher, row.exe) == nil {
			t.Fatal("foreign endpoint accepted")
		}
	}
	makeProc("14", 10, 11, "/exact-engine")
	if verifyDiagnosticEndpoint(root, 14, 10, "/exact-engine") == nil {
		t.Fatal("foreign group accepted")
	}
	if stopDiagnosticGroup(context.Background(), 0, nil) == nil || stopDiagnosticGroup(context.Background(), syscall.Getpgrp(), nil) == nil {
		t.Fatal("broad group target accepted")
	}
}

// DG07 runs only a tiny process tree, not DB/Liquibase, to verify actual strace.
func TestDiagnosticStraceProcessSmoke(t *testing.T) {
	if os.Getenv("SQLRS_DIAGNOSTIC_SMOKE") != "1" {
		t.Skip("explicit Actions strace smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := t.TempDir()
	trace, err := newDiagnosticTrace(root)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "strace", diagnosticTraceArgs("/bin/sh", []string{"-c", "/bin/true"})...)
	cmd.ExtraFiles = []*os.File{trace.writer}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		trace.abort()
		t.Fatal("strace smoke unavailable")
	}
	trace.begin(cmd.Process.Pid)
	if err := cmd.Wait(); err != nil {
		trace.abort()
		t.Fatal("strace smoke failed")
	}
	rows, err := trace.finish(-1)
	if err != nil || len(rows) == 0 {
		t.Fatal("real strace was not decoded")
	}
}

// DG08 reads actual SQLite streams; rows for another job must not escape.
func TestDiagnosticJournalPhases(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE prepare_jobs(job_id TEXT,created_at TEXT,started_at TEXT,finished_at TEXT);
CREATE TABLE prepare_tasks(job_id TEXT,task_id TEXT,position INTEGER,type TEXT,started_at TEXT,finished_at TEXT);
INSERT INTO prepare_jobs VALUES('owned','1970-01-01T00:01:40Z','1970-01-01T00:01:40.1Z','1970-01-01T00:01:41Z');
INSERT INTO prepare_tasks VALUES('owned','PRIVATE_SENTINEL',2,'state_execute','1970-01-01T00:01:40.2Z','1970-01-01T00:01:40.8Z');
INSERT INTO prepare_tasks VALUES('foreign','SECRET',1,'plan','invalid','invalid');`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readDiagnosticJournal(context.Background(), db, "owned", time.Unix(100, 0))
	if err != nil || len(rows) < 2 {
		t.Fatal("journal phases unavailable")
	}
	if rows[1].Ordinal != 2 || rows[1].StartMS == nil || *rows[1].StartMS != 200 || rows[1].EndMS == nil || *rows[1].EndMS != 800 {
		t.Fatal("ordinal/relative timestamps differ")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readDiagnosticJournal(ctx, db, "owned", time.Unix(100, 0)); err == nil {
		t.Fatal("cancelled journal accepted")
	}
	if _, err := readDiagnosticJournal(context.Background(), db, "missing", time.Unix(100, 0)); err == nil {
		t.Fatal("missing job accepted")
	}
	if _, err := db.Exec(`UPDATE prepare_jobs SET started_at='PRIVATE_SENTINEL' WHERE job_id='owned'`); err != nil {
		t.Fatal(err)
	}
	if _, err := readDiagnosticJournal(context.Background(), db, "owned", time.Unix(100, 0)); err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
		t.Fatal("invalid journal leaked/accepted")
	}
}

// DG02: metadata follows the Linux executable, not a similarly named sibling.
func TestDiagnosticNativeLauncherSelection(t *testing.T) {
	collector, err := os.ReadFile("collector_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(collector, []byte("launcher = filepath.Base(req.LiquibaseExec)")) || bytes.Contains(collector, []byte(`launcher = "liquibase.bat"`)) {
		t.Fatal("Linux launcher provenance differs from invoked entry point")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "liquibase"), []byte("POSIX"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "liquibase.bat"), []byte("BAT"), 0600); err != nil {
		t.Fatal(err)
	}
	if a, _ := os.ReadFile(filepath.Join(root, "liquibase")); bytes.Equal(a, []byte("BAT")) {
		t.Fatal("distinct provenance fixture collapsed")
	}
}

// DG06/DG07 exercise a real private pipe and owned processes without Docker/DB.
func TestDiagnosticTraceLifecycle(t *testing.T) {
	for _, mode := range []string{"finish", "abort", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			trace, err := newDiagnosticTrace(root)
			if err != nil {
				t.Fatal(err)
			}
			if info, err := os.Stat(filepath.Join(root, "diagnostic.trace")); err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("trace is not private")
			}
			if _, err := newDiagnosticTrace(root); err == nil {
				t.Fatal("existing trace overwritten")
			}
			writer, err := syscall.Dup(int(trace.writer.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			privateWriter := os.NewFile(uintptr(writer), "test-pipe")
			trace.begin(0)
			if mode == "abort" {
				privateWriter.Close()
				trace.abort()
				return
			}
			if mode == "overflow" {
				_, _ = io.Copy(privateWriter, io.LimitReader(&diagnosticZeroReader{}, (32<<20)+1))
			} else {
				_, err = io.WriteString(privateWriter, "2 100.0 execve(\"/bin/true\", [\"true\"], 0x0) = 0\n2 100.1 +++ exited with 0 +++\n")
				if err != nil {
					t.Fatal(err)
				}
			}
			privateWriter.Close()
			rows, err := trace.finish(1)
			if (err != nil) != (mode == "overflow") {
				t.Fatal("reader completion/quota differs")
			}
			if mode == "finish" && len(rows) != 1 {
				t.Fatal("trace lost")
			}
			info, statErr := os.Stat(filepath.Join(root, "diagnostic.trace"))
			if statErr != nil || info.Size() > 32<<20 {
				t.Fatal("private storage exceeded cap")
			}
		})
	}
	if _, err := newDiagnosticTrace("relative"); err == nil {
		t.Fatal("relative root accepted")
	}
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := newDiagnosticTrace(link); err == nil {
		t.Fatal("symlink root accepted")
	}
	trace, err := newDiagnosticTrace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trace.finish(1); err == nil {
		t.Fatal("unstarted transport accepted")
	}
}

type diagnosticZeroReader struct{}

func (*diagnosticZeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestDiagnosticOwnedGroupShutdown(t *testing.T) {
	for _, mode := range []string{"term", "kill", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{"-c", "exec sleep 30"}
			if mode == "kill" {
				args = []string{"-c", `trap '' TERM; echo ready; exec sleep 30`}
			}
			cmd := exec.Command("/bin/sh", args...)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			ready, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { _ = cmd.Wait(); close(done) }()
			t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); <-done })
			if mode == "kill" {
				b := make([]byte, 6)
				if _, err := io.ReadFull(ready, b); err != nil {
					t.Fatal("owned process did not reach TERM trap")
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			err = stopDiagnosticGroup(ctx, cmd.Process.Pid, done)
			if (err != nil) != (mode == "cancel") {
				t.Fatal("owned group shutdown differs")
			}
			if err == nil && !errors.Is(syscall.Kill(-cmd.Process.Pid, 0), syscall.ESRCH) {
				t.Fatal("success before group absence")
			}
		})
	}
}

func TestDiagnosticJournalCurrentSchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE prepare_jobs(job_id TEXT,created_at TEXT,started_at TEXT,finished_at TEXT);
CREATE TABLE prepare_tasks(job_id TEXT,task_id TEXT,position INTEGER,type TEXT,started_at TEXT,finished_at TEXT);
CREATE TABLE liquibase_job_bindings(job_id TEXT,bound_at TEXT);
CREATE TABLE prepare_planning_intents(job_id TEXT,created_at TEXT,plan_saved_at TEXT);
CREATE TABLE prepare_liquibase_sessions(job_id TEXT,session_id TEXT,session_no INT,created_at TEXT);
CREATE TABLE prepare_liquibase_session_readiness(session_id TEXT,verified_at TEXT);
CREATE TABLE prepare_liquibase_invocations(job_id TEXT,task_id TEXT,attempt_no INT,start_requested_at TEXT,complete_at TEXT);
INSERT INTO prepare_jobs VALUES('j','1970-01-01T00:01:40Z','1970-01-01T00:01:40.01Z','1970-01-01T00:01:41Z');
INSERT INTO prepare_tasks VALUES('j','p',0,'plan','1970-01-01T00:01:40.02Z','1970-01-01T00:01:40.03Z');
INSERT INTO prepare_tasks VALUES('j','r',1,'resolve_image','1970-01-01T00:01:40.02Z','1970-01-01T00:01:40.03Z');
INSERT INTO prepare_tasks VALUES('j','i',2,'instance_create',NULL,NULL);
INSERT INTO prepare_tasks VALUES('j','x',3,'unknown',NULL,NULL);
INSERT INTO liquibase_job_bindings VALUES('j','1970-01-01T00:01:40.1Z');
INSERT INTO prepare_planning_intents VALUES('j','1970-01-01T00:01:40.1Z','1970-01-01T00:01:40.5Z');
INSERT INTO prepare_liquibase_sessions VALUES('j','s',1,'1970-01-01T00:01:40.5Z');
INSERT INTO prepare_liquibase_session_readiness VALUES('s','1970-01-01T00:01:40.6Z');
INSERT INTO prepare_liquibase_invocations VALUES('j','p',1,'1970-01-01T00:01:40.7Z','1970-01-01T00:01:40.8Z');`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readDiagnosticJournal(context.Background(), db, "j", time.Unix(100, 0))
	if err != nil || len(rows) != 9 {
		t.Fatal("current journal unavailable")
	}
	if rows[5].Category != "image_binding" || *rows[5].EndMS != 100 || rows[8].Category != "invocation" || *rows[8].StartMS != 700 {
		t.Fatal("current phase linkage differs")
	}
	for _, schema := range []string{"../../../backend/local-engine-go/internal/prepare/queue/assets/liquibase-main-schema.sql", "../../../backend/local-engine-go/internal/managedstore/testdata/issue89-released.sql"} {
		b, err := os.ReadFile(schema)
		if err != nil {
			t.Fatal(err)
		}
		for _, column := range []string{"created_at", "started_at", "finished_at", "prepare_tasks", "position"} {
			if !bytes.Contains(b, []byte(column)) {
				t.Fatal("journal query contradicts canonical columns")
			}
		}
	}
	for _, sqlText := range []string{"UPDATE prepare_planning_intents SET plan_saved_at='invalid'", "DROP TABLE prepare_planning_intents", "DROP TABLE prepare_liquibase_invocations"} {
		if _, err := db.Exec(sqlText); err != nil {
			t.Fatal(err)
		}
		_, err := readDiagnosticJournal(context.Background(), db, "j", time.Unix(100, 0))
		if strings.HasPrefix(sqlText, "UPDATE") && err == nil {
			t.Fatal("invalid optional time accepted")
		}
	}
}
