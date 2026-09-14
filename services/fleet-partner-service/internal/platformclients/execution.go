package platformclients

import (
	"context"
	"errors"
	"time"
)

// The execution contract is provider-neutral at the fleet layer: one
// interface, three service-specific adapters. It covers the marketplace
// reservation and canonical execution operations Sections 11–12 need.
//
// Every state-changing operation carries an idempotency key and has a
// matching read-back, because a timeout can only be resolved by asking the
// source service what actually happened — never by repeating the write.

// ReserveRequest asks the source service to reserve a marketplace request for
// an organization. Only the source service decides the winner.
type ReserveRequest struct {
	SourceRequestID       string
	ExpectedSourceVersion int64
	OrganizationID        string
	AcceptanceSource      string
	// AcceptingProviderID and OrganizationMembershipID are set only for
	// provider-direct acceptance (delegated / hybrid).
	AcceptingProviderID      string
	OrganizationMembershipID string
	IdempotencyKey           string
	CorrelationID            string
}

// ReservationOutcome is the authoritative reservation result.
type ReservationOutcome struct {
	ReservationID string
	// Status is RESERVED, ALREADY_CLAIMED, NOT_ELIGIBLE, EXPIRED, CANCELLED
	// or VERSION_MISMATCH.
	Status               string
	WinnerOrganizationID string
	SourceVersion        int64
}

// Reservation outcome statuses.
const (
	ReservationReserved        = "RESERVED"
	ReservationAlreadyClaimed  = "ALREADY_CLAIMED"
	ReservationNotEligible     = "NOT_ELIGIBLE"
	ReservationExpired         = "EXPIRED"
	ReservationCancelled       = "CANCELLED"
	ReservationVersionMismatch = "VERSION_MISMATCH"
)

// ConfirmProviderRequest asks the source service to bind an organization's
// managed provider to a reserved request and create canonical execution.
//
// SettlementOrganizationID and ExecutingProviderID are deliberately distinct:
// the organization settles, the provider executes.
type ConfirmProviderRequest struct {
	SourceRequestID          string
	ExpectedSourceVersion    int64
	ReservationID            string
	OrganizationID           string
	SettlementOrganizationID string
	ExecutingProviderID      string
	OrganizationMembershipID string
	AssetOrVehicleReference  string
	FleetAssignmentID        string
	ProviderResponseID       string
	AcceptanceSource         string
	IdempotencyKey           string
	CorrelationID            string
}

// ExecutionOutcome is the authoritative execution result.
type ExecutionOutcome struct {
	// Status is CREATED, ALREADY_EXISTS, REJECTED or NOT_RESERVED.
	Status        string
	TripID        string
	RequestID     string
	SourceVersion int64
	RejectionCode string
}

// Execution outcome statuses.
const (
	ExecutionCreatedStatus  = "CREATED"
	ExecutionAlreadyExists  = "ALREADY_EXISTS"
	ExecutionRejectedStatus = "REJECTED"
	ExecutionNotReserved    = "NOT_RESERVED"
)

// SourceRequestView is the authoritative request state used by reconciliation.
type SourceRequestView struct {
	RequestID            string
	BookingID            string
	Status               string
	SourceVersion        int64
	WinnerOrganizationID string
	AcceptedProviderID   string
	ExpiresAt            time.Time
}

// SourceTripView is the authoritative trip state used by reconciliation.
type SourceTripView struct {
	TripID              string
	RequestID           string
	BookingID           string
	Status              string
	SourceVersion       int64
	ExecutingProviderID string
	CompletedAt         *time.Time
	CancelledAt         *time.Time
}

// ProviderEligibility is the source service's answer on whether a provider
// and vehicle may take this work right now.
type ProviderEligibility struct {
	Eligible       bool
	PlatformStatus string
	ReasonCode     string
	VehicleOK      bool
	SourceVersion  int64
}

// ExecutionContract is the marketplace and execution capability of one
// authoritative execution service.
type ExecutionContract interface {
	ReserveForOrganization(ctx context.Context, meta CallMeta, request ReserveRequest) (ReservationOutcome, error)
	ReadReservation(ctx context.Context, meta CallMeta, sourceRequestID string, idempotencyKey string) (ReservationOutcome, error)
	ReleaseReservation(ctx context.Context, meta CallMeta, sourceRequestID string, organizationID string, reason string, idempotencyKey string) error

	ValidateProviderEligibility(ctx context.Context, meta CallMeta, sourceRequestID string, providerID string, vehicleReference string) (ProviderEligibility, error)
	ConfirmProviderAndCreateExecution(ctx context.Context, meta CallMeta, request ConfirmProviderRequest) (ExecutionOutcome, error)
	ReadExecutionOutcome(ctx context.Context, meta CallMeta, sourceRequestID string, idempotencyKey string) (ExecutionOutcome, error)

	ReadRequest(ctx context.Context, meta CallMeta, sourceRequestID string) (SourceRequestView, error)
	ReadTrip(ctx context.Context, meta CallMeta, sourceTripID string) (SourceTripView, error)
	CancelExecution(ctx context.Context, meta CallMeta, sourceTripID string, reason string, idempotencyKey string) error

	// Configured reports whether live calls are possible. A disabled or
	// unimplemented service reports false and every call fails closed.
	Configured() bool
	// ServiceCategory is DISPATCH, TAXI or HAULING.
	ServiceCategory() string
}

// ErrExecutionNotImplemented means the authoritative service does not expose
// the marketplace contract yet. It is deliberately distinct from a transport
// failure so operators can tell "not built" from "temporarily down".
var ErrExecutionNotImplemented = errors.New("platformclients: execution service has not implemented the marketplace contract")

// UnimplementedExecution is the fail-closed adapter for an execution service
// whose marketplace and trip layers do not exist yet.
//
// Per the Sections 11–12 audit this is the honest state of
// driver-taxi-service (a scaffold), and it is also used for any source whose
// required configuration is absent. Every call fails; nothing is fabricated.
type UnimplementedExecution struct {
	Category string
	Service  string
}

func (u UnimplementedExecution) unimplemented() error {
	return &Error{
		Service: u.Service,
		Outcome: OutcomeUnknown,
		Message: "marketplace contract is not implemented by this execution service",
	}
}

// ReserveForOrganization always fails closed.
func (u UnimplementedExecution) ReserveForOrganization(context.Context, CallMeta, ReserveRequest) (ReservationOutcome, error) {
	return ReservationOutcome{}, u.unimplemented()
}

// ReadReservation always fails closed.
func (u UnimplementedExecution) ReadReservation(context.Context, CallMeta, string, string) (ReservationOutcome, error) {
	return ReservationOutcome{}, u.unimplemented()
}

// ReleaseReservation always fails closed.
func (u UnimplementedExecution) ReleaseReservation(context.Context, CallMeta, string, string, string, string) error {
	return u.unimplemented()
}

// ValidateProviderEligibility always fails closed.
func (u UnimplementedExecution) ValidateProviderEligibility(context.Context, CallMeta, string, string, string) (ProviderEligibility, error) {
	return ProviderEligibility{}, u.unimplemented()
}

// ConfirmProviderAndCreateExecution always fails closed.
func (u UnimplementedExecution) ConfirmProviderAndCreateExecution(context.Context, CallMeta, ConfirmProviderRequest) (ExecutionOutcome, error) {
	return ExecutionOutcome{}, u.unimplemented()
}

// ReadExecutionOutcome always fails closed.
func (u UnimplementedExecution) ReadExecutionOutcome(context.Context, CallMeta, string, string) (ExecutionOutcome, error) {
	return ExecutionOutcome{}, u.unimplemented()
}

// ReadRequest always fails closed.
func (u UnimplementedExecution) ReadRequest(context.Context, CallMeta, string) (SourceRequestView, error) {
	return SourceRequestView{}, u.unimplemented()
}

// ReadTrip always fails closed.
func (u UnimplementedExecution) ReadTrip(context.Context, CallMeta, string) (SourceTripView, error) {
	return SourceTripView{}, u.unimplemented()
}

// CancelExecution always fails closed.
func (u UnimplementedExecution) CancelExecution(context.Context, CallMeta, string, string, string) error {
	return u.unimplemented()
}

// Configured reports false: there is nothing to call.
func (u UnimplementedExecution) Configured() bool { return false }

// ServiceCategory returns the category this adapter stands in for.
func (u UnimplementedExecution) ServiceCategory() string { return u.Category }

// ExecutionSet bundles the execution contracts by service category.
type ExecutionSet map[string]ExecutionContract

// For returns the contract for a service category.
func (s ExecutionSet) For(category string) (ExecutionContract, bool) {
	contract, ok := s[category]
	return contract, ok
}

// AnyConfigured reports whether at least one category can actually reach its
// source service. Used by the composition root to avoid starting a worker that
// could only fail.
func (s ExecutionSet) AnyConfigured() bool {
	for _, contract := range s {
		if contract != nil && contract.Configured() {
			return true
		}
	}
	return false
}

// BuildExecutionSet assembles the execution contracts.
//
// Dispatch and Hauling get independent real adapters when their respective
// base URLs and secrets are present. Taxi deliberately remains
// UnimplementedExecution until it has a production contract.

// BuildExecutionSet accepts Dispatch options followed optionally by Hauling
// options. The variadic form preserves existing composition roots while they
// roll forward. Taxi has no production contract and remains hard-disabled.
func BuildExecutionSet(dispatchOpts Options, haulingOpts ...Options) ExecutionSet {
	opts := ExecutionSetOptions{Dispatch: dispatchOpts}
	if len(haulingOpts) > 0 {
		opts.Hauling = &haulingOpts[0]
	}
	return BuildExecutionSetWith(opts)
}

// ExecutionSetOptions is the explicit form of BuildExecutionSet.
//
// A nil per-category pointer means "no live contract": that category keeps the
// fail-closed UnimplementedExecution adapter. This is what keeps Section 8
// TAX-028 honest -- enabling the Taxi source is a deliberate act of supplying
// Taxi's options, never a side effect of Taxi merely being configured
// elsewhere in the deployment.
type ExecutionSetOptions struct {
	Dispatch Options
	Hauling  *Options
	Taxi     *Options
}

// BuildExecutionSetWith assembles the execution contracts from explicit
// per-category options.
func BuildExecutionSetWith(opts ExecutionSetOptions) ExecutionSet {
	hauling := ExecutionContract(UnimplementedExecution{Category: CategoryHauling, Service: "driver-hauling-service"})
	if opts.Hauling != nil {
		hauling = NewHaulingExecution(New(*opts.Hauling))
	}
	taxi := ExecutionContract(UnimplementedExecution{Category: CategoryTaxi, Service: "driver-taxi-service"})
	if opts.Taxi != nil {
		taxi = NewTaxiExecution(New(*opts.Taxi))
	}
	return ExecutionSet{
		CategoryDispatch: NewDispatchExecution(New(opts.Dispatch)),
		CategoryTaxi:     taxi,
		CategoryHauling:  hauling,
	}
}
