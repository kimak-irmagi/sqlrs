//go:build linux

package benchmark

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "modernc.org/sqlite"
)

type releasedBenchmarkSample struct {
	SourceRevision, JavaDetails                   string
	CoreSHA256, LauncherSHA256                    string
	JVMStartsFromPinnedLauncher                   int
	Route, Fixture, Pass, EngineSHA256, Platform  string
	ElapsedMS                                     float64
	Changesets, CacheHits, LiquibaseCLIStarts     int
	MigrationCLIStarts, ImageInspectionContainers int
	ContainerCreates                              int
	LiquibaseVersion, DriverSHA256                string
	ImageDigest, HelperDigest                     string
	OracleOK, CleanupOK                           bool
	SnapshotBackend                               string
}

type benchmarkContainerCreate struct {
	at          int64
	image, name string
}

type benchmarkDockerCreates struct {
	mu     sync.Mutex
	events []benchmarkContainerCreate
	err    error
	cmd    *exec.Cmd
	closed chan struct{}
}

// barrier drains every create preceding an out-of-interval marker. It avoids
// interpreting a delayed scanner as a low container count (PA06).
func (r *benchmarkDockerCreates) barrier(t *testing.T, ctx context.Context, image string) {
	t.Helper()
	name := fmt.Sprintf("sqlrs-benchmark-barrier-%d", time.Now().UnixNano())
	created, err := runDockerE2ECommand(ctx, "create", "--name", name, image)
	id := strings.TrimSpace(string(created))
	if err != nil || len(id) != 64 {
		t.Fatal("event barrier creation failed")
	}
	defer func() {
		if _, err := runDockerE2ECommand(context.Background(), "rm", "-f", id); err != nil {
			t.Error("event barrier cleanup failed")
		}
	}()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		r.mu.Lock()
		seen := false
		for _, event := range r.events {
			if event.name == name {
				seen = true
			}
		}
		streamErr := r.err
		r.mu.Unlock()
		if streamErr != nil {
			t.Fatal("event stream failed")
		}
		if seen {
			return
		}
		select {
		case <-r.closed:
			t.Fatal("event stream ended before barrier")
		case <-ctx.Done():
			t.Fatal("event barrier cancelled")
		case <-deadline.C:
			t.Fatal("event barrier timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// startBenchmarkDockerCreates observes real Docker create events without a
// wrapper on the measured command path. Qualifying runs require a quiet host;
// the caller records all creates, including short-lived init/chmod helpers.
func startBenchmarkDockerCreates(t *testing.T, ctx context.Context) *benchmarkDockerCreates {
	t.Helper()
	r := &benchmarkDockerCreates{closed: make(chan struct{})}
	r.cmd = exec.CommandContext(ctx, "docker", "events", "--since", time.Now().UTC().Format(time.RFC3339Nano), "--filter", "type=container", "--filter", "event=create", "--format", "{{json .}}")
	pipe, err := r.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(r.closed)
		scanner := bufio.NewScanner(pipe)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var event struct {
				Action   string
				TimeNano int64 `json:"timeNano"`
				Actor    struct{ Attributes map[string]string }
			}
			err := json.Unmarshal(scanner.Bytes(), &event)
			r.mu.Lock()
			if err != nil || event.Action != "create" || event.TimeNano <= 0 || event.Actor.Attributes["image"] == "" || event.Actor.Attributes["name"] == "" {
				r.err = fmt.Errorf("Docker create stream is incomplete or invalid")
			} else {
				r.events = append(r.events, benchmarkContainerCreate{at: event.TimeNano, image: event.Actor.Attributes["image"], name: event.Actor.Attributes["name"]})
			}
			r.mu.Unlock()
		}
		r.mu.Lock()
		if err := scanner.Err(); err != nil {
			r.err = err
		}
		r.mu.Unlock()
	}()
	t.Cleanup(func() { _ = r.cmd.Process.Kill(); <-r.closed; _ = r.cmd.Wait() })
	return r
}

func (r *benchmarkDockerCreates) count(start, end time.Time, image string) (int, int, error) {
	return r.countNano(start.UnixNano(), end.UnixNano(), image)
}
func (r *benchmarkDockerCreates) countNano(start, end int64, image string) (int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return 0, 0, r.err
	}
	count := 0
	inspections := 0
	for _, event := range r.events {
		if event.at >= start && event.at <= end {
			count++
			// In both pinned container-engine sources, the only unnamed
			// Liquibase-image container is the offline version/JAR probe.
			// Migration executors have durable sqlrs-* names. A qualifying
			// pass rejects extra/missing probes instead of assuming one.
			if event.image == image && !strings.HasPrefix(event.name, "sqlrs-") {
				inspections++
			}
		}
	}
	return count, inspections, nil
}

// TestLiquibaseReleasedEngineBenchmarkDocker executes one isolated engine
// artifact's complete uncached/cached public workload. The external driver
// owns alternating control/comparison cohorts; this single run is not P01b.
// Requirements: docs/architecture/issue89-performance-actions-tests.md PA04–PA11.
func collect(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_RELEASED_BENCH") != "1" {
		t.Skip("opt-in external released-engine comparison")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Fatal("Linux/amd64 collector required")
	}
	var samples []releasedBenchmarkSample
	stage := "identity"
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("P01B_FAILURE_STAGE %s", stage)
		}
		if !t.Failed() {
			for _, sample := range samples {
				sample.OracleOK = true
				sample.CleanupOK = true
				sample.SnapshotBackend = "copy"
				data, _ := json.Marshal(sample)
				t.Logf("P01B_SAMPLE %s", data)
			}
		}
	})
	route, fixture := os.Getenv("SQLRS_BENCH_ROUTE"), os.Getenv("SQLRS_BENCH_FIXTURE")
	if route != "v1" && route != "v2" {
		t.Fatal("explicit released-engine benchmark route required")
	}
	repo, err := filepath.Abs(os.Getenv("SQLRS_BENCH_REPO"))
	if err != nil {
		t.Fatal(err)
	}
	engine := os.Getenv("SQLRS_BENCH_ENGINE")
	if !filepath.IsAbs(engine) {
		t.Fatal("absolute benchmark artifact path required")
	}
	engineBytes, err := os.ReadFile(engine)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(engineBytes)
	engineSHA := hex.EncodeToString(digest[:])
	info, err := buildinfo.ReadFile(engine)
	if err != nil {
		t.Fatal("benchmark artifact has no readable build provenance")
	}
	revision, modified := "", false
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			revision = setting.Value
		}
		if setting.Key == "vcs.modified" {
			modified = setting.Value == "true"
		}
	}
	if validateArtifact(info, os.Getenv("SQLRS_BENCH_REVISION")) != nil {
		t.Fatal("benchmark artifact has no fixed source revision")
	}
	if modified {
		t.Fatal("dirty benchmark artifact")
	}
	if expected := os.Getenv("SQLRS_BENCH_ENGINE_SHA256"); expected == "" || expected != engineSHA {
		t.Fatal("benchmark artifact differs from the collector's pinned checksum")
	}
	jar := filepath.Join(repo, "dist", "deps", "postgresql-42.7.13.jar")
	jarBytes, err := os.ReadFile(jar)
	if err != nil {
		t.Fatal(err)
	}
	jarDigest := sha256.Sum256(jarBytes)
	if hex.EncodeToString(jarDigest[:]) != "6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93" {
		t.Fatal("benchmark JDBC artifact changed")
	}
	if route != "v1" {
		installed, err := os.ReadFile(filepath.Join(filepath.Dir(engine), "lib", filepath.Base(jar)))
		if err != nil || !bytes.Equal(installed, jarBytes) {
			t.Fatal("container benchmark engine must have the same verified project JDBC bundle beside its executable")
		}
	}
	req := Request{PrepareKind: "lb", ImageID: os.Getenv("SQLRS_BENCH_DB_IMAGE"), WorkDir: os.Getenv("SQLRS_BENCH_INPUTS"), LiquibaseVersion: "5.0.4"}
	if !strings.Contains(req.ImageID, "@sha256:") || !filepath.IsAbs(req.WorkDir) {
		t.Fatal("pinned database and absolute fixture root required")
	}
	var queryPath, goldenPath, changePrefix string
	switch fixture {
	case "jhipster":
		req.LiquibaseVersion = "5.0.3"
		req.LiquibaseArgs = []string{"update", "--context-filter=prod", "--searchPath", "jhipster-sample-app", "--changelog-file", "jhipster-sample-app/config/liquibase/master.xml"}
		queryPath = filepath.Join(repo, "test", "performance", "liquibase", "fixtures", "jhipster.sql")
		goldenPath = filepath.Join(repo, "test", "performance", "liquibase", "fixtures", "jhipster.txt")
		changePrefix = "change|"
	case "mobilitydata-user", "mobilitydata-main":
		req.ImageID = "postgis/postgis@sha256:2a60827705ad26af212f31ca6ed4884f9fa95fffbe7eed5be516fece1bca25f8"
		file := "changelog.xml"
		if fixture == "mobilitydata-user" {
			file = "changelog_user.xml"
		}
		req.LiquibaseArgs = []string{"update", "--searchPath", "mobilitydata", "--changelog-file", "mobilitydata/" + file}
		queryPath = filepath.Join(repo, "test", "performance", "liquibase", "fixtures", "mobilitydata.sql")
		goldenPath = filepath.Join(repo, "test", "performance", "liquibase", "fixtures", fixture+".txt")
		changePrefix = "changelog|"
	default:
		t.Fatal("explicit public benchmark fixture required")
	}
	queryBytes, err := os.ReadFile(queryPath)
	if err != nil {
		t.Fatal(err)
	}
	query := strings.TrimPrefix(strings.ReplaceAll(string(queryBytes), "\r\n", "\n"), "\\c postgres\n")
	query = strings.ReplaceAll(query, "author, filename, md5sum", "author, regexp_replace(filename, '^((path[0-9]+|jhipster-sample-app)/)+', ''), md5sum")
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	golden := strings.TrimSpace(strings.ReplaceAll(string(goldenBytes), "\r\n", "\n"))
	expectedChanges := 0
	for _, line := range strings.Split(golden, "\n") {
		if strings.HasPrefix(line, changePrefix) {
			expectedChanges++
		}
	}
	if expectedChanges == 0 {
		t.Fatal("independent public oracle has no changesets")
	}
	version := req.LiquibaseVersion
	distribution := filepath.Join(os.Getenv("SQLRS_BENCH_DISTRIBUTIONS"), "liquibase-"+version)
	javaHome := os.Getenv("JAVA_HOME")
	if override := os.Getenv("SQLRS_BENCH_JAVA_HOME"); override != "" {
		javaHome = override
	}
	if javaHome == "" {
		java, err := exec.LookPath("java")
		if err != nil {
			t.Fatal("host Java is required for the real v1 comparison")
		}
		if resolved, err := filepath.EvalSymlinks(java); err == nil {
			java = resolved
		}
		javaHome = filepath.Dir(filepath.Dir(java))
	}
	liquibaseImage := "sha256:89efc0141afc7c7afe8df1b8fecf30a755da2e29a55e259c3dadd24adc9aea11"
	coreSHA := "00d0a18af81a12583d03524b6e685b26e3734183d965a6d79c1e9479d5e1c115"
	if version == "5.0.3" {
		liquibaseImage = "sha256:17f98b85e6d4c33b84ee409d055f1526227c77b55dd30e3879ec5088716a32eb"
		coreSHA = "3ccd4608829836b4c0f7fb1d23a4619bac16c68ca725d623aae0838bce734156"
	}
	owned, err := runDockerE2ECommand(context.Background(), "ps", "-a", "--filter", "name=sqlrs-", "--format", "{{.ID}}")
	if err != nil || len(bytes.TrimSpace(owned)) != 0 {
		t.Fatal("collection requires no other sqlrs Docker workload")
	}
	if route == "v1" {
		req, err = hostRequest(req, fixture)
		if err != nil {
			t.Fatal("native fixture root unavailable")
		}
		req.LiquibaseExec = filepath.Join(distribution, "liquibase")
		req.LiquibaseExecMode = "native"
		req.LiquibaseVersion = ""
		// RC7 owns runtime flags, so use the stock distribution's documented
		// global library directory instead of an unsupported --classpath flag.
		installed, err := os.ReadFile(filepath.Join(filepath.Dir(req.LiquibaseExec), "lib", filepath.Base(jar)))
		if err != nil || !bytes.Equal(installed, jarBytes) {
			t.Fatal("host Liquibase must use the same verified project JDBC dependency in its stock lib directory")
		}
		core, err := os.ReadFile(filepath.Join(filepath.Dir(req.LiquibaseExec), "internal", "lib", "liquibase-core.jar"))
		digest := sha256.Sum256(core)
		if err != nil || hex.EncodeToString(digest[:]) != coreSHA {
			t.Fatal("host Liquibase distribution differs from its pinned container extraction")
		}
	}
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stopSignals()
	ctx, cancel := context.WithTimeout(signalCtx, 145*time.Minute)
	defer cancel()
	// Metadata is outside the measured interval. Linux Java is explicitly
	// the pinned image's declared build metadata, not an OS process probe.
	javaDetails, launcher := "", "liquibase"
	if route == "v1" {
		launcher = "liquibase.bat"
		versionCtx, done := context.WithTimeout(ctx, 15*time.Second)
		output, err := exec.CommandContext(versionCtx, filepath.Join(javaHome, "bin", "java"), "-version").CombinedOutput()
		done()
		if err != nil {
			t.Fatal("cannot establish host Java version")
		}
		javaDetails = "host runtime: " + strings.TrimSpace(string(output))
	} else {
		output, err := runDockerE2ECommand(ctx, "image", "inspect", "liquibase/liquibase@"+liquibaseImage, "--format", "{{json .Config.Env}}")
		var entries []string
		if err != nil || json.Unmarshal(output, &entries) != nil {
			t.Fatal("cannot read pinned Java image metadata")
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry, "JAVA_VERSION=") {
				javaDetails = "Linux image build metadata: " + entry
			}
		}
		if javaDetails == "" {
			t.Fatal("pinned image has no declared Java version")
		}
	}
	launcherBytes, err := os.ReadFile(filepath.Join(distribution, launcher))
	if err != nil {
		t.Fatal("stock launcher bytes must be retained for the declared JVM count")
	}
	launcherDigest := sha256.Sum256(launcherBytes)
	root := t.TempDir()
	ownerBytes := make([]byte, 32)
	if _, err := rand.Read(ownerBytes); err != nil {
		t.Fatal("private ownership unavailable")
	}
	ownerToken := hex.EncodeToString(ownerBytes)
	if err := os.WriteFile(filepath.Join(root, ".collector-owner"), []byte(ownerToken), 0600); err != nil {
		t.Fatal("private ownership unavailable")
	}
	storeRoot := filepath.Join(root, "store")
	statePath := filepath.Join(root, "engine.json")
	dbPath := filepath.Join(storeRoot, "state.db")
	child := exec.CommandContext(ctx, engine, "--listen", "127.0.0.1:0", "--write-engine-json", statePath, "--idle-timeout", "0")
	child.Dir = req.WorkDir
	stage = "engine_start"
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "PG") || upper == "SQLRS_STATE_STORE" || upper == "SQLRS_STATE_DB" || upper == "SQLRS_CONTAINER_RUNTIME" || upper == "JAVA_HOME" {
			continue
		}
		child.Env = append(child.Env, entry)
	}
	child.Env = append(child.Env, "SQLRS_STATE_STORE="+storeRoot, "SQLRS_STATE_DB="+dbPath, "SQLRS_CONTAINER_RUNTIME=docker", "JAVA_HOME="+javaHome)
	child.Stdout, child.Stderr = io.Discard, io.Discard
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan struct{})
	go func() { _ = child.Wait(); close(wait) }()
	t.Cleanup(func() {
		// The exact process group is owned even if its leader already exited.
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
		select {
		case <-wait:
		default:
			_ = syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
			select {
			case <-wait:
			case <-time.After(5 * time.Second):
				_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
			}
			select {
			case <-wait:
			case <-time.After(10 * time.Second):
				t.Error("benchmark engine process did not exit")
			}
		}
		clean, cleanCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cleanCancel()
		if err := cleanupDockerE2EContainers(clean, root, runDockerE2ECommand); err != nil {
			t.Log("P01B_CLEANUP_FAILURE containers")
			t.Error("exact cleanup failed")
			return
		}
		if err := cleanupPrivateRoot(clean, root, ownerToken, req.ImageID, runDockerE2ECommand); err != nil {
			t.Log("P01B_CLEANUP_FAILURE files")
			t.Error("exact cleanup failed")
		}
	})
	var state struct {
		Endpoint  string `json:"endpoint"`
		AuthToken string `json:"authToken"`
		PID       int    `json:"pid"`
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		data, err := os.ReadFile(statePath)
		if err == nil && json.Unmarshal(data, &state) == nil && state.AuthToken != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("isolated released engine did not publish its endpoint")
		}
		select {
		case <-wait:
			t.Fatal("isolated released engine exited during startup")
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if state.PID != child.Process.Pid || !strings.HasPrefix(state.Endpoint, "127.0.0.1:") {
		t.Fatal("benchmark endpoint is not the exact spawned local engine")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	call := func(method, path string, body any, target any) int {
		t.Helper()
		var encoded []byte
		if body != nil {
			var err error
			encoded, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		request, err := http.NewRequestWithContext(ctx, method, "http://"+state.Endpoint+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+state.AuthToken)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("isolated benchmark HTTP request failed")
		}
		defer response.Body.Close()
		if target != nil && response.StatusCode < 300 {
			if err := json.NewDecoder(response.Body).Decode(target); err != nil {
				t.Fatal("invalid benchmark response")
			}
		}
		return response.StatusCode
	}
	for _, setting := range []struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}{
		{"snapshot.backend", "copy"}, {"cache.capacity.maxBytes", 0}, {"cache.capacity.reserveBytes", 0},
	} {
		stage = "configuration"
		if code := call(http.MethodPatch, "/v1/config", setting, nil); code >= 300 {
			t.Fatalf("benchmark configuration %s rejected with %d", setting.Path, code)
		}
	}
	var backend struct {
		Value string `json:"value"`
	}
	if call(http.MethodGet, "/v1/config?path=snapshot.backend&effective=true", nil, &backend) != http.StatusOK || backend.Value != "copy" {
		t.Fatal("copy backend was not confirmed")
	}
	creates := startBenchmarkDockerCreates(t, ctx)
	creates.barrier(t, ctx, req.ImageID)
	workingHelper := ""
	for _, pass := range []string{"uncached", "cached"} {
		stage = pass
		start := time.Now()
		// RC7 rejects unknown request fields; construct each actual contract,
		// rather than relying on compatibility support excluded from #89.
		payload := map[string]any{"prepare_kind": req.PrepareKind, "image_id": req.ImageID, "work_dir": req.WorkDir, "liquibase_args": req.LiquibaseArgs}
		if route == "v1" {
			payload["liquibase_exec"], payload["liquibase_exec_mode"] = req.LiquibaseExec, req.LiquibaseExecMode
		} else {
			payload["liquibase_version"] = req.LiquibaseVersion
		}
		var accepted Accepted
		if code := call(http.MethodPost, "/v1/prepare-jobs", payload, &accepted); code != http.StatusCreated || accepted.JobID == "" {
			t.Fatalf("released %s prepare rejected: HTTP %d", route, code)
		}
		var status Status
		for {
			if code := call(http.MethodGet, "/v1/prepare-jobs/"+accepted.JobID, nil, &status); code != http.StatusOK {
				t.Fatalf("released status HTTP %d", code)
			}
			if status.Status == StatusSucceeded {
				break
			}
			if status.Status == StatusFailed || status.Status == StatusRecoveryRequired {
				t.Logf("P01B_FAILURE_KIND %s", classifyEngineFailure(status.Error))
				code := "missing_error"
				message := ""
				if status.Error != nil {
					code = "engine_error"
					message = "private diagnostics withheld"
				}
				// Error messages name the failed engine operation; omit details,
				// private capture and DSNs from benchmark diagnostics.
				t.Fatalf("released %s prepare ended %s (%s: %s)", route, status.Status, code, message)
			}
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			time.Sleep(100 * time.Millisecond)
		}
		end := time.Now()
		stage = "oracle"
		creates.barrier(t, ctx, req.ImageID)
		if status.Result == nil || status.Result.DSN == "" {
			t.Fatal("released prepare has no usable instance")
		}
		conn, err := pgconn.Connect(ctx, status.Result.DSN)
		if err != nil {
			t.Fatal("cannot verify benchmark result database")
		}
		results, err := conn.Exec(ctx, query).ReadAll()
		conn.Close(ctx)
		if err != nil {
			t.Fatal("benchmark oracle query failed")
		}
		var actual []string
		for _, result := range results {
			for _, row := range result.Rows {
				fields := make([]string, len(row))
				for i, value := range row {
					fields[i] = string(value)
				}
				actual = append(actual, strings.Join(fields, "|"))
			}
		}
		if strings.Join(actual, "\n") != golden {
			for _, part := range oracleMismatchParts(actual, golden) {
				t.Logf("P01B_ORACLE_PART %s", part)
			}
			t.Fatal("released engine result differs from independent public oracle")
		}
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		var steps, hits int
		if err := db.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(cached),0) FROM prepare_tasks WHERE job_id=? AND type='state_execute'`, accepted.JobID).Scan(&steps, &hits); err != nil {
			t.Fatal(err)
		}
		if steps != expectedChanges || (pass == "uncached" && hits != 0) || (pass == "cached" && hits != steps) {
			t.Fatalf("benchmark cache pass is not the declared workload: steps=%d hits=%d", steps, hits)
		}
		starts := 0
		image, helper := liquibaseImage, ""
		if route == "v1" {
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM prepare_events WHERE job_id=? AND message LIKE 'liquibase: exec %'`, accepted.JobID).Scan(&starts); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM prepare_step_attempts WHERE job_id=?`, accepted.JobID).Scan(&starts); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRowContext(ctx, `SELECT image_manifest_digest FROM liquibase_execution_bindings e JOIN liquibase_job_bindings j USING(execution_fingerprint) WHERE j.job_id=?`, accepted.JobID).Scan(&image); err != nil {
				t.Fatal(err)
			}
			if route == "v2" && pass == "uncached" {
				if err := db.QueryRowContext(ctx, `SELECT capture_helper_digest FROM prepare_liquibase_sessions WHERE job_id=?`, accepted.JobID).Scan(&helper); err != nil {
					t.Fatal(err)
				}
				workingHelper = helper
			}
		}
		db.Close()
		wantStarts := 1
		if pass == "uncached" {
			wantStarts += expectedChanges
		}
		if starts != wantStarts {
			t.Fatalf("benchmark Liquibase CLI count=%d expected=%d", starts, wantStarts)
		}
		count, inspections, err := creates.count(start, end, "liquibase/liquibase@"+image)
		if err != nil || count == 0 {
			t.Fatal("benchmark Docker create stream is not complete")
		}
		wantInspections := 1
		if route == "v1" {
			wantInspections = 0
		}
		if inspections != wantInspections {
			t.Fatalf("offline image probes=%d expected=%d", inspections, wantInspections)
		}
		sample := releasedBenchmarkSample{Route: route, Fixture: fixture, Pass: pass, EngineSHA256: engineSHA, Platform: runtime.GOOS + "/" + runtime.GOARCH, ElapsedMS: float64(end.Sub(start)) / float64(time.Millisecond), Changesets: steps, CacheHits: hits, LiquibaseCLIStarts: starts, ContainerCreates: count, LiquibaseVersion: version, DriverSHA256: hex.EncodeToString(jarDigest[:]), ImageDigest: image, HelperDigest: helper}
		sample.SourceRevision, sample.JavaDetails = revision, javaDetails
		sample.MigrationCLIStarts, sample.ImageInspectionContainers = starts, inspections
		sample.LiquibaseCLIStarts += inspections
		sample.CoreSHA256, sample.LauncherSHA256 = coreSHA, hex.EncodeToString(launcherDigest[:])
		// Both retained stock launchers perform a java -version probe and a
		// main JVM per CLI, including offline --version. Image inspection
		// also directly runs java -version once. This inventory is derived
		// from pinned engine/launcher bytes and observed probe containers,
		// not presented as independent OS process telemetry.
		sample.JVMStartsFromPinnedLauncher = sample.LiquibaseCLIStarts*2 + inspections
		if route == "v2" {
			sample.HelperDigest = workingHelper
		}
		samples = append(samples, sample)
	}
}
