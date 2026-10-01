package orchestration

import (
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/geo"
)

// Multi-vendor grouping and splitting.
//
// Two vendors on one trip is cheaper for the platform and faster for nobody if
// it is done wrong. The failure mode that matters is not an inefficient route:
// it is a rider sent to collect from a bakery that will not be ready for forty
// minutes because a grocery two streets away was ready now.
//
// So grouping is conservative. A candidate joins a group only when every hard
// compatibility holds, and any one failure splits it out. What is never done is
// grouping first and repairing later: once custody starts, parcel identity,
// manifest history and custody ownership are fixed, and a reshuffle would
// rewrite a chain of evidence somebody may later need.

// GroupCandidate is one vendor order being considered for a shared trip.
type GroupCandidate struct {
	VendorOrderID string
	BusinessID    string
	BranchID      string

	// PredictedReadyStart is when this vendor expects to be ready.
	PredictedReadyStart time.Time
	PredictedReadyEnd   time.Time

	// Deadline is this order's own promised delivery time, when it has one.
	Deadline time.Time

	Latitude  float64
	Longitude float64

	WeightGrams  int64
	VolumeMl     int64
	PackageCount int64
	Handling     []string

	// TemperatureBand is the narrowest band this order needs. An ambient order
	// and a frozen one cannot share a compartment.
	TemperatureBand string

	SecurePickup bool
}

// GroupingPolicy is the operational envelope grouping must stay inside.
//
// Every value is a policy input rather than a constant so a market can be tuned
// without a deploy, and so a test can state the policy it is exercising instead
// of depending on whichever numbers happened to be compiled in.
type GroupingPolicy struct {
	// MaxReadinessSpread is how far apart two vendors' ready times may be. It
	// is the single most important limit: beyond it, one vendor's goods sit in
	// a vehicle while another finishes.
	MaxReadinessSpread time.Duration

	// MaxDetourKm is how far the route may stretch to add a pickup.
	MaxDetourKm float64

	// MaxPickups is the authority's own declared limit. It comes from the
	// authority's capability document, never from a constant here: a service
	// that has quietly become single-pickup must not keep receiving groups.
	MaxPickups int64

	// Capacity bounds, also the authority's own.
	MaxWeightGrams int64
	MaxVolumeMl    int64
	MaxPackages    int64
}

// DefaultGroupingPolicy is the conservative starting point.
func DefaultGroupingPolicy() GroupingPolicy {
	return GroupingPolicy{
		MaxReadinessSpread: 10 * time.Minute,
		MaxDetourKm:        4,
		MaxPickups:         1,
	}
}

// FromCapability fills the authority-declared limits from its capability
// document.
//
// An undeclared bound is left at zero and treated as UNKNOWN by the checks
// below, never as "unlimited". An authority that did not say how much it can
// carry has not said it can carry everything.
func (p GroupingPolicy) FromCapability(capability logistics.Capability) GroupingPolicy {
	p.MaxPickups = capability.MaxPickups
	if capability.MaxPickups <= 0 {
		// A capability that declares no pickup limit is treated as single
		// pickup. Grouping against an unknown limit is how a three-stop mission
		// reaches a service that only ever accepted one.
		p.MaxPickups = 1
	}
	p.MaxWeightGrams = capability.MaxWeightGrams
	p.MaxVolumeMl = capability.MaxVolumeMl
	p.MaxPackages = capability.MaxPackageCount
	return p
}

// SplitReason says why a candidate could not join a group.
type SplitReason string

const (
	SplitReadinessSpread SplitReason = "READINESS_SPREAD"
	SplitTemperature     SplitReason = "TEMPERATURE_INCOMPATIBLE"
	SplitHandling        SplitReason = "HANDLING_INCOMPATIBLE"
	SplitPickupLimit     SplitReason = "PICKUP_LIMIT"
	SplitCapacity        SplitReason = "CAPACITY_EXCEEDED"
	SplitDetour          SplitReason = "DETOUR_EXCEEDED"
	SplitDeadline        SplitReason = "DEADLINE_AT_RISK"
	SplitSecureHandover  SplitReason = "SECURE_HANDOVER_INCOMPATIBLE"
)

// Group is one set of vendor orders that will travel together.
type Group struct {
	Members []GroupCandidate

	// PickupWindowStart is the latest member's ready time: a vehicle cannot
	// collect the group before its slowest member is done.
	PickupWindowStart time.Time
	PickupWindowEnd   time.Time
}

// Split records a candidate that was left out and why.
type Split struct {
	Candidate GroupCandidate
	Reason    SplitReason
}

// Plan is the outcome of grouping.
type Plan struct {
	Groups []Group
	Splits []Split
}

// GroupCandidates partitions candidates into groups that may share a trip.
//
// The algorithm is deliberately simple and greedy rather than optimal. An
// optimal packing would be harder to explain when it surprises somebody, and
// the constraint that actually binds -- readiness spread -- makes most groups
// small anyway. Correctness here means never grouping two orders that should
// not travel together; efficiency is a distant second.
func GroupCandidates(candidates []GroupCandidate, policy GroupingPolicy) Plan {
	plan := Plan{Groups: []Group{}, Splits: []Split{}}
	if len(candidates) == 0 {
		return plan
	}

	// Earliest-ready first, so the anchor of each group is the member that
	// constrains it least.
	ordered := make([]GroupCandidate, len(candidates))
	copy(ordered, candidates)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].PredictedReadyStart.Before(ordered[j].PredictedReadyStart)
	})

	assigned := make(map[string]bool, len(ordered))
	// firstRefusal remembers why each candidate was turned away the FIRST time
	// it was considered. That first reason is the useful one: it is the group
	// the candidate would most naturally have joined, and reporting the last
	// refusal instead would blame whichever unrelated group happened to be
	// evaluated most recently.
	firstRefusal := make(map[string]SplitReason, len(ordered))

	for _, anchor := range ordered {
		if assigned[anchor.VendorOrderID] {
			continue
		}
		group := Group{Members: []GroupCandidate{anchor}}
		assigned[anchor.VendorOrderID] = true

		for _, candidate := range ordered {
			if assigned[candidate.VendorOrderID] {
				continue
			}
			reason, ok := canJoin(group, candidate, policy)
			if !ok {
				if _, already := firstRefusal[candidate.VendorOrderID]; !already {
					firstRefusal[candidate.VendorOrderID] = reason
				}
				continue
			}
			group.Members = append(group.Members, candidate)
			assigned[candidate.VendorOrderID] = true
			// Joining clears the record: this candidate is not a split.
			delete(firstRefusal, candidate.VendorOrderID)
		}

		group.PickupWindowStart, group.PickupWindowEnd = groupWindow(group)
		plan.Groups = append(plan.Groups, group)
	}

	// Every candidate that was refused by a group it was considered for, and
	// never joined one, is reported with the reason.
	//
	// The evidence matters as much as the partition. An operator asking why two
	// nearby orders did not share a trip needs an answer, and "there were no
	// multi-member groups to compare against" is not one -- which is exactly
	// what an earlier version of this loop produced when every group ended up
	// solo.
	for _, group := range plan.Groups {
		if len(group.Members) != 1 {
			continue
		}
		solo := group.Members[0]
		if reason, refused := firstRefusal[solo.VendorOrderID]; refused {
			plan.Splits = append(plan.Splits, Split{Candidate: solo, Reason: reason})
		}
	}
	sort.SliceStable(plan.Splits, func(i, j int) bool {
		return plan.Splits[i].Candidate.VendorOrderID < plan.Splits[j].Candidate.VendorOrderID
	})
	return plan
}

// canJoin applies every hard compatibility in turn.
//
// Order matters only for the reason reported, and the cheap physical checks
// come first so the most common refusal is also the clearest.
func canJoin(group Group, candidate GroupCandidate, policy GroupingPolicy) (SplitReason, bool) {
	if policy.MaxPickups > 0 && int64(len(group.Members))+1 > policy.MaxPickups {
		return SplitPickupLimit, false
	}

	anchor := group.Members[0]

	// Temperature. An ambient order and a frozen one cannot share a
	// compartment, and "close enough" is not a temperature band.
	if !temperatureCompatible(anchor.TemperatureBand, candidate.TemperatureBand) {
		return SplitTemperature, false
	}

	// Handling. Fragile-upright and stackable-heavy do not travel together.
	if !handlingCompatible(group, candidate) {
		return SplitHandling, false
	}

	// A protected pickup and an unprotected one can share a vehicle; what they
	// cannot share is a credential. Mixing them is allowed only when the whole
	// group is protected, so no unverified collection rides along inside a
	// verified one.
	if candidate.SecurePickup != anchor.SecurePickup {
		return SplitSecureHandover, false
	}

	// Readiness spread. The binding constraint: a group leaves when its
	// slowest member is ready, so a materially later vendor makes everyone wait.
	start, _ := groupWindow(group)
	spread := candidate.PredictedReadyStart.Sub(start)
	if spread < 0 {
		spread = -spread
	}
	if policy.MaxReadinessSpread > 0 && spread > policy.MaxReadinessSpread {
		return SplitReadinessSpread, false
	}

	// Capacity, against the authority's own declared bounds. A zero bound is
	// UNKNOWN and skipped rather than treated as zero capacity.
	weight, volume, packages := groupLoad(group)
	if policy.MaxWeightGrams > 0 && weight+candidate.WeightGrams > policy.MaxWeightGrams {
		return SplitCapacity, false
	}
	if policy.MaxVolumeMl > 0 && volume+candidate.VolumeMl > policy.MaxVolumeMl {
		return SplitCapacity, false
	}
	if policy.MaxPackages > 0 && packages+candidate.PackageCount > policy.MaxPackages {
		return SplitCapacity, false
	}

	// Detour. Distance from the group's existing pickups.
	if policy.MaxDetourKm > 0 {
		nearest := -1.0
		for _, member := range group.Members {
			distance := geo.HaversineKm(
				geo.Point{Lat: member.Latitude, Lng: member.Longitude},
				geo.Point{Lat: candidate.Latitude, Lng: candidate.Longitude})
			if nearest < 0 || distance < nearest {
				nearest = distance
			}
		}
		if nearest > policy.MaxDetourKm {
			return SplitDetour, false
		}
	}

	// A candidate whose own deadline would be missed by waiting for this group
	// must travel separately, however convenient the shared trip is.
	groupReady := latestReady(append(group.Members, candidate))
	if !candidate.Deadline.IsZero() && groupReady.After(candidate.Deadline) {
		return SplitDeadline, false
	}
	for _, member := range group.Members {
		if !member.Deadline.IsZero() && groupReady.After(member.Deadline) {
			return SplitDeadline, false
		}
	}

	return "", true
}

// temperatureCompatible reports whether two bands can share a compartment.
//
// An empty band means ambient, which is the only band that merges with itself.
// Anything else must match exactly: "chilled" and "frozen" are not close
// enough, and deciding they are is how a frozen order thaws.
func temperatureCompatible(left, right string) bool {
	return normalizeBand(left) == normalizeBand(right)
}

func normalizeBand(band string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(band))
	if trimmed == "" {
		return "AMBIENT"
	}
	return trimmed
}

// handlingCompatible reports whether the handling codes can coexist.
//
// The incompatibility is not between two codes in the abstract; it is that a
// group requiring upright carriage cannot also contain something that must be
// laid flat. Only genuinely opposed pairs are refused, so a fragile order and
// an ordinary one still share a trip.
func handlingCompatible(group Group, candidate GroupCandidate) bool {
	opposed := map[string]string{
		"UPRIGHT_CARRY": "LIE_FLAT",
		"LIE_FLAT":      "UPRIGHT_CARRY",
	}
	present := map[string]bool{}
	for _, member := range group.Members {
		for _, code := range member.Handling {
			present[strings.ToUpper(strings.TrimSpace(code))] = true
		}
	}
	for _, code := range candidate.Handling {
		normalized := strings.ToUpper(strings.TrimSpace(code))
		if conflict, has := opposed[normalized]; has && present[conflict] {
			return false
		}
	}
	return true
}

func groupLoad(group Group) (int64, int64, int64) {
	var weight, volume, packages int64
	for _, member := range group.Members {
		weight += member.WeightGrams
		volume += member.VolumeMl
		packages += member.PackageCount
	}
	return weight, volume, packages
}

// groupWindow is when the whole group can be collected.
//
// The start is the LATEST member's ready time, not the earliest: a vehicle
// cannot leave with goods that do not exist yet.
func groupWindow(group Group) (time.Time, time.Time) {
	if len(group.Members) == 0 {
		return time.Time{}, time.Time{}
	}
	start := group.Members[0].PredictedReadyStart
	end := group.Members[0].PredictedReadyEnd
	for _, member := range group.Members[1:] {
		if member.PredictedReadyStart.After(start) {
			start = member.PredictedReadyStart
		}
		if member.PredictedReadyEnd.After(end) {
			end = member.PredictedReadyEnd
		}
	}
	return start, end
}

func latestReady(members []GroupCandidate) time.Time {
	var latest time.Time
	for _, member := range members {
		if member.PredictedReadyStart.After(latest) {
			latest = member.PredictedReadyStart
		}
	}
	return latest
}

// CustodyStarted reports whether a group may still be reshuffled.
//
// Once any member's goods have been collected, the group is fixed. Parcel
// identity, manifest history and custody ownership are a chain of evidence, and
// re-cutting the group afterwards would rewrite a record somebody may need to
// prove who held what and when.
func CustodyStarted(state State) bool { return state.Committed() }
