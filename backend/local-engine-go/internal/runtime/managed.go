package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
)

var ErrManagedRuntime = errors.New("managed PostgreSQL runtime operation failed")

// ManagedInitRequest binds empty-target initialization to a persisted lineage
// and protected bootstrap reference. No password is part of the runtime DTO.
type ManagedInitRequest struct {
	ImageID   string
	DataDir   string
	Identity  managedidentity.IdentityBinding
	Bootstrap instanceaccess.SecretBinding
}

// ManagedStartRequest starts an already initialized, assigned writable clone.
// PhysicalIdentity comes from the durable clone operation, never its SQL role.
type ManagedStartRequest struct {
	StartRequest
	Identity         managedidentity.IdentityBinding
	PhysicalIdentity string
}

type ManagedRuntime interface {
	InitManagedBase(context.Context, ManagedInitRequest) error
	StartManaged(context.Context, ManagedStartRequest) (Instance, error)
}

// privateRun deliberately bypasses recipe log sinks and bounds driver output.
func (r *DockerRuntime) privateRun(ctx context.Context, args []string, input *string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if ensureMountFn != nil {
		if err := ensureMountFn(); err != nil {
			return "", ErrManagedRuntime
		}
	}
	out, err := r.runner.Run(ctx, r.binary, args, input)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrManagedRuntime
	}
	return out, nil
}

func (r *DockerRuntime) managedPaths(data string, mounts []Mount) error {
	if !filepath.IsAbs(data) || !filepath.IsAbs(r.protectedRoot) {
		return ErrManagedRuntime
	}
	for _, path := range append([]Mount{{HostPath: data}}, mounts...) {
		if !filepath.IsAbs(path.HostPath) || pathsOverlap(path.HostPath, r.protectedRoot) {
			return ErrManagedRuntime
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (r *DockerRuntime) InitManagedBase(ctx context.Context, req ManagedInitRequest) error {
	if req.Identity.Validate() != nil || req.Bootstrap.Validate() != nil || req.Bootstrap.OwnerRef != req.Identity.LineageRef || req.Bootstrap.IdentityDigest != req.Identity.IdentityDigest || req.Bootstrap.Purpose != "bootstrap" || req.ImageID == "" || r.managedSecrets == nil || r.managedPaths(req.DataDir, nil) != nil {
		return ErrManagedRuntime
	}
	secret, err := r.managedSecrets.Resolve(req.Bootstrap)
	if err != nil {
		return ErrManagedRuntime
	}
	// Reject existing contents before chown/initdb. Failure never clears a target.
	script := `set -eu
d=/var/lib/postgresql/data/pgdata
chmod 755 /var/lib/postgresql/data
 if [ -d "$d" ]; then [ -z "$(find "$d" -mindepth 1 -maxdepth 1 -print -quit)" ] || exit 1; fi
 mkdir -p "$d"
 chown postgres:postgres "$d"
chmod 700 "$d"
umask 077
f=/run/managed/password
cat > "$f"
chown postgres:postgres /run/managed "$f"
trap 'rm -f "$f"' EXIT
gosu postgres initdb -D "$d" --username="$1" --auth-local=trust --auth-host=scram-sha-256 --pwfile="$f"
`
	input := secret.Password + "\n"
	_, err = r.privateRun(ctx, []string{"run", "--rm", "-i", "--tmpfs", "/run/managed:rw,noexec,nosuid,mode=0700", "-v", dockerBindSpec(req.DataDir, PostgresDataDirRoot, false), req.ImageID, "sh", "-c", script, "managed-init", req.Identity.Username}, &input)
	if err != nil {
		return err
	}
	// Prepend exact-role SCRAM for local, TCP and physical replication. Keep the
	// generated application rules; neither OS postgres nor database postgres changes.
	hba := managedHBA(req.Identity.Username)
	patch := `set -eu
 f=/var/lib/postgresql/data/pgdata/pg_hba.conf
 t="$f.managed"
 umask 077
 cat > "$t"
 cat "$f" >> "$t"
 printf '\nhost all all 0.0.0.0/0 scram-sha-256\nhost all all ::/0 scram-sha-256\n' >> "$t"
 chown postgres:postgres "$t"
 mv "$t" "$f"
`
	_, err = r.privateRun(ctx, []string{"run", "--rm", "-i", "-v", dockerBindSpec(req.DataDir, PostgresDataDirRoot, false), req.ImageID, "sh", "-c", patch}, &hba)
	if err == nil {
		_, err = r.privateRun(ctx, []string{"run", "--rm", "-v", dockerBindSpec(req.DataDir, PostgresDataDirRoot, false), req.ImageID, "chmod", "-R", "a+rX", PostgresDataDir}, nil)
	}
	return err
}

func managedHBA(username string) string {
	return "# managed-identity.v1\nlocal all " + username + " scram-sha-256\nlocal replication " + username + " scram-sha-256\nhost all " + username + " 0.0.0.0/0 scram-sha-256\nhost all " + username + " ::/0 scram-sha-256\nhost replication " + username + " 0.0.0.0/0 scram-sha-256\nhost replication " + username + " ::/0 scram-sha-256\n"
}

func (r *DockerRuntime) StartManaged(ctx context.Context, req ManagedStartRequest) (Instance, error) {
	binding := managedidentity.RuntimeBinding{RuntimeRef: "pending", PhysicalIdentity: req.PhysicalIdentity, IdentityBinding: req.Identity}
	if binding.Validate() != nil || req.ImageID == "" || req.AllowInitdb || r.managedPaths(req.DataDir, req.Mounts) != nil {
		return Instance{}, ErrManagedRuntime
	}
	args := []string{"run", "-d", "--rm", "-p", "127.0.0.1::5432", "-v", dockerBindSpec(req.DataDir, PostgresDataDirRoot, false), "-e", "PGDATA=" + PostgresDataDir}
	args = append(args, "--label", "sqlrs.managed.identity="+req.Identity.IdentityDigest, "--label", "sqlrs.managed.physical="+req.PhysicalIdentity)
	for _, mount := range req.Mounts {
		if mount.ContainerPath == "" {
			return Instance{}, ErrManagedRuntime
		}
		args = append(args, "-v", dockerBindSpec(mount.HostPath, mount.ContainerPath, mount.ReadOnly))
	}
	if req.Name != "" {
		args = append(args, "--name", req.Name)
	}
	args = append(args, req.ImageID, "sleep", "infinity")
	out, err := r.privateRun(ctx, args, nil)
	if err != nil {
		return Instance{}, err
	}
	id := strings.TrimSpace(out)
	if !managedContainerID.MatchString(id) {
		return Instance{}, ErrManagedRuntime
	}
	success := false
	defer func() {
		if !success {
			_, _ = r.privateRun(context.WithoutCancel(ctx), []string{"rm", "-f", id}, nil)
		}
	}()
	// Never initialize or rewrite HBA on a clone; native proof checks inherited
	// identity/authentication before any recipe, seal or instance publication.
	_, err = r.privateRun(ctx, []string{"exec", id, "chown", "-R", "postgres:postgres", PostgresDataDir}, nil)
	if err != nil {
		return Instance{}, err
	}
	_, err = r.privateRun(ctx, []string{"exec", id, "chmod", "755", PostgresDataDirRoot}, nil)
	if err != nil {
		return Instance{}, err
	}
	_, err = r.privateRun(ctx, []string{"exec", id, "chmod", "-R", "0700", PostgresDataDir}, nil)
	if err != nil {
		return Instance{}, err
	}
	_, err = r.privateRun(ctx, []string{"exec", "-u", "postgres", id, "pg_ctl", "-D", PostgresDataDir, "-o", "-c listen_addresses=* -p 5432", "-w", "start"}, nil)
	if err != nil {
		return Instance{}, err
	}
	out, err = r.privateRun(ctx, []string{"port", id, "5432/tcp"}, nil)
	if err != nil {
		return Instance{}, err
	}
	port, err := parseHostPort(out)
	if err != nil {
		return Instance{}, ErrManagedRuntime
	}
	binding.RuntimeRef = id
	r.managedContainers.Store(id, binding)
	success = true
	return Instance{ID: id, Host: "127.0.0.1", Port: port, Binding: binding}, nil
}

// InspectManaged reconnects only to the exact recorded container and labels.
// Container names and SQL usernames alone never establish runtime ownership.
func (r *DockerRuntime) InspectManaged(ctx context.Context, b managedidentity.RuntimeBinding) (Instance, error) {
	if b.Validate() != nil || !managedContainerID.MatchString(b.RuntimeRef) {
		return Instance{}, ErrManagedRuntime
	}
	out, err := r.privateRun(ctx, []string{"inspect", "--format", `{{.Id}} {{index .Config.Labels "sqlrs.managed.identity"}} {{index .Config.Labels "sqlrs.managed.physical"}} {{.State.Running}}`, b.RuntimeRef}, nil)
	if err != nil || strings.TrimSpace(out) != b.RuntimeRef+" "+b.IdentityDigest+" "+b.PhysicalIdentity+" true" {
		return Instance{}, ErrManagedRuntime
	}
	out, err = r.privateRun(ctx, []string{"port", b.RuntimeRef, "5432/tcp"}, nil)
	if err != nil {
		return Instance{}, err
	}
	port, err := parseHostPort(out)
	if err != nil {
		return Instance{}, ErrManagedRuntime
	}
	r.managedContainers.Store(b.RuntimeRef, b)
	return Instance{ID: b.RuntimeRef, Host: "127.0.0.1", Port: port, Binding: b}, nil
}

var managedVerifierPattern = regexp.MustCompile(`SCRAM-SHA-256\$[0-9]+:[A-Za-z0-9+/=]+\$[A-Za-z0-9+/=]+:[A-Za-z0-9+/=]+`)

// RedactManagedOutput applies before recipe output reaches logs or queue events.
func RedactManagedOutput(output string, password string) string {
	if password != "" {
		output = strings.ReplaceAll(output, password, "[redacted]")
	}
	return managedVerifierPattern.ReplaceAllString(output, "[redacted verifier]")
}

func (r *DockerRuntime) execManaged(ctx context.Context, id string, req ExecRequest) (string, error) {
	if !managedContainerID.MatchString(id) || r.managedSecrets == nil || len(req.Args) == 0 {
		return "", ErrManagedRuntime
	}
	secret, err := r.managedSecrets.Resolve(*req.Secret)
	if err != nil {
		return "", ErrManagedRuntime
	}
	args := []string{"exec", "-i"}
	if req.User != "" {
		args = append(args, "-u", req.User)
	}
	if req.Dir != "" {
		args = append(args, "-w", req.Dir)
	}
	for key, value := range req.Env {
		if strings.HasPrefix(strings.ToUpper(key), "PG") {
			return "", ErrManagedRuntime
		}
		args = append(args, "-e", key+"="+value)
	}
	args = append(args, id, "sh", "-c", `IFS= read -r PGPASSWORD; export PGPASSWORD; exec "$@"`, "managed-exec")
	args = append(args, req.Args...)
	input := secret.Password + "\n"
	if req.Stdin != nil {
		input += *req.Stdin
	}
	var output string
	if streaming, ok := r.runner.(streamingRunner); ok && logSinkFromContext(ctx) != nil {
		sink := logSinkFromContext(ctx)
		output, err = streaming.RunStreaming(ctx, r.binary, args, &input, func(line string) { sink(RedactManagedOutput(line, secret.Password)) })
	} else {
		output, err = r.runner.Run(ctx, r.binary, args, &input)
	}
	output = RedactManagedOutput(output, secret.Password)
	if err != nil {
		if ctx.Err() != nil {
			return output, ctx.Err()
		}
		return output, ErrManagedRuntime
	}
	return output, nil
}
