package fulfilmentgroup

// The versioned fulfilment plan.
//
// One plan version is one immutable planning decision. Replanning appends the
// next version; it never edits a previous one. That rule is the difference
// between a system that can explain why a delivery cost what it cost and one
// that can only show what it currently believes.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// PlanStatus is a plan version's lifecycle.
type PlanStatus string

const (
	// PlanDraft is written but not in force. It exists so activation can be a
	// transaction rather than a hope.
	PlanDraft PlanStatus = "DRAFT"

	PlanActive     PlanStatus = "ACTIVE"
	PlanSuperseded PlanStatus = "SUPERSEDED"

	// PlanAbandoned is a draft that never activated. It stays readable: a plan
	// that was considered and rejected is evidence too.
	PlanAbandoned PlanStatus = "ABANDONED"
)

// Valid reports whether the plan status is known.
func (s PlanStatus) Valid() bool {
	switch s {
	case PlanDraft, PlanActive, PlanSuperseded, PlanAbandoned:
		return true
	default:
		return false
	}
}

// PlanReason is why a plan version exists.
//
// The overflow values are the delivery feature's existing OverflowTrigger
// vocabulary, reused verbatim rather than duplicated under new names: a
// capacity overflow that produced a replan and a capacity overflow that
// produced a hybrid fallback are the same operational condition, and giving
// them two spellings would make them impossible to count together.
type PlanReason string

const (
	ReasonInitialPlan PlanReason = "INITIAL_PLAN"

	ReasonOwnRiderFirst    PlanReason = "OWN_RIDER_FIRST"
	ReasonDistanceOverflow PlanReason = "DISTANCE_OVERFLOW"
	ReasonTimeOverflow     PlanReason = "TIME_OVERFLOW"
	ReasonCapacityOverflow PlanReason = "CAPACITY_OVERFLOW"
	ReasonVehicleOverflow  PlanReason = "VEHICLE_OVERFLOW"
	ReasonSLAOverflow      PlanReason = "SLA_OVERFLOW"

	ReasonProviderUnavailable PlanReason = "PROVIDER_UNAVAILABLE"
	ReasonVendorUnavailable   PlanReason = "VENDOR_UNAVAILABLE"
	ReasonCustomerApproved    PlanReason = "CUSTOMER_APPROVED_CHANGE"
	ReasonMissionReplaced     PlanReason = "MISSION_REPLACED"
	ReasonReturn              PlanReason = "RETURN"
	ReasonTransfer            PlanReason = "TRANSFER"
)

// Valid reports whether the reason is known.
func (r PlanReason) Valid() bool {
	switch r {
	case ReasonInitialPlan, ReasonOwnRiderFirst, ReasonDistanceOverflow,
		ReasonTimeOverflow, ReasonCapacityOverflow, ReasonVehicleOverflow,
		ReasonSLAOverflow, ReasonProviderUnavailable, ReasonVendorUnavailable,
		ReasonCustomerApproved, ReasonMissionReplaced, ReasonReturn,
		ReasonTransfer:
		return true
	default:
		return false
	}
}

// Replan reports whether the reason may be used for a version after the first.
func (r PlanReason) Replan() bool {
	return r.Valid() && r != ReasonInitialPlan
}

// DeliveryMode is the plan-level execution preference, matching the delivery
// feature's existing vocabulary.
type DeliveryMode string

const (
	ModePlatform DeliveryMode = "PLATFORM"
	ModeVendor   DeliveryMode = "VENDOR"
	ModeHybrid   DeliveryMode = "HYBRID"
)

// Valid reports whether the delivery mode is known.
func (m DeliveryMode) Valid() bool {
	return m == ModePlatform || m == ModeVendor || m == ModeHybrid
}

// Owner is who executes one planned mission.
//
// HYBRID is deliberately absent. Hybrid is a planning behaviour that chooses
// between the two owners; at execution time a mission has an actual executor,
// and a third value would let a mission exist with nobody responsible for it.
type Owner string

const (
	OwnerPlatform Owner = "PLATFORM"
	OwnerVendor   Owner = "VENDOR"
)

// Valid reports whether the owner is known.
func (o Owner) Valid() bool { return o == OwnerPlatform || o == OwnerVendor }

// Authority names the service whose word is final about a mission.
type Authority string

const (
	AuthorityDispatch Authority = "DISPATCH"
	AuthorityHauling  Authority = "HAULING"

	// AuthorityTaxi is the distinct Taxi Goods authority. Runtime feature
	// flags still decide whether Vendor can select it for a deployment.
	AuthorityTaxi Authority = "TAXI"

	AuthorityVendor Authority = "VENDOR"
)

// Valid reports whether the authority is known.
func (a Authority) Valid() bool {
	switch a {
	case AuthorityDispatch, AuthorityHauling, AuthorityTaxi, AuthorityVendor:
		return true
	default:
		return false
	}
}

// Platform reports whether the authority is a platform logistics service.
func (a Authority) Platform() bool {
	return a == AuthorityDispatch || a == AuthorityHauling || a == AuthorityTaxi
}

// ActiveForCommerce reports whether a mission may actually be created against
// this authority today.
func (a Authority) ActiveForCommerce() bool {
	return a == AuthorityDispatch || a == AuthorityTaxi || a == AuthorityHauling || a == AuthorityVendor
}

// OwnerFor returns the executor implied by an authority.
func OwnerFor(authority Authority) Owner {
	if authority == AuthorityVendor {
		return OwnerVendor
	}
	return OwnerPlatform
}

// StopType distinguishes a collection from the final handover.
type StopType string

const (
	StopPickup  StopType = "PICKUP"
	StopDropoff StopType = "DROPOFF"
)

// Valid reports whether the stop type is known.
func (s StopType) Valid() bool { return s == StopPickup || s == StopDropoff }

// Stop is one point on a mission's route.
type Stop struct {
	ID string `json:"stop_id,omitempty"`

	Sequence int      `json:"sequence"`
	Type     StopType `json:"stop_type"`

	// UnitID is set on pickup stops and empty on the dropoff: a collection
	// names what it collects, a delivery does not.
	UnitID string `json:"fulfilment_unit_id,omitempty"`

	BusinessID string `json:"business_id,omitempty"`
	BranchID   string `json:"branch_id,omitempty"`

	Address   string  `json:"address,omitempty"`
	Latitude  float64 `json:"-"`
	Longitude float64 `json:"-"`
}

// PlanMission is one planned mission slot inside one plan version.
//
// SlotRef is the stable identity that makes mission creation idempotent. A
// recovery pass asks "does this slot already have a live mission" — a question
// the database can answer — rather than "did my earlier request arrive", which
// nothing durable can answer after a lost response.
type PlanMission struct {
	ID     string `json:"plan_mission_id"`
	PlanID string `json:"plan_id"`

	SlotRef  string `json:"slot_ref"`
	Sequence int    `json:"sequence"`

	Owner     Owner     `json:"owner"`
	Authority Authority `json:"authority"`

	ServiceClass string `json:"service_class,omitempty"`
	VehicleClass string `json:"vehicle_class,omitempty"`

	PlannedCostMinor       int64  `json:"planned_cost_minor"`
	Currency               string `json:"currency,omitempty"`
	LoadingDurationSeconds int64  `json:"loading_duration_seconds,omitempty"`

	Stops     []Stop   `json:"stops"`
	ParcelIDs []string `json:"parcel_ids"`

	CreatedAt time.Time `json:"created_at"`
}

// PickupStops returns the collection stops in sequence order.
func (m PlanMission) PickupStops() []Stop {
	var stops []Stop
	for _, stop := range m.Stops {
		if stop.Type == StopPickup {
			stops = append(stops, stop)
		}
	}
	sort.Slice(stops, func(i, j int) bool {
		return stops[i].Sequence < stops[j].Sequence
	})
	return stops
}

// Dropoff returns the single receiver destination.
func (m PlanMission) Dropoff() (Stop, bool) {
	for _, stop := range m.Stops {
		if stop.Type == StopDropoff {
			return stop, true
		}
	}
	return Stop{}, false
}

// UnitIDs returns the units this mission collects, in stop order.
func (m PlanMission) UnitIDs() []string {
	var units []string
	for _, stop := range m.PickupStops() {
		if strings.TrimSpace(stop.UnitID) != "" {
			units = append(units, stop.UnitID)
		}
	}
	return units
}

// Validate reports the first reason a mission slot could not be planned.
func (m PlanMission) Validate() error {
	switch {
	case strings.TrimSpace(m.SlotRef) == "":
		return ErrInvalid
	case m.Sequence < 1:
		return ErrInvalid
	case !m.Owner.Valid() || !m.Authority.Valid():
		return ErrInvalid
	case OwnerFor(m.Authority) != m.Owner:
		// A vendor-owned mission executed by a platform authority, or the
		// reverse, would make responsibility unanswerable.
		return ErrInvalid
	case m.PlannedCostMinor < 0:
		return ErrInvalid
	case m.LoadingDurationSeconds < 0 || m.LoadingDurationSeconds > 8*60*60:
		return ErrInvalid
	case m.Currency != "" && len(m.Currency) != 3:
		return ErrInvalid
	}

	pickups, dropoffs := 0, 0
	sequences := map[int]bool{}
	for _, stop := range m.Stops {
		if !stop.Type.Valid() || stop.Sequence < 1 {
			return ErrInvalid
		}
		if sequences[stop.Sequence] {
			// Two stops claiming the same position is not a route.
			return ErrInvalid
		}
		sequences[stop.Sequence] = true

		switch stop.Type {
		case StopPickup:
			if strings.TrimSpace(stop.UnitID) == "" {
				return ErrInvalid
			}
			pickups++
		case StopDropoff:
			if strings.TrimSpace(stop.UnitID) != "" {
				return ErrInvalid
			}
			dropoffs++
		}
	}
	if pickups < 1 || dropoffs != 1 {
		// Exactly one receiver destination per mission. Batching several
		// customers onto one mission is a different capability that this
		// section does not own.
		return ErrInvalid
	}
	return nil
}

// Plan is one immutable fulfilment planning decision.
type Plan struct {
	ID      string `json:"plan_id"`
	GroupID string `json:"fulfilment_group_id"`

	Version int64      `json:"version"`
	Status  PlanStatus `json:"status"`

	Reason       PlanReason `json:"reason_code"`
	ReasonDetail string     `json:"reason_detail,omitempty"`

	PlannerVersion string `json:"planner_version"`
	PolicyVersion  int64  `json:"policy_version"`

	DeliveryMode DeliveryMode `json:"delivery_mode"`

	// PlannedCostMinor is planning EVIDENCE in integer minor units. It is not
	// a ledger entry and creates no financial obligation; payment-wallet
	// remains the authority for what was actually charged.
	PlannedCostMinor int64           `json:"planned_cost_minor"`
	Currency         string          `json:"currency,omitempty"`
	CostSnapshot     json.RawMessage `json:"cost_snapshot,omitempty"`
	RoutingEvidence  json.RawMessage `json:"routing_evidence,omitempty"`

	Fingerprint string `json:"fingerprint"`

	SupersedesVersion int64 `json:"supersedes_version,omitempty"`

	IdempotencyKey string `json:"-"`

	CreatedAt    time.Time  `json:"created_at"`
	ActivatedAt  *time.Time `json:"activated_at,omitempty"`
	SupersededAt *time.Time `json:"superseded_at,omitempty"`

	Missions []PlanMission `json:"missions"`

	// RequirementDecisions is the vehicle-selection evidence behind this plan
	// version: what the load needed, which classes were considered, and why
	// the chosen one was chosen. It is read back with the plan so a plan
	// history is self-explaining rather than a list of vehicle names.
	RequirementDecisions []RequirementDecision `json:"requirement_decisions,omitempty"`
}

// Live reports whether the plan is the one currently in force.
func (p Plan) Live() bool { return p.Status == PlanActive }

// PlanInput is a request to produce the next plan version for a group.
//
// ExpectedVersion is the optimistic-concurrency control that makes concurrent
// replanning safe: the caller states which version it planned against, and a
// caller that planned against a superseded version is told its plan is stale
// rather than silently winning.
type PlanInput struct {
	ExpectedVersion int64

	Reason       PlanReason
	ReasonDetail string

	PolicyVersion int64
	DeliveryMode  DeliveryMode

	PlannedCostMinor int64
	Currency         string
	CostSnapshot     json.RawMessage
	RoutingEvidence  json.RawMessage

	IdempotencyKey string

	Missions []PlanMission

	// RequirementDecisions is the vehicle-selection evidence to store with
	// this version. It is written in the SAME transaction as the plan, so a
	// plan version can never exist without the evidence that justified it -
	// which is exactly what a crash between two writes would otherwise leave
	// behind, and exactly the row Support would then be unable to explain.
	RequirementDecisions []RequirementDecision
}

// Validate reports the first reason a plan version could not be produced.
func (in PlanInput) Validate() error {
	switch {
	case strings.TrimSpace(in.IdempotencyKey) == "":
		return ErrInvalid
	case !in.Reason.Valid():
		return ErrInvalid
	case !in.DeliveryMode.Valid():
		return ErrInvalid
	case in.ExpectedVersion < 0:
		return ErrInvalid
	case in.PlannedCostMinor < 0:
		return ErrInvalid
	case in.Currency != "" && len(in.Currency) != 3:
		return ErrInvalid
	case len(in.Missions) == 0:
		return ErrInvalid
	case len(in.Missions) > maxMissionsPerPlan:
		return ErrInvalid
	}

	// The first version is the initial plan and nothing else; every later one
	// must name the operational condition that forced it.
	if in.ExpectedVersion == 0 && in.Reason != ReasonInitialPlan {
		return ErrInvalid
	}
	if in.ExpectedVersion > 0 && !in.Reason.Replan() {
		return ErrInvalid
	}

	slots := map[string]bool{}
	sequences := map[int]bool{}
	for _, mission := range in.Missions {
		if err := mission.Validate(); err != nil {
			return err
		}
		if slots[mission.SlotRef] || sequences[mission.Sequence] {
			return ErrInvalid
		}
		slots[mission.SlotRef] = true
		sequences[mission.Sequence] = true

		if !mission.Authority.ActiveForCommerce() {
			// Refused, not attempted. A plan naming an authority that cannot
			// execute is a delivery nobody will perform.
			return ErrConflict
		}
	}

	// Evidence is validated with the plan it justifies. A decision rejected
	// after the plan committed would leave a version nobody can explain.
	slotted := map[string]bool{}

	for _, decision := range in.RequirementDecisions {
		if err := decision.Validate(); err != nil {
			return err
		}

		if slotted[decision.SlotRef] {
			// Two decisions for one slot is not evidence, it is a
			// contradiction about the same mission.
			return ErrInvalid
		}

		slotted[decision.SlotRef] = true

		// Evidence must describe a mission this plan actually contains.
		// Otherwise a decision could explain a slot that does not exist while
		// the slot that does exist stays unexplained.
		if decision.SlotRef != "" && !slots[decision.SlotRef] {
			return ErrInvalid
		}
	}

	return nil
}

// maxMissionsPerPlan bounds one plan. A group needing more missions than this
// is a planning fault, not a large order.
const maxMissionsPerPlan = 64

// Fingerprint is a stable digest of the planning semantics of one plan.
//
// It covers what would make two plans operationally different — the missions,
// their executors, their routes and the parcels riding them — and deliberately
// excludes identifiers and timestamps, which differ between two attempts at
// the same decision. That is what makes it usable for stale-plan detection and
// replan idempotency rather than just a checksum of a row.
func Fingerprint(mode DeliveryMode, missions []PlanMission) string {
	ordered := append([]PlanMission(nil), missions...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].SlotRef < ordered[j].SlotRef
	})

	digest := sha256.New()
	fmt.Fprintf(digest, "mode=%s\n", mode)

	for _, mission := range ordered {
		fmt.Fprintf(digest, "slot=%s|owner=%s|authority=%s|service=%s|vehicle=%s\n",
			mission.SlotRef, mission.Owner, mission.Authority,
			mission.ServiceClass, mission.VehicleClass)

		stops := append([]Stop(nil), mission.Stops...)
		sort.Slice(stops, func(i, j int) bool {
			return stops[i].Sequence < stops[j].Sequence
		})
		for _, stop := range stops {
			fmt.Fprintf(digest, "  stop=%d|%s|unit=%s|branch=%s\n",
				stop.Sequence, stop.Type, stop.UnitID, stop.BranchID)
		}

		parcels := append([]string(nil), mission.ParcelIDs...)
		sort.Strings(parcels)
		for _, parcel := range parcels {
			fmt.Fprintf(digest, "  parcel=%s\n", parcel)
		}
	}

	return hex.EncodeToString(digest.Sum(nil))
}
