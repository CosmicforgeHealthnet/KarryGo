package models

import "testing"

func TestValidClass(t *testing.T) {
	for class := range PlatformMinimumDays {
		if !ValidClass(class) {
			t.Errorf("expected %q to be a valid class", class)
		}
	}
	if ValidClass("NOT_A_CLASS") {
		t.Fatal("expected NOT_A_CLASS to be invalid")
	}
}

func TestEffectiveDays_NeverBelowPlatformMinimum(t *testing.T) {
	p := Policy{RetentionClass: ClassFinancial, RetentionDays: 1}
	if got := p.EffectiveDays(); got != PlatformMinimumDays[ClassFinancial] {
		t.Fatalf("EffectiveDays() = %d, want platform minimum %d", got, PlatformMinimumDays[ClassFinancial])
	}
}

func TestEffectiveDays_HonorsExtendedOverride(t *testing.T) {
	minimum := PlatformMinimumDays[ClassOperational]
	p := Policy{RetentionClass: ClassOperational, RetentionDays: minimum + 100}
	if got := p.EffectiveDays(); got != minimum+100 {
		t.Fatalf("EffectiveDays() = %d, want %d", got, minimum+100)
	}
}

func TestAllFourteenClassesHavePlatformMinimums(t *testing.T) {
	want := []string{
		ClassTemporary, ClassOperational, ClassCustomerRecord, ClassProviderRecord,
		ClassTripExecution, ClassSafetyEvidence, ClassIncidentEvidence, ClassCompliance,
		ClassFinancial, ClassPayroll, ClassSupport, ClassAudit, ClassLegalHold, ClassCompanyConfidential,
	}
	if len(PlatformMinimumDays) != len(want) {
		t.Fatalf("PlatformMinimumDays has %d classes, want %d", len(PlatformMinimumDays), len(want))
	}
	for _, class := range want {
		if _, ok := PlatformMinimumDays[class]; !ok {
			t.Errorf("missing platform minimum for %q", class)
		}
	}
}
