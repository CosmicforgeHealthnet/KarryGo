package basket

// Recurring generation: exactly once per occurrence, re-shopped every time.
//
// Two guarantees, and they pull in opposite directions, which is why they are
// stated together.
//
// EXACTLY ONCE. One logical generation per scheduled occurrence. A retry, a
// worker replay, a process restart, a duplicate scheduler delivery or an
// at-least-once event must not produce two proposals, two accepted cart
// versions, two quotes, two orders or two charges. The guarantee rests on a
// deterministic occurrence key and a UNIQUE index, never on an in-memory
// lock: a lock is scoped to one process and every failure mode above involves
// more than one.
//
// RE-SHOPPED EVERY TIME. The stored template is a list of needs, so an
// occurrence goes shopping against today's catalogue: today's active
// listings, today's sellable stock, today's prices, today's eligible vendors
// and today's delivery plan. Nothing is carried forward from last week except
// the customer's own requirements and constraints.
//
// And the boundary that keeps this safe: GENERATING a proposal is not
// ORDERING anything. The stages are distinct and each has its own gate -
// generation, proposal, customer acceptance, cart version, quote, order,
// payment. This file produces the first two and never reaches the rest.

import (
	"context"
	"sort"
	"strings"
	"time"
)

// GenerationState is a recurrence generation's lifecycle.
type GenerationState string

const (
	// GenerationProposed means a basket was computed and is waiting for the
	// customer.
	GenerationProposed GenerationState = "PROPOSED"

	// GenerationAwaitingApproval means the basket changed materially in a way
	// the customer's policy does not preauthorize.
	GenerationAwaitingApproval GenerationState = "AWAITING_APPROVAL"

	// GenerationAutoApproved means the change was inside the customer's own
	// explicit, bounded preauthorization.
	//
	// It still creates no order and moves no money. It means the proposal may
	// proceed to the ordinary purchase path without asking again, not that
	// anything has been bought.
	GenerationAutoApproved GenerationState = "AUTO_APPROVED"

	// GenerationUnfulfillable means today's catalogue cannot satisfy the
	// template at all.
	GenerationUnfulfillable GenerationState = "UNFULFILLABLE"
)

// ChangeClass names one kind of difference from the last approved basket.
//
// The vocabulary is closed and the distinctions are the point. A different
// PRODUCT is a change the customer must see. A different provider assigned by
// a logistics service is not a change to their basket at all, and treating
// every operational difference as material would make a recurring basket ask
// for approval every single week - which trains people to approve without
// reading.
const (
	ChangeDifferentProduct = "DIFFERENT_PRODUCT"
	ChangeDifferentBrand   = "DIFFERENT_BRAND"
	ChangeDifferentVendor  = "DIFFERENT_VENDOR"
	ChangeQuantityUnmet    = "QUANTITY_UNMET"
	ChangePriceIncrease    = "MATERIAL_PRICE_INCREASE"
	ChangeCeilingExceeded  = "SPENDING_CEILING_EXCEEDED"
	ChangeNothingMaterial  = "NO_MATERIAL_CHANGE"
)

// MaterialPriceIncreaseBPS is how far a delivered total may rise before the
// increase is material on its own.
//
// Five per cent. Prices move; a recurring basket that demanded approval for
// every kobo would be approved unread within a month, which is worse than
// not asking.
const MaterialPriceIncreaseBPS int64 = 500

// PlanComparison is the SAME_VENDOR versus OPTIMIZED answer.
//
// Both sides are computed against TODAY's catalogue. The same-vendor side is
// not last week's receipt: it is what buying from the same vendors would cost
// now, which is the only comparison that means anything - a customer deciding
// whether to accept a change needs to know what declining it would cost, not
// what it cost before.
type PlanComparison struct {
	// SameVendorAvailable is false when last week's vendors can no longer
	// supply the basket. There is then nothing to compare against and the
	// optimized plan is the only plan, which is stated rather than presented
	// as a choice.
	SameVendorAvailable bool `json:"same_vendor_plan_available"`

	SameVendorReason string `json:"same_vendor_unavailable_reason,omitempty"`

	SameVendorMerchandiseMinor int64 `json:"same_vendor_merchandise_minor,omitempty"`
	SameVendorDeliveryMinor    int64 `json:"same_vendor_delivery_minor,omitempty"`
	SameVendorTotalMinor       int64 `json:"same_vendor_final_total_minor,omitempty"`
	SameVendorCount            int   `json:"same_vendor_vendor_count,omitempty"`
	SameVendorMinutes          int   `json:"same_vendor_completion_minutes,omitempty"`

	OptimizedMerchandiseMinor int64 `json:"optimized_merchandise_minor"`
	OptimizedDeliveryMinor    int64 `json:"optimized_delivery_minor"`
	OptimizedTotalMinor       int64 `json:"optimized_final_total_minor"`
	OptimizedVendorCount      int   `json:"optimized_vendor_count"`
	OptimizedMinutes          int   `json:"optimized_completion_minutes"`

	// Deltas are optimized minus same-vendor. Negative means the optimized
	// plan is cheaper, faster or uses fewer vendors.
	MerchandiseDeltaMinor  int64 `json:"merchandise_delta_minor"`
	DeliveryDeltaMinor     int64 `json:"delivery_delta_minor"`
	TotalDeltaMinor        int64 `json:"final_total_delta_minor"`
	VendorCountDelta       int   `json:"vendor_count_delta"`
	CompletionDeltaMinutes int   `json:"expected_delivery_delta_minutes"`

	// Changes are the material differences from the last approved basket.
	Changes []string `json:"material_changes,omitempty"`

	// VendorChanges and ProductChanges name exactly what moved, so a customer
	// approving a change can see what they are approving.
	VendorChanges []string       `json:"vendor_changes,omitempty"`
	Substitutions []Substitution `json:"substitutions,omitempty"`
}

// Generation is one recurrence occurrence, generated exactly once.
type Generation struct {
	ID string `json:"generation_id"`

	TemplateID string `json:"template_id"`
	CustomerID string `json:"-"`

	// OccurrenceKey is the deterministic identity of the scheduled moment.
	// The UNIQUE index on (template_id, occurrence_key) is what makes
	// exactly-once a property of the database rather than of the worker.
	OccurrenceKey string    `json:"occurrence_key"`
	OccurrenceAt  time.Time `json:"occurrence_at"`
	Timezone      string    `json:"timezone"`

	State GenerationState `json:"state"`

	// ProposalID is the offer this occurrence produced, when it produced one.
	// A generation without a proposal is one that could not be fulfilled, and
	// the two are distinguished so an empty proposal is never mistaken for a
	// missing one.
	ProposalID string `json:"proposal_id,omitempty"`

	Comparison PlanComparison `json:"comparison"`

	Reason string `json:"reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// GenerateOccurrence produces exactly one generation for one scheduled
// moment.
//
// The first thing it does is CLAIM the occurrence, before any shopping. That
// ordering is deliberate: claiming first means a duplicate delivery loses the
// race immediately and cheaply, whereas shopping first and claiming after
// would let two workers both do the expensive work and both call logistics
// authorities, with one of them discarding the result.
//
// A generation that already exists is RETURNED, not recreated. That is what
// makes a crash between the claim and the response recoverable: the retry
// finds its own earlier work and resumes rather than duplicating it.
func (s *Service) GenerateOccurrence(
	ctx context.Context,
	template Template,
	occurrenceAt time.Time,
	destination Destination,
) (Generation, error) {
	if s.store == nil {
		return Generation{}, ErrDependencyMissing
	}

	if !template.Enabled {
		return Generation{}, ErrTemplateDisabled
	}

	key := template.OccurrenceKey(occurrenceAt)

	// Claim first. The insert is ON CONFLICT DO NOTHING against a unique
	// index, so exactly one caller wins and every other caller reads back the
	// winner's row.
	generation, created, err := s.store.ClaimOccurrence(ctx, Generation{
		TemplateID:    template.ID,
		CustomerID:    template.CustomerID,
		OccurrenceKey: key,
		OccurrenceAt:  occurrenceAt.UTC(),
		Timezone:      template.Timezone,
		State:         GenerationProposed,
	})
	if err != nil {
		return Generation{}, err
	}

	if !created {
		// Somebody else generated this occurrence, or this process did before
		// it crashed. Either way the work is done and doing it again would
		// produce a second proposal for one scheduled delivery.
		return generation, nil
	}

	result, err := s.optimize(ctx, Request{
		CustomerID:           template.CustomerID,
		Currency:             template.Currency,
		Destination:          destination,
		FulfilmentPreference: template.FulfilmentPreference,
		Requirements:         template.Requirements,
		Objectives:           []Objective{template.Objective},
	}, nil)
	if err != nil {
		return Generation{}, err
	}

	if len(result.Variants) == 0 {
		generation.State = GenerationUnfulfillable
		generation.Reason = strings.Join(result.BindingConstraints, ",")

		if err = s.store.CompleteOccurrence(ctx, generation); err != nil {
			return Generation{}, err
		}

		return generation, nil
	}

	optimized := result.Variants[0]

	previous, err := s.store.LastApprovedBasket(
		ctx, template.CustomerID, template.ID,
	)
	if err != nil {
		return Generation{}, err
	}

	sameVendor := s.sameVendorPlan(ctx, template, destination, previous)

	generation.Comparison = compareRecurringPlans(optimized, sameVendor, previous)

	generation.State = approvalStateFor(
		template, generation.Comparison, optimized,
	)

	proposal, err := s.store.CreateProposal(ctx, Proposal{
		CustomerID:               template.CustomerID,
		Objective:                optimized.Objective,
		State:                    ProposalOpen,
		Currency:                 optimized.Currency,
		MerchandiseTotalMinor:    optimized.MerchandiseTotalMinor,
		DeliveryTotalMinor:       optimized.DeliveryTotalMinor,
		FinalDeliveredTotalMinor: optimized.FinalDeliveredTotalMinor,
		VendorCount:              optimized.VendorCount,
		Lines:                    optimized.Lines,
		Substitutions:            optimized.Substitutions,
		Fingerprint:              optimized.Fingerprint,
		ExpiresAt:                s.now().UTC().Add(s.proposalTTL),
	})
	if err != nil {
		return Generation{}, err
	}

	generation.ProposalID = proposal.ID

	if err = s.store.CompleteOccurrence(ctx, generation); err != nil {
		return Generation{}, err
	}

	return generation, nil
}

// sameVendorPlan prices the previous basket's vendors against TODAY.
//
// It returns an unavailable comparison rather than an error when the old
// vendors can no longer supply the basket. That is an ordinary, expected
// outcome - shops run out, prices change, listings are withdrawn - and it has
// a correct customer-facing answer: there is nothing to compare against, so
// the optimized plan is the only plan.
func (s *Service) sameVendorPlan(
	ctx context.Context,
	template Template,
	destination Destination,
	previous []ProposedLine,
) *Variant {
	if len(previous) == 0 {
		return nil
	}

	refs := make([]string, 0, len(previous))

	for _, line := range previous {
		refs = append(refs, line.ListingRef)
	}

	listings, err := s.catalogue.ResolveListings(ctx, refs)

	if err != nil || len(listings) == 0 {
		return nil
	}

	basket := candidateBasket{objective: template.Objective}

	byRef := map[string]Requirement{}

	for _, requirement := range template.Requirements {
		byRef[requirement.Ref] = requirement
	}

	for _, line := range previous {
		listing, ok := listings[line.ListingRef]

		if !ok {
			// One line of the old basket can no longer be bought, so the old
			// basket as a whole cannot be repriced. A partial same-vendor
			// total compared against a complete optimized one would make the
			// old plan look cheaper by being incomplete.
			return nil
		}

		requirement, ok := byRef[line.RequirementRef]

		if !ok {
			return nil
		}

		if listing.AvailableQuantity < requirement.Quantity {
			return nil
		}

		basket.selections = append(basket.selections, selection{
			requirement: requirement,
			listing:     listing,
			basis:       BasisExactListing,
		})
	}

	request := Request{
		CustomerID:           template.CustomerID,
		Currency:             template.Currency,
		Destination:          destination,
		FulfilmentPreference: template.FulfilmentPreference,
	}

	quote, err := s.quoteBasket(ctx, request, basket)

	if err != nil || !quote.Serviceable {
		return nil
	}

	variant := buildVariant(request, basket, quote, nil)

	return &variant
}

// compareRecurringPlans produces the SAME_VENDOR versus OPTIMIZED answer and
// the list of material changes.
func compareRecurringPlans(
	optimized Variant,
	sameVendor *Variant,
	previous []ProposedLine,
) PlanComparison {
	comparison := PlanComparison{
		OptimizedMerchandiseMinor: optimized.MerchandiseTotalMinor,
		OptimizedDeliveryMinor:    optimized.DeliveryTotalMinor,
		OptimizedTotalMinor:       optimized.FinalDeliveredTotalMinor,
		OptimizedVendorCount:      optimized.VendorCount,
		OptimizedMinutes:          optimized.PlanSummary.CompletionMinutes,
		Substitutions:             optimized.Substitutions,
	}

	if sameVendor != nil {
		comparison.SameVendorAvailable = true
		comparison.SameVendorMerchandiseMinor = sameVendor.MerchandiseTotalMinor
		comparison.SameVendorDeliveryMinor = sameVendor.DeliveryTotalMinor
		comparison.SameVendorTotalMinor = sameVendor.FinalDeliveredTotalMinor
		comparison.SameVendorCount = sameVendor.VendorCount
		comparison.SameVendorMinutes = sameVendor.PlanSummary.CompletionMinutes

		comparison.MerchandiseDeltaMinor = optimized.MerchandiseTotalMinor -
			sameVendor.MerchandiseTotalMinor
		comparison.DeliveryDeltaMinor = optimized.DeliveryTotalMinor -
			sameVendor.DeliveryTotalMinor
		comparison.TotalDeltaMinor = optimized.FinalDeliveredTotalMinor -
			sameVendor.FinalDeliveredTotalMinor
		comparison.VendorCountDelta = optimized.VendorCount -
			sameVendor.VendorCount
		comparison.CompletionDeltaMinutes = optimized.PlanSummary.CompletionMinutes -
			sameVendor.PlanSummary.CompletionMinutes
	} else if len(previous) > 0 {
		comparison.SameVendorReason = "PREVIOUS_VENDORS_CANNOT_SUPPLY_TODAY"
	}

	comparison.Changes, comparison.VendorChanges =
		materialChanges(optimized, previous)

	return comparison
}

// materialChanges names what actually moved since the last approved basket.
//
// Only differences a customer would care about. A different provider assigned
// by a logistics service, a different route, a different mission grouping -
// none of those is a change to what the customer is buying, and reporting
// them as material would make a recurring basket demand approval every week
// for nothing.
func materialChanges(
	optimized Variant,
	previous []ProposedLine,
) ([]string, []string) {
	if len(previous) == 0 {
		// The first occurrence has nothing to differ from.
		return []string{ChangeNothingMaterial}, nil
	}

	previousByRef := make(map[string]ProposedLine, len(previous))

	var previousTotal int64

	for _, line := range previous {
		previousByRef[line.RequirementRef] = line
		previousTotal += line.LineTotalMinor
	}

	changes := map[string]bool{}

	vendorChanges := map[string]bool{}

	for _, line := range optimized.Lines {
		before, ok := previousByRef[line.RequirementRef]

		if !ok {
			changes[ChangeDifferentProduct] = true

			continue
		}

		if !strings.EqualFold(before.ProductRef, line.ProductRef) ||
			!strings.EqualFold(before.VariantRef, line.VariantRef) {
			changes[ChangeDifferentProduct] = true
		}

		if !strings.EqualFold(before.Brand, line.Brand) {
			changes[ChangeDifferentBrand] = true
		}

		if before.BusinessID != line.BusinessID ||
			before.BranchID != line.BranchID {
			changes[ChangeDifferentVendor] = true
			vendorChanges[before.BusinessID+" -> "+line.BusinessID] = true
		}

		if before.Quantity != line.Quantity {
			changes[ChangeQuantityUnmet] = true
		}
	}

	if previousTotal > 0 {
		increase := optimized.MerchandiseTotalMinor - previousTotal

		if increase > 0 &&
			increase*10000 > previousTotal*MaterialPriceIncreaseBPS {
			changes[ChangePriceIncrease] = true
		}
	}

	if len(changes) == 0 {
		changes[ChangeNothingMaterial] = true
	}

	return sortedKeys(changes), sortedKeys(vendorChanges)
}

// approvalStateFor decides whether this occurrence may proceed without
// asking.
//
// Preauthorization is explicit and bounded, and every gate below has to pass.
// The default, whenever anything is unclear, is to ask - a recurring basket
// that guessed in favour of proceeding would be spending money on a guess.
func approvalStateFor(
	template Template,
	comparison PlanComparison,
	optimized Variant,
) GenerationState {
	if template.ApprovalPolicy == ApprovalAlwaysAsk {
		return GenerationProposed
	}

	// A ceiling is mandatory for any auto-approving policy, and it is
	// re-checked here rather than trusted from validation: this is the last
	// point before a customer stops being asked.
	if template.CeilingMinor <= 0 ||
		optimized.FinalDeliveredTotalMinor > template.CeilingMinor {
		return GenerationAwaitingApproval
	}

	for _, change := range comparison.Changes {
		switch change {
		case ChangeNothingMaterial:
			continue

		case ChangeDifferentVendor:
			// Permitted ONLY by the policy that names it, and only because
			// the article is unchanged: the customer receives exactly what
			// they chose, bought at a different shop.
			if template.ApprovalPolicy == ApprovalAutoWithinCeiling {
				continue
			}

			return GenerationAwaitingApproval

		default:
			// A different product, a different brand, an unmet quantity or a
			// material price rise. None of these is preauthorized by any
			// policy this feature offers.
			return GenerationAwaitingApproval
		}
	}

	return GenerationAutoApproved
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))

	for key := range set {
		out = append(out, key)
	}

	sort.Strings(out)

	return out
}
