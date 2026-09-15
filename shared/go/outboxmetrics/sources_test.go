package outboxmetrics

import (
	"context"
	"testing"
)

// The configurations the services compile in must actually match their
// tables. A wrong column name is a query that fails at runtime on a live
// service and shows up as permanently missing metrics — the failure mode this
// whole package exists to remove.
func TestServiceConfigurationsMatchTheirRealTables(t *testing.T) {
	cases := []struct {
		env    string
		source Source
	}{
		{"HAULING_TEST_DATABASE_URL", Source{Config: Config{
			Service: "driver-hauling-service", Table: "hauling_domain_outbox",
			Target: "fleet-completed-work", TargetColumn: "target",
			CreatedColumn: "occurred_at", DeliveredColumn: "delivered_at",
			DeadLetterColumn: "dead_lettered_at", AttemptsColumn: "attempts",
			ReconciliationColumn: "reconciliation_state",
		}, Unresolved: []string{"PENDING", "RETRY_SCHEDULED", "RECONCILIATION_REQUIRED"}}},

		{"TAXI_TEST_DATABASE_URL", Source{Config: Config{
			Service: "driver-taxi-service", Table: "taxi_outbox",
			Target: "fleet-partner-service", TargetColumn: "target",
			CreatedColumn: "occurred_at", DeliveredColumn: "delivered_at",
			DeadLetterColumn: "dead_lettered_at", AttemptsColumn: "attempts",
			ReconciliationColumn: "reconciliation_state",
		}, Unresolved: []string{"PENDING", "RETRY_SCHEDULED", "RECONCILIATION_REQUIRED"}}},

		{"PAYMENT_WALLET_TEST_DATABASE_URL", Source{Config: Config{
			Service: "payment-wallet-service", Table: "finance_outbox",
			CreatedColumn: "created_at", DeliveredColumn: "published_at",
			StatusColumn: "status", DeadLetterStatus: "FAILED",
			AttemptsColumn: "attempts", ReconciliationColumn: "status",
		}, Unresolved: []string{"PENDING", "FAILED"}}},

		{"PAYMENT_WALLET_TEST_DATABASE_URL", Source{Config: Config{
			Service: "payment-wallet-service", Table: "managed_settlements",
			CreatedColumn: "created_at", ReconciliationColumn: "status",
		}, Unresolved: []string{"PENDING", "FAILED_RETRYABLE", "RECONCILIATION_REQUIRED"}}},
	}

	for _, tc := range cases {
		name := tc.source.Config.Service + "/" + tc.source.Config.Table
		if tc.source.Config.Target != "" {
			name += "->" + tc.source.Config.Target
		}
		t.Run(name, func(t *testing.T) {
			pool := collectPool(t, tc.env)
			ctx := context.Background()

			if err := tc.source.Config.Valid(); err != nil {
				t.Fatalf("configuration is invalid: %v", err)
			}
			if _, err := Collect(ctx, pool, tc.source.Config); err != nil {
				t.Fatalf("collect against the real table: %v", err)
			}
			if _, _, err := CollectStates(ctx, pool, tc.source.Config, tc.source.Unresolved); err != nil {
				t.Fatalf("collect states against the real table: %v", err)
			}
		})
	}
}
