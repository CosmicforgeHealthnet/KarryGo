// Package recovery is Haulage's authority over a job whose assigned provider
// can no longer finish it.
//
// It exists because Section 9 puts post-assignment recovery in Haulage rather
// than in Fleet or Vendor, and the reason is not organisational: the only
// service that can decide whether a REPLACEMENT truck can carry this cargo is
// the one that owns the cargo requirement and the fit engine. A replacement
// chosen anywhere else would either re-derive the fit rules -- and drift from
// them -- or skip them, which is how a breakdown becomes a second breakdown.
//
// Two invariants shape everything here:
//
//   - The booking's identity survives a replacement. The same booking id, the
//     same payment source reference, the same cargo requirement, the same
//     handover binding. A replacement continues the work; it does not restart
//     it, because restarting it would strand the customer's money against a
//     booking nobody is executing.
//
//   - A replacement is chosen by cargofit.EvaluateProvider -- literally the
//     same call the original match used. Recovery has no fit rules of its own,
//     so "the replacement is as eligible as the original" is a property of the
//     code rather than a claim in a comment.
package recovery

import (
	"strings"
	"time"
)

// State is the recovery lifecycle.
//
// The vocabulary is deliberately the one driver-taxi-service already persists
// in taxi_recovery_cases. A second spelling of the same lifecycle would make a
// cross-authority operator console impossible, and the lifecycle is not
// improved by being reinvented.
type State string

const (
	// StateDetected is a reported failure not yet assessed.
	StateDetected State = "DETECTED"
	// StateEvaluating is the assessment of what the failure requires: whether
	// custody has moved, whether the cargo can still be carried at all.
	StateEvaluating State = "EVALUATING"
	// StateReplacementSearch is an active search for a compatible provider.
	StateReplacementSearch State = "REPLACEMENT_SEARCH"
	// StateTransferRequired means a replacement has been chosen and the cargo
	// must physically change hands before it can drive.
	StateTransferRequired State = "TRANSFER_REQUIRED"
	// StateInProgress means the replacement is executing the work.
	StateInProgress State = "IN_PROGRESS"
	// StateResolved is a completed recovery.
	StateResolved State = "RESOLVED"
	// StateOpsRequired means no safe automatic recovery exists and a human
	// must decide. It is NOT a failure: the work is still live and the cargo
	// still somewhere. It is the honest answer when the alternative would be
	// to assign an incompatible truck to keep the mission moving.
	StateOpsRequired State = "OPS_REQUIRED"
	// StateFailed is a recovery abandoned.
	StateFailed State = "FAILED"
)

// Terminal reports whether a state closes the case.
func (s State) Terminal() bool { return s == StateResolved || s == StateFailed }

// Open reports whether the case still occupies its booking's recovery slot.
func (s State) Open() bool { return !s.Terminal() }

// Valid reports whether the state is one of the eight.
func (s State) Valid() bool {
	switch s {
	case StateDetected, StateEvaluating, StateReplacementSearch, StateTransferRequired,
		StateInProgress, StateResolved, StateOpsRequired, StateFailed:
		return true
	default:
		return false
	}
}

// rank orders states so a stale command cannot walk a case backwards.
//
// Terminal states share the highest rank: once a case is closed, nothing
// reopens it. A late "replacement found" arriving after an operator marked the
// case failed must be refused, not applied.
var rank = map[State]int{
	StateDetected: 1, StateEvaluating: 2, StateReplacementSearch: 3,
	StateTransferRequired: 4, StateInProgress: 5,
	StateOpsRequired: 6,
	StateResolved:    7, StateFailed: 7,
}

// CanAdvanceTo reports whether a transition moves the case forward.
//
// OPS_REQUIRED is the one state that may move "backwards", to
// REPLACEMENT_SEARCH: an operator who has fixed whatever blocked automatic
// recovery -- surveyed a truck, restored a certification -- must be able to
// hand the case back to the engine rather than having to resolve it by hand.
func (s State) CanAdvanceTo(next State) bool {
	if !next.Valid() || s.Terminal() {
		return false
	}
	if s == StateOpsRequired && next == StateReplacementSearch {
		return true
	}
	return rank[next] > rank[s]
}

// Trigger codes, using the stable vocabulary custodyops already speaks for
// incidents so an operator sees one set of words across services.
const (
	TriggerProviderBreakdown   = "PROVIDER_BREAKDOWN"
	TriggerProviderIllness     = "PROVIDER_ILLNESS"
	TriggerProviderCancelled   = "PROVIDER_CANCELLED"
	TriggerProviderUnreachable = "PROVIDER_UNREACHABLE"
	TriggerSafetyIncident      = "SAFETY_INCIDENT"
	TriggerVehicleFailure      = "VEHICLE_FAILURE"
	TriggerComplianceRevoked   = "COMPLIANCE_REVOKED"
)

// ValidTrigger reports whether a trigger is one Haulage recognises.
//
// An unrecognised trigger is refused rather than stored: the trigger is what
// an operator filters on when a pattern of failures appears, and a free-text
// field makes that impossible.
func ValidTrigger(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case TriggerProviderBreakdown, TriggerProviderIllness, TriggerProviderCancelled,
		TriggerProviderUnreachable, TriggerSafetyIncident, TriggerVehicleFailure,
		TriggerComplianceRevoked:
		return true
	default:
		return false
	}
}

// Case is one recovery.
type Case struct {
	ID         string
	BookingID  string
	MissionRef string

	TriggerCode string
	State       State

	OriginalProviderID    string
	OriginalTruckID       string
	ReplacementProviderID string
	ReplacementTruckID    string

	CustodyTransferRequired bool
	CustodyTransferRef      string
	EvidenceRef             string

	ReasonCode    string
	OperationRef  string
	CorrelationID string

	DetectedAt time.Time
	StartedAt  *time.Time
	ResolvedAt *time.Time

	Version int64
}

// Public is the operator-facing projection.
//
// It carries provider and truck ids because the reader is an operator of this
// platform, not a customer and not Vendor: recovery evidence is exactly the
// information an operator needs and exactly the information a caller must not
// be able to use to steer an assignment.
type Public struct {
	RecoveryCaseID          string     `json:"recovery_case_id"`
	BookingID               string     `json:"booking_id"`
	MissionRef              string     `json:"mission_ref,omitempty"`
	TriggerCode             string     `json:"trigger_code"`
	State                   string     `json:"state"`
	OriginalProviderID      string     `json:"original_provider_id,omitempty"`
	OriginalTruckID         string     `json:"original_truck_id,omitempty"`
	ReplacementProviderID   string     `json:"replacement_provider_id,omitempty"`
	ReplacementTruckID      string     `json:"replacement_truck_id,omitempty"`
	CustodyTransferRequired bool       `json:"custody_transfer_required"`
	CustodyTransferRef      string     `json:"custody_transfer_ref,omitempty"`
	EvidenceRef             string     `json:"evidence_ref,omitempty"`
	ReasonCode              string     `json:"reason_code,omitempty"`
	DetectedAt              time.Time  `json:"detected_at"`
	StartedAt               *time.Time `json:"started_at,omitempty"`
	ResolvedAt              *time.Time `json:"resolved_at,omitempty"`
	Version                 int64      `json:"version"`
}

// Public projects the case.
func (c Case) Public() Public {
	return Public{
		RecoveryCaseID: c.ID, BookingID: c.BookingID, MissionRef: c.MissionRef,
		TriggerCode: c.TriggerCode, State: string(c.State),
		OriginalProviderID: c.OriginalProviderID, OriginalTruckID: c.OriginalTruckID,
		ReplacementProviderID: c.ReplacementProviderID, ReplacementTruckID: c.ReplacementTruckID,
		CustodyTransferRequired: c.CustodyTransferRequired,
		CustodyTransferRef:      c.CustodyTransferRef, EvidenceRef: c.EvidenceRef,
		ReasonCode: c.ReasonCode, DetectedAt: c.DetectedAt,
		StartedAt: c.StartedAt, ResolvedAt: c.ResolvedAt, Version: c.Version,
	}
}

// StartCommand opens a recovery.
type StartCommand struct {
	BookingID string
	// OperationRef is the caller's idempotency key. A retry with the same ref
	// returns the original case rather than opening a second one for the same
	// failure.
	OperationRef  string
	TriggerCode   string
	CorrelationID string
	Actor         string
}

// TransferCommand completes a controlled custody handover to the replacement.
type TransferCommand struct {
	BookingID    string
	OperationRef string
	TransferRef  string
	EvidenceRef  string
	// OldProviderConfirmed and NewProviderConfirmed are both required for a
	// transfer. A handover one side did not confirm is not a handover, and
	// recording it as one would leave the ledger asserting custody moved when
	// the cargo may still be in the first truck.
	OldProviderConfirmed bool
	NewProviderConfirmed bool
	// ExpectedVersion makes the command conditional on the case the caller
	// read. Zero skips the check, for callers with no version in hand.
	ExpectedVersion int64
	Actor           string
}

// ResolveCommand closes a recovery.
type ResolveCommand struct {
	BookingID       string
	OperationRef    string
	ExpectedVersion int64
	Actor           string
}
