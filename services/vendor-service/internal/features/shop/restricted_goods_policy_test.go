package shop

import "testing"

func TestPharmacyMedicinePolicyRejectsExcludedProducts(t *testing.T) {
	cases := []Product{
		{Name: "Prescription medicine refill"},
		{Name: "Baby care", Description: "OTC medicinal product"},
		{Name: "Grocery item", Brand: "Community Pharmacy"},
		{Name: "Pain relief tablets", Manufacturer: "Pharmaceutical Works"},
	}
	for _, tc := range cases {
		if !IsPharmacyExcludedProduct(tc) {
			t.Fatalf("product was not pharmacy/medicine excluded: %+v", tc)
		}
	}
}

func TestPharmacyMedicinePolicyAllowsNonMedicinalHealthGoods(t *testing.T) {
	product := Product{
		Name:         "Toothbrush",
		Description:  "Personal-care grocery item",
		Brand:        "Daily Care",
		Manufacturer: "Household Goods Ltd",
	}
	if IsPharmacyExcludedProduct(product) {
		t.Fatalf("non-medicinal personal-care good was excluded: %+v", product)
	}
}

func TestPharmacyMedicineValidationRejectsBulkImportProductName(t *testing.T) {
	err := validatePharmacyMedicineExclusion("rows.name", "Prescription medicine")
	if err == nil {
		t.Fatal("pharmacy product import row was accepted")
	}
}

func TestCanonicalRestrictedGoodsPolicyRejectsGeneralProhibition(t *testing.T) {
	product := Product{Name: "Firearm ammunition bundle"}
	if !IsRestrictedGoodsProduct(product) {
		t.Fatalf("general prohibited product was accepted: %+v", product)
	}
}
