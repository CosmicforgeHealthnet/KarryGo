package inventory

import "testing"

func TestSection14CPermissionCompatibility(t *testing.T) {
	tests := map[string]string{
		"vendor.shop.inventory.read": PermissionInventoryRead,

		"vendor.shop.inventory.receive": PermissionInventoryReceive,

		"vendor.shop.inventory.adjust": PermissionInventoryAdjust,

		"vendor.shop.inventory.transfer": PermissionInventoryTransfer,

		"vendor.shop.inventory.stocktake": PermissionInventoryStocktake,

		"vendor.shop.inventory.reserve": PermissionInventoryReserve,

		"vendor.shop.inventory.quarantine": PermissionInventoryQuarantine,

		"vendor.shop.inventory.manage_policy": PermissionInventoryPolicy,
	}

	for shopPermission, want := range tests {
		got, ok :=
			GenericPermissionForShopPermission(
				shopPermission,
			)

		if !ok {
			t.Fatalf(
				"missing compatibility mapping for %q",
				shopPermission,
			)
		}

		if got != want {
			t.Fatalf(
				"%q maps to %q; want %q",
				shopPermission,
				got,
				want,
			)
		}
	}
}

func TestSection14CTrackingCompatibility(t *testing.T) {
	if got := ShopTrackingMode(
		StockBasisVariableFulfilment,
		false,
	); got != TrackingVariableMeasure {
		t.Fatalf(
			"variable tracking = %q",
			got,
		)
	}

	if got := ShopTrackingMode(
		StockBasisExactConversion,
		true,
	); got != TrackingLot {
		t.Fatalf(
			"lot tracking = %q",
			got,
		)
	}

	if got := ShopTrackingMode(
		StockBasisSameUnit,
		false,
	); got != TrackingSimple {
		t.Fatalf(
			"simple tracking = %q",
			got,
		)
	}
}

func TestSection14CMeasurementFamilyCompatibility(t *testing.T) {
	tests := map[string]string{
		"COUNT": StockUnitCount,

		"MASS": StockUnitMass,

		"VOLUME": StockUnitVolume,

		"LOCAL_MEASURE": StockUnitConfiguredMeasure,
	}

	for family, want := range tests {
		got :=
			StockUnitKindFromMeasurementFamily(
				family,
			)

		if got != want {
			t.Fatalf(
				"%q = %q; want %q",
				family,
				got,
				want,
			)
		}
	}
}
