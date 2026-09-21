package fulfilment_readiness

import (
	"math"
	"time"
)

// Readiness prediction, v1: deterministic and explainable.
//
// There is no model here and that is the design, not a shortcut. A planner
// acting on this number decides when to send a rider, and when it is wrong
// somebody has to be able to say why. Every input below is a number a person
// can look up, and the same inputs always produce the same window.
//
// Accuracy is a second-order concern behind two properties that matter more:
// the prediction must exist for every order, and it must degrade honestly. An
// Analytics outage lowers confidence; it never blocks readiness, because a
// branch that cannot be predicted is still a branch that is cooking.

// Prediction is one forecast of when an order will be ready.
type Prediction struct {
	Start time.Time      `json:"predicted_ready_start"`
	End   time.Time      `json:"predicted_ready_end"`
	Score float64        `json:"confidence_score"`
	Band  ConfidenceBand `json:"confidence_band"`
	// Basis names what the estimate was built from, so an operator asking "why
	// does it say forty minutes" gets an answer rather than a number.
	Basis string `json:"basis"`
}

// Prediction bases, in descending order of how much they are trusted.
const (
	BasisVendorEstimate = "VENDOR_ESTIMATE"
	BasisBranchProfile  = "BRANCH_PROFILE"
	BasisTypeDefault    = "TYPE_DEFAULT"
)

// PredictionInput is everything the predictor may look at.
type PredictionInput struct {
	Now        time.Time
	VendorType VendorType
	Rules      Rules

	// Profile is the branch's own history. A zero value means none is
	// available, which is a normal state for a new branch and during an
	// Analytics outage.
	Profile      BranchProfile
	ProfileFresh bool

	// State and Substate locate the order in its own preparation.
	State    State
	Substate Substate

	PrepStartedAt          *time.Time
	VendorEstimatedReadyAt *time.Time
	LastSignalAt           *time.Time

	// QueueDepth is how many other orders the branch is working on. It comes
	// from Vendor's own production and capacity features, which already count
	// it; this does not recompute it.
	QueueDepth int

	// ItemLeadTime is the longest item-level lead time the order carries, when
	// the catalogue records one. A cake with a four-hour lead does not become a
	// twenty-minute order because the branch is quiet.
	ItemLeadTime time.Duration
}

// ProfileMaxAge is how old a branch profile may be and still be preferred over
// the vendor-type default.
//
// Fourteen days is chosen so a profile survives a quiet week but not a menu
// change, a kitchen refit or a new shift pattern -- the things that actually
// invalidate a baseline.
const ProfileMaxAge = 14 * 24 * time.Hour

// Predict computes the ready window and how much to trust it.
//
// The order of preference is deliberate. A vendor's own estimate wins because
// the person holding the pan knows more than any baseline; a branch's measured
// history comes next; the category default is the floor. What the floor
// guarantees is that every order has a window, which is what lets the
// orchestrator plan at all.
func Predict(input PredictionInput) Prediction {
	now := input.Now.UTC()
	rules := input.Rules
	if rules.DefaultPrep <= 0 {
		rules = DefaultRules(input.VendorType)
	}

	start := now
	if input.PrepStartedAt != nil && !input.PrepStartedAt.IsZero() {
		start = input.PrepStartedAt.UTC()
	}

	duration, spread, basis, score := baseline(input, rules)

	// Work already done shortens what remains. Progress is read from the
	// substate's position in its own sequence, which is why the sequences are
	// ordered rather than a set.
	if progress := SubstateProgress(input.VendorType, input.Substate); progress > 0 {
		duration = time.Duration(float64(duration) * (1 - progress*0.6))
	}

	// A queue is the branch working on other people's orders first. It is
	// additive rather than multiplicative because a kitchen does not take twice
	// as long per dish when it is busy; it starts later.
	if input.QueueDepth > 0 {
		duration += time.Duration(input.QueueDepth) * queueCostPerOrder(input.VendorType)
	}

	// An item's own lead time is a floor, never an average. A four-hour cake
	// cannot be produced in twenty minutes by a quiet branch.
	if input.ItemLeadTime > duration {
		duration = input.ItemLeadTime
		basis = BasisTypeDefault
	}

	// A waiting substate means the branch is blocked on somebody outside it.
	// The estimate cannot be tightened while that is true, and the confidence
	// in it drops: nobody knows when the customer will answer.
	if input.Substate.Waiting() {
		spread += 10 * time.Minute
		score *= 0.7
	}

	// A signal that has gone quiet does not change the estimate, only the trust
	// in it. Widening the window rather than moving it is the honest response:
	// the order is probably fine and nobody has said so.
	if stale, elapsed := staleness(input, rules, now); stale {
		score *= 0.6
		spread += elapsed / 2
	}

	// A delayed or blocked order has already proven the estimate optimistic.
	switch input.State {
	case StateDelayed:
		score *= 0.6
		spread += 5 * time.Minute
	case StateBlocked:
		score *= 0.4
		spread += 15 * time.Minute
	case StateReady:
		// Nothing left to predict: the window is the moment it was declared,
		// with full confidence, because this is an observation rather than a
		// forecast.
		return Prediction{Start: now, End: now, Score: 1, Band: ConfidenceHigh, Basis: basis}
	}

	predictedStart := start.Add(duration)
	if predictedStart.Before(now) {
		// The estimate has already elapsed. Reporting a window in the past
		// would let the orchestrator commit immediately against an order that
		// demonstrably is not ready.
		predictedStart = now
	}

	return Prediction{
		Start: predictedStart,
		End:   predictedStart.Add(spread),
		Score: clamp(score),
		Band:  BandFor(clamp(score)),
		Basis: basis,
	}
}

// baseline picks the duration, its spread, what it was based on, and the
// starting confidence.
func baseline(input PredictionInput, rules Rules) (time.Duration, time.Duration, string, float64) {
	// 1. The vendor's own estimate, bounded by policy.
	if input.VendorEstimatedReadyAt != nil && !input.VendorEstimatedReadyAt.IsZero() {
		start := input.Now
		if input.PrepStartedAt != nil && !input.PrepStartedAt.IsZero() {
			start = *input.PrepStartedAt
		}
		estimate, clamped := rules.ClampVendorEstimate(start.UTC(), input.VendorEstimatedReadyAt.UTC())
		duration := estimate.Sub(start.UTC())
		if duration < 0 {
			duration = 0
		}
		score := 0.85
		if clamped {
			// A clamped estimate is one the vendor got wrong by a wide margin.
			// Using it is still better than ignoring them, but not by as much.
			score = 0.55
		}
		return duration, spreadFor(duration, 0.25), BasisVendorEstimate, score
	}

	// 2. The branch's measured history, when it is recent and has enough
	// observations behind it.
	if input.ProfileFresh && input.Profile.BaselineP50 > 0 {
		duration := time.Duration(input.Profile.BaselineP50) * time.Second
		// P80 minus P50 is the branch's own observed variability, which is a
		// far better spread than any fixed percentage.
		spread := time.Duration(input.Profile.BaselineP80-input.Profile.BaselineP50) * time.Second
		if spread <= 0 {
			spread = spreadFor(duration, 0.2)
		}
		score := 0.5 + 0.3*input.Profile.Confidence
		return duration, spread, BasisBranchProfile, score
	}

	// 3. The category default. Always available, which is the point.
	return rules.DefaultPrep, spreadFor(rules.DefaultPrep, 0.35), BasisTypeDefault, 0.4
}

// staleness reports whether the order has gone quiet and by how much.
func staleness(input PredictionInput, rules Rules, now time.Time) (bool, time.Duration) {
	if !input.State.Active() || rules.StaleSignal <= 0 {
		return false, 0
	}
	last := input.LastSignalAt
	if last == nil || last.IsZero() {
		return false, 0
	}
	elapsed := now.Sub(last.UTC())
	if elapsed <= rules.StaleSignal {
		return false, 0
	}
	return true, elapsed - rules.StaleSignal
}

// queueCostPerOrder is how much each queued order delays this one.
//
// It differs by category because the constraint differs: a fast-food line is
// parallel and cheap per order, a bakery oven is not, and a wholesale load-out
// is one bay at a time.
func queueCostPerOrder(vendorType VendorType) time.Duration {
	switch vendorType {
	case VendorFastFood:
		return 45 * time.Second
	case VendorRestaurant, VendorCloudKitchen:
		return 90 * time.Second
	case VendorShopGrocery, VendorMarket:
		return 2 * time.Minute
	case VendorPastryBakery:
		return 5 * time.Minute
	case VendorWholesale, VendorCatering, VendorManufacturer:
		return 10 * time.Minute
	default:
		return 2 * time.Minute
	}
}

// spreadFor derives a window width from a duration.
//
// The window is never zero: an estimate presented as a single instant invites a
// planner to treat it as certainty, and no prep estimate is certain. A minimum
// of two minutes keeps even the shortest order honest.
func spreadFor(duration time.Duration, fraction float64) time.Duration {
	spread := time.Duration(float64(duration) * fraction)
	if spread < 2*time.Minute {
		spread = 2 * time.Minute
	}
	return spread
}

func clamp(score float64) float64 {
	if math.IsNaN(score) {
		return 0
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	// Three decimals, matching the column, so a stored value and a computed one
	// compare equal instead of differing in the eighth digit.
	return math.Round(score*1000) / 1000
}
