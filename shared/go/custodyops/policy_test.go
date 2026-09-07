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
