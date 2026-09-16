//go:build managedintegration

package dbms

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type nativeCancellationBoundary struct {
	context.Context
	cancel    context.CancelFunc
	remaining atomic.Int32
}

func (c *nativeCancellationBoundary) Done() <-chan struct{} {
	if c.remaining.Add(-1) == 0 {
		c.cancel()
	}
	return c.Context.Done()
}

// This explicitly selected suite requires Docker and the official postgres:17
// image. It does not skip when unavailable and never touches user store data.
func TestManagedNativePostgres17(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	run := func(args ...string) string {
		t.Helper()
		output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture Docker command %s failed", args[0])
		}
		return strings.TrimSpace(string(output))
	}
	image := run("image", "inspect", "postgres:17", "--format", "{{index .RepoDigests 0}}")
	if !strings.HasPrefix(image, "postgres@sha256:") {
		t.Fatal("official immutable postgres image required")
	}
	request := managedAccessFixture(t)
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		t.Fatal("fixture entropy unavailable")
	}
	name := "sqlrs-managed-proof-test-" + hex.EncodeToString(entropy[:])
	container := run("run", "--detach", "--rm", "--name", name, "--publish", "127.0.0.1::5432", "--publish", "[::1]::5432", "--env", "POSTGRES_USER="+request.Binding.Username, "--env", "POSTGRES_DB=postgres", "--env", "POSTGRES_HOST_AUTH_METHOD=trust", image)
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if exec.CommandContext(cleanup, "docker", "rm", "-f", container).Run() != nil {
			t.Error("fixture container cleanup failed")
		}
	})
	request.Binding.RuntimeRef = container
	ports := map[string]uint16{}
	for _, address := range strings.Split(run("port", container, "5432/tcp"), "\n") {
		host, portText, err := net.SplitHostPort(strings.TrimSpace(address))
		if err != nil {
			t.Fatal("fixture port unavailable")
		}
		port, err := strconv.ParseUint(portText, 10, 16)
		if err != nil {
			t.Fatal("fixture port invalid")
		}
		ports[host] = uint16(port)
	}
	if ports["127.0.0.1"] == 0 || ports["::1"] == 0 {
		t.Fatal("both loopback IP families must be published")
	}
	request.Port = ports["127.0.0.1"]
	config, err := managedConnectionConfig(request, nil)
	if err != nil {
		t.Fatal(err)
	}
	config.RequireAuth = "" // Disposable fixture setup initially uses entrypoint trust.
	var admin *pgconn.PgConn
	for ctx.Err() == nil {
		admin, err = pgconn.ConnectConfig(ctx, config)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("fixture PostgreSQL did not become ready")
	}
	defer closeManagedConnection(admin)
	sql := func(statement string) {
		t.Helper()
		if _, err := admin.Exec(ctx, statement).ReadAll(); err != nil {
			t.Fatal("fixture SQL failed")
		}
	}
	setHBA := func(method, replicationMethod string) {
		t.Helper()
		command := exec.CommandContext(ctx, "docker", "exec", "-i", container, "sh", "-c", `cat > "$PGDATA/pg_hba.conf"`)
		command.Stdin = strings.NewReader("local all all trust\nhost all all 0.0.0.0/0 " + method + "\nhost all all ::/0 " + method + "\nhost replication all 0.0.0.0/0 " + replicationMethod + "\nhost replication all ::/0 " + replicationMethod + "\n")
		if command.Run() != nil {
			t.Fatal("fixture HBA setup failed")
		}
		sql("SELECT pg_reload_conf()")
		// PostgreSQL's SIGHUP is asynchronous. Positive native proof below also
		// checks that SCRAM is actually negotiated after the reload.
		time.Sleep(150 * time.Millisecond)
	}
	if _, err := VerifyManagedAccess(ctx, request); err != ErrManagedAccessUnavailable {
		t.Fatal("trust-only access was accepted")
	}
	// An inherited trust rule must also fail the negative-probe boundary even
	// if it appears after an earlier positive proof (HBA configuration drift).
	if err := verifyRejectedCredential(ctx, config); err != ErrManagedAccessUnavailable {
		t.Fatal("trust was counted as rejected password")
	}
	sql("ALTER ROLE " + request.Binding.Username + " PASSWORD '" + request.Password + "'")
	setHBA("scram-sha-256", "trust")
	if _, err := VerifyManagedAccess(ctx, request); err != ErrManagedAccessUnavailable {
		t.Fatal("replication trust bypass was accepted")
	}
	setHBA("scram-sha-256", "scram-sha-256")
	proof, err := VerifyManagedAccess(ctx, request)
	if err != nil || proof.Binding != request.Binding {
		t.Fatalf("native managed proof failed: %v", err)
	}
	ipv6 := request
	ipv6.Host, ipv6.Port = "::1", ports["::1"]
	if proof, err := VerifyManagedAccess(ctx, ipv6); err != nil || proof.Binding != request.Binding {
		t.Fatalf("IPv6 managed SQL/replication proof failed: %v", err)
	}
	wrong := request
	wrong.Password = strings.Repeat("b", 64)
	if _, err := VerifyManagedAccess(ctx, wrong); err != ErrManagedAccessUnavailable {
		t.Fatal("wrong credential accepted")
	}
	// Both historical names remain ordinary application roles.
	sql("CREATE ROLE postgres LOGIN; CREATE ROLE sqlrs LOGIN; SET ROLE postgres; RESET ROLE; DROP ROLE postgres; DROP ROLE sqlrs")
	if _, err := VerifyManagedAccess(ctx, request); err != nil {
		t.Fatal("application role lifecycle damaged managed access")
	}
	previous := request
	request.Password = strings.Repeat("d", 64)
	for i := 0; i < 2; i++ {
		if err := EnsureManagedPassword(ctx, previous, request.Password); err != nil {
			t.Fatal("native password activation/retry", err)
		}
	}
	if _, err := VerifyManagedAccess(ctx, previous); err != ErrManagedAccessUnavailable {
		t.Fatal("previous password remained usable")
	}
	// Cancellation can arrive before or after ALTER has reached PostgreSQL. A
	// retry of the same version must succeed with no guessed replacement secret.
	for boundary := int32(1); boundary <= 256; boundary++ {
		candidate := strings.Repeat("e", 64)
		if candidate == request.Password {
			candidate = strings.Repeat("d", 64)
		}
		base, cancelAttempt := context.WithCancel(ctx)
		attempt := &nativeCancellationBoundary{Context: base, cancel: cancelAttempt}
		attempt.remaining.Store(boundary)
		err := EnsureManagedPassword(attempt, request, candidate)
		cancelAttempt()
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("native cancellation boundary %d: %v", boundary, err)
		}
		if err := EnsureManagedPassword(ctx, request, candidate); err != nil {
			t.Fatalf("retry after native cancellation boundary %d: %v", boundary, err)
		}
		request.Password = candidate
	}
	sql("CREATE ROLE fixture_recovery_admin LOGIN SUPERUSER PASSWORD '" + strings.Repeat("c", 64) + "'")
	recoveryConfig := config.Copy()
	recoveryConfig.User = "fixture_recovery_admin"
	recoveryConfig.Password = strings.Repeat("c", 64)
	recovery, err := pgconn.ConnectConfig(ctx, recoveryConfig)
	if err != nil {
		t.Fatal("fixture recovery connection failed")
	}
	defer closeManagedConnection(recovery)
	// ALTER ROLE protects the bootstrap administrator's SUPERUSER attribute.
	// A superuser recipe can still damage pg_authid directly; exercise that
	// reachable mutation without requiring an impossible bootstrap-role drop.
	if _, err := recovery.Exec(ctx, "UPDATE pg_catalog.pg_authid SET rolsuper=false WHERE rolname='"+request.Binding.Username+"'").ReadAll(); err != nil {
		t.Fatal("fixture privilege mutation failed")
	}
	if _, err := VerifyManagedAccess(ctx, request); err != ErrManagedAccessUnavailable {
		t.Fatal("lost administrative privilege was accepted")
	}
	if err := EnsureManagedPassword(ctx, request, strings.Repeat("e", 64)); err != ErrManagedAccessUnavailable {
		t.Fatal("activation repaired damaged role", err)
	}
	// Verification must not silently restore the altered privilege.
	rows := recovery.ExecParams(ctx, "SELECT rolsuper FROM pg_roles WHERE rolname=$1", [][]byte{[]byte(request.Binding.Username)}, nil, nil, nil)
	if !rows.NextRow() || string(rows.Values()[0]) != "f" {
		t.Fatal("verification repaired managed role")
	}
	if _, err := rows.Close(); err != nil {
		t.Fatal("fixture role observation failed")
	}
	if logs := run("logs", container); strings.Contains(logs, request.Password) || strings.Contains(logs, previous.Password) {
		t.Fatal("managed credential leaked into container logs")
	}
}
