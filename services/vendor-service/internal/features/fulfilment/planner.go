package fulfilment

// The planning decision.
//
// Given validated Section 17B groups, produce an authoritative candidate plan:
// which authority serves each group, what each delivery actually costs, who
// funded it, and — for anything that cannot be served — an explicit reason.
//
// Three rules govern everything below:
//
//  1. VENDOR OWN DELIVERY IS INDEPENDENT. When it applies, no platform
//     mission is created and no platform rider is selected. The two paths do
//     not blend.
//
//  2. VENDOR SERVICE NEVER PRICES A PLATFORM DELIVERY. Every platform fee
//     comes from the owning authority's own quote, carried verbatim. There is
//     no fallback fee, no estimate and no default — an authority that cannot
//     be reached makes the group unserviceable, not cheaper.
//
//  3. NOTHING IS SILENTLY DROPPED. Every group is planned or refused with a
//     reason the customer can be shown.

import (
	"context"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	"cosmicforge/logistics/services/vendor-service/internal/platform/geo"
	"cosmicforge/logistics/shared/go/commercequote"
)

// GroupInput is one validated Section 17B group presented for planning.
//
// Every commercial value here is server-resolved checkout evidence. Nothing in
// it may come from a customer request body — that is what stops a customer
// overwriting a logistics fee or injecting another vendor's branch as a pickup.
type GroupInput struct {
	CheckoutGroupID string

	BusinessID string
	BranchID   string

	// OrderValueMinor is the group's revalidated merchandise total, used only
	// for the vendor's own minimum-order and free-delivery rules.
	OrderValueMinor int64

	Currency string

	Lines []LineLogistics
}

// Destination is the customer's snapshotted delivery address.
type Destination struct {
	Latitude  float64
	Longitude float64
	Address   string
}

// PlanRequest is one planning attempt.
type PlanRequest struct {
	CustomerID string

	CheckoutSessionID          string
	CheckoutValidationRevision int64

	Currency string

	Destination Destination

	Groups []GroupInput

	// CorrelationID ties every quote request to this checkout for operator
	// tracing. It carries no authority.
	CorrelationID string
}

// QuoteAuthority requests an authoritative quote from one logistics service.
//
// Implemented by an adapter over shared/go/commercequote in the composition
// root, so this package never learns a base URL or a secret.
type QuoteAuthority interface {
	// Available reports whether this deployment can reach the authority at
	// all. A disabled integration must never be silently treated as a refusal.
	Available() bool

	RequestQuote(
		ctx context.Context,
		request commercequote.Request,
	) (commercequote.Quote, error)
}

// VendorDeliveryAuthority resolves and evaluates a vendor's own delivery
// policy. Implemented by an adapter over internal/features/delivery.
type VendorDeliveryAuthority interface {
	// PlatformEnabled is VENDOR_OWN_DELIVERY_ENABLED.
	PlatformEnabled() bool

	ResolvePolicy(
		ctx context.Context,
		businessID, branchID string,
	) (delivery.Policy, bool, error)
}

// Options configures the planner.
type Options struct {
	Logistics    LogisticsAuthority
	Branches     BranchAuthority
	VendorOwn    VendorDeliveryAuthority
	Dispatch     QuoteAuthority
	Hauling      QuoteAuthority
	Capabilities []Capability

	// ConsolidationRadiusMeters is how close two vendor pickups must be before
	// one mission may cover both.
	//
	// It is deliberately small. Neither dispatch nor hauling models a
	// multi-pickup mission today, so consolidating two genuinely distant
	// pickups would produce a fee for a route nobody can drive. Co-located
	// vendors — two traders in one market, two kitchens in one food court —
	// are the real case this serves.
	ConsolidationRadiusMeters int

	// PlanTTL bounds a plan independently of its quotes. The effective expiry
	// is always the earlier of this and the soonest quote inside the plan.
	PlanTTL time.Duration

	Now func() time.Time
}

// DefaultConsolidationRadiusMeters is a tight co-location bound.
const DefaultConsolidationRadiusMeters = 150

// DefaultPlanTTL bounds a plan when nothing shorter applies.
const DefaultPlanTTL = 15 * time.Minute

// Planner produces candidate plans.
type Planner struct {
	logistics    LogisticsAuthority
	branches     BranchAuthority
	vendorOwn    VendorDeliveryAuthority
	dispatch     QuoteAuthority
	hauling      QuoteAuthority
	capabilities []Capability

	consolidationRadiusMeters int
	planTTL                   time.Duration

	now func() time.Time
}

// NewPlanner builds the planner. A missing branch or logistics authority is a
// wiring error rather than a runtime refusal, so it is reported at build time.
func NewPlanner(options Options) (*Planner, error) {
	if options.Branches == nil || options.Logistics == nil {
		return nil, ErrDependencyMissing
	}

	capabilities := options.Capabilities
	if len(capabilities) == 0 {
		capabilities = DefaultCapabilities()
	}

	radius := options.ConsolidationRadiusMeters
	if radius <= 0 {
		radius = DefaultConsolidationRadiusMeters
	}

	ttl := options.PlanTTL
	if ttl <= 0 {
		ttl = DefaultPlanTTL
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Planner{
		logistics:                 options.Logistics,
		branches:                  options.Branches,
		vendorOwn:                 options.VendorOwn,
		dispatch:                  options.Dispatch,
		hauling:                   options.Hauling,
		capabilities:              capabilities,
		consolidationRadiusMeters: radius,
		planTTL:                   ttl,
		now:                       now,
	}, nil
}

// availableServices reports which logistics authorities this deployment can
// actually use, so a class is never selected that nothing can execute.
func (p *Planner) availableServices() map[Service]bool {
	return map[Service]bool{
		ServiceDispatch: p.dispatch != nil && p.dispatch.Available(),
		ServiceHauling:  p.hauling != nil && p.hauling.Available(),
	}
}

func (p *Planner) authorityFor(service Service) QuoteAuthority {
	switch service {
	case ServiceDispatch:
		return p.dispatch
	case ServiceHauling:
		return p.hauling
	default:
		return nil
	}
}

// planned is one group's resolved outcome before consolidation.
type planned struct {
	group GroupInput

	requirement Requirement

	branch Branch

	authority Authority

	// vendorOwn carries the vendor's own decision when that authority applies.
	vendorOwn delivery.Decision

	selection Selection

	serviceable bool
	reasonCode  string
}

// Build produces a candidate plan.
//
// It returns a Plan whose status is PLANNED when every group can be delivered,
// and UNSERVICEABLE when at least one cannot — with that group's reason
// recorded. It returns an error only for a genuine fault.
func (p *Planner) Build(
	ctx context.Context,
	request PlanRequest,
) (Plan, []PlanEvent, error) {
	now := p.now().UTC()

	plan := Plan{
		CustomerID:                 request.CustomerID,
		CheckoutSessionID:          request.CheckoutSessionID,
		CheckoutValidationRevision: request.CheckoutValidationRevision,
		Status:                     StatusPlanning,
		Mode:                       ModeSingle,
		PlanRevision:               1,
		Currency:                   strings.ToUpper(strings.TrimSpace(request.Currency)),
		Version:                    1,
		ExpiresAt:                  now.Add(p.planTTL),
	}

	events := []PlanEvent{{Action: EventPlanningStarted}}

	if len(request.Groups) == 0 {
		plan.Status = StatusFailed
		plan.FailureCode = FailureCheckoutNotUsable

		return plan, append(events, PlanEvent{
			Action: EventFailed, Detail: FailureCheckoutNotUsable,
		}), nil
	}

	if !geo.ValidCoordinate(
		request.Destination.Latitude, request.Destination.Longitude,
	) {
		// Every group fails for the same reason, and each one still gets its
		// own row: a customer must see which vendors are affected, not a bare
		// plan-level error.
		return p.refuseAll(
			plan, request, ReasonNoDeliveryAddress, events,
		), events, nil
	}

	resolved := make([]planned, 0, len(request.Groups))

	for _, group := range request.Groups {
		outcome, groupEvents, err := p.resolveGroup(ctx, request, group)
		if err != nil {
			return Plan{}, nil, err
		}

		events = append(events, groupEvents...)
		resolved = append(resolved, outcome)
	}

	return p.assemble(ctx, plan, request, resolved, events, now)
}

// refuseAll records every group as unserviceable for one shared reason.
func (p *Planner) refuseAll(
	plan Plan,
	request PlanRequest,
	reason string,
	events []PlanEvent,
) Plan {
	for _, group := range request.Groups {
		plan.Groups = append(plan.Groups, PlanGroup{
			CheckoutGroupID: group.CheckoutGroupID,
			BusinessID:      group.BusinessID,
			BranchID:        group.BranchID,
			Serviceable:     false,
			ReasonCode:      reason,
			Authority:       AuthorityNone,
		})
	}

	plan.Status = StatusUnserviceable
	plan.FailureCode = FailureGroupUnserviceable

	return plan
}

// resolveGroup decides one group's authority and cost.
func (p *Planner) resolveGroup(
	ctx context.Context,
	request PlanRequest,
	group GroupInput,
) (planned, []PlanEvent, error) {
	outcome := planned{
		group:     group,
		authority: AuthorityNone,
	}

	var events []PlanEvent

	branch, found, err := p.branches.ResolveBranch(
		ctx, group.BusinessID, group.BranchID,
	)
	if err != nil {
		return planned{}, nil, err
	}

	// The pickup must belong to this group's own vendor. ResolveBranch is
	// scoped by business id, so a branch borrowed from another business simply
	// does not resolve — a vendor cannot inject another vendor's pickup.
	switch {
	case !found:
		outcome.reasonCode = ReasonBranchUnknown

	case !branch.Active():
		outcome.reasonCode = ReasonBranchInactive

	case branch.HasOperationalSetting && !branch.AcceptingNewOrders:
		outcome.reasonCode = ReasonBranchNotAcceptingOrders
	}

	if outcome.reasonCode != "" {
		return outcome, append(events, PlanEvent{
			Action:  EventGroupUnserviceable,
			GroupID: group.CheckoutGroupID,
			Detail:  outcome.reasonCode,
		}), nil
	}

	outcome.branch = branch

	profiles, err := p.logistics.ResolveLineProfiles(
		ctx, group.BusinessID, group.Lines,
	)
	if err != nil {
		return planned{}, nil, err
	}

	outcome.requirement = Aggregate(
		group.BusinessID, group.BranchID, group.Lines, profiles,
	)

	events = append(events, PlanEvent{
		Action:  EventRequirementSolved,
		GroupID: group.CheckoutGroupID,
		Detail:  string(outcome.requirement.Urgency),
	})

	// Vendor Own Delivery first. It is independent of platform logistics and,
	// when it applies, no platform authority is consulted at all.
	if decision, ok := p.tryVendorOwn(ctx, request, group, outcome); ok {
		outcome.authority = AuthorityVendorOwn
		outcome.vendorOwn = decision
		outcome.serviceable = true
		outcome.reasonCode = ReasonServiceable

		return outcome, append(events, PlanEvent{
			Action:  EventVendorOwnSelected,
			GroupID: group.CheckoutGroupID,
		}), nil
	}

	selection, err := SelectVehicle(
		outcome.requirement, p.capabilities, p.availableServices(),
	)
	if err != nil {
		outcome.reasonCode = selection.ReasonCode

		if outcome.reasonCode == "" {
			outcome.reasonCode = ReasonNoAuthorityAvailable
		}

		return outcome, append(events, PlanEvent{
			Action:  EventGroupUnserviceable,
			GroupID: group.CheckoutGroupID,
			Detail:  outcome.reasonCode,
		}), nil
	}

	outcome.selection = selection
	outcome.authority = AuthorityPlatform
	outcome.serviceable = true
	outcome.reasonCode = ReasonServiceable

	return outcome, events, nil
}

// tryVendorOwn evaluates the vendor's own delivery policy for a group.
//
// A refusal is not an error and not a plan failure: it simply means this group
// falls through to platform logistics, which is the whole point of having two
// independent authorities.
func (p *Planner) tryVendorOwn(
	ctx context.Context,
	request PlanRequest,
	group GroupInput,
	outcome planned,
) (delivery.Decision, bool) {
	if p.vendorOwn == nil || !p.vendorOwn.PlatformEnabled() {
		return delivery.Decision{}, false
	}

	policy, found, err := p.vendorOwn.ResolvePolicy(
		ctx, group.BusinessID, group.BranchID,
	)
	if err != nil || !found {
		return delivery.Decision{}, false
	}

	decision := delivery.Quote(delivery.QuoteRequest{
		PlatformEnabled: true,
		Policy:          policy,
		Area: delivery.ServiceArea{
			CenterLatitude:  outcome.branch.AreaCenterLatitude,
			CenterLongitude: outcome.branch.AreaCenterLongitude,
			RadiusMeters:    outcome.branch.AreaRadiusMeters,
		},
		HasArea:          outcome.branch.HasServiceArea,
		PickupLatitude:   outcome.branch.Latitude,
		PickupLongitude:  outcome.branch.Longitude,
		DropoffLatitude:  request.Destination.Latitude,
		DropoffLongitude: request.Destination.Longitude,
		OrderValueMinor:  group.OrderValueMinor,
		Currency:         group.Currency,
		TotalWeightGrams: outcome.requirement.TotalWeightGrams,
	})

	return decision, decision.Serviceable
}

// assemble turns resolved groups into missions, quotes the platform ones, and
// finalises the plan.
func (p *Planner) assemble(
	ctx context.Context,
	plan Plan,
	request PlanRequest,
	resolved []planned,
	events []PlanEvent,
	now time.Time,
) (Plan, []PlanEvent, error) {
	// Consolidation runs before quoting, so a consolidated mission is quoted
	// once for its real combined load rather than being quoted per group and
	// then discounted — which would be a fee nobody stood behind.
	clusters, clusterEvents := p.cluster(resolved)
	events = append(events, clusterEvents...)

	earliestQuoteExpiry := plan.ExpiresAt
	unserviceable := false

	for _, cluster := range clusters {
		if !cluster.serviceable() {
			for _, member := range cluster.members {
				plan.Groups = append(plan.Groups, refusedGroup(member))
			}

			unserviceable = true

			continue
		}

		mission, reason, quoteEvents, err := p.quoteCluster(
			ctx, plan, request, cluster, now,
		)

		events = append(events, quoteEvents...)

		if err != nil {
			return Plan{}, nil, err
		}

		if mission == nil {
			for _, member := range cluster.members {
				member.reasonCode = reason
				plan.Groups = append(plan.Groups, refusedGroup(member))
			}

			unserviceable = true

			continue
		}

		plan.Missions = append(plan.Missions, *mission)
		plan.Funding = plan.Funding.Add(mission.Funding)

		if mission.Quote.ExpiresAt != nil &&
			mission.Quote.ExpiresAt.Before(earliestQuoteExpiry) {
			earliestQuoteExpiry = *mission.Quote.ExpiresAt
		}

		shares := shareFunding(mission.Funding, cluster.members)

		for index, member := range cluster.members {
			plan.Groups = append(plan.Groups, PlanGroup{
				CheckoutGroupID: member.group.CheckoutGroupID,
				BusinessID:      member.group.BusinessID,
				BranchID:        member.group.BranchID,
				Serviceable:     true,
				ReasonCode:      ReasonServiceable,
				Authority:       mission.Authority,
				MissionID:       mission.ID,
				VehicleClass:    mission.VehicleClass,
				Requirement:     member.requirement,
				Funding:         shares[index],
			})
		}
	}

	// A plan may never outlive the quotes inside it: a fee stands only as long
	// as the authority that gave it says it does.
	plan.ExpiresAt = earliestQuoteExpiry

	sortPlan(&plan)

	if unserviceable {
		plan.Status = StatusUnserviceable
		plan.FailureCode = FailureGroupUnserviceable

		return plan, append(events, PlanEvent{
			Action: EventFailed, Detail: FailureGroupUnserviceable,
		}), nil
	}

	plan.Status = StatusPlanned
	plan.PlannedAt = &now

	if plan.Currency == "" && len(plan.Missions) > 0 {
		plan.Currency = plan.Missions[0].Currency
	}

	return plan, append(events, PlanEvent{Action: EventPlanned}), nil
}

func refusedGroup(member planned) PlanGroup {
	reason := member.reasonCode

	if reason == "" || reason == ReasonServiceable {
		reason = ReasonNoAuthorityAvailable
	}

	return PlanGroup{
		CheckoutGroupID: member.group.CheckoutGroupID,
		BusinessID:      member.group.BusinessID,
		BranchID:        member.group.BranchID,
		Serviceable:     false,
		ReasonCode:      reason,
		Authority:       AuthorityNone,
		Requirement:     member.requirement,
	}
}

// sortPlan makes the output deterministic. Plans are persisted, compared
// across replans and shown to customers, so map or slice ordering must not
// leak into them.
func sortPlan(plan *Plan) {
	sort.SliceStable(plan.Groups, func(i, j int) bool {
		return plan.Groups[i].CheckoutGroupID < plan.Groups[j].CheckoutGroupID
	})

	sort.SliceStable(plan.Missions, func(i, j int) bool {
		return plan.Missions[i].ID < plan.Missions[j].ID
	})
}

// shareFunding attributes a mission's fee across the groups it covers.
//
// The split is by weight where weight is known, and evenly otherwise. Whatever
// the basis, the shares are made to sum EXACTLY back to the mission total by
// giving the remainder to the last group — an attribution that lost a kobo
// would break the funding invariant the database enforces.
func shareFunding(total Funding, members []planned) []Funding {
	shares := make([]Funding, len(members))

	if len(members) == 0 {
		return shares
	}

	if len(members) == 1 {
		shares[0] = total

		return shares
	}

	weights := make([]int64, len(members))

	var totalWeight int64

	for index, member := range members {
		weights[index] = member.requirement.TotalWeightGrams
		totalWeight += weights[index]
	}

	if totalWeight <= 0 {
		for index := range weights {
			weights[index] = 1
		}

		totalWeight = int64(len(members))
	}

	apportion := func(amount int64) []int64 {
		parts := make([]int64, len(members))

		var assigned int64

		for index := 0; index < len(members)-1; index++ {
			parts[index] = amount * weights[index] / totalWeight
			assigned += parts[index]
		}

		parts[len(members)-1] = amount - assigned

		return parts
	}

	gross := apportion(total.GrossMinor)
	customer := apportion(total.CustomerMinor)
	vendor := apportion(total.VendorFundedMinor)
	platform := apportion(total.PlatformFundedMinor)

	for index := range members {
		shares[index] = Funding{
			GrossMinor:          gross[index],
			CustomerMinor:       customer[index],
			VendorFundedMinor:   vendor[index],
			PlatformFundedMinor: platform[index],
		}
	}

	// Rounding each component independently can leave a share unbalanced even
	// though every component sums correctly. The last group absorbs the
	// difference so each row satisfies the invariant on its own.
	for index := range shares {
		if !shares[index].Balanced() {
			shares[index].CustomerMinor = shares[index].GrossMinor -
				shares[index].VendorFundedMinor -
				shares[index].PlatformFundedMinor
		}
	}

	return shares
}
