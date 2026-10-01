package integration_test

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/payment-wallet-service/internal/events"
	"cosmicforge/logistics/shared/go/outboxmetrics"
)

// The configurations this service compiles in must match the tables its own
// migrations create.
//
// This assertion belongs HERE and not in shared/go: shared/go owns no schema,
// so a test placed there can only reach finance_outbox and
// managed_settlements by borrowing this service's database — which is how it
// passed locally against a populated database and then failed in CI against
// an empty one. Run against the migrated schema, a drifted column name fails
// the build instead of producing a live service whose metrics are silently
// always missing.
func TestMetricsSourcesMatchTheMigratedTables(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	sources := events.MetricsSources()
	if len(sources) != 2 {
		t.Fatalf("expected the delivery queue and the settlement table, got %d sources", len(sources))
	}
	for _, source := range sources {
		t.Run(source.Config.Table, func(t *testing.T) {
			if err := source.Config.Valid(); err != nil {
				t.Fatalf("configuration is invalid: %v", err)
			}
			if _, err := outboxmetrics.Collect(ctx, pool, source.Config); err != nil {
				t.Fatalf("collect against the migrated table: %v", err)
			}
			if _, _, err := outboxmetrics.CollectStates(ctx, pool, source.Config, source.Unresolved); err != nil {
				t.Fatalf("collect states against the migrated table: %v", err)
			}
		})
	}

	// managed_settlements is the money itself and has no delivery queue, so
	// it must be reported states-only: a pending count of zero on a table
	// that has no queue would read as "drained" rather than "not measured".
	for _, source := range sources {
		if source.Config.Table == "managed_settlements" && !source.Config.StatesOnly() {
			t.Fatal("managed_settlements must be reported states-only")
		}
	}
}
