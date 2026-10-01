package providercompat

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// FailureKind classifies why a managed (organization-owned) operation could
// not be authorized. The distinction is the whole point of this type: a
// dependency failure must be retried, every other kind must be rejected, and
// neither may ever be downgraded into "treat the provider as independent".
//
// Sections 8/9 require a managed operation to fail CLOSED. Collapsing these
// into one error is what previously allowed a Fleet outage to look identical
// to a suspended membership.
type FailureKind string

const (
	// FailureDependency means Fleet could not be reached or did not answer in
	// time. The caller must surface a retryable dependency error, never a
	// rejection and never an independent-provider fallback.
	FailureDependency FailureKind = "FLEET_DEPENDENCY_UNAVAILABLE"
	// FailureNotManaged means Fleet authoritatively reported the provider is
	// not organization-managed, while the operation requires a managed one.
	FailureNotManaged FailureKind = "PROVIDER_NOT_ORGANIZATION_MANAGED"
	// FailureMembershipInvalid means the membership exists but is not live
	// (inactive, suspended, terminated, or any non-ACTIVE status).
	FailureMembershipInvalid FailureKind = "MEMBERSHIP_NOT_ACTIVE"
	// FailureServiceIneligible means the membership is live but does not
	// permit this service category (for example membership does not allow
	// TAXI).
	FailureServiceIneligible FailureKind = "SERVICE_NOT_ELIGIBLE"
	// FailureProviderMismatch means the authenticated provider is not the
	// provider bound to the authoritative Fleet assignment.
	FailureProviderMismatch FailureKind = "PROVIDER_ASSIGNMENT_MISMATCH"
	// FailureOrganizationMismatch means the operation named a different
	// organization than the authoritative membership's organization.
	FailureOrganizationMismatch FailureKind = "ORGANIZATION_MISMATCH"
	// FailureOperatingMode means Fleet's persisted operating mode does not
	// permit this acquisition path (for example a direct offer accept under
	// CENTRALIZED).
	FailureOperatingMode FailureKind = "OPERATING_MODE_NOT_PERMITTED"
)

// Retryable reports whether a caller should retry rather than reject. Only a
// dependency failure is retryable; every authoritative answer is final.
func (k FailureKind) Retryable() bool { return k == FailureDependency }

// ContextError carries a classified managed-operation failure.
type ContextError struct {
	Kind    FailureKind
	Service string
	Detail  string
	Err     error
}

func (e *ContextError) Error() string {
	parts := []string{string(e.Kind)}
	if e.Service != "" {
		parts = append(parts, "service="+e.Service)
	}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	out := strings.Join(parts, " ")
	if e.Err != nil {
		return out + ": " + e.Err.Error()
	}
	return out
}

func (e *ContextError) Unwrap() error { return e.Err }

// Retryable reports whether the underlying kind is retryable.
func (e *ContextError) Retryable() bool { return e != nil && e.Kind.Retryable() }

// NewContextError builds a classified failure.
func NewContextError(kind FailureKind, service, detail string, err error) *ContextError {
	return &ContextError{Kind: kind, Service: service, Detail: detail, Err: err}
}

// DependencyError wraps a transport/timeout/5xx failure as retryable.
func DependencyError(service string, err error) *ContextError {
	return NewContextError(FailureDependency, service,
		"Fleet corporate context could not be verified", err)
}

// KindOf extracts the classification from err, reporting whether err was a
// classified managed-operation failure at all. An unclassified error is
// treated as a dependency failure by callers, never as a rejection, because
// an unknown cause must not be allowed to authorize independent behaviour.
func KindOf(err error) (FailureKind, bool) {
	var ctxErr *ContextError
	if errors.As(err, &ctxErr) {
		return ctxErr.Kind, true
	}
	return "", false
}

// MembershipStatusActive is the only membership status that authorizes
// managed work.
const MembershipStatusActive = "ACTIVE"

// ManagedRequest describes one corporate-sensitive operation to authorize.
//
// Empty ExpectedProviderID / ExpectedOrganizationID mean "no authoritative
// binding was supplied for this operation", so that check is skipped. A
// supplied value that does not match is always a rejection.
type ManagedRequest struct {
	// Service is the Fleet service category, e.g. "TAXI" or "DISPATCH".
	Service string
	// ProviderID is the authenticated provider performing the operation.
	ProviderID string
	// ExpectedProviderID is the provider bound to the authoritative Fleet
	// assignment, when the operation names one.
	ExpectedProviderID string
	// ExpectedOrganizationID is the organization the operation claims, when
	// the operation names one.
	ExpectedOrganizationID string
	// RequireManaged rejects an independent provider. Operations that are
	// legitimately available to both leave this false and branch on the
	// returned context instead.
	RequireManaged bool
}

// AuthorizeManagedOperation is the single fail-closed gate every
// corporate-sensitive provider operation goes through.
//
// It never returns a usable context together with an error, and it never
// reports "independent" because Fleet was unreachable: a dependency failure
// is returned as a retryable FailureDependency so the caller answers 503,
// not 200-as-independent.
func AuthorizeManagedOperation(ctx context.Context, reader CorporateContextReader, req ManagedRequest) (CorporateContext, error) {
	service := strings.TrimSpace(req.Service)
	if service == "" {
		return CorporateContext{}, NewContextError(FailureDependency, service,
			"managed authorization requires a service category", nil)
	}
	if reader == nil {
		// Fleet integration is not configured at all. A managed-only
		// operation cannot be authorized, and must not silently succeed.
		if req.RequireManaged {
			return CorporateContext{}, NewContextError(FailureDependency, service,
				"Fleet corporate context is not configured", nil)
		}
		return CorporateContext{}, nil
	}
	if strings.TrimSpace(req.ProviderID) == "" {
		return CorporateContext{}, NewContextError(FailureProviderMismatch, service,
			"provider identity is required", nil)
	}

	corporate, err := reader.ReadCorporateContext(ctx, service, req.ProviderID)
	if err != nil {
		// An unclassified read failure is a dependency failure, never a
		// rejection and never an independent downgrade.
		if kind, ok := KindOf(err); ok && !kind.Retryable() {
			return CorporateContext{}, err
		}
		return CorporateContext{}, DependencyError(service, err)
	}

	if !corporate.IsManagedProvider {
		if req.RequireManaged {
			return CorporateContext{}, NewContextError(FailureNotManaged, service,
				"provider is not organization-managed", nil)
		}
		return corporate, nil
	}

	if err := VerifyManagedContext(corporate, req); err != nil {
		return CorporateContext{}, err
	}
	return corporate, nil
}

// VerifyManagedContext applies every authoritative managed-operation rule to
// an already-read context. Exported separately so a caller that already holds
// a fresh context (for example one enriched onto the session in the same
// request) validates it with exactly the same rules instead of a second copy.
func VerifyManagedContext(corporate CorporateContext, req ManagedRequest) error {
	service := strings.TrimSpace(req.Service)
	if !corporate.IsManagedProvider {
		if req.RequireManaged {
			return NewContextError(FailureNotManaged, service,
				"provider is not organization-managed", nil)
		}
		return nil
	}
	if status := ParseMembershipStatus(corporate.MembershipStatus); !status.AllowsNewManagedWork() {
		// The refusal names the CLASS, not just "not active": suspended,
		// expired and terminated are three different answers to the provider
		// asking why, and Section 16 handles them separately.
		return NewContextError(FailureMembershipInvalid, service,
			fmt.Sprintf("%s (status %q, class %s)",
				membershipFailureDetail(status), corporate.MembershipStatus, status.Class()), nil)
	}
	if strings.TrimSpace(corporate.MembershipID) == "" || strings.TrimSpace(corporate.OrganizationID) == "" {
		// A managed context missing its own identifiers is not usable as an
		// authority. Treated as a dependency failure, not a rejection: the
		// provider did nothing wrong and a retry may return a complete one.
		return NewContextError(FailureDependency, service,
			"managed corporate context is incomplete", nil)
	}
	if !corporate.ServiceEligibility[service] {
		return NewContextError(FailureServiceIneligible, service,
			"membership does not permit this service", nil)
	}
	if expected := strings.TrimSpace(req.ExpectedProviderID); expected != "" && expected != strings.TrimSpace(req.ProviderID) {
		return NewContextError(FailureProviderMismatch, service,
			"provider does not match the authoritative assignment", nil)
	}
	if expected := strings.TrimSpace(req.ExpectedOrganizationID); expected != "" && expected != strings.TrimSpace(corporate.OrganizationID) {
		return NewContextError(FailureOrganizationMismatch, service,
			"organization does not match the authoritative membership", nil)
	}
	return nil
}
