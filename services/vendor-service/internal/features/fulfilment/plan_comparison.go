package fulfilment

// Candidate comparison: choosing between alternatives that are all safe.
//
// Everything unsafe or unexecutable has already been removed by
// consolidation_feasibility.go. Nothing in this file can resurrect it: the
// comparison only ever sees feasible candidates, so a cheap score literally
// cannot outrank a hard constraint. That separation is the reason the rule
// "hard constraints first, objectives second" holds structurally instead of
// depending on every future edit remembering it.
//
// The arithmetic is integer and the ordering is total. Same inputs, same
// answer, every time - including the ties, which are broken by the
// candidate's own semantic fingerprint rather than by slice order. Slice
// order is an accident of how the clustering iterated, and a decision that
// depends on it is a decision that changes when an unrelated map is ranged
// over differently.
//
// No weights, no scores, no learned model. Each mode is a lexicographic
// ordering over facts the plan already contains, which is what makes
// "why did FASTEST cost more" answerable by reading the row.

import "time"

// comparison is the outcome of ranking one set of candidates.
type comparison struct {
	// selected is the index of the winning candidate, or -1 when nothing was
	// feasible.
	selected int

	// reasons holds each losing candidate's reason code, keyed by index.
	reasons map[int]string
}

// compareCandidates ranks feasible candidates under one mode.
//
// It does not mutate the candidates. Writing the outcome back is the caller's
// job, and keeping the two apart is what lets the whole ordering be tested
// without building a plan.
func compareCandidates(
	candidates []PlanCandidate,
	mode Mode,
	policy ModePolicy,
) comparison {
	result := comparison{selected: -1, reasons: map[int]string{}}

	feasible := make([]int, 0, len(candidates))

	for index := range candidates {
		if candidates[index].Feasible() {
			feasible = append(feasible, index)
		}
	}

	if len(feasible) == 0 {
		return result
	}

	best := feasible[0]

	for _, index := range feasible[1:] {
		if betterThan(candidates[index], candidates[best], mode, policy) {
			best = index
		}
	}

	result.selected = best

	for _, index := range feasible {
		if index == best {
			continue
		}

		result.reasons[index] = losingReason(
			candidates[index], candidates[best], mode, policy,
		)
	}

	return result
}

// betterThan is the total ordering for one mode.
//
// Strictly better, never "at least as good": returning true on equality would
// make the winner depend on iteration order, which is exactly the
// non-determinism the tie-break exists to remove.
func betterThan(
	candidate, incumbent PlanCandidate,
	mode Mode,
	policy ModePolicy,
) bool {
	switch mode {
	case ModeFastest:
		return fastestBetter(candidate, incumbent, policy)
	case ModeSaver:
		return saverBetter(candidate, incumbent, policy)
	default:
		return balancedBetter(candidate, incumbent, policy)
	}
}

// fastestBetter minimises time to the last delivery.
//
// Cost is a tie-break, never an objective. A FASTEST customer has accepted
// that the fastest plan may be the dearer one - that acceptance is carried by
// the quote they approve - so letting cost outrank time here would quietly
// sell them something other than what they chose.
func fastestBetter(candidate, incumbent PlanCandidate, policy ModePolicy) bool {
	if candidate.CompletionMinutes != incumbent.CompletionMinutes {
		return candidate.CompletionMinutes < incumbent.CompletionMinutes
	}

	if candidate.DeliveryCustomerMinor != incumbent.DeliveryCustomerMinor {
		return candidate.DeliveryCustomerMinor < incumbent.DeliveryCustomerMinor
	}

	return tieBreak(candidate, incumbent)
}

// saverBetter minimises what the CUSTOMER pays.
//
// Customer cost, not gross cost. A plan whose delivery a vendor subsidises is
// genuinely cheaper for the customer even when it costs the platform more,
// and SAVER is a promise made to the customer about their own bill.
//
// Cost differences smaller than the mode's materiality threshold are treated
// as equal, so SAVER does not buy a kobo with a quarter of an hour.
func saverBetter(candidate, incumbent PlanCandidate, policy ModePolicy) bool {
	delta := incumbent.DeliveryCustomerMinor - candidate.DeliveryCustomerMinor

	if abs64(delta) > policy.MaterialCostSavingMinor {
		return delta > 0
	}

	if candidate.CompletionMinutes != incumbent.CompletionMinutes {
		return candidate.CompletionMinutes < incumbent.CompletionMinutes
	}

	return tieBreak(candidate, incumbent)
}

// balancedBetter is the default trade-off.
//
// It is stated as dominance and materiality rather than as a weighted score,
// because a weighted score answers "why" with a number nobody can argue with
// and everybody has to trust. Dominance answers it with a sentence: this plan
// was cheaper and no slower.
//
// The order is:
//
//  1. Materially cheaper and not materially slower wins.
//  2. Materially faster and not materially costlier wins.
//  3. Otherwise the simpler plan wins - fewer missions means fewer things to
//     go wrong, fewer arrivals for the customer to wait in for, and fewer
//     providers to find. That is the section's own instruction to prefer safe
//     consolidation where the trade-offs are not materially different.
func balancedBetter(candidate, incumbent PlanCandidate, policy ModePolicy) bool {
	costDelta := incumbent.DeliveryCustomerMinor - candidate.DeliveryCustomerMinor
	timeDelta := incumbent.CompletionMinutes - candidate.CompletionMinutes

	materialTimeMinutes := int(policy.MaterialTimeSaving / time.Minute)

	cheaper := costDelta > policy.MaterialCostSavingMinor
	costlier := -costDelta > policy.MaterialCostSavingMinor

	faster := timeDelta > materialTimeMinutes
	slower := -timeDelta > materialTimeMinutes

	if cheaper && !slower {
		return true
	}

	if costlier && !faster {
		return false
	}

	if faster && !costlier {
		return true
	}

	if slower && !cheaper {
		return false
	}

	// Both dimensions genuinely trade off: one plan is materially cheaper AND
	// materially slower. This is the case a weighted score is usually reached
	// for, and it is the case where a weighted score is least defensible -
	// the weight is a number nobody can argue with because nobody can explain
	// it.
	//
	// BALANCED resolves it with a rule that can be stated in a sentence: the
	// saving wins, because the time cost is already bounded. Any candidate
	// still in this comparison has passed the mode detour tolerance, so
	// "slower" here means slower by an amount BALANCED has already declared
	// acceptable. A candidate slower than that was refused outright as
	// ETA_DETOUR_EXCEEDED and never reached this function.
	if cheaper != costlier {
		return cheaper
	}

	if candidate.MissionCount != incumbent.MissionCount {
		return candidate.MissionCount < incumbent.MissionCount
	}

	if candidate.DeliveryCustomerMinor != incumbent.DeliveryCustomerMinor {
		return candidate.DeliveryCustomerMinor < incumbent.DeliveryCustomerMinor
	}

	if candidate.CompletionMinutes != incumbent.CompletionMinutes {
		return candidate.CompletionMinutes < incumbent.CompletionMinutes
	}

	return tieBreak(candidate, incumbent)
}

// tieBreak is the final, total ordering.
//
// Fewer missions, then fewer vendor groups on the road, then the semantic
// fingerprint. The fingerprint is not meaningful to a human and is not
// supposed to be - it is there so two genuinely equivalent plans resolve the
// same way on every machine and on every replan, which no meaningful
// criterion can guarantee once all the meaningful ones are exhausted.
func tieBreak(candidate, incumbent PlanCandidate) bool {
	if candidate.MissionCount != incumbent.MissionCount {
		return candidate.MissionCount < incumbent.MissionCount
	}

	if candidate.VendorGroupCount != incumbent.VendorGroupCount {
		return candidate.VendorGroupCount < incumbent.VendorGroupCount
	}

	return candidate.Fingerprint < incumbent.Fingerprint
}

// losingReason names the single dimension that actually decided against a
// candidate.
//
// "Dominated" is technically correct and almost useless to a customer.
// "Slower than the plan we chose" is an answer. Dominance is reported only
// when the candidate genuinely lost on both dimensions at once, which is the
// one case where naming a single one would be misleading.
func losingReason(
	loser, winner PlanCandidate,
	mode Mode,
	policy ModePolicy,
) string {
	// The section's own mandatory-split condition: under FASTEST, a customer
	// who accepted the faster parallel service is not offered consolidation.
	// The consolidated candidate did not lose on arithmetic - it lost because
	// the customer chose otherwise, and saying "costlier" would misattribute
	// their decision to ours.
	if mode == ModeFastest &&
		winner.Kind == CandidateSplitParallel &&
		loser.Consolidates() {
		return ReasonCustomerFastestParallel
	}

	materialTimeMinutes := int(policy.MaterialTimeSaving / time.Minute)

	slower := loser.CompletionMinutes-winner.CompletionMinutes >
		materialTimeMinutes

	costlier := loser.DeliveryCustomerMinor-winner.DeliveryCustomerMinor >
		policy.MaterialCostSavingMinor

	switch {
	case slower && costlier:
		return ReasonCandidateDominated
	case slower:
		return ReasonCandidateSlower
	case costlier:
		return ReasonCandidateCostlier
	default:
		return ReasonCandidateTieBreak
	}
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
}
