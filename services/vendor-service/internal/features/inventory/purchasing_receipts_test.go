package inventory

import "testing"

// Lot metadata is required only for genuinely tracked inventory. This is a
// separate question from whether a lot row is created, which 15D changed: every
// posted purchasing line now creates a lot, because generic Inventory allocates
// reservations and adjustments out of lots. Before that change, stock received
// for an untracked item sat on the balance, read as available, and could not be
// reserved, adjusted or sold.
func TestSection15C_TrackedReceiptRequiresLotMetadata(t *testing.T) {
	if !purchasingReceiptRequiresLotMetadata("LOT", true) {
		t.Fatal("LOT-tracked inventory did not require lot metadata")
	}

	if !purchasingReceiptRequiresLotMetadata("EXPIRY", true) {
		t.Fatal("EXPIRY-tracked inventory did not require lot metadata")
	}

	if purchasingReceiptRequiresLotMetadata("NONE", false) {
		t.Fatal("untracked inventory unexpectedly demanded a lot number from " +
			"the receiver; it gets a lot row, but the operator does not have " +
			"to name one")
	}
}

func TestSection15C_PurchasingReceiptSourceIsStable(t *testing.T) {
	if got := purchasingReceiptSource("gr_123"); got != "PURCHASING:gr_123" {
		t.Fatalf("source=%q", got)
	}
}
