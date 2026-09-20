package observability

import "cosmicforge/logistics/shared/go/capabilityfit"

// fitReasons is the closed vocabulary a parcel-fit outcome may be labelled
// with: the shared Appendix B catalogue, restated as a list so Bounded has
// something to check against.
//
// The closed vocabulary is not a style preference. A label taking a bike id, a
// provider id or a free-text reason turns one time series into one per entity,
// and a fleet-sized cardinality explosion takes the metrics backend down
// rather than the service.
var fitReasons = []string{
	capabilityfit.ReasonEligible,
	capabilityfit.ReasonRequiredCapabilityUnavailable,
	capabilityfit.ReasonCapabilityUnknown,
	capabilityfit.ReasonDependencyUnavailable,
	capabilityfit.ReasonCapacityExceeded,
	capabilityfit.ReasonCargoFitFailed,
	capabilityfit.ReasonWrongAsset,
	capabilityfit.ReasonAssetNotCertified,
	capabilityfit.ReasonNetworkProductUnavailable,
	capabilityfit.ReasonNoEligibleProvider,
	capabilityfit.ReasonServiceNotServiceable,
	capabilityfit.ReasonSupplyCurrentlyUnavailable,
	capabilityfit.ReasonProviderNotEligible,
}

func capabilityMode(shadow bool) string {
	if shadow {
		return "shadow"
	}
	return "enforced"
}

// CapabilityMismatch records one hard-fit decision by its primary reason.
func CapabilityMismatch(reason string, shadow bool) {
	Metrics.IncCounter("dispatch_capability_fit_total", map[string]string{
		"reason": Bounded(reason, fitReasons...),
		"mode":   capabilityMode(shadow),
	}, 1)
}

// EligibleCandidates records how many candidates survived hard fit, and how
// many were considered.
//
// Both numbers matter together: "3 eligible" is reassuring until you learn it
// was 3 of 40, at which point the fleet has a capability-data problem rather
// than a supply one.
func EligibleCandidates(considered, eligible int, shadow bool) {
	labels := map[string]string{"mode": capabilityMode(shadow)}
	Metrics.IncCounter("dispatch_match_candidates_considered_total", labels, float64(considered))
	Metrics.IncCounter("dispatch_match_candidates_eligible_total", labels, float64(eligible))
}

// CapabilityLookupFailure records a capability-store outage during matching.
// It is its own counter rather than a reason label because an outage is an
// operational alert and a physical mismatch is not.
func CapabilityLookupFailure() {
	Metrics.IncCounter("dispatch_capability_lookup_failure_total", nil, 1)
}

// supplyStatuses is the closed supply-preview vocabulary.
var supplyStatuses = []string{"AVAILABLE", "LIMITED", "UNKNOWN"}

// SupplyPreview records one preview answer and its latency.
func SupplyPreview(status, reason string, latencyMs float64) {
	labels := map[string]string{
		"status": Bounded(status, supplyStatuses...),
		"reason": Bounded(reason, fitReasons...),
	}
	Metrics.IncCounter("dispatch_supply_preview_total", labels, 1)
	Metrics.SetGauge("dispatch_supply_preview_latency_ms",
		map[string]string{"status": labels["status"]}, latencyMs)
}

// PolicySnapshotLag publishes how stale the newest local policy snapshot is.
//
// Assignment keeps working against the last snapshot when policy stops
// arriving, which is correct and also completely silent. This gauge is what
// makes it visible.
func PolicySnapshotLag(kind string, seconds float64) {
	Metrics.SetGauge("dispatch_policy_snapshot_lag_seconds",
		map[string]string{"kind": Bounded(kind, "DISPATCH_CAPABILITY", "DISPATCH_PRODUCT")}, seconds)
}

// FleetCapabilityCompleteness publishes the share of the active fleet that has
// been surveyed, 0-100. It is the number that decides whether enforcement is
// safe to switch on.
func FleetCapabilityCompleteness(percent float64) {
	Metrics.SetGauge("dispatch_fleet_capability_completeness_percent", nil, percent)
}

// ProductEligibilityChange records a certification transition.
func ProductEligibilityChange(status string) {
	Metrics.IncCounter("dispatch_product_eligibility_total", map[string]string{
		"status": Bounded(status, "PENDING", "ELIGIBLE", "SUSPENDED", "EXPIRED", "REJECTED"),
	}, 1)
}

// PolicySnapshotApplied counts locally applied control-plane policy versions.
func PolicySnapshotApplied(kind string) {
	Metrics.IncCounter("dispatch_policy_snapshot_applied_total",
		map[string]string{"kind": Bounded(kind, "DISPATCH_CAPABILITY", "DISPATCH_PRODUCT")}, 1)
}

// PolicyPublicationsPending publishes how many publications are awaiting a
// retryable read-back.
//
// It pairs with the lag gauge and answers a different question. Lag says "the
// newest policy we hold is old"; this says "we have been handed newer policy
// and cannot finish taking it". A deployment can sit at zero lag with a
// growing backlog, and that combination is exactly the silent failure worth
// alerting on.
func PolicyPublicationsPending(count float64) {
	Metrics.SetGauge("dispatch_policy_publications_pending", nil, count)
}
