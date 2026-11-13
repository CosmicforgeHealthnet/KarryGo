// Package orgprovider is the wire contract between fleet-partner-service and
// the three provider-owning services (driver-dispatch-delivery-service,
// driver-taxi-service, driver-hauling-service) for organization-managed
// providers.
//
// It holds only what must be identical on both sides of the contract: the
// association (provider link) state machine, the resolve outcomes, the
// onboarding modes and the JSON shapes. Persistence, authentication and the
// rules about who may be linked stay in each owning service, because each one
// owns its own provider table and its own notion of a verified provider.
//
// The two decisions that matter:
//
//   - an organization can REQUEST an association, but only the provider can
//     move it from REQUESTED to ACTIVE or REJECTED (ProviderDecision);
//   - resolution never merges: if the email and the phone identify two
//     different providers the answer is CONFLICT and nothing is created.
package orgprovider

import (
	"strings"
	"time"
)

// Association (link) states. These values are persisted by the owning
// services and read back by Fleet, so renaming one is a breaking change.
const (
	StateRequested = "REQUESTED"
	StateActive    = "ACTIVE"
	StateSuspended = "SUSPENDED"
	StateRevoked   = "REVOKED"
	StateRejected  = "REJECTED"
)

// transitions is the complete association state machine. Anything absent is
// refused. REVOKED and REJECTED are terminal.
var transitions = map[string]map[string]bool{
	StateRequested: {StateActive: true, StateRejected: true, StateRevoked: true},
	StateActive:    {StateSuspended: true, StateRevoked: true},
	StateSuspended: {StateActive: true, StateRevoked: true},
}

// CanTransition reports whether from -> to is a legal association change.
func CanTransition(from, to string) bool {
	return transitions[strings.ToUpper(strings.TrimSpace(from))][strings.ToUpper(strings.TrimSpace(to))]
}

// Live reports whether a state still binds the provider to the organization.
// One provider may hold at most one live association per service.
func Live(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case StateRequested, StateActive, StateSuspended:
		return true
	}
	return false
}

// ProviderDecision is the only transition a provider can make: accepting or
// declining a REQUESTED association. Organizations use TransitionRequest for
// everything else (suspend, reactivate, revoke) and can never set ACTIVE on a
// REQUESTED association themselves, because the company cannot consent on the
// provider's behalf.
func ProviderDecision(accept bool) string {
	if accept {
		return StateActive
	}
	return StateRejected
}

// OrganizationMayTransition reports whether an organization (as opposed to the
// provider) may make from -> to. It is CanTransition minus the provider's
// consent decision.
func OrganizationMayTransition(from, to string) bool {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == StateRequested && (to == StateActive || to == StateRejected) {
		return false
	}
	return CanTransition(from, to)
}

// Resolve outcomes.
const (
	ResolveMatched  = "MATCHED"
	ResolveNotFound = "NOT_FOUND"
	ResolveConflict = "CONFLICT"
)

// Onboarding modes recorded on a provider identity.
const (
	OnboardingIndependent = "INDEPENDENT"
	OnboardingSponsored   = "ORGANIZATION_SPONSORED"
)

// Provider account states reported to Fleet. ACTIVE is the only state that
// passes Fleet's provider_platform_status_active activation gate; it means the
// owning service considers the provider fully verified and able to work.
// Organization sponsorship never shortcuts it.
const (
	AccountActive              = "ACTIVE"
	AccountPendingVerification = "PENDING_VERIFICATION"
	AccountSuspended           = "SUSPENDED"
	AccountInactive            = "INACTIVE"
)

// Error codes returned in {"error":{"code":...}} bodies.
const (
	CodeInvalidRequest       = "INVALID_REQUEST"
	CodeNotFound             = "NOT_FOUND"
	CodeIdempotencyConflict  = "IDEMPOTENCY_CONFLICT"
	CodeIdentityConflict     = "PROVIDER_IDENTITY_CONFLICT"
	CodeProviderIneligible   = "PROVIDER_INELIGIBLE"
	CodeLinkConflict         = "LINK_CONFLICT"
	CodeInvalidStateVersion  = "INVALID_STATE_OR_VERSION"
	CodeIntegrationDisabled  = "ORGANIZATION_PROVIDERS_DISABLED"
	CodeWriteHeadersRequired = "WRITE_HEADERS_REQUIRED"
	CodeAuthorityUnavailable = "AUTHORITY_UNAVAILABLE"
)

// ResolveRequest looks a person up by BOTH normalized contacts.
type ResolveRequest struct {
	EmailNormalized string `json:"emailNormalized"`
	PhoneE164       string `json:"phoneE164"`
}

// ResolveResponse is the outcome of a resolve call. It deliberately carries
// no name, email, phone or profile data.
type ResolveResponse struct {
	Outcome         string `json:"outcome"`
	ProviderID      string `json:"providerId,omitempty"`
	PlatformStatus  string `json:"platformStatus,omitempty"`
	ManagedByOrgID  string `json:"managedByOrgId,omitempty"`
	ServiceCategory string `json:"serviceCategory"`
	SourceVersion   int64  `json:"sourceVersion,omitempty"`
}

// SponsoredRequest asks the owning service to create an
// organization-sponsored identity. It is sent only after a NOT_FOUND resolve.
type SponsoredRequest struct {
	OrganizationID  string `json:"organizationId"`
	LegalName       string `json:"legalName"`
	EmailNormalized string `json:"emailNormalized"`
	PhoneE164       string `json:"phoneE164"`
	ServiceCategory string `json:"serviceCategory"`
	OnboardingMode  string `json:"onboardingMode"`
	ProviderMode    string `json:"providerMode"`
}

// ProviderView is the identity projection Fleet stores.
type ProviderView struct {
	ProviderID    string `json:"providerId"`
	AccountState  string `json:"accountState"`
	SourceVersion int64  `json:"sourceVersion"`
}

// LinkRequest asks the owning service to record a REQUESTED association.
type LinkRequest struct {
	OrganizationID string `json:"organizationId"`
	BranchID       string `json:"branchId,omitempty"`
	MembershipID   string `json:"membershipId"`
	ProviderID     string `json:"providerId"`
}

// LinkView is the authoritative association state.
type LinkView struct {
	LinkID         string     `json:"linkId"`
	OrganizationID string     `json:"organizationId"`
	BranchID       string     `json:"branchId,omitempty"`
	MembershipID   string     `json:"membershipId,omitempty"`
	ProviderID     string     `json:"providerId,omitempty"`
	Status         string     `json:"status"`
	SourceVersion  int64      `json:"sourceVersion"`
	DecidedAt      *time.Time `json:"decidedAt,omitempty"`
}

// TransitionRequest is an organization-initiated association change.
type TransitionRequest struct {
	State           string `json:"state"`
	ExpectedVersion int64  `json:"expectedVersion"`
	OrganizationID  string `json:"organizationId"`
	BranchID        string `json:"branchId,omitempty"`
	ReasonCode      string `json:"reasonCode,omitempty"`
}

// ProviderAssociationView is what a provider sees about an organization that
// asked to manage them. The organization's display name is Fleet's and is not
// stored by the owning service, so only identifiers are returned.
type ProviderAssociationView struct {
	LinkID         string     `json:"linkId"`
	OrganizationID string     `json:"organizationId"`
	Status         string     `json:"status"`
	SourceVersion  int64      `json:"sourceVersion"`
	RequestedAt    time.Time  `json:"requestedAt"`
	DecidedAt      *time.Time `json:"decidedAt,omitempty"`
}

// ProviderDecisionRequest is the provider's own accept/decline body.
type ProviderDecisionRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

// NormalizeEmail trims and lowercases an email address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// SplitLegalName splits a legal name into a first name and the remainder.
func SplitLegalName(name string) (first, last string) {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "", ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}
