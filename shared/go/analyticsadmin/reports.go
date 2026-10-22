package analyticsadmin

// Report is a finite contract, never a caller-selected owner URL or SQL query.
type Report struct {
	Name, Permission, Contract string
	Keys                       []string
}

func Reports() []Report {
	return []Report{
		{"mission-success", MissionSuccessPermission, "analytics.scoped-mission-success.v1", []string{"completed", "failed", "cancelled", "guarantee_breaches"}},
		{"mission-outcomes", "analytics.scoped.mission-outcomes.read", "analytics.scoped-mission-outcomes.v1", []string{"committed", "assigned", "completed", "failed", "cancelled", "recovery_resolved", "recovery_failed", "guarantee_breaches"}},
		{"recovery", "analytics.scoped.recovery.read", "analytics.scoped-recovery.v1", []string{"started", "replaced", "resolved", "failed"}},
		{"capability-mismatches", "analytics.scoped.capability-mismatches.read", "analytics.scoped-capability-mismatches.v1", []string{"mismatches"}},
		{"declines", "analytics.scoped.declines.read", "analytics.scoped-declines.v1", []string{"declines", "safety_declines"}},
		{"demand", "analytics.scoped.demand.read", "analytics.scoped-demand.v1", []string{"requests", "unmatched"}},
		{"demand-forecast", "analytics.scoped.demand-forecast.read", "analytics.scoped-demand-forecast.v1", []string{"forecast_requests"}},
		{"supply", "analytics.scoped.supply.read", "analytics.scoped-supply.v1", []string{"available_providers", "busy_providers"}},
		{"provider-quality", "analytics.scoped.provider-quality.read", "analytics.scoped-provider-quality.v1", []string{"completed", "failed", "cancelled", "arrivals", "declines", "safety_declines", "recoveries_as_original", "recoveries_as_replacement"}},
		{"readiness", "analytics.scoped.readiness-components.read", "analytics.scoped-readiness-components.v1", []string{"ready_count", "on_time_count", "late_count", "lateness_seconds_total", "prep_seconds_total", "prep_count"}},
	}
}
func ReportNamed(name string) (Report, bool) {
	for _, r := range Reports() {
		if r.Name == name {
			return r, true
		}
	}
	return Report{}, false
}
func (r Report) Path() string { return "/internal/analytics/v1/admin/scoped-network/" + r.Name }
func (r Report) ValidSourceContract(value string) bool {
	for _, field := range []string{"provider_id", "original_provider_id", "replacement_provider_id", "branch_id", "source_entity_id"} {
		if value == r.Contract+":"+field {
			return true
		}
	}
	return false
}
