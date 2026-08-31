package basket

// The optimizer.
//
// Given a list of things a customer needs, produce the best basket that can
// actually be bought and delivered - and, when none fits their budget, say
// exactly how far short it falls and which constraint is holding it there.
//
// # Why the search is bounded
//
// Choosing one listing per requirement across a catalogue is a product of
// choices: twelve candidates for each of twelve requirements is more baskets
// than there are grains of sand worth counting. An unbounded search would not
// be thorough, it would be a way for one HTTP request to occupy a server
// indefinitely.
//
// So the search is bounded and the bounds are stated in the answer. The
// consequence is admitted rather than hidden: this finds the best valid
// candidate under the configured search. For most inputs it has not looked at
// every basket, so it never claims to have found the provably cheapest one.
//
// # How the bound is spent well
//
// Not by sampling, which would make the answer non-deterministic, but by
// exploiting the structure of the actual problem. Merchandise price is
// SEPARABLE across requirements - the cheapest tin of tomatoes is the
// cheapest tin of tomatoes regardless of what else is in the basket - while
// delivery is NOT, because delivery is charged per vendor mission and depends
// on which vendors the whole basket touches.
//
// That gives a search worth doing:
//
//  1. Pick the per-requirement best ignoring delivery. This is optimal on
//     merchandise and is the plan a naive optimizer would stop at.
//
//  2. Then try to CONSOLIDATE onto fewer vendors. For each vendor already in
//     play, build the basket that buys everything it can from that vendor and
//     takes the cheapest available elsewhere for the rest.
//
//  3. Quote delivery for each distinct basket through the real fulfilment
//     planner, and compare DELIVERED totals.
//
// The number of baskets is therefore linear in the number of candidate
// vendors rather than exponential in the number of requirements, and the
// baskets it does evaluate are the ones where the delivery saving actually
// lives.

import (
	"context"
	"sort"
	"strings"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
)

// Search bounds.
//
// MaxEvaluatedBaskets is the one that matters. Each evaluated basket costs at
// least one round trip to a logistics authority, so it is the number that
// decides how long a request can take and how much load one customer can
// generate.
const (
	MaxEvaluatedBaskets = 8
	MaxCandidateVendors = 12
)

// selection is one requirement satisfied by one listing.
type selection struct {
	requirement Requirement
	listing     Listing
	basis       EquivalenceBasis
}

// lineTotalMinor is what this selection costs in merchandise.
func (s selection) lineTotalMinor() int64 {
	return s.listing.UnitPriceMinor * s.requirement.Quantity
}

func (s selection) branchKey() string {
	return s.listing.BusinessID + "|" + s.listing.BranchID
}

// candidateBasket is one complete, buyable basket before it has been priced
// for delivery.
type candidateBasket struct {
	objective  Objective
	selections []selection
}

// fingerprint is the basket's semantic identity: which listing satisfies
// which requirement, and how many of each.
//
// It exists to deduplicate. Two objectives very often reach the same basket -
// when one vendor is both cheapest and the only vendor, VALUE and
// FEWEST_VENDORS agree - and offering that basket twice under two headings
// would be the same answer wearing two labels.
func (b candidateBasket) fingerprint() string {
	parts := make([]string, 0, len(b.selections))

	for _, item := range b.selections {
		parts = append(parts, strings.Join([]string{
			item.requirement.Ref,
			item.listing.ListingRef,
			item.listing.SellUnitRef,
		}, ":"))
	}

	sort.Strings(parts)

	return strings.Join(parts, "|")
}

func (b candidateBasket) merchandiseMinor() int64 {
	var total int64

	for _, item := range b.selections {
		total += item.lineTotalMinor()
	}

	return total
}

func (b candidateBasket) vendorCount() int {
	seen := map[string]bool{}

	for _, item := range b.selections {
		seen[item.branchKey()] = true
	}

	return len(seen)
}

// resolvedRequirement is one requirement with every candidate that could
// satisfy it, and the reasons the others could not.
type resolvedRequirement struct {
	requirement Requirement

	// admissible are the listings that passed every HARD constraint, ordered
	// cheapest first. Only these are ever put in a basket.
	admissible []selection

	// blockingFailures are the constraint failures seen while filtering, most
	// informative first.
	//
	// They are the entire content of a truthful no-solution answer. Without
	// them "no basket fits your budget" is a dead end; with them it is "the
	// brand you require is only stocked by one vendor, and it is out of
	// stock".
	blockingFailures []string
}

// satisfiable reports whether anything at all can satisfy this requirement.
func (r resolvedRequirement) satisfiable() bool {
	return len(r.admissible) > 0
}

// cheapest is the lowest merchandise cost for this requirement, ignoring
// delivery.
//
// It is what the minimum-feasible calculation is built from: no basket can
// cost less on merchandise than the sum of each requirement's cheapest
// admissible option, because the options are chosen independently.
func (r resolvedRequirement) cheapest() (selection, bool) {
	if len(r.admissible) == 0 {
		return selection{}, false
	}

	return r.admissible[0], true
}

// resolve turns requirements into admissible candidates.
//
// Every listing comes from the catalogue authority and is already active,
// sellable and priced. What happens here is the CUSTOMER's constraints, which
// the catalogue knows nothing about: their brand, their quality floor, their
// substitution allowance, their quantity.
func (s *Service) resolve(
	ctx context.Context,
	request Request,
	originals map[string]Listing,
) ([]resolvedRequirement, error) {
	resolved := make([]resolvedRequirement, 0, len(request.Requirements))

	for _, requirement := range request.Requirements {
		listings, err := s.catalogue.FindCandidates(ctx, CandidateQuery{
			Requirement: requirement,
			Currency:    request.Currency,
			Destination: request.Destination,
			Limit:       MaxCandidateListingsPerNeed,
		})
		if err != nil {
			return nil, err
		}

		entry := resolvedRequirement{requirement: requirement}

		failures := map[string]bool{}

		var original *Listing

		if found, ok := originals[requirement.Ref]; ok {
			copied := found
			original = &copied
		}

		for _, listing := range listings {
			basis, failure := satisfies(
				requirement, listing, original, request.Currency,
			)

			if failure != "" {
				failures[failure] = true

				continue
			}

			entry.admissible = append(entry.admissible, selection{
				requirement: requirement,
				listing:     listing,
				basis:       basis,
			})
		}

		sortSelections(requirement, entry.admissible)

		// A requirement nothing satisfied but which the catalogue returned
		// nothing for at all is unmet for a different reason than one whose
		// candidates were all refused. Saying "nothing is listed" when the
		// truth is "everything listed is the wrong brand" sends the customer
		// to look for a product that is right in front of them.
		if len(entry.admissible) == 0 && len(listings) == 0 {
			failures[FailureNoEquivalence] = true
		}

		entry.blockingFailures = sortedFailures(failures)

		resolved = append(resolved, entry)
	}

	return resolved, nil
}

// sortSelections orders a requirement's admissible candidates.
//
// Cheapest first, because merchandise price is the only thing comparable
// before delivery is known. Soft brand preference breaks price ties, and the
// listing reference breaks everything else - a total order, so the same
// catalogue always produces the same basket.
func sortSelections(requirement Requirement, selections []selection) {
	sort.SliceStable(selections, func(i, j int) bool {
		left, right := selections[i], selections[j]

		leftTotal, rightTotal := left.lineTotalMinor(), right.lineTotalMinor()

		if leftTotal != rightTotal {
			return leftTotal < rightTotal
		}

		leftRank := preferredBrandRank(requirement, left.listing)
		rightRank := preferredBrandRank(requirement, right.listing)

		if leftRank != rightRank {
			return leftRank < rightRank
		}

		return left.listing.ListingRef < right.listing.ListingRef
	})
}

func sortedFailures(failures map[string]bool) []string {
	// Most informative first: a customer told "wrong brand" knows to relax a
	// constraint, a customer told "out of stock" knows to wait, and a
	// customer told "no equivalence" knows the platform simply cannot map
	// what they asked for.
	order := []string{
		FailureBrandMismatch,
		FailureQualityBelowFloor,
		FailureUnitIncompatible,
		FailureSubstitutionDenied,
		FailureInsufficientStock,
		FailureCurrencyMismatch,
		FailureNoEquivalence,
	}

	out := make([]string, 0, len(failures))

	for _, failure := range order {
		if failures[failure] {
			out = append(out, failure)
		}
	}

	return out
}

// buildBaskets enumerates the baskets worth evaluating.
//
// The count is bounded by MaxEvaluatedBaskets. The second return value
// reports whether the bound actually bit, which is carried into the answer -
// a truncated search that says so is a bounded search, and one that does not
// is a false claim of thoroughness.
func buildBaskets(
	resolved []resolvedRequirement,
	objectives []Objective,
) ([]candidateBasket, bool) {
	var baskets []candidateBasket

	seen := map[string]bool{}

	add := func(basket candidateBasket) bool {
		if len(basket.selections) != len(resolved) {
			return true
		}

		key := basket.fingerprint()

		if seen[key] {
			return true
		}

		if len(baskets) >= MaxEvaluatedBaskets {
			return false
		}

		seen[key] = true
		baskets = append(baskets, basket)

		return true
	}

	truncated := false

	// 1. The merchandise-optimal basket. Optimal on goods, and usually not on
	//    delivery, which is the whole reason the next step exists.
	if basket, ok := cheapestBasket(resolved); ok {
		if !add(candidateBasket{
			objective: ObjectiveValue, selections: basket,
		}) {
			truncated = true
		}
	}

	// 2. Consolidation candidates: for each vendor in play, the basket that
	//    buys as much as possible from that one vendor.
	//
	//    This is where the delivery saving actually lives. Fewer vendors
	//    means fewer missions, and one mission covering six items is almost
	//    always cheaper delivered than six missions covering one each - even
	//    when each individual item costs slightly more.
	for _, branch := range candidateVendors(resolved) {
		if truncated {
			break
		}

		basket, ok := basketAnchoredOn(resolved, branch)

		if !ok {
			continue
		}

		if !add(candidateBasket{
			objective: ObjectiveFewestVendors, selections: basket,
		}) {
			truncated = true
		}
	}

	// 3. The quality-led basket, ONLY where the catalogue carries a quality
	//    signal to lead with. Where it does not, no variant is produced and
	//    the caller reports why - an invented quality ranking presented to a
	//    shopper is advice with nothing behind it.
	if wantsObjective(objectives, ObjectiveQualityBalanced) {
		if basket, ok := qualityBasket(resolved); ok && !truncated {
			if !add(candidateBasket{
				objective: ObjectiveQualityBalanced, selections: basket,
			}) {
				truncated = true
			}
		}
	}

	return baskets, truncated
}

func wantsObjective(objectives []Objective, want Objective) bool {
	for _, objective := range objectives {
		if objective == want {
			return true
		}
	}

	return false
}

// cheapestBasket takes each requirement's cheapest admissible option.
func cheapestBasket(resolved []resolvedRequirement) ([]selection, bool) {
	basket := make([]selection, 0, len(resolved))

	for _, entry := range resolved {
		best, ok := entry.cheapest()

		if !ok {
			return nil, false
		}

		basket = append(basket, best)
	}

	return basket, true
}

// candidateVendors lists the branches worth anchoring a basket on, most
// broadly stocked first.
//
// Ordering by coverage rather than by price is deliberate: a vendor that can
// supply eight of ten requirements can remove a lot of delivery cost even if
// none of its individual prices is the lowest, and that trade is exactly what
// the delivered-total comparison is for.
func candidateVendors(resolved []resolvedRequirement) []string {
	coverage := map[string]int{}

	for _, entry := range resolved {
		seen := map[string]bool{}

		for _, option := range entry.admissible {
			key := option.branchKey()

			if seen[key] {
				continue
			}

			seen[key] = true
			coverage[key]++
		}
	}

	branches := make([]string, 0, len(coverage))

	for branch := range coverage {
		branches = append(branches, branch)
	}

	sort.SliceStable(branches, func(i, j int) bool {
		if coverage[branches[i]] != coverage[branches[j]] {
			return coverage[branches[i]] > coverage[branches[j]]
		}

		// A total order. Map iteration is random in Go, and a candidate list
		// built from it would make the whole optimizer non-deterministic in a
		// way no test would reliably catch.
		return branches[i] < branches[j]
	})

	if len(branches) > MaxCandidateVendors {
		branches = branches[:MaxCandidateVendors]
	}

	return branches
}

// basketAnchoredOn buys everything it can from one branch and takes the
// cheapest available elsewhere for the rest.
func basketAnchoredOn(
	resolved []resolvedRequirement,
	branch string,
) ([]selection, bool) {
	basket := make([]selection, 0, len(resolved))

	anchored := false

	for _, entry := range resolved {
		var chosen *selection

		for index := range entry.admissible {
			if entry.admissible[index].branchKey() == branch {
				chosen = &entry.admissible[index]
				anchored = true

				break
			}
		}

		if chosen == nil {
			fallback, ok := entry.cheapest()

			if !ok {
				return nil, false
			}

			basket = append(basket, fallback)

			continue
		}

		basket = append(basket, *chosen)
	}

	// A branch that supplied nothing produces the cheapest basket over again.
	// It is deduplicated downstream anyway; refusing it here saves the work.
	if !anchored {
		return nil, false
	}

	return basket, true
}

// qualityBasket takes the highest-graded admissible option per requirement.
//
// It is produced only when EVERY requirement has at least one candidate with
// an authoritative quality signal. A "quality balanced" basket where half the
// lines were chosen on quality and half on price is not quality balanced; it
// is a price basket with a misleading name.
//
// Where the signal exists, "highest" is the count of stated grades and then
// price - not a ranking of the grade VALUES, because the platform holds no
// ordering over them and inventing one would be the fabrication this whole
// variant is gated to avoid.
func qualityBasket(resolved []resolvedRequirement) ([]selection, bool) {
	basket := make([]selection, 0, len(resolved))

	for _, entry := range resolved {
		var chosen *selection

		for index := range entry.admissible {
			option := entry.admissible[index]

			if !hasQualitySignal(option.listing) {
				continue
			}

			if chosen == nil {
				chosen = &entry.admissible[index]

				continue
			}

			if len(option.listing.QualityGrades) >
				len(chosen.listing.QualityGrades) {
				chosen = &entry.admissible[index]
			}
		}

		if chosen == nil {
			return nil, false
		}

		basket = append(basket, *chosen)
	}

	return basket, true
}

// quoteBasket prices one basket's delivery through the quote-only planner.
//
// This is the step a naive optimizer skips, and skipping it is what makes a
// naive optimizer wrong. Delivery is most of the difference between two
// baskets of the same groceries, and estimating it as a fixed fee per vendor
// would decide the comparison on a number nobody stands behind.
func (s *Service) quoteBasket(
	ctx context.Context,
	request Request,
	basket candidateBasket,
) (fulfilment.QuoteOnlyResult, error) {
	type groupAccumulator struct {
		businessID string
		branchID   string
		merch      int64
		lines      []fulfilment.LineLogistics
	}

	groups := map[string]*groupAccumulator{}

	var order []string

	for index, item := range basket.selections {
		key := item.branchKey()

		if _, ok := groups[key]; !ok {
			groups[key] = &groupAccumulator{
				businessID: item.listing.BusinessID,
				branchID:   item.listing.BranchID,
			}

			order = append(order, key)
		}

		group := groups[key]
		group.merch += item.lineTotalMinor()

		group.lines = append(group.lines, fulfilment.LineLogistics{
			CheckoutLineID: basketLineRef(index, item),
			Domain:         "SHOP",
			BusinessID:     item.listing.BusinessID,
			BranchID:       item.listing.BranchID,
			ItemRef:        item.listing.ProductRef,
			VariantRef:     item.listing.VariantRef,
			ListingRef:     item.listing.ListingRef,
			SellUnitRef:    item.listing.SellUnitRef,
			Quantity:       item.requirement.Quantity,
		})
	}

	// Group order follows first appearance in the basket, which is itself
	// deterministic. Ranging the map directly would make two identical
	// requests produce different quote references.
	sort.Strings(order)

	quoteGroups := make([]fulfilment.QuoteOnlyGroup, 0, len(order))

	for _, key := range order {
		group := groups[key]

		quoteGroups = append(quoteGroups, fulfilment.QuoteOnlyGroup{
			Ref:              key,
			BusinessID:       group.businessID,
			BranchID:         group.branchID,
			MerchandiseMinor: group.merch,
			Currency:         request.Currency,
			Lines:            group.lines,
		})
	}

	return s.quoter.QuotePlan(ctx, fulfilment.QuoteOnlyRequest{
		CustomerID: request.CustomerID,
		Currency:   request.Currency,
		Destination: fulfilment.Destination{
			Latitude:  request.Destination.Latitude,
			Longitude: request.Destination.Longitude,
			Address:   request.Destination.Address,
		},
		Mode:   basketMode(request),
		Groups: quoteGroups,
	})
}

func basketMode(request Request) fulfilment.Mode {
	mode, ok := fulfilment.ParseMode(request.FulfilmentPreference)

	if !ok {
		return fulfilment.DefaultMode
	}

	return mode
}

func basketLineRef(index int, item selection) string {
	return item.requirement.Ref + "#" + item.listing.ListingRef +
		"#" + itoa(index)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}

	var digits []byte

	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}

	return string(digits)
}
