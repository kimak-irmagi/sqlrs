package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"time"

	"github.com/sqlrs/engine-local/internal/managedidentity"
)

// ManagedLineageSchemaSQL exposes the approved reservation schema for explicit
// post-preflight installation. It is deliberately absent from initDB: existing
// stores must not be upgraded before the runtime inventory gate is implemented.
// See docs/architecture/managed-database-identity-internals.md.
func ManagedLineageSchemaSQL() string { return managedLineageSchema }

const managedLineageSchema = `CREATE TABLE managed_base_lineages (
 domain_ref TEXT NOT NULL CHECK(length(domain_ref) BETWEEN 1 AND 128),
 lineage_ref TEXT NOT NULL CHECK(length(lineage_ref) BETWEEN 1 AND 128),
 engine_kind TEXT NOT NULL CHECK(engine_kind = 'postgres'),
 image_digest TEXT NOT NULL CHECK(length(image_digest) = 71),
 init_spec_digest TEXT NOT NULL CHECK(length(init_spec_digest) = 64),
 policy_version TEXT NOT NULL CHECK(length(policy_version) > 0),
 username TEXT NOT NULL CHECK(length(username) = 44 AND substr(username,1,12) = 'sqlrs_admin_' AND substr(username,13) NOT GLOB '*[^0-9a-f]*'),
 identity_digest TEXT NOT NULL CHECK(length(identity_digest) = 64),
 created_at TEXT NOT NULL CHECK(length(created_at) > 0),
 PRIMARY KEY(domain_ref,lineage_ref),
 UNIQUE(domain_ref,engine_kind,image_digest,init_spec_digest,policy_version)
);`

const lineageColumns = `domain_ref,lineage_ref,engine_kind,image_digest,init_spec_digest,policy_version,username,identity_digest,created_at`
const lineageSelectorWhere = `domain_ref = ? AND engine_kind = ? AND image_digest = ? AND init_spec_digest = ? AND policy_version = ?`

var lineageReference = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var _ managedidentity.Store = (*Store)(nil)

// Find reads the exact initialization selector. Missing schema is unavailable,
// not absence; the coordinator must not generate a replacement for that error.
func (s *Store) Find(ctx context.Context, selector managedidentity.BaseSelector) (managedidentity.Record, error) {
	if err := selector.Validate(); err != nil {
		return managedidentity.Record{}, err
	}
	return scanLineage(s.db.QueryRowContext(ctx, "SELECT "+lineageColumns+" FROM managed_base_lineages WHERE "+lineageSelectorWhere,
		selector.DomainRef, selector.EngineKind, selector.ImageDigest, selector.InitSpecDigest, selector.PolicyVersion))
}

// Reserve atomically inserts an immutable candidate or reads the committed
// winner. The database constraint, not an in-process mutex, resolves competition.
// A colliding primary key for a different selector is an error, not replacement.
func (s *Store) Reserve(ctx context.Context, r managedidentity.Record) (managedidentity.Record, error) {
	if err := r.Validate(); err != nil {
		return managedidentity.Record{}, err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO managed_base_lineages (`+lineageColumns+`)
 VALUES (?,?,?,?,?,?,?,?,?)
 ON CONFLICT(domain_ref,engine_kind,image_digest,init_spec_digest,policy_version) DO NOTHING`,
		r.Selector.DomainRef, r.Binding.LineageRef, r.Selector.EngineKind, r.Selector.ImageDigest, r.Selector.InitSpecDigest, r.Selector.PolicyVersion, r.Binding.Username, r.Binding.IdentityDigest, r.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return managedidentity.Record{}, lineageError(err)
	}
	return s.Find(ctx, r.Selector)
}

// Get resolves an exact lineage inside an authorized domain; it never searches
// for a similarly named SQL administrator in the database.
func (s *Store) Get(ctx context.Context, domain, lineage string) (managedidentity.Record, error) {
	if !lineageReference.MatchString(domain) || !lineageReference.MatchString(lineage) {
		return managedidentity.Record{}, managedidentity.ErrInvalid
	}
	return scanLineage(s.db.QueryRowContext(ctx, "SELECT "+lineageColumns+" FROM managed_base_lineages WHERE domain_ref = ? AND lineage_ref = ?", domain, lineage))
}

// scanLineage verifies stored shape and digest before returning authoritative
// evidence. Corruption remains an error even when a selector itself matched.
func scanLineage(row *sql.Row) (managedidentity.Record, error) {
	var r managedidentity.Record
	var created string
	err := row.Scan(&r.Selector.DomainRef, &r.Binding.LineageRef, &r.Selector.EngineKind, &r.Selector.ImageDigest, &r.Selector.InitSpecDigest, &r.Selector.PolicyVersion, &r.Binding.Username, &r.Binding.IdentityDigest, &created)
	if err != nil {
		return managedidentity.Record{}, lineageError(err)
	}
	r.Binding.EngineKind = r.Selector.EngineKind
	r.Binding.PolicyVersion = r.Selector.PolicyVersion
	r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return managedidentity.Record{}, managedidentity.ErrInvalid
	}
	if err := r.Validate(); err != nil {
		return managedidentity.Record{}, err
	}
	return r, nil
}

// lineageError never exposes database paths, SQL or driver diagnostics.
func lineageError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return managedidentity.ErrNotFound
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return managedidentity.ErrUnavailable
}
