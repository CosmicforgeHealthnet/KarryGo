package inventory

import "testing"

func TestProductionPostingFingerprintIsOrderIndependent(t *testing.T) {
	first := productionPostingFingerprint("batch_1", []string{"r2", "r1"}, []ProductionActualLine{
		{InventoryItemID: "item_b", UsedQuantity: 2},
		{InventoryItemID: "item_a", UsedQuantity: 4},
	}, &ProductionOutputInput{InventoryItemID: "prepared", Quantity: 96})
	second := productionPostingFingerprint("batch_1", []string{"r1", "r2"}, []ProductionActualLine{
		{InventoryItemID: "item_a", UsedQuantity: 4},
		{InventoryItemID: "item_b", UsedQuantity: 2},
	}, &ProductionOutputInput{InventoryItemID: "prepared", Quantity: 96})
	if first != second {
		t.Fatalf("equivalent production completion requests have different fingerprints: %s != %s", first, second)
	}
}

func TestProductionPostingFingerprintChangesForDifferentActualOutput(t *testing.T) {
	base := productionPostingFingerprint("batch_1", []string{"r1"}, []ProductionActualLine{{InventoryItemID: "item_a", UsedQuantity: 4}}, &ProductionOutputInput{InventoryItemID: "prepared", Quantity: 96})
	changed := productionPostingFingerprint("batch_1", []string{"r1"}, []ProductionActualLine{{InventoryItemID: "item_a", UsedQuantity: 5}}, &ProductionOutputInput{InventoryItemID: "prepared", Quantity: 96})
	if base == changed {
		t.Fatal("different actual consumption reused the same production posting fingerprint")
	}
}
