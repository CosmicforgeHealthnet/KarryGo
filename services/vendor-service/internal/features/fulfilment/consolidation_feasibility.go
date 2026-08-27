package fulfilment

// The mandatory-split rules.
//
// Section 27 states them as safety and service constraints, and it says one
// thing about them twice: optimization preference cannot override them. This
// file is where that becomes structural rather than a convention. Every
// function here answers a yes/no question and, when the answer is no, names
// the reason from the closed candidate vocabulary. None of them can see a
// price, a score or an objective, so none of them can be talked out of a
// refusal by a cheaper number.
//
// The order matters and is deliberate:
//
//  1. Physics first - capacity, temperature, fragility, separation. These do
//     not depend on anyone's preference and no mode may widen them.
//
//  2. Then service commitments - ready windows, promised arrival. These DO
//     depend on the mode, because a customer who accepted a Saver window has
//     genuinely agreed to wait longer. What they never agreed to is arriving
//     after a promise they were given, which is why the ETA tolerance is zero
//     in every mode.
//
//  3. Then runtime capability - can the owning service actually drive this
//     mission today. A candidate can be perfect on every other axis and still
//     be unexecutable, and saying so is not pessimism: submitting a
//     multi-stop mission to a runtime that models one pickup would produce a
//     fee for a route nobody can drive.

import (
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/platform/geo"
)

// FragileCoLoadWeightLimitGrams is how much OTHER load a fragile consignment
// may share a vehicle with.
//
// Fragility is not a weight problem on its own - a single crate of eggs is
// fine. It becomes one when the fragile load is consolidated underneath or
// alongside a heavy one, which is exactly the situation consolidation
// creates and a split plan avoids. Twenty kilograms is the point past which
// a courier is stacking rather than placing.
const FragileCoLoadWeightLimitGrams int64 = 20000

// mergeCheck is one candidate-construction decision.
type mergeCheck struct {
	ok     bool
	reason string
}

func mergeOK() mergeCheck { return mergeCheck{ok: true} }

func mergeRefused(reason string) mergeCheck {
	return mergeCheck{ok: false, reason: reason}
}

// canMerge decides whether one more group may join a cluster, and says why
// not when it cannot.
//
// It is the reasoned successor to the original boolean mergeable(): same
// rules, plus the ones Section 27 adds, and every refusal now carries a code
// a customer can be shown.
func (p *Planner) canMerge(
	existing cluster,
	candidate planned,
	mode Mode,
) mergeCheck {
	if existing.authority != AuthorityPlatform || len(existing.members) == 0 {
		return mergeRefused(ReasonCandidateServiceMismatch)
	}

	// Different services or classes are not one mission by definition. This
	// is a shape refusal rather than a safety one, but it is still absolute:
	// no objective can put a motorcycle load and a truck load on one vehicle.
	if existing.service != candidate.selection.Service ||
		existing.class != candidate.selection.Class {
		return mergeRefused(ReasonCandidateServiceMismatch)
	}

	anchor := existing.members[0]

	// --- Route proximity -------------------------------------------------
	//
	// Candidate CONSTRUCTION is bounded here, not by the co-location radius.
	// Pickups further apart than this are not operationally related, so
	// evaluating them as one mission would produce a candidate whose only
	// possible fate is rejection.
	if !geo.WithinRadiusMeters(
		anchor.branch.Latitude, anchor.branch.Longitude,
		candidate.branch.Latitude, candidate.branch.Longitude,
		p.optimization.ProximityRadiusMeters,
	) {
		return mergeRefused(ReasonCandidateProximityExceeded)
	}

	// --- Urgency ---------------------------------------------------------
	//
	// An immediate group must not be made to wait for a standard one, and a
	// standard one must not be dragged into express pricing.
	if anchor.requirement.Urgency != candidate.requirement.Urgency {
		return mergeRefused(ReasonCandidateUrgencyConflict)
	}

	// --- Vendor-declared separation --------------------------------------
	if conflictingSeparation(existing, candidate) {
		return mergeRefused(ReasonCandidateSeparationConflict)
	}

	trial := cluster{
		authority: existing.authority,
		service:   existing.service,
		class:     existing.class,
		members: append(
			append([]planned{}, existing.members...), candidate,
		),
	}

	combined := trial.combined()

	// --- Temperature -----------------------------------------------------
	//
	// Two groups each safe on their own can be unsafe together: a hot meal
	// and a frozen consignment are each ordinary work, and one compartment
	// holding both is not. This check runs on the COMBINED capabilities for
	// exactly that reason.
	if temperatureConflict(combined.Capabilities) {
		return mergeRefused(ReasonCandidateTemperatureConflict)
	}

	// --- Fragility -------------------------------------------------------
	if fragileUnsafe(trial) {
		return mergeRefused(ReasonCandidateFragileUnsafe)
	}

	// --- Capacity --------------------------------------------------------
	//
	// The combined load must still fit the class. Consolidating two loads
	// that only fit separately is how a mission gets quoted and then cannot
	// be filled.
	if !p.classFits(existing.class, combined) {
		return mergeRefused(ReasonCandidateCapacityExceeded)
	}

	// --- Ready windows ---------------------------------------------------
	if reason, ok := readyWindowCheck(
		trial, p.optimization.For(mode).ReadyGapTolerance,
	); !ok {
		return mergeRefused(reason)
	}

	return mergeOK()
}

// classFits reports whether a vehicle class can carry a combined load.
func (p *Planner) classFits(class VehicleClass, combined Requirement) bool {
	for _, capability := range p.capabilities {
		if capability.Class != class {
			continue
		}

		return capability.Fits(combined)
	}

	// A class with no capability entry cannot be shown to fit anything. It is
	// refused rather than assumed adequate: assuming would put a load on a
	// vehicle nobody has stated can carry it.
	return false
}

// temperatureConflict is the combined-load temperature rule.
//
// It states the same conflicting pairs as HandlingProfile.TemperatureConflict
// and reuses that function rather than restating the pairs, because two
// copies of a safety rule is one copy too many.
func temperatureConflict(capabilities []HandlingCapability) bool {
	return HandlingProfile{Capabilities: capabilities}.TemperatureConflict()
}

// fragileUnsafe reports that consolidating this cluster would put a fragile
// consignment under a load that should not be on top of it.
//
// The question is deliberately asymmetric. It is not "is this load heavy" -
// a heavy load alone is fine and a fragile load alone is fine. It is "would
// consolidation place OTHER weight alongside the fragile part", which is a
// question only consolidation can create and only a split can answer.
func fragileUnsafe(trial cluster) bool {
	var (
		fragileFound bool
		otherWeight  int64
	)

	for _, member := range trial.members {
		if member.requirement.HasCapability(CapabilityFragileCarry) {
			fragileFound = true

			continue
		}

		otherWeight += member.requirement.TotalWeightGrams
	}

	if !fragileFound {
		return false
	}

	return otherWeight > FragileCoLoadWeightLimitGrams
}

// readyWindowCheck is the ready-time overlap rule.
//
// Two facts decide it:
//
//   - The GAP between the earliest and latest ready time is how long the
//     first vendor's goods sit waiting. Beyond the mode's tolerance, one
//     vendor is being made to wait for another to save delivery cost, which
//     the section forbids as a matter of service and not of arithmetic.
//
//   - A vendor's own ReadyUntil is a HANDOVER DEADLINE. If the mission cannot
//     depart until after that vendor's window has closed, the consolidation
//     is not slow, it is impossible - no tolerance widens it.
//
// Groups that stated no timing at all impose no constraint. An SLA nobody
// stated cannot be missed, and inventing one would refuse ordinary work.
func readyWindowCheck(
	trial cluster,
	tolerance time.Duration,
) (string, bool) {
	var (
		earliest time.Time
		latest   time.Time
		found    bool
	)

	for _, member := range trial.members {
		from := member.group.Timing.ReadyFrom

		if from == nil {
			continue
		}

		at := from.UTC()

		if !found || at.Before(earliest) {
			earliest = at
		}

		if !found || at.After(latest) {
			latest = at
		}

		found = true
	}

	if !found {
		return "", true
	}

	if latest.Sub(earliest) > tolerance {
		return ReasonCandidateReadyWindowConflict, false
	}

	// The mission departs when its LAST pickup is ready. Any member whose
	// handover window closes before then cannot be collected on this mission
	// at all.
	for _, member := range trial.members {
		until := member.group.Timing.ReadyUntil

		if until == nil {
			continue
		}

		if until.UTC().Before(latest) {
			return ReasonCandidateReadyWindowConflict, false
		}
	}

	return "", true
}

// pickupStopCount is how many DISTINCT physical collection points a cluster
// needs.
//
// This is the single most consequential number in Section 27's execution
// gate, and it is not the member count. Several vendor obligations at one
// address - two kitchens in one food court, two traders in one market - are
// ONE stop. A cluster spread across a district is several, however few
// members it has.
//
// Counting members instead would make every consolidation look like a
// multi-stop mission and would destroy the co-located optimization that
// already works today. Counting one would make every consolidation look
// executable and would submit undrivable routes to a runtime that models a
// single pickup. Both errors are silent; this function is where neither is
// allowed to happen.
func pickupStopCount(group cluster, radiusMeters int) int {
	type point struct {
		latitude  float64
		longitude float64
	}

	var stops []point

	for _, member := range group.members {
		matched := false

		for _, stop := range stops {
			if geo.WithinRadiusMeters(
				stop.latitude, stop.longitude,
				member.branch.Latitude, member.branch.Longitude,
				radiusMeters,
			) {
				matched = true

				break
			}
		}

		if !matched {
			stops = append(stops, point{
				latitude:  member.branch.Latitude,
				longitude: member.branch.Longitude,
			})
		}
	}

	if len(stops) == 0 {
		return 1
	}

	return len(stops)
}

// executableToday reports whether the owning logistics runtime can actually
// drive a leg, and names the reason when it cannot.
//
// Today the answer for any multi-stop leg is no, for both dispatch and
// hauling. That is read from the capability snapshot rather than hardcoded,
// which is the whole point: when a runtime gains multi-pickup, the capability
// flips and this gate opens without a single line here changing.
func executableToday(
	leg CandidateLeg,
	capabilities map[Service]bool,
) (string, bool) {
	if !leg.MultiStop() {
		return "", true
	}

	// Vendor-own delivery is executed by the vendor's own fleet, which the
	// platform does not model and does not constrain. It is never merged with
	// anything, so a multi-stop vendor-own leg cannot arise; refusing it here
	// anyway would be a rule about a case that does not exist.
	if leg.Authority == AuthorityVendorOwn {
		return "", true
	}

	if capabilities[leg.Service] {
		return "", true
	}

	return ReasonCandidateMultiStopUnsupported, false
}

// etaBreached reports that a candidate's own quoted transit would miss a
// promise a member vendor's group was given.
//
// The evidence is the AUTHORITY'S quoted minutes for the leg that would
// actually be driven, not an estimate and not a straight-line guess. That is
// why both alternatives are quoted before either is compared: a detour
// measured against an invented baseline is not a detour, it is a preference
// dressed as a fact.
func etaBreached(
	leg CandidateLeg,
	members []planned,
	slack time.Duration,
) bool {
	if leg.ReadyAt == nil {
		// Without a stated departure there is no schedule to break. The
		// promise, if any, was made against timing the vendor never supplied.
		return false
	}

	arrival := leg.ReadyAt.UTC().
		Add(time.Duration(leg.EstimatedMinutes) * time.Minute)

	for _, member := range members {
		promised := member.group.Timing.PromisedBy

		if promised == nil {
			continue
		}

		if arrival.After(promised.UTC().Add(slack)) {
			return true
		}
	}

	return false
}
