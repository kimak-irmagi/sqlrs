package managedidentity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// RuntimeBinding ties an immutable lineage to the assigned runtime and physical
// clone. Neither identifier may be inferred from recipe-controlled PGDATA files.
// See docs/architecture/managed-database-identity-internals.md.
type RuntimeBinding struct {
	RuntimeRef       string
	PhysicalIdentity string
	IdentityBinding
}

// Validate checks bounded identifiers and identity integrity. The coordinator
// must additionally match this value to the current durable operation.
func (b RuntimeBinding) Validate() error {
	if !referencePattern.MatchString(b.RuntimeRef) || !referencePattern.MatchString(b.PhysicalIdentity) {
		return ErrInvalid
	}
	return b.IdentityBinding.Validate()
}

// BaseKey binds immutable initialization inputs to the cache owner's reserved
// identity. Descendant keys inherit this input through their parent. Passwords
// and password verifiers are deliberately absent from this typed interface.
func BaseKey(selector BaseSelector, identity IdentityBinding) (string, error) {
	if selector.Validate() != nil || identity.Validate() != nil {
		return "", ErrInvalid
	}
	h := sha256.New()
	for _, field := range []string{"managed-base-key.v1", selector.DomainRef, selector.EngineKind, selector.ImageDigest, selector.InitSpecDigest, selector.PolicyVersion, identity.IdentityDigest} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(field)))
		h.Write(size[:])
		h.Write([]byte(field))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
