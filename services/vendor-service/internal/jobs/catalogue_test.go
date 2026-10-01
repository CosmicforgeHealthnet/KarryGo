package jobs

import "testing"

func TestSection45CatalogueIsCompleteAndHonest(t *testing.T) {
	catalogue := Section45Catalogue()
	expected := []string{
		"checkout reservation expiry", "menu/product scheduler", "stock alerts", "inventory expiry", "scheduled order activation",
		"capacity release", "RFQ expiry", "order acceptance timeout", "substitution timeout", "fulfilment planner worker",
		"platform mission reconciliation", "handover integration reconciliation", "settlement reconciliation", "CRM segment refresh",
		"reliability aggregation", "search index update", "quote expiry / price-lock release", "payment reconciliation",
		"hybrid delivery fallback", "confidence ETA updater", "verification expiry/recheck", "variable-weight timeout",
		"recurring basket generator", "vendor intelligence aggregation", "search projection rebuild",
		"budget/optimization proposal expiry", "order guarantee evaluator",
	}
	if len(catalogue) != len(expected) {
		t.Fatalf("worker requirements=%d want=%d", len(catalogue), len(expected))
	}
	for index, name := range expected {
		if catalogue[index].Name != name {
			t.Fatalf("worker[%d]=%q want=%q", index, catalogue[index].Name, name)
		}
	}
	if err := ValidateSection45Catalogue(catalogue); err != nil {
		t.Fatal(err)
	}
	for _, worker := range catalogue {
		if worker.Name == "order guarantee evaluator" && worker.Status != WorkerFutureDisabled {
			t.Fatalf("Order Guarantee worker status=%s", worker.Status)
		}
	}
}
