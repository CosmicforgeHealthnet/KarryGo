// Package privateexecution adapts Fleet private work to Dispatch's existing
// canonical request, reservation and trip lifecycle.
package privateexecution

import "time"

const (
	StatusPreparing = "PREPARING"
	StatusReady     = "READY"
	StatusConfirmed = "CONFIRMED"
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
	StatusCancelled = "CANCELLED"
	StatusUnknown   = "OUTCOME_UNKNOWN"
)

type RouteSnapshot struct {
	PickupAddress    string  `json:"pickupAddress"`
	PickupLatitude   float64 `json:"pickupLatitude"`
	PickupLongitude  float64 `json:"pickupLongitude"`
	DropoffAddress   string  `json:"dropoffAddress"`
	DropoffLatitude  float64 `json:"dropoffLatitude"`
	DropoffLongitude float64 `json:"dropoffLongitude"`
}

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
	VehicleID                string        `json:"vehicleId,omitempty"`
	ContractReference        string        `json:"contractReference,omitempty"`
	QuotationReference       string        `json:"quotationReference,omitempty"`
	Route                    RouteSnapshot `json:"route"`
	CorrelationID            string        `json:"correlationId,omitempty"`
	IdempotencyKey           string        `json:"-"`
}

type View struct {
	FleetOperationID       string    `json:"fleetOperationId"`
	Status                 string    `json:"status"`
	CanonicalRequestID     string    `json:"canonicalRequestId,omitempty"`
	CanonicalReservationID string    `json:"canonicalReservationId,omitempty"`
	CanonicalTripID        string    `json:"canonicalTripId,omitempty"`
	SourceVersion          int64     `json:"sourceVersion"`
	CancellationStatus     string    `json:"cancellationStatus"`
	LastErrorCode          string    `json:"lastErrorCode,omitempty"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

type record struct {
	View
	ID, IdempotencyKey, RequestFingerprint               string
	OrganizationID, SettlementOrganizationID             string
	ExecutingProviderID, OrganizationMembershipID        string
	PrivateOrderID, ShipmentID, PrivateAssignmentID      string
	ProviderResponseID, VehicleID, ServiceCategory       string
	ContractReference, QuotationReference, CorrelationID string
	CancellationKey, CancellationFingerprint             string
}

type CancelRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"-"`
}

type CancelView struct {
	Status        string `json:"status"`
	SourceVersion int64  `json:"sourceVersion"`
}
