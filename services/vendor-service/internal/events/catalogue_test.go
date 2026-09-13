package events

import (
	"errors"
	"testing"
)

func TestSection44CatalogueIsCanonicalAndAuthoritySafe(t *testing.T) {
	catalogue := Section44Catalogue()
	expected := []string{
		"vendor.business.activated", "vendor.branch.status_changed", "vendor.catalog.item_changed", "vendor.inventory.stock_changed",
		"commerce.checkout.paid", "commerce.order.created", "vendor.order.accepted", "vendor.order.rejected", "vendor.order.ready",
		"commerce.fulfilment.plan_created", "commerce.fulfilment.plan_replanned", "commerce.mission.requested",
		"delivery.provider_assigned", "delivery.pickup.verified", "delivery.parcel.picked_up", "delivery.receiver.verified",
		"delivery.completed", "delivery.failed", "delivery.returned", "commerce.refund.completed", "commerce.vendor_settlement.completed",
		"commerce.quote.locked", "commerce.fulfilment.option_selected", "commerce.allocation.created",
		"commerce.substitution.proposed", "commerce.substitution.resolved", "commerce.variable_weight.finalized",
		"commerce.optimization.proposal_created", "commerce.recurring_basket.generated", "commerce.rfq.quote_submitted",
		"commerce.rfq.quote_revised", "commerce.rfq.quote_accepted", "vendor.verification.badge_changed", "vendor.reliability.updated",
		"vendor.intelligence.snapshot_updated", "commerce.budget_basket.proposal_created", "commerce.recurring_basket.approved",
		"commerce.order_guarantee.evaluated", "commerce.order_guarantee.compensation_issued",
	}
	if len(catalogue) != len(expected) {
		t.Fatalf("canonical events=%d want=%d", len(catalogue), len(expected))
	}
	for index, name := range expected {
		if catalogue[index].Name != name {
			t.Fatalf("canonical event[%d]=%q want=%q", index, catalogue[index].Name, name)
		}
	}
	if err := ValidateSection44Catalogue(catalogue); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, event := range catalogue {
		counts[event.Name]++
	}
	if counts["commerce.recurring_basket.generated"] != 1 {
		t.Fatalf("recurring basket canonical count=%d want=1", counts["commerce.recurring_basket.generated"])
	}
	for _, name := range []string{"commerce.order_guarantee.evaluated", "commerce.order_guarantee.compensation_issued"} {
		for _, event := range catalogue {
			if event.Name == name && (event.Status != CatalogueFutureDisabled || len(event.Aliases) != 0) {
				t.Fatalf("future event has active implementation: %+v", event)
			}
		}
	}
}

func TestSection44VersionGateFailsClosed(t *testing.T) {
	if err := ValidateCatalogueVersion("delivery.completed", 1); err != nil {
		t.Fatalf("supported version rejected: %v", err)
	}
	for _, input := range []struct {
		name    string
		version int
	}{
		{"delivery.completed", 2},
		{"commerce.order_guarantee.evaluated", 1},
		{"unknown.event", 1},
	} {
		if err := ValidateCatalogueVersion(input.name, input.version); !errors.Is(err, ErrUnsupportedEventVersion) {
			t.Fatalf("%s@v%d err=%v, want unsupported", input.name, input.version, err)
		}
	}
}
