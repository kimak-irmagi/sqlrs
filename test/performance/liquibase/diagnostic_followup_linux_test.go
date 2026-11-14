package benchmark

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// DC02/DG03: independently malformed syntax must not masquerade as an empty
// successful trace. See issue89-performance-diagnostic-tests.md DC01–DC03.
func TestDiagnosticTraceRejectsMalformedRecords(t *testing.T) {
	for name, raw := range map[string]string{
		"zero_pid":            "0 100.1 +++ exited with 0 +++\n",
		"overflow_pid":        "999999999999999999999 100.1 +++ exited with 0 +++\n",
		"overflow_time":       "2 99999999999999999999.1 +++ exited with 0 +++\n",
		"long_fraction":       "2 100.1234567890 +++ exited with 0 +++\n",
		"pending_exit":        "2 100.1 execve(\"/bin/true\", [\"true\"], 0x0 <unfinished ...>\n2 100.2 +++ exited with 0 +++\n",
		"two_pending":         "2 100.1 execve(\"/bin/true\", [\"true\"], 0x0 <unfinished ...>\n2 100.2 execve(\"/bin/true\", [\"true\"], 0x0 <unfinished ...>\n",
		"not_exec":            "2 100.1 unknown() = 0\n",
		"no_result":           "2 100.1 execve(\"/bin/true\", [\"true\"], 0x0) = ?\n",
		"no_program":          "2 100.1 execve(0x1, 0x2, 0x3) = 0\n",
		"bad_program_escape":  `2 100.1 execve("\q", ["true"], 0x0) = 0` + "\n",
		"no_argv":             "2 100.1 execve(\"/bin/true\", 0x0, 0x0) = 0\n",
		"bad_argument_escape": `2 100.1 execve("/bin/true", ["\q"], 0x0) = 0` + "\n",
		"bad_separator":       "2 100.1 execve(\"/bin/true\", [\"true\" PRIVATE_SENTINEL], 0x0) = 0\n",
		"row_limit":           strings.Repeat("2 100.1 +++ exited with 0 +++\n", 250001),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeDiagnosticTrace(strings.NewReader(raw), 1); err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
				t.Fatal("malformed trace accepted or disclosed")
			}
		})
	}
	for _, raw := range []string{"100", "100.", "100.1234567890", "-1.1", "9223372036.0", "PRIVATE_SENTINEL.1", "1.PRIVATE_SENTINEL"} {
		if _, err := diagnosticTimestamp(raw); err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
			t.Fatal("invalid timestamp accepted/disclosed")
		}
	}
	if err := copyDiagnosticTrace(io.Discard, strings.NewReader("x"), 0); err == nil {
		t.Fatal("zero quota accepted")
	}
}

// DC02/DG04: argument text which resembles result/continuation syntax must not
// replace the actual return value or classify SQL as a process operation.
func TestDiagnosticTraceLiteralMarkersAndHelperCategories(t *testing.T) {
	trace := `2 100.1 execve("/bin/echo", ["echo", " = -1 ", "<unfinished ...>"], 0x0) = 0 <0.001>` + "\n2 100.2 +++ exited with 0 +++\n"
	rows, err := decodeDiagnosticTrace(strings.NewReader(trace), 1)
	if err != nil || !reflect.DeepEqual(rows, []diagnosticInterval{{"other", 100100000000, 100200000000}}) {
		t.Fatal("literal argument markers changed execution evidence")
	}
	for _, row := range []struct {
		args []string
		want string
	}{
		{[]string{"docker"}, "other"},
		{[]string{"docker", "exec", "id", "sh", "/sqlrs/helper/step-capture.sh"}, "docker.step"},
		{[]string{"docker", "exec", "id", "sh", "/sqlrs/helper/capture.sh"}, "docker.planner"},
	} {
		if diagnosticCategory("/usr/bin/docker", row.args) != row.want {
			t.Fatal("helper category differs")
		}
	}
}

// DC01: every negative SQLite case reaches its own independently specified
// read after a successful control, with no global interception or fake driver.
func TestDiagnosticJournalReadFailures(t *testing.T) {
	for name, mutation := range map[string]string{
		"missing_finished":                 "UPDATE prepare_jobs SET finished_at=NULL",
		"bad_job_time":                     "UPDATE prepare_jobs SET finished_at='PRIVATE_SENTINEL'",
		"before_origin":                    "UPDATE prepare_jobs SET created_at='1970-01-01T00:01:39Z'",
		"missing_task_column":              "ALTER TABLE prepare_tasks RENAME COLUMN position TO absent",
		"task_scan":                        "UPDATE prepare_tasks SET position='PRIVATE_SENTINEL'",
		"task_time":                        "UPDATE prepare_tasks SET finished_at='PRIVATE_SENTINEL'",
		"binding_query":                    "ALTER TABLE liquibase_job_bindings RENAME COLUMN bound_at TO absent",
		"planning_time":                    "UPDATE prepare_planning_intents SET plan_saved_at='PRIVATE_SENTINEL'",
		"invocation_query":                 "ALTER TABLE prepare_liquibase_invocations RENAME COLUMN complete_at TO absent",
		"large_ordinal_invalid_invocation": "UPDATE prepare_tasks SET position=9223372036854775807; UPDATE prepare_liquibase_invocations SET complete_at='PRIVATE_SENTINEL'",
		"invocation_time":                  "UPDATE prepare_liquibase_invocations SET complete_at='PRIVATE_SENTINEL'",
	} {
		t.Run(name, func(t *testing.T) {
			db := diagnosticReadFixture(t)
			if _, err := readDiagnosticJournal(context.Background(), db, "owned", time.Unix(100, 0)); err != nil {
				t.Fatal("positive journal control failed")
			}
			if _, err := db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if _, err := readDiagnosticJournal(context.Background(), db, "owned", time.Unix(100, 0)); err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
				t.Fatal("read failure accepted/disclosed")
			}
		})
	}
}

func diagnosticReadFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE prepare_jobs(job_id TEXT,created_at TEXT,started_at TEXT,finished_at TEXT);
CREATE TABLE prepare_tasks(job_id TEXT,task_id TEXT,position INTEGER,type TEXT,started_at TEXT,finished_at TEXT);
CREATE TABLE liquibase_job_bindings(job_id TEXT,bound_at TEXT);
CREATE TABLE prepare_planning_intents(job_id TEXT,created_at TEXT,plan_saved_at TEXT);
CREATE TABLE prepare_liquibase_invocations(job_id TEXT,task_id TEXT,attempt_no INT,start_requested_at TEXT,complete_at TEXT);
INSERT INTO prepare_jobs VALUES('owned','1970-01-01T00:01:40Z','1970-01-01T00:01:40.1Z','1970-01-01T00:01:41Z');
INSERT INTO prepare_tasks VALUES('owned','task',0,'state_execute','1970-01-01T00:01:40.2Z','1970-01-01T00:01:40.8Z');
INSERT INTO liquibase_job_bindings VALUES('owned','1970-01-01T00:01:40.15Z');
INSERT INTO prepare_planning_intents VALUES('owned','1970-01-01T00:01:40.15Z','1970-01-01T00:01:40.2Z');
INSERT INTO prepare_liquibase_invocations VALUES('owned','task',1,'1970-01-01T00:01:40.3Z','1970-01-01T00:01:40.7Z');`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDiagnosticProcessFailureBoundaries(t *testing.T) {
	want := []string{"-f", "-ttt", "-T", "-I", "3", "-s", "4096", "-e", "trace=execve,execveat", "-e", "signal=none", "-o", "/proc/self/fd/3", "--", "/engine", "one"}
	if !reflect.DeepEqual(diagnosticTraceArgs("/engine", []string{"one"}), want) {
		t.Fatal("external trace boundary differs")
	}
	for _, stat := range []string{"PRIVATE_SENTINEL", "12 (name) Z 10 10", "12 (name) S invalid 10", "12 (name) S 10"} {
		root := t.TempDir()
		dir := filepath.Join(root, "12")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0600); err != nil {
			t.Fatal(err)
		}
		if err := verifyDiagnosticEndpoint(root, 12, 10, "/exact"); err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
			t.Fatal("malformed process identity accepted/disclosed")
		}
	}
	for _, mode := range []string{"closed_file", "reader_timeout"} {
		t.Run(mode, func(t *testing.T) {
			trace, err := newDiagnosticTrace(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			fd, err := syscall.Dup(int(trace.writer.Fd()))
			if err != nil {
				trace.abort()
				t.Fatal(err)
			}
			writer := os.NewFile(uintptr(fd), "private-test-writer")
			defer writer.Close()
			trace.begin(0)
			if mode == "closed_file" {
				writer.Close()
				// Establish successful acquisition before failing Seek. Otherwise
				// the same test could pass on an earlier writer failure.
				readErr := <-trace.done
				if readErr != nil {
					t.Fatal("positive acquisition control failed")
				}
				trace.done <- readErr
				trace.file.Close()
			}
			started := time.Now()
			if _, err := trace.finish(1); err == nil {
				t.Fatal("unfinished/unreadable transport accepted")
			}
			if mode == "reader_timeout" && (time.Since(started) < 9*time.Second || time.Since(started) > 15*time.Second) {
				t.Fatal("reader timeout/join differs")
			}
		})
	}
	cmd := exec.Command("/bin/true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(syscall.Kill(-cmd.Process.Pid, 0), syscall.ESRCH) {
		t.Fatal("owned control group still exists")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := stopDiagnosticGroup(ctx, cmd.Process.Pid, nil); err == nil {
		t.Fatal("missing Wait proof accepted")
	}
}
