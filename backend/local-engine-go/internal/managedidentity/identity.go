// Package managedidentity implements managed database identity policy described in
// docs/architecture/managed-database-identity-internals.md.
package managedidentity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
)

// PolicyVersion separates cache namespaces when administrative identity policy changes.
const PolicyVersion = "postgres-managed-identity.v1"

var (
	// ErrInvalid rejects malformed or internally inconsistent non-secret metadata.
	ErrInvalid = errors.New("managed database identity invalid")
	// ErrUnavailable deliberately hides entropy-provider diagnostics.
	ErrUnavailable   = errors.New("managed database identity unavailable")
	referencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	usernamePattern  = regexp.MustCompile(`^sqlrs_admin_[0-9a-f]{32}$`)
	imagePattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	digestPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// BaseSelector identifies the authorized cache domain and immutable initialization
// inputs. DomainRef is supplied by the owner, never inferred from recipe SQL.
type BaseSelector struct {
	DomainRef      string
	EngineKind     string
	ImageDigest    string
	InitSpecDigest string
	PolicyVersion  string
}

// Validate accepts only the currently supported PostgreSQL identity policy.
func (s BaseSelector) Validate() error {
	if !referencePattern.MatchString(s.DomainRef) || s.EngineKind != "postgres" ||
		s.PolicyVersion != PolicyVersion || !imagePattern.MatchString(s.ImageDigest) ||
		!digestPattern.MatchString(s.InitSpecDigest) {
		return ErrInvalid
	}
	return nil
}

// ManagedIdentity is immutable, non-secret lineage metadata. It is independent
// of the SQL database name, operating-system user and instance password.
type ManagedIdentity struct {
	LineageRef    string
	EngineKind    string
	PolicyVersion string
	Username      string
}

// IdentityBinding detects inconsistent metadata; its digest is not authentication.
// Callers must also compare against the trusted owner's persisted binding.
type IdentityBinding struct {
	ManagedIdentity
	IdentityDigest string
}

// Digest uses domain-separated, length-delimited SHA-256 encoding. The identical
// algorithm is used in both repositories; see managed-database-identity-internals.md.
func (i ManagedIdentity) Digest() string {
	h := sha256.New()
	for _, field := range []string{"managed-identity-binding.v1", i.LineageRef, i.EngineKind, i.PolicyVersion, i.Username} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(field)))
		h.Write(size[:])
		h.Write([]byte(field))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Validate rejects invalid shape or changed fields without a matching digest.
// This does not establish ownership: a caller can compute a digest of its own.
func (b IdentityBinding) Validate() error {
	if !referencePattern.MatchString(b.LineageRef) || b.EngineKind != "postgres" ||
		b.PolicyVersion != PolicyVersion || !usernamePattern.MatchString(b.Username) ||
		b.IdentityDigest != b.ManagedIdentity.Digest() {
		return ErrInvalid
	}
	return nil
}

// Generate consumes exactly 16 bytes from a trusted cryptographic source
// (production callers must supply crypto/rand.Reader). It neither persists nor
// initializes anything; the cache owner must atomically reserve the winner before
// use. A failed source returns no partial identity and no underlying diagnostics.
func Generate(source io.Reader) (IdentityBinding, error) {
	if source == nil {
		return IdentityBinding{}, ErrUnavailable
	}
	var entropy [16]byte
	if _, err := io.ReadFull(source, entropy[:]); err != nil {
		return IdentityBinding{}, ErrUnavailable
	}
	suffix := hex.EncodeToString(entropy[:])
	identity := ManagedIdentity{LineageRef: "lineage_" + suffix, EngineKind: "postgres", PolicyVersion: PolicyVersion, Username: "sqlrs_admin_" + suffix}
	return IdentityBinding{ManagedIdentity: identity, IdentityDigest: identity.Digest()}, nil
}
