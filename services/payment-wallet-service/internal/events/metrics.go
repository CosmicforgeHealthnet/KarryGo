package events

import (
	"time"

	"cosmicforge/logistics/shared/go/outboxmetrics"
)

// MetricsInterval is how often the reporter re-reads the tables.
const MetricsInterval = 30 * time.Second

// MetricsSources describes this service's durable queues to the shared
// collector.
//
// Two sources, because they answer different questions. finance_outbox is a
// delivery queue. managed_settlements is the money itself: a settlement
// sitting in RECONCILIATION_REQUIRED is a provider who has not been paid, and
// it has no delivery queue to show up in.
//
// It lives here rather than inline in cmd/main.go because a column name that
// drifts from the migration produces a query that fails only at runtime on a
// live service — and shows up as permanently missing metrics, which is the
// exact failure this reporting exists to remove. Keeping it in a package with
// a test suite lets that mismatch fail the build instead.
func MetricsSources() []outboxmetrics.Source {
	return []outboxmetrics.Source{
		{
			Config: outboxmetrics.Config{
				Service: "payment-wallet-service", Table: "finance_outbox",
				CreatedColumn: "created_at", DeliveredColumn: "published_at",
				StatusColumn: "status", DeadLetterStatus: "FAILED",
				AttemptsColumn: "attempts", ReconciliationColumn: "status",
			},
			Unresolved: []string{"PENDING", "FAILED"},
		},
		{
			// States-only: managed_settlements has no delivery queue, and a
			// pending count of zero on a table that has none would read as
			// "drained" rather than "not measured".
			Config: outboxmetrics.Config{
				Service: "payment-wallet-service", Table: "managed_settlements",
				CreatedColumn: "created_at", ReconciliationColumn: "status",
			},
			Unresolved: []string{"PENDING", "FAILED_RETRYABLE", "RECONCILIATION_REQUIRED"},
		},
	}
}
