package models

import "time"

// Delegation statuses. Status is derived from the record and the clock, never
// stored, so a delegation cannot be "ACTIVE" in a column after it expired.
const (
	DelegationScheduled = "SCHEDULED"
	DelegationActive    = "ACTIVE"
	DelegationExpired   = "EXPIRED"
	DelegationRevoked   = "REVOKED"
)

// MaxDelegationDuration bounds how long one delegation may last. Cover for an
// absence is time-bounded by definition; a standing grant is a role binding.
const MaxDelegationDuration = 90 * 24 * time.Hour

// Delegation lets a delegate decide approvals on the delegator's authority
// for a bounded time and scope.
//
// It grants nothing of its own. At decision time the delegate may act only
// with the authority the delegator still holds through their own role
// bindings, so a delegation can never exceed what the delegator possesses,
// and authority received by delegation can never be passed on again.
type Delegation struct {
	ID               string
	OrganizationID   string
	DelegatorID      string
	DelegateID       string
	ResourceType     *string
	Action           *string
	BranchID         *string
	EffectiveFrom    time.Time
	ExpiresAt        time.Time
	Reason           string
	RevokedAt        *time.Time
	RevokedBy        *string
	RevocationReason *string
	CreatedBy        string
	IdempotencyKey   *string
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Status reports the delegation's state at now.
func (d Delegation) Status(now time.Time) string {
	switch {
	case d.RevokedAt != nil:
		return DelegationRevoked
	case !now.Before(d.ExpiresAt):
		return DelegationExpired
	case now.Before(d.EffectiveFrom):
		return DelegationScheduled
	default:
		return DelegationActive
	}
}

// Covers reports whether the delegation applies, at now, to a request for
// resourceType/action raised in branchID. An unset scope field matches any
// value; a branch-scoped delegation never covers an organization-wide request.
func (d Delegation) Covers(resourceType string, action string, branchID string, now time.Time) bool {
	if d.Status(now) != DelegationActive {
		return false
	}
	if d.ResourceType != nil && *d.ResourceType != resourceType {
		return false
	}
	if d.Action != nil && *d.Action != action {
		return false
	}
	if d.BranchID != nil && *d.BranchID != branchID {
		return false
	}
	return true
}
