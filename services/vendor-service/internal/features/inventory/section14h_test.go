package inventory

import "testing"

func TestSection14HEventVocabulary(
	t *testing.T,
) {
	required :=
		[]string{
			EventInventoryReservationHeld,
			EventInventoryReservationConfirmed,
			EventInventoryReservationCommitted,
			EventInventoryReservationReleased,
			EventInventoryReservationExpired,
			EventInventoryReceiptPosted,
			EventInventoryAdjusted,
			EventInventoryQuarantined,
			EventInventoryQuarantineReleased,
			EventInventoryTransferDispatched,
			EventInventoryTransferReceived,
			EventInventoryStocktakePosted,
			EventInventoryLowStock,
			EventInventoryLotExpired,
		}

	for _, value := range required {

		if value == "" {
			t.Fatal(
				"empty event type",
			)
		}
	}

	if EventInventoryOutOfStock !=
		EventInventoryItemOutOfStock {

		t.Fatal(
			"14G out-of-stock event convention changed",
		)
	}

	if EventInventoryRestocked !=
		EventInventoryItemAvailable {

		t.Fatal(
			"14G available/restocked event convention changed",
		)
	}
}

func TestSection14HIAMCompatibility(
	t *testing.T,
) {
	cases :=
		map[string]string{
			"vendor.shop.inventory.read": PermissionInventoryRead,

			"vendor.shop.inventory.receive": PermissionInventoryReceive,

			"vendor.shop.inventory.adjust": PermissionInventoryAdjust,

			"vendor.shop.inventory.reserve": PermissionInventoryReserve,

			"vendor.shop.inventory.transfer": PermissionInventoryTransfer,

			"vendor.shop.inventory.stocktake": PermissionInventoryStocktake,

			"vendor.shop.inventory.quarantine": PermissionInventoryQuarantine,

			"vendor.shop.inventory.manage_policy": PermissionInventoryPolicy,
		}

	for shopPermission, want := range cases {

		got,
			ok :=
			GenericPermissionForShopPermission(
				shopPermission,
			)

		if !ok {
			t.Fatalf(
				"missing %s",
				shopPermission,
			)
		}

		if got != want {
			t.Fatalf(
				"%s -> %s want %s",
				shopPermission,
				got,
				want,
			)
		}
	}
}
