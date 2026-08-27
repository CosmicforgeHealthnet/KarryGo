package walletrepositories

import "testing"

func TestDeliveryResponsibilityFingerprintBindsOperation(t *testing.T) {
	base := TransitionDeliveryResponsibilityInput{VendorOrderID: "vo_1", OperationRef: "fallback:1", IdempotencyKey: "fallback:1", ReasonCode: "VENDOR_UNAVAILABLE", PlatformExecutionCostMinor: 1_200}
	first, _ := stableFingerprint(base)
	base.VendorOrderID = "vo_2"
	second, _ := stableFingerprint(base)
	if first == second {
		t.Fatal("transition fingerprint does not bind vendor order")
	}
	base.VendorOrderID = "vo_1"
	base.PlatformExecutionCostMinor++
	third, _ := stableFingerprint(base)
	if first == third {
		t.Fatal("transition fingerprint does not bind platform execution cost")
	}
}
