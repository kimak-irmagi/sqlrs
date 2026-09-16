package managedidentity

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"sync"
	"time"
)

// ErrNotFound denotes an absent authoritative record, not corrupt metadata.
var ErrNotFound = errors.New("managed database identity not found")

// Record is the immutable reservation persisted by the cache owner. See the
// managed_base_lineages schema in managed-database-identity-internals.md.
type Record struct {
	Selector  BaseSelector
	Binding   IdentityBinding
	CreatedAt time.Time
}

// Validate rejects incomplete records. Scope authorization is additionally checked
// against the actual requested selector by Service, not against this record alone.
func (r Record) Validate() error {
	if r.Selector.Validate() != nil || r.Binding.Validate() != nil || r.CreatedAt.IsZero() {
		return ErrInvalid
	}
	return nil
}

// Store owns atomic selection and durable storage. Reserve must insert a candidate
// or return the existing winner for its exact selector, never replace that winner.
// Find/Get return ErrNotFound only for absence; corrupt records must be errors.
// Implementations must enforce selector uniqueness across processes.
type Store interface {
	Find(context.Context, BaseSelector) (Record, error)
	Reserve(context.Context, Record) (Record, error)
	Get(context.Context, string, string) (Record, error)
}

// Service resolves metadata only; it cannot initialize, activate or publish a
// runtime. The mutex serializes entropy reads, not database ownership.
type Service struct {
	store     Store
	entropy   io.Reader
	entropyMu sync.Mutex
}

// NewService uses the OS cryptographic entropy source in production.
func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, ErrUnavailable
	}
	return &Service{store: store, entropy: rand.Reader}, nil
}

// ResolveOrReserveBase first reads the durable identity. Only absence permits
// generation; after a race the database's committed winner is authoritative.
// Cancellation after a write may leave a valid reservation but never a returned
// success; retry discovers the same record.
func (s *Service) ResolveOrReserveBase(ctx context.Context, selector BaseSelector) (IdentityBinding, error) {
	if err := selector.Validate(); err != nil {
		return IdentityBinding{}, err
	}
	if err := ctx.Err(); err != nil {
		return IdentityBinding{}, err
	}
	record, err := s.store.Find(ctx, selector)
	if err == nil {
		return resolved(ctx, record, selector, "")
	}
	if !errors.Is(err, ErrNotFound) {
		return IdentityBinding{}, safeStoreError(err)
	}
	if err := ctx.Err(); err != nil {
		return IdentityBinding{}, err
	}
	s.entropyMu.Lock()
	candidate, err := Generate(s.entropy)
	s.entropyMu.Unlock()
	if err != nil {
		return IdentityBinding{}, err
	}
	if err := ctx.Err(); err != nil {
		return IdentityBinding{}, err
	}
	record, err = s.store.Reserve(ctx, Record{Selector: selector, Binding: candidate, CreatedAt: time.Now().UTC()})
	if err != nil {
		return IdentityBinding{}, safeStoreError(err)
	}
	return resolved(ctx, record, selector, "")
}

// GetIdentity reads an exact authorized lineage; absence never causes generation.
func (s *Service) GetIdentity(ctx context.Context, domain, lineage string) (IdentityBinding, error) {
	if !referencePattern.MatchString(domain) || !referencePattern.MatchString(lineage) {
		return IdentityBinding{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return IdentityBinding{}, err
	}
	record, err := s.store.Get(ctx, domain, lineage)
	if err != nil {
		return IdentityBinding{}, safeStoreError(err)
	}
	if record.Selector.DomainRef != domain {
		return IdentityBinding{}, ErrInvalid
	}
	return resolved(ctx, record, record.Selector, lineage)
}

// resolved checks persisted evidence and cancellation before exposing the binding.
func resolved(ctx context.Context, record Record, selector BaseSelector, lineage string) (IdentityBinding, error) {
	if err := ctx.Err(); err != nil {
		return IdentityBinding{}, err
	}
	if record.Validate() != nil || record.Selector != selector || (lineage != "" && record.Binding.LineageRef != lineage) {
		return IdentityBinding{}, ErrInvalid
	}
	return record.Binding, nil
}

// safeStoreError preserves actionable bounded classifications, never raw SQL,
// connection strings, filesystem paths or driver diagnostics.
func safeStoreError(err error) error {
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, ErrNotFound, ErrInvalid} {
		if errors.Is(err, known) {
			return known
		}
	}
	return ErrUnavailable
}
