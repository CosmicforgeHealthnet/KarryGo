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
	ID, OrganizationID, BranchID, WorkOrigin, PrivateOrderID, ShipmentID, RouteStopID, ProviderMembershipID, ProviderID, VehicleOrTruckReference, ExecutionService, Status string
	ResponseDeadline                                                                                                                                                       time.Time
	AcceptedAt, DeclinedAt, ExpiredAt, CancelledAt                                                                                                                         *time.Time
	DeclineReason, DeclineNote, ReplacementAssignmentID                                                                                                                    string
	Version                                                                                                                                                                int64
	CreatedBy                                                                                                                                                              string
	CreatedAt, UpdatedAt                                                                                                                                                   time.Time
}
type Decision struct {
	ID, OrganizationID, AssignmentID, ProviderID, Decision, ReasonCode, SafeNote string
	SafetyProtected                                                              bool
	IdempotencyKey, RequestFingerprint                                           string
	DecidedAt                                                                    time.Time
}
type ExecutionOperation struct {
	ID                        string
	OrganizationID            string
	SettlementOrganizationID  string
	ExecutingProviderID       string
	OrganizationMembershipID  string
	PrivateOrderID            string
	ShipmentID                string
	AssignmentID              string
	ProviderResponseID        string
	VehicleOrTruckReference   string
	ContractReference         string
	QuotationReference        string
	ExecutionService          string
	Status                    string
	IdempotencyKey            string
	RequestFingerprint        string
	CanonicalRequestReference string
	CanonicalTripReference    string
	CorrelationID             string
	CausationID               string
	Payload                   ExecutionPayload
	SourceVersion             int64
	Attempts                  int
	LastErrorCode             string
	NextAttemptAt             time.Time
	LeaseHeartbeatAt          *time.Time
	CompletedAt               *time.Time
	DeadLetteredAt            *time.Time
	Version                   int64
	CreatedAt, UpdatedAt      time.Time
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
