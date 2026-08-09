// Package tenant defines the typed tenant context and the trusted resolution
// boundary for organization isolation.
//
// Tenant context is resolved only from trusted authentication (verified
// access-token claims plus organization-membership lookup) or from validated
// internal service claims. Organization IDs supplied in request bodies,
// query strings or ad-hoc headers never override it. Missing tenant context
// fails closed.
package tenant

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ActorType enumerates who is acting.
type ActorType string

const (
	ActorTypeOrganizationUser      ActorType = "organization_user"
	ActorTypeOrganizationAPIClient ActorType = "organization_api_client"
	ActorTypePlatformService       ActorType = "platform_service"
	ActorTypeSystem                ActorType = "system"
)

// Context is the resolved tenant scope of a request, event or job.
type Context struct {
	OrganizationID     string
	OrganizationUserID string
	ActorType          ActorType
	ActorID            string
	PermittedBranchIDs []string
	RequestID          string
	CorrelationID      string
}

// Validate ensures the context is usable for tenant-scoped work.
func (t Context) Validate() error {
	if t.OrganizationID == "" {
		return errors.New("tenant: organization id is required")
	}
	if t.ActorType == "" || t.ActorID == "" {
		return errors.New("tenant: actor is required")
	}
	return nil
}

// BranchPermitted reports whether the actor may act on the given branch.
// An empty PermittedBranchIDs slice means "all branches of the organization".
func (t Context) BranchPermitted(branchID string) bool {
	if len(t.PermittedBranchIDs) == 0 {
		return true
	}
	for _, id := range t.PermittedBranchIDs {
		if id == branchID {
			return true
		}
	}
	return false
}

// Membership statuses.
const (
	MembershipStatusActive    = "ACTIVE"
	MembershipStatusSuspended = "SUSPENDED"
)

// Membership is the resolved membership of an authenticated user. Only
// ACTIVE memberships grant tenant scope.
type Membership struct {
	OrganizationID     string
	OrganizationUserID string
	Status             string
	PermittedBranchIDs []string
}

// ErrMembershipNotFound means the authenticated subject has no active
// organization membership.
var ErrMembershipNotFound = errors.New("tenant: no active organization membership")

// ErrMembershipSuspended means the subject's membership exists but is not
// active; tenant-scoped access is refused.
var ErrMembershipSuspended = errors.New("tenant: organization membership is suspended")

// ErrResolverUnconfigured means membership resolution infrastructure is not
// configured yet (staff/IAM work package is deferred). Tenant-scoped routes
// fail closed with this error.
var ErrResolverUnconfigured = errors.New("tenant: membership resolver is not configured")

// MembershipResolver resolves the active organization membership for an
// authenticated subject. Implementations must query trusted local state only.
type MembershipResolver interface {
	ResolveMembership(ctx context.Context, subjectID string) (Membership, error)
}

// UnconfiguredResolver fails closed until the staff/IAM work package provides
// a real membership store.
type UnconfiguredResolver struct{}

// ResolveMembership always returns ErrResolverUnconfigured.
func (UnconfiguredResolver) ResolveMembership(context.Context, string) (Membership, error) {
	return Membership{}, ErrResolverUnconfigured
}

type contextKey struct{}

// Into stores the tenant context in a context.Context.
func Into(ctx context.Context, t Context) context.Context {
	return context.WithValue(ctx, contextKey{}, t)
}

// From extracts the tenant context; ok is false when absent.
func From(ctx context.Context) (Context, bool) {
	t, ok := ctx.Value(contextKey{}).(Context)
	return t, ok
}

// Require extracts the tenant context or fails closed with a forbidden error.
func Require(ctx context.Context) (Context, error) {
	t, ok := From(ctx)
	if !ok {
		return Context{}, apperrors.Forbidden("Organization scope is required.", nil)
	}
	if err := t.Validate(); err != nil {
		return Context{}, apperrors.Forbidden("Organization scope is required.", err)
	}
	return t, nil
}

// Query is the tenant-scoped query object required by list repositories.
type Query struct {
	OrganizationID     string
	PermittedBranchIDs []string
	Limit              int
	CursorSortValue    string
	CursorLastID       string
}
