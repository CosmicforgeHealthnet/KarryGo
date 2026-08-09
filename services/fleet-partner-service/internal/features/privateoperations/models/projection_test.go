package models

import "testing"

func TestAggregateOrderOutcome(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"COMPLETED", "COMPLETED"}, OutcomeCompleted},
		{[]string{"COMPLETED", "FAILED"}, OutcomePartial},
		{[]string{"FAILED", "FAILED"}, OutcomeFailed},
		{[]string{"CANCELLED", "CANCELLED"}, OutcomeCancelled},
		{[]string{"COMPLETED", "IN_TRANSIT"}, OutcomeActive},
		{[]string{"ASSIGNED", "PLANNED"}, OutcomeAssigned},
		{[]string{"FAILED", "CANCELLED"}, ""},
	}
	for _, tc := range cases {
		if got := AggregateOrderOutcome(tc.in); got != tc.want {
			t.Fatalf("AggregateOrderOutcome(%v)=%s want %s", tc.in, got, tc.want)
		}
	}
}

func TestGapSafePaths(t *testing.T) {
	if got := ShipmentProjectionPath("ASSIGNED", ""); len(got) != 0 {
		t.Fatalf("unknown outcome advanced shipment: %v", got)
	}
}
