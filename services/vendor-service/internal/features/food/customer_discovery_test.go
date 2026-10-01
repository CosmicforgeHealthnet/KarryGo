package food

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCustomerCatalogueNeverSerializesInternalOperationalEvidence(t *testing.T) {
	item := DiscoveryItem{Ref: "meal", Name: "Meal", Price: &ResolvedPrice{Currency: "NGN", EffectiveUnitPriceMinor: 123, BranchOverrideRef: "private-price-override"}, Sellability: Sellability{Sellable: true, Detail: "private-detail", MatchedScheduleRef: "private-schedule", OperationalStatus: "private-operations"}, Variants: []VariantView{{ID: "variant", Name: "Regular", Version: 99}}, ModifierGroups: []DiscoveryModifierGroup{{Group: ModifierGroupView{ID: "group", Name: "Options", Version: 88}, Modifiers: []ModifierView{{ID: "modifier", Name: "Extra", Version: 77}}}}}
	section := DiscoverySection{Items: []DiscoveryItem{item}}
	snapshot := DiscoverySnapshot{Menus: []DiscoveryMenu{{Sections: []DiscoverySection{section}}}}
	raw, err := json.Marshal(customerCatalogueView(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-", "branch_override", "operational_status", "matched_schedule", "archived_at", "created_at", "version", "logistics"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("internal field leaked: %s", forbidden)
		}
	}
	if !strings.Contains(string(raw), "effective_unit_price_minor") || !strings.Contains(string(raw), "modifier_groups") {
		t.Fatal("public ordering evidence missing")
	}
}
