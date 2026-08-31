package basket

// The optimization service.
//
// One evaluation pipeline serves all three capabilities, because they are the
// same question asked from three starting points:
//
//	Optimize My Cart   requirements derived from what is already in the cart
//	Budget shopping    requirements stated directly by the customer
//	Recurring basket   requirements stored on a template
//
// Everything downstream of "here are the requirements" is shared: resolve
// against the real catalogue, enumerate bounded baskets, price each one's
// delivery through the quote-only planner, compare delivered totals, and
// report - including reporting failure honestly when there is nothing to
// report success about.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
)

// ServiceOptions configures the service.
type ServiceOptions struct {
	Catalogue CatalogueAuthority
	Quoter    PlanQuoter
	Carts     CartAuthority
	Store     *Store

	// ProposalTTL bounds how long an offer stands. Left zero, the default
	// applies.
	ProposalTTL time.Duration

	Now Clock
}

// Service is the basket optimization entry point.
type Service struct {
	catalogue CatalogueAuthority
	quoter    PlanQuoter
	carts     CartAuthority
	store     *Store

	// addresses resolves a recurring template saved-location reference. It is
	// attached separately because only the recurring path needs it: a cart
	// carries its own destination snapshot already.
	addresses AddressResolver

	proposalTTL time.Duration

	now Clock
}

// NewService builds the service.
//
// A missing catalogue or quoter is a wiring error rather than a runtime
// refusal: an optimizer with no catalogue would return "nothing is available"
// for every request, which is a lie that looks like an answer.
func NewService(options ServiceOptions) (*Service, error) {
	if options.Catalogue == nil || options.Quoter == nil {
		return nil, ErrDependencyMissing
	}

	ttl := options.ProposalTTL

	if ttl <= 0 {
		ttl = DefaultProposalTTL
	}

	now := options.Now

	if now == nil {
		now = time.Now
	}

	return &Service{
		catalogue:   options.Catalogue,
		quoter:      options.Quoter,
		carts:       options.Carts,
		store:       options.Store,
		proposalTTL: ttl,
		now:         now,
	}, nil
}

// Optimize answers one budget-or-basket question.
//
// It creates nothing: no missions, no reservations, no payment intents, no
// orders and no cart versions. Persisting the variants as acceptable
// proposals is a separate, explicit step the caller asks for, because most
// callers are asking "what would this cost" and have nothing to accept.
func (s *Service) Optimize(
	ctx context.Context,
	request Request,
) (Result, error) {
	return s.optimize(ctx, request, nil)
}

// optimize is the shared pipeline.
//
// originals maps a requirement ref to the listing it was derived FROM, which
// only the cart path has. It is what lets a manufacturer trade identifier be
// compared at all: a requirement carries no identifiers of its own, so
// without the original there is nothing to match against and the equivalence
// falls back to catalogue and taxonomy evidence.
func (s *Service) optimize(
	ctx context.Context,
	request Request,
	originals map[string]Listing,
) (Result, error) {
	normalized, err := request.Normalize()
	if err != nil {
		return Result{}, err
	}

	resolved, err := s.resolve(ctx, normalized, originals)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Currency:       normalized.Currency,
		BudgetMinor:    normalized.BudgetMinor,
		OptimalityNote: OptimalityUnderBoundedSearch,
	}

	// A requirement nothing can satisfy makes the whole basket impossible.
	// There is no cheaper complete basket, because there is no complete
	// basket, and reporting a minimum feasible total here would be quoting a
	// price for something that cannot be bought.
	for _, entry := range resolved {
		if entry.satisfiable() {
			continue
		}

		result.NoCompleteBasket = true

		result.UnmetRequirements = append(
			result.UnmetRequirements, UnmetRequirement{
				RequirementRef: entry.requirement.Ref,
				Constraints:    entry.blockingFailures,
			},
		)
	}

	if result.NoCompleteBasket {
		result.BindingConstraints = unionConstraints(result.UnmetRequirements)
		result.QualityVariantUnavailableReason =
			qualityUnavailableReason(resolved, normalized.Objectives)

		return result, nil
	}

	baskets, truncated := buildBaskets(resolved, normalized.Objectives)

	result.SearchTruncated = truncated
	result.EvaluatedBaskets = len(baskets)

	for _, basket := range baskets {
		quote, quoteErr := s.quoteBasket(ctx, normalized, basket)

		if quoteErr != nil {
			return Result{}, quoteErr
		}

		// A basket whose delivery cannot be planned is not a cheaper basket.
		// It is a basket that cannot be delivered, and letting it into the
		// comparison with a zero delivery cost would make it win.
		if !quote.Serviceable {
			continue
		}

		result.Variants = append(
			result.Variants,
			buildVariant(normalized, basket, quote, originals),
		)
	}

	rankVariants(result.Variants, normalized)

	result.Variants = dedupeVariants(result.Variants)

	for index := range result.Variants {
		if normalized.HasBudget() &&
			result.Variants[index].FinalDeliveredTotalMinor <=
				normalized.BudgetMinor {
			result.Variants[index].FitsBudget = true
			result.FitsBudget = true
		}
	}

	// The minimum feasible delivered total is the cheapest COMPLETE basket
	// that satisfies every hard constraint, whatever it costs. It is read off
	// the evaluated variants rather than extrapolated, so it is a number the
	// platform can actually produce a basket for.
	if len(result.Variants) > 0 {
		minimum := result.Variants[0].FinalDeliveredTotalMinor

		for _, variant := range result.Variants[1:] {
			if variant.FinalDeliveredTotalMinor < minimum {
				minimum = variant.FinalDeliveredTotalMinor
			}
		}

		result.MinimumFeasibleDeliveredTotalMinor = minimum

		if normalized.HasBudget() && minimum > normalized.BudgetMinor {
			result.BudgetShortfallMinor = minimum - normalized.BudgetMinor
			result.BindingConstraints = budgetBindingConstraints(resolved)
		}
	}

	// Every basket was assembled and none could be delivered. That is not a
	// budget problem and must not be reported as one.
	if len(result.Variants) == 0 {
		result.NoCompleteBasket = true
		result.BindingConstraints = append(
			result.BindingConstraints, "NO_DELIVERABLE_BASKET",
		)
	}

	result.QualityVariantUnavailableReason =
		qualityUnavailableReason(resolved, normalized.Objectives)

	return result, nil
}

// buildVariant assembles one priced basket into a customer-facing variant.
func buildVariant(
	request Request,
	basket candidateBasket,
	quote fulfilment.QuoteOnlyResult,
	originals map[string]Listing,
) Variant {
	lines := make([]ProposedLine, 0, len(basket.selections))

	var substitutions []Substitution

	for _, item := range basket.selections {
		lines = append(lines, ProposedLine{
			RequirementRef:          item.requirement.Ref,
			ListingRef:              item.listing.ListingRef,
			ProductRef:              item.listing.ProductRef,
			VariantRef:              item.listing.VariantRef,
			SellUnitRef:             item.listing.SellUnitRef,
			BusinessID:              item.listing.BusinessID,
			BranchID:                item.listing.BranchID,
			DisplayName:             item.listing.DisplayName,
			Brand:                   item.listing.Brand,
			Quantity:                item.requirement.Quantity,
			UnitPriceMinor:          item.listing.UnitPriceMinor,
			LineTotalMinor:          item.lineTotalMinor(),
			Currency:                item.listing.Currency,
			PriceReference:          item.listing.PriceReference,
			CatalogueRevision:       item.listing.CatalogueRevision,
			QuotedAvailableQuantity: item.listing.AvailableQuantity,
		})

		if substitution, changed := substitutionOf(item, originals); changed {
			substitutions = append(substitutions, substitution)
		}
	}

	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].RequirementRef < lines[j].RequirementRef
	})

	sort.SliceStable(substitutions, func(i, j int) bool {
		return substitutions[i].RequirementRef < substitutions[j].RequirementRef
	})

	merchandise := basket.merchandiseMinor()
	delivery := quote.DeliveryCustomerMinor

	variant := Variant{
		Objective:                basket.objective,
		Currency:                 request.Currency,
		MerchandiseTotalMinor:    merchandise,
		DeliveryTotalMinor:       delivery,
		FinalDeliveredTotalMinor: merchandise + delivery,
		VendorCount:              basket.vendorCount(),
		MissionCount:             quote.MissionCount,
		FulfilmentMode:           string(quote.Mode),
		DeliveryWindow:           deliveryRangeFrom(quote),
		Lines:                    lines,
		Substitutions:            substitutions,
		ConstraintsSatisfied:     constraintsSatisfied(request),
		PlanSummary: PlanSummary{
			Mode:                  string(quote.Mode),
			MissionCount:          quote.MissionCount,
			VendorGroupCount:      quote.VendorGroupCount,
			DeliveryGrossMinor:    quote.DeliveryGrossMinor,
			DeliveryCustomerMinor: quote.DeliveryCustomerMinor,
			CompletionMinutes:     quote.CompletionMinutes,
			ConsolidationReasons:  consolidationReasons(quote),
			PolicyVersion:         quote.PolicyVersion,
		},
		Fingerprint: fingerprintOf(lines),
	}

	variant.Reasons = explain(variant, substitutions)

	return variant
}

// substitutionOf reports whether a selection differs from what the customer
// started with, and describes the difference.
func substitutionOf(
	item selection,
	originals map[string]Listing,
) (Substitution, bool) {
	original, ok := originals[item.requirement.Ref]

	if !ok {
		// Budget shopping has no "before". A requirement stated as a need was
		// never a specific product, so satisfying it is not a substitution.
		return Substitution{}, false
	}

	if strings.EqualFold(original.ListingRef, item.listing.ListingRef) {
		return Substitution{}, false
	}

	return Substitution{
		RequirementRef:  item.requirement.Ref,
		FromListingRef:  original.ListingRef,
		FromDisplayName: original.DisplayName,
		FromBrand:       original.Brand,
		FromUnitPrice:   original.UnitPriceMinor,
		FromSellUnitRef: original.SellUnitRef,

		ToListingRef:  item.listing.ListingRef,
		ToDisplayName: item.listing.DisplayName,
		ToBrand:       item.listing.Brand,
		ToUnitPrice:   item.listing.UnitPriceMinor,
		ToSellUnitRef: item.listing.SellUnitRef,

		Basis: item.basis,

		UnitPriceDeltaMinor: item.listing.UnitPriceMinor -
			original.UnitPriceMinor,

		VendorChanged: original.BusinessID != item.listing.BusinessID ||
			original.BranchID != item.listing.BranchID,
	}, true
}

// deliveryRangeFrom turns quoted transit into a customer-facing window.
//
// The range is the planner's own completion estimate with a proportional
// allowance either side. It is deliberately derived from the quote rather
// than from a stored average: an average is a claim about deliveries in
// general, and this is a statement about this one.
func deliveryRangeFrom(quote fulfilment.QuoteOnlyResult) DeliveryRange {
	minutes := quote.CompletionMinutes

	if minutes <= 0 {
		return DeliveryRange{}
	}

	slack := minutes / 4

	if slack < 5 {
		slack = 5
	}

	earliest := minutes - slack

	if earliest < 1 {
		earliest = 1
	}

	return DeliveryRange{
		EarliestMinutes: earliest,
		LatestMinutes:   minutes + slack,
	}
}

// constraintsSatisfied states, positively, which of the customer's own hard
// constraints this basket respects.
//
// Stated rather than implied. Silence about a constraint reads identically
// whether it was honoured or never checked, and only one of those is
// acceptable to show a customer.
func constraintsSatisfied(request Request) []string {
	satisfied := map[string]bool{}

	for _, requirement := range request.Requirements {
		if requirement.RequiredBrand != "" {
			satisfied["REQUIRED_BRAND_HONOURED"] = true
		}

		if requirement.MinimumQualityGrade != "" {
			satisfied["REQUIRED_QUALITY_GRADE_HONOURED"] = true
		}

		if requirement.Substitution == AllowExactOnly {
			satisfied["NO_SUBSTITUTION_MADE"] = true
		}

		satisfied["REQUESTED_QUANTITY_MET"] = true
		satisfied["SELLABLE_STOCK_CONFIRMED_AT_QUOTE_TIME"] = true
	}

	out := make([]string, 0, len(satisfied))

	for constraint := range satisfied {
		out = append(out, constraint)
	}

	sort.Strings(out)

	return out
}

// consolidationReasons lifts the fulfilment planner's candidate reasons.
//
// This is how a shopping proposal can answer "why is this arriving in two
// deliveries" with the same evidence a checkout would give - including, today,
// that no logistics runtime can drive a multi-stop collection.
func consolidationReasons(quote fulfilment.QuoteOnlyResult) []string {
	seen := map[string]bool{}

	var reasons []string

	for _, candidate := range quote.Candidates {
		if candidate.ReasonCode == "" || seen[candidate.ReasonCode] {
			continue
		}

		seen[candidate.ReasonCode] = true
		reasons = append(reasons, candidate.ReasonCode)
	}

	sort.Strings(reasons)

	return reasons
}

// explain produces the short, customer-readable reasons for a basket's shape.
func explain(variant Variant, substitutions []Substitution) []string {
	var reasons []string

	switch {
	case variant.VendorCount == 1:
		reasons = append(reasons, "SINGLE_VENDOR_BASKET")
	default:
		reasons = append(reasons, fmt.Sprintf(
			"VENDOR_COUNT_%d", variant.VendorCount,
		))
	}

	if variant.MissionCount > 1 {
		reasons = append(reasons, fmt.Sprintf(
			"DELIVERIES_%d", variant.MissionCount,
		))
	}

	if len(substitutions) > 0 {
		reasons = append(reasons, fmt.Sprintf(
			"SUBSTITUTIONS_%d", len(substitutions),
		))
	}

	return reasons
}

// rankVariants orders variants best-first for the requested objectives.
//
// The primary objective is whichever the customer asked for first. Ties fall
// through to delivered total, then vendor count, then the fingerprint - a
// total order, so the same catalogue always produces the same ranking.
func rankVariants(variants []Variant, request Request) {
	primary := ObjectiveValue

	if len(request.Objectives) > 0 {
		primary = request.Objectives[0]
	}

	sort.SliceStable(variants, func(i, j int) bool {
		left, right := variants[i], variants[j]

		if primary == ObjectiveFewestVendors {
			if left.VendorCount != right.VendorCount {
				return left.VendorCount < right.VendorCount
			}

			if left.DeliveryTotalMinor != right.DeliveryTotalMinor {
				return left.DeliveryTotalMinor < right.DeliveryTotalMinor
			}
		}

		if left.FinalDeliveredTotalMinor != right.FinalDeliveredTotalMinor {
			return left.FinalDeliveredTotalMinor <
				right.FinalDeliveredTotalMinor
		}

		if left.VendorCount != right.VendorCount {
			return left.VendorCount < right.VendorCount
		}

		return left.Fingerprint < right.Fingerprint
	})
}

// dedupeVariants removes baskets that are the same basket.
//
// Two objectives frequently reach the same answer - when one vendor is both
// cheapest and the only vendor, VALUE and FEWEST_VENDORS agree - and
// presenting that basket twice under two headings would be one answer wearing
// two labels.
func dedupeVariants(variants []Variant) []Variant {
	seen := map[string]bool{}

	out := make([]Variant, 0, len(variants))

	for _, variant := range variants {
		if seen[variant.Fingerprint] {
			continue
		}

		seen[variant.Fingerprint] = true
		out = append(out, variant)
	}

	return out
}

// qualityUnavailableReason explains an absent QUALITY_BALANCED variant.
func qualityUnavailableReason(
	resolved []resolvedRequirement,
	objectives []Objective,
) string {
	if !wantsObjective(objectives, ObjectiveQualityBalanced) {
		return ""
	}

	for _, entry := range resolved {
		hasSignal := false

		for _, option := range entry.admissible {
			if hasQualitySignal(option.listing) {
				hasSignal = true

				break
			}
		}

		if !hasSignal {
			// Named rather than vague. The platform's catalogue carries a
			// quality-grade SELECTION vocabulary for fresh goods and no
			// ordinal quality score for anything, so for most requirements
			// there is genuinely nothing to rank by - and inventing a score
			// to fill the slot would be presenting a fabrication as advice.
			return "NO_AUTHORITATIVE_QUALITY_SIGNAL_FOR_" +
				strings.ToUpper(entry.requirement.Ref)
		}
	}

	return ""
}

func unionConstraints(unmet []UnmetRequirement) []string {
	seen := map[string]bool{}

	var out []string

	for _, entry := range unmet {
		for _, constraint := range entry.Constraints {
			if seen[constraint] {
				continue
			}

			seen[constraint] = true
			out = append(out, constraint)
		}
	}

	sort.Strings(out)

	return out
}

// budgetBindingConstraints names what is stopping a cheaper valid basket from
// existing.
//
// It reports the customer's OWN hard constraints, because those are the ones
// they can choose to relax. Telling a customer their basket is over budget
// because prices are what they are is true and useless; telling them it is
// over budget because they required one brand is actionable.
func budgetBindingConstraints(resolved []resolvedRequirement) []string {
	seen := map[string]bool{}

	var out []string

	for _, entry := range resolved {
		if entry.requirement.RequiredBrand != "" {
			seen[FailureBrandMismatch] = true
		}

		if entry.requirement.MinimumQualityGrade != "" {
			seen[FailureQualityBelowFloor] = true
		}

		if entry.requirement.Substitution == AllowExactOnly {
			seen[FailureSubstitutionDenied] = true
		}

		// A requirement with exactly one admissible option has no cheaper
		// alternative to find, whatever the budget.
		if len(entry.admissible) == 1 {
			seen["ONLY_ONE_AVAILABLE_OPTION"] = true
		}
	}

	for constraint := range seen {
		out = append(out, constraint)
	}

	sort.Strings(out)

	return out
}
