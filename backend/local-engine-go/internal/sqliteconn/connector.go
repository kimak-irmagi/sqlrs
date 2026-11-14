// Package sqliteconn configures every physical managed-file connection before
// database/sql can lend it to engine, store or queue callers. It owns no schema
// migration or business retry. Requirements: finalization-connection-ownership.md
// §1 and the separately approved constructor boundary §6.
package sqliteconn

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"

	"modernc.org/sqlite"
)

// Open creates a lazy, single-connection managed-file pool. Callers retain path
// validation/directory creation and schema initialization. Every first or later
// physical opening passes through Connect; caller-owned New(db) is unchanged.
func Open(path string) (*sql.DB, error) {
	db := sql.OpenDB(newConnector(path, &sqlite.Driver{}))
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

// connector has immutable opening dependencies. Only same-package tests supply
// delegating driver wrappers; product callers always use Open's pinned driver.
type connector struct {
	path     string
	source   driver.Driver
	reporter closeReporter
}

func newConnector(path string, source driver.Driver) driver.Connector {
	return newConnectorWithReporter(path, source, reportClose)
}

// newConnectorWithReporter fixes a process receiver per factory, without a
// mutable global hook. Requirements: close-observation-design.md §4.2.
func newConnectorWithReporter(path string, source driver.Driver, reporter closeReporter) driver.Connector {
	return &connector{path: path, source: source, reporter: reporter}
}

// Driver identifies the same source used by Connect; it does not register hooks.
func (c *connector) Driver() driver.Driver { return c.source }

// Connect owns a successfully opened connection until verification completes.
// A refusal attempts exact Close once and preserves all observed causes. Native
// Open/Close are not context-aware, so this cannot interrupt a stuck driver call.
func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	physical, err := c.source.Open(c.path)
	if err != nil {
		return nil, errors.Join(err, ctx.Err())
	}
	refuse := func(cause error) (driver.Conn, error) {
		closeErr := physical.Close()
		return nil, errors.Join(cause, ctx.Err(), closeErr)
	}
	if err := ctx.Err(); err != nil {
		return refuse(err)
	}
	exec, canExec := physical.(driver.ExecerContext)
	query, canQuery := physical.(driver.QueryerContext)
	if !canExec || !canQuery {
		return refuse(errors.New("SQLite connection lacks context-aware configuration interfaces"))
	}
	_, canBegin := physical.(driver.ConnBeginTx)
	_, canPrepare := physical.(driver.ConnPrepareContext)
	_, canPing := physical.(driver.Pinger)
	if !canBegin || !canPrepare || !canPing {
		return refuse(errors.New("SQLite connection lacks managed context-aware interfaces"))
	}
	for _, setting := range []struct {
		name, value string
		expected    int64
	}{
		{"foreign_keys", "ON", 1}, {"busy_timeout", "0", 0}, {"read_uncommitted", "0", 0},
	} {
		if err := ctx.Err(); err != nil {
			return refuse(err)
		}
		if _, err := exec.ExecContext(ctx, "PRAGMA "+setting.name+"="+setting.value, nil); err != nil {
			return refuse(err)
		}
		if err := verify(ctx, query, setting.name, setting.expected); err != nil {
			return refuse(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return refuse(err)
	}
	return newObservedConn(physical, c.reporter), nil
}

// verify requires exactly one integer result matching the requested setting.
// It closes its rows before Connect can close the physical connection; even a
// close failure prevents handoff. Messages contain only fixed setting names.
func verify(ctx context.Context, query driver.QueryerContext, name string, expected int64) (err error) {
	rows, err := query.QueryContext(ctx, "PRAGMA "+name, nil)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	values := make([]driver.Value, 1)
	if err := rows.Next(values); err != nil {
		return fmt.Errorf("SQLite setting %s could not be read: %w", name, err)
	}
	value, ok := values[0].(int64)
	if !ok || value != expected {
		return fmt.Errorf("SQLite setting %s did not match required value", name)
	}
	if err := rows.Next(values); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("SQLite setting %s returned multiple rows", name)
	}
	return nil
}
