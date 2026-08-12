package platformclients

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The fleet side of the vehicle authority contract.
//
// Fleet does not own vehicles and holds no vehicle table. Every question about
// a vehicle — does it exist, who controls it, is it verified, is it available,
// is it restricted, are its documents in order, is it already committed — is
// answered by the service that owns it, over the private HMAC-signed contract
// that service exposes.
//
// There is deliberately no local fallback and no cache. A cached "verified"
// would outlive a suspension, and a fallback would be fleet deciding a
// question it has no standing to decide.

// VehicleDocumentStatus is one required document's standing at the authority.
type VehicleDocumentStatus struct {
	Type      string     `json:"type"`
	Present   bool       `json:"present"`
	Expired   bool       `json:"expired"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// VehicleAuthorityState is the authoritative answer about one vehicle.
//
// Field names mirror the owning service's contract exactly. Renaming them here
// would make a contract change look like a local refactor.
type VehicleAuthorityState struct {
	VehicleID  string `json:"vehicleId"`
	ProviderID string `json:"providerId"`

	ControlState  string `json:"controlState"`
	ProviderMatch *bool  `json:"providerMatch,omitempty"`

	ServiceType      string `json:"serviceType"`
	ServiceTypeMatch *bool  `json:"serviceTypeMatch,omitempty"`

	VerificationState string `json:"verificationState"`
	Available         bool   `json:"available"`
	AvailabilityState string `json:"availabilityState"`
	Restricted        bool   `json:"restricted"`
	RestrictionState  string `json:"restrictionState"`

	RequiredDocuments  []VehicleDocumentStatus `json:"requiredDocuments"`
	DocumentsSatisfied bool                    `json:"documentsSatisfied"`

	ActiveAssignmentConflict bool   `json:"activeAssignmentConflict"`
	AssignmentConflictBasis  string `json:"assignmentConflictBasis"`

	SourceVersion int64     `json:"sourceVersion"`
	ObservedAt    time.Time `json:"observedAt"`
}

// Verification states in the vehicle authority contract.
const (
	VehicleVerificationVerified   = "VERIFIED"
	VehicleVerificationPending    = "PENDING"
	VehicleVerificationUnverified = "UNVERIFIED"
	VehicleVerificationRejected   = "REJECTED"
	VehicleVerificationSuspended  = "SUSPENDED"
)

// VehicleLookup is one authority question.
type VehicleLookup struct {
	VehicleID string
	// ExpectedProviderID, when set, makes the authority answer the ownership
	// question directly instead of fleet inferring it from a returned id.
	ExpectedProviderID string
	// ExpectedServiceType, when set, makes the authority answer the service
	// match directly.
	ExpectedServiceType string
}

// ErrVehicleNotFound means the authority does not know this vehicle.
//
// Distinct from a transport failure on purpose: "the authority says no such
// vehicle" is a definitive answer fleet records as INELIGIBLE, while "the
// authority could not be reached" is an UNKNOWN that must fail closed without
// claiming the vehicle is bad.
var ErrVehicleNotFound = &Error{Service: "vehicle-authority", Outcome: OutcomeNotFound,
	Status: http.StatusNotFound, Message: "vehicle not found"}

// VehicleAuthority is the capability fleet needs from the vehicle owner.
type VehicleAuthority interface {
	// LookupVehicle returns the authoritative state, ErrVehicleNotFound when
	// the vehicle does not exist, or an error when the answer is unavailable.
	LookupVehicle(ctx context.Context, meta CallMeta, lookup VehicleLookup) (VehicleAuthorityState, error)
	// AuthorityService names the owning service, recorded on every decision so
	// a later reader knows who was asked.
	AuthorityService() string
	Configured() bool
}

// DispatchVehicleAuthorityBase is the prefix the dispatch service mounts.
const DispatchVehicleAuthorityBase = "/api/v1/internal/authority"

// DispatchVehicleAuthorityClient reads vehicles from
// driver-dispatch-delivery-service.
type DispatchVehicleAuthorityClient struct {
	client  *Client
	apiBase string
	service string
}

// NewDispatchVehicleAuthorityClient builds the client.
func NewDispatchVehicleAuthorityClient(opts Options) *DispatchVehicleAuthorityClient {
	if opts.ServiceName == "" {
		opts.ServiceName = "driver-dispatch-delivery-service"
	}
	return &DispatchVehicleAuthorityClient{client: New(opts), apiBase: DispatchVehicleAuthorityBase, service: "driver-dispatch-delivery-service"}
}

// NewHaulingVehicleAuthorityClient targets Hauling's private provider/truck
// lookup surface. The shared type keeps the fail-closed response validation
// identical across authorities.
func NewHaulingVehicleAuthorityClient(opts Options) *DispatchVehicleAuthorityClient {
	if opts.ServiceName == "" {
		opts.ServiceName = "driver-hauling-service"
	}
	return &DispatchVehicleAuthorityClient{client: New(opts), apiBase: haulingInternalBase, service: "driver-hauling-service"}
}

// WithAPIBase overrides the mounted prefix.
func (c *DispatchVehicleAuthorityClient) WithAPIBase(base string) *DispatchVehicleAuthorityClient {
	if base != "" {
		c.apiBase = strings.TrimSuffix(base, "/")
	}
	return c
}

// AuthorityService names the owning service.
func (c *DispatchVehicleAuthorityClient) AuthorityService() string {
	return c.service
}

// Configured reports whether live calls are possible.
func (c *DispatchVehicleAuthorityClient) Configured() bool {
	return c != nil && c.client.Configured()
}

// vehicleAuthorityEnvelope matches the owning service's response shape.
type vehicleAuthorityEnvelope struct {
	Data VehicleAuthorityState `json:"data"`
}

// LookupVehicle asks the authority about one vehicle.
//
// The base client signs the request, attaches a fresh single-use nonce and
// propagates correlation and causation identifiers; the authority rejects a
// replay of either. Nothing about the vehicle is cached here — the answer is
// carried straight into an eligibility decision that records its own expiry.
func (c *DispatchVehicleAuthorityClient) LookupVehicle(ctx context.Context, meta CallMeta,
	lookup VehicleLookup) (VehicleAuthorityState, error) {
	if strings.TrimSpace(lookup.VehicleID) == "" {
		return VehicleAuthorityState{}, ErrVehicleNotFound
	}
	if !c.Configured() {
		return VehicleAuthorityState{}, ErrNotConfigured
	}

	path := c.apiBase + "/vehicles/" + url.PathEscape(strings.TrimSpace(lookup.VehicleID))
	query := url.Values{}
	if provider := strings.TrimSpace(lookup.ExpectedProviderID); provider != "" {
		query.Set("providerId", provider)
	}
	if serviceType := strings.TrimSpace(lookup.ExpectedServiceType); serviceType != "" {
		query.Set("serviceType", serviceType)
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var envelope vehicleAuthorityEnvelope
	if err := c.client.Do(ctx, meta, http.MethodGet, path, nil, &envelope); err != nil {
		if clientErr, ok := err.(*Error); ok && clientErr.Outcome == OutcomeNotFound {
			return VehicleAuthorityState{}, ErrVehicleNotFound
		}
		return VehicleAuthorityState{}, err
	}
	// A 200 carrying no vehicle identity is not a usable answer. Treating it as
	// one would let a misconfigured proxy returning an empty body read as an
	// eligible vehicle.
	if strings.TrimSpace(envelope.Data.VehicleID) == "" {
		return VehicleAuthorityState{}, &Error{Service: c.AuthorityService(),
			Outcome: OutcomeUnknown, Status: http.StatusOK,
			Message: "authority returned no vehicle identity"}
	}
	return envelope.Data, nil
}

// Compile-time proof that the adapter satisfies the port.
var _ VehicleAuthority = (*DispatchVehicleAuthorityClient)(nil)
