package cart

import (
	"errors"
	"testing"
)

func TestParseDomainHardBoundary(t *testing.T) {
	t.Parallel()

	food, err := ParseDomain(" food ")

	if err != nil || food != DomainFood {
		t.Fatalf("food parse = %q err=%v", food, err)
	}

	shop, err := ParseDomain("SHOP")

	if err != nil || shop != DomainShop {
		t.Fatalf("shop parse = %q err=%v", shop, err)
	}

	if _, err = ParseDomain("mixed"); !errors.Is(err, ErrInvalidDomain) {
		t.Fatalf("invalid domain err=%v", err)
	}
}

func TestGroupShopCartKeepsVendorBranchGroupsSeparate(t *testing.T) {
	t.Parallel()

	cart := Cart{
		Domain: DomainShop,
		Items: []CartItem{
			{
				ID:         "i3",
				Domain:     DomainShop,
				BusinessID: "vendor-b",
				BranchID:   "branch-4",
			},
			{
				ID:         "i1",
				Domain:     DomainShop,
				BusinessID: "vendor-a",
				BranchID:   "branch-1",
			},
			{
				ID:         "i2",
				Domain:     DomainShop,
				BusinessID: "vendor-a",
				BranchID:   "branch-2",
			},
		},
	}

	groups, err := GroupShopCart(cart)

	if err != nil {
		t.Fatalf("GroupShopCart: %v", err)
	}

	if len(groups) != 3 {
		t.Fatalf("groups=%d want=3", len(groups))
	}

	if groups[0].BusinessID != "vendor-a" ||
		groups[0].BranchID != "branch-1" {
		t.Fatalf("first group=%+v", groups[0])
	}

	if groups[1].BusinessID != "vendor-a" ||
		groups[1].BranchID != "branch-2" {
		t.Fatalf("second group=%+v", groups[1])
	}

	if groups[2].BusinessID != "vendor-b" ||
		groups[2].BranchID != "branch-4" {
		t.Fatalf("third group=%+v", groups[2])
	}
}

func TestGroupShopCartRejectsFood(t *testing.T) {
	t.Parallel()

	_, err := GroupShopCart(
		Cart{
			Domain: DomainFood,
		},
	)

	if !errors.Is(err, ErrDomainConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateReceiverMinimalDeliveryFields(t *testing.T) {
	t.Parallel()

	receiver, err := ValidateReceiver(
		Receiver{
			Name:  " Ada Obi ",
			Phone: " +2348011111111 ",
		},
	)

	if err != nil {
		t.Fatalf("ValidateReceiver: %v", err)
	}

	if receiver.Name != "Ada Obi" {
		t.Fatalf("name=%q", receiver.Name)
	}

	if receiver.Phone != "+2348011111111" {
		t.Fatalf("phone=%q", receiver.Phone)
	}

	if _, err = ValidateReceiver(
		Receiver{
			Name:  "",
			Phone: "+2348011111111",
		},
	); !errors.Is(err, ErrInvalidReceiver) {
		t.Fatalf("missing name err=%v", err)
	}
}

func TestValidateAddressSnapshot(t *testing.T) {
	t.Parallel()

	address, err := ValidateAddressSnapshot(
		AddressSnapshot{
			SourceAddressRef: " location-1 ",
			Title:            " Home ",
			Address:          " 23 Akeju Victoria Island ",
			Latitude:         6.43,
			Longitude:        3.42,
		},
	)

	if err != nil {
		t.Fatalf("ValidateAddressSnapshot: %v", err)
	}

	if address.SourceAddressRef != "location-1" {
		t.Fatalf("source=%q", address.SourceAddressRef)
	}

	if address.Address != "23 Akeju Victoria Island" {
		t.Fatalf("address=%q", address.Address)
	}
}

func TestValidateValidatedItemHardDomainReferences(t *testing.T) {
	t.Parallel()

	food := ValidatedItem{
		Domain:            DomainFood,
		BusinessID:        "vendor-a",
		BranchID:          "branch-a",
		ItemRef:           "food-1",
		MenuRef:           "menu-1",
		Quantity:          1,
		UnitPriceMinor:    1000,
		Currency:          "NGN",
		PriceReference:    "price-1",
		CatalogueRevision: 4,
		DisplayName:       "Jollof Rice",
		PricingBasis:      PricingBasisFixedUnit,
	}

	if _, err := ValidateValidatedItem(
		DomainFood,
		food,
	); err != nil {
		t.Fatalf("valid Food: %v", err)
	}

	mixed := food
	mixed.ListingRef = "shop-listing"

	if _, err := ValidateValidatedItem(
		DomainFood,
		mixed,
	); !errors.Is(err, ErrInvalidCatalogueItem) {
		t.Fatalf("mixed Food/Shop references err=%v", err)
	}

	shop := ValidatedItem{
		Domain:            DomainShop,
		BusinessID:        "vendor-b",
		BranchID:          "branch-b",
		ItemRef:           "product-1",
		VariantRef:        "variant-1",
		ListingRef:        "listing-1",
		SellUnitRef:       "sell-unit-1",
		Quantity:          2,
		UnitPriceMinor:    500,
		Currency:          "NGN",
		PriceReference:    "shop-price-1",
		CatalogueRevision: 8,
		DisplayName:       "Tomatoes",
		PricingBasis:      PricingBasisFixedUnit,
	}

	if _, err := ValidateValidatedItem(
		DomainShop,
		shop,
	); err != nil {
		t.Fatalf("valid Shop: %v", err)
	}
}

func TestVariableMeasureRequiresPreparationEvidence(t *testing.T) {
	t.Parallel()

	item := ValidatedItem{
		Domain:                  DomainShop,
		BusinessID:              "vendor-a",
		BranchID:                "branch-a",
		ItemRef:                 "product-1",
		VariantRef:              "variant-1",
		ListingRef:              "listing-1",
		SellUnitRef:             "unit-kg",
		Quantity:                1,
		UnitPriceMinor:          1500,
		Currency:                "NGN",
		PriceReference:          "price-kg",
		CatalogueRevision:       10,
		DisplayName:             "Fresh Fish",
		PricingBasis:            PricingBasisVariableMeasure,
		RequestedMeasureRef:     "kg",
		ExpectedMeasureQuantity: 2,
	}

	if _, err := ValidateValidatedItem(
		DomainShop,
		item,
	); err != nil {
		t.Fatalf("variable measure should validate: %v", err)
	}

	item.ExpectedMeasureQuantity = 0

	if _, err := ValidateValidatedItem(
		DomainShop,
		item,
	); !errors.Is(err, ErrInvalidCatalogueItem) {
		t.Fatalf("missing expected measure err=%v", err)
	}
}

func TestVersionConflictCarriesExpectedAndActual(t *testing.T) {
	t.Parallel()

	err := checkExpectedVersion(
		Cart{
			Version: 7,
		},
		6,
	)

	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("err=%v", err)
	}
}
