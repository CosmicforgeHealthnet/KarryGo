// Package orchestration is Section 11's just-in-time fulfilment planner:
// Plan, then Warm Match, then Commit.
//
// The problem it solves is a rider standing outside a kitchen. Fulfilment used
// to be planned when the order was placed, which meant a mission existed before
// anyone had started cooking. The provider arrives, waits, and the platform
// pays for the waiting.
//
// So commitment is deferred and continuously recalculated. Plan decides WHAT is
// needed. Warm Match asks each authority whether it probably could, creating
// nothing. Commit happens once, as late as the readiness prediction and the
// authority's own pickup estimate allow.
//
// Two boundaries hold this together and neither is negotiable.
//
// Vendor selects an AUTHORITY and a PRODUCT. It never selects a provider. There
// is no field on any type here for a driver, rider, vehicle or fleet member,
// and a reflective test proves it -- because provider identity is the executing
// service's to resolve, after Vendor's work is done.
//
// And selection is never by price. Which authority carries an order is decided
// by physical suitability, serviceability, product eligibility, supply and
// operational policy. Nothing in this package reads a fare, and a test asserts
// no pricing package is imported.
package orchestration

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// State is the orchestration lifecycle.
type State string

const (
	// StatePendingRequirements means the order exists but its transport
	// requirement cannot yet be built.
	StatePendingRequirements State = "PENDING_REQUIREMENTS"

	// StatePlanned means the requirement is known and an authority may be
	// chosen. Nothing has been asked of any authority yet.
	StatePlanned State = "PLANNED"

	// StateWaitingReadiness means the plan is sound and the branch is still
	// preparing. This is where an order spends most of its life.
	StateWaitingReadiness State = "WAITING_READINESS"

	// StateWarmMatch means supply is being checked, repeatedly, as commitment
	// approaches. It creates nothing.
	StateWarmMatch State = "WARM_MATCH"

	// StateCommitDue means the moment has arrived.
	StateCommitDue State = "COMMIT_DUE"

	// StateCommitting means a mission create is in flight. A crash here is the
	// dangerous one, which is why the reconciler reads back rather than retries.
	StateCommitting State = "COMMITTING"

	StateCommitted   State = "COMMITTED"
	StateInExecution State = "IN_EXECUTION"
	StateCompleted   State = "COMPLETED"

	// Side states.
	StateRecovering State = "RECOVERING"
	StateOpsReview  State = "OPS_REVIEW"
	StateCancelled  State = "CANCELLED"
	StateFailed     State = "FAILED"
)

// Valid reports whether a state is recognised.
func (s State) Valid() bool {
	_, known := allowedTransitions[s]
	return known
}

// Terminal reports whether the orchestration is finished.
func (s State) Terminal() bool {
	return s == StateCompleted || s == StateCancelled || s == StateFailed
}

// Committed reports whether an execution authority now owns this work.
//
// It is the single most consequential predicate here: after it becomes true,
// Vendor may no longer re-time, reselect or rematch. The authority owns
// provider recovery, and a second planner touching it would be a second
// dispatch.
func (s State) Committed() bool {
	switch s {
	case StateCommitted, StateInExecution, StateCompleted, StateRecovering:
		return true
	}
	return false
}

var allowedTransitions = map[State]map[State]bool{
	StatePendingRequirements: {StatePlanned: true, StateCancelled: true, StateFailed: true, StateOpsReview: true},
	StatePlanned:             {StateWaitingReadiness: true, StateWarmMatch: true, StateCommitDue: true, StateCancelled: true, StateFailed: true, StateOpsReview: true},
	StateWaitingReadiness:    {StateWarmMatch: true, StateCommitDue: true, StatePlanned: true, StateCancelled: true, StateFailed: true, StateOpsReview: true},
	// Warm Match can fall back to waiting: a vendor who slips before
	// commitment must un-arm the commit rather than have a rider sent anyway.
	StateWarmMatch:  {StateCommitDue: true, StateWaitingReadiness: true, StatePlanned: true, StateCancelled: true, StateFailed: true, StateOpsReview: true},
	StateCommitDue:  {StateCommitting: true, StateWarmMatch: true, StateWaitingReadiness: true, StateCancelled: true, StateFailed: true, StateOpsReview: true},
	StateCommitting: {StateCommitted: true, StateFailed: true, StateOpsReview: true, StateCommitDue: true},
	// After commitment the authority drives. Vendor projects.
	StateCommitted:   {StateInExecution: true, StateRecovering: true, StateCompleted: true, StateCancelled: true, StateOpsReview: true},
	StateInExecution: {StateCompleted: true, StateRecovering: true, StateCancelled: true, StateOpsReview: true},
	StateRecovering:  {StateInExecution: true, StateCompleted: true, StateOpsReview: true, StateCancelled: true, StateFailed: true},
	StateOpsReview:   {StatePlanned: true, StateWaitingReadiness: true, StateCommitDue: true, StateInExecution: true, StateCancelled: true, StateFailed: true},
	StateCompleted:   {},
	StateCancelled:   {},
	StateFailed:      {},
}

// CanTransition reports whether a state move is allowed.
func CanTransition(from, to State) bool {
	if from == to {
		return from.Valid()
	}
	next, known := allowedTransitions[from]
	return known && next[to]
}

// SelectionSource records how the authority was chosen.
type SelectionSource string

const (
	// SelectionAutomatic is the normal path: chosen from the authorities' own
	// answers.
	SelectionAutomatic SelectionSource = "AUTOMATIC"

	// SelectionAdminOverride is an audited platform-operator decision.
	SelectionAdminOverride SelectionSource = "ADMIN_OVERRIDE"

	// SelectionAssistedOverride is an authorized operational decision made
	// alongside the automatic one.
	SelectionAssistedOverride SelectionSource = "ASSISTED_OVERRIDE"
)

// Valid reports whether a selection source is recognised.
func (s SelectionSource) Valid() bool {
	return s == SelectionAutomatic || s == SelectionAdminOverride || s == SelectionAssistedOverride
}

// RecoveryState is who is currently repairing this work.
type RecoveryState string

const (
	RecoveryNone RecoveryState = "NONE"
	// RecoveryAuthority means the executing authority is replacing its own
	// provider. Vendor observes and does not intervene.
	RecoveryAuthority RecoveryState = "AUTHORITY_RECOVERING"
	// RecoveryReplanning is Vendor's own pre-commitment replan.
	RecoveryReplanning RecoveryState = "REPLANNING"
	// RecoveryReconciling means the mission's true state is unknown and is
	// being read back from the authority.
	RecoveryReconciling RecoveryState = "RECONCILING"
)

// Sentinel errors.
var (
	ErrValidation = errors.New("orchestration: validation failed")
	ErrNotFound   = errors.New("orchestration: not found")
	ErrVersion    = errors.New("orchestration: version conflict")
	ErrDisabled   = errors.New("orchestration: not enabled")
	ErrTransition = errors.New("orchestration: transition is not permitted")
	// ErrCommitted means the caller tried to do something only a pre-commitment
	// orchestration allows. After commitment the authority owns the work.
	ErrCommitted = errors.New("orchestration: the executing authority already owns this work")
	// ErrNoEligibleAuthority means every configured authority refused on
	// physical or product grounds. It is distinct from ErrAuthoritiesUnreachable
	// because one is a fact about the goods and the other is a fact about the
	// network.
	ErrNoEligibleAuthority = errors.New("orchestration: no authority can carry this")
	// ErrAuthoritiesUnreachable means nothing could be asked. Retrying may
	// succeed; concluding the goods cannot be carried would be inventing a fact.
	ErrAuthoritiesUnreachable = errors.New("orchestration: no authority could be reached")
)

// Orchestration is one fulfilment group's plan.
//
// Note what is absent: no provider, no driver, no rider, no vehicle, no fleet
// member, and no price. Both absences are enforced by tests rather than left to
// review.
type Orchestration struct {
	ID                 string `json:"id"`
	FulfilmentGroupRef string `json:"fulfilment_group_ref"`
	ParentOrderID      string `json:"parent_order_id"`
	PlanVersion        int64  `json:"plan_version"`
	State              State  `json:"state"`

	Requirement          Requirement `json:"requirements_snapshot"`
	RequirementsFrozenAt *time.Time  `json:"requirements_frozen_at,omitempty"`

	SelectedAuthority       logistics.Authority `json:"selected_authority,omitempty"`
	SelectedProductCode     string              `json:"selected_product_code,omitempty"`
	CapabilityPolicyVersion string              `json:"capability_policy_version,omitempty"`
	SelectionSource         SelectionSource     `json:"selection_source"`

	DesiredPickupStart *time.Time `json:"desired_pickup_start,omitempty"`
	DesiredPickupEnd   *time.Time `json:"desired_pickup_end,omitempty"`
	CommitNotBefore    *time.Time `json:"commit_not_before,omitempty"`
	NextEvaluationAt   *time.Time `json:"next_evaluation_at,omitempty"`

	CurrentMissionRef string        `json:"current_mission_ref,omitempty"`
	RecoveryState     RecoveryState `json:"recovery_state"`
	LastReasonCode    string        `json:"last_reason_code,omitempty"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Requirement is the normalized transport requirement.
//
// It is built ONCE from immutable fulfilment evidence and sent unchanged to
// every candidate authority. Building a per-authority shape would let the same
// order be described differently to each, and the one that said yes would be
// answering a different question from the one that said no.
type Requirement struct {
	// Cargo, in the shared contract's units.
	WeightGrams        int64    `json:"weight_grams,omitempty"`
	VolumeMl           int64    `json:"volume_ml,omitempty"`
	LongestDimensionMm int64    `json:"longest_dimension_mm,omitempty"`
	PackageCount       int64    `json:"package_count,omitempty"`
	Handling           []string `json:"handling,omitempty"`

	// Stops. Counted from the actual pickups rather than declared, so a
	// three-vendor order cannot be described to the gate as a one-pickup order.
	PickupCount  int64 `json:"pickup_count"`
	DropoffCount int64 `json:"dropoff_count"`

	// Protected handover. A requirement that can be downgraded is not a
	// requirement: an authority that cannot verify a delivery credential must
	// be refused rather than quietly given the work.
	SecurePickup   bool `json:"secure_pickup,omitempty"`
	SecureDelivery bool `json:"secure_delivery,omitempty"`

	// Timing.
	DesiredPickupStart time.Time `json:"desired_pickup_start"`
	DesiredPickupEnd   time.Time `json:"desired_pickup_end"`
	DeliveryDeadline   time.Time `json:"delivery_deadline,omitempty"`

	// Geography.
	PickupLatitude   float64 `json:"pickup_latitude"`
	PickupLongitude  float64 `json:"pickup_longitude"`
	DropoffLatitude  float64 `json:"dropoff_latitude"`
	DropoffLongitude float64 `json:"dropoff_longitude"`

	// Temperature is the narrowest band any item in the group needs. Two orders
	// with incompatible bands cannot share a vehicle, which is why the planner
	// reads it before grouping.
	TemperatureBand string `json:"temperature_band,omitempty"`
}

// Validate refuses a requirement that could not be executed as stated.
func (r Requirement) Validate() error {
	switch {
	case r.PickupCount < 1:
		return fmt.Errorf("%w: at least one pickup is required", ErrValidation)
	case r.DropoffCount < 1:
		return fmt.Errorf("%w: at least one dropoff is required", ErrValidation)
	case r.PickupLatitude == 0 && r.PickupLongitude == 0:
		return fmt.Errorf("%w: a real pickup coordinate is required", ErrValidation)
	case r.DropoffLatitude == 0 && r.DropoffLongitude == 0:
		return fmt.Errorf("%w: a real dropoff coordinate is required", ErrValidation)
	case !r.DesiredPickupEnd.IsZero() && r.DesiredPickupEnd.Before(r.DesiredPickupStart):
		return fmt.Errorf("%w: the pickup window ends before it starts", ErrValidation)
	}
	return nil
}

// Capability projects the requirement onto the shared capability contract.
//
// One projection, used for both the serviceability gate and the supply preview,
// so an authority is never asked two subtly different questions about one order.
func (r Requirement) Capability() logistics.Requirement {
	return logistics.Requirement{
		PickupCount:        r.PickupCount,
		DropoffCount:       r.DropoffCount,
		WeightGrams:        r.WeightGrams,
		VolumeMl:           r.VolumeMl,
		LongestDimensionMm: r.LongestDimensionMm,
		PackageCount:       r.PackageCount,
		Handling:           r.Handling,
		SecurePickup:       r.SecurePickup,
		SecureDelivery:     r.SecureDelivery,
	}
}

// Preview projects the requirement onto the supply-preview contract.
func (r Requirement) Preview(productCode string) supplypreview.Request {
	pickupAt := r.DesiredPickupStart
	request := supplypreview.Request{
		PickupLatitude:   r.PickupLatitude,
		PickupLongitude:  r.PickupLongitude,
		DropoffLatitude:  r.DropoffLatitude,
		DropoffLongitude: r.DropoffLongitude,
		ProductCode:      productCode,
		WeightGrams:      r.WeightGrams,
		VolumeMl:         r.VolumeMl,
		PackageCount:     r.PackageCount,
		Handling:         r.Handling,
		SecurePickup:     r.SecurePickup,
		SecureDelivery:   r.SecureDelivery,
		PickupCount:      r.PickupCount,
		DropoffCount:     r.DropoffCount,
	}
	if !pickupAt.IsZero() {
		request.RequestedPickupAt = &pickupAt
	}
	// The capability contract bounds one edge rather than three, so the longest
	// is supplied directly instead of being re-derived from dimensions this
	// requirement does not carry separately.
	request.LengthMm = r.LongestDimensionMm
	return request
}

// Pickup is one vendor order collected by this orchestration.
//
// Persisted per vendor order even when two share an address: custody,
// manifests and handover are per vendor, and collapsing co-located pickups
// would make a partial collection unrepresentable.
type Pickup struct {
	VendorOrderID string `json:"vendor_order_id"`
	Sequence      int    `json:"sequence"`
	BusinessID    string `json:"business_id"`
	BranchID      string `json:"branch_id"`
}

// DecisionEvent is one piece of append-only selection evidence.
//
// It is what makes "why did this go by truck?" answerable months later, and
// what keeps an authority that refused distinguishable from one that could not
// be asked.
type DecisionEvent struct {
	ID                        string              `json:"id"`
	OrchestrationID           string              `json:"orchestration_id"`
	PlanVersion               int64               `json:"plan_version"`
	EventType                 string              `json:"event_type"`
	ReadinessVersion          int64               `json:"readiness_version"`
	CandidateAuthority        logistics.Authority `json:"candidate_authority,omitempty"`
	CandidateProduct          string              `json:"candidate_product_code,omitempty"`
	Serviceable               *bool               `json:"serviceable,omitempty"`
	OutcomeClass              OutcomeClass        `json:"outcome_class,omitempty"`
	ReasonCode                string              `json:"reason_code,omitempty"`
	SupplyStatus              string              `json:"supply_status,omitempty"`
	EstimatedPickupETASeconds int64               `json:"estimated_pickup_eta_seconds,omitempty"`
	PolicyVersion             string              `json:"policy_version,omitempty"`
	Selected                  bool                `json:"selected"`
	ActorRef                  string              `json:"actor_ref,omitempty"`
	OverrideReason            string              `json:"override_reason,omitempty"`
	CorrelationID             string              `json:"correlation_id,omitempty"`
	CreatedAt                 time.Time           `json:"created_at"`
}

// Event types.
const (
	EventRequirementBuilt      = "REQUIREMENT_BUILT"
	EventServiceabilityChecked = "SERVICEABILITY_CHECKED"
	EventSupplyPreviewed       = "SUPPLY_PREVIEWED"
	EventAuthoritySelected     = "AUTHORITY_SELECTED"
	EventCommitScheduled       = "COMMIT_SCHEDULED"
	EventCommitAttempted       = "COMMIT_ATTEMPTED"
	EventMissionCommitted      = "MISSION_COMMITTED"
	EventReplanned             = "REPLANNED"
	EventRecoveryObserved      = "RECOVERY_OBSERVED"
	EventOverridden            = "OVERRIDDEN"
)

// OutcomeClass is why a candidate did or did not survive.
//
// The distinction the whole selection rests on is between the middle group
// (facts about the goods) and DependencyUnavailable (a fact about the network).
// A caller that collapses them turns one timeout into a standing belief that an
// authority does not serve a city.
type OutcomeClass string

const (
	OutcomeServiceable OutcomeClass = "SERVICEABLE"

	// Positive incompatibilities. The authority answered, and the answer is no.
	OutcomeCapabilityMismatch OutcomeClass = "CAPABILITY_MISMATCH"
	OutcomeNotServiceable     OutcomeClass = "NOT_SERVICEABLE"
	OutcomeSupplyUnavailable  OutcomeClass = "SUPPLY_UNAVAILABLE"
	OutcomeCredential         OutcomeClass = "CREDENTIAL"

	// Not a refusal. Nobody answered.
	OutcomeDependencyUnavailable OutcomeClass = "DEPENDENCY_UNAVAILABLE"

	// Deployment state, not an authority's opinion.
	OutcomeDisabled OutcomeClass = "DISABLED"
)

// PositiveIncompatibility reports whether the authority actually refused.
//
// Only a true here may eliminate a candidate permanently. Everything else means
// ask again later.
func (o OutcomeClass) PositiveIncompatibility() bool {
	switch o {
	case OutcomeCapabilityMismatch, OutcomeNotServiceable, OutcomeCredential:
		return true
	}
	return false
}

// Eliminates reports whether a candidate is out for THIS evaluation.
//
// Supply being unavailable right now eliminates the candidate for this pass
// without being a permanent fact, which is why it is separated from the
// positive incompatibilities above.
func (o OutcomeClass) Eliminates() bool {
	return o != OutcomeServiceable
}

// ClassifyFailure maps the logistics integration's failure categories onto
// this package's outcome classes.
func ClassifyFailure(category logistics.FailureCategory, reasonCode string) OutcomeClass {
	switch category {
	case logistics.FailureCapabilityMismatch:
		return OutcomeCapabilityMismatch
	case logistics.FailureCredential:
		return OutcomeCredential
	case logistics.FailureDependencyUnavailable:
		if strings.Contains(strings.ToUpper(reasonCode), "DISABLED") {
			return OutcomeDisabled
		}
		return OutcomeDependencyUnavailable
	case logistics.FailureNotServiceable:
		return OutcomeNotServiceable
	default:
		return OutcomeNotServiceable
	}
}

// Candidate is one authority's answer for one evaluation.
type Candidate struct {
	Authority         logistics.Authority
	ProductCode       string
	Outcome           OutcomeClass
	ReasonCode        string
	SupplyStatus      supplypreview.SupplyStatus
	PickupETA         time.Duration
	PolicyVersion     string
	CapabilityVersion int64
}

// Usable reports whether this candidate may be selected now.
func (c Candidate) Usable() bool { return c.Outcome == OutcomeServiceable }

// authorityFamilySize ranks authorities by the size of vehicle they represent.
//
// It is the ONLY ordering selection applies, and it is a physical judgement
// rather than a commercial one: a bike is smaller than a car, a car is smaller
// than a truck. Prefer the smallest adequate family, which is a congestion and
// suitability rule. Nothing here reads a fare, and no authority is preferred
// for being cheaper.
func authorityFamilySize(authority logistics.Authority) int {
	switch authority {
	case logistics.Dispatch:
		return 1
	case logistics.Taxi:
		return 2
	case logistics.Hauling:
		return 3
	default:
		return 99
	}
}

// SelectAuthority picks the authority to use from the evaluated candidates.
//
// The policy is "smallest adequate", applied only among candidates that passed
// every hard requirement. A larger vehicle is never chosen because a smaller
// one is busy -- that is what the supply signal is for, and it eliminates a
// candidate before it reaches here.
//
// It returns the distinguishable failures rather than a single "no": the caller
// must be able to tell an order that cannot be carried from one that could not
// be asked about, because the first is final and the second is a retry.
func SelectAuthority(candidates []Candidate) (Candidate, error) {
	usable := make([]Candidate, 0, len(candidates))
	positives, unreachable := 0, 0
	for _, candidate := range candidates {
		switch {
		case candidate.Usable():
			usable = append(usable, candidate)
		case candidate.Outcome.PositiveIncompatibility():
			positives++
		default:
			unreachable++
		}
	}
	if len(usable) == 0 {
		if unreachable > 0 {
			// At least one authority never answered. Concluding the goods
			// cannot be carried would be inventing a fact about them.
			return Candidate{}, ErrAuthoritiesUnreachable
		}
		if positives > 0 {
			return Candidate{}, ErrNoEligibleAuthority
		}
		return Candidate{}, ErrNoEligibleAuthority
	}

	sort.SliceStable(usable, func(i, j int) bool {
		left, right := usable[i], usable[j]
		if authorityFamilySize(left.Authority) != authorityFamilySize(right.Authority) {
			return authorityFamilySize(left.Authority) < authorityFamilySize(right.Authority)
		}
		// Among equals, a confident supply signal beats a thin one. This is
		// still not price: it is the likelihood the commit will actually match.
		return supplyRank(left.SupplyStatus) < supplyRank(right.SupplyStatus)
	})
	return usable[0], nil
}

func supplyRank(status supplypreview.SupplyStatus) int {
	switch status {
	case supplypreview.StatusAvailable:
		return 0
	case supplypreview.StatusLimited:
		return 1
	default:
		return 2
	}
}
