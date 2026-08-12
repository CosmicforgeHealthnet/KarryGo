package dispatch

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func eligibleFact() CandidateFact {
	return CandidateFact{ProviderID: "provider-1", ProviderMembershipID: "membership-1", VehicleReference: "vehicle-1", ExecutionService: "DISPATCH", MembershipActive: true, ProviderActive: true, CapabilityMatch: true, Available: true, VehicleEligible: true, CapacityOK: true, BranchAllowed: true, CargoAllowed: true, SafetyAllowed: true, ReservationFree: true, ActiveWorkFree: true, TrackingFreshness: Fresh, SourceVersions: map[string]int64{"provider": 4, "vehicle": 7}}
}
func TestCandidateScoringIsExplainableDeterministicAndFailClosed(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	fact := eligibleFact()
	a := ScoreCandidate("org", "branch", "ROUTE", "route", fact, now, 5*time.Minute)
	b := ScoreCandidate("org", "branch", "ROUTE", "route", fact, now, 5*time.Minute)
	if a.EligibilityStatus != "ELIGIBLE" || a.TotalScore != 100 || len(a.ScoreComponents) != 11 || len(a.EligibilityReasons) != 11 || len(a.HardBlocks) != 0 {
		t.Fatalf("eligible candidate=%+v", a)
	}
	if a.InputFingerprint != b.InputFingerprint || !reflect.DeepEqual(a.ScoreComponents, b.ScoreComponents) {
		t.Fatal("same source versions must generate the same explainable score")
	}
	if !a.ExpiresAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatal("candidate expiration is not bounded")
	}
	fact.SourceVersions["vehicle"]++
	c := ScoreCandidate("org", "branch", "ROUTE", "route", fact, now, 5*time.Minute)
	if c.InputFingerprint == a.InputFingerprint {
		t.Fatal("source version change must invalidate recommendation")
	}
}
func TestCandidateHardBlocksAndFreshnessWarnings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CandidateFact)
		block  string
	}{{"inactive membership", func(f *CandidateFact) { f.MembershipActive = false }, "INACTIVE_MEMBERSHIP"}, {"inactive provider", func(f *CandidateFact) { f.ProviderActive = false }, "PROVIDER_INACTIVE"}, {"capability mismatch", func(f *CandidateFact) { f.CapabilityMatch = false }, "CAPABILITY_MISMATCH"}, {"availability", func(f *CandidateFact) { f.Available = false }, "PROVIDER_UNAVAILABLE"}, {"vehicle", func(f *CandidateFact) { f.VehicleEligible = false }, "VEHICLE_INELIGIBLE"}, {"capacity", func(f *CandidateFact) { f.CapacityOK = false }, "CAPACITY_EXCEEDED"}, {"branch", func(f *CandidateFact) { f.BranchAllowed = false }, "BRANCH_RESTRICTED"}, {"cargo", func(f *CandidateFact) { f.CargoAllowed = false }, "CARGO_RESTRICTED"}, {"safety", func(f *CandidateFact) { f.SafetyAllowed = false }, "SAFETY_BLOCK"}, {"reservation", func(f *CandidateFact) { f.ReservationFree = false }, "RESERVATION_CONFLICT"}, {"active work", func(f *CandidateFact) { f.ActiveWorkFree = false }, "ACTIVE_WORK_CONFLICT"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := eligibleFact()
			tt.mutate(&f)
			c := ScoreCandidate("o", "b", "ROUTE", "r", f, time.Now(), time.Minute)
			if c.EligibilityStatus != "INELIGIBLE" || !contains(c.HardBlocks, tt.block) {
				t.Fatalf("candidate=%+v", c)
			}
		})
	}
	f := eligibleFact()
	f.TrackingFreshness = Stale
	c := ScoreCandidate("o", "b", "ROUTE", "r", f, time.Now(), time.Minute)
	if c.EligibilityStatus != "ELIGIBLE" || !contains(c.Warnings, "TRACKING_STALE") {
		t.Fatalf("stale authoritative location should warn, got %+v", c)
	}
}
func contains(v []string, w string) bool {
	for _, x := range v {
		if x == w {
			return true
		}
	}
	return false
}
func TestCentralStateTransitions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ok, bad bool
	}{{"manual plan approval", ValidPlanTransition(PlanDraft, PlanApproved), ValidPlanTransition(PlanCompleted, PlanDraft)}, {"manual route approval", ValidRouteTransition(RouteDraft, RouteApproved), ValidRouteTransition(RoutePublished, RouteDraft)}, {"route lifecycle", ValidRouteTransition(RoutePublished, RouteInProgress), ValidRouteTransition(RouteCompleted, RouteInProgress)}, {"stop lifecycle", ValidStopTransition(StopArrived, StopServiceStarted), ValidStopTransition(StopCompleted, StopPlanned)}, {"alert lifecycle", ValidAlertTransition(AlertAcknowledged, AlertResolved), ValidAlertTransition(AlertResolved, AlertOpen)}} {
		if !tc.ok || tc.bad {
			t.Errorf("%s ok=%v bad=%v", tc.name, tc.ok, tc.bad)
		}
	}
}
func TestTrackingFreshnessAndGap(t *testing.T) {
	now := time.Now()
	if got := Freshness(now.Add(-time.Minute), now, 2*time.Minute, 10*time.Minute, false); got != Fresh {
		t.Fatal(got)
	}
	if got := Freshness(now.Add(-5*time.Minute), now, 2*time.Minute, 10*time.Minute, false); got != Aging {
		t.Fatal(got)
	}
	if got := Freshness(now.Add(-20*time.Minute), now, 2*time.Minute, 10*time.Minute, false); got != Stale {
		t.Fatal(got)
	}
	if got := Freshness(time.Time{}, now, 2*time.Minute, 10*time.Minute, false); got != Unknown {
		t.Fatal(got)
	}
	if got := Freshness(now, now, 2*time.Minute, 10*time.Minute, true); got != Gapped {
		t.Fatal(got)
	}
}
func TestGeofenceValidationAndContainment(t *testing.T) {
	circle := Geofence{GeometryType: "CIRCLE", Geometry: Geometry{Center: &Point{0, 0}, RadiusMeters: 1000}}
	if e := ValidateGeometry(circle.GeometryType, circle.Geometry); e != nil {
		t.Fatal(e)
	}
	if !Contains(circle, Point{0.001, 0.001}) || Contains(circle, Point{1, 1}) {
		t.Fatal("circle containment")
	}
	polygon := Geofence{GeometryType: "POLYGON", Geometry: Geometry{Points: []Point{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}}}}
	if e := ValidateGeometry(polygon.GeometryType, polygon.Geometry); e != nil {
		t.Fatal(e)
	}
	if !Contains(polygon, Point{.5, .5}) || Contains(polygon, Point{2, 2}) {
		t.Fatal("polygon containment")
	}
	bow := Geometry{Points: []Point{{0, 0}, {1, 1}, {0, 1}, {1, 0}, {0, 0}}}
	if e := ValidateGeometry("POLYGON", bow); e == nil || !strings.Contains(e.Error(), "self-intersect") {
		t.Fatalf("invalid polygon accepted: %v", e)
	}
	if e := ValidateGeometry("CIRCLE", Geometry{Center: &Point{100, 0}, RadiusMeters: 10}); e == nil {
		t.Fatal("invalid circle accepted")
	}
}
func TestOptimizationFingerprintCoversRouteVersionAndInputs(t *testing.T) {
	a := OptimizationInput{RouteID: "r", RouteVersion: 1, Stops: []RouteStop{{ID: "s1", Sequence: 1}}}
	b := a
	if OptimizationFingerprint(a) != OptimizationFingerprint(a) {
		t.Fatal("fingerprint must be deterministic")
	}
	b.RouteVersion = 2
	if OptimizationFingerprint(a) == OptimizationFingerprint(b) {
		t.Fatal("changed route version must create a new fingerprint")
	}
	b = a
	b.Stops = append([]RouteStop(nil), a.Stops...)
	b.Stops[0].Sequence = 2
	if OptimizationFingerprint(a) == OptimizationFingerprint(b) {
		t.Fatal("changed route input must create a new fingerprint")
	}
	raw, _ := json.Marshal(a)
	if len(raw) == 0 {
		t.Fatal("optimization input must be serializable")
	}
}
func TestSection16RouteCatalogue(t *testing.T) {
	routes := TenantRoutes()
	if len(routes) != 37 {
		t.Fatalf("tenant route count=%d", len(routes))
	}
	seen := map[string]bool{}
	for _, r := range routes {
		k := r.Method + " " + r.Path
		if seen[k] {
			t.Fatalf("duplicate route %s", k)
		}
		seen[k] = true
		if r.Permission == "" || r.FeatureFlag == "" || r.AuthClass != "corporate" {
			t.Fatalf("incomplete route metadata %+v", r)
		}
	}
	for _, required := range []string{"GET /dispatch/board", "GET /dispatch/unassigned", "GET /dispatch/candidates", "POST /routes/:routeId/publish", "GET /control-tower", "GET /tracking/:canonicalTripReference", "POST /alerts/:alertId/resolve"} {
		if !seen[required] {
			t.Errorf("missing %s", required)
		}
	}
	for key := range seen {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "/partner") || strings.Contains(lower, "/taxi") {
			t.Errorf("deferred route present: %s", key)
		}
	}
}
