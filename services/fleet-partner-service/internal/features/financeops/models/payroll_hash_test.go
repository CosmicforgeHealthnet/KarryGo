package models

import "testing"

func TestPayrollCalculationHashIsDeterministicAndInputSensitive(t *testing.T) {
	a := CompensationItem{MembershipID: "m1", SourceType: "PAYROLL_PERIOD", SourceReference: "p1", RuleID: "r1", RuleVersion: 1, AmountMinor: 50000, Currency: "NGN", ReasonCode: "SALARY", SourceInputs: []byte(`{"ruleVersion":1}`), CalculationVersion: 1}
	b := CompensationItem{MembershipID: "m2", SourceType: "COMPLETED_ASSIGNMENT", SourceReference: "asg1", RuleID: "r2", RuleVersion: 3, AmountMinor: 1200, Currency: "NGN", ReasonCode: "PER_JOB", SourceInputs: []byte(`{"assignmentId":"asg1"}`), CalculationVersion: 1}
	h1 := PayrollCalculationHash([]CompensationItem{a, b})
	// Order and calculation version do not change the fingerprint.
	b2 := b
	b2.CalculationVersion = 7
	if h2 := PayrollCalculationHash([]CompensationItem{b2, a}); h2 != h1 {
		t.Fatalf("hash depends on order/calculation version: %s vs %s", h1, h2)
	}
	// Any amount, rule version or source change does.
	for name, mutate := range map[string]func(*CompensationItem){
		"amount":      func(i *CompensationItem) { i.AmountMinor++ },
		"ruleVersion": func(i *CompensationItem) { i.RuleVersion++ },
		"source":      func(i *CompensationItem) { i.SourceReference = "asg2" },
		"currency":    func(i *CompensationItem) { i.Currency = "USD" },
	} {
		c := b
		mutate(&c)
		if PayrollCalculationHash([]CompensationItem{a, c}) == h1 {
			t.Errorf("%s change did not change the hash", name)
		}
	}
	// A removed item changes it too.
	if PayrollCalculationHash([]CompensationItem{a}) == h1 {
		t.Error("removing an item did not change the hash")
	}
	if len(h1) != 64 {
		t.Fatalf("hash length %d", len(h1))
	}
}
