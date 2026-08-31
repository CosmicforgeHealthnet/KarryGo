package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection21CommerceMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000093_scheduled_group_rfq_commerce.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"commerce_scheduled_orders", "desired_delivery_at", "production_ready_by", "capacity_reservation_id",
		"commerce_group_sessions", "invite_token_hash", "commerce_group_selections", "commerce_group_attributions",
		"commerce_rfqs", "DRAFT", "QUOTED", "ACCEPTED", "EXPIRED", "commerce_rfq_quote_revisions",
		"idempotency_key",
		"commerce_rfq_checkout_bindings", "binding_digest", "accepted_revision",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("000093 migration missing %q", required)
		}
	}
}

func TestSection21Prompt2MigrationShape(t *testing.T) {
	for name, required := range map[string][]string{
		"000094_rfq_payment_policy_deposits.up.sql": {"payment_policy", "cancellation_policy", "commerce_rfq_payment_obligations", "wallet_idempotency_key", "required_minor"},
		"000095_corporate_recurring_feeding.up.sql": {"commerce_recurring_schedules", "weekdays", "timezone", "headcount", "allowance_minor", "commerce_recurring_occurrences", "occurrence_key", "commerce_recurring_order_proposals"},
	} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range required {
			if !strings.Contains(string(raw), needle) {
				t.Fatalf("%s missing %q", name, needle)
			}
		}
	}
}
