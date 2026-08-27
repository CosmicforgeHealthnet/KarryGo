package orders

import "testing"

func TestHybridFundingPreservesCustomerGuaranteeAndAllocatesGap(t *testing.T) {
	current := DeliveryFundingSnapshot{Currency: "NGN", AuthoritativeDeliveryMinor: 1500, CustomerContributionMinor: 1500}
	got, err := ReconcileHybridFunding(current, 2300, HybridFundingCombined, 3750)
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomerContributionMinor != 1500 || got.VendorSubsidyMinor != 300 || got.PlatformSubsidyMinor != 500 || got.AuthoritativeDeliveryMinor != 2300 {
		t.Fatalf("got %+v", got)
	}
}
func TestDeliveryFundingRejectsUnderAndOverFunding(t *testing.T) {
	for _, value := range []DeliveryFundingSnapshot{{Currency: "NGN", AuthoritativeDeliveryMinor: 2000, CustomerContributionMinor: 1000, VendorSubsidyMinor: 500, PlatformSubsidyMinor: 499}, {Currency: "NGN", AuthoritativeDeliveryMinor: 2000, CustomerContributionMinor: 1000, VendorSubsidyMinor: 500, PlatformSubsidyMinor: 501}} {
		if value.Validate() == nil {
			t.Fatalf("unbalanced funding accepted: %+v", value)
		}
	}
}

func TestVendorPromotionTargetsOnlySponsoringVendor(t *testing.T) {
	vendors := []VendorAllocation{{BusinessID: "biz_a", BranchID: "a", ProductSubtotalMinor: 1000}, {BusinessID: "biz_b", BranchID: "b", ProductSubtotalMinor: 1000}}
	adjustment := AdjustmentSource{AdjustmentID: "promo:v1", Type: AdjustmentDiscount, Scope: ScopeProduct, FundingSource: FundingVendor, SourceRef: "promo", AmountMinor: 400, BusinessID: "biz_a", BranchID: "a"}
	weights, _ := discountWeights(vendors, ScopeProduct)
	shares, err := attribute(vendors, adjustment, weights)
	if err != nil {
		t.Fatal(err)
	}
	if shares[0] != 400 || shares[1] != 0 {
		t.Fatalf("foreign vendor charged: %v", shares)
	}
}
