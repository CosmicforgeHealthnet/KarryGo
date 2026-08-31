package orders

import "testing"

func TestParentOrderStateDerivationRefundAndMixedOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		statuses []VendorStatus
		want     ParentState
	}{
		{"all completed", []VendorStatus{VendorStatusCompleted, VendorStatusDelivered}, ParentStateCompleted},
		{"completed and cancelled", []VendorStatus{VendorStatusCompleted, VendorStatusCancelled}, ParentStatePartiallyCompleted},
		{"completed and refunded", []VendorStatus{VendorStatusCompleted, VendorStatusRefunded}, ParentStatePartiallyCompleted},
		{"refund pending sibling", []VendorStatus{VendorStatusCompleted, VendorStatusRefundPending}, ParentStateRefundPending},
		{"active and complete", []VendorStatus{VendorStatusPreparing, VendorStatusCompleted}, ParentStateInProgress},
		{"all rejected or cancelled", []VendorStatus{VendorStatusRejected, VendorStatusCancelled}, ParentStateCancelled},
		{"all refunded", []VendorStatus{VendorStatusRefunded, VendorStatusRefunded}, ParentStateRefunded},
		{"partial refunds", []VendorStatus{VendorStatusPartiallyRefunded, VendorStatusRefunded}, ParentStatePartiallyRefunded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DeriveParentState(test.statuses); got != test.want {
				t.Fatalf("DeriveParentState(%v) = %s, want %s", test.statuses, got, test.want)
			}
		})
	}
}
