package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommerceAnalyticsMigrationShape(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "001_commerce_analytics_reporting.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	required := []string{
		"analytics_commerce_events",
		"PRIMARY KEY",
		"analytics_vendor_daily_metrics",
		"analytics_vendor_reliability_scores",
		"analytics_order_guarantee_policy_models",
		"FUTURE_DISABLED",
		"idx_analytics_commerce_events_source_version",
	}
	for _, token := range required {
		if !strings.Contains(sql, token) {
			t.Fatalf("migration missing %s", token)
		}
	}
	if strings.Contains(sql, "customer_email") || strings.Contains(sql, "competitor_id") {
		t.Fatal("migration must not persist customer PII or competitor identity")
	}
}
