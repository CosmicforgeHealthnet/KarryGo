package fulfilment

// Consolidation and quoting.
//
// The section's instruction is exact: do NOT blindly charge one delivery fee
// per vendor, calculate the actual selected plan, and "only consolidate when
// real logistics constraints permit it".
//
// The real constraint today is that neither dispatch nor hauling models a
// multi-pickup mission. Consolidating two genuinely distant pickups would
// therefore produce a fee for a route nobody can drive — a fake fee wearing a
// consolidation label. So consolidation is permitted only between groups that
// share a pickup point within a tight radius (two traders in one market, two
// kitchens in one food court), and that are compatible in class, handling,
// urgency and combined capacity.
//
// When that holds, the cluster is quoted ONCE, for its real combined load,
// from the shared pickup. The fee is the authority's answer for the route that
// will actually be driven. A later section may split the quoted mission into
// several execution missions without changing that commitment.

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	"cosmicforge/logistics/shared/go/commercequote"
)

// cluster is a set of groups planned as one quoted mission.
type cluster struct {
	authority Authority
	service   Service
	class     VehicleClass

	members []planned
}

// combined aggregates a cluster's load. It is what the authority is asked to
// quote, so it must be the real total rather than the largest member.
func (c cluster) combined() Requirement {
	combined := Requirement{Urgency: UrgencyStandard}

	seen := map[HandlingCapability]bool{}

	for _, member := range c.members {
		requirement := member.requirement

		combined.PackageCount += requirement.PackageCount
		combined.TotalWeightGrams += requirement.TotalWeightGrams
		combined.TotalVolumeMl += requirement.TotalVolumeMl

		if requirement.LongestDimensionMm > combined.LongestDimensionMm {
			combined.LongestDimensionMm = requirement.LongestDimensionMm
		}

		if requirement.LargeLoad {
			combined.LargeLoad = true
		}

		if requirement.Urgency == UrgencyImmediate {
			combined.Urgency = UrgencyImmediate
		}

		// The weakest evidence wins: a combined total is only as trustworthy
		// as its least-measured part.
		combined.WeightEvidence = weakestEvidence(
			combined.WeightEvidence, requirement.WeightEvidence,
		)

		for _, capability := range requirement.Capabilities {
			seen[capability] = true
		}
	}

	for capability := range seen {
		combined.Capabilities = append(combined.Capabilities, capability)
	}

	sortCapabilities(combined.Capabilities)

	return combined
}

func weakestEvidence(current, next Evidence) Evidence {
	rank := func(evidence Evidence) int {
		switch evidence {
		case EvidenceComplete:
			return 3
		case EvidencePartial:
			return 2
		default:
			return 1
		}
	}

	if current == "" {
		return next
	}

	if rank(next) < rank(current) {
		return next
	}

	return current
}

func sortCapabilities(capabilities []HandlingCapability) {
	for i := 1; i < len(capabilities); i++ {
		for j := i; j > 0 && capabilities[j] < capabilities[j-1]; j-- {
			capabilities[j], capabilities[j-1] =
				capabilities[j-1], capabilities[j]
		}
	}
}

// The greedy cluster()/mergeable() pair that used to live here is gone.
//
// It made one grouping and executed it. Section 27 needs every grouping the
// planner could have made, each with a reason for its fate, so construction
// moved to mission_candidate.go and the merge rules moved to
// consolidation_feasibility.go where each refusal names a code. Leaving the
// old pair behind would have left two consolidation rules in one package,
// which is how they start disagreeing.
//
// What remains here is what quoting needs: how a cluster aggregates its load,
// and how a cluster becomes a mission.

func conflictingSeparation(existing cluster, candidate planned) bool {
	seen := map[string]bool{}

	for _, member := range existing.members {
		for _, group := range member.requirement.SeparationGroups {
			seen[group] = true
		}
	}

	for _, group := range candidate.requirement.SeparationGroups {
		if len(seen) > 0 && !seen[group] {
			return true
		}
	}

	// A cluster already carrying two separation groups is itself conflicted.
	return len(seen) > 1
}

// quoteCluster produces the mission for one cluster.
//
// A nil mission with no error means the cluster could not be served — the
// authority refused, or was unreachable. Neither case produces a fee.
// The refusal reason is RETURNED rather than written back into the cluster's
// members. Reaching into a shared slice to record a refusal happens to work
// because slices alias their backing array, and that is exactly why it should
// not be done: the next reader has no way to know the value changed.
func (p *Planner) quoteCluster(
	ctx context.Context,
	plan Plan,
	request PlanRequest,
	group cluster,
	now time.Time,
) (mission *Mission, reason string, events []PlanEvent, err error) {
	anchor := group.members[0]

	if group.authority == AuthorityVendorOwn {
		return p.vendorOwnMission(group, anchor), "", nil, nil
	}

	return p.platformMission(ctx, plan, request, group, anchor, now)
}

// vendorOwnMission builds a mission from the vendor's own decision.
//
// No platform mission is created, no rider is selected and no logistics
// authority is called. The fee is the vendor's own, and the vendor is the
// beneficiary — the only case in this platform where that is true.
func (p *Planner) vendorOwnMission(
	group cluster,
	anchor planned,
) *Mission {
	decision := anchor.vendorOwn

	return &Mission{
		ID:               "vfm_" + uuid.NewString(),
		Authority:        AuthorityVendorOwn,
		Service:          ServiceVendorOwn,
		PickupBusinessID: anchor.group.BusinessID,
		PickupBranchID:   anchor.group.BranchID,
		PickupLatitude:   anchor.branch.Latitude,
		PickupLongitude:  anchor.branch.Longitude,
		DistanceKm:       decision.DistanceKm,
		EstimatedMinutes: decision.EstimatedMinutes,
		Currency:         decision.Currency,
		Funding: Funding{
			GrossMinor:          decision.Funding.GrossFeeMinor,
			CustomerMinor:       decision.Funding.CustomerFeeMinor,
			VendorFundedMinor:   decision.Funding.VendorFundedMinor,
			PlatformFundedMinor: decision.Funding.PlatformFundedMinor,
		},
		FeeBeneficiary:              beneficiaryFor(AuthorityVendorOwn),
		DeliveryMode:                stringOrDefault(anchor.deliveryMode, "VENDOR"),
		FallbackTrigger:             anchor.fallbackTrigger,
		PolicyVersion:               anchor.policyVersion,
		CustomerPriceGuaranteeMinor: anchor.priceGuaranteeMinor,
		CustodyState:                "NOT_TRANSFERRED",
		TotalWeightGrams:            anchor.requirement.TotalWeightGrams,
		PackageCount:                anchor.requirement.PackageCount,
		GroupIDs:                    groupIDs(group),
	}
}

// platformMission asks the owning logistics authority for a real quote.
func (p *Planner) platformMission(
	ctx context.Context,
	plan Plan,
	request PlanRequest,
	group cluster,
	anchor planned,
	now time.Time,
) (*Mission, string, []PlanEvent, error) {
	authority := p.authorityFor(group.service)

	if authority == nil || !authority.Available() {
		// A disabled integration is not a refusal by the authority; it is this
		// deployment declining to offer the option at all.
		return nil, ReasonNoAuthorityAvailable, []PlanEvent{{
			Action:  EventQuoteUnavailable,
			GroupID: anchor.group.CheckoutGroupID,
			Detail:  string(group.service),
		}}, nil
	}

	combined := group.combined()

	quoteRequest := commercequote.Request{
		QuoteReference: quoteReference(plan, group),
		VehicleClass:   commercequote.VehicleClass(group.class),
		Pickup: commercequote.Point{
			Latitude:  anchor.branch.Latitude,
			Longitude: anchor.branch.Longitude,
		},
		Dropoff: commercequote.Point{
			Latitude:  request.Destination.Latitude,
			Longitude: request.Destination.Longitude,
			Address:   request.Destination.Address,
		},
		Load: commercequote.Load{
			PackageCount:       combined.PackageCount,
			TotalWeightGrams:   combined.TotalWeightGrams,
			WeightEvidence:     string(combined.WeightEvidence),
			TotalVolumeMl:      combined.TotalVolumeMl,
			LongestDimensionMm: combined.LongestDimensionMm,
			LargeLoad:          combined.LargeLoad,
			Capabilities:       capabilityStrings(combined.Capabilities),
		},
		Urgency:       commercequote.Urgency(combined.Urgency),
		Currency:      strings.ToUpper(strings.TrimSpace(request.Currency)),
		CorrelationID: request.CorrelationID,
	}
	if group.service == ServiceHauling && len(group.members) > 0 {
		quoteRequest.PickupStops = haulingQuotePickupStops(group)
	}

	events := []PlanEvent{{
		Action:  EventQuoteRequested,
		GroupID: anchor.group.CheckoutGroupID,
		Detail:  string(group.service),
	}}

	quote, err := authority.RequestQuote(ctx, quoteRequest)

	if err != nil {
		// An unreachable or misbehaving authority makes the group
		// unserviceable. It never makes the delivery cheaper, and it never
		// falls back to a Vendor Service estimate — there is no such estimate
		// to fall back to.
		return nil, ReasonAuthorityUnavailable, append(events, PlanEvent{
			Action:  EventQuoteUnavailable,
			GroupID: anchor.group.CheckoutGroupID,
			Detail:  err.Error(),
		}), nil
	}

	if !quote.Serviceable {
		return nil, quote.ReasonCode, append(events, PlanEvent{
			Action:  EventQuoteRefused,
			GroupID: anchor.group.CheckoutGroupID,
			Detail:  quote.ReasonCode,
		}), nil
	}

	// A quote that has already lapsed is not usable evidence. Treating it as
	// serviceable would put a fee on a plan the authority no longer stands
	// behind.
	if quote.Expired(now) {
		return nil, ReasonAuthorityUnavailable, append(events, PlanEvent{
			Action:  EventQuoteUnavailable,
			GroupID: anchor.group.CheckoutGroupID,
			Detail:  "quote already expired",
		}), nil
	}

	expiresAt := quote.ExpiresAt

	mission := &Mission{
		ID:               "pfm_" + uuid.NewString(),
		Authority:        AuthorityPlatform,
		Service:          group.service,
		VehicleClass:     group.class,
		PickupBusinessID: anchor.group.BusinessID,
		PickupBranchID:   anchor.group.BranchID,
		PickupLatitude:   anchor.branch.Latitude,
		PickupLongitude:  anchor.branch.Longitude,
		DropoffLatitude:  request.Destination.Latitude,
		DropoffLongitude: request.Destination.Longitude,
		Consolidated:     len(group.members) > 1,
		Quote: QuoteEvidence{
			QuoteID:        quote.QuoteID,
			QuoteReference: quote.QuoteReference,
			Revision:       quote.Revision,
			PickupRef:      quote.PickupRef,
			DropoffRef:     quote.DropoffRef,
			ExpiresAt:      &expiresAt,
		},
		DistanceKm:       quote.DistanceKm,
		EstimatedMinutes: quote.EstimatedMinutes,
		Currency:         quote.Currency,
		Funding:          hybridPlatformFunding(anchor, quote.FeeMinor),
		Breakdown: Breakdown{
			BaseMinor:      quote.Breakdown.BaseMinor,
			DistanceMinor:  quote.Breakdown.DistanceMinor,
			SurchargeMinor: quote.Breakdown.SurchargeMinor,
		},
		FeeBeneficiary:              beneficiaryFor(AuthorityPlatform),
		DeliveryMode:                stringOrDefault(anchor.deliveryMode, "PLATFORM"),
		FallbackTrigger:             anchor.fallbackTrigger,
		PolicyVersion:               anchor.policyVersion,
		CustomerPriceGuaranteeMinor: guaranteeOrFee(anchor.priceGuaranteeMinor, quote.FeeMinor),
		CustodyState:                "NOT_TRANSFERRED",
		TotalWeightGrams:            combined.TotalWeightGrams,
		PackageCount:                combined.PackageCount,
		GroupIDs:                    groupIDs(group),
	}

	return mission, "", append(events, PlanEvent{
		Action:    EventQuoteReceived,
		GroupID:   anchor.group.CheckoutGroupID,
		MissionID: mission.ID,
		Detail:    quote.QuoteID,
	}), nil
}

func haulingQuotePickupStops(group cluster) []commercequote.PickupStop {
	stops := make([]commercequote.PickupStop, 0, len(group.members))
	seen := map[string]bool{}
	for _, member := range group.members {
		key := strings.TrimSpace(member.group.BusinessID) + "|" + strings.TrimSpace(member.group.BranchID)
		if seen[key] {
			continue
		}
		seen[key] = true
		stops = append(stops, commercequote.PickupStop{
			StopRef:   key,
			VendorRef: member.group.BusinessID,
			BranchRef: member.group.BranchID,
			Location: commercequote.Point{
				Latitude:  member.branch.Latitude,
				Longitude: member.branch.Longitude,
				Address:   "Vendor branch " + member.group.BranchID,
			},
		})
	}
	return stops
}

func hybridPlatformFunding(anchor planned, grossMinor int64) Funding {
	funding := Funding{GrossMinor: grossMinor, CustomerMinor: grossMinor}
	if anchor.deliveryMode != string(delivery.ModeHybrid) || anchor.priceGuaranteeMinor < 0 || anchor.fallbackTrigger == "" {
		return funding
	}
	if anchor.priceGuaranteeMinor < funding.CustomerMinor {
		funding.CustomerMinor = anchor.priceGuaranteeMinor
	}
	gap := grossMinor - funding.CustomerMinor
	switch anchor.hybridFallbackFunding {
	case delivery.FallbackFundingPlatform:
		funding.PlatformFundedMinor = gap
	case delivery.FallbackFundingCombined:
		funding.VendorFundedMinor = gap * int64(anchor.hybridVendorShareBPS) / 10000
		funding.PlatformFundedMinor = gap - funding.VendorFundedMinor
	default:
		funding.VendorFundedMinor = gap
	}
	return funding
}

func stringOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func guaranteeOrFee(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

// quoteReference is the caller-owned idempotency key for one cluster's quote.
//
// It is derived from the plan and the cluster's own groups, so re-planning the
// same checkout asks the same question under the same reference and gets the
// same answer back instead of minting a second quote.
func quoteReference(plan Plan, group cluster) string {
	parts := make([]string, 0, len(group.members)+1)

	parts = append(parts, plan.CheckoutSessionID)

	for _, member := range group.members {
		parts = append(parts, member.group.CheckoutGroupID)
	}

	return "vfq_" + strings.Join(parts, "_")
}

func groupIDs(group cluster) []string {
	ids := make([]string, 0, len(group.members))

	for _, member := range group.members {
		ids = append(ids, member.group.CheckoutGroupID)
	}

	return ids
}

func capabilityStrings(capabilities []HandlingCapability) []string {
	if len(capabilities) == 0 {
		return nil
	}

	out := make([]string, 0, len(capabilities))

	for _, capability := range capabilities {
		out = append(out, string(capability))
	}

	return out
}

// PlanEvent is one append-only planning record.
type PlanEvent struct {
	Action    string
	GroupID   string
	MissionID string
	Detail    string
}
