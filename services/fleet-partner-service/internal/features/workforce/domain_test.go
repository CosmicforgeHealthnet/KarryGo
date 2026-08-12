package workforce

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

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
		if got := declineCategory(in); got != want {
			t.Errorf("%s=%s want %s", in, got, want)
		}
	}
}

func TestSection17WorkerRejectsTenantMismatch(t *testing.T) {
	s := &Service{}
	err := s.handleMaintenance(context.Background(), jobs.Job{OrganizationID: "org-a", Payload: []byte(`{"organizationId":"org-b"}`)})
	if !errors.Is(err, jobs.ErrTenantMismatch) {
		t.Fatalf("expected tenant mismatch, got %v", err)
	}
}
func TestSection17RouteCatalogue(t *testing.T) {
	routes := TenantRoutes()
	if len(routes) != 37 {
		t.Fatalf("routes=%d", len(routes))
	}
	seen := map[string]bool{}
	for _, r := range routes {
		k := r.Method + " " + r.Path
		if seen[k] {
			t.Fatalf("duplicate %s", k)
		}
		seen[k] = true
		if r.Permission == "" || r.FeatureFlag == "" || r.AuthClass != "corporate" {
			t.Fatalf("incomplete %+v", r)
		}
		if strings.Contains(strings.ToLower(k), "payroll") {
			t.Fatalf("excluded route %s", k)
		}
	}
}
