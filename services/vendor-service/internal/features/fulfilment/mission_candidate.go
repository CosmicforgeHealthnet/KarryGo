package fulfilment

// Building the alternatives.
//
// Two clusterings are constructed for every plan:
//
//   SPLIT        every platform group gets its own mission. Always
//                executable when the groups themselves are, which makes it
//                the fallback the whole design rests on while no logistics
//                runtime can drive a multi-stop collection.
//
//   CONSOLIDATED groups that may safely share a mission do. Whether that is
//                actually executable depends on whether the shared pickups
//                are one physical stop or several.
//
// BOTH are quoted before either is compared. That is the expensive choice and
// it is the correct one: a detour measured against an estimate is not
// evidence, and the section requires the comparison to be between two real
// quotes rather than between a quote and a guess. The quote cache below keeps
// the cost honest - a cluster that appears identically in both alternatives,
// which is most single-group clusters, is quoted exactly once.
//
// A clustering that merges nothing is not offered as a second alternative.
// Returning the same plan twice under two labels would be, in the section's
// own words, a lie told twice.

import (
	"context"
	"sort"
	"time"
)

// clustering is one complete way of covering every eligible group.
type clustering struct {
	kind     CandidateKind
	clusters []cluster
}

// quoteCache memoises one planning pass's authority answers.
//
// Keyed by the cluster's own quote reference, which is derived from the plan
// and the cluster's members - so the key is the QUESTION, not the caller.
// Two alternatives asking the same question get one answer and one call.
type quoteCache struct {
	entries map[string]quoteResult
}

type quoteResult struct {
	mission *Mission
	reason  string
	events  []PlanEvent
}

func newQuoteCache() *quoteCache {
	return &quoteCache{entries: map[string]quoteResult{}}
}

// quote asks the authority once per distinct cluster.
//
// The cached events are replayed only on the first call. A quote that was
// requested once must not appear in the plan's history as though it had been
// requested twice.
func (c *quoteCache) quote(
	ctx context.Context,
	planner *Planner,
	plan Plan,
	request PlanRequest,
	group cluster,
	now time.Time,
) (quoteResult, []PlanEvent, error) {
	key := quoteReference(plan, group)

	if cached, ok := c.entries[key]; ok {
		return cached, nil, nil
	}

	mission, reason, events, err := planner.quoteCluster(
		ctx, plan, request, group, now,
	)
	if err != nil {
		return quoteResult{}, nil, err
	}

	result := quoteResult{mission: mission, reason: reason, events: events}
	c.entries[key] = result

	return result, events, nil
}

// splitClustering gives every eligible group its own cluster.
func (p *Planner) splitClustering(eligible []planned) clustering {
	clusters := make([]cluster, 0, len(eligible))

	for _, outcome := range eligible {
		clusters = append(clusters, cluster{
			authority: outcome.authority,
			service:   outcome.selection.Service,
			class:     outcome.selection.Class,
			members:   []planned{outcome},
		})
	}

	return clustering{kind: CandidateSplitParallel, clusters: clusters}
}

// consolidatedClustering merges what may safely be merged under one mode.
//
// Vendor-own groups are never merged with anything. They are executed by the
// vendor's own fleet, which is not a shared platform mission and which the
// platform neither quotes nor routes.
//
// The refusals are returned rather than discarded: the FIRST reason a group
// could not join a cluster is the most useful thing Support can be told about
// why the order was split, and it is lost forever if the merge loop just
// returns false.
func (p *Planner) consolidatedClustering(
	eligible []planned,
	mode Mode,
) (clustering, []PlanEvent, []string) {
	var (
		clusters []cluster
		events   []PlanEvent
		refusals []string
	)

	seen := map[string]bool{}

	for _, outcome := range eligible {
		if outcome.authority != AuthorityPlatform {
			clusters = append(clusters, cluster{
				authority: outcome.authority,
				service:   outcome.selection.Service,
				class:     outcome.selection.Class,
				members:   []planned{outcome},
			})

			continue
		}

		merged := false

		var firstRefusal string

		for index := range clusters {
			check := p.canMerge(clusters[index], outcome, mode)

			if !check.ok {
				// Only the first real refusal is worth recording. The second
				// cluster refusing on SERVICE_MISMATCH says nothing about why
				// this group could not be consolidated at all.
				if firstRefusal == "" &&
					check.reason != ReasonCandidateServiceMismatch {
					firstRefusal = check.reason
				}

				continue
			}

			clusters[index].members = append(clusters[index].members, outcome)
			merged = true

			events = append(events, PlanEvent{
				Action:  EventConsolidated,
				GroupID: outcome.group.CheckoutGroupID,
				Detail:  clusters[index].members[0].group.CheckoutGroupID,
			})

			break
		}

		if merged {
			continue
		}

		if firstRefusal != "" && !seen[firstRefusal] {
			seen[firstRefusal] = true
			refusals = append(refusals, firstRefusal)
		}

		clusters = append(clusters, cluster{
			authority: AuthorityPlatform,
			service:   outcome.selection.Service,
			class:     outcome.selection.Class,
			members:   []planned{outcome},
		})
	}

	return clustering{kind: CandidateConsolidated, clusters: clusters},
		events, refusals
}

// merges reports whether a clustering actually puts two groups together.
func (c clustering) merges() bool {
	for _, group := range c.clusters {
		if len(group.members) > 1 {
			return true
		}
	}

	return false
}

// evaluateClustering quotes a clustering and turns it into a candidate.
//
// A candidate can fail here in exactly two ways, and they are different:
//
//   - A leg needs a multi-stop mission nobody can drive. The candidate is
//     INELIGIBLE, and it would become eligible the day the runtime supports
//     it.
//
//   - A leg could not be quoted. The candidate is INELIGIBLE because Vendor
//     Service will not invent a fee, and it might be quotable a minute later.
//
// Neither is a rejection on price, and neither is recorded as one.
func (p *Planner) evaluateClustering(
	ctx context.Context,
	plan Plan,
	request PlanRequest,
	target clustering,
	mode Mode,
	cache *quoteCache,
	splitBaselineMinutes int,
	now time.Time,
) (PlanCandidate, []PlanEvent, map[string]quoteResult, error) {
	capabilities := p.multiPickupCapabilities()

	candidate := PlanCandidate{
		Kind:                target.kind,
		Mode:                mode,
		Status:              StatusCandidateSelected,
		PolicyVersion:       p.optimization.Version,
		ServiceCapabilities: capabilities,
	}

	var (
		events   []PlanEvent
		quotes   = map[string]quoteResult{}
		hardCode string
	)

	policy := p.optimization.For(mode)

	for _, group := range target.clusters {
		leg := CandidateLeg{
			GroupIDs:  groupIDs(group),
			Authority: group.authority,
			Service:   group.service,
			VehicleClass: func() VehicleClass {
				if group.authority == AuthorityVendorOwn {
					return ""
				}

				return group.class
			}(),
			PickupStopCount: pickupStopCount(
				group, p.optimization.ColocationRadiusMeters,
			),
			ReadyAt: clusterReadyAt(group),
		}

		// The execution gate runs BEFORE the quote. Asking an authority to
		// price a route it cannot drive would spend a real call to obtain a
		// number that must be thrown away, and worse, would put that number
		// in the plan's history as though it meant something.
		if reason, ok := executableToday(leg, capabilities); !ok {
			candidate.Legs = append(candidate.Legs, leg)

			if hardCode == "" {
				hardCode = reason
			}

			continue
		}

		result, fresh, err := cache.quote(
			ctx, p, plan, request, group, now,
		)
		if err != nil {
			return PlanCandidate{}, nil, nil, err
		}

		events = append(events, fresh...)
		quotes[quoteReference(plan, group)] = result

		if result.mission == nil {
			candidate.Legs = append(candidate.Legs, leg)

			if hardCode == "" {
				hardCode = ReasonCandidateNotQuotable
			}

			continue
		}

		mission := result.mission

		leg.DistanceKm = mission.DistanceKm
		leg.EstimatedMinutes = mission.EstimatedMinutes
		leg.Currency = mission.Currency
		leg.Funding = mission.Funding
		leg.QuoteID = mission.Quote.QuoteID

		// The promise check needs the authority's own transit for the route
		// that would actually be driven, which only exists after the quote.
		if etaBreached(leg, group.members, policy.ETASlackTolerance) &&
			hardCode == "" {
			hardCode = ReasonCandidateETADetourExceeded
		}

		candidate.Legs = append(candidate.Legs, leg)
	}

	sort.SliceStable(candidate.Legs, func(i, j int) bool {
		return firstGroupID(candidate.Legs[i]) < firstGroupID(candidate.Legs[j])
	})

	candidate.Aggregate(splitBaselineMinutes)

	if hardCode != "" {
		candidate.Status = StatusCandidateIneligible
		candidate.ReasonCode = hardCode
	}

	return candidate, events, quotes, nil
}

func firstGroupID(leg CandidateLeg) string {
	if len(leg.GroupIDs) == 0 {
		return ""
	}

	return leg.GroupIDs[0]
}

// clusterReadyAt is when a cluster's mission could actually depart: the
// LATEST ready time among its members.
//
// A mission leaves when its last pickup is ready, not its first. Using the
// earliest would make every consolidation look as prompt as its fastest
// vendor, which is precisely the optimism that produces a plan the customer
// waits past.
func clusterReadyAt(group cluster) *time.Time {
	var (
		latest time.Time
		found  bool
	)

	for _, member := range group.members {
		from := member.group.Timing.ReadyFrom

		if from == nil {
			continue
		}

		at := from.UTC()

		if !found || at.After(latest) {
			latest = at
			found = true
		}
	}

	if !found {
		return nil
	}

	return &latest
}

// multiPickupCapabilities reads which authorities can drive a multi-stop
// collection right now.
//
// It is the ONLY place the answer is decided, and it is read rather than
// assumed. Today neither dispatch nor hauling models a multi-pickup mission,
// so both are false and every multi-stop candidate is refused with a reason.
// When a runtime gains the capability, this map changes and consolidated
// multi-stop candidates start passing the gate with no other edit anywhere.
func (p *Planner) multiPickupCapabilities() map[Service]bool {
	capabilities := map[Service]bool{
		ServiceDispatch: false,
		ServiceHauling:  false,
		ServiceTaxi:     false,
	}

	for service, supported := range p.multiPickup {
		capabilities[service] = supported
	}

	return capabilities
}

// slowestLeg is the split baseline the detour is measured against.
func slowestLeg(candidate PlanCandidate) int {
	slowest := 0

	for _, leg := range candidate.Legs {
		if leg.EstimatedMinutes > slowest {
			slowest = leg.EstimatedMinutes
		}
	}

	return slowest
}
