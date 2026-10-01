package handover

import "testing"

func TestHaulingSecureHandoverReportsAvailableWithoutSecrets(t *testing.T) {
	readiness := HaulingSecureHandover()
	if readiness.Dependency != "HAULING_SECURE_HANDOVER" ||
		readiness.OTPStatus != StatusAvailable || readiness.QRStatus != StatusAvailable ||
		readiness.Status != StatusAvailable || readiness.Activation != "READY" ||
		!readiness.Available() || readiness.Reason != "" {
		t.Fatalf("unexpected Hauling handover readiness: %+v", readiness)
	}
}

func TestSection32AuthorityReadinessIsServiceSpecific(t *testing.T) {
	for _, ready := range []Readiness{DispatchSecureHandover(), HaulingSecureHandover(), TaxiSecureHandover(), VendorOwnedSecureHandover()} {
		if !ready.Available() {
			t.Fatalf("expected available authority: %+v", ready)
		}
	}
}
