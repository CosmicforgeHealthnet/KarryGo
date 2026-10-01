package completedwork

import (
	"time"

	"cosmicforge/logistics/shared/go/outboxmetrics"
)

// MetricsSource describes this service's outbox to the shared collector.
//
// It lives here, next to the table's own writer, rather than inline in
// cmd/main.go, because a column name that drifts from the migration produces
// a query that fails only at runtime on a live service — and shows up as
// permanently missing metrics, which is the exact failure this reporting
// exists to remove. Keeping it in a package with a test suite lets that
// mismatch fail the build instead.
func MetricsSource() outboxmetrics.Source {
	return outboxmetrics.Source{
		Config: outboxmetrics.Config{
			Service: "driver-hauling-service", Table: "hauling_domain_outbox",
			Target: Target, TargetColumn: "target",
			CreatedColumn: "occurred_at", DeliveredColumn: "delivered_at",
			DeadLetterColumn: "dead_lettered_at", AttemptsColumn: "attempts",
			ReconciliationColumn: "reconciliation_state",
		},
		// A state not listed here is still counted and labelled; it simply
		// does not set the oldest-case age. A delivered row from last month
		// is not a backlog, and letting it set the age would leave the metric
		// permanently red and permanently ignored.
		Unresolved: []string{"PENDING", "RETRY_SCHEDULED", "RECONCILIATION_REQUIRED"},
	}
}

// MetricsInterval is how often the reporter re-reads the outbox.
const MetricsInterval = 30 * time.Second
