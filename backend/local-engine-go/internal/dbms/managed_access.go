package dbms

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sqlrs/engine-local/internal/managedidentity"
)

// ErrManagedAccessUnavailable is the only non-cancellation diagnostic exposed
// by native probes; PostgreSQL/driver errors may contain sensitive query data.
var ErrManagedAccessUnavailable = errors.New("managed PostgreSQL access verification failed")

// ManagedConnection is ephemeral, sensitive input. It must not be persisted in
// jobs, events, SQLite, journals or generic logs. Only authorized result assembly
// may encode the password in a DSN. See managed-database-identity-internals.md.
type ManagedConnection struct {
	Binding  managedidentity.RuntimeBinding `json:"-"`
	Host     string                         `json:"-"`
	Port     uint16                         `json:"-"`
	Password string                         `json:"-"`
}

func (ManagedConnection) String() string   { return "[managed PostgreSQL connection]" }
func (ManagedConnection) GoString() string { return "[managed PostgreSQL connection]" }

// ManagedAccessProof is non-secret evidence for the exact live runtime binding.
// The coordinator must recheck operation ownership and deletion state before
// consuming it; the value is not a reusable authorization capability.
type ManagedAccessProof struct {
	Binding managedidentity.RuntimeBinding
}

// VerifyManagedAccess authenticates a fresh native connection using SCRAM,
// verifies the exact administrative identity, then proves rejection of wrong and
// absent passwords. Transport failures never count as rejected credentials.
func VerifyManagedAccess(ctx context.Context, request ManagedConnection) (ManagedAccessProof, error) {
	if err := ctx.Err(); err != nil {
		return ManagedAccessProof{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cfg, err := managedConnectionConfig(request, os.Environ())
	if err != nil {
		return ManagedAccessProof{}, err
	}
	conn, err := pgconn.ConnectConfig(ctx, cfg)
	if err != nil {
		return ManagedAccessProof{}, managedAccessError(ctx)
	}
	valid := managedRoleEvidence(ctx, conn, request.Binding.Username)
	closeManagedConnection(conn)
	if !valid {
		return ManagedAccessProof{}, managedAccessError(ctx)
	}
	// PostgreSQL's "all" database HBA rule excludes physical replication.
	// Prove that the same managed role cannot bypass SCRAM through that route.
	replication := cfg.Copy()
	replication.RuntimeParams = map[string]string{"replication": "true"}
	conn, err = pgconn.ConnectConfig(ctx, replication)
	if err != nil {
		return ManagedAccessProof{}, managedAccessError(ctx)
	}
	_, err = conn.Exec(ctx, "IDENTIFY_SYSTEM").ReadAll()
	closeManagedConnection(conn)
	if err != nil {
		return ManagedAccessProof{}, managedAccessError(ctx)
	}
	for _, route := range []*pgconn.Config{cfg, replication} {
		for _, password := range []string{wrongManagedPassword(request.Password), ""} {
			negative := route.Copy()
			negative.Password = password
			if err := verifyRejectedCredential(ctx, negative); err != nil {
				return ManagedAccessProof{}, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return ManagedAccessProof{}, err
	}
	return ManagedAccessProof{Binding: request.Binding}, nil
}

// wrongManagedPassword chooses a syntactically valid credential guaranteed to
// differ from the actual one. No statistical uniqueness assumption is needed.
func wrongManagedPassword(password string) string {
	wrong := strings.Repeat("0", 64)
	if wrong == password {
		return strings.Repeat("1", 64)
	}
	return wrong
}

// verifyRejectedCredential tests the server's authentication decision, without
// RequireAuth making a permissive trust rule look like rejected credentials.
// It deliberately distinguishes password rejection from transport failure.
func verifyRejectedCredential(ctx context.Context, cfg *pgconn.Config) error {
	negative := cfg.Copy()
	negative.RequireAuth = ""
	conn, err := pgconn.ConnectConfig(ctx, negative)
	if err == nil {
		closeManagedConnection(conn)
		return ErrManagedAccessUnavailable
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "28P01" || postgresError.SeverityUnlocalized != "FATAL" {
		return managedAccessError(ctx)
	}
	return ctx.Err()
}

// managedConnectionConfig never inherits service/pass files or PostgreSQL
// environment defaults. The engine must launch with a clean PG* environment;
// validation fails closed if that contract is not met, as in the shared probe.
func managedConnectionConfig(request ManagedConnection, environment []string) (*pgconn.Config, error) {
	address := net.ParseIP(request.Host)
	if request.Binding.Validate() != nil || address == nil || !managedEndpointAllowed(request.Host, environment) || request.Port == 0 || len(request.Password) != 64 || strings.Trim(request.Password, "0123456789abcdef") != "" {
		return nil, ErrManagedAccessUnavailable
	}
	for _, entry := range environment {
		if strings.HasPrefix(strings.ToUpper(entry), "PG") {
			return nil, ErrManagedAccessUnavailable
		}
	}
	// Nonempty placeholder suppresses the driver's default .pgpass lookup. Real
	// credentials are assigned only after parsing, so parse errors contain none.
	cfg, err := pgconn.ParseConfig("host=127.0.0.1 port=5432 user=unused dbname=postgres password=unused sslmode=disable connect_timeout=3 target_session_attrs=any")
	if err != nil {
		return nil, ErrManagedAccessUnavailable
	}
	cfg.Host, cfg.Port, cfg.User, cfg.Password = request.Host, request.Port, request.Binding.Username, request.Password
	cfg.TLSConfig = nil
	cfg.Fallbacks = nil
	cfg.RuntimeParams = map[string]string{}
	cfg.RequireAuth = "scram-sha-256"
	return cfg, nil
}

// managedEndpointAllowed accepts loopback for local daemons and the exact IP
// of an explicitly configured remote Linux Docker daemon. The latter is
// needed when a native Windows engine talks to a Docker daemon inside WSL;
// arbitrary network destinations remain invalid managed endpoints.
func managedEndpointAllowed(host string, environment []string) bool {
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[strings.ToUpper(name)] = strings.TrimSpace(value)
		}
	}
	if strings.ToLower(values["SQLRS_DOCKER_HOST_PATH_STYLE"]) != "linux" || !strings.HasPrefix(strings.ToLower(values["DOCKER_HOST"]), "tcp://") {
		return false
	}
	remote, _, err := net.SplitHostPort(strings.TrimPrefix(values["DOCKER_HOST"], "tcp://"))
	if err != nil {
		return false
	}
	remoteIP := net.ParseIP(strings.Trim(remote, "[]"))
	return remoteIP != nil && !remoteIP.IsLoopback() && !remoteIP.IsUnspecified() && remoteIP.Equal(ip)
}

// managedRoleEvidence permits no recipe-provided SQL and reads at most one row.
// Exact session/current identity and LOGIN/SUPERUSER are both required.
func managedRoleEvidence(ctx context.Context, conn *pgconn.PgConn, username string) bool {
	const query = `SELECT current_user = $1 AND session_user = $1 AND EXISTS (
 SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1 AND rolcanlogin AND rolsuper)`
	rows := conn.ExecParams(ctx, query, [][]byte{[]byte(username)}, nil, nil, nil)
	if !rows.NextRow() || len(rows.Values()) != 1 || string(rows.Values()[0]) != "t" {
		_, _ = rows.Close()
		return false
	}
	if rows.NextRow() {
		_, _ = rows.Close()
		return false
	}
	_, err := rows.Close()
	return err == nil && ctx.Err() == nil
}

// closeManagedConnection bounds cleanup independently of canceled request work.
func closeManagedConnection(conn *pgconn.PgConn) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = conn.Close(ctx)
}

func managedAccessError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrManagedAccessUnavailable
}
