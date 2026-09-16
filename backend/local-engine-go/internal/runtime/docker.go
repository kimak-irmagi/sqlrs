package runtime

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
)

var (
	cmdStdoutPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return cmd.StdoutPipe() }
	cmdStderrPipe = func(cmd *exec.Cmd) (io.ReadCloser, error) { return cmd.StderrPipe() }
	cmdStart      = func(cmd *exec.Cmd) error { return cmd.Start() }
	cmdWait       = func(cmd *exec.Cmd) error { return cmd.Wait() }
	execLookPath  = exec.LookPath
	osStat        = os.Stat
	ensureMountFn = ensureStateStoreMount
)

const (
	defaultDockerBinary    = "docker"
	dockerHostPathStyleEnv = "SQLRS_DOCKER_HOST_PATH_STYLE"
	dockerHostPathLinux    = "linux"
)

type Options struct {
	ManagedSecrets *instanceaccess.Secrets
	ProtectedRoot  string
	Binary         string
	Runner         commandRunner
}

type DockerUnavailableError struct {
	Message string
}

func (e DockerUnavailableError) Error() string {
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		return "docker daemon unavailable"
	}
	return msg
}

type commandRunner interface {
	Run(ctx context.Context, name string, args []string, stdin *string) (string, error)
}

type streamingRunner interface {
	RunStreaming(ctx context.Context, name string, args []string, stdin *string, sink LogSink) (string, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args []string, stdin *string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}
	hideWindow(cmd)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (execRunner) RunStreaming(ctx context.Context, name string, args []string, stdin *string, sink LogSink) (string, error) {
	if sink == nil {
		return execRunner{}.Run(ctx, name, args, stdin)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}
	hideWindow(cmd)
	stdout, err := cmdStdoutPipe(cmd)
	if err != nil {
		return "", err
	}
	stderr, err := cmdStderrPipe(cmd)
	if err != nil {
		return "", err
	}
	if err := cmdStart(cmd); err != nil {
		return "", err
	}

	lineCh := make(chan string, 16)
	var wg sync.WaitGroup
	readPipe := func(reader io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lineCh <- strings.TrimRight(scanner.Text(), "\r")
		}
	}
	wg.Add(2)
	go readPipe(stdout)
	go readPipe(stderr)
	go func() {
		wg.Wait()
		close(lineCh)
	}()

	var output bytes.Buffer
	for line := range lineCh {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			sink(trimmed)
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	err = cmdWait(cmd)
	return output.String(), err
}

type DockerRuntime struct {
	managedContainers sync.Map
	managedSecrets    *instanceaccess.Secrets
	protectedRoot     string
	binary            string
	runner            commandRunner
}

func NewDocker(opts Options) *DockerRuntime {
	binary := strings.TrimSpace(opts.Binary)
	if binary == "" {
		binary = defaultDockerBinary
	}
	if runtime.GOOS == "linux" && binary == defaultDockerBinary {
		if path, err := execLookPath(binary); err == nil {
			if strings.HasSuffix(strings.ToLower(path), ".exe") {
				if _, err := osStat("/usr/bin/docker"); err == nil {
					binary = "/usr/bin/docker"
				}
			}
		}
	}
	runner := opts.Runner
	if runner == nil {
		runner = execRunner{}
	}
	return &DockerRuntime{binary: binary, runner: runner, managedSecrets: opts.ManagedSecrets, protectedRoot: opts.ProtectedRoot}
}

func (r *DockerRuntime) InitBase(ctx context.Context, imageID string, dataDir string) error {
	if strings.TrimSpace(imageID) == "" {
		return fmt.Errorf("image id is required")
	}
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("data dir is required")
	}
	if err := r.ensureDataDirOwner(ctx, imageID, dataDir); err != nil {
		return err
	}
	if ok, err := r.pgVersionReady(ctx, imageID, dataDir); err != nil {
		return err
	} else if ok {
		if err := r.ensureHostReadableDataDir(ctx, imageID, dataDir); err != nil {
			return err
		}
		return ensureHostAuth(dataDir)
	}
	args := []string{
		"run", "--rm",
		"-u", "postgres",
		"-v", dockerBindSpec(dataDir, PostgresDataDirRoot, false),
		imageID,
		"initdb",
		"--username=sqlrs",
		"--auth=trust",
		"--auth-host=trust",
		"--auth-local=trust",
		"-D", PostgresDataDir,
	}
	output, err := r.run(ctx, args, nil)
	if err != nil {
		if isDockerUnavailable(err) {
			return fmt.Errorf("docker is not running: %w", err)
		}
		if isInitdbPermissionOutput(output) || isInitdbPermissionOutput(err.Error()) {
			return fmt.Errorf("initdb failed: data directory permissions are not supported on this filesystem; use WSL2/ext4 or a docker volume")
		}
		if ok, checkErr := r.pgVersionReady(ctx, imageID, dataDir); checkErr == nil && ok {
			return nil
		}
		return fmt.Errorf("initdb failed: %w", err)
	}
	if ok, checkErr := r.pgVersionReady(ctx, imageID, dataDir); checkErr == nil && ok {
		if err := r.ensureHostReadableDataDir(ctx, imageID, dataDir); err != nil {
			return err
		}
		return ensureHostAuth(dataDir)
	} else if checkErr != nil {
		return checkErr
	}
	return fmt.Errorf("initdb did not produce PG_VERSION (check docker WSL integration and store path)")
}

func (r *DockerRuntime) pgVersionReady(ctx context.Context, imageID string, dataDir string) (bool, error) {
	if hasPGVersion(dataDir) {
		return true, nil
	}
	return r.hasPGVersionInContainer(ctx, imageID, dataDir)
}

func (r *DockerRuntime) hasPGVersionInContainer(ctx context.Context, imageID string, dataDir string) (bool, error) {
	args := []string{
		"run", "--rm",
		"-v", dockerBindSpec(dataDir, PostgresDataDirRoot, false),
		imageID,
		"test", "-f", filepath.ToSlash(filepath.Join(PostgresDataDirRoot, "pgdata", "PG_VERSION")),
	}
	_, err := r.run(ctx, args, nil)
	if err != nil {
		if isDockerUnavailable(err) {
			return false, fmt.Errorf("docker is not running: %w", err)
		}
		return false, nil
	}
	return true, nil
}

func hasPGVersion(dataDir string) bool {
	if strings.TrimSpace(dataDir) == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); err == nil {
		return true
	}
	rel := strings.TrimPrefix(PostgresDataDir, PostgresDataDirRoot)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return false
	}
	pgdataDir := filepath.Join(dataDir, filepath.FromSlash(rel))
	if _, err := os.Stat(filepath.Join(pgdataDir, "PG_VERSION")); err == nil {
		return true
	}
	return false
}

func ensureHostAuth(dataDir string) error {
	pgdataDir := pgDataHostDir(dataDir)
	if strings.TrimSpace(pgdataDir) == "" {
		return fmt.Errorf("data dir is required")
	}
	path := filepath.Join(pgdataDir, "pg_hba.conf")
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if errors.Is(err, os.ErrPermission) {
			// Some runtimes initialize pgdata with container-only ownership (e.g. uid 999, mode 0700).
			// In that case host-side patching is not possible, but initdb host auth settings still apply.
			return nil
		}
		return err
	}
	existing := string(content)
	needsIPv4 := !strings.Contains(existing, "0.0.0.0/0")
	needsIPv6 := !strings.Contains(existing, "::/0")
	if !needsIPv4 && !needsIPv6 {
		return nil
	}
	var b strings.Builder
	if len(existing) > 0 && !strings.HasSuffix(existing, "\n") {
		b.WriteString("\n")
	}
	if needsIPv4 {
		b.WriteString("host all all 0.0.0.0/0 trust\n")
	}
	if needsIPv6 {
		b.WriteString("host all all ::/0 trust\n")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return nil
		}
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(b.String()); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return nil
		}
		return err
	}
	return nil
}

func (r *DockerRuntime) ensureContainerHostAuth(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("container id is required")
	}

	script := fmt.Sprintf(
		"set -e; f=%q; "+
			"[ -f \"$f\" ] || exit 0; "+
			"grep -Fq \"0.0.0.0/0\" \"$f\" || echo \"host all all 0.0.0.0/0 trust\" >> \"$f\"; "+
			"grep -Fq \"::/0\" \"$f\" || echo \"host all all ::/0 trust\" >> \"$f\"",
		filepath.ToSlash(filepath.Join(PostgresDataDir, "pg_hba.conf")),
	)
	_, err := r.Exec(ctx, containerID, ExecRequest{
		User: "postgres",
		Args: []string{"sh", "-c", script},
	})
	if err != nil {
		if isDockerUnavailable(err) {
			return fmt.Errorf("docker is not running: %w", err)
		}
		return fmt.Errorf("pg_hba.conf update failed: %w", err)
	}
	return nil
}

func pgDataHostDir(dataDir string) string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return ""
	}
	rel := strings.TrimPrefix(PostgresDataDir, PostgresDataDirRoot)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return dataDir
	}
	return filepath.Join(dataDir, filepath.FromSlash(rel))
}

func (r *DockerRuntime) ResolveImage(ctx context.Context, imageID string) (string, error) {
	imageID = strings.TrimSpace(imageID)
	if imageID == "" {
		return "", fmt.Errorf("image id is required")
	}
	if strings.Contains(imageID, "@") {
		return imageID, nil
	}
	resolved, err := r.inspectImageDigest(ctx, imageID)
	if err == nil && strings.TrimSpace(resolved) != "" {
		return strings.TrimSpace(resolved), nil
	}
	if err != nil && isDockerUnavailable(err) {
		return "", fmt.Errorf("docker is not running: %w", err)
	}
	if _, pullErr := r.run(ctx, []string{"pull", imageID}, nil); pullErr != nil {
		if isDockerUnavailable(pullErr) {
			return "", fmt.Errorf("docker is not running: %w", pullErr)
		}
		return "", fmt.Errorf("docker pull failed: %w", pullErr)
	}
	resolved, err = r.inspectImageDigest(ctx, imageID)
	if err != nil {
		if isDockerUnavailable(err) {
			return "", fmt.Errorf("docker is not running: %w", err)
		}
		return "", fmt.Errorf("docker inspect failed: %w", err)
	}
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return "", fmt.Errorf("image digest is empty")
	}
	return resolved, nil
}

func (r *DockerRuntime) inspectImageDigest(ctx context.Context, imageID string) (string, error) {
	out, err := r.run(ctx, []string{"image", "inspect", "--format", "{{index .RepoDigests 0}}", imageID}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (r *DockerRuntime) ensureDataDirOwner(ctx context.Context, imageID string, dataDir string) error {
	ensureHostDataDirAccess(dataDir)
	args := []string{
		"run", "--rm",
		"-v", dockerBindSpec(dataDir, PostgresDataDirRoot, false),
		imageID,
		"mkdir", "-p", PostgresDataDir,
	}
	if err := r.runPermissionCommand(ctx, args); err != nil {
		return err
	}
	args = []string{
		"run", "--rm",
		"-v", dockerBindSpec(dataDir, PostgresDataDirRoot, false),
		imageID,
		"chown", "-R", "postgres:postgres", PostgresDataDir,
	}
	if err := r.runPermissionCommand(ctx, args); err != nil {
		return err
	}
	args = []string{
		"run", "--rm",
		"-v", dockerBindSpec(dataDir, PostgresDataDirRoot, false),
		imageID,
		"chmod", "-R", "0700", PostgresDataDir,
	}
	if err := r.runPermissionCommand(ctx, args); err != nil {
		return err
	}
	return nil
}

func (r *DockerRuntime) runPermissionCommand(ctx context.Context, args []string) error {
	output, err := r.run(ctx, args, nil)
	if err != nil {
		if isDockerUnavailable(err) {
			return fmt.Errorf("docker is not running: %w", err)
		}
		if isInitdbPermissionOutput(output) || isInitdbPermissionOutput(err.Error()) {
			return fmt.Errorf("data directory permissions are not supported on this filesystem; use WSL2/ext4 or a docker volume")
		}
		return fmt.Errorf("data directory setup failed: %w", err)
	}
	return nil
}

func (r *DockerRuntime) Start(ctx context.Context, req StartRequest) (Instance, error) {
	if strings.TrimSpace(req.ImageID) == "" {
		return Instance{}, fmt.Errorf("image id is required")
	}
	if strings.TrimSpace(req.DataDir) == "" {
		return Instance{}, fmt.Errorf("data dir is required")
	}
	if err := r.ensureDataDirOwner(ctx, req.ImageID, req.DataDir); err != nil {
		return Instance{}, err
	}
	args := []string{
		"run", "-d", "--rm",
		"-p", "5432",
		"-v", dockerBindSpec(req.DataDir, PostgresDataDirRoot, false),
		"-e", "PGDATA=" + PostgresDataDir,
		"-e", "POSTGRES_HOST_AUTH_METHOD=trust",
	}
	for _, mount := range req.Mounts {
		if strings.TrimSpace(mount.HostPath) == "" || strings.TrimSpace(mount.ContainerPath) == "" {
			continue
		}
		args = append(args, "-v", dockerBindSpec(mount.HostPath, mount.ContainerPath, mount.ReadOnly))
	}
	if strings.TrimSpace(req.Name) != "" {
		args = append(args, "--name", req.Name)
	}
	args = append(args, req.ImageID, "sleep", "infinity")
	out, err := r.run(ctx, args, nil)
	if err != nil {
		if isDockerUnavailable(err) {
			return Instance{}, fmt.Errorf("docker is not running: %w", err)
		}
		return Instance{}, fmt.Errorf("docker run failed: %w", err)
	}
	containerID := strings.TrimSpace(out)
	if containerID == "" {
		return Instance{}, fmt.Errorf("docker run returned empty container id")
	}

	ok, err := r.pgVersionReadyInContainer(ctx, containerID)
	if err != nil {
		_ = r.Stop(ctx, containerID)
		return Instance{}, err
	}
	if !ok {
		if !req.AllowInitdb {
			_ = r.Stop(ctx, containerID)
			return Instance{}, fmt.Errorf("postgres data directory is not initialized (missing PG_VERSION)")
		}
		if err := r.initdbInContainer(ctx, containerID); err != nil {
			_ = r.Stop(ctx, containerID)
			return Instance{}, err
		}
	}
	if err := ensureHostAuth(req.DataDir); err != nil {
		_ = r.Stop(ctx, containerID)
		return Instance{}, err
	}
	if err := r.ensureContainerHostAuth(ctx, containerID); err != nil {
		_ = r.Stop(ctx, containerID)
		return Instance{}, err
	}

	if _, err := r.Exec(ctx, containerID, ExecRequest{
		User: "postgres",
		Args: []string{
			"pg_ctl", "-D", PostgresDataDir,
			"-o", "-c listen_addresses=* -p 5432",
			"-w", "start",
		},
	}); err != nil {
		_ = r.Stop(ctx, containerID)
		return Instance{}, fmt.Errorf("postgres start failed: %w", err)
	}

	if err := r.WaitForReady(ctx, containerID, 15*time.Second); err != nil {
		_ = r.Stop(ctx, containerID)
		return Instance{}, err
	}

	portOut, err := r.run(ctx, []string{"port", containerID, "5432/tcp"}, nil)
	if err != nil {
		_ = r.Stop(ctx, containerID)
		return Instance{}, fmt.Errorf("docker port failed: %w", err)
	}
	port, err := parseHostPort(portOut)
	if err != nil {
		_ = r.Stop(ctx, containerID)
		return Instance{}, err
	}
	return Instance{
		ID:   containerID,
		Host: "127.0.0.1",
		Port: port,
	}, nil
}

func (r *DockerRuntime) pgVersionReadyInContainer(ctx context.Context, id string) (bool, error) {
	_, err := r.Exec(ctx, id, ExecRequest{
		Args: []string{"test", "-f", filepath.ToSlash(filepath.Join(PostgresDataDirRoot, "pgdata", "PG_VERSION"))},
	})
	if err == nil {
		return true, nil
	}
	if isDockerUnavailable(err) {
		return false, fmt.Errorf("docker is not running: %w", err)
	}
	return false, nil
}

func (r *DockerRuntime) initdbInContainer(ctx context.Context, id string) error {
	output, err := r.Exec(ctx, id, ExecRequest{
		User: "postgres",
		Args: []string{
			"initdb",
			"--username=sqlrs",
			"--auth=trust",
			"--auth-host=trust",
			"--auth-local=trust",
			"-D", PostgresDataDir,
		},
	})
	if err != nil {
		if isDockerUnavailable(err) {
			return fmt.Errorf("docker is not running: %w", err)
		}
		if isInitdbPermissionOutput(output) || isInitdbPermissionOutput(err.Error()) {
			return fmt.Errorf("initdb failed: data directory permissions are not supported on this filesystem; use WSL2/ext4 or a docker volume")
		}
		return fmt.Errorf("initdb failed: %w", err)
	}
	ok, err := r.pgVersionReadyInContainer(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("initdb did not produce PG_VERSION (check docker WSL integration and store path)")
	}
	return nil
}

func (r *DockerRuntime) Stop(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if _, managed := r.managedContainers.Load(id); managed {
		_, err := r.privateRun(ctx, []string{"exec", id, "sh", "-c", `set -e; gosu postgres pg_ctl -D "$PGDATA" -m fast -w stop >/dev/null 2>&1 || [ ! -f "$PGDATA/postmaster.pid" ]; chmod -R a+rwX "$PGDATA"`}, nil)
		if err != nil {
			return err
		}
	}
	// Instances are ephemeral; force-remove is faster than graceful stop and
	// avoids waiting on PostgreSQL shutdown during cleanup paths.
	output, err := r.run(ctx, []string{"rm", "-f", id}, nil)
	if err != nil && isDockerNotFoundOutput(output, err) {
		return nil
	}
	return err
}

func (r *DockerRuntime) Exec(ctx context.Context, id string, req ExecRequest) (string, error) {
	if req.Secret != nil {
		return r.execManaged(ctx, id, req)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("container id is required")
	}
	args := []string{"exec"}
	if strings.TrimSpace(req.User) != "" {
		args = append(args, "-u", req.User)
	}
	if strings.TrimSpace(req.Dir) != "" {
		args = append(args, "-w", req.Dir)
	}
	for key, value := range req.Env {
		if strings.TrimSpace(key) == "" {
			continue
		}
		args = append(args, "-e", key+"="+value)
	}
	if req.Stdin != nil {
		args = append(args, "-i")
	}
	args = append(args, id)
	args = append(args, req.Args...)
	return r.run(ctx, args, req.Stdin)
}

func (r *DockerRuntime) WaitForReady(ctx context.Context, id string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		out, err := r.Exec(ctx, id, ExecRequest{
			User: "postgres",
			Args: []string{"pg_isready", "-U", "sqlrs", "-d", "postgres", "-h", "127.0.0.1", "-p", "5432"},
		})
		if err == nil && strings.Contains(out, "accepting connections") {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("postgres readiness failed: %w", err)
			}
			return fmt.Errorf("postgres readiness timed out")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (r *DockerRuntime) run(ctx context.Context, args []string, stdin *string) (string, error) {
	if ensureMountFn != nil {
		if err := ensureMountFn(); err != nil {
			return "", err
		}
	}
	sink := logSinkFromContext(ctx)
	if sink != nil {
		if runner, ok := r.runner.(streamingRunner); ok {
			output, err := runner.RunStreaming(ctx, r.binary, args, stdin, sink)
			if err != nil {
				return output, wrapDockerError(err, output)
			}
			return output, nil
		}
	}
	output, err := r.runner.Run(ctx, r.binary, args, stdin)
	if sink != nil {
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			sink(line)
		}
	}
	if err != nil {
		return output, wrapDockerError(err, output)
	}
	return output, nil
}

func (r *DockerRuntime) RunContainer(ctx context.Context, req RunRequest) (string, error) {
	imageID := strings.TrimSpace(req.ImageID)
	if imageID == "" {
		return "", fmt.Errorf("image id is required")
	}
	args := []string{"run", "--rm"}
	if strings.TrimSpace(req.Name) != "" {
		args = append(args, "--name", strings.TrimSpace(req.Name))
	}
	if strings.TrimSpace(req.User) != "" {
		args = append(args, "-u", strings.TrimSpace(req.User))
	}
	if strings.TrimSpace(req.Dir) != "" {
		args = append(args, "-w", strings.TrimSpace(req.Dir))
	}
	if strings.TrimSpace(req.Network) != "" {
		args = append(args, "--network", strings.TrimSpace(req.Network))
	}
	for key, value := range req.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		args = append(args, "-e", key+"="+value)
	}
	for _, mount := range req.Mounts {
		if strings.TrimSpace(mount.HostPath) == "" || strings.TrimSpace(mount.ContainerPath) == "" {
			continue
		}
		args = append(args, "-v", dockerBindSpec(mount.HostPath, mount.ContainerPath, mount.ReadOnly))
	}
	args = append(args, imageID)
	if len(req.Args) > 0 {
		args = append(args, req.Args...)
	}
	output, err := r.run(ctx, args, nil)
	if err != nil {
		if isDockerUnavailable(err) {
			return output, fmt.Errorf("docker is not running: %w", err)
		}
		return output, fmt.Errorf("docker run failed: %w", err)
	}
	return output, nil
}

func parseHostPort(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("docker port output is empty")
	}
	lines := strings.Split(value, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		portStr := parts[len(parts)-1]
		port, err := strconv.Atoi(strings.TrimSpace(portStr))
		if err != nil {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("cannot parse docker port output: %q", value)
}

func dockerBindSpec(hostPath, containerPath string, readOnly bool) string {
	spec := fmt.Sprintf("%s:%s", normalizeDockerHostPath(hostPath), containerPath)
	if readOnly {
		spec += ":ro"
	}
	return spec
}

func normalizeDockerHostPath(hostPath string) string {
	hostPath = strings.TrimSpace(hostPath)
	if hostPath == "" || !useLinuxMountPathStyle() {
		return hostPath
	}
	if converted, ok := windowsWSLUNCPathToLinux(hostPath); ok {
		return converted
	}
	if converted, ok := windowsDrivePathToLinux(hostPath); ok {
		return converted
	}
	return hostPath
}

func windowsDrivePathToLinux(hostPath string) (string, bool) {
	if len(hostPath) < 2 || hostPath[1] != ':' {
		return "", false
	}
	drive := hostPath[0]
	if !((drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z')) {
		return "", false
	}
	rest := strings.TrimLeft(hostPath[2:], `\/`)
	rest = strings.ReplaceAll(rest, `\`, "/")
	prefix := "/mnt/" + strings.ToLower(string(drive))
	if rest == "" {
		return prefix, true
	}
	return prefix + "/" + rest, true
}

func windowsWSLUNCPathToLinux(hostPath string) (string, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(hostPath), "/", `\`)
	lower := strings.ToLower(normalized)
	prefixes := []string{`\\wsl$\`, `\\wsl.localhost\`}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		rest := normalized[len(prefix):]
		parts := strings.SplitN(rest, `\`, 2)
		if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
			return "", false
		}
		subPath := strings.TrimLeft(parts[1], `\`)
		if subPath == "" {
			return "/", true
		}
		subPath = strings.ReplaceAll(subPath, `\`, "/")
		return "/" + strings.TrimLeft(subPath, "/"), true
	}
	return "", false
}

func wrapDockerError(err error, output string) error {
	trimmed := strings.TrimSpace(output)
	if isDockerUnavailableOutput(trimmed, err) {
		return DockerUnavailableError{Message: dockerUnavailableHint(trimmed, err)}
	}
	if trimmed != "" {
		return fmt.Errorf("%w: %s", err, trimmed)
	}
	return err
}

func dockerUnavailableHint(output string, err error) string {
	combined := strings.TrimSpace(output)
	if err != nil {
		combined = strings.TrimSpace(combined + " " + err.Error())
	}
	combined = strings.ToLower(combined)
	if strings.Contains(combined, "dockerdesktoplinuxengine") || strings.Contains(combined, "npipe") {
		return "start Docker Desktop and retry"
	}
	if strings.Contains(combined, "docker.sock") || strings.Contains(combined, "unix://") {
		return "start the Docker daemon and retry"
	}
	return "start Docker and retry"
}

func isDockerUnavailable(err error) bool {
	var unavailable DockerUnavailableError
	return errors.As(err, &unavailable)
}

func isDockerUnavailableOutput(output string, err error) bool {
	if err == nil && strings.TrimSpace(output) == "" {
		return false
	}
	combined := strings.TrimSpace(output)
	if err != nil {
		combined = strings.TrimSpace(combined + " " + err.Error())
	}
	combined = strings.ToLower(combined)
	if combined == "" {
		return false
	}
	if strings.Contains(combined, "cannot connect to the docker daemon") {
		return true
	}
	if strings.Contains(combined, "failed to connect to the docker api") {
		return true
	}
	if strings.Contains(combined, "is the docker daemon running") {
		return true
	}
	if strings.Contains(combined, "dockerdesktoplinuxengine") {
		return true
	}
	if strings.Contains(combined, "npipe") && strings.Contains(combined, "docker") && strings.Contains(combined, "pipe") {
		return true
	}
	return false
}

func isDockerNotFoundOutput(output string, err error) bool {
	combined := strings.ToLower(strings.TrimSpace(output))
	if err != nil {
		combined = strings.TrimSpace(combined + " " + err.Error())
	}
	if combined == "" {
		return false
	}
	if strings.Contains(combined, "no such container") {
		return true
	}
	if strings.Contains(combined, "is not running") && strings.Contains(combined, "container") {
		return true
	}
	return false
}

func isInitdbPermissionOutput(output string) bool {
	combined := strings.ToLower(strings.TrimSpace(output))
	if combined == "" {
		return false
	}
	if strings.Contains(combined, "initdb: error: could not change permissions of directory") {
		return true
	}
	if strings.Contains(combined, "chown") && strings.Contains(combined, "operation not permitted") {
		return true
	}
	if strings.Contains(combined, "chmod") && strings.Contains(combined, "operation not permitted") {
		return true
	}
	if strings.Contains(combined, "operation not permitted") && strings.Contains(combined, "permissions") && strings.Contains(combined, "data") {
		return true
	}
	if strings.Contains(combined, "operation not permitted") && strings.Contains(combined, PostgresDataDir) {
		return true
	}
	return false
}
