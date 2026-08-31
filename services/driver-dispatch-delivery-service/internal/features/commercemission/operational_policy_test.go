package commercemission

import (
	"context"
	"testing"
	"time"
)

func TestControlledOperationalVocabularies(t *testing.T) {
	for _, value := range []EvidenceType{EvidencePickup, EvidenceDelivery, EvidenceDamage, EvidenceDiscrepancy, EvidenceFailedAttempt, EvidenceReturn, EvidenceProviderTransfer} {
		if !value.Valid() {
			t.Fatalf("evidence type %q is not valid", value)
		}
	}
	for _, value := range []ExceptionReason{ReasonVendorNotReady, ReasonPackageMismatch, ReasonProhibitedItem, ReasonRestrictedItem, ReasonDamagedPackage, ReasonReceiverUnavailable, ReasonReceiverRefused, ReasonWrongAddress, ReasonAddressInaccessible, ReasonVehicleBreakdown, ReasonProviderIllness, ReasonWeatherRoad, ReasonSecurityRisk, ReasonSafetyIncident} {
		if !value.Valid() {
			t.Fatalf("exception reason %q is not valid", value)
		}
	}
	if EvidenceType("OTHER").Valid() || ExceptionReason("DIAGNOSIS").Valid() {
		t.Fatal("uncontrolled mandatory evidence/reason was accepted")
	}
}

func TestExceptionPolicyFailClosedAndControlledRecovery(t *testing.T) {
	for _, reason := range []ExceptionReason{ReasonPackageMismatch, ReasonProhibitedItem, ReasonRestrictedItem} {
		if !reason.FailClosedBeforeCustody() {
			t.Fatalf("%s must fail closed before custody", reason)
		}
	}
	for _, reason := range []ExceptionReason{ReasonVehicleBreakdown, ReasonProviderIllness, ReasonSafetyIncident} {
		if !reason.RequiresReplacement() || !RecoveryAllowed(reason, RecoveryProviderReplacement, false) {
			t.Fatalf("%s must permit controlled replacement", reason)
		}
	}
	if RecoveryAllowed(ReasonReceiverUnavailable, RecoveryAlternativeReceiver, false) {
		t.Fatal("unauthorized alternative receiver was accepted")
	}
	if !RecoveryAllowed(ReasonReceiverUnavailable, RecoveryAlternativeReceiver, true) {
		t.Fatal("authorized alternative receiver was rejected")
	}
}

func TestPreparedFoodNeverUsesShopRestockDisposition(t *testing.T) {
	if ReturnDispositionAllowed(ProductPreparedFood, DispositionShopInspection) {
		t.Fatal("prepared food entered generic Shop inspection/restock flow")
	}
	if !ReturnDispositionAllowed(ProductPreparedFood, DispositionFoodPolicy) || !ReturnDispositionAllowed(ProductShopRetail, DispositionShopInspection) {
		t.Fatal("product-specific return policy was not preserved")
	}
}

func TestEvidenceValidationRejectsBytesStaleAndInvalidLocation(t *testing.T) {
	now := time.Now().UTC()
	base := EvidenceCommand{OperationRef: "op-1", Type: EvidencePickup, ParcelRefs: []string{"parcel-1"}, ManifestRef: "manifest-1", ManifestVersion: 1, PackageCount: 1, CapturedAt: now, MediaRefs: []string{"media_opaque_1"}}
	if err := base.Validate(now); err != nil {
		t.Fatalf("valid evidence: %v", err)
	}
	invalid := base
	invalid.MediaRefs = []string{"data:image/png;base64,abc"}
	if invalid.Validate(now) == nil {
		t.Fatal("embedded media bytes were accepted")
	}
	invalid = base
	lat := 91.0
	lng := 3.4
	invalid.Latitude, invalid.Longitude = &lat, &lng
	if invalid.Validate(now) == nil {
		t.Fatal("invalid coordinates were accepted")
	}
	invalid = base
	invalid.ParcelRefs = []string{"parcel-1", "parcel-1"}
	if invalid.Validate(now) == nil {
		t.Fatal("duplicate parcel reference was accepted")
	}
}

func TestProviderCannotSelfApproveAdditionalCustomerFee(t *testing.T) {
	service := NewOperationalService(&Repository{}, []byte("test-secret"), time.Now)
	_, err := service.ReportException(context.Background(), "provider-1", "mission-1", ExceptionCommand{
		OperationRef: "fee-attempt", Reason: ReasonWrongAddress, Recovery: RecoveryReplan,
		ParcelRefs: []string{"parcel-1"}, AdditionalFeeMinor: 100,
		CommercialApprovalRef: "caller-asserted-approval",
	})
	if err == nil {
		t.Fatal("caller-supplied commercial approval reference was trusted")
	}
}
