package models

import (
	"encoding/json"
	"time"
)

// Rescue case statuses (spec 16.14, 45.2). A replacement provider is offered
// through an ordinary Section 15 private assignment: REPLACEMENT_PENDING
// means the provider has not yet accepted, and nothing is forced on them.
const (
	RescueOpen                = "OPEN"
	RescueReplacementPending  = "REPLACEMENT_PENDING"
	RescueReplacementAccepted = "REPLACEMENT_ACCEPTED"
	RescueResolved            = "RESOLVED"
	RescueCancelled           = "CANCELLED"
)

var rescueTriggers = map[string]bool{"BREAKDOWN": true, "PROVIDER_UNAVAILABLE": true, "DELAY": true, "FAILED_DELIVERY": true, "UNSAFE_VEHICLE": true, "ACCIDENT": true, "CARGO_TRANSFER": true, "OTHER": true}
var rescueSeverities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}
var rescueResolutionCodes = map[string]bool{"REPLACEMENT_COMPLETED": true, "RECOVERED_ON_SITE": true, "CARGO_TRANSFERRED": true, "TRIP_CANCELLED_AT_SOURCE": true, "NO_ACTION_REQUIRED": true}

// ValidRescueTrigger reports whether trigger is a supported rescue trigger.
func ValidRescueTrigger(trigger string) bool { return rescueTriggers[trigger] }

// ValidRescueSeverity reports whether severity is a supported severity.
func ValidRescueSeverity(severity string) bool { return rescueSeverities[severity] }

// ValidRescueResolution reports whether code is a supported resolution code.
func ValidRescueResolution(code string) bool { return rescueResolutionCodes[code] }

type RescueCase struct {
	ID                          string          `json:"id"`
	OrganizationID              string          `json:"organizationId"`
	BranchID                    string          `json:"branchId"`
	TriggerType                 string          `json:"triggerType"`
	Severity                    string          `json:"severity"`
	Status                      string          `json:"status"`
	PrivateAssignmentID         string          `json:"privateAssignmentId,omitempty"`
	CanonicalTripReference      string          `json:"canonicalTripReference,omitempty"`
	AssetID                     string          `json:"assetId,omitempty"`
	BreakdownID                 string          `json:"breakdownId,omitempty"`
	Reason                      string          `json:"reason"`
	AssignedToUserID            string          `json:"assignedToUserId,omitempty"`
	CurrentAssignmentID         string          `json:"currentAssignmentId,omitempty"`
	ReplacementAssignmentID     string          `json:"replacementAssignmentId,omitempty"`
	ReplacementAssignmentStatus string          `json:"replacementAssignmentStatus,omitempty"`
	ReplacementMembershipID     string          `json:"replacementMembershipId,omitempty"`
	ReplacementAssetReference   string          `json:"replacementAssetReference,omitempty"`
	ReplacementAttempts         int             `json:"replacementAttempts"`
	AlertID                     string          `json:"alertId,omitempty"`
	ResolutionCode              string          `json:"resolutionCode,omitempty"`
	Resolution                  json.RawMessage `json:"resolution"`
	ResolvedBy                  string          `json:"resolvedBy,omitempty"`
	ResolvedAt                  *time.Time      `json:"resolvedAt,omitempty"`
	CancelledAt                 *time.Time      `json:"cancelledAt,omitempty"`
	CreatedBy                   string          `json:"createdBy"`
	Version                     int64           `json:"version"`
	CreatedAt                   time.Time       `json:"createdAt"`
	UpdatedAt                   time.Time       `json:"updatedAt"`
}

type OpenRescueCaseInput struct {
	BranchID               string `json:"branchId"`
	TriggerType            string `json:"triggerType"`
	Severity               string `json:"severity"`
	PrivateAssignmentID    string `json:"privateAssignmentId"`
	CanonicalTripReference string `json:"canonicalTripReference"`
	AssetID                string `json:"assetId"`
	BreakdownID            string `json:"breakdownId"`
	Reason                 string `json:"reason"`
	AssignedToUserID       string `json:"assignedToUserId"`
	IdempotencyKey         string `json:"-"`
}

type RescueReplacementInput struct {
	ProviderMembershipID    string     `json:"providerMembershipId"`
	ProviderID              string     `json:"providerId"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference"`
	ExecutionService        string     `json:"executionService"`
	ResponseDeadline        *time.Time `json:"responseDeadline"`
	ExpectedVersion         int64      `json:"expectedVersion"`
}

type ResolveRescueCaseInput struct {
	ResolutionCode  string `json:"resolutionCode"`
	Notes           string `json:"notes"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

type CancelRescueCaseInput struct {
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

// RescueFilter selects rescue cases. It pages by Offset or by the keyset
// position After/AfterID, never both.
type RescueFilter struct {
	BranchID, Status, TriggerType string
	Limit, Offset                 int
	After                         *time.Time
	AfterID                       string
}

// RescueOrganizationRef names one rescue case due for replacement sync.
type RescueOrganizationRef struct{ OrganizationID, ID string }

// RescueAssignment is the current private assignment of a rescue case, as
// locked for a replacement decision.
type RescueAssignment struct {
	Status, WorkOrigin, WorkID, ProviderMembershipID, ExecutionService, BranchID string
	Version                                                                      int64
}

// RescueSubjectKey identifies the work a rescue case is about, so one open
// case exists per subject and trigger.
func RescueSubjectKey(in OpenRescueCaseInput) string {
	switch {
	case in.CanonicalTripReference != "":
		return "trip:" + in.CanonicalTripReference
	case in.PrivateAssignmentID != "":
		return "assignment:" + in.PrivateAssignmentID
	case in.BreakdownID != "":
		return "breakdown:" + in.BreakdownID
	default:
		return "asset:" + in.AssetID
	}
}
