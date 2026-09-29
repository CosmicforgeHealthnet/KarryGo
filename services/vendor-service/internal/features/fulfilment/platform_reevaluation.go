package fulfilment

import (
	"context"

	"cosmicforge/logistics/services/vendor-service/internal/platform/geo"
)

// Post-pick physical re-evaluation (Karryam V4 §14.6).
//
// Checkout planned the delivery on the ESTIMATED load. Picking, measurement and
// substitution can change the real load before custody. The platform must then
// re-answer the same question with the same engine - branch authority, catalogue
// logistics profiles x FINAL quantities, the requirement/vehicle engine and the
// owning authority's real quote - but for PLATFORM execution only: a paid
// platform delivery is never silently turned into a vendor-owned one here.

// PlatformReevaluation is the platform-only answer for one load.
type PlatformReevaluation struct {
	// Snapshot is the immutable requirement evidence, exactly as checkout
	// stores it.
	Snapshot RequirementSnapshot

	// Requirement is the aggregated load that was evaluated.
	Requirement Requirement

	// Selected reports that a safe executable class exists.
	Selected bool

	Service      Service
	VehicleClass VehicleClass

	// ReasonCode explains a refusal: no safe class, missing metadata, an
	// inactive branch, an unreachable or refusing authority.
	ReasonCode string

	// Mission is the authority-quoted mission when Selected and the authority
	// answered serviceable. Nil otherwise.
	Mission *Mission
}

// EvaluatePlatformLoad re-plans ONE group's load for platform execution.
//
// It creates nothing and persists nothing. The only side effect is the owning
// authority storing the quote it was asked for, which is the same quote-only
// call checkout makes.
//
// quoteWhen decides whether the selected class needs an authority quote at
// all; nil always quotes. A caller that keeps its current class passes a
// predicate so an unchanged load makes no authority call.
func (p *Planner) EvaluatePlatformLoad(
	ctx context.Context, request PlanRequest, group GroupInput,
	quoteWhen func(VehicleClass) bool,
) (PlatformReevaluation, error) {
	out := PlatformReevaluation{}
	if !geo.ValidCoordinate(request.Destination.Latitude, request.Destination.Longitude) {
		out.ReasonCode = ReasonNoDeliveryAddress
		return out, nil
	}

	branch, found, err := p.branches.ResolveBranch(ctx, group.BusinessID, group.BranchID)
	if err != nil {
		return out, err
	}
	switch {
	case !found:
		out.ReasonCode = ReasonBranchUnknown
	case !branch.Active():
		out.ReasonCode = ReasonBranchInactive
	}
	if out.ReasonCode != "" {
		return out, nil
	}

	profiles, err := p.logistics.ResolveLineProfiles(ctx, group.BusinessID, group.Lines)
	if err != nil {
		return out, err
	}
	out.Requirement = Aggregate(group.BusinessID, group.BranchID, group.Lines, profiles)

	snapshot, err := p.evaluateRequirement(group, out.Requirement)
	if err != nil {
		return out, err
	}
	out.Snapshot = snapshot
	if !snapshot.Decision.Selected() {
		out.ReasonCode = snapshot.Decision.PrimaryReason
		if out.ReasonCode == "" {
			out.ReasonCode = ReasonNoAuthorityAvailable
		}
		return out, nil
	}

	selection := snapshot.Decision.Selection()
	out.Selected = true
	out.Service = selection.Service
	out.VehicleClass = selection.Class

	if quoteWhen != nil && !quoteWhen(selection.Class) {
		return out, nil
	}

	anchor := planned{
		group: group, requirement: out.Requirement, branch: branch,
		authority: AuthorityPlatform, selection: selection, snapshot: &snapshot,
		serviceable: true, reasonCode: ReasonServiceable,
	}
	plan := Plan{CheckoutSessionID: request.CheckoutSessionID}
	mission, reason, _, err := p.platformMission(ctx, plan, request, cluster{
		authority: AuthorityPlatform, service: selection.Service, class: selection.Class,
		members: []planned{anchor},
	}, anchor, p.now().UTC())
	if err != nil {
		return out, err
	}
	if mission == nil {
		out.ReasonCode = reason
		return out, nil
	}
	out.Mission = mission
	return out, nil
}
