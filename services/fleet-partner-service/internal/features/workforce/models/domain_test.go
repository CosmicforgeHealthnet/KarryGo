package models

import "testing"

func TestSection17StateMachinesAndDeclineProtection(t *testing.T) {
	if !ValidProfileTransition("ACTIVE", "ENDED") || ValidProfileTransition("ENDED", "ACTIVE") {
		t.Fatal("profile transition table")
	}
	if !ValidShiftTransition("DRAFT", "PUBLISHED") || ValidShiftTransition("COMPLETED", "DRAFT") {
		t.Fatal("shift transition table")
	}
	if !ValidAssignmentTransition("CHECKED_IN", "CHECKED_OUT") || ValidAssignmentTransition("CONFIRMED", "CHECKED_OUT") {
		t.Fatal("attendance transition table")
	}
	if !ValidLeaveTransition("REQUESTED", "APPROVED") || ValidLeaveTransition("REJECTED", "APPROVED") {
		t.Fatal("leave transition table")
	}
	cases := map[string]string{"UNSAFE_VEHICLE": "Safety", "VEHICLE_DEFECT": "Safety", "BUSY": "Availability", "DEVICE_TECHNICAL": "Technical", "PERSONAL_EMERGENCY": "Personal", "OPERATIONAL_CONFLICT": "Operational", "OTHER": "Other"}
	for in, want := range cases {
		if got := DeclineCategory(in); got != want {
			t.Errorf("%s=%s want %s", in, got, want)
		}
	}
}
