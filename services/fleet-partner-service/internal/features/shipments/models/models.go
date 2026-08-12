// Package models holds the Section 15 shipment and cargo item domain.
//
// A shipment belongs to exactly one private order and is never a canonical
// trip: nothing here reserves capacity, moves money or starts execution.
// Nothing in this package imports gin, pgx or an HTTP client, and the status
// machines are the point of the package — a shipment or cargo item can only
// ever reach a status this file admits a transition to.
package models

import (
	"errors"
	"regexp"
	"time"
)

// JSONMap is an untyped jsonb column.
type JSONMap map[string]any

// Clone returns a defensive copy so a stored value cannot be edited through
// the map that produced it.
func (m JSONMap) Clone() JSONMap {
	if m == nil {
		return nil
	}
	out := make(JSONMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Shipment
// ---------------------------------------------------------------------------

// Shipment statuses.
const (
	ShipmentDraft              = "DRAFT"
	ShipmentReady              = "READY"
	ShipmentPlanned            = "PLANNED"
	ShipmentAssigned           = "ASSIGNED"
	ShipmentInTransit          = "IN_TRANSIT"
	ShipmentCompleted          = "COMPLETED"
	ShipmentFailed             = "FAILED"
	ShipmentPartiallyCompleted = "PARTIALLY_COMPLETED"
	ShipmentCancelled          = "CANCELLED"
)

// shipmentTransitions is the whole shipment status matrix.
//
// Once a shipment reaches ASSIGNED it has no direct edge to CANCELLED: a
// private assignment or canonical execution may already exist by then, and
// the owning execution service — not a local status write — is authoritative
// for stopping it. That path belongs to the private-assignment/execution
// orchestration slice, not this one.
var shipmentTransitions = map[string]map[string]bool{
	ShipmentDraft:              {ShipmentReady: true, ShipmentCancelled: true},
	ShipmentReady:              {ShipmentPlanned: true, ShipmentCancelled: true},
	ShipmentPlanned:            {ShipmentAssigned: true, ShipmentCancelled: true},
	ShipmentAssigned:           {ShipmentInTransit: true, ShipmentFailed: true, ShipmentPartiallyCompleted: true},
	ShipmentInTransit:          {ShipmentCompleted: true, ShipmentFailed: true, ShipmentPartiallyCompleted: true},
	ShipmentCompleted:          {},
	ShipmentFailed:             {},
	ShipmentPartiallyCompleted: {},
	ShipmentCancelled:          {},
}

// CanTransitionShipment reports whether a shipment status change is permitted.
func CanTransitionShipment(from, to string) bool { return shipmentTransitions[from][to] }

// TerminalShipment reports whether the status admits no further movement.
func TerminalShipment(status string) bool {
	switch status {
	case ShipmentCompleted, ShipmentFailed, ShipmentPartiallyCompleted, ShipmentCancelled:
		return true
	}
	return false
}

// ShipmentLocked reports whether a shipment's core fields (route, cargo,
// declared value) may no longer be casually edited. DRAFT is the only
// freely-editable state — everything past it is either mid-execution or
// terminal.
func ShipmentLocked(status string) bool { return status != ShipmentDraft }

// ShipmentEditable reports whether the shipment may still gain or change
// cargo items.
func ShipmentEditable(status string) bool { return status == ShipmentDraft || status == ShipmentReady }

// Shipment is an organization-owned unit of work inside a private order.
type Shipment struct {
	ID                        string
	OrganizationID            string
	BranchID                  string
	OrderID                   string
	OriginStopID              string
	DestinationStopID         string
	CargoProfile              JSONMap
	PassengerProfile          JSONMap
	RequiredVehicleClass      string
	RequiredServiceCapability string
	TemperatureRequirements   JSONMap
	DeclaredValueAmountMinor  int64
	DeclaredValueCurrency     string
	PackageCount              int
	Status                    string
	Version                   int64
	CreatedBy                 string
	UpdatedBy                 string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	CompletedAt               *time.Time
	CancelledAt               *time.Time
}

// ShipmentHistory is one immutable shipment status change.
type ShipmentHistory struct {
	ID             string
	OrganizationID string
	ShipmentID     string
	PreviousStatus string
	NewStatus      string
	ReasonCode     string
	ActorType      string
	ActorID        string
	CorrelationID  string
	OccurredAt     time.Time
}

// ---------------------------------------------------------------------------
// Cargo items
// ---------------------------------------------------------------------------

// Handling categories.
const (
	HandlingGeneral         = "GENERAL"
	HandlingDangerousGoods  = "DANGEROUS_GOODS"
	HandlingMedical         = "MEDICAL"
	HandlingColdChain       = "COLD_CHAIN"
	HandlingFragile         = "FRAGILE"
	HandlingHighValue       = "HIGH_VALUE"
	HandlingFoodAgriculture = "FOOD_AGRICULTURE"
	HandlingRestricted      = "RESTRICTED"
)

var validHandlingCategories = map[string]bool{
	HandlingGeneral: true, HandlingDangerousGoods: true, HandlingMedical: true,
	HandlingColdChain: true, HandlingFragile: true, HandlingHighValue: true,
	HandlingFoodAgriculture: true, HandlingRestricted: true,
}

// ValidHandlingCategory reports whether the category is recognized.
func ValidHandlingCategory(value string) bool { return validHandlingCategories[value] }

// RequiresComplianceReview reports whether a handling category can never be
// waved through on cargo description text alone.
func RequiresComplianceReview(category string) bool {
	switch category {
	case HandlingDangerousGoods, HandlingMedical, HandlingRestricted:
		return true
	}
	return false
}

// Restricted-item statuses. UNKNOWN is the default and the fail-closed
// state: a cargo item nobody has decided about is treated exactly like one
// still pending review, never like one cleared.
const (
	RestrictionNotRestricted  = "NOT_RESTRICTED"
	RestrictionRequiresReview = "REQUIRES_COMPLIANCE_REVIEW"
	RestrictionApproved       = "COMPLIANCE_APPROVED"
	RestrictionRejected       = "COMPLIANCE_REJECTED"
	RestrictionUnknown        = "UNKNOWN"
)

// RestrictionBlocksShipment reports whether a cargo item's restriction state
// blocks the shipment from moving forward. Only an explicit approval or an
// explicit not-restricted determination clears it; everything else —
// including silence — fails closed.
func RestrictionBlocksShipment(status string) bool {
	return status != RestrictionNotRestricted && status != RestrictionApproved
}

// CargoItem is an organization-owned piece of cargo inside a shipment.
type CargoItem struct {
	ID                        string
	OrganizationID            string
	ShipmentID                string
	Description               string
	Quantity                  string // decimal string, never a float
	Unit                      string
	HandlingCategory          string
	HazardousClassification   string
	TemperatureMin            *string
	TemperatureMax            *string
	TemperatureUnit           string
	DeclaredValueAmountMinor  int64
	DeclaredValueCurrency     string
	RestrictedItemStatus      string
	ComplianceReference       string
	ComplianceDecisionVersion int64
	Version                   int64
	CreatedBy                 string
	UpdatedBy                 string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// ErrInvalidQuantity is returned when a cargo quantity is not a positive
// decimal.
var ErrInvalidQuantity = errors.New("shipments: quantity must be a positive decimal")

var decimalPattern = regexp.MustCompile(`^\d{1,12}(\.\d{1,6})?$`)

// ValidateQuantity checks that a caller-supplied quantity is a positive
// decimal with bounded precision, without ever parsing it into a float. A
// malformed value is refused here, cleanly, rather than surfacing as a raw
// database numeric-cast error.
func ValidateQuantity(raw string) error {
	if !decimalPattern.MatchString(raw) || raw == "0" {
		return ErrInvalidQuantity
	}
	return nil
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidateCurrency enforces an ISO 4217 alphabetic code.
func ValidateCurrency(code string) bool { return currencyPattern.MatchString(code) }
