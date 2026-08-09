package assets

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

func ptr(v float64) *float64 { return &v }
func TestMaintenanceScheduleDueModes(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	asset := Asset{Odometer: 100, OperatingHours: 50}
	cases := []struct {
		s    Schedule
		want bool
	}{{Schedule{TriggerType: "DATE", NextDueDate: &past}, true}, {Schedule{TriggerType: "DATE", NextDueDate: &future}, false}, {Schedule{TriggerType: "MILEAGE", NextDueOdometer: ptr(100)}, true}, {Schedule{TriggerType: "OPERATING_HOURS", NextDueHours: ptr(51)}, false}, {Schedule{TriggerType: "COMBINED", NextDueDate: &future, NextDueOdometer: ptr(99)}, true}}
	for _, c := range cases {
		if got := ScheduleDue(c.s, asset, now); got != c.want {
			t.Errorf("%+v=%v", c.s, got)
		}
	}
}

func TestSection18WorkerRejectsTenantMismatch(t *testing.T) {
	s := &Service{}
	err := s.handleMaintenance(context.Background(), jobs.Job{OrganizationID: "org-a", Payload: []byte(`{"organizationId":"org-b"}`)})
	if !errors.Is(err, jobs.ErrTenantMismatch) {
		t.Fatalf("expected tenant mismatch, got %v", err)
	}
}
func TestSection18RouteCatalogueAndNoFinancialSurface(t *testing.T) {
	routes := TenantRoutes()
	if len(routes) != 43 {
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
		lower := strings.ToLower(k)
		for _, bad := range []string{"wallet", "ledger", "payroll", "procurement", "invoice"} {
			if strings.Contains(lower, bad) {
				t.Fatalf("excluded surface %s", k)
			}
		}
	}
}
