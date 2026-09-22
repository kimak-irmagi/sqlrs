package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"io"
	"regexp"
)

// ManagedStoreFormatVersion identifies the coordinated managed-identity schema.
// An unrecognized format never selects a legacy fallback or a new DomainRef.
const ManagedStoreFormatVersion = "sqlrs-managed-store.v1"

// ManagedStoreFormat is non-secret store provenance, independent of its path.
// Persist it with schema installation in the same caller-owned transaction.
type ManagedStoreFormat struct {
	Version   string
	DomainRef string
}

var storeDomainPattern = regexp.MustCompile(`^store_[0-9a-f]{32}$`)

// EnsureManagedStoreFormat reserves store provenance only after empty legacy
// metadata and physical inventory are established. The caller must keep admission
// closed, install all managed schemas using tx, and commit them together. A
// rollback preserves the old format, including when later schema installation
// fails. This function alone is not a startup upgrade. Existing new-format stores
// retain their DomainRef and do not undergo the empty-legacy inventory again.
// See docs/architecture/managed-database-identity-internals.md, upgrade conditions.
func EnsureManagedStoreFormat(ctx context.Context, tx *sql.Tx, inventory func(context.Context) error) (ManagedStoreFormat, error) {
	return ensureManagedStoreFormat(ctx, tx, inventory, rand.Reader)
}

// ensureManagedStoreFormat isolates entropy failures for deterministic tests.
func ensureManagedStoreFormat(ctx context.Context, tx *sql.Tx, inventory func(context.Context) error, source io.Reader) (ManagedStoreFormat, error) {
	if err := ctx.Err(); err != nil {
		return ManagedStoreFormat{}, err
	}
	if tx == nil {
		return ManagedStoreFormat{}, ErrManagedPreflightUnavailable
	}
	var count int
	var kind string
	if err := tx.QueryRowContext(ctx, `SELECT count(*), coalesce(min(type), '') FROM main.sqlite_master WHERE name = 'managed_store_format' COLLATE NOCASE`).Scan(&count, &kind); err != nil {
		return ManagedStoreFormat{}, managedPreflightError(ctx, err)
	}
	if count != 0 {
		if count != 1 || kind != "table" {
			return ManagedStoreFormat{}, ErrManagedPreflightUnavailable
		}
		var format ManagedStoreFormat
		var slot int
		if err := tx.QueryRowContext(ctx, `SELECT count(*), coalesce(min(slot),0), coalesce(min(format_version),''), coalesce(min(domain_ref),'') FROM main.managed_store_format`).Scan(&count, &slot, &format.Version, &format.DomainRef); err != nil {
			return ManagedStoreFormat{}, managedPreflightError(ctx, err)
		}
		if count != 1 || slot != 1 || format.Version != ManagedStoreFormatVersion || !storeDomainPattern.MatchString(format.DomainRef) {
			return ManagedStoreFormat{}, ErrManagedPreflightUnavailable
		}
		return format, nil
	}
	// An unversioned lineage table is ambiguous, even if empty. Legitimate new
	// installation commits lineage and format atomically; never guess a repair.
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM main.sqlite_master WHERE name = 'managed_base_lineages' COLLATE NOCASE`).Scan(&count); err != nil {
		return ManagedStoreFormat{}, managedPreflightError(ctx, err)
	}
	if count != 0 || inventory == nil {
		return ManagedStoreFormat{}, ErrManagedPreflightUnavailable
	}
	if err := checkEmptyManagedTables(ctx, tx); err != nil {
		return ManagedStoreFormat{}, err
	}
	if err := inventory(ctx); err != nil {
		if err == ErrLegacyManagedData {
			return ManagedStoreFormat{}, err
		}
		return ManagedStoreFormat{}, managedPreflightError(ctx, err)
	}
	var entropy [16]byte
	if _, err := io.ReadFull(source, entropy[:]); err != nil {
		return ManagedStoreFormat{}, ErrManagedPreflightUnavailable
	}
	format := ManagedStoreFormat{Version: ManagedStoreFormatVersion, DomainRef: "store_" + hex.EncodeToString(entropy[:])}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE managed_store_format (
 slot INTEGER PRIMARY KEY CHECK(slot = 1), format_version TEXT NOT NULL, domain_ref TEXT NOT NULL
 ); INSERT INTO managed_store_format(slot,format_version,domain_ref) VALUES (1,?,?)`, format.Version, format.DomainRef); err != nil {
		return ManagedStoreFormat{}, managedPreflightError(ctx, err)
	}
	return format, nil
}
