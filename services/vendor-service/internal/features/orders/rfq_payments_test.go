package orders

import (
	"testing"
)

func TestRFQPaymentPolicyDeterministicDepositRounding(t *testing.T) {
	policy, err := parseRFQPaymentPolicy([]byte(`{"type":"DEPOSIT_PERCENTAGE","basis_points":3000}`), 3000000)
	if err != nil || policy.DepositMinor != 900000 {
		t.Fatalf("30%% deposit = 900000, got %+v err=%v", policy, err)
	}
	policy, err = parseRFQPaymentPolicy([]byte(`{"type":"DEPOSIT_PERCENTAGE","basis_points":1}`), 1)
	if err != nil || policy.DepositMinor != 1 {
		t.Fatalf("positive percentage must round up, got %+v err=%v", policy, err)
	}
}

func TestRFQPaymentPolicyRejectsOverflowAndInvalidModes(t *testing.T) {
	if _, err := parseRFQPaymentPolicy([]byte(`{"type":"DEPOSIT_FIXED","amount_minor":0}`), 100); err == nil {
		t.Fatal("zero fixed deposit must fail closed")
	}
	if _, err := parseRFQPaymentPolicy([]byte(`{"type":"DEPOSIT_PERCENTAGE","basis_points":10001}`), 100); err == nil {
		t.Fatal("percentage over 100%% must fail closed")
	}
	if _, err := parseRFQPaymentPolicy([]byte(`{"type":"UNKNOWN"}`), 100); err == nil {
		t.Fatal("unknown policy must fail closed")
	}
}
