package fulfilmentgroup

// The stored delivery-requirement decision.
//
// This is the fulfilment aggregate's own record of a vehicle decision taken
// elsewhere. The engine that produces it lives in internal/features/fulfilment
// and is not imported here: this package owns the plan, so it owns the shape
// the plan stores, and the composition root maps between the two.
//
// The record exists because a plan version alone cannot answer the question
// people actually ask. A plan says a van was used. It does not say that a
// motorcycle was refused for weight, that a car was the smallest suitable
// class, and whether the gated Taxi Goods authority was available. Those facts
// are the answer, and they have to be
// stored at the moment the decision is taken because the thresholds and the
// integrations they refer to will both have moved by the time anyone asks.
//
// It is APPEND-ONLY and carries no version column. A requirement that changes
// produces a new record against a new plan version; nothing here is updated.

import (
	"encoding/json"
	"strings"
	"time"
)

// RequirementDecision is one stored vehicle decision.
type RequirementDecision struct {
	ID string `json:"requirement_decision_id"`

	GroupID string `json:"fulfilment_group_id"`
	PlanID  string `json:"plan_id"`

	// SlotRef names the plan mission slot the decision was taken for. Empty
	// when the decision covers the group as a whole.
	SlotRef string `json:"slot_ref,omitempty"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	// MinimumPhysicalClass is the smallest class that could physically carry
	// the load. It may be CAR. SelectedVehicleClass may not: the two fields
	// are separate precisely so "we could not run the right vehicle" never
	// looks like "the load needed a bigger one".
	MinimumPhysicalClass string `json:"minimum_physical_class,omitempty"`

	SelectedVehicleClass string `json:"selected_vehicle_class,omitempty"`
	SelectedService      string `json:"selected_service,omitempty"`

	Executable bool `json:"executable"`

	PrimaryReason     string   `json:"primary_reason"`
	SupportingReasons []string `json:"supporting_reasons,omitempty"`

	// Candidates is every class the engine walked, with its own outcome.
	Candidates json.RawMessage `json:"candidates,omitempty"`

	MeasurementState  string `json:"measurement_state"`
	MeasurementSource string `json:"measurement_source,omitempty"`

	TotalWeightGrams   int64 `json:"total_weight_grams"`
	TotalVolumeMl      int64 `json:"total_volume_ml"`
	EffectiveVolumeMl  int64 `json:"effective_volume_ml"`
	LongestDimensionMm int   `json:"longest_dimension_mm"`
	PackageCount       int64 `json:"package_count"`

	RouteDistanceMeters int64 `json:"route_distance_meters"`
	PickupCount         int   `json:"pickup_count"`

	RiskClass string `json:"risk_class,omitempty"`

	// DeclaredValueMinor is risk metadata. It never becomes a fee, a ledger
	// entry or an insurance charge: payment-wallet-service remains the only
	// authority for money.
	DeclaredValueMinor    int64  `json:"declared_value_minor,omitempty"`
	DeclaredValueCurrency string `json:"declared_value_currency,omitempty"`

	PolicyVersion string `json:"policy_version"`

	// Evidence is the complete replay document: the requirement, the policy as
	// it was, the serviceability as it was, and the decision. A stored
	// decision must be reproducible with no live service reachable.
	Evidence json.RawMessage `json:"evidence,omitempty"`

	Fingerprint string `json:"fingerprint"`

	CreatedAt time.Time `json:"created_at"`
}

// Refused reports whether the decision found no executable service.
func (d RequirementDecision) Refused() bool { return !d.Executable }

// Escalated reports whether the executed class is larger than the minimum
// physical requirement.
//
// It is the signal an operator looks for when asking whether the platform is
// systematically running bigger vehicles than its loads need, which on this
// platform today is usually the disabled taxi integration rather than heavy
// orders.
func (d RequirementDecision) Escalated() bool {
	return d.Executable &&
		d.MinimumPhysicalClass != "" &&
		d.MinimumPhysicalClass != d.SelectedVehicleClass
}

// Validate reports the first reason a decision must not be persisted.
//
// The rules mirror the database constraints rather than replacing them. The
// database is the enforcement; this is the earlier, clearer error.
func (d RequirementDecision) Validate() error {
	if strings.TrimSpace(d.PrimaryReason) == "" {
		return ErrInvalid
	}

	if strings.TrimSpace(d.Fingerprint) == "" {
		return ErrInvalid
	}

	if strings.TrimSpace(d.PolicyVersion) == "" {
		return ErrInvalid
	}

	// An executable decision names both halves of the answer; a refused one
	// names neither. A stored class with no service is a mission nobody owns.
	switch {
	case d.Executable &&
		(d.SelectedVehicleClass == "" || d.SelectedService == ""):
		return ErrInvalid

	case !d.Executable &&
		(d.SelectedVehicleClass != "" || d.SelectedService != ""):
		return ErrInvalid
	}

	// The structural taxi gate, restated where a plan is written. The database
	// refuses it too; refusing here as well means a bug is caught by a clear
	// error rather than a constraint violation.
	if d.SelectedVehicleClass == "CAR" {
		return ErrConflict
	}

	switch {
	case d.TotalWeightGrams < 0 || d.TotalVolumeMl < 0:
		return ErrInvalid
	case d.EffectiveVolumeMl < 0 || d.LongestDimensionMm < 0:
		return ErrInvalid
	case d.PackageCount < 0 || d.PickupCount < 0:
		return ErrInvalid
	case d.RouteDistanceMeters < 0 || d.DeclaredValueMinor < 0:
		return ErrInvalid
	case d.DeclaredValueMinor > 0 && d.DeclaredValueCurrency == "":
		return ErrInvalid
	case d.DeclaredValueCurrency != "" && len(d.DeclaredValueCurrency) != 3:
		return ErrInvalid
	}

	return nil
}
