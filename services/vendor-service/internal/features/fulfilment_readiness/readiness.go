// Package fulfilment_readiness is Section 11's generalized readiness model:
// what state an order is actually in, when it will be ready, and how much that
// estimate can be trusted.
//
// Vendor already recorded that a production job had promised a window. What it
// could not say was whether anyone had started, what they were doing, who last
// said so, or how confident the estimate was. Without those, fulfilment can
// only be planned by assuming: a rider is dispatched when the order is placed
// and waits outside a kitchen that has not begun cooking.
//
// Two rules shape everything here.
//
// The first is that readiness is Vendor's to declare and nobody else's. A
// customer cannot mark their own order ready. Neither can the rider standing at
// the counter, however obvious it looks to them: they can see a bag, not
// whether it is the right bag, complete, and paid for.
//
// The second is that a correction never erases the original claim. A vendor who
// declares READY and then says "actually, ten more minutes" has still declared
// it, and a dispute about a rider who arrived to nothing depends on that record
// surviving. first_actual_ready_at is write-once, enforced by the database.
package fulfilment_readiness

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// State is the canonical readiness progression.
type State string

const (
	StateNotStarted State = "NOT_STARTED"
	StateAccepted   State = "ACCEPTED"
	StatePreparing  State = "PREPARING"
	StateReadySoon  State = "READY_SOON"
	StateReady      State = "READY"

	// Side states. They are not later stages: an order can be BLOCKED at any
	// point and return to where it was.
	StateBlocked   State = "BLOCKED"
	StateDelayed   State = "DELAYED"
	StateCancelled State = "CANCELLED"
)

// Valid reports whether a state is recognised.
func (s State) Valid() bool {
	switch s {
	case StateNotStarted, StateAccepted, StatePreparing, StateReadySoon, StateReady,
		StateBlocked, StateDelayed, StateCancelled:
		return true
	}
	return false
}

// Terminal reports whether nothing further will happen.
func (s State) Terminal() bool { return s == StateCancelled }

// Active reports whether the order is still being worked on, which is what the
// stale-signal sweep and the orchestrator both mean by "in progress".
func (s State) Active() bool {
	switch s {
	case StateAccepted, StatePreparing, StateReadySoon, StateDelayed, StateBlocked:
		return true
	}
	return false
}

// Source is who moved the readiness.
type Source string

const (
	// SourceVendorStaff is a person at the branch.
	SourceVendorStaff Source = "VENDOR_STAFF"

	// SourceSystem is an adapter applying a Vendor-owned production, picking or
	// batch event. It is never a customer and never a provider: both are
	// outside the branch and neither can see whether an order is complete.
	SourceSystem Source = "SYSTEM"

	// SourceAdmin is a platform operator's audited override.
	SourceAdmin Source = "ADMIN"
)

// Valid reports whether a source is recognised.
func (s Source) Valid() bool {
	return s == SourceVendorStaff || s == SourceSystem || s == SourceAdmin
}

// ConfidenceBand is the coarse trust in a prediction.
//
// Three bands rather than a number, because the consumer is a planner deciding
// how much buffer to leave, and it cannot act on the difference between 0.61
// and 0.64.
type ConfidenceBand string

const (
	ConfidenceLow    ConfidenceBand = "LOW"
	ConfidenceMedium ConfidenceBand = "MEDIUM"
	ConfidenceHigh   ConfidenceBand = "HIGH"
)

// BandFor maps a score onto a band. The thresholds are the only place the
// mapping exists, so a planner and an operator surface never disagree about
// what "HIGH" means.
func BandFor(score float64) ConfidenceBand {
	switch {
	case score >= 0.75:
		return ConfidenceHigh
	case score >= 0.45:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

// Sentinel errors.
var (
	ErrValidation = errors.New("readiness: validation failed")
	ErrForbidden  = errors.New("readiness: caller may not change this readiness")
	ErrNotFound   = errors.New("readiness: not found")
	ErrVersion    = errors.New("readiness: version conflict")
	ErrDisabled   = errors.New("readiness: orchestration is not enabled")
	// ErrTransition means the move is not one the state machine allows.
	ErrTransition = errors.New("readiness: transition is not permitted")
)

// allowedTransitions is the readiness state machine.
//
// READY is deliberately not a dead end: a vendor who declares ready and then
// discovers a missing item must be able to say so, because the alternative is
// that they say nothing and a rider is sent to collect an incomplete order.
// What the machine does NOT allow is a silent correction -- see ValidateMove,
// which requires a privileged source and a reason to leave READY.
var allowedTransitions = map[State]map[State]bool{
	StateNotStarted: {StateAccepted: true, StateCancelled: true, StateBlocked: true},
	StateAccepted:   {StatePreparing: true, StateBlocked: true, StateDelayed: true, StateCancelled: true},
	StatePreparing:  {StateReadySoon: true, StateReady: true, StateBlocked: true, StateDelayed: true, StateCancelled: true},
	StateReadySoon:  {StateReady: true, StateBlocked: true, StateDelayed: true, StateCancelled: true},
	StateReady:      {StateDelayed: true, StateBlocked: true, StateCancelled: true},
	// A blocked or delayed order resumes where it was, which is why both lead
	// back into the middle of the machine rather than to the start.
	StateBlocked:   {StateAccepted: true, StatePreparing: true, StateReadySoon: true, StateReady: true, StateDelayed: true, StateCancelled: true},
	StateDelayed:   {StatePreparing: true, StateReadySoon: true, StateReady: true, StateBlocked: true, StateCancelled: true},
	StateCancelled: {},
}

// CanTransition reports whether a state move is structurally allowed.
func CanTransition(from, to State) bool {
	if from == to {
		// A same-state update (a new estimate, a substate change) is always
		// structurally fine; its own validation happens elsewhere.
		return from.Valid()
	}
	next, known := allowedTransitions[from]
	return known && next[to]
}

// Move is one requested readiness change.
type Move struct {
	To         State
	Substate   Substate
	Source     Source
	ReasonCode string
	ActorRef   string
	// OriginEventRef is the Vendor-owned event a SYSTEM move came from. A
	// system transition with no origin is unattributable, and an
	// unattributable automatic state change is one nobody can audit.
	OriginEventRef string
	EstimatedAt    *time.Time
}

// ValidateMove checks a requested change against the machine and the rules
// that are not expressible as a transition table.
func ValidateMove(current State, currentSubstate Substate, vendorType VendorType, move Move) error {
	if !move.To.Valid() {
		return fmt.Errorf("%w: %q is not a readiness state", ErrValidation, move.To)
	}
	if !move.Source.Valid() {
		return fmt.Errorf("%w: %q is not a readiness source", ErrValidation, move.Source)
	}
	if !CanTransition(current, move.To) {
		return fmt.Errorf("%w: %s cannot move to %s", ErrTransition, current, move.To)
	}

	// Leaving READY is a correction, not a normal step. It requires a reason so
	// the record says why, and a privileged source so a stray automatic signal
	// cannot quietly un-ready an order a person declared ready.
	if current == StateReady && move.To != StateReady && move.To != StateCancelled {
		if strings.TrimSpace(move.ReasonCode) == "" {
			return fmt.Errorf("%w: correcting a READY order requires a reason", ErrValidation)
		}
		if move.Source == SourceSystem {
			return fmt.Errorf("%w: only vendor staff or an admin may correct a READY order", ErrForbidden)
		}
	}

	// A block or a delay without a reason is a state an operator cannot act on.
	if (move.To == StateBlocked || move.To == StateDelayed) && strings.TrimSpace(move.ReasonCode) == "" {
		return fmt.Errorf("%w: %s requires a reason", ErrValidation, move.To)
	}

	// A system transition must name the event it came from.
	if move.Source == SourceSystem && strings.TrimSpace(move.OriginEventRef) == "" {
		return fmt.Errorf("%w: a system transition must reference its origin event", ErrValidation)
	}

	if move.Substate != "" {
		if err := ValidateSubstate(vendorType, move.To, move.Substate); err != nil {
			return err
		}
	}
	_ = currentSubstate
	return nil
}

// Readiness is one vendor order's preparation state.
type Readiness struct {
	VendorOrderID string     `json:"vendor_order_id"`
	ParentOrderID string     `json:"parent_order_id"`
	BusinessID    string     `json:"business_id"`
	BranchID      string     `json:"branch_id"`
	VendorType    VendorType `json:"vendor_type"`

	State    State    `json:"state"`
	Substate Substate `json:"substate,omitempty"`
	Version  int64    `json:"readiness_version"`

	PrepStartedAt          *time.Time `json:"prep_started_at,omitempty"`
	PredictedReadyStart    *time.Time `json:"predicted_ready_start,omitempty"`
	PredictedReadyEnd      *time.Time `json:"predicted_ready_end,omitempty"`
	VendorEstimatedReadyAt *time.Time `json:"vendor_estimated_ready_at,omitempty"`
	ActualReadyAt          *time.Time `json:"actual_ready_at,omitempty"`
	FirstActualReadyAt     *time.Time `json:"first_actual_ready_at,omitempty"`

	ConfidenceScore float64        `json:"confidence_score"`
	ConfidenceBand  ConfidenceBand `json:"confidence_band"`

	Source       Source     `json:"source"`
	BlockReason  string     `json:"block_reason,omitempty"`
	DelayReason  string     `json:"delay_reason,omitempty"`
	LastSignalAt *time.Time `json:"last_signal_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Stale reports whether this readiness has gone quiet for longer than policy
// allows.
//
// A stale signal is not a failure. It is a reason to trust the prediction less,
// which is what the orchestrator does with it: widen the buffer rather than
// cancel the plan. An order nobody has reported on for twenty minutes may be
// perfectly fine and simply unattended.
func (r Readiness) Stale(now time.Time, threshold time.Duration) bool {
	if !r.State.Active() || threshold <= 0 {
		return false
	}
	last := r.LastSignalAt
	if last == nil {
		last = &r.UpdatedAt
	}
	return now.Sub(*last) > threshold
}

// HistoryEntry is one append-only readiness transition.
type HistoryEntry struct {
	ID             string     `json:"id"`
	VendorOrderID  string     `json:"vendor_order_id"`
	FromState      State      `json:"from_state"`
	ToState        State      `json:"to_state"`
	FromSubstate   Substate   `json:"from_substate,omitempty"`
	ToSubstate     Substate   `json:"to_substate,omitempty"`
	Source         Source     `json:"source"`
	EstimateBefore *time.Time `json:"estimate_before,omitempty"`
	EstimateAfter  *time.Time `json:"estimate_after,omitempty"`
	ActorRef       string     `json:"actor_ref,omitempty"`
	OriginEventRef string     `json:"origin_event_ref,omitempty"`
	ReasonCode     string     `json:"reason_code,omitempty"`
	Version        int64      `json:"readiness_version"`
	CreatedAt      time.Time  `json:"created_at"`
}

// BranchProfile is what a branch has historically taken.
type BranchProfile struct {
	BusinessID     string     `json:"business_id"`
	BranchID       string     `json:"branch_id"`
	VendorType     VendorType `json:"vendor_type"`
	ProfileVersion int64      `json:"profile_version"`
	BaselineP50    int        `json:"baseline_p50_seconds"`
	BaselineP80    int        `json:"baseline_p80_seconds"`
	BaselineP90    int        `json:"baseline_p90_seconds"`
	HandoverP50    int        `json:"handover_p50_seconds"`
	HandoverP90    int        `json:"handover_p90_seconds"`
	Confidence     float64    `json:"confidence"`
	SampleCount    int        `json:"sample_count"`
	DataAsOf       time.Time  `json:"data_as_of"`
}

// Fresh reports whether a profile is recent enough to use.
//
// A profile describing how a branch behaved six months ago is a worse input
// than the vendor-type default, because it is specific enough to be trusted and
// old enough to be wrong.
func (p BranchProfile) Fresh(now time.Time, maxAge time.Duration) bool {
	return p.SampleCount >= MinProfileSamples && now.Sub(p.DataAsOf) <= maxAge
}

// MinProfileSamples is how many observations a branch profile needs before it
// beats the vendor-type default. Below it, one unusual lunchtime would move the
// whole branch's baseline.
const MinProfileSamples = 20

// Rules are the operational bounds for one scope.
//
// There is deliberately no price, fee or currency here. Readiness is about
// time; a rule that could change what an order costs would make a kitchen's
// prep estimate a commercial term.
type Rules struct {
	ID                 string     `json:"id"`
	VendorType         VendorType `json:"vendor_type"`
	BusinessID         string     `json:"business_id,omitempty"`
	BranchID           string     `json:"branch_id,omitempty"`
	DefaultPrep        time.Duration
	ReadySoonThreshold time.Duration
	StaleSignal        time.Duration
	MinVendorEstimate  time.Duration
	MaxVendorEstimate  time.Duration
	Version            int64 `json:"version"`
}

// Specificity ranks how narrowly a rule is scoped, so the narrowest applicable
// rule wins without the caller having to sort by hand.
func (r Rules) Specificity() int {
	switch {
	case r.BranchID != "":
		return 2
	case r.BusinessID != "":
		return 1
	default:
		return 0
	}
}

// Applies reports whether a rule covers this order.
func (r Rules) Applies(vendorType VendorType, businessID, branchID string) bool {
	if r.VendorType != vendorType {
		return false
	}
	if r.BusinessID != "" && r.BusinessID != businessID {
		return false
	}
	if r.BranchID != "" && r.BranchID != branchID {
		return false
	}
	return true
}

// SelectRules picks the narrowest applicable rule.
//
// Returning the default when nothing matches is deliberate: a branch with no
// configured rule must still be plannable, and refusing to predict would take
// the whole branch offline for a missing configuration row.
func SelectRules(candidates []Rules, vendorType VendorType, businessID, branchID string) Rules {
	applicable := make([]Rules, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Applies(vendorType, businessID, branchID) {
			applicable = append(applicable, candidate)
		}
	}
	if len(applicable) == 0 {
		return DefaultRules(vendorType)
	}
	sort.SliceStable(applicable, func(i, j int) bool {
		if applicable[i].Specificity() != applicable[j].Specificity() {
			return applicable[i].Specificity() > applicable[j].Specificity()
		}
		return applicable[i].Version > applicable[j].Version
	})
	return applicable[0]
}

// DefaultRules are the fallback bounds per vendor type.
//
// The numbers differ because the work differs: a fast-food counter is minutes,
// a bakery's proving and cooling is not, and a wholesale load-out is measured
// against a loading bay rather than a stove.
func DefaultRules(vendorType VendorType) Rules {
	rules := Rules{
		VendorType:         vendorType,
		DefaultPrep:        20 * time.Minute,
		ReadySoonThreshold: 5 * time.Minute,
		StaleSignal:        15 * time.Minute,
		MinVendorEstimate:  time.Minute,
		MaxVendorEstimate:  8 * time.Hour,
		Version:            1,
	}
	switch vendorType {
	case VendorFastFood:
		rules.DefaultPrep = 10 * time.Minute
		rules.ReadySoonThreshold = 3 * time.Minute
	case VendorRestaurant:
		rules.DefaultPrep = 25 * time.Minute
	case VendorCloudKitchen:
		rules.DefaultPrep = 20 * time.Minute
	case VendorPastryBakery:
		rules.DefaultPrep = 45 * time.Minute
		rules.ReadySoonThreshold = 10 * time.Minute
		rules.StaleSignal = 30 * time.Minute
	case VendorShopGrocery:
		rules.DefaultPrep = 30 * time.Minute
		rules.ReadySoonThreshold = 8 * time.Minute
	case VendorMarket:
		rules.DefaultPrep = 35 * time.Minute
		rules.ReadySoonThreshold = 8 * time.Minute
	case VendorWholesale:
		rules.DefaultPrep = 90 * time.Minute
		rules.ReadySoonThreshold = 20 * time.Minute
		rules.StaleSignal = 45 * time.Minute
	case VendorCatering:
		rules.DefaultPrep = 3 * time.Hour
		rules.ReadySoonThreshold = 30 * time.Minute
		rules.StaleSignal = time.Hour
	case VendorManufacturer:
		rules.DefaultPrep = 4 * time.Hour
		rules.ReadySoonThreshold = 30 * time.Minute
		rules.StaleSignal = time.Hour
	}
	return rules
}

// ClampVendorEstimate bounds what a vendor may claim.
//
// An estimate of four seconds or four days is not an estimate. Accepting either
// would let one order drag a whole branch's planning with it, and the clamp is
// reported rather than silent so the vendor's own number stays visible in the
// history beside the one actually used.
func (r Rules) ClampVendorEstimate(prepStart time.Time, estimate time.Time) (time.Time, bool) {
	if estimate.IsZero() {
		return estimate, false
	}
	minimum := prepStart.Add(r.MinVendorEstimate)
	maximum := prepStart.Add(r.MaxVendorEstimate)
	switch {
	case estimate.Before(minimum):
		return minimum, true
	case estimate.After(maximum):
		return maximum, true
	default:
		return estimate, false
	}
}
