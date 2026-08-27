package orders

import "testing"

func TestSubstitutionPolicyEvaluator(t *testing.T) {
	base := SubstitutionCandidate{Active: true, Sellable: true, SameBusiness: true, SameBranch: true, SameBrand: true, CategoryCompatible: true, MeasurementCompatible: true, QualityCompatible: true, InventoryAvailable: true, OriginalPriceMinor: 1000, CandidatePriceMinor: 1050, ToleranceMinor: 100}
	tests := []struct {
		policy SubstitutionPolicy
		want   SubstitutionDecision
	}{
		{PolicyExactOnly, SubstitutionForbidden},
		{PolicySpecificReplacement, SubstitutionAllowedWithoutNewDecision},
		{PolicySameBrand, SubstitutionAllowedWithoutNewDecision},
		{PolicySimilarWithinPriceTolerance, SubstitutionAllowedWithoutNewDecision},
		{PolicyVendorSuggestAndApprove, SubstitutionRequiresCustomerDecision},
		{PolicyContactFirst, SubstitutionRequiresCustomerDecision},
		{PolicyAutoRefund, SubstitutionAutoRefund},
	}
	for _, tt := range tests {
		if got := EvaluateSubstitution(tt.policy, base).Decision; got != tt.want {
			t.Fatalf("%s: got %s want %s", tt.policy, got, tt.want)
		}
	}
	base.CandidatePriceMinor = 1201
	if got := EvaluateSubstitution(PolicySimilarWithinPriceTolerance, base).Decision; got != SubstitutionRequiresCommercialReapproval {
		t.Fatalf("outside tolerance: got %s", got)
	}
}

func TestQualityPreferencesAllowMultipleTypesAndRejectContradiction(t *testing.T) {
	values := []QualityPreference{{Type: QualityRipeness, Value: "RIPE", Source: PreferenceCheckoutExplicit}, {Type: QualityFirmness, Value: "FIRM", Source: PreferenceCustomerSaved}}
	if err := ValidateQualityPreferences(values); err != nil {
		t.Fatal(err)
	}
	values = append(values, QualityPreference{Type: QualityRipeness, Value: "GREEN", Source: PreferenceCheckoutExplicit})
	if err := ValidateQualityPreferences(values); err == nil {
		t.Fatal("expected contradictory preference error")
	}
}

func TestVariableMeasurementCapAndInventory(t *testing.T) {
	m := VariableMeasurement{Mode: MeasurementVariableWeight, EstimatedQuantity: 1000, UnitPriceMinor: 3, EstimatedAmountMinor: 3000, HardMaxQuantity: 1300, ReservedUpperQuantity: 1200, State: FinancialEstimated}
	var err error
	m, err = m.RecordActual(1100)
	if err != nil {
		t.Fatal(err)
	}
	if m.FinalAmountMinor != 3300 {
		t.Fatalf("amount=%d", m.FinalAmountMinor)
	}
	m, err = m.Finalize(3000)
	if err == nil || m.State != FinancialReapprovalRequired {
		t.Fatalf("expected reapproval, state=%s err=%v", m.State, err)
	}
	if a, r, err := ReplacementInventoryCommit(6, 3, 10, 4); err != nil || a != 6 || r != 3 {
		t.Fatalf("replacement commit: %d %d %v", a, r, err)
	}
	if ParentCapAllows(100, 70, 40, 0, 0, 0, 0) {
		t.Fatal("combined parent cap must block")
	}
}

func TestPickingGateAndQualityResults(t *testing.T) {
	p := ShopPickingLine{ParentOrderID: "p", VendorOrderID: "v", OrderLineID: "l", OrderedQuantity: 4, RemainingQuantity: 4, SubstitutionPolicy: PolicyAutoRefund, ExpectedVendorOrderVersion: 1, Version: 1}
	p, _ = p.RecordAvailability(3)
	if !p.PackingBlocked(false, false, false, true, true) {
		t.Fatal("partial pick must block packing")
	}
	if err := ValidateQualityResult(QualitySubstitute); err != nil {
		t.Fatal(err)
	}
}
