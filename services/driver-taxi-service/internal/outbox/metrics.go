package outbox

import (
	"time"

	"cosmicforge/logistics/shared/go/outboxmetrics"
)

// MetricsInterval is how often the reporter re-reads the outbox.
const MetricsInterval = 30 * time.Second

// MetricsSources describes this service's outbox to the shared collector.
//
// One taxi_outbox serves two destinations, so each is reported separately: a
// Vendor backlog and a Fleet backlog are different incidents with different
// responders, and a combined number would hide whichever is healthy.
//
// It lives here, next to the table's own writer, rather than inline in
// cmd/main.go, because a column name that drifts from the migration produces
// a query that fails only at runtime on a live service — and shows up as
// permanently missing metrics, which is the exact failure this reporting
// exists to remove. Keeping it in a package with a test suite lets that
// mismatch fail the build instead.
func MetricsSources() []outboxmetrics.Source {
	return []outboxmetrics.Source{
		metricsSourceFor(TargetFleet),
		metricsSourceFor(TargetVendor),
	}
}

func metricsSourceFor(target string) outboxmetrics.Source {
	return outboxmetrics.Source{
		Config: outboxmetrics.Config{
			Service: "driver-taxi-service", Table: "taxi_outbox",
			Target: target, TargetColumn: "target",
			CreatedColumn: "occurred_at", DeliveredColumn: "delivered_at",
			DeadLetterColumn: "dead_lettered_at", AttemptsColumn: "attempts",
			ReconciliationColumn: "reconciliation_state",
		},
		// A state not listed here is still counted and labelled; it simply
		// does not set the oldest-case age.
		Unresolved: []string{"PENDING", "RETRY_SCHEDULED", "RECONCILIATION_REQUIRED"},
	}
}
