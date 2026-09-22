package managedidentity

import "context"

// BaseSelection is the cache owner's non-secret resolution, shared by planning
// and execution. Key is derived from the exact selector and committed identity.
// See docs/architecture/managed-database-identity-internals.md, cache and recovery.
type BaseSelection struct {
	IdentityBinding
	Key string
}

// Owner fixes the authorized store domain and effective initialization policy.
// Callers supply an immutable image digest, never a username or password.
type Owner struct {
	service    *Service
	domain     string
	initDigest string
}

func NewOwner(service *Service, domain, initDigest string) (*Owner, error) {
	if service == nil || !referencePattern.MatchString(domain) || !digestPattern.MatchString(initDigest) {
		return nil, ErrInvalid
	}
	return &Owner{service: service, domain: domain, initDigest: initDigest}, nil
}

func (o *Owner) selector(imageDigest string) BaseSelector {
	return BaseSelector{DomainRef: o.domain, EngineKind: "postgres", ImageDigest: imageDigest, InitSpecDigest: o.initDigest, PolicyVersion: PolicyVersion}
}

// ResolveBase may reserve metadata only. The identity service has no runtime,
// queue, state-publication or secret-storage capability.
func (o *Owner) ResolveBase(ctx context.Context, imageDigest string) (BaseSelection, error) {
	selector := o.selector(imageDigest)
	binding, err := o.service.ResolveOrReserveBase(ctx, selector)
	if err != nil {
		return BaseSelection{}, err
	}
	return selectedBase(selector, binding), nil
}

// RestoreBase is read-only even when a committed job's lineage disappeared.
// Looking up the full original selector prevents changing policy/image/domain
// under an otherwise valid lineage. Missing metadata never triggers generation.
func (o *Owner) RestoreBase(ctx context.Context, imageDigest string, expected IdentityBinding) (BaseSelection, error) {
	if expected.Validate() != nil {
		return BaseSelection{}, ErrInvalid
	}
	return o.RestoreStored(ctx, imageDigest, expected.LineageRef, expected.IdentityDigest)
}

// RestoreStored resolves the normalized reference/digest retained by a queued
// job. The authoritative row supplies the username; the job cannot override it.
func (o *Owner) RestoreStored(ctx context.Context, imageDigest, lineage, digest string) (BaseSelection, error) {
	selector := o.selector(imageDigest)
	if selector.Validate() != nil || !referencePattern.MatchString(lineage) || !digestPattern.MatchString(digest) {
		return BaseSelection{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return BaseSelection{}, err
	}
	record, err := o.service.store.Find(ctx, selector)
	if err != nil {
		return BaseSelection{}, safeStoreError(err)
	}
	binding, err := resolved(ctx, record, selector, lineage)
	if err != nil {
		return BaseSelection{}, err
	}
	if binding.IdentityDigest != digest {
		return BaseSelection{}, ErrInvalid
	}
	return selectedBase(selector, binding), nil
}

// selectedBase only accepts values validated by ResolveOrReserveBase/resolved.
func selectedBase(selector BaseSelector, binding IdentityBinding) BaseSelection {
	key, _ := BaseKey(selector, binding)
	return BaseSelection{IdentityBinding: binding, Key: key}
}
