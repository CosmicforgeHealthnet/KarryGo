package fulfilment

// Plan candidates: the alternatives the optimizer actually considered.
//
// Before this file the planner produced one grouping and one answer. That is
// enough to charge a customer and not nearly enough to explain the charge. A
// customer asking "why did you split my order into two deliveries" and a
// Support agent asking "why did FASTEST cost more" are asking about the
// alternative that LOST, and an answer can only be given if the losing
// alternative was written down at the time.
//
// So planning now builds every alternative it can, quotes each one, and keeps
// all of them with a status and a reason. The selected candidate is one row
// among several rather than the only row there is.
//
// Two ideas keep this honest:
//
//  1. INELIGIBLE is not REJECTED. A candidate that would break a temperature
//     rule or needs a multi-stop mission nobody can drive was never a real
//     option. Recording it as "rejected on price" would suggest the optimizer
//     weighed safety against money, which it must never do.
//
//  2. A candidate carries the POLICY VERSION it was decided under. Thresholds
//     move; a stored explanation that silently re-reads today's numbers stops
//     being an explanation of what happened.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CandidateKind is the shape of an alternative.
type CandidateKind string

const (
	// CandidateConsolidated covers several vendor groups with fewer missions
	// than there are groups.
	CandidateConsolidated CandidateKind = "CONSOLIDATED"

	// CandidateSplitParallel gives every platform group its own mission, run
	// in parallel. It is always executable when the groups themselves are, so
	// it is the fallback the whole design leans on while no logistics runtime
	// can drive a multi-stop collection.
	CandidateSplitParallel CandidateKind = "SPLIT_PARALLEL"
)

// CandidateStatus is what happened to an alternative.
type CandidateStatus string

const (
	// StatusCandidateSelected is the plan that was built.
	StatusCandidateSelected CandidateStatus = "SELECTED"

	// StatusCandidateRejected is a feasible alternative the objective did not
	// choose. It could have been executed; it simply lost on time or cost.
	StatusCandidateRejected CandidateStatus = "REJECTED"

	// StatusCandidateIneligible is an alternative that was never permissible.
	// A hard constraint refused it, and no objective could have rescued it.
	StatusCandidateIneligible CandidateStatus = "INELIGIBLE"
)

// Hard is whether a status means a constraint refused the candidate.
func (s CandidateStatus) Hard() bool { return s == StatusCandidateIneligible }

// Candidate reason codes.
//
// The set is closed for the same reasons the requirement vocabulary is: these
// are read by Support, counted by operations, and stored on evidence that has
// to keep its meaning. Codes are grouped by which question they answer.
const (
	// --- Selection outcomes ---------------------------------------------

	// ReasonCandidateChosen is recorded on the candidate that was selected.
	ReasonCandidateChosen = "SELECTED_BY_OBJECTIVE"

	// ReasonCandidateDominated means another candidate was at least as good
	// on every dimension the mode cares about and better on one.
	ReasonCandidateDominated = "DOMINATED_BY_SELECTED_CANDIDATE"

	// ReasonCandidateSlower / ReasonCandidateCostlier name the single
	// dimension that actually decided it, which is more useful to a customer
	// than "dominated".
	ReasonCandidateSlower   = "SLOWER_THAN_SELECTED_CANDIDATE"
	ReasonCandidateCostlier = "COSTLIER_THAN_SELECTED_CANDIDATE"

	// ReasonCandidateTieBreak means the two were equivalent under the mode
	// and a deterministic ordering picked one. Recording it prevents the
	// impression that a real trade-off was made.
	ReasonCandidateTieBreak = "EQUIVALENT_RESOLVED_BY_DETERMINISTIC_ORDER"

	// ReasonCustomerFastestParallel is the mandatory-split condition stated
	// by the section itself: the customer chose the faster parallel service
	// under a quote that covers it, so consolidation is not offered.
	ReasonCustomerFastestParallel = "CUSTOMER_FASTEST_PARALLEL_SELECTED"

	// --- Hard refusals ---------------------------------------------------

	// ReasonCandidateCapacityExceeded means the combined load does not fit
	// any single vehicle the platform will send.
	ReasonCandidateCapacityExceeded = "CAPACITY_EXCEEDED"

	// ReasonCandidateTemperatureConflict means the combined load carries
	// temperature requirements that cannot share one compartment. No larger
	// vehicle fixes it and no mode may override it.
	ReasonCandidateTemperatureConflict = "TEMPERATURE_CONFLICT"

	// ReasonCandidateFragileUnsafe means consolidation would put a fragile
	// load under a heavier one.
	ReasonCandidateFragileUnsafe = "FRAGILE_UNSAFE"

	// ReasonCandidateSeparationConflict means the vendors themselves declared
	// these items must not travel together.
	ReasonCandidateSeparationConflict = "SEPARATION_CONFLICT"

	// ReasonCandidateReadyWindowConflict means one vendor would have to wait
	// for another beyond the tolerance of the selected mode, or a vendor's
	// handover window would have closed before the other was ready.
	ReasonCandidateReadyWindowConflict = "READY_WINDOW_CONFLICT"

	// ReasonCandidateETADetourExceeded means the consolidated route's own
	// quoted transit breaks a delivery promise the customer was given.
	ReasonCandidateETADetourExceeded = "ETA_DETOUR_EXCEEDED"

	// ReasonCandidateMultiStopUnsupported is the live limitation: the
	// candidate needs a mission with several separate collection points and
	// the owning logistics service cannot drive one today.
	//
	// It is a RUNTIME capability statement. The candidate is physically fine
	// and would become eligible the moment the service supports it, which is
	// why the capability is read rather than hardcoded.
	ReasonCandidateMultiStopUnsupported = "MULTI_STOP_UNSUPPORTED"

	// ReasonCandidateServiceMismatch means the members need different
	// logistics services or vehicle classes, so no one mission covers them.
	ReasonCandidateServiceMismatch = "SERVICE_OR_CLASS_MISMATCH"

	// ReasonCandidateUrgencyConflict means an immediate group would be made
	// to travel with a standard one.
	ReasonCandidateUrgencyConflict = "URGENCY_CONFLICT"

	// ReasonCandidateNotQuotable means the owning authority refused or could
	// not be reached for this candidate. It is not a refusal of the plan -
	// the split alternative may still be quotable - but this candidate has no
	// fee anyone stands behind, and Vendor Service never invents one.
	ReasonCandidateNotQuotable = "CANDIDATE_NOT_QUOTABLE"

	// ReasonCandidateProximityExceeded means the pickups are not
	// operationally related enough to evaluate as one mission.
	ReasonCandidateProximityExceeded = "PICKUP_PROXIMITY_EXCEEDED"
)

// KnownCandidateReason reports whether a code is in the controlled candidate
// vocabulary.
//
// It exists so persistence and tests can assert that no free-text reason ever
// reaches a stored candidate. A code failing this check is a bug.
func KnownCandidateReason(code string) bool {
	switch code {
	case ReasonCandidateChosen, ReasonCandidateDominated,
		ReasonCandidateSlower, ReasonCandidateCostlier,
		ReasonCandidateTieBreak, ReasonCustomerFastestParallel,
		ReasonCandidateCapacityExceeded, ReasonCandidateTemperatureConflict,
		ReasonCandidateFragileUnsafe, ReasonCandidateSeparationConflict,
		ReasonCandidateReadyWindowConflict, ReasonCandidateETADetourExceeded,
		ReasonCandidateMultiStopUnsupported, ReasonCandidateServiceMismatch,
		ReasonCandidateUrgencyConflict, ReasonCandidateNotQuotable,
		ReasonCandidateProximityExceeded:
		return true
	default:
		return false
	}
}

// HardCandidateReason reports whether a reason is a constraint refusal rather
// than an objective outcome.
//
// This is the machine-checkable form of the rule that a cheap score may never
// beat a hard constraint: anything answering true here can only ever appear
// on an INELIGIBLE candidate, which persistence enforces.
func HardCandidateReason(code string) bool {
	switch code {
	case ReasonCandidateCapacityExceeded, ReasonCandidateTemperatureConflict,
		ReasonCandidateFragileUnsafe, ReasonCandidateSeparationConflict,
		ReasonCandidateReadyWindowConflict, ReasonCandidateETADetourExceeded,
		ReasonCandidateMultiStopUnsupported, ReasonCandidateServiceMismatch,
		ReasonCandidateUrgencyConflict, ReasonCandidateNotQuotable,
		ReasonCandidateProximityExceeded:
		return true
	default:
		return false
	}
}

// CandidateLeg is one mission inside a candidate.
//
// It is deliberately NOT a Mission: a leg belongs to an alternative that may
// never be executed, and giving it a mission identity would invite something
// downstream to treat a rejected alternative as an operational commitment.
type CandidateLeg struct {
	// GroupIDs are the checkout groups this leg would collect, in stable
	// order.
	GroupIDs []string `json:"group_ids"`

	Authority Authority `json:"authority"`
	Service   Service   `json:"service"`

	VehicleClass VehicleClass `json:"vehicle_class,omitempty"`

	// PickupStopCount is how many DISTINCT physical collection points this
	// leg needs. It is the number the multi-stop gate reads.
	//
	// Several vendor obligations at one address are one stop, not several.
	// Conflating the two is precisely how a plan gets marked executable
	// against a runtime that cannot execute it.
	PickupStopCount int `json:"pickup_stop_count"`

	DistanceKm       float64 `json:"distance_km"`
	EstimatedMinutes int     `json:"estimated_minutes"`

	// ReadyAt is when this leg could actually depart: the LATEST ready time
	// among its groups, because a mission leaves when its last pickup is
	// ready and not when its first one is.
	ReadyAt *time.Time `json:"ready_at,omitempty"`

	Currency string  `json:"currency,omitempty"`
	Funding  Funding `json:"funding"`

	// QuoteID is the authority's own evidence for this leg. Empty on a
	// vendor-own leg, which is quoted by the vendor's policy and not by any
	// logistics authority.
	QuoteID string `json:"quote_id,omitempty"`
}

// Consolidated reports whether this leg covers more than one vendor group.
func (l CandidateLeg) Consolidated() bool { return len(l.GroupIDs) > 1 }

// MultiStop reports whether this leg needs a mission with separate collection
// points.
func (l CandidateLeg) MultiStop() bool { return l.PickupStopCount > 1 }

// PlanCandidate is one complete alternative, with the evidence for its fate.
type PlanCandidate struct {
	ID string `json:"id"`

	Kind CandidateKind `json:"kind"`

	// Mode is the preference this candidate was evaluated for. A candidate
	// evaluated under SAVER is not evidence about what FASTEST would have
	// done, and storing the mode is what stops it being read that way.
	Mode Mode `json:"mode"`

	Status     CandidateStatus `json:"status"`
	ReasonCode string          `json:"reason_code"`

	// PolicyVersion is the optimization envelope in force when this candidate
	// was decided.
	PolicyVersion int64 `json:"policy_version"`

	Legs []CandidateLeg `json:"legs"`

	// MissionCount, VendorGroupCount and the totals are denormalised from the
	// legs so a stored candidate explains itself without the reader having to
	// re-aggregate it - and so a later change to aggregation cannot silently
	// restate a historical decision.
	MissionCount     int `json:"mission_count"`
	VendorGroupCount int `json:"vendor_group_count"`

	Currency string `json:"currency,omitempty"`

	// DeliveryGrossMinor is the true cost of the plan; DeliveryCustomerMinor
	// is the part the customer is asked for. They differ whenever a vendor or
	// the platform funds delivery, and comparing the wrong one is how a
	// subsidised plan gets picked as "cheaper for the customer" when it is
	// not.
	DeliveryGrossMinor    int64 `json:"delivery_gross_minor"`
	DeliveryCustomerMinor int64 `json:"delivery_customer_minor"`

	// CompletionMinutes is time to the LAST delivery, measured from the
	// earliest moment any of this candidate's goods could move.
	//
	// Parallel legs overlap, so a split plan's completion is its slowest leg
	// and not the sum of its legs. Summing would make parallel service look
	// worse than it is and would break FASTEST entirely.
	CompletionMinutes int `json:"completion_minutes"`

	// DetourMinutes is how much longer this candidate's slowest leg is than
	// the slowest leg of the fully split alternative. Zero on the split
	// alternative itself, which is the baseline.
	DetourMinutes int `json:"detour_minutes"`

	// MultiStopRequired records that at least one leg needs a multi-stop
	// mission, so the reason a consolidation was unavailable stays legible
	// after the runtime gains the capability.
	MultiStopRequired bool `json:"multi_stop_required"`

	// ServiceCapabilities is the runtime capability evidence this candidate
	// was judged against: which authorities could drive a multi-stop
	// collection at the moment of the decision.
	ServiceCapabilities map[Service]bool `json:"service_capabilities,omitempty"`

	// Fingerprint is the candidate's semantic identity. It deliberately
	// excludes timestamps, ids and quote references so re-planning identical
	// inputs produces the same fingerprint and duplicates can be detected.
	Fingerprint string `json:"fingerprint"`
}

// Feasible reports whether this candidate could have been executed.
func (c PlanCandidate) Feasible() bool { return !c.Status.Hard() }

// Consolidates reports whether the candidate actually merges anything.
//
// A "consolidated" candidate whose legs each carry one group merges nothing
// and is the split plan wearing a different label. Detecting that is what
// stops the optimizer offering the same plan twice under two names.
func (c PlanCandidate) Consolidates() bool {
	for _, leg := range c.Legs {
		if leg.Consolidated() {
			return true
		}
	}

	return false
}

// computeFingerprint derives the candidate's semantic identity.
//
// The inputs are the shape of the plan - which groups travel together, on
// what service, in what class, with how many stops - and nothing volatile.
// Two candidates with the same fingerprint are the same plan.
func (c *PlanCandidate) computeFingerprint() {
	parts := make([]string, 0, len(c.Legs)+2)

	parts = append(parts, string(c.Kind), string(c.Mode))

	legKeys := make([]string, 0, len(c.Legs))

	for _, leg := range c.Legs {
		ids := append([]string(nil), leg.GroupIDs...)
		sort.Strings(ids)

		legKeys = append(legKeys, strings.Join([]string{
			string(leg.Authority),
			string(leg.Service),
			string(leg.VehicleClass),
			strconv.Itoa(leg.PickupStopCount),
			strings.Join(ids, "+"),
		}, "|"))
	}

	// Leg order inside a candidate is an implementation detail of how the
	// clustering happened to iterate. Sorting makes the identity depend on
	// the plan rather than on that.
	sort.Strings(legKeys)

	parts = append(parts, legKeys...)

	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))

	c.Fingerprint = hex.EncodeToString(sum[:])
}

// Aggregate recomputes the denormalised totals from the legs.
//
// It is called once, when a candidate is built, and never again. Recomputing
// a stored candidate later would let today's arithmetic restate a historical
// decision.
func (c *PlanCandidate) Aggregate(splitBaselineMinutes int) {
	c.MissionCount = len(c.Legs)
	c.VendorGroupCount = 0
	c.DeliveryGrossMinor = 0
	c.DeliveryCustomerMinor = 0
	c.CompletionMinutes = 0
	c.MultiStopRequired = false

	groups := map[string]bool{}

	slowest := 0

	for _, leg := range c.Legs {
		for _, id := range leg.GroupIDs {
			groups[id] = true
		}

		c.DeliveryGrossMinor += leg.Funding.GrossMinor
		c.DeliveryCustomerMinor += leg.Funding.CustomerMinor

		if leg.MultiStop() {
			c.MultiStopRequired = true
		}

		if c.Currency == "" {
			c.Currency = leg.Currency
		}

		if leg.EstimatedMinutes > slowest {
			slowest = leg.EstimatedMinutes
		}
	}

	c.VendorGroupCount = len(groups)
	c.CompletionMinutes = c.completion()

	if splitBaselineMinutes > 0 && slowest > splitBaselineMinutes {
		c.DetourMinutes = slowest - splitBaselineMinutes
	} else {
		c.DetourMinutes = 0
	}

	c.computeFingerprint()
}

// completion is time from the earliest possible departure to the last
// delivery.
//
// Legs run in parallel, so the answer is the latest arrival among them. When
// no vendor stated a ready time the arithmetic degenerates to the slowest
// transit, which is the honest answer for a plan with no stated timing rather
// than a fabricated schedule.
func (c PlanCandidate) completion() int {
	var (
		earliest  time.Time
		haveReady bool
		latest    time.Time
		slowest   int
	)

	for _, leg := range c.Legs {
		if leg.EstimatedMinutes > slowest {
			slowest = leg.EstimatedMinutes
		}

		if leg.ReadyAt == nil {
			continue
		}

		ready := leg.ReadyAt.UTC()
		arrival := ready.Add(time.Duration(leg.EstimatedMinutes) * time.Minute)

		if !haveReady || ready.Before(earliest) {
			earliest = ready
		}

		if !haveReady || arrival.After(latest) {
			latest = arrival
		}

		haveReady = true
	}

	if !haveReady {
		return slowest
	}

	minutes := int(latest.Sub(earliest) / time.Minute)

	if minutes < slowest {
		// A stated schedule can never make a plan finish sooner than its own
		// slowest journey. If it appears to, the inputs disagree, and the
		// journey is the fact.
		return slowest
	}

	return minutes
}

// SortCandidates puts the selected candidate first and orders the rest
// deterministically, so a stored plan reads the same way every time.
func SortCandidates(candidates []PlanCandidate) {
	rank := func(status CandidateStatus) int {
		switch status {
		case StatusCandidateSelected:
			return 0
		case StatusCandidateRejected:
			return 1
		default:
			return 2
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if rank(candidates[i].Status) != rank(candidates[j].Status) {
			return rank(candidates[i].Status) < rank(candidates[j].Status)
		}

		if candidates[i].Kind != candidates[j].Kind {
			return candidates[i].Kind < candidates[j].Kind
		}

		return candidates[i].Fingerprint < candidates[j].Fingerprint
	})
}
