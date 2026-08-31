package fulfilmentgroup

// Multi-pickup mission grouping.
//
// One platform mission may collect from several vendor branches and deliver to
// one receiver. That is a genuine optimisation — fewer vehicles, one fee — and
// it is also the easiest place in this whole aggregate to do real damage, so
// the rules here are deliberately conservative:
//
//   - A shared destination is NOT a reason to group. Two parcels going to the
//     same customer can still need two different vehicles.
//   - Grouping is refused whenever the constraint evidence is unknown. An
//     unstated weight is not a light parcel.
//   - An authority that cannot execute several stops causes a SPLIT, not a
//     mission it would have to fail after the customer is already waiting.
//
// Nothing here consolidates parcels. Parcels from different vendors travelling
// on one mission remain separate parcels with separate sources; only the
// VEHICLE is shared.

import (
	"sort"
	"strings"
)

// Capability describes what one logistics authority can actually execute.
//
// It is supplied by the composition root from each authority's own contract
// rather than assumed here, so this package never claims a capability a
// logistics service does not have.
type Capability struct {
	Authority Authority

	// MultiStopPickup reports whether the authority accepts more than one
	// pickup stop in a single mission. Where it is false, compatible units are
	// still planned onto separate missions rather than merged into one the
	// authority would reject.
	MultiStopPickup bool

	// MaxPickupStops bounds a multi-stop mission. Zero means the authority
	// stated no bound.
	MaxPickupStops int

	// MaxWeightGrams is the physical envelope of the vehicle class. Zero means
	// unstated, which is treated as unknown rather than unlimited.
	MaxWeightGrams int64

	// MaxPackageCount bounds the load. Zero means unstated.
	MaxPackageCount int64
}

// Serves reports whether the capability describes the given authority.
func (c Capability) Serves(authority Authority) bool {
	return c.Authority == authority && authority.ActiveForCommerce()
}

// GroupingRefusal names why two units were not put on one mission.
//
// A refusal is normal planning output, not an error: it is the reason the plan
// contains two missions instead of one, and an operator asking "why did this
// order need two riders" deserves an answer.
type GroupingRefusal string

const (
	RefuseDifferentDestination GroupingRefusal = "DIFFERENT_RECEIVER_DESTINATION"
	RefuseDifferentAuthority   GroupingRefusal = "DIFFERENT_LOGISTICS_AUTHORITY"
	RefuseDifferentVehicle     GroupingRefusal = "INCOMPATIBLE_VEHICLE_CLASS"
	RefuseDifferentService     GroupingRefusal = "INCOMPATIBLE_SERVICE_CLASS"
	RefuseAuthorityNoMultiStop GroupingRefusal = "AUTHORITY_DOES_NOT_SUPPORT_MULTI_STOP"
	RefuseStopLimit            GroupingRefusal = "PICKUP_STOP_LIMIT_REACHED"
	RefuseWeightCapacity       GroupingRefusal = "COMBINED_LOAD_EXCEEDS_CAPACITY"
	RefusePackageCapacity      GroupingRefusal = "COMBINED_PACKAGE_COUNT_EXCEEDS_CAPACITY"
	RefuseHandlingConflict     GroupingRefusal = "INCOMPATIBLE_HANDLING_REQUIREMENTS"
	RefuseVendorOwned          GroupingRefusal = "VENDOR_OWNED_DELIVERY_NOT_CONSOLIDATED"
	RefuseUnknownConstraint    GroupingRefusal = "CONSTRAINT_EVIDENCE_UNKNOWN"
)

// Candidate is one unit offered to the grouping decision.
type Candidate struct {
	Unit Unit

	// Owner and Authority are the executor already selected for this unit by
	// the delivery-mode policy. Grouping never changes them; it only decides
	// whether two units that already agree can share a vehicle.
	Owner     Owner
	Authority Authority

	// DestinationRef identifies the receiver destination. Units with different
	// destinations are never grouped: this section plans one commerce order's
	// fulfilment, not a multi-customer round.
	DestinationRef string

	ParcelIDs []string

	// WeightKnown is explicit. A unit whose weight was never stated is
	// reported as unknown rather than as weighing nothing, because a mission
	// planned on a guessed weight fails at the roadside.
	WeightKnown bool
}

// Compatible reports whether two candidates may share one platform mission.
//
// The order of the checks is the order of severity: the ones that would put
// the wrong vehicle at the kerb come before the ones that merely make the
// mission bigger than allowed.
func Compatible(a, b Candidate, capability Capability) (GroupingRefusal, bool) {
	// Vendor-owned delivery is the vendor's own vehicle and its own rider.
	// Merging two vendors' own deliveries onto one mission would put one
	// vendor's goods in another vendor's van.
	if a.Owner == OwnerVendor || b.Owner == OwnerVendor {
		return RefuseVendorOwned, false
	}
	if a.Authority != b.Authority {
		return RefuseDifferentAuthority, false
	}
	if !capability.Serves(a.Authority) {
		return RefuseDifferentAuthority, false
	}
	// A shared destination is necessary but never sufficient — this check
	// removes the pairs that must not be grouped, it does not authorise the
	// rest.
	if a.DestinationRef == "" || a.DestinationRef != b.DestinationRef {
		return RefuseDifferentDestination, false
	}
	if a.Unit.VehicleClass != b.Unit.VehicleClass {
		// A motorcycle mission and a truck-required cargo are not one mission
		// at any load. This is the check that stops a truck consignment being
		// silently put on a bike.
		return RefuseDifferentVehicle, false
	}
	if a.Unit.ServiceClass != b.Unit.ServiceClass {
		// Different service classes are different SLAs. Grouping them to save
		// a mission makes the faster one late.
		return RefuseDifferentService, false
	}
	if !capability.MultiStopPickup {
		return RefuseAuthorityNoMultiStop, false
	}
	if !handlingCompatible(a.Unit.HandlingRequirements, b.Unit.HandlingRequirements) {
		return RefuseHandlingConflict, false
	}
	if !a.WeightKnown || !b.WeightKnown {
		return RefuseUnknownConstraint, false
	}
	if capability.MaxWeightGrams > 0 &&
		a.Unit.RequiredWeightGrams+b.Unit.RequiredWeightGrams > capability.MaxWeightGrams {
		return RefuseWeightCapacity, false
	}
	if capability.MaxPackageCount > 0 &&
		a.Unit.RequiredPackageCount+b.Unit.RequiredPackageCount > capability.MaxPackageCount {
		return RefusePackageCapacity, false
	}
	return "", true
}

// handlingCompatible reports whether two handling requirement sets can travel
// together.
//
// Conservative on purpose: any requirement one side states and the other does
// not is treated as a conflict. Chilled goods and ambient goods in one
// insulated box is a food-safety decision, not a routing convenience, and this
// package is not the authority that may make it.
func handlingCompatible(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	set := func(values []string) map[string]bool {
		out := make(map[string]bool, len(values))
		for _, value := range values {
			if value != "" {
				out[value] = true
			}
		}
		return out
	}
	left, right := set(a), set(b)
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}

// MissionGroup is one set of candidates planned onto a single mission.
type MissionGroup struct {
	Authority Authority
	Owner     Owner

	Candidates []Candidate

	// Refusals records why other candidates were not merged in. Kept so a plan
	// can explain its own mission count instead of merely asserting it.
	Refusals map[string]GroupingRefusal
}

// TotalWeightGrams is the combined load of the group.
func (g MissionGroup) TotalWeightGrams() int64 {
	var total int64
	for _, candidate := range g.Candidates {
		total += candidate.Unit.RequiredWeightGrams
	}
	return total
}

// GroupCandidates partitions candidates into missions.
//
// Deterministic by construction: candidates are considered in a stable order
// and each is placed in the first existing group that will take it, so the same
// inputs always produce the same missions. A planner whose output depended on
// map iteration order would produce a different plan on every retry, and
// replan idempotency would be impossible to assert.
//
// Vendor-owned candidates are never merged with anything, which is why each of
// them becomes its own group.
func GroupCandidates(
	candidates []Candidate, capabilities map[Authority]Capability,
) []MissionGroup {
	ordered := append([]Candidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Unit.BusinessID != ordered[j].Unit.BusinessID {
			return ordered[i].Unit.BusinessID < ordered[j].Unit.BusinessID
		}
		if ordered[i].Unit.BranchID != ordered[j].Unit.BranchID {
			return ordered[i].Unit.BranchID < ordered[j].Unit.BranchID
		}
		return ordered[i].Unit.ID < ordered[j].Unit.ID
	})

	var groups []MissionGroup

	for _, candidate := range ordered {
		capability := capabilities[candidate.Authority]

		placed := false
		if candidate.Owner != OwnerVendor {
			for index := range groups {
				group := &groups[index]
				if group.Owner == OwnerVendor || group.Authority != candidate.Authority {
					continue
				}

				refusal, ok := groupAccepts(*group, candidate, capability)
				if !ok {
					if group.Refusals == nil {
						group.Refusals = map[string]GroupingRefusal{}
					}
					group.Refusals[candidate.Unit.ID] = refusal
					continue
				}

				group.Candidates = append(group.Candidates, candidate)
				placed = true
				break
			}
		}

		if !placed {
			groups = append(groups, MissionGroup{
				Authority:  candidate.Authority,
				Owner:      candidate.Owner,
				Candidates: []Candidate{candidate},
			})
		}
	}

	return groups
}

// groupAccepts reports whether a group will take one more candidate.
//
// Compatibility is checked against EVERY existing member, not only the first.
// Pairwise compatibility is not transitive once cumulative capacity is
// involved, and checking only the head is how an overloaded mission is built
// one safe-looking addition at a time.
func groupAccepts(
	group MissionGroup, candidate Candidate, capability Capability,
) (GroupingRefusal, bool) {
	if capability.MaxPickupStops > 0 && len(group.Candidates) >= capability.MaxPickupStops {
		return RefuseStopLimit, false
	}

	for _, member := range group.Candidates {
		if refusal, ok := Compatible(member, candidate, capability); !ok {
			return refusal, false
		}
	}

	// Cumulative load, which no pairwise check can see.
	if capability.MaxWeightGrams > 0 &&
		group.TotalWeightGrams()+candidate.Unit.RequiredWeightGrams > capability.MaxWeightGrams {
		return RefuseWeightCapacity, false
	}
	if capability.MaxPackageCount > 0 {
		var packages int64
		for _, member := range group.Candidates {
			packages += member.Unit.RequiredPackageCount
		}
		if packages+candidate.Unit.RequiredPackageCount > capability.MaxPackageCount {
			return RefusePackageCapacity, false
		}
	}

	return "", true
}

// BuildStops turns one mission group into a deterministic stop sequence.
//
// Pickups first in candidate order, then exactly one dropoff. The sequence is
// persisted with the plan because unordered database rows are not a route, and
// a rider needs to know which branch to visit first.
func BuildStops(group MissionGroup, destination Stop) []Stop {
	stops := make([]Stop, 0, len(group.Candidates)+1)

	for index, candidate := range group.Candidates {
		stops = append(stops, Stop{
			Sequence:   index + 1,
			Type:       StopPickup,
			UnitID:     candidate.Unit.ID,
			BusinessID: candidate.Unit.BusinessID,
			BranchID:   candidate.Unit.BranchID,
			Address:    candidate.Unit.PickupAddress,
			Latitude:   candidate.Unit.PickupLatitude,
			Longitude:  candidate.Unit.PickupLongitude,
		})
	}

	destination.Sequence = len(stops) + 1
	destination.Type = StopDropoff
	destination.UnitID = ""
	stops = append(stops, destination)

	return stops
}

// CollectParcelIDs returns every parcel riding one mission group, in a stable
// order.
//
// Parcels are collected, never merged: two vendors' parcels on one mission
// remain two parcels with two sources, and this function does nothing that
// would let them become one.
func CollectParcelIDs(group MissionGroup) []string {
	var parcels []string
	for _, candidate := range group.Candidates {
		parcels = append(parcels, candidate.ParcelIDs...)
	}
	sort.Strings(parcels)
	return parcels
}

// BuildPlanMissions turns candidates into the plan mission slots a replan will
// persist.
//
// It is the bridge between the compatibility rules above and the plan: group
// what may share a vehicle, lay out each group's stops in a deterministic
// order, and give every resulting mission a stable slot reference so mission
// creation stays idempotent across a crash.
//
// Cost is deliberately left at zero on every slot. What a replanned delivery
// costs is the executing authority's own answer under an existing quote, and
// inventing a figure here would be this service pricing a platform delivery.
func BuildPlanMissions(
	candidates []Candidate,
	capabilities map[Authority]Capability,
	destination Stop,
	currency string,
) []PlanMission {
	groups := GroupCandidates(candidates, capabilities)

	missions := make([]PlanMission, 0, len(groups))
	for index, group := range groups {
		if len(group.Candidates) == 0 {
			continue
		}
		if !group.Authority.ActiveForCommerce() {
			// An authority that cannot execute is skipped rather than planned
			// against. A plan naming it would be a delivery nobody performs.
			continue
		}

		missions = append(missions, PlanMission{
			// The slot reference is derived from the group's own membership,
			// so replanning the same units twice produces the same slot and
			// the authority sees one idempotency key rather than two.
			SlotRef:      slotRefFor(group),
			Sequence:     index + 1,
			Owner:        group.Owner,
			Authority:    group.Authority,
			ServiceClass: group.Candidates[0].Unit.ServiceClass,
			VehicleClass: group.Candidates[0].Unit.VehicleClass,
			Currency:     currency,
			Stops:        BuildStops(group, destination),
			ParcelIDs:    CollectParcelIDs(group),
		})
	}

	// Sequences must be contiguous from 1 even if a group was skipped, or the
	// plan's own uniqueness constraint on (plan_id, sequence) would hold while
	// the route order read oddly.
	for index := range missions {
		missions[index].Sequence = index + 1
	}
	return missions
}

// slotRefFor derives a stable reference from a mission group's membership.
func slotRefFor(group MissionGroup) string {
	units := make([]string, 0, len(group.Candidates))
	for _, candidate := range group.Candidates {
		units = append(units, candidate.Unit.ID)
	}
	sort.Strings(units)
	return strings.ToLower(string(group.Authority)) + ":" + strings.Join(units, "+")
}
