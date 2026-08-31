package walletrepositories

import (
	"testing"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

func TestRefundRecoveryCannotConsumeSiblingEscrow(t *testing.T) {
	allocation := walletmodels.CommerceAllocation{MaxAuthorizedMinor: 5_000}
	if err := validateRefundRecoverySources(allocation, RefundCommerceAllocationInput{EscrowRecoveryMinor: 5_001}); err == nil {
		t.Fatal("refund recovered more than this allocation's escrow")
	}
	if err := validateRefundRecoverySources(allocation, RefundCommerceAllocationInput{EscrowRecoveryMinor: 5_000}); err != nil {
		t.Fatalf("allocation-local escrow rejected: %v", err)
	}
}

func TestRefundAfterReleaseUsesOnlyReleasedBeneficiaryBalances(t *testing.T) {
	allocation := walletmodels.CommerceAllocation{MaxAuthorizedMinor: 5_000, VendorMerchandisePayableMinor: 4_000, VendorReleasedMinor: 4_000, PlatformRevenueMinor: 1_000}
	if err := validateRefundRecoverySources(allocation, RefundCommerceAllocationInput{VendorRecoveryMinor: 4_000, PlatformRecoveryMinor: 1_000}); err != nil {
		t.Fatalf("released allocation recovery rejected: %v", err)
	}
	if err := validateRefundRecoverySources(allocation, RefundCommerceAllocationInput{VendorRecoveryMinor: 4_001, PlatformRecoveryMinor: 999}); err == nil {
		t.Fatal("vendor recovery exceeded this allocation's released payable")
	}
}

func TestRepeatedVendorDeliveryRecoveryUsesRemainingAllocationBalance(t *testing.T) {
	allocation := walletmodels.CommerceAllocation{
		MaxAuthorizedMinor: 1_000, DeliveryResponsibility: walletmodels.DeliveryResponsibilityVendor,
		VendorDeliveryPayableMinor: 1_000, DeliveryReleasedMinor: 1_000, RefundVendorRecoveryMinor: 600,
	}
	if err := validateRefundRecoverySources(allocation, RefundCommerceAllocationInput{VendorRecoveryMinor: 401}); err == nil {
		t.Fatal("second recovery exceeded the remaining Vendor-owned delivery balance")
	}
	if err := validateRefundRecoverySources(allocation, RefundCommerceAllocationInput{VendorRecoveryMinor: 400}); err != nil {
		t.Fatalf("remaining Vendor-owned delivery recovery rejected: %v", err)
	}
}
