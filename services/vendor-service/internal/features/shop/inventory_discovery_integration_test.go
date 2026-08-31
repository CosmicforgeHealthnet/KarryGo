package shop

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestDiscoveryReturnsEffectiveInventoryPricingFreshAndHandling(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	basis := createInventoryBasis(t, f, "Discovery Fresh", 10)
	product, err := f.service.GetProduct(ctx, f.actorA, scope, basis.ProductRef)
	if err != nil {
		t.Fatal(err)
	}
	class, status := "FRESH_PRODUCE", StatusActive
	if _, err = f.service.UpdateProduct(ctx, f.actorA, scope, product.ID, UpdateProductInput{ProductClass: &class, Status: &status, ExpectedVersion: product.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ConfigureFreshAttribute(ctx, f.actorA, scope, product.ID, ConfigureFreshAttributeInput{AttributeType: AttrRipeness, Required: boolp(true), Options: []FreshOptionInput{{Value: "GREEN", DisplayName: "Green"}, {Value: "RIPE", DisplayName: "Ripe", IsDefault: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO shop_variant_handling_flags(id,business_id,variant_id,handling_flag) VALUES($1,$2,$3,'CRUSH_SENSITIVE')`, newID("svhf_"), f.actorA.VendorID, basis.VariantRef); err != nil {
		t.Fatal(err)
	}
	items, _, catalogueRevision, inventoryRevision, err := f.service.DiscoverCatalogue(ctx, f.actorA, scope, f.branchShop, DiscoveryOptions{ProductID: product.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("discovery items=%d want=1", len(items))
	}
	item := items[0]
	if !item.Sellability.Sellable || item.Sellability.Availability.PhysicalAvailable != 10 || item.Sellability.CatalogueRevision != catalogueRevision || item.Sellability.InventoryRevision != inventoryRevision {
		t.Fatalf("discovery sellability/revisions=%+v", item.Sellability)
	}
	if len(item.SellUnits) != 1 || item.SellUnits[0].RetailPriceMinor == nil || *item.SellUnits[0].RetailPriceMinor != 1000 || item.SellUnits[0].MinimumOrderQuantity != 1 {
		t.Fatalf("discovery effective sell unit=%+v", item.SellUnits)
	}
	if len(item.HandlingFlags) != 1 || item.HandlingFlags[0] != "CRUSH_SENSITIVE" {
		t.Fatalf("discovery handling=%v", item.HandlingFlags)
	}
	if len(item.FreshAttributes) != 1 || item.FreshAttributes[0].Type != AttrRipeness || len(item.FreshAttributes[0].Options) != 2 {
		t.Fatalf("discovery fresh=%+v", item.FreshAttributes)
	}

	if _, err = f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "discovery-hide-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 10}}}); err != nil {
		t.Fatal(err)
	}
	items, _, _, _, err = f.service.DiscoverCatalogue(ctx, f.actorA, scope, f.branchShop, DiscoveryOptions{ProductID: product.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("auto-hide returned an unavailable item: %+v", items)
	}
	if _, err = f.service.SetInventoryPolicy(ctx, f.actorA, scope, basis.ListingRef, SetInventoryPolicyInput{Mode: "BACKORDER", MaximumOutstandingQuantity: int64p(20), MaximumPerRequest: int64p(10)}); err != nil {
		t.Fatal(err)
	}
	items, _, _, _, err = f.service.DiscoverCatalogue(ctx, f.actorA, scope, f.branchShop, DiscoveryOptions{ProductID: product.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Sellability.Sellable || items[0].Sellability.ReasonCode != "BACKORDER_AVAILABLE" {
		t.Fatalf("backorder discovery=%+v", items)
	}
}
