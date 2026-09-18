// Package observability instruments Hauling using the shared repository
// registry, mirroring the equivalent package in Dispatch.
//
// Every label value here comes from a closed vocabulary. That is not a style
// rule: a metric label taking a truck id, a booking id or a provider id turns
// one time series into one per entity, and a fleet-sized cardinality explosion
// takes the metrics backend down rather than the service. Bounded() is the
// guard, and it is applied to every value that did not originate as a constant
// in this repository.
package observability

import (
	"cosmicforge/logistics/shared/go/capabilityfit"
	platform "cosmicforge/logistics/shared/go/observability"
)

// Metrics is Hauling's registry.
var Metrics = platform.NewMetrics()

// fitReasons is the closed vocabulary a cargo-fit outcome may be labelled
// with. It is the shared Appendix B catalogue, restated as a list so Bounded
// has something to check against.
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

func init() {
	// Pre-register every reason at zero so a dashboard shows an explicit
	// "nothing failed for this reason" rather than a missing series, which is
	// indistinguishable from a broken exporter.
	for _, reason := range fitReasons {
		for _, mode := range []string{"enforced", "shadow"} {
			Metrics.IncCounter("hauling_cargo_fit_total",
				map[string]string{"reason": reason, "mode": mode}, 0)
		}
	}
}

// Bounded collapses an unexpected value to "other" so one stray string cannot
// create an unbounded series.
func Bounded(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "other"
}

func mode(shadow bool) string {
	if shadow {
		return "shadow"
	}
	return "enforced"
}

// CargoFitOutcome records one hard-fit decision by its primary reason.
func CargoFitOutcome(reason string, shadow bool) {
	Metrics.IncCounter("hauling_cargo_fit_total", map[string]string{
		"reason": Bounded(reason, fitReasons...),
		"mode":   mode(shadow),
	}, 1)
}

// EligibleCandidates records how many candidates survived hard fit for one
// booking, and how many were considered.
//
// Both numbers matter together: "3 eligible" is reassuring until you learn it
// was 3 of 40, at which point the fleet has a capability-data problem rather
// than a supply one.
func EligibleCandidates(considered, eligible int, shadow bool) {
	labels := map[string]string{"mode": mode(shadow)}
	Metrics.IncCounter("hauling_match_candidates_considered_total", labels, float64(considered))
	Metrics.IncCounter("hauling_match_candidates_eligible_total", labels, float64(eligible))
}

// TruckLookupFailure records a capability-store outage during matching. It is
// deliberately its own counter rather than a reason label on the fit metric:
// an outage is an operational alert, a physical mismatch is not.
func TruckLookupFailure() {
	Metrics.IncCounter("hauling_truck_lookup_failure_total", nil, 1)
}

// supplyStatuses is the closed supply-preview vocabulary.
var supplyStatuses = []string{"AVAILABLE", "LIMITED", "UNKNOWN"}

// SupplyPreview records one preview answer and its latency.
func SupplyPreview(status, reason string, latencyMs float64) {
	labels := map[string]string{
		"status": Bounded(status, supplyStatuses...),
		"reason": Bounded(reason, fitReasons...),
	}
	Metrics.IncCounter("hauling_supply_preview_total", labels, 1)
	Metrics.SetGauge("hauling_supply_preview_latency_ms", map[string]string{"status": labels["status"]}, latencyMs)
}

// PolicySnapshotLag publishes how stale the newest local policy snapshot is.
//
// Matching keeps working against the last snapshot when policy stops arriving,
// which is correct and also means the failure is completely silent. This gauge
// is the only thing that makes it visible.
func PolicySnapshotLag(kind string, seconds float64) {
	Metrics.SetGauge("hauling_policy_snapshot_lag_seconds",
		map[string]string{"kind": Bounded(kind, "HAULING_CAPABILITY", "HAULING_PRODUCT")}, seconds)
}

// FleetCapabilityCompleteness publishes the share of the active fleet that has
// been surveyed, 0-100. It is the number that decides whether enforcement is
// safe to switch on.
func FleetCapabilityCompleteness(percent float64) {
	Metrics.SetGauge("hauling_fleet_capability_completeness_percent", nil, percent)
}

// RecoveryOutcome records a provider/truck replacement attempt.
func RecoveryOutcome(outcome string) {
	Metrics.IncCounter("hauling_recovery_total", map[string]string{
		"outcome": Bounded(outcome, "REPLACED", "NO_COMPATIBLE_PROVIDER", "OPERATOR_REQUIRED", "FAILED"),
	}, 1)
}

// RecoveryDuration records how long a replacement took, in seconds.
func RecoveryDuration(seconds float64) {
	Metrics.SetGauge("hauling_recovery_duration_seconds", nil, seconds)
}

// ProductEligibilityChange records a certification transition.
func ProductEligibilityChange(status string) {
	Metrics.IncCounter("hauling_product_eligibility_total", map[string]string{
		"status": Bounded(status, "PENDING", "ELIGIBLE", "SUSPENDED", "EXPIRED", "REJECTED"),
	}, 1)
}

// Render exposes the registry for a metrics endpoint.
func Render() string { return Metrics.Render() }
