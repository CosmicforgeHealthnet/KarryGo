// Package organizationproviders is Taxi's half of the organization-managed
// provider contract (Fleet FLEET-10.2).
//
// fleet-partner-service never creates a driver of its own. It asks here:
//
//  1. resolve — is this email/phone pair already a driver? If the two contacts
//     identify different drivers the answer is CONFLICT and nothing is merged;
//  2. sponsor — when nobody matches, create ONE organization-sponsored driver
//     identity. It skips the independent onboarding screens but keeps
//     first-login security: the driver still signs in with the existing
//     passwordless email/phone OTP flow, and still has to pass the platform's
//     own verification before they can work;
//  3. associate — record a REQUESTED association that only the driver can
//     accept or decline, from their own app.
//
// Fleet reads the association back; the driver's decision is what moves the
// organization membership out of PENDING_PROVIDER_ACCEPTANCE.
package organizationproviders

import (
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/orgprovider"
)

// ServiceCategory is the category this service answers for.
const ServiceCategory = "TAXI"

// Errors. The HTTP layer maps each onto one contract status code.
var (
	ErrInvalid              = errors.New("organizationproviders: invalid request")
	ErrNotFound             = errors.New("organizationproviders: not found")
	ErrIdempotencyConflict  = errors.New("organizationproviders: idempotency key reused with a different request")
	ErrIdentityConflict     = errors.New("organizationproviders: contact already belongs to a driver")
	ErrIneligible           = errors.New("organizationproviders: provider cannot be associated")
	ErrLinkConflict         = errors.New("organizationproviders: provider already has a live organization association")
	ErrInvalidStateVersion  = errors.New("organizationproviders: invalid state or stale version")
	ErrDisabled             = errors.New("organizationproviders: integration disabled")
	ErrDecisionNotAvailable = errors.New("organizationproviders: association is not awaiting this provider")
)

// Candidate is one driver found by a contact lookup, with the status
// dimensions taxi keeps separately (addendum 11.1).
type Candidate struct {
	ProviderID                 string
	AccountStatus              string
	OnboardingStatus           string
	IdentityVerificationStatus string
	VehicleVerificationStatus  string
	DocumentComplianceStatus   string
	ManagedByOrgID             string
	SourceVersion              int64
}

// AccountState is the contract account state of a driver. ACTIVE uses the
// same definition of "fully approved" as going available: account active,
// onboarding complete, identity and vehicle verified, documents compliant.
// Sponsorship does not shortcut any of it.
func (c Candidate) AccountState() string {
	switch {
	case c.AccountStatus != "active":
		return orgprovider.AccountSuspended
	case c.OnboardingStatus == "complete" && c.IdentityVerificationStatus == "verified" &&
		c.VehicleVerificationStatus == "verified" && c.DocumentComplianceStatus == "compliant":
		return orgprovider.AccountActive
	default:
		return orgprovider.AccountPendingVerification
	}
}

func (c Candidate) match() *orgprovider.Match {
	return &orgprovider.Match{ProviderID: c.ProviderID, PlatformStatus: c.AccountState(),
		ManagedByOrgID: c.ManagedByOrgID, SourceVersion: c.SourceVersion}
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

// ProviderView converts the record into what a driver sees.
func (r LinkRecord) ProviderView() orgprovider.ProviderAssociationView {
	return orgprovider.ProviderAssociationView{
		LinkID: r.LinkID, OrganizationID: r.OrganizationID, Status: r.Status,
		SourceVersion: r.SourceVersion, RequestedAt: r.CreatedAt, DecidedAt: r.DecidedAt,
	}
}
