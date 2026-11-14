// Package models defines Section 15 private assignments and execution operations.
package models

import (
	marketmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"time"
)

const (
	StatusPending                = "PENDING_PROVIDER_RESPONSE"
	StatusAccepted               = "ACCEPTED"
	StatusReady                  = "READY_FOR_EXECUTION"
	StatusInProgress             = "IN_PROGRESS"
	StatusCompleted              = "COMPLETED"
	StatusDeclined               = "DECLINED"
	StatusExpired                = "EXPIRED"
	StatusCancelled              = "CANCELLED"
	StatusReassignment           = "REASSIGNMENT_REQUIRED"
	ExecutionPending             = "PENDING"
	ExecutionSubmitting          = "SUBMITTING"
	ExecutionUnknown             = "OUTCOME_UNKNOWN"
	ExecutionConfirmed           = "CONFIRMED"
	ExecutionFailed              = "FAILED"
	ExecutionCancellationPending = "CANCELLATION_PENDING"
	ExecutionCancellationUnknown = "CANCELLATION_UNKNOWN"
	ExecutionCancelled           = "CANCELLED"
	ExecutionReconciliation      = "RECONCILIATION_REQUIRED"
	ExecutionDeadLetter          = "DEAD_LETTER"
)

func ValidOrigin(v string) bool {
	return v == "PRIVATE_ORDER" || v == "PRIVATE_SHIPMENT" || v == "ROUTE_STOP"
}
func ValidExecutionService(v string) bool { return v == "DISPATCH" || v == "TAXI" || v == "HAULING" }
func ValidDecline(v string) bool          { return marketmodels.ValidDeclineReason(v) }
func SafetyProtected(v string) bool       { return marketmodels.SafetyProtected(v) }
func RequiresNote(v string) bool          { return marketmodels.RequiresNote(v) }

type Assignment struct {
	ID                      string     `json:"id"`
	OrganizationID          string     `json:"organizationId"`
	BranchID                string     `json:"branchId"`
	WorkOrigin              string     `json:"workOrigin"`
	PrivateOrderID          string     `json:"privateOrderId"`
	ShipmentID              string     `json:"shipmentId"`
	RouteStopID             string     `json:"routeStopId"`
	ProviderMembershipID    string     `json:"providerMembershipId"`
	ProviderID              string     `json:"providerId"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference"`
	ExecutionService        string     `json:"executionService"`
	Status                  string     `json:"status"`
	ResponseDeadline        time.Time  `json:"responseDeadline"`
	AcceptedAt              *time.Time `json:"acceptedAt"`
	DeclinedAt              *time.Time `json:"declinedAt"`
	ExpiredAt               *time.Time `json:"expiredAt"`
	CancelledAt             *time.Time `json:"cancelledAt"`
	DeclineReason           string     `json:"declineReason"`
	DeclineNote             string     `json:"declineNote"`
	ReplacementAssignmentID string     `json:"replacementAssignmentId"`
	Version                 int64      `json:"version"`
	CreatedBy               string     `json:"createdBy"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}
type Decision struct {
	ID, OrganizationID, AssignmentID, ProviderID, Decision, ReasonCode, SafeNote string
	SafetyProtected                                                              bool
	IdempotencyKey, RequestFingerprint                                           string
	DecidedAt                                                                    time.Time
}
type ExecutionOperation struct {
	ID                        string           `json:"id"`
	OrganizationID            string           `json:"organizationId"`
	SettlementOrganizationID  string           `json:"settlementOrganizationId"`
	ExecutingProviderID       string           `json:"executingProviderId"`
	OrganizationMembershipID  string           `json:"organizationMembershipId"`
	PrivateOrderID            string           `json:"privateOrderId"`
	ShipmentID                string           `json:"shipmentId"`
	AssignmentID              string           `json:"assignmentId"`
	ProviderResponseID        string           `json:"providerResponseId"`
	VehicleOrTruckReference   string           `json:"vehicleOrTruckReference"`
	ContractReference         string           `json:"contractReference"`
	QuotationReference        string           `json:"quotationReference"`
	ExecutionService          string           `json:"executionService"`
	Status                    string           `json:"status"`
	IdempotencyKey            string           `json:"idempotencyKey"`
	RequestFingerprint        string           `json:"requestFingerprint"`
	CanonicalRequestReference string           `json:"canonicalRequestReference"`
	CanonicalTripReference    string           `json:"canonicalTripReference"`
	CorrelationID             string           `json:"correlationId"`
	CausationID               string           `json:"causationId"`
	Payload                   ExecutionPayload `json:"payload"`
	SourceVersion             int64            `json:"sourceVersion"`
	Attempts                  int              `json:"attempts"`
	LastErrorCode             string           `json:"lastErrorCode"`
	NextAttemptAt             time.Time        `json:"nextAttemptAt"`
	LeaseHeartbeatAt          *time.Time       `json:"leaseHeartbeatAt"`
	CompletedAt               *time.Time       `json:"completedAt"`
	DeadLetteredAt            *time.Time       `json:"deadLetteredAt"`
	Version                   int64            `json:"version"`
	CreatedAt                 time.Time        `json:"createdAt"`
	UpdatedAt                 time.Time        `json:"updatedAt"`
}

type ExecutionPayload struct {
	PickupAddress    string  `json:"pickupAddress"`
	PickupLatitude   float64 `json:"pickupLatitude"`
	PickupLongitude  float64 `json:"pickupLongitude"`
	DropoffAddress   string  `json:"dropoffAddress"`
	DropoffLatitude  float64 `json:"dropoffLatitude"`
	DropoffLongitude float64 `json:"dropoffLongitude"`
	CargoDescription string  `json:"cargoDescription,omitempty"`
	CargoWeightKG    int     `json:"cargoWeightKg,omitempty"`
	CargoType        string  `json:"cargoType,omitempty"`
}

type ReconciliationCase struct {
	ID, OrganizationID, ExecutionOperationID, ProjectionID, ReasonCode, Status string
	Attempts                                                                   int
	CorrelationID                                                              string
}
