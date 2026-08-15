package pastry

import "testing"

func TestMilestoneTransitionOrdering(t *testing.T) {
	if !validMilestoneTransition("BASE_PREPARED", "DECORATION") {
		t.Fatal("forward milestone transition rejected")
	}
	if validMilestoneTransition("DECORATION", "BASE_PREPARED") {
		t.Fatal("backward milestone transition accepted")
	}
	if !validMilestoneTransition("DECORATION", "BLOCKED") {
		t.Fatal("blocked operational transition rejected")
	}
	if !validMilestoneTransition("DECORATION", "BLOCKED", "ACTIVE") {
		t.Fatal("blocked milestone could not resume")
	}
}

func TestValidateStoredPoliciesRejectsUnboundedFields(t *testing.T) {
	if err := validateStoredPolicies([]byte(`{"minutes_before_required_at":1440,"disallowed_after_milestone":"DECORATION"}`), []byte(`{"review_after_milestone":"BASE_PREPARED","restrict_after_milestone":"DECORATION"}`)); err != nil {
		t.Fatalf("valid bounded policies rejected: %v", err)
	}
	if err := validateStoredPolicies([]byte(`{"arbitrary_formula":"now()"}`), []byte(`{}`)); err == nil {
		t.Fatal("unknown change-policy field accepted")
	}
	if err := validateStoredPolicies([]byte(`{}`), []byte(`{"restrict_after_milestone":"NOT_A_MILESTONE"}`)); err == nil {
		t.Fatal("unknown cancellation milestone accepted")
	}
}
