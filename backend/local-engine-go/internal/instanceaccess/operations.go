package instanceaccess

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/sqlrs/engine-local/internal/managedidentity"
)

// Operation assigns a physical target before any external mutation. References
// and fingerprints contain no credentials; targets cannot be adopted by name.
type Operation struct {
	Ref, Target, Source, PhysicalIdentity, RuntimeRef, Stage string
	Identity                                                 managedidentity.IdentityBinding
}

func (s *Service) Assign(ctx context.Context, ref, target, source string, id managedidentity.IdentityBinding) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !accessRefPattern.MatchString(ref) || !filepath.IsAbs(target) || source == "" || id.Validate() != nil {
		return Operation{}, ErrInvalid
	}
	op, err := s.operation(ctx, ref)
	if err == nil {
		if op.Target != target || op.Source != source || op.Identity != id || op.Stage == "retired" {
			return Operation{}, ErrConflict
		}
		return op, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Operation{}, err
	}
	op = Operation{Ref: ref, Target: target, Source: source, Identity: id, PhysicalIdentity: "physical_" + rand.Text(), Stage: "assigned"}
	raw, _ := json.Marshal(id)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO managed_runtime_operations(operation_ref,target_path,source_ref,identity_json,physical_identity,stage) VALUES(?,?,?,?,?,'assigned')`, ref, target, source, string(raw), op.PhysicalIdentity); err != nil {
		return Operation{}, accessError(ctx)
	}
	return op, nil
}

func (s *Service) operation(ctx context.Context, ref string) (Operation, error) {
	var op Operation
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT operation_ref,target_path,source_ref,identity_json,physical_identity,runtime_ref,stage FROM managed_runtime_operations WHERE operation_ref=?`, ref).Scan(&op.Ref, &op.Target, &op.Source, &raw, &op.PhysicalIdentity, &op.RuntimeRef, &op.Stage)
	if errors.Is(err, sql.ErrNoRows) {
		return op, ErrNotFound
	}
	if err != nil {
		return op, accessError(ctx)
	}
	if json.Unmarshal([]byte(raw), &op.Identity) != nil || op.Identity.Validate() != nil || !accessRefPattern.MatchString(op.PhysicalIdentity) {
		return Operation{}, ErrInvalid
	}
	return op, nil
}

// BaseReady records completed controlled initialization. Existing unowned or
// partially initialized directories are never reset by this operation.
func (s *Service) BaseReady(ctx context.Context, op Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.ExecContext(ctx, `UPDATE managed_runtime_operations SET stage='ready' WHERE operation_ref=? AND physical_identity=? AND stage='assigned'`, op.Ref, op.PhysicalIdentity)
	return operationResult(ctx, result, err)
}

func (s *Service) Attach(ctx context.Context, op Operation, b managedidentity.RuntimeBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.Validate() != nil || b.IdentityBinding != op.Identity || b.PhysicalIdentity != op.PhysicalIdentity {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE managed_runtime_operations SET runtime_ref=?,stage='running' WHERE operation_ref=? AND physical_identity=? AND stage='assigned' AND runtime_ref=''`, b.RuntimeRef, op.Ref, op.PhysicalIdentity)
	return operationResult(ctx, result, err)
}

// RecordSeal consumes proof while the caller holds its runtime exclusion and
// PostgreSQL is stopped. Capture and metadata publication follow this record.
func (s *Service) RecordSeal(ctx context.Context, state string, op Operation, b managedidentity.RuntimeBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.operation(ctx, op.Ref)
	if err != nil {
		return err
	}
	if current.Stage != "running" || current.RuntimeRef != b.RuntimeRef || current.PhysicalIdentity != b.PhysicalIdentity || current.Identity != b.IdentityBinding {
		return ErrConflict
	}
	raw, _ := json.Marshal(b)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO managed_state_seals(state_id,operation_ref,binding_json) VALUES(?,?,?) ON CONFLICT(state_id) DO NOTHING`, state, op.Ref, string(raw)); err != nil {
		return accessError(ctx)
	}
	return s.checkSeal(ctx, state, op.Identity)
}

func (s *Service) CheckSeal(ctx context.Context, state string, id managedidentity.IdentityBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checkSeal(ctx, state, id)
}
func (s *Service) checkSeal(ctx context.Context, state string, id managedidentity.IdentityBinding) error {
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT binding_json FROM managed_state_seals WHERE state_id=?", state).Scan(&raw); err != nil {
		return accessError(ctx)
	}
	var b managedidentity.RuntimeBinding
	if json.Unmarshal([]byte(raw), &b) != nil || b.Validate() != nil || b.IdentityBinding != id {
		return ErrInvalid
	}
	return nil
}

func (s *Service) RetireOperation(ctx context.Context, op Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.ExecContext(ctx, `UPDATE managed_runtime_operations SET stage='retired' WHERE operation_ref=? AND physical_identity=?`, op.Ref, op.PhysicalIdentity)
	return operationResult(ctx, result, err)
}

func operationResult(ctx context.Context, result sql.Result, err error) error {
	if err != nil {
		return accessError(ctx)
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return ErrConflict
	}
	return nil
}

// BaseOperation preserves lineage through physical eviction while assigning a
// fresh physical generation. Old journal records remain retired and immutable.
// The coordinator must check existence without following links under its base
// exclusion; only an absent target permits a new generation.
func (s *Service) BaseOperation(ctx context.Context, key, target string, id managedidentity.IdentityBinding, exists bool) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !accessRefPattern.MatchString(key) || !filepath.IsAbs(target) || id.Validate() != nil {
		return Operation{}, ErrInvalid
	}
	var current string
	err := s.db.QueryRowContext(ctx, "SELECT operation_ref FROM managed_base_targets WHERE base_key=?", key).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Operation{}, accessError(ctx)
	}
	if current != "" {
		op, err := s.operation(ctx, current)
		if err != nil {
			return Operation{}, err
		}
		if op.Target != target || op.Identity != id || (op.Stage != "assigned" && op.Stage != "ready") {
			return Operation{}, ErrConflict
		}
		if exists || op.Stage == "assigned" {
			return op, nil
		}
	} else if exists {
		return Operation{}, ErrConflict
	}
	physical := "physical_" + rand.Text()
	op := Operation{Ref: "base_" + rand.Text(), Target: target, Source: key, PhysicalIdentity: physical, Stage: "assigned", Identity: id}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Operation{}, accessError(ctx)
	}
	defer tx.Rollback()
	if current != "" {
		if _, err := tx.ExecContext(ctx, "UPDATE managed_runtime_operations SET stage='retired' WHERE operation_ref=? AND stage='ready'", current); err != nil {
			return Operation{}, accessError(ctx)
		}
	}
	raw, _ := json.Marshal(id)
	if _, err := tx.ExecContext(ctx, `INSERT INTO managed_runtime_operations(operation_ref,target_path,source_ref,identity_json,physical_identity,stage) VALUES(?,?,?,?,?,'assigned')`, op.Ref, target, key, string(raw), physical); err != nil {
		return Operation{}, accessError(ctx)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO managed_base_targets(base_key,operation_ref) VALUES(?,?) ON CONFLICT(base_key) DO UPDATE SET operation_ref=excluded.operation_ref`, key, op.Ref); err != nil {
		return Operation{}, accessError(ctx)
	}
	if err := tx.Commit(); err != nil {
		return Operation{}, accessError(ctx)
	}
	return op, nil
}
