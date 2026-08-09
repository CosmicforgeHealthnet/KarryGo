package platformclients

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The fleet side of the provider authority contract.
//
// Fleet owns the organization's RELATIONSHIP with a provider — the invitation,
// the branch, the employment type, the acceptance. It does not own the
// provider's platform standing, and this client is how it asks.
//
// The split matters at exactly one moment: an organization wants to put
// somebody on a customer's job. Fleet can answer "we invited them and they
// accepted". Only the owning service can answer "their identity check passed,
// their licence has not lapsed, they are not suspended, and they are not
// already on another job". Both halves are required, and neither is allowed to
// stand in for the other.

// ProviderAccountState values in the contract.
const (
	ProviderAccountActive     = "ACTIVE"
	ProviderAccountInactive   = "INACTIVE"
	ProviderAccountIncomplete = "ONBOARDING_INCOMPLETE"
)

// ProviderAuthorityState is the authoritative standing of one provider.
type ProviderAuthorityState struct {
	ProviderID string `json:"providerId"`

	AccountState      string `json:"accountState"`
	VerificationState string `json:"verificationState"`

	ServiceType      string `json:"serviceType"`
	ServiceTypeMatch *bool  `json:"serviceTypeMatch,omitempty"`

	Available         bool   `json:"available"`
	AvailabilityState string `json:"availabilityState"`
	ShiftOpen         bool   `json:"shiftOpen"`

	Restricted       bool   `json:"restricted"`
	RestrictionState string `json:"restrictionState"`

	RequiredDocuments  []VehicleDocumentStatus `json:"requiredDocuments"`
	DocumentsSatisfied bool                    `json:"documentsSatisfied"`

	ActiveAssignmentConflict bool   `json:"activeAssignmentConflict"`
	AssignmentConflictBasis  string `json:"assignmentConflictBasis"`

	SourceVersion int64     `json:"sourceVersion"`
	ObservedAt    time.Time `json:"observedAt"`
}

// ProviderLookup is one provider authority question.
type ProviderLookup struct {
	ProviderID          string
	ExpectedServiceType string
}

// ErrProviderNotFound means the authority does not know this provider.
var ErrProviderNotFound = &Error{Service: "provider-authority", Outcome: OutcomeNotFound,
	Status: http.StatusNotFound, Message: "provider not found"}

// ProviderAuthority is the capability fleet needs from the provider owner.
type ProviderAuthority interface {
	LookupProvider(ctx context.Context, meta CallMeta, lookup ProviderLookup) (ProviderAuthorityState, error)
	AuthorityService() string
	Configured() bool
}

// providerAuthorityEnvelope matches the owning service's response shape.
type providerAuthorityEnvelope struct {
	Data ProviderAuthorityState `json:"data"`
}

// LookupProvider asks the authority about one provider.
//
// A 404 is a definitive answer — this provider does not exist — and is
// returned as ErrProviderNotFound so the caller records INELIGIBLE. Every
// other failure is returned as-is so the caller records UNKNOWN and blocks,
// because an unreachable authority is not evidence of anything.
func (c *DispatchVehicleAuthorityClient) LookupProvider(ctx context.Context, meta CallMeta,
	lookup ProviderLookup) (ProviderAuthorityState, error) {
	if strings.TrimSpace(lookup.ProviderID) == "" {
		return ProviderAuthorityState{}, ErrProviderNotFound
	}
	if !c.Configured() {
		return ProviderAuthorityState{}, ErrNotConfigured
	}

	path := c.apiBase + "/providers/" + url.PathEscape(strings.TrimSpace(lookup.ProviderID))
	if serviceType := strings.TrimSpace(lookup.ExpectedServiceType); serviceType != "" {
		query := url.Values{}
		query.Set("serviceType", serviceType)
		path += "?" + query.Encode()
	}

	var envelope providerAuthorityEnvelope
	if err := c.client.Do(ctx, meta, http.MethodGet, path, nil, &envelope); err != nil {
		var clientErr *Error
		if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeNotFound {
			return ProviderAuthorityState{}, ErrProviderNotFound
		}
		return ProviderAuthorityState{}, err
	}
	if strings.TrimSpace(envelope.Data.ProviderID) == "" {
		return ProviderAuthorityState{}, &Error{Service: c.AuthorityService(),
			Outcome: OutcomeUnknown, Status: http.StatusOK,
			Message: "authority returned no provider identity"}
	}
	return envelope.Data, nil
}

// Compile-time proof that the adapter satisfies the port.
var _ ProviderAuthority = (*DispatchVehicleAuthorityClient)(nil)
