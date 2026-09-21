// Package privateexecution is Taxi's half of Fleet's private corporate
// execution contract (Section 10).
//
// It is the counterpart of driver-hauling-service's package of the same name
// and driver-dispatch-delivery-service's private-execution surface, and it
// speaks the same wire shape on purpose: Fleet drives all three through one
// adapter, and a fourth protocol here would buy nothing and cost a second
// integration to keep in step.
//
// The division of authority is the point. Fleet decides WHO does the work: it
// holds the private order, the assignment and the provider's acceptance. Taxi
// decides WHETHER that is executable and owns everything afterwards -- the
// booking, the lifecycle, the cancellation rules, the settlement mode. Fleet
// never writes a taxi trip; it asks for one and records the reference it gets
// back.
//
// A private execution therefore creates an ORDINARY taxi_bookings row. There
// is no corporate lifecycle, no parallel state machine and no second matcher.
// Corporate identity is metadata on the same row an independent ride uses, so
// every guard that already protects a ride protects this one unchanged.
package privateexecution

import (
	"errors"
	"time"
)

// Errors the handler maps onto status codes. They are sentinels rather than
// strings so a caller's retry decision follows the kind of failure, not a
// substring of its message.
var (
	// ErrNotFound means no such operation, booking or mapping.
	ErrNotFound = errors.New("taxi private execution not found")

	// ErrConflict means an idempotency key was reused for a different request.
	// It is never retryable: the caller changed its mind mid-retry.
	ErrConflict = errors.New("taxi private execution idempotency conflict")

	// ErrIneligible means Taxi refuses the provider or vehicle. This is Taxi's
	// own authority (TAX-004): Fleet membership is necessary but never
	// sufficient, because a driver with perfect corporate standing who is
	// offline, already on a trip, or has no active vehicle cannot drive.
	ErrIneligible = errors.New("taxi provider or vehicle is ineligible")

	// ErrNotCancellable means the trip has moved past the point where a
	// corporate cancellation is meaningful.
	ErrNotCancellable = errors.New("taxi execution is not cancellable")

	// ErrDisabled means this deployment has not enabled private execution.
	ErrDisabled = errors.New("taxi private execution is disabled")

	// ErrUnserviceable means the pickup point falls outside every operating
	// zone this service runs. A booking needs a market and a zone, and
	// inventing one would put a corporate trip in a market nobody operates.
	ErrUnserviceable = errors.New("taxi pickup is outside every operating zone")
)

// RouteSnapshot is the journey Fleet planned.
type RouteSnapshot struct {
	PickupAddress    string  `json:"pickupAddress"`
	PickupLatitude   float64 `json:"pickupLatitude"`
	PickupLongitude  float64 `json:"pickupLongitude"`
	DropoffAddress   string  `json:"dropoffAddress"`
	DropoffLatitude  float64 `json:"dropoffLatitude"`
	DropoffLongitude float64 `json:"dropoffLongitude"`
}

// RideSnapshot is the corporate ride's own shape.
//
// It is small by design. Fleet states what the trip is; it does not state a
// fare, and there is deliberately no price field here: corporate settlement is
// the wallet's and Fleet's business, and a number arriving on this request
// would be a price Taxi neither computed nor can honour.
type RideSnapshot struct {
	PassengerCount int    `json:"passengerCount,omitempty"`
	VehicleClass   string `json:"vehicleClass,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

// CreateRequest is Fleet's command to start a corporate taxi execution.
//
// Every reference is opaque to the other side: Fleet's order, shipment,
// assignment and response ids mean nothing here beyond being recorded, and
// Taxi's provider and vehicle ids mean nothing to Fleet beyond being carried.
type CreateRequest struct {
	OrganizationID           string        `json:"organizationId"`
	SettlementOrganizationID string        `json:"settlementOrganizationId"`
	ExecutingProviderID      string        `json:"executingProviderId"`
	OrganizationMembershipID string        `json:"organizationMembershipId"`
	PrivateOrderID           string        `json:"privateOrderId"`
	ShipmentID               string        `json:"shipmentId"`
	PrivateAssignmentID      string        `json:"privateAssignmentId"`
	ProviderResponseID       string        `json:"providerResponseId"`
	FleetOperationID         string        `json:"fleetOperationId"`
	ServiceCategory          string        `json:"serviceCategory"`
	VehicleID                string        `json:"vehicleId"`
	ContractReference        string        `json:"contractReference,omitempty"`
	QuotationReference       string        `json:"quotationReference,omitempty"`
	Route                    RouteSnapshot `json:"route"`
	Ride                     RideSnapshot  `json:"ride,omitempty"`
	CorrelationID            string        `json:"correlationId,omitempty"`

	// IdempotencyKey travels in the header, never the body, so a body replay
	// cannot quietly change the operation's identity.
	IdempotencyKey string `json:"-"`
}

// View is what Fleet records.
//
// The field names match what Dispatch and Haulage already return, because
// Fleet parses all three with one struct. CanonicalBookingID and
// CanonicalTripID carry the same value: in Taxi the booking IS the trip, and
// reporting them separately would imply a second record that does not exist.
type View struct {
	FleetOperationID   string    `json:"fleetOperationId"`
	Status             string    `json:"status"`
	CanonicalBookingID string    `json:"canonicalBookingId,omitempty"`
	CanonicalTripID    string    `json:"canonicalTripId,omitempty"`
	SourceVersion      int64     `json:"sourceVersion"`
	CancellationStatus string    `json:"cancellationStatus"`
	LastErrorCode      string    `json:"lastErrorCode,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// CancelRequest is Fleet withdrawing a corporate trip.
type CancelRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"-"`
}

// CancelView is the cancellation outcome.
type CancelView struct {
	Status        string `json:"status"`
	SourceVersion int64  `json:"sourceVersion"`
}

// record is the stored mapping plus the identities the service compares
// against Fleet's acceptance record.
type record struct {
	View
	IdempotencyKey, RequestFingerprint              string
	OrganizationID, SettlementOrganizationID        string
	ExecutingProviderID, OrganizationMembershipID   string
	PrivateOrderID, ShipmentID, PrivateAssignmentID string
	ProviderResponseID, VehicleID, CorrelationID    string
	CancellationKey, CancellationFingerprint        string
}

// Statuses of the mapping itself, distinct from the booking's own lifecycle.
const (
	StatusConfirmed = "CONFIRMED"
	StatusCancelled = "CANCELLED"

	CancellationNone      = "NONE"
	CancellationConfirmed = "CONFIRMED"
)

// ServiceCategoryTaxi is the only category this surface accepts. A Dispatch or
// Haulage request arriving here is a misrouted call, and executing it would
// create a taxi trip for work nobody asked Taxi to do.
const ServiceCategoryTaxi = "TAXI"

// SourceContextPrivateExecution marks the booking as Fleet-originated. It is
// an already-accepted value of taxi_bookings.source_context, added by
// migration 012 alongside the rest of the corporate metadata.
const SourceContextPrivateExecution = "FLEET_PRIVATE_EXECUTION"

// SettlementModeOrganizationManaged routes the trip's money away from the
// independent provider payout path. A corporate trip settled as independent
// would pay the driver directly for work their organization is owed for.
const SettlementModeOrganizationManaged = "ORGANIZATION_MANAGED"
