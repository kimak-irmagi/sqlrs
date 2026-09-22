package dbms

import (
	"context"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// EnsureManagedPassword activates one assigned clone through native PostgreSQL
// connections. An already applied password is verified on retry. The inherited
// bootstrap password must still prove the exact LOGIN/SUPERUSER role before any
// ALTER, so missing roles or lost privileges cannot be repaired by activation.
// The caller holds the operation/retirement exclusion across this call and
// publication; see managed-database-identity-internals.md.
func EnsureManagedPassword(ctx context.Context, previous ManagedConnection, password string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	next := previous
	next.Password = password
	if _, err := managedConnectionConfig(next, os.Environ()); err != nil {
		return err
	}
	if _, err := VerifyManagedAccess(ctx, next); err == nil {
		return nil
	}
	if _, err := VerifyManagedAccess(ctx, previous); err != nil {
		return err
	}
	cfg, err := managedConnectionConfig(previous, os.Environ())
	if err != nil {
		return err
	}
	conn, err := pgconn.ConnectConfig(ctx, cfg)
	if err != nil {
		return managedAccessError(ctx)
	}
	defer closeManagedConnection(conn)
	if !managedRoleEvidence(ctx, conn, previous.Binding.Username) {
		return managedAccessError(ctx)
	}
	// Never forward raw SQL/driver diagnostics; quiet the maintenance session
	// before sending the password. Identifier and password have strict alphabets.
	if _, err := conn.Exec(ctx, "SET log_statement='none'; SET log_min_error_statement='panic'; SET log_min_messages='panic'; SET password_encryption='scram-sha-256'").ReadAll(); err != nil {
		return managedAccessError(ctx)
	}
	statement := `ALTER ROLE "` + strings.ReplaceAll(previous.Binding.Username, `"`, `""`) + `" PASSWORD '` + password + `'`
	if _, err := conn.Exec(ctx, statement).ReadAll(); err != nil {
		return managedAccessError(ctx)
	}
	_, err = VerifyManagedAccess(ctx, next)
	return err
}
