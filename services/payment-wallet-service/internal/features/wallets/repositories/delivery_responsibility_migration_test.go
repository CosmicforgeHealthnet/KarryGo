package walletrepositories

import (
	"os"
	"strings"
	"testing"
)

func TestDeliveryResponsibilityTransitionMigrationShape(t *testing.T) {
	raw, err := os.ReadFile("../../../../migrations/000012_commerce_delivery_responsibility_transition.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"commerce_delivery_responsibility_transitions", "effective_delivery_responsibility", "effective_vendor_delivery_payable_minor", "effective_provider_delivery_payable_minor", "fallback_platform_funding_minor", "platform_execution_cost_minor", "commerce_delivery_release_effective_bound_check", "UNIQUE (allocation_id)", "customer_contribution_minor", "vendor_subsidy_minor", "platform_subsidy_minor"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
