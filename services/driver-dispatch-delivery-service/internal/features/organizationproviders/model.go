// Package organizationproviders is Dispatch's half of the organization-managed
// provider contract (Fleet FLEET-10.2).
//
// fleet-partner-service never creates a rider of its own. It asks here:
//
//  1. resolve — is this email/phone pair already a rider? If the two contacts
//     identify different riders the answer is CONFLICT and nothing is merged;
//  2. sponsor — when nobody matches, create ONE organization-sponsored rider
//     identity. It skips the independent onboarding screens but keeps
//     first-login security: the rider still signs in with the existing
//     passwordless email/phone OTP flow, and still has to pass the platform's
//     own verification before they can work;
//  3. associate — record a REQUESTED association that only the rider can
//     accept or decline, from their own app.
//
// Fleet reads the association back; the rider's decision is what moves the
// organization membership out of PENDING_PROVIDER_ACCEPTANCE.
package organizationproviders

import (
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/orgprovider"
)

// ServiceCategory is the category this service answers for.
const ServiceCategory = "DISPATCH"

// Errors. The HTTP layer maps each onto one contract status code.
var (
	ErrInvalid              = errors.New("organizationproviders: invalid request")
	ErrNotFound             = errors.New("organizationproviders: not found")
	ErrIdempotencyConflict  = errors.New("organizationproviders: idempotency key reused with a different request")
	ErrIdentityConflict     = errors.New("organizationproviders: contact already belongs to a rider")
	ErrIneligible           = errors.New("organizationproviders: provider cannot be associated")
	ErrLinkConflict         = errors.New("organizationproviders: provider already has a live organization association")
	ErrInvalidStateVersion  = errors.New("organizationproviders: invalid state or stale version")
	ErrDisabled             = errors.New("organizationproviders: integration disabled")
	ErrDecisionNotAvailable = errors.New("organizationproviders: association is not awaiting this provider")
)

// Candidate is one rider found by a contact lookup.
type Candidate struct {
	ProviderID         string
	IsActive           bool
	VerificationStatus string
	OnboardingComplete bool
	ManagedByOrgID     string
	SourceVersion      int64
}

// AccountState is the contract account state of a rider. Only a fully
// verified, active, onboarded rider is ACTIVE; sponsorship does not change
// that.
func (c Candidate) AccountState() string {
	switch {
	case !c.IsActive:
		return orgprovider.AccountInactive
	case c.VerificationStatus == "suspended" || c.VerificationStatus == "rejected":
		return orgprovider.AccountSuspended
	case c.VerificationStatus == "verified" && c.OnboardingComplete:
		return orgprovider.AccountActive
	default:
		return orgprovider.AccountPendingVerification
	}
}

// Sponsored is a validated sponsored-identity request.
type Sponsored struct {
	OrganizationID string
	FirstName      string
	LastName       string
	Email          string
	Phone          string
}

// LinkRecord is a stored association with its creation time.
type LinkRecord struct {
	orgprovider.LinkView
	CreatedAt time.Time
}

// ProviderView converts the record into what a rider sees.
func (r LinkRecord) ProviderView() orgprovider.ProviderAssociationView {
	return orgprovider.ProviderAssociationView{
		LinkID: r.LinkID, OrganizationID: r.OrganizationID, Status: r.Status,
		SourceVersion: r.SourceVersion, RequestedAt: r.CreatedAt, DecidedAt: r.DecidedAt,
	}
}
