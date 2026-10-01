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
	"encoding/json"
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

	// Timing is when this vendor can hand the load over and when the customer
	// was promised it, with the PLATFORM's transit estimate for the route.
	// Every field is optional: a group that states none is planned exactly as
	// before, because an SLA nobody stated cannot be missed.
	Timing TimingProfile

	// VendorEstimateMinutes is how long the VENDOR's own delivery would take.
	//
	// It is deliberately separate from Timing.EstimatedTransitMinutes. The two
	// are different journeys made by different fleets, and collapsing them
	// would make an SLA overflow impossible to express: the whole condition is
	// that the vendor is too slow for a promise the platform could still meet.
	VendorEstimateMinutes int

	// Value is the load's declared value and risk classification. It is risk
	// metadata that can exclude a carrier class; it never becomes a fee, a
	// ledger entry or an insurance charge.
	Value ValueProfile
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

	// Mode is the customer's fulfilment preference.
	//
	// It is the one value in this contract a customer may supply, and it is
	// safe precisely because it is not a number: it CHOOSES among candidates
	// the server built and the authorities priced. It can no more set a fee
	// than it can set a distance. Left empty it resolves to the default.
	Mode Mode

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
	Taxi         QuoteAuthority
	Hauling      QuoteAuthority
	Capabilities []Capability

	// CapabilityPolicy is the versioned vehicle envelope the requirement
	// engine decides against. Left zero, the platform default applies.
	//
	// It is a PLATFORM policy, never a vendor one: a vendor may state what
	// its own riders can carry, and may never widen what a platform
	// motorcycle is allowed to be asked to carry.
	CapabilityPolicy CapabilityPolicy

	// Optimization is the versioned envelope every candidate comparison is
	// decided under: per-mode ready and detour tolerances, materiality
	// thresholds, and the two pickup radii. Left zero, the platform default
	// applies.
	Optimization OptimizationPolicy

	// MultiPickup declares which logistics authorities can actually execute a
	// mission with several separate collection points.
	//
	// Absent means NOT supported, which is the honest default and today's
	// truth for both dispatch and hauling. It is wired as data rather than
	// hardcoded so a runtime that gains the capability opens the gate without
	// a code change in this package.
	MultiPickup map[Service]bool

	// CapabilityAuthorities read each logistics authority's live execution
	// envelope. An authority wired here overrides MultiPickup for its own
	// service, including when it refuses or cannot be reached: an unread
	// capability is never an assumed capability.
	CapabilityAuthorities map[Service]CapabilityAuthority

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
	taxi         QuoteAuthority
	hauling      QuoteAuthority
	capabilities []Capability
	policy       CapabilityPolicy

	consolidationRadiusMeters int
	planTTL                   time.Duration

	optimization OptimizationPolicy
	multiPickup  map[Service]bool

	capabilityAuthorities map[Service]CapabilityAuthority

	now func() time.Time
}

// NewPlanner builds the planner. A missing branch or logistics authority is a
// wiring error rather than a runtime refusal, so it is reported at build time.
func NewPlanner(options Options) (*Planner, error) {
	if options.Branches == nil || options.Logistics == nil {
		return nil, ErrDependencyMissing
	}

	policy := options.CapabilityPolicy
	if len(policy.Classes) == 0 {
		policy = DefaultCapabilityPolicy()
	}

	policy = policy.Normalized()

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

	optimization := options.Optimization
	// The two radii have a single source of truth. A deployment that
	// configured only the legacy consolidation radius must keep getting the
	// co-location bound it asked for, so it is carried into the optimization
	// envelope rather than quietly replaced by the default.
	if optimization.ColocationRadiusMeters <= 0 {
		optimization.ColocationRadiusMeters = radius
	}

	optimization = optimization.Normalized()

	multiPickup := make(map[Service]bool, len(options.MultiPickup))
	for service, supported := range options.MultiPickup {
		multiPickup[service] = supported
	}

	authorities := make(map[Service]CapabilityAuthority, len(options.CapabilityAuthorities))
	for service, authority := range options.CapabilityAuthorities {
		authorities[service] = authority
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
		taxi:                      options.Taxi,
		hauling:                   options.Hauling,
		capabilities:              capabilities,
		policy:                    policy,
		consolidationRadiusMeters: radius,
		planTTL:                   ttl,
		optimization:              optimization,
		multiPickup:               multiPickup,
		capabilityAuthorities:     authorities,
		now:                       now,
	}, nil
}

// availableServices reports which logistics authorities this deployment can
// actually use, so a class is never selected that nothing can execute.
func (p *Planner) availableServices() map[Service]bool {
	return map[Service]bool{
		ServiceDispatch: p.dispatch != nil && p.dispatch.Available(),
		ServiceTaxi:     p.taxi != nil && p.taxi.Available(),
		ServiceHauling:  p.hauling != nil && p.hauling.Available(),
	}
}

func (p *Planner) authorityFor(service Service) QuoteAuthority {
	switch service {
	case ServiceDispatch:
		return p.dispatch
	case ServiceTaxi:
		return p.taxi
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

	// snapshot is the immutable requirement evidence for this group. It is
	// produced before the delivery mode is decided, because the vendor's own
	// policy is evaluated against the class the load actually needs - a
	// question only the requirement engine can answer.
	snapshot *RequirementSnapshot

	serviceable           bool
	reasonCode            string
	fallbackTrigger       string
	deliveryMode          string
	policyVersion         int64
	priceGuaranteeMinor   int64
	policySnapshot        json.RawMessage
	hybridFallbackFunding delivery.FallbackFundingSponsor
	hybridVendorShareBPS  int
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

	mode := request.Mode
	if !mode.Selectable() {
		mode = DefaultMode
	}

	plan := Plan{
		CustomerID:                 request.CustomerID,
		CheckoutSessionID:          request.CheckoutSessionID,
		CheckoutValidationRevision: request.CheckoutValidationRevision,
		Status:                     StatusPlanning,
		Mode:                       mode,
		OptimizationPolicyVersion:  p.optimization.Version,
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

	// The requirement engine runs BEFORE the delivery mode is chosen.
	//
	// That order is not cosmetic. The vendor's own policy states which
	// vehicle classes its riders have, and evaluating that policy without
	// knowing which class the load needs is how VEHICLE_OVERFLOW silently
	// never fires and a vendor is handed a load its fleet cannot carry.
	snapshot, err := p.evaluateRequirement(group, outcome.requirement)
	if err != nil {
		return planned{}, nil, err
	}

	outcome.snapshot = &snapshot

	events = append(events, PlanEvent{
		Action:  EventRequirementSolved,
		GroupID: group.CheckoutGroupID,
		Detail:  string(outcome.requirement.Urgency),
	})

	// Vendor Own Delivery first. It is independent of platform logistics and,
	// when it applies, no platform authority is consulted at all.
	decision, evaluation, ok := p.tryVendorOwn(ctx, request, group, outcome)
	if ok {
		outcome.authority = AuthorityVendorOwn
		outcome.vendorOwn = decision
		outcome.deliveryMode = string(evaluation.Mode)
		outcome.policyVersion = evaluation.PolicyVersion
		outcome.priceGuaranteeMinor = evaluation.PriceGuarantee.CustomerFeeMinor
		outcome.policySnapshot = append([]byte(nil), evaluation.PolicySnapshot...)
		outcome.hybridFallbackFunding = evaluation.HybridFallbackFunding
		outcome.hybridVendorShareBPS = evaluation.HybridVendorShareBPS
		outcome.serviceable = true
		outcome.reasonCode = ReasonServiceable

		return outcome, append(events, PlanEvent{
			Action:  EventVendorOwnSelected,
			GroupID: group.CheckoutGroupID,
		}), nil
	}

	// A HYBRID vendor refusal is an explicit platform fallback, not a vendor
	// failure. Preserve its bounded reason on the platform group/mission.
	if evaluation.FallbackTrigger != "" {
		outcome.fallbackTrigger = string(evaluation.FallbackTrigger)
		outcome.deliveryMode = string(evaluation.Mode)
		outcome.policyVersion = evaluation.PolicyVersion
		outcome.priceGuaranteeMinor = evaluation.PriceGuarantee.CustomerFeeMinor
		outcome.policySnapshot = append([]byte(nil), evaluation.PolicySnapshot...)
		outcome.hybridFallbackFunding = evaluation.HybridFallbackFunding
		outcome.hybridVendorShareBPS = evaluation.HybridVendorShareBPS
	}
	if evaluation.Mode == delivery.ModeVendor && evaluation.Owner != delivery.OwnerVendor {
		outcome.reasonCode = evaluation.ReasonCode
		if outcome.reasonCode == "" {
			outcome.reasonCode = delivery.ReasonVendorUnavailable
		}
		return outcome, append(events, PlanEvent{
			Action: EventGroupUnserviceable, GroupID: group.CheckoutGroupID,
			Detail: outcome.reasonCode,
		}), nil
	}
	if evaluation.Mode == delivery.ModeHybrid && evaluation.Owner != delivery.OwnerPlatform && evaluation.FallbackTrigger == "" {
		outcome.reasonCode = evaluation.ReasonCode
		if outcome.reasonCode == "" {
			outcome.reasonCode = delivery.ReasonVendorUnavailable
		}
		return outcome, append(events, PlanEvent{
			Action: EventGroupUnserviceable, GroupID: group.CheckoutGroupID,
			Detail: outcome.reasonCode,
		}), nil
	}

	// Platform execution. The requirement engine has already decided; this
	// only reads its answer, so the class a group is planned with is always
	// the class its stored snapshot explains.
	if !snapshot.Decision.Selected() {
		outcome.reasonCode = snapshot.Decision.PrimaryReason

		if outcome.reasonCode == "" {
			outcome.reasonCode = ReasonNoAuthorityAvailable
		}

		return outcome, append(events, PlanEvent{
			Action:  EventGroupUnserviceable,
			GroupID: group.CheckoutGroupID,
			Detail:  outcome.reasonCode,
		}), nil
	}

	outcome.selection = snapshot.Decision.Selection()
	outcome.authority = AuthorityPlatform
	outcome.serviceable = true
	outcome.reasonCode = ReasonServiceable

	return outcome, events, nil
}

// evaluateRequirement runs the requirement and vehicle-selection engine for
// one group and returns the immutable evidence of its decision.
//
// Serviceability is assembled here rather than inside the engine, so the
// engine stays a pure function of its inputs and a stored snapshot replays to
// exactly the decision that was originally taken.
func (p *Planner) evaluateRequirement(
	group GroupInput,
	requirement Requirement,
) (RequirementSnapshot, error) {
	deliveryRequirement := RequirementFrom(requirement)
	deliveryRequirement.Timing = group.Timing
	deliveryRequirement.Value = group.Value

	// One pickup per mission. Consolidation may later put two co-located
	// vendors behind one collection point, but it never produces a route with
	// two separate stops, precisely because no logistics runtime can drive
	// one yet.
	deliveryRequirement.Route.PickupCount = 1

	return EvaluateAndSnapshot(SelectionRequest{
		Requirement:    deliveryRequirement,
		Policy:         p.policy,
		Serviceability: ServiceabilityFrom(p.availableServices(), p.now()),
		Now:            p.now(),
	})
}

// requiredVehicleClass is the class the vendor's own fleet would have to
// have, expressed for the delivery policy evaluation.
//
// It is the MINIMUM PHYSICAL class, not the selected executable one. A vendor
// with its own car should not be told its fleet is inadequate because the
// platform cannot run a taxi; the two questions are independent, and this is
// where keeping them separate pays for itself.
func requiredVehicleClass(snapshot *RequirementSnapshot) string {
	if snapshot == nil || !snapshot.Decision.MinimumPhysicalFound {
		return ""
	}

	return string(snapshot.Decision.MinimumPhysicalClass)
}

// promisedMaxMinutes is how long the delivery has left against its promise.
//
// Zero means no promise was stated, which the delivery evaluation reads as
// "no SLA to miss" rather than as "already late".
func promisedMaxMinutes(timing TimingProfile, now time.Time) int {
	if timing.PromisedBy == nil {
		return 0
	}

	remaining := timing.PromisedBy.UTC().Sub(now.UTC())
	if remaining <= 0 {
		// Already past the promise. One minute, not zero: zero would read as
		// "no promise stated" and silently skip the SLA check entirely.
		return 1
	}

	return int(remaining/time.Minute) + 1
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
) (delivery.Decision, delivery.Evaluation, bool) {
	if p.vendorOwn == nil || !p.vendorOwn.PlatformEnabled() {
		return delivery.Decision{}, delivery.Evaluation{}, false
	}

	policy, found, err := p.vendorOwn.ResolvePolicy(
		ctx, group.BusinessID, group.BranchID,
	)
	if err != nil || !found {
		return delivery.Decision{}, delivery.Evaluation{}, false
	}
	if policy.Mode != "" {
		originLat, originLng := outcome.branch.Latitude, outcome.branch.Longitude
		if outcome.branch.HasServiceArea {
			originLat, originLng = outcome.branch.AreaCenterLatitude, outcome.branch.AreaCenterLongitude
		}
		distance := int64(geo.HaversineKm(
			originLat, originLng,
			request.Destination.Latitude, request.Destination.Longitude,
		) * 1000)
		evaluation := delivery.Evaluate(delivery.EvaluationRequest{
			Policy: policy, OrderSubtotalMinor: group.OrderValueMinor,
			Currency: group.Currency, DistanceMeters: distance,
			Now: p.now(), PlatformEligible: true,
			PackageCount:            outcome.requirement.PackageCount,
			WeightGrams:             outcome.requirement.TotalWeightGrams,
			VolumeMl:                outcome.requirement.TotalVolumeMl,
			VendorRiderAvailability: policy.RiderAvailability,
			BeforeCustody:           true,

			// The requirement engine's answer, handed to the vendor policy so
			// VEHICLE_OVERFLOW and SLA_OVERFLOW can actually fire. Before
			// this, both fields were always empty and both triggers were
			// unreachable however the vendor configured its policy.
			RequiredVehicleClass:  requiredVehicleClass(outcome.snapshot),
			VendorEstimateMinutes: group.VendorEstimateMinutes,
			PromisedMaxMinutes:    promisedMaxMinutes(group.Timing, p.now()),
		})
		if evaluation.Owner != delivery.OwnerVendor || !evaluation.Eligible {
			return delivery.Decision{}, evaluation, false
		}
		return delivery.Decision{
			Serviceable: true, ReasonCode: delivery.ReasonServiceable,
			FeeBeneficiary: delivery.BeneficiaryVendor, Currency: evaluation.Currency,
			Funding:    delivery.Funding{GrossFeeMinor: evaluation.FeeMinor, CustomerFeeMinor: evaluation.PriceGuarantee.CustomerFeeMinor, VendorFundedMinor: evaluation.FeeMinor - evaluation.PriceGuarantee.CustomerFeeMinor},
			DistanceKm: float64(distance) / 1000, PolicyVersion: evaluation.PolicyVersion,
		}, evaluation, true
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

	return decision, delivery.Evaluation{Mode: delivery.ModeVendor, Owner: delivery.OwnerVendor, Eligible: decision.Serviceable, PolicyVersion: decision.PolicyVersion, PriceGuarantee: delivery.PriceGuarantee{Currency: decision.Currency, CustomerFeeMinor: decision.Funding.CustomerFeeMinor}}, decision.Serviceable
}

// assemble turns resolved groups into missions, quotes the platform ones, and
// finalises the plan.
//
// The shape of the work changed with Section 27 and the reason is worth
// stating. Before, one clustering was produced and executed. Now every
// alternative is built, quoted and compared, the winner is executed, and the
// losers are kept with the reason they lost. The executed plan is identical
// in structure to what it always was - the same missions, the same funding,
// the same groups - because the alternatives decide WHICH clustering runs,
// not what a running clustering looks like.
func (p *Planner) assemble(
	ctx context.Context,
	plan Plan,
	request PlanRequest,
	resolved []planned,
	events []PlanEvent,
	now time.Time,
) (Plan, []PlanEvent, error) {
	var (
		eligible []planned
		refused  []planned
	)

	for _, outcome := range resolved {
		if outcome.serviceable {
			eligible = append(eligible, outcome)

			continue
		}

		refused = append(refused, outcome)
	}

	selected, candidates, cache, selectionEvents, err := p.selectClustering(
		ctx, plan, request, eligible, plan.Mode, now,
	)
	if err != nil {
		return Plan{}, nil, err
	}

	events = append(events, selectionEvents...)

	plan.Candidates = candidates

	earliestQuoteExpiry := plan.ExpiresAt
	unserviceable := len(refused) > 0

	for _, member := range refused {
		plan.Groups = append(plan.Groups, refusedGroup(member))
	}

	for _, group := range selected.clusters {
		// Every cluster in the SELECTED clustering has already been quoted
		// during candidate evaluation. Reading the cached answer rather than
		// asking again is what makes selection free of side effects: the
		// authority was asked once, for the plan that is now being built.
		result, ok := cache.entries[quoteReference(plan, group)]

		if !ok {
			// A cluster the selected candidate never had quoted can only be
			// one the execution gate refused. It cannot be executed, and
			// inventing a fee for it is the one thing this package never does.
			for _, member := range group.members {
				member.reasonCode = ReasonNoAuthorityAvailable
				plan.Groups = append(plan.Groups, refusedGroup(member))
			}

			unserviceable = true

			continue
		}

		if result.mission == nil {
			for _, member := range group.members {
				member.reasonCode = result.reason
				plan.Groups = append(plan.Groups, refusedGroup(member))
			}

			unserviceable = true

			continue
		}

		mission := *result.mission

		plan.Missions = append(plan.Missions, mission)
		plan.Funding = plan.Funding.Add(mission.Funding)

		if len(plan.DeliveryPolicySnapshot) == 0 &&
			len(group.members[0].policySnapshot) > 0 {
			plan.DeliveryPolicySnapshot = append(
				[]byte(nil), group.members[0].policySnapshot...,
			)
		}

		plan.CustomerDeliveryPriceGuaranteeMinor +=
			mission.CustomerPriceGuaranteeMinor

		if plan.DeliveryPriceGuaranteeCurrency == "" {
			plan.DeliveryPriceGuaranteeCurrency = mission.Currency
		}

		if mission.Quote.ExpiresAt != nil &&
			mission.Quote.ExpiresAt.Before(earliestQuoteExpiry) {
			earliestQuoteExpiry = *mission.Quote.ExpiresAt
		}

		shares := shareFunding(mission.Funding, group.members)

		for index, member := range group.members {
			plan.Groups = append(plan.Groups, PlanGroup{
				CheckoutGroupID:             member.group.CheckoutGroupID,
				BusinessID:                  member.group.BusinessID,
				BranchID:                    member.group.BranchID,
				Serviceable:                 true,
				ReasonCode:                  ReasonServiceable,
				Authority:                   mission.Authority,
				MissionID:                   mission.ID,
				VehicleClass:                mission.VehicleClass,
				Requirement:                 member.requirement,
				RequirementSnapshot:         member.snapshot,
				Funding:                     shares[index],
				DeliveryMode:                mission.DeliveryMode,
				FallbackTrigger:             mission.FallbackTrigger,
				CustomerPriceGuaranteeMinor: mission.CustomerPriceGuaranteeMinor,
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

// selectClustering builds every alternative, quotes them, and picks one.
//
// It returns the winning clustering, ALL candidates with their outcomes, and
// the quote cache, so the caller executes exactly the quotes that were
// compared rather than asking the authorities a second set of questions.
func (p *Planner) selectClustering(
	ctx context.Context,
	plan Plan,
	request PlanRequest,
	eligible []planned,
	mode Mode,
	now time.Time,
) (clustering, []PlanCandidate, *quoteCache, []PlanEvent, error) {
	cache := newQuoteCache()

	var events []PlanEvent

	split := p.splitClustering(eligible)

	// The split alternative is evaluated FIRST and unconditionally. It is the
	// baseline the detour of every other alternative is measured against, and
	// it is the fallback when nothing else is executable.
	splitCandidate, splitEvents, _, err := p.evaluateClustering(
		ctx, plan, request, split, mode, cache, 0, now,
	)
	if err != nil {
		return clustering{}, nil, nil, nil, err
	}

	events = append(events, splitEvents...)

	baseline := slowestLeg(splitCandidate)

	candidates := []PlanCandidate{splitCandidate}
	clusterings := []clustering{split}

	capabilities := p.serviceCapabilities(ctx)

	consolidated, mergeEvents, refusals := p.consolidatedClustering(
		eligible, mode, capabilities,
	)

	switch {
	case consolidated.merges():
		consolidatedCandidate, consolidatedEvents, _, evalErr :=
			p.evaluateClustering(
				ctx, plan, request, consolidated, mode, cache, baseline, now,
			)
		if evalErr != nil {
			return clustering{}, nil, nil, nil, evalErr
		}

		// A consolidated candidate whose detour exceeds the mode tolerance is
		// not a slower option, it is an option the customer own preference
		// excludes. It is recorded as a hard refusal so nothing downstream
		// can trade it back in on price.
		if consolidatedCandidate.Feasible() &&
			consolidatedCandidate.DetourMinutes >
				int(p.optimization.For(mode).DetourTolerance/time.Minute) {
			consolidatedCandidate.Status = StatusCandidateIneligible
			consolidatedCandidate.ReasonCode = ReasonCandidateETADetourExceeded
		}

		events = append(events, mergeEvents...)
		events = append(events, consolidatedEvents...)

		candidates = append(candidates, consolidatedCandidate)
		clusterings = append(clusterings, consolidated)

	case len(refusals) > 0:
		// Nothing could be merged, and the reasons are worth keeping. This
		// records the would-be consolidation as an ineligible candidate with
		// the first real refusal, so "why was my order split" is answerable
		// even when no consolidated plan was ever constructible.
		ineligible := PlanCandidate{
			Kind:                CandidateConsolidated,
			Mode:                mode,
			Status:              StatusCandidateIneligible,
			ReasonCode:          refusals[0],
			PolicyVersion:       p.optimization.Version,
			ServiceCapabilities: multiPickupEvidence(capabilities),
		}

		ineligible.Aggregate(baseline)

		candidates = append(candidates, ineligible)
		clusterings = append(clusterings, clustering{
			kind: CandidateConsolidated,
		})
	}

	outcome := compareCandidates(candidates, mode, p.optimization.For(mode))

	if outcome.selected < 0 {
		// Nothing was feasible. The split alternative is still executed so
		// each group is refused with its own reason rather than the whole
		// plan failing with none.
		markCandidateOutcomes(candidates, -1, outcome)
		SortCandidates(candidates)

		return split, candidates, cache, events, nil
	}

	markCandidateOutcomes(candidates, outcome.selected, outcome)

	winner := clusterings[outcome.selected]
	winnerKind := candidates[outcome.selected].Kind

	SortCandidates(candidates)

	return winner, candidates, cache, append(events, PlanEvent{
		Action: EventCandidateSelected,
		Detail: string(winnerKind),
	}), nil
}

// markCandidateOutcomes writes the comparison result onto the candidates.
//
// An INELIGIBLE candidate keeps the constraint reason it was refused with.
// Overwriting it with a comparison outcome would turn "this could not be
// done" into "this cost more", which is the one misrepresentation the whole
// candidate model exists to prevent.
func markCandidateOutcomes(
	candidates []PlanCandidate,
	selected int,
	outcome comparison,
) {
	for index := range candidates {
		if candidates[index].Status == StatusCandidateIneligible {
			continue
		}

		if index == selected {
			candidates[index].Status = StatusCandidateSelected
			candidates[index].ReasonCode = ReasonCandidateChosen

			continue
		}

		candidates[index].Status = StatusCandidateRejected

		if reason, ok := outcome.reasons[index]; ok {
			candidates[index].ReasonCode = reason

			continue
		}

		candidates[index].ReasonCode = ReasonCandidateTieBreak
	}
}

func refusedGroup(member planned) PlanGroup {
	reason := member.reasonCode

	if reason == "" || reason == ReasonServiceable {
		reason = ReasonNoAuthorityAvailable
	}

	return PlanGroup{
		CheckoutGroupID:     member.group.CheckoutGroupID,
		BusinessID:          member.group.BusinessID,
		BranchID:            member.group.BranchID,
		Serviceable:         false,
		ReasonCode:          reason,
		Authority:           AuthorityNone,
		Requirement:         member.requirement,
		RequirementSnapshot: member.snapshot,
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
