package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommerceRefundPolicyMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000099_commerce_refund_policy_decisions.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{
		"commerce_refund_policy_decisions", "refund_policy_version", "policy_version",
		"idempotency_key text not null unique", "support_case_ref", "pending_liability",
		"projection_applied boolean not null default false",
		"platform_loss_funding_minor", "authorization_release_minor",
		"prevent_commerce_refund_policy_evidence_mutation", "delivery_failed",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("commerce refund-policy migration missing %q", required)
		}
	}
}
