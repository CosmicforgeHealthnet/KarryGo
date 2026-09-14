// Package managedexec implements Taxi's side of the Fleet private-execution
// authority (Section 8 TAX-012 / TAX-013 / TAX-014).
//
// Taxi does NOT invent a second protocol here. Fleet already defines one
// provider-neutral execution contract that Dispatch and Hauling implement
// (platformclients.ExecutionContract); this package exposes the same
// operations, on the same request/response shapes, under Taxi's own internal
// marketplace base path. Fleet's TaxiExecution adapter is the other half.
//
// The flow this package enables, end to end:
//
//	Fleet assignment -> Taxi execution reservation -> provider accepts
//	  -> Fleet authorizes execution -> Taxi creates/links the execution
//	  -> Taxi starts the NORMAL taxi lifecycle
//
// Nothing here owns a lifecycle of its own. Once an execution is created the
// booking is an ordinary taxi booking with corporate metadata attached, and
// every existing lifecycle guard continues to apply unchanged.
package managedexec

import (
	"errors"
	"strings"
	"time"
)

// Reservation statuses. RESERVED is a live claim, CONFIRMED means an
// execution has been bound to it, and the rest are terminal releases.
const (
	StatusReserved  = "RESERVED"
	StatusConfirmed = "CONFIRMED"
	StatusReleased  = "RELEASED"
	StatusExpired   = "EXPIRED"
	StatusCancelled = "CANCELLED"
)

// Reservation outcome statuses, matching Fleet's ReservationOutcome contract
// exactly. Taxi never invents a status Fleet cannot interpret.
const (
	OutcomeReserved        = "RESERVED"
	OutcomeAlreadyClaimed  = "ALREADY_CLAIMED"
	OutcomeNotEligible     = "NOT_ELIGIBLE"
	OutcomeExpired         = "EXPIRED"
	OutcomeCancelled       = "CANCELLED"
	OutcomeVersionMismatch = "VERSION_MISMATCH"
)

// Execution outcome statuses, matching Fleet's ExecutionOutcome contract.
const (
	ExecutionCreated       = "CREATED"
	ExecutionAlreadyExists = "ALREADY_EXISTS"
	ExecutionRejected      = "REJECTED"
	ExecutionNotReserved   = "NOT_RESERVED"
)

// ErrNotFound is returned when a reservation or execution does not exist.
var ErrNotFound = errors.New("managedexec: not found")

// ErrIdempotencyConflict is returned when an idempotency key is replayed with
// materially different input. Replaying the SAME input always returns the
// original result instead.
var ErrIdempotencyConflict = errors.New("managedexec: idempotency key reused with different input")

// Reservation is Taxi's authoritative claim on a booking for one organization.
//
// It exists to make four things impossible, which is why it is a row and not
// an in-memory lock (Section 8 TAX-013):
//   - the same booking being executed twice,
//   - two providers executing one job,
//   - one provider taking conflicting work,
//   - a repeated Fleet retry creating a second taxi trip.
type Reservation struct {
	ID                    string
	BookingID             string
	SourceRequestID       string
	FleetAssignmentID     string
	ProviderID            string
	OrganizationID        string
	MembershipID          string
	AcceptanceSource      string
	Status                string
	ExpectedSourceVersion int64
	SourceVersion         int64
	Version               int64
	IdempotencyKey        string
	RequestFingerprint    string
	CorrelationID         string
	ExecutionBookingID    *string
	ExpiresAt             *time.Time
	ReleasedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Live reports whether this reservation still holds the booking.
func (r Reservation) Live() bool {
	return r.Status == StatusReserved || r.Status == StatusConfirmed
}

// ReserveInput is Fleet's reservation request, in Taxi's own terms.
type ReserveInput struct {
	SourceRequestID          string
	ExpectedSourceVersion    int64
	OrganizationID           string
	AcceptanceSource         string
	AcceptingProviderID      string
	OrganizationMembershipID string
	IdempotencyKey           string
	CorrelationID            string
}

// Validate applies the contract's own required-field rules.
func (in ReserveInput) Validate() error {
	if strings.TrimSpace(in.SourceRequestID) == "" {
		return errors.New("source_request_id is required")
	}
	if strings.TrimSpace(in.OrganizationID) == "" {
		return errors.New("organization_id is required")
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return errors.New("idempotency_key is required")
	}
	return nil
}

// ReservationOutcome is the authoritative answer Fleet consumes.
type ReservationOutcome struct {
	ReservationID        string `json:"reservation_id"`
	Status               string `json:"status"`
	WinnerOrganizationID string `json:"winner_organization_id"`
	SourceVersion        int64  `json:"source_version"`
}

// ConfirmInput binds a managed provider to a reserved booking and creates the
// canonical taxi execution.
//
// SettlementOrganizationID and ExecutingProviderID are deliberately distinct,
// exactly as in Fleet's contract: the organization settles, the provider
// executes.
type ConfirmInput struct {
	SourceRequestID          string
	ExpectedSourceVersion    int64
	ReservationID            string
	OrganizationID           string
	SettlementOrganizationID string
	ExecutingProviderID      string
	OrganizationMembershipID string
	VehicleID                string
	FleetAssignmentID        string
	FleetAssignmentVersion   int64
	ProviderResponseID       string
	AcceptanceSource         string
	IdempotencyKey           string
	CorrelationID            string
}

// Validate enforces every field TAX-012 requires before Taxi will create an
// organization-authorized execution.
func (in ConfirmInput) Validate() error {
	required := map[string]string{
		"requestId":                in.SourceRequestID,
		"reservationId":            in.ReservationID,
		"organizationId":           in.OrganizationID,
		"executingProviderId":      in.ExecutingProviderID,
		"organizationMembershipId": in.OrganizationMembershipID,
		"fleetAssignmentId":        in.FleetAssignmentID,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return errors.New(name + " is required")
		}
	}
	return nil
}

// ExecutionOutcome is the authoritative execution answer Fleet consumes.
type ExecutionOutcome struct {
	Status         string `json:"status"`
	TripID         string `json:"tripId"`
	RequestID      string `json:"requestId"`
	RequestVersion int64  `json:"requestVersion"`
	RejectionCode  string `json:"rejectionCode"`
}

// EligibilityAnswer is Taxi's own readiness verdict.
//
// Section 8 TAX-004: Fleet eligibility is necessary but NOT sufficient. Taxi's
// readiness rules stay authoritative for taxi execution, and this is where
// they are asserted. Fleet asks; Taxi answers; Taxi never delegates the
// question.
type EligibilityAnswer struct {
	Eligible       bool   `json:"eligible"`
	PlatformStatus string `json:"platform_status"`
	ReasonCode     string `json:"reason_code"`
	VehicleOK      bool   `json:"vehicle_ok"`
	SourceVersion  int64  `json:"source_version"`
}

// RequestView is the authoritative booking state used by Fleet reconciliation.
type RequestView struct {
	RequestID            string `json:"request_id"`
	BookingID            string `json:"booking_id"`
	Status               string `json:"status"`
	SourceVersion        int64  `json:"source_version"`
	WinnerOrganizationID string `json:"winner_organization_id"`
	AcceptedProviderID   string `json:"accepted_provider_id"`
	ExpiresAt            string `json:"expires_at"`
}

// TripView is the authoritative trip state used by Fleet reconciliation.
type TripView struct {
	TripID              string `json:"trip_id"`
	RequestID           string `json:"request_id"`
	BookingID           string `json:"booking_id"`
	Status              string `json:"status"`
	SourceVersion       int64  `json:"source_version"`
	ExecutingProviderID string `json:"executing_provider_id"`
	CompletedAt         string `json:"completed_at"`
	CancelledAt         string `json:"cancelled_at"`
}
