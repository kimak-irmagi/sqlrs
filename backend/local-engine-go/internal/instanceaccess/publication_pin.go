package instanceaccess

import (
	"context"
	"database/sql"
	"errors"
)

var ErrPublicationPinned = errors.New("instance publication is still being finalized")

// PublicationPins is a narrow queue-owned lookup; instanceaccess does not import
// the prepare coordinator. Wiring is immutable for the service lifetime.
type PublicationPins interface {
	HasPublicationPin(context.Context, string) (bool, error)
}

// NewServiceWithPublicationPins is required by production startup when the
// finalization component is enabled. The plain constructor remains usable by
// independent identity/access deployments with no prepare component.
func NewServiceWithPublicationPins(db *sql.DB, secrets *Secrets, domain string, pins PublicationPins) (*Service, error) {
	if pins == nil {
		return nil, ErrInvalid
	}
	service, err := NewService(db, secrets, domain)
	if err != nil {
		return nil, err
	}
	service.publicationPins = pins
	return service, nil
}

// checkPublicationPin is called under the exact retirement fence. No global
// metadata mutex spans an injected lookup or native removal (LC14/20).
// Only current finalization format 2 is runtime authority; older terminal-only
// stores are upgraded by managedstore before access admission (JF02).
func (s *Service) checkPublicationPin(ctx context.Context, ref string) error {
	var enabled int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE lower(name) IN ('prepare_liquibase_finalization_store_format','prepare_liquibase_job_finalizations','prepare_liquibase_job_cancellations') OR lower(name) GLOB 'prepare_lb_finalization_*'`).Scan(&enabled); err != nil {
		return accessError(ctx)
	}
	if enabled == 0 {
		return nil
	}
	var records, version int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*),coalesce(max(format_version),0) FROM prepare_liquibase_finalization_store_format WHERE slot=1`).Scan(&records, &version); err != nil || records != 1 || version != 2 {
		return ErrUnavailable
	}
	if s.publicationPins == nil {
		return ErrUnavailable
	}
	pinned, err := s.publicationPins.HasPublicationPin(ctx, ref)
	if err != nil {
		return ErrUnavailable
	}
	if pinned {
		return ErrPublicationPinned
	}
	return nil
}

// CheckRetirement exposes a read-only preview under the exact instance fence.
// Actual retirement rechecks the pin before effects; this snapshot is not a
// lasting deletion capability. Used by direct/ancestor deletion and dry runs.
func (s *Service) CheckRetirement(ctx context.Context, ref string) error {
	unlock, err := s.lockInstance(ctx, ref)
	if err != nil {
		return err
	}
	defer unlock()
	return s.checkPublicationPin(ctx, ref)
}
