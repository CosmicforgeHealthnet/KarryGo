package walletmodels

import "testing"

func validCommerceAllocation() CommerceAllocation {
	return CommerceAllocation{
		AllocationReference: "vendor-order:cvo_a", Scope: CommerceAllocationVendor,
		VendorOrderID: "cvo_a", VendorID: "vendor_a", BranchID: "branch_a",
		DeliveryResponsibility: DeliveryResponsibilityPlatform, Currency: "NGN",
		CustomerPayableMinor: 8_000, MaxAuthorizedMinor: 9_000,
		PlatformProductDiscountMinor: 500, PlatformDeliverySubsidyMinor: 500,
		VendorMerchandisePayableMinor: 7_000, ProviderDeliveryPayableMinor: 1_000, PlatformRevenueMinor: 1_000,
		SnapshotFingerprint: "snapshot-a",
	}
}

func TestCommerceAllocationConservesFundingAndSeparatesPlatformDelivery(t *testing.T) {
	allocation := validCommerceAllocation()
	if err := allocation.Validate("NGN"); err != nil {
		t.Fatalf("valid allocation: %v", err)
	}
	if allocation.OutputMinor() != allocation.CustomerPayableMinor+allocation.ExternalFundingMinor() {
		t.Fatal("allocation does not conserve money")
	}
	if allocation.VendorDeliveryPayableMinor != 0 || allocation.ProviderDeliveryPayableMinor == 0 {
		t.Fatal("platform delivery leaked into Vendor payable")
	}
}

func TestCommerceAllocationRejectsCrossBeneficiaryAndMaximumErrors(t *testing.T) {
	allocation := validCommerceAllocation()
	allocation.VendorDeliveryPayableMinor = 1
	if allocation.Validate("NGN") == nil {
		t.Fatal("platform delivery accepted a Vendor delivery payable")
	}
	allocation = validCommerceAllocation()
	allocation.MaxAuthorizedMinor = allocation.CustomerPayableMinor - 1
	if allocation.Validate("NGN") == nil {
		t.Fatal("maximum below final amount was accepted")
	}
}

func TestVendorOwnedDeliveryCannotCreateProviderPayable(t *testing.T) {
	allocation := validCommerceAllocation()
	allocation.DeliveryResponsibility = DeliveryResponsibilityVendor
	if allocation.Validate("NGN") == nil {
		t.Fatal("vendor-owned delivery accepted a provider payable")
	}
}
