package managedstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"

	sqliteDriver "modernc.org/sqlite"
)

var errNIInjected = errors.New("NI observed SQL boundary refusal")

// This is a per-DB transparent real-driver observer, never a global hook.
// It records the actual statements/commit outcomes before testing atomicity.
type niSQLTrace struct {
	mu                                               sync.Mutex
	statements                                       []string
	writeAttempts, hitCount, begins, commits, nextID int
	commitCodes                                      []int
	closed                                           []int
	faultKind, needle, when                          string
	requireWrite, requireCommit, lostCommitReply     bool
	cancelAfterWrite                                 context.CancelFunc
	committed                                        bool
	faultConsumed                                    bool
}

func (s *niSQLTrace) mutations() int { s.mu.Lock(); defer s.mu.Unlock(); return s.writeAttempts }
func (s *niSQLTrace) hits() int      { s.mu.Lock(); defer s.mu.Unlock(); return s.hitCount }
func (s *niSQLTrace) observe(kind, query string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statements = append(s.statements, query)
	upper := strings.ToUpper(strings.TrimSpace(query))
	if kind == "exec" || kind == "query" {
		for _, prefix := range []string{"CREATE ", "DROP ", "ALTER ", "INSERT ", "UPDATE ", "DELETE ", "REPLACE "} {
			if strings.HasPrefix(upper, prefix) {
				s.writeAttempts++
				break
			}
		}
	}
}
func (s *niSQLTrace) refuse(kind, query, when string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.faultConsumed || s.faultKind != kind || s.when != when || !strings.Contains(query, s.needle) {
		return false
	}
	if s.requireWrite && s.writeAttempts == 0 || s.requireCommit && !s.committed {
		return false
	}
	s.hitCount++
	s.faultConsumed = true
	return true
}
func (s *niSQLTrace) afterWrite(query string) {
	s.mu.Lock()
	cancel := s.cancelAfterWrite
	if cancel != nil && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "CREATE TABLE LIQUIBASE_EXECUTION_BINDINGS") {
		s.hitCount++
		s.cancelAfterWrite = nil
	} else {
		cancel = nil
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

type niConnector struct {
	path  string
	trace *niSQLTrace
}

func (c *niConnector) Driver() driver.Driver { return &sqliteDriver.Driver{} }
func (c *niConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := c.Driver().Open(c.path)
	if err != nil {
		return nil, err
	}
	// A replacement is configured before delivery, matching the required
	// trusted factory. These PRAGMAs do not substitute for Initialize's checks.
	if execer, ok := raw.(driver.ExecerContext); ok {
		if _, err := execer.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=0", nil); err != nil {
			raw.Close()
			return nil, err
		}
	}
	c.trace.mu.Lock()
	c.trace.nextID++
	id := c.trace.nextID
	c.trace.mu.Unlock()
	return &niDriverConn{Conn: raw, trace: c.trace, id: id}, nil
}
func openNIAudited(t *testing.T, path string, trace *niSQLTrace) *sql.DB {
	t.Helper()
	db := sql.OpenDB(&niConnector{path, trace})
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type niDriverConn struct {
	driver.Conn
	trace *niSQLTrace
	id    int
}

func (c *niDriverConn) Close() error {
	c.trace.mu.Lock()
	c.trace.closed = append(c.trace.closed, c.id)
	c.trace.mu.Unlock()
	return c.Conn.Close()
}
func (c *niDriverConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *niDriverConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.trace.mu.Lock()
	c.trace.begins++
	c.trace.mu.Unlock()
	if c.trace.refuse("begin", "", "before") {
		return nil, errNIInjected
	}
	var tx driver.Tx
	var err error
	if b, ok := c.Conn.(driver.ConnBeginTx); ok {
		tx, err = b.BeginTx(ctx, opts)
	} else {
		tx, err = c.Conn.Begin()
	}
	if err != nil {
		return nil, err
	}
	return &niDriverTx{Tx: tx, trace: c.trace}, nil
}
func (c *niDriverConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &niDriverStmt{Stmt: stmt, query: query, trace: c.trace}, nil
}
func (c *niDriverConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if p, ok := c.Conn.(driver.ConnPrepareContext); ok {
		stmt, err := p.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		return &niDriverStmt{Stmt: stmt, query: query, trace: c.trace}, nil
	}
	return c.Prepare(query)
}
func (c *niDriverConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	c.trace.observe("exec", query)
	if c.trace.refuse("exec", query, "before") {
		return nil, errNIInjected
	}
	result, err := execer.ExecContext(ctx, query, args)
	if err == nil {
		c.trace.afterWrite(query)
	}
	if err == nil && c.trace.refuse("exec", query, "after") {
		return nil, errNIInjected
	}
	return result, err
}
func (c *niDriverConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	c.trace.observe("query", query)
	if c.trace.refuse("query", query, "before") {
		return nil, errNIInjected
	}
	return queryer.QueryContext(ctx, query, args)
}
func (c *niDriverConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}
func (c *niDriverConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

type niDriverTx struct {
	driver.Tx
	trace *niSQLTrace
}

func (tx *niDriverTx) Commit() error {
	tx.trace.mu.Lock()
	tx.trace.commits++
	tx.trace.mu.Unlock()
	if tx.trace.refuse("commit", "", "before") {
		return errNIInjected
	}
	err := tx.Tx.Commit()
	code := 0
	if err != nil {
		var coded interface{ Code() int }
		code = -1
		if errors.As(err, &coded) {
			code = coded.Code()
		}
	}
	tx.trace.mu.Lock()
	tx.trace.commitCodes = append(tx.trace.commitCodes, code)
	if err == nil {
		tx.trace.committed = true
	}
	lose := tx.trace.lostCommitReply
	if lose && err == nil {
		tx.trace.hitCount++
	}
	tx.trace.mu.Unlock()
	if lose && err == nil {
		return errNIInjected
	}
	return err
}

type niDriverStmt struct {
	driver.Stmt
	query string
	trace *niSQLTrace
}

func (s *niDriverStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.trace.observe("exec", s.query)
	if s.trace.refuse("exec", s.query, "before") {
		return nil, errNIInjected
	}
	result, err := s.Stmt.Exec(args)
	if err == nil {
		s.trace.afterWrite(s.query)
	}
	if err == nil && s.trace.refuse("exec", s.query, "after") {
		return nil, errNIInjected
	}
	return result, err
}
func (s *niDriverStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.trace.observe("query", s.query)
	if s.trace.refuse("query", s.query, "before") {
		return nil, errNIInjected
	}
	return s.Stmt.Query(args)
}
func (s *niDriverStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if e, ok := s.Stmt.(driver.StmtExecContext); ok {
		s.trace.observe("exec", s.query)
		if s.trace.refuse("exec", s.query, "before") {
			return nil, errNIInjected
		}
		result, err := e.ExecContext(ctx, args)
		if err == nil {
			s.trace.afterWrite(s.query)
		}
		if err == nil && s.trace.refuse("exec", s.query, "after") {
			return nil, errNIInjected
		}
		return result, err
	}
	values := make([]driver.Value, len(args))
	for i, v := range args {
		values[i] = v.Value
	}
	return s.Exec(values)
}
func (s *niDriverStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if q, ok := s.Stmt.(driver.StmtQueryContext); ok {
		s.trace.observe("query", s.query)
		if s.trace.refuse("query", s.query, "before") {
			return nil, errNIInjected
		}
		return q.QueryContext(ctx, args)
	}
	values := make([]driver.Value, len(args))
	for i, v := range args {
		values[i] = v.Value
	}
	return s.Query(values)
}
