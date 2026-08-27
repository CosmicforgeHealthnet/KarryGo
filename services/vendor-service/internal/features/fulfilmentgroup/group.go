package fulfilmentgroup

// The fulfilment group and the units inside it.
//
// Types and pure rules only. Persistence lives in store.go and the workflow in
// service.go, so what a group IS can be reasoned about without either.

import (
	"errors"
	"strings"
	"time"
)

// Errors. These are the vocabulary the HTTP layer maps to status codes, so a
// caller can tell a stale write apart from a forbidden one.
var (
	ErrNotFound = errors.New("fulfilmentgroup: not found")

	// ErrForbidden covers both "you lack the permission" and "you hold it but
	// not for this branch". Deliberately one error: a probe must not be able
	// to tell a foreign tenant's parcel from a nonexistent one.
	ErrForbidden = errors.New("fulfilmentgroup: forbidden")

	ErrInvalid = errors.New("fulfilmentgroup: invalid input")

	// ErrVersionConflict is optimistic-concurrency failure.
	ErrVersionConflict = errors.New("fulfilmentgroup: version conflict")

	// ErrStalePlan means the caller planned against a version that is no
	// longer current. It is the answer the loser of a concurrent replan gets,
	// and it is deliberately not a retry-the-same-thing error.
	ErrStalePlan = errors.New("fulfilmentgroup: plan version is stale")

	// ErrConflict is a state conflict: a transition the aggregate refuses in
	// its current state.
	ErrConflict = errors.New("fulfilmentgroup: conflicts with current state")

	// ErrCustodySealed means the parcel has left the vendor's hands and its
	// historical manifest may no longer be rewritten.
	ErrCustodySealed = errors.New("fulfilmentgroup: parcel manifest is sealed by custody")

	ErrDependencyMissing   = errors.New("fulfilmentgroup: required dependency is not connected")
	ErrHandoverUnavailable = errors.New("fulfilmentgroup: vendor handover authority is unavailable")

	ErrDisabled = errors.New("fulfilmentgroup: commerce fulfilment is not enabled")
)

// Permission codes seeded by migration 000100.
const (
	PermRead          = "vendor.fulfilment.read"
	PermParcelManage  = "vendor.fulfilment.parcel.manage"
	PermMissionManage = "vendor.fulfilment.mission.manage"
)

// PlannerVersion identifies the planning rules that produced a plan version.
//
// Stored on every plan so a decision can be explained against the rules in
// force when it was made, rather than against whatever the rules became.
const PlannerVersion = "commerce-fulfilment-planner.v1"

// Outbox event types. They reuse the existing commerce event namespace rather
// than opening a second one.
const (
	EventGroupOpened      = "commerce.fulfilment.group_opened"
	EventPlanActivated    = "commerce.fulfilment.plan_activated"
	EventParcelCreated    = "commerce.fulfilment.parcel_created"
	EventParcelReady      = "commerce.fulfilment.parcel_ready"
	EventMissionRequested = "commerce.fulfilment.mission_requested"
	EventMissionLinked    = "commerce.fulfilment.mission_linked"
	EventVendorMission    = "commerce.fulfilment.vendor_mission_updated"
	EventReturnInitiated  = "commerce.fulfilment.return_initiated"
	EventCustodyRecorded  = "commerce.fulfilment.custody_recorded"

	AggregateType = "commerce_fulfilment_group"
)

// Readiness is the pickup obligation's operational state.
//
// It is deliberately narrower than VendorOrder.Status and cannot settle,
// cancel or complete anything commercial: a packing screen must not be able to
// move money.
type Readiness string

const (
	ReadinessNotReady  Readiness = "NOT_READY"
	ReadinessPacking   Readiness = "PACKING"
	ReadinessReady     Readiness = "READY"
	ReadinessCollected Readiness = "COLLECTED"
	ReadinessCancelled Readiness = "CANCELLED"
)

// Valid reports whether the readiness value is known.
func (r Readiness) Valid() bool {
	switch r {
	case ReadinessNotReady, ReadinessPacking, ReadinessReady,
		ReadinessCollected, ReadinessCancelled:
		return true
	default:
		return false
	}
}

// AdvanceReadiness reports whether a readiness change is legal.
//
// Forward-only, with the one exception that anything not yet collected may be
// cancelled. Allowing a collected unit to go back to packing would contradict
// custody evidence the logistics authority already recorded.
func AdvanceReadiness(from, to Readiness) error {
	if !from.Valid() || !to.Valid() {
		return ErrInvalid
	}
	if from == to {
		return nil
	}
	if to == ReadinessCancelled {
		if from == ReadinessCollected {
			return ErrConflict
		}
		return nil
	}
	rank := map[Readiness]int{
		ReadinessNotReady: 0, ReadinessPacking: 1,
		ReadinessReady: 2, ReadinessCollected: 3,
	}
	fromRank, fromOK := rank[from]
	toRank, toOK := rank[to]
	if !fromOK || !toOK || toRank <= fromRank {
		return ErrConflict
	}
	return nil
}

// Group is the enduring fulfilment planning root for one commerce order.
//
// It carries no status of its own. The commercial state of the purchase is
// already derived on the parent order, and a second independently-written copy
// of it could only ever drift; what the group adds is identity that survives
// every operational change, plus a pointer to the plan currently in force.
type Group struct {
	ID string `json:"fulfilment_group_id"`

	ParentOrderID string `json:"parent_order_id"`
	CustomerID    string `json:"-"`

	CurrentPlanID      string `json:"current_plan_id,omitempty"`
	CurrentPlanVersion int64  `json:"current_plan_version"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Units   []Unit   `json:"units,omitempty"`
	Parcels []Parcel `json:"parcels,omitempty"`
}

// Planned reports whether any plan version has been activated yet.
func (g Group) Planned() bool {
	return g.CurrentPlanVersion > 0 && strings.TrimSpace(g.CurrentPlanID) != ""
}

// Unit is one vendor order's pickup obligation at one branch.
//
// The relationship to the vendor order is permanent. Logistics replanning
// never creates a new vendor order and therefore never creates a new unit:
// where the goods are collected from is a fact about the purchase, not about
// the vehicle that happens to be collecting them.
type Unit struct {
	ID string `json:"fulfilment_unit_id"`

	GroupID       string `json:"fulfilment_group_id"`
	ParentOrderID string `json:"parent_order_id"`
	VendorOrderID string `json:"vendor_order_id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	PickupAddress   string  `json:"pickup_address,omitempty"`
	PickupLatitude  float64 `json:"-"`
	PickupLongitude float64 `json:"-"`

	ServiceClass string `json:"service_class,omitempty"`
	VehicleClass string `json:"vehicle_class,omitempty"`

	RequiredWeightGrams  int64 `json:"required_weight_grams"`
	RequiredPackageCount int64 `json:"required_package_count"`

	HandlingRequirements []string `json:"handling_requirements,omitempty"`

	Readiness Readiness `json:"readiness"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SameBranchObligation reports whether two units are the same pickup
// obligation.
//
// Business id alone is deliberately not enough. Two branches of one vendor are
// two places a rider has to physically go, and treating them as one obligation
// because the owning company matches is how a pickup gets silently skipped.
func SameBranchObligation(a, b Unit) bool {
	return a.BusinessID == b.BusinessID && a.BranchID == b.BranchID &&
		strings.TrimSpace(a.BranchID) != ""
}

// UnitInput is the server-resolved evidence a unit is opened from.
//
// There is no business_id or branch_id field a caller can set: both are read
// from the vendor order, so a caller can never declare a unit into another
// vendor's tenancy.
type UnitInput struct {
	VendorOrderID string

	PickupAddress   string
	PickupLatitude  float64
	PickupLongitude float64

	ServiceClass string
	VehicleClass string

	RequiredWeightGrams  int64
	RequiredPackageCount int64

	HandlingRequirements []string
}

// Validate reports the first reason a unit could not be opened.
func (in UnitInput) Validate() error {
	switch {
	case strings.TrimSpace(in.VendorOrderID) == "":
		return ErrInvalid
	case in.RequiredWeightGrams < 0 || in.RequiredPackageCount < 0:
		return ErrInvalid
	}
	return nil
}
