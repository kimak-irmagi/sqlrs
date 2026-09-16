package instanceaccess

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"

	"github.com/sqlrs/engine-local/internal/managedidentity"
)

// SchemaSQL stores immutable bindings and monotone lifecycle stages, never
// passwords, verifiers, or DSNs. Installed inside the coordinated cutover tx.
const SchemaSQL = `
CREATE TABLE instance_access (
 instance_ref TEXT PRIMARY KEY NOT NULL,
 binding_json TEXT NOT NULL,
 secret_ref TEXT NOT NULL UNIQUE,
 stage TEXT NOT NULL CHECK(stage IN ('reserved','applying','verified','retiring','retired','quarantined'))
);
CREATE TRIGGER instance_access_immutable BEFORE UPDATE OF instance_ref,binding_json,secret_ref ON instance_access
BEGIN SELECT RAISE(ABORT,'immutable managed access binding'); END;
CREATE TRIGGER instance_access_lifecycle BEFORE UPDATE OF stage ON instance_access
WHEN NOT (NEW.stage=OLD.stage OR
 (OLD.stage='reserved' AND NEW.stage IN ('applying','retiring','quarantined')) OR
 (OLD.stage='applying' AND NEW.stage IN ('verified','retiring','quarantined')) OR
 (OLD.stage='verified' AND NEW.stage IN ('retiring','quarantined')) OR
 (OLD.stage='quarantined' AND NEW.stage='retiring') OR
 (OLD.stage='retiring' AND NEW.stage='retired'))
BEGIN SELECT RAISE(ABORT,'invalid managed access transition'); END;
CREATE TABLE bootstrap_access (lineage_ref TEXT PRIMARY KEY NOT NULL, binding_json TEXT NOT NULL, secret_ref TEXT NOT NULL UNIQUE);
CREATE TRIGGER bootstrap_access_immutable BEFORE UPDATE ON bootstrap_access
BEGIN SELECT RAISE(ABORT,'immutable bootstrap access binding'); END;
CREATE TABLE managed_runtime_operations (
 operation_ref TEXT PRIMARY KEY NOT NULL, target_path TEXT NOT NULL,
 source_ref TEXT NOT NULL, identity_json TEXT NOT NULL,
 physical_identity TEXT NOT NULL UNIQUE, runtime_ref TEXT NOT NULL DEFAULT '',
 stage TEXT NOT NULL CHECK(stage IN ('assigned','ready','running','retired'))
);
CREATE TABLE managed_state_seals (
 state_id TEXT PRIMARY KEY NOT NULL, operation_ref TEXT NOT NULL,
 binding_json TEXT NOT NULL
);
CREATE TABLE managed_base_targets (
 base_key TEXT PRIMARY KEY NOT NULL,
 operation_ref TEXT NOT NULL REFERENCES managed_runtime_operations(operation_ref)
);
`

func InstallSchema(ctx context.Context, tx *sql.Tx) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx == nil {
		return ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, SchemaSQL); err != nil {
		return accessError(ctx)
	}
	return nil
}

// AccessBinding scopes a secret to an assigned physical clone and one version.
type AccessBinding struct {
	managedidentity.RuntimeBinding
	InstanceRef   string
	DomainRef     string
	AccessVersion string
	SecretRef     string
}

// Service fences activation, authorized use and retirement per instance. Startup
// holds the exclusive engine-store lock, so no second process can publish.
// See managed-database-identity-internals.md, atomicity and recovery: runtime I/O
// retains only its instance fence, never the shared metadata/registry mutexes.
type Service struct {
	db       *sql.DB
	secrets  *Secrets
	domain   string
	mu       sync.Mutex
	fencesMu sync.Mutex
	fences   map[string]*instanceFence
}

func NewService(db *sql.DB, secrets *Secrets, domain string) (*Service, error) {
	if db == nil || secrets == nil || !accessRefPattern.MatchString(domain) {
		return nil, ErrInvalid
	}
	return &Service{db: db, secrets: secrets, domain: domain}, nil
}

func (s *Service) secretBinding(ref string, b managedidentity.RuntimeBinding) SecretBinding {
	return SecretBinding{DomainRef: s.domain, OwnerRef: ref, IdentityDigest: b.IdentityDigest, Version: b.PhysicalIdentity, Purpose: "instance"}
}

// Activate persists intent before mutation, then verifies through the supplied
// native adapter before invoking publication under the same exclusion. Failed
// verification quarantines the intent and never repairs managed role privileges.
func (s *Service) Activate(ctx context.Context, ref string, b managedidentity.RuntimeBinding, apply func(Secret) error, publish func() error) error {
	unlock, err := s.lockInstance(ctx, ref)
	if err != nil {
		return err
	}
	defer unlock()
	if b.Validate() != nil || !accessRefPattern.MatchString(ref) || apply == nil || publish == nil {
		return ErrInvalid
	}
	sb := s.secretBinding(ref, b)
	expected := AccessBinding{RuntimeBinding: b, InstanceRef: ref, DomainRef: s.domain, AccessVersion: sb.Version, SecretRef: sb.Ref()}
	binding, stage, err := s.load(ctx, ref)
	var secret Secret
	if errors.Is(err, ErrNotFound) {
		secret, err = s.secrets.Reserve(sb)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(expected)
		if _, err = s.db.ExecContext(ctx, "INSERT INTO instance_access(instance_ref,binding_json,secret_ref,stage) VALUES(?,?,?,'reserved')", ref, string(raw), sb.Ref()); err != nil {
			return accessError(ctx)
		}
		binding, stage = expected, "reserved"
	} else if err != nil {
		return err
	} else {
		if binding != expected {
			return ErrConflict
		}
		secret, err = s.secrets.Resolve(sb)
		if err != nil {
			return err
		}
	}
	if binding != expected || (stage != "reserved" && stage != "applying" && stage != "verified") {
		return ErrConflict
	}
	if stage == "reserved" {
		if err := s.transition(ctx, ref, stage, "applying"); err != nil {
			return err
		}
		stage = "applying"
	}
	if err := apply(secret); err != nil {
		_ = s.transition(context.WithoutCancel(ctx), ref, stage, "quarantined")
		return accessError(ctx)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if stage != "verified" {
		if err := s.transition(ctx, ref, stage, "verified"); err != nil {
			return err
		}
	}
	if err := publish(); err != nil {
		return accessError(ctx)
	}
	return nil
}

// Resolve returns credentials only for an exact verified live-runtime binding.
func (s *Service) Resolve(ctx context.Context, ref string, b managedidentity.RuntimeBinding) (Secret, error) {
	unlock, err := s.lockInstance(ctx, ref)
	if err != nil {
		return Secret{}, err
	}
	defer unlock()
	binding, stage, err := s.load(ctx, ref)
	if err != nil {
		return Secret{}, err
	}
	if stage != "verified" || binding.RuntimeBinding != b || binding.DomainRef != s.domain {
		return Secret{}, ErrConflict
	}
	return s.secrets.Resolve(s.secretBinding(ref, b))
}

// Use retains the retirement fence until the authorized operation ends. The
// callback receives a protected reference after its immutable record is checked.
func (s *Service) Use(ctx context.Context, ref string, fn func(AccessBinding, SecretBinding) error) error {
	unlock, err := s.lockInstance(ctx, ref)
	if err != nil {
		return err
	}
	defer unlock()
	b, stage, err := s.load(ctx, ref)
	if err != nil {
		return err
	}
	if stage != "verified" || fn == nil {
		return ErrConflict
	}
	sb := s.secretBinding(ref, b.RuntimeBinding)
	if _, err := s.secrets.Resolve(sb); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fn(b, sb)
}

// Lookup returns non-secret provenance for reconnecting after engine restart.
func (s *Service) Lookup(ctx context.Context, ref string) (AccessBinding, error) {
	unlock, err := s.lockInstance(ctx, ref)
	if err != nil {
		return AccessBinding{}, err
	}
	defer unlock()
	binding, stage, err := s.load(ctx, ref)
	if err != nil {
		return AccessBinding{}, err
	}
	if stage != "verified" {
		return AccessBinding{}, ErrConflict
	}
	return binding, nil
}

// Retire fences access before stopping/removing a physical instance. Failure
// leaves a retryable retiring intent; it can never become verified again.
func (s *Service) Retire(ctx context.Context, ref string, remove func() error) error {
	if remove == nil {
		return s.RetireBound(ctx, ref, nil)
	}
	return s.RetireBound(ctx, ref, func(AccessBinding) error { return remove() })
}

// RetireBound supplies the immutable binding under the same retirement fence,
// including after restart or a failed physical cleanup.
func (s *Service) RetireBound(ctx context.Context, ref string, remove func(AccessBinding) error) error {
	unlock, err := s.lockInstance(ctx, ref)
	if err != nil {
		return err
	}
	defer unlock()
	binding, stage, err := s.load(ctx, ref)
	if err != nil {
		return err
	}
	if stage == "retired" {
		return nil
	}
	if stage != "retiring" {
		if err := s.transition(ctx, ref, stage, "retiring"); err != nil {
			return err
		}
	}
	if remove == nil {
		return ErrInvalid
	}
	if err := remove(binding); err != nil {
		return accessError(ctx)
	}
	return s.transition(ctx, ref, "retiring", "retired")
}

func (s *Service) load(ctx context.Context, ref string) (AccessBinding, string, error) {
	var raw, secretRef, stage string
	err := s.db.QueryRowContext(ctx, "SELECT binding_json,secret_ref,stage FROM instance_access WHERE instance_ref=?", ref).Scan(&raw, &secretRef, &stage)
	if errors.Is(err, sql.ErrNoRows) {
		return AccessBinding{}, "", ErrNotFound
	}
	if err != nil {
		return AccessBinding{}, "", accessError(ctx)
	}
	var b AccessBinding
	if json.Unmarshal([]byte(raw), &b) != nil || b.Validate() != nil || b.InstanceRef != ref || b.DomainRef != s.domain || b.AccessVersion != b.PhysicalIdentity || b.SecretRef != secretRef || b.SecretRef != s.secretBinding(ref, b.RuntimeBinding).Ref() {
		return AccessBinding{}, "", ErrInvalid
	}
	return b, stage, nil
}

func (s *Service) transition(ctx context.Context, ref, from, to string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE instance_access SET stage=? WHERE instance_ref=? AND stage=?", to, ref, from)
	if err != nil {
		return accessError(ctx)
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return ErrConflict
	}
	return nil
}

// Bootstrap reserves separately from public activation. Once its metadata is
// committed, a missing private credential always fails closed (no regeneration).
func (s *Service) Bootstrap(ctx context.Context, b managedidentity.IdentityBinding, reserve bool) (SecretBinding, Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.Validate() != nil {
		return SecretBinding{}, Secret{}, ErrInvalid
	}
	sb := SecretBinding{DomainRef: s.domain, OwnerRef: b.LineageRef, IdentityDigest: b.IdentityDigest, Version: "bootstrap-v1", Purpose: "bootstrap"}
	var raw, ref string
	err := s.db.QueryRowContext(ctx, "SELECT binding_json,secret_ref FROM bootstrap_access WHERE lineage_ref=?", b.LineageRef).Scan(&raw, &ref)
	if errors.Is(err, sql.ErrNoRows) && reserve {
		secret, err := s.secrets.Reserve(sb)
		if err != nil {
			return sb, Secret{}, err
		}
		encoded, _ := json.Marshal(b)
		if _, err := s.db.ExecContext(ctx, "INSERT INTO bootstrap_access VALUES(?,?,?)", b.LineageRef, string(encoded), sb.Ref()); err != nil {
			return sb, Secret{}, accessError(ctx)
		}
		return sb, secret, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return sb, Secret{}, ErrNotFound
	}
	if err != nil {
		return sb, Secret{}, accessError(ctx)
	}
	var stored managedidentity.IdentityBinding
	if json.Unmarshal([]byte(raw), &stored) != nil || stored != b || ref != sb.Ref() {
		return sb, Secret{}, ErrInvalid
	}
	secret, err := s.secrets.Resolve(sb)
	return sb, secret, err
}

func accessError(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrUnavailable
}
