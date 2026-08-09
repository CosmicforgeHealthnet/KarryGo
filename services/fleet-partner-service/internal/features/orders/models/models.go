// Package models holds the Section 15 private order and order stop domain.
//
// A private order is company-owned work — manual, converted from an accepted
// Section 14 quotation, generated from a contract or a recurring template, or
// imported in bulk — and is never a marketplace request. Nothing here imports
// gin, pgx or an HTTP client. The status machine is the point of the package:
// an order can only ever reach a status this file admits a transition to, and
// no handler, worker or repair script can bypass it.
package models

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidTransition is returned when a status change is not in the matrix.
var ErrInvalidTransition = errors.New("orders: transition is not permitted")

// JSONMap is an untyped jsonb column.
type JSONMap map[string]any

// Clone returns a defensive copy so a stored snapshot cannot be edited through
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

// Order sources.
//
// PARTNER_INTEGRATION is a source label and a secured integration entry
// point only; it does not reintroduce a separate partner order API.
const (
	SourceManual             = "MANUAL"
	SourceQuotation          = "QUOTATION"
	SourceContract           = "CONTRACT"
	SourceRecurringTemplate  = "RECURRING_TEMPLATE"
	SourceBulkImport         = "BULK_IMPORT"
	SourcePartnerIntegration = "PARTNER_INTEGRATION"
)

var validSources = map[string]bool{
	SourceManual: true, SourceQuotation: true, SourceContract: true,
	SourceRecurringTemplate: true, SourceBulkImport: true, SourcePartnerIntegration: true,
}

// ValidSource reports whether the order source is recognized.
func ValidSource(value string) bool { return validSources[value] }

// Priorities.
const (
	PriorityLow      = "LOW"
	PriorityStandard = "STANDARD"
	PriorityHigh     = "HIGH"
	PriorityUrgent   = "URGENT"
)

var validPriorities = map[string]bool{
	PriorityLow: true, PriorityStandard: true, PriorityHigh: true, PriorityUrgent: true,
}

// ValidPriority reports whether the priority is recognized.
func ValidPriority(value string) bool { return validPriorities[value] }

// Order statuses.
const (
	StatusDraft              = "DRAFT"
	StatusAwaitingApproval   = "AWAITING_APPROVAL"
	StatusConfirmed          = "CONFIRMED"
	StatusPlanned            = "PLANNED"
	StatusAssigned           = "ASSIGNED"
	StatusInProgress         = "IN_PROGRESS"
	StatusCompleted          = "COMPLETED"
	StatusCancelled          = "CANCELLED"
	StatusFailed             = "FAILED"
	StatusPartiallyCompleted = "PARTIALLY_COMPLETED"
)

// orderTransitions is the whole order status matrix.
//
// Once an order reaches ASSIGNED or IN_PROGRESS it has no direct edge to
// CANCELLED: an execution request or canonical trip may already exist by
// then, and the owning execution service — not a local order-status write —
// is authoritative for stopping it. That path is orchestration's
// responsibility (a later Section 15 slice), not this one's.
var orderTransitions = map[string]map[string]bool{
	StatusDraft:              {StatusAwaitingApproval: true, StatusConfirmed: true, StatusCancelled: true},
	StatusAwaitingApproval:   {StatusConfirmed: true, StatusCancelled: true},
	StatusConfirmed:          {StatusPlanned: true, StatusCancelled: true},
	StatusPlanned:            {StatusAssigned: true, StatusCancelled: true},
	StatusAssigned:           {StatusInProgress: true, StatusFailed: true, StatusPartiallyCompleted: true},
	StatusInProgress:         {StatusCompleted: true, StatusFailed: true, StatusPartiallyCompleted: true},
	StatusCompleted:          {},
	StatusCancelled:          {},
	StatusFailed:             {},
	StatusPartiallyCompleted: {},
}

// CanTransition reports whether an order status change is permitted.
func CanTransition(from, to string) bool { return orderTransitions[from][to] }

// Terminal reports whether the status admits no further movement.
func Terminal(status string) bool {
	switch status {
	case StatusCompleted, StatusCancelled, StatusFailed, StatusPartiallyCompleted:
		return true
	}
	return false
}

// Cancellable reports whether an order in this status can still be cancelled
// through the direct order-level cancel path.
func Cancellable(status string) bool {
	switch status {
	case StatusDraft, StatusAwaitingApproval, StatusConfirmed, StatusPlanned:
		return true
	}
	return false
}

// Order is an organization-owned private order.
type Order struct {
	ID                     string
	OrganizationID         string
	BranchID               string
	OrderNumber            string
	CustomerID             string
	CustomerReference      string
	Source                 string
	SourceReference        string
	ServiceCatalogItemID   string
	ServiceCatalogSnapshot JSONMap
	ServiceFields          JSONMap
	ContractID             string
	ContractVersionID      string
	QuotationID            string
	QuotationVersionID     string
	ConversionIntentID     string
	RequestedScheduleStart *time.Time
	RequestedScheduleEnd   *time.Time
	Timezone               string
	Priority               string
	SpecialInstructions    string
	Status                 string
	ConfirmationSnapshot   JSONMap
	ApprovalRequestID      string
	CancelReasonCode       string
	Version                int64
	CreatedBy              string
	UpdatedBy              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	ConfirmedAt            *time.Time
	CancelledAt            *time.Time
	CompletedAt            *time.Time
}

// History change types.
const (
	ChangeCreated   = "CREATED"
	ChangeConfirmed = "CONFIRMED"
	ChangeCancelled = "CANCELLED"
)

// OrderHistory is one immutable order status change.
type OrderHistory struct {
	ID             string
	OrganizationID string
	OrderID        string
	PreviousStatus string
	NewStatus      string
	ReasonCode     string
	ActorType      string
	ActorID        string
	CorrelationID  string
	OccurredAt     time.Time
}

// ---------------------------------------------------------------------------
// Order stops
// ---------------------------------------------------------------------------

// Stop types.
const (
	StopTypePickup     = "PICKUP"
	StopTypeDelivery   = "DELIVERY"
	StopTypeWaypoint   = "WAYPOINT"
	StopTypeDepot      = "DEPOT"
	StopTypeWarehouse  = "WAREHOUSE"
	StopTypeCustoms    = "CUSTOMS"
	StopTypeInspection = "INSPECTION"
	StopTypeService    = "SERVICE"
	StopTypeReturn     = "RETURN"
)

var validStopTypes = map[string]bool{
	StopTypePickup: true, StopTypeDelivery: true, StopTypeWaypoint: true, StopTypeDepot: true,
	StopTypeWarehouse: true, StopTypeCustoms: true, StopTypeInspection: true, StopTypeService: true,
	StopTypeReturn: true,
}

// ValidStopType reports whether the stop type is recognized.
func ValidStopType(value string) bool { return validStopTypes[value] }

// Stop statuses.
const (
	StopStatusPending   = "PENDING"
	StopStatusConfirmed = "CONFIRMED"
	StopStatusCompleted = "COMPLETED"
	StopStatusSkipped   = "SKIPPED"
	StopStatusFailed    = "FAILED"
)

// OrderStop is one stop on a private order's route.
type OrderStop struct {
	ID                 string
	OrganizationID     string
	OrderID            string
	Sequence           int
	StopType           string
	CustomerLocationID string
	AddressSnapshot    JSONMap
	Latitude           *float64
	Longitude          *float64
	TimeWindowStart    *time.Time
	TimeWindowEnd      *time.Time
	Timezone           string
	ContactSnapshot    JSONMap
	Instructions       string
	Status             string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ValidateServiceFields checks that every field the catalogue item's
// required_fields_schema marks required is present with a non-nil value.
//
// This is a presence check only: the schema's field-shape rules (type, enum,
// maxLength) are enforced once, on the schema itself, by the service
// catalogue domain when the schema was authored. An order is not re-run
// through that shape validator here, it is only checked against what that
// schema actually requires to be present.
func ValidateServiceFields(schema JSONMap, fields JSONMap) error {
	for name, raw := range schema {
		descriptor, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		required, _ := descriptor["required"].(bool)
		if !required {
			continue
		}
		value, present := fields[name]
		if !present || value == nil || value == "" {
			return fmt.Errorf("orders: required service field %q is missing", name)
		}
	}
	return nil
}

// ValidateCoordinates enforces the coordinate pair invariant: a coordinate is
// a pair or it is nothing, because half a coordinate routes a vehicle to the
// equator.
func ValidateCoordinates(lat, lng *float64) error {
	if (lat == nil) != (lng == nil) {
		return errors.New("orders: latitude and longitude must be provided together")
	}
	if lat == nil {
		return nil
	}
	if *lat < -90 || *lat > 90 {
		return errors.New("orders: latitude must be between -90 and 90")
	}
	if *lng < -180 || *lng > 180 {
		return errors.New("orders: longitude must be between -180 and 180")
	}
	return nil
}
