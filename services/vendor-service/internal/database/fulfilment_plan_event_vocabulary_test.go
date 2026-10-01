package database_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFulfilmentPlanEventVocabularyMigrationIsStrictAndComplete(t *testing.T) {
	root := filepath.Join("..", "..", "migrations")
	body, err := os.ReadFile(filepath.Join(root, "000112_fulfilment_plan_event_vocabulary.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))
	for _, action := range []string{
		"commerce.fulfilment.planning_started",
		"commerce.fulfilment.group_requirement_resolved",
		"commerce.fulfilment.vendor_own_selected",
		"commerce.fulfilment.quote_requested",
		"commerce.fulfilment.quote_received",
		"commerce.fulfilment.quote_refused",
		"commerce.fulfilment.quote_unavailable",
		"commerce.fulfilment.group_unserviceable",
		"commerce.fulfilment.consolidated",
		"commerce.fulfilment.candidate_selected",
		"commerce.fulfilment.planned",
		"commerce.fulfilment.failed",
		"commerce.fulfilment.expired",
		"commerce.fulfilment.superseded",
	} {
		if !strings.Contains(sql, "'"+action+"'") {
			t.Fatalf("migration omits controlled event action %q", action)
		}
	}
	if !strings.Contains(sql, "check (action in (") {
		t.Fatal("event action constraint is not a strict allowlist")
	}

	down, err := os.ReadFile(filepath.Join(root, "000112_fulfilment_plan_event_vocabulary.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(down)), "cannot downgrade") {
		t.Fatal("down migration does not explicitly refuse unsafe evidence loss")
	}
}
