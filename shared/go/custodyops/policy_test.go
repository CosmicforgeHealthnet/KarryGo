package custodyops

import (
	"errors"
	"testing"
	"time"
)

func TestExceptionPolicyFailsClosed(t *testing.T) {
	p := Policy{}
	if err := p.ValidateException(ExceptionCommand{Reason: ExceptionSecurityRisk, RecoveryAction: ActionWait}); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateException(ExceptionCommand{Reason: ExceptionSecurityRisk, RecoveryAction: ActionAlternativeReceiver}); !errors.Is(err, ErrPolicy) {
		t.Fatalf("error=%v", err)
	}
	if err := p.ValidateException(ExceptionCommand{Reason: ExceptionReceiverUnavailable, RecoveryAction: ActionReschedule}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error=%v", err)
	}
	now := time.Now()
	if err := p.ValidateException(ExceptionCommand{Reason: ExceptionReceiverUnavailable, RecoveryAction: ActionReschedule, ScheduledFor: &now}); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceTypesRemainDistinct(t *testing.T) {
	for _, value := range []EvidenceType{EvidencePickup, EvidenceDelivery, EvidenceDamage, EvidenceDiscrepancy, EvidenceFailedAttempt, EvidenceReturn, EvidenceProviderTransfer} {
		if !validEvidenceType(value) {
			t.Fatalf("missing evidence type %s", value)
		}
	}
	if expectedEvidenceForState("RETURN_REQUIRED") == EvidenceProviderTransfer {
		t.Fatal("transfer evidence satisfied return")
	}
}

func TestCanonicalRefsRejectsDuplicates(t *testing.T) {
	if _, err := CanonicalRefs([]string{"parcel-a", "parcel-a"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error=%v", err)
	}
	if !sameRefs([]string{"b", "a"}, []string{"a", "b"}) {
		t.Fatal("same set rejected")
	}
}

func TestAlternativeReceiverRequiresChallengeRotation(t *testing.T) {
	p := Policy{}
	command := ExceptionCommand{
		Reason: ExceptionReceiverUnavailable, RecoveryAction: ActionAlternativeReceiver,
		ReceiverRef: "receiver-new", PreviousReceiverChallengeRef: "challenge-old",
		ReplacementReceiverChallengeRef: "challenge-new",
	}
	if err := p.ValidateException(command); err != nil {
		t.Fatal(err)
	}
	command.ReplacementReceiverChallengeRef = command.PreviousReceiverChallengeRef
	if err := p.ValidateException(command); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error=%v", err)
	}
}

func TestCrossAuthorityTransferRequiresCompleteProvenance(t *testing.T) {
	repo := &Repository{}
	command := TransferCommand{
		OperationRef: "operation-1", MissionRef: "mission-1", TenantRef: "tenant-1",
		TransferRef: "transfer-1", OldProviderRef: "rider-1", NewProviderRef: "provider-2",
		ManifestRef: "manifest-1", ManifestVersion: 1, ParcelRefs: []string{"parcel-1"},
		PackageCount: 1, EvidenceRef: "evidence-1", OldProviderConfirm: true,
		NewProviderConfirm: true, NewReceiverChallengeRef: "challenge-2",
		OldAuthority: "VENDOR_OWNED", NewAuthority: "DISPATCH",
	}
	if _, err := repo.CompleteTransfer(t.Context(), command); !errors.Is(err, ErrInvalid) {
		t.Fatalf("incomplete cross-authority provenance error=%v", err)
	}
}
