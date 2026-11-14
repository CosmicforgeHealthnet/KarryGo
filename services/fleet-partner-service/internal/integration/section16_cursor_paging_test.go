package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	routemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	trackingmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// pageAll walks a cursor-paged list two rows at a time and returns every id
// seen, failing on a repeated id or a list that never ends.
func pageAll(t *testing.T, name string, list func(after *time.Time, afterID string) ([]string, string, error)) []string {
	t.Helper()
	seen := map[string]bool{}
	out := []string{}
	var after *time.Time
	afterID := ""
	for round := 0; ; round++ {
		page, next, e := list(after, afterID)
		if e != nil {
			t.Fatalf("%s: %v", name, e)
		}
		for _, id := range page {
			if seen[id] {
				t.Fatalf("%s: %s repeated across pages", name, id)
			}
			seen[id] = true
			out = append(out, id)
		}
		if next == "" {
			return out
		}
		if round > 20 {
			t.Fatalf("%s: pages never ended", name)
		}
		position, ok, e := cursor.Decode(next)
		if e != nil || !ok {
			t.Fatalf("%s: nextCursor %q not decodable: %v", name, next, e)
		}
		at := position.SortTime()
		after, afterID = &at, position.LastID
	}
}

// TestSection16ListsPageByCursorWithinBranchScope covers FLEET-6f for the
// Section 16 lists: page 1 -> nextCursor -> page 2 ... visits every row once,
// with no gap or duplicate, even when rows share one sort timestamp (so only
// the id tie-breaker separates them), and a branch-scoped caller never sees
// another branch's rows on any page.
func TestSection16ListsPageByCursorWithinBranchScope(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	otherBranch := ids.New("brn")
	if _, e := f.pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone) VALUES($1,$2,'SEC16B','Section 16 B','Europe/Rome')`, otherBranch, f.org); e != nil {
		t.Fatal(e)
	}
	scoped := f.t
	scoped.PermittedBranchIDs = []string{f.branch}
	counts := map[string]int{f.branch: 5, otherBranch: 2}
	stamp := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)

	routesByBranch := map[string]string{}
	for branch, n := range counts {
		for i := 0; i < n; i++ {
			if _, e := f.s.Dispatch.CreatePlan(ctx, f.t, dispatchmodels.CreatePlanInput{BranchID: branch, PlanDate: f.now, WorkWindowStart: f.now, WorkWindowEnd: f.now.Add(time.Hour), Timezone: "Europe/Rome", IdempotencyKey: ids.New("plan")}); e != nil {
				t.Fatal(e)
			}
			r, e := f.s.Routes.CreateRoute(ctx, f.t, routemodels.CreateRouteInput{BranchID: branch, RouteReference: "R-" + ids.New("ref"), RouteType: "DELIVERY", ExecutionService: "DISPATCH", ConstraintSnapshot: json.RawMessage(`{}`), IdempotencyKey: ids.New("route")})
			if e != nil {
				t.Fatal(e)
			}
			routesByBranch[branch] = r.ID
			if _, e = f.s.Dispatch.CreateGeofence(ctx, f.t, dispatchmodels.CreateGeofenceInput{BranchID: branch, Name: "fence", GeometryType: "CIRCLE", Geometry: dispatchmodels.Geometry{Center: &dispatchmodels.Point{Latitude: 1, Longitude: 1}, RadiusMeters: 50}, IdempotencyKey: ids.New("geo")}); e != nil {
				t.Fatal(e)
			}
			if _, e = f.pool.Exec(ctx, `INSERT INTO fleet_partner_operational_alerts(id,organization_id,branch_id,alert_type,severity,source_type,source_id,condition_fingerprint,sla_due_at,created_at) VALUES($1,$2,$3,'ROUTE_DELAY','LOW','TEST','src',$4,$5,$5)`, ids.New("alt"), f.org, branch, ids.New("cond"), stamp); e != nil {
				t.Fatal(e)
			}
			if _, e = f.pool.Exec(ctx, `INSERT INTO fleet_partner_rescue_cases(id,organization_id,branch_id,trigger_type,severity,subject_key,canonical_trip_reference,reason,created_by,idempotency_key,request_fingerprint,created_at) VALUES($1,$2,$3,'DELAY','LOW',$4,$4,'late','tester',$4,'fp',$5)`, ids.New("rsc"), f.org, branch, ids.New("trip"), stamp); e != nil {
				t.Fatal(e)
			}
		}
	}
	for branch, n := range counts {
		var fence string
		if e := f.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_geofences WHERE organization_id=$1 AND branch_id=$2 LIMIT 1`, f.org, branch).Scan(&fence); e != nil {
			t.Fatal(e)
		}
		for i := 0; i < n; i++ {
			trip := ids.New("trip")
			if _, e := f.pool.Exec(ctx, `INSERT INTO fleet_partner_geofence_events(id,organization_id,geofence_id,canonical_trip_reference,event_rule,source_version,confidence,occurred_at) VALUES($1,$2,$3,$4,'ENTER',1,'HIGH',$5)`, ids.New("gfe"), f.org, fence, trip, stamp); e != nil {
				t.Fatal(e)
			}
			lat, lng := 1.0, 1.0
			if _, _, e := f.s.Tracking.IngestTracking(ctx, trackingmodels.TrackingEvent{EventID: ids.New("evt"), OrganizationID: f.org, SourceService: "driver-dispatch-delivery-service", CanonicalTripReference: trip, RoutePlanID: routesByBranch[branch], OperationalStatus: "IN_PROGRESS", Latitude: &lat, Longitude: &lng, SourceTimestamp: f.now, SourceVersion: 1}); e != nil {
				t.Fatal(e)
			}
		}
	}

	type lister func(tc tenant.Context, after *time.Time, afterID string) ([]string, string, error)
	lists := map[string]lister{
		"plans": func(tc tenant.Context, after *time.Time, afterID string) ([]string, string, error) {
			v, next, e := f.s.Dispatch.ListPlans(ctx, tc, dispatchmodels.PageFilter{Limit: 2, After: after, AfterID: afterID})
			out := []string{}
			for _, x := range v {
				out = append(out, x.BranchID+"/"+x.ID)
			}
			return out, next, e
		},
		"routes": func(tc tenant.Context, after *time.Time, afterID string) ([]string, string, error) {
			v, next, e := f.s.Routes.ListRoutes(ctx, tc, dispatchmodels.PageFilter{Limit: 2, After: after, AfterID: afterID})
			out := []string{}
			for _, x := range v {
				out = append(out, x.BranchID+"/"+x.ID)
			}
			return out, next, e
		},
		"geofences": func(tc tenant.Context, after *time.Time, afterID string) ([]string, string, error) {
			v, next, e := f.s.Dispatch.ListGeofences(ctx, tc, dispatchmodels.PageFilter{Limit: 2, After: after, AfterID: afterID})
			out := []string{}
			for _, x := range v {
				out = append(out, x.BranchID+"/"+x.ID)
			}
			return out, next, e
		},
		"alerts": func(tc tenant.Context, after *time.Time, afterID string) ([]string, string, error) {
			v, next, e := f.s.Dispatch.ListAlerts(ctx, tc, dispatchmodels.PageFilter{Limit: 2, After: after, AfterID: afterID})
			out := []string{}
			for _, x := range v {
				out = append(out, x.BranchID+"/"+x.ID)
			}
			return out, next, e
		},
		"rescue cases": func(tc tenant.Context, after *time.Time, afterID string) ([]string, string, error) {
			v, next, e := f.s.Dispatch.ListRescueCases(ctx, tc, dispatchmodels.RescueFilter{Limit: 2, After: after, AfterID: afterID})
			out := []string{}
			for _, x := range v {
				out = append(out, x.BranchID+"/"+x.ID)
			}
			return out, next, e
		},
		"geofence events": func(tc tenant.Context, after *time.Time, afterID string) ([]string, string, error) {
			v, next, e := f.s.Dispatch.ListGeofenceEvents(ctx, tc, dispatchmodels.GeofenceEventFilter{Limit: 2, After: after, AfterID: afterID})
			out := []string{}
			for _, x := range v {
				out = append(out, x.BranchID+"/"+x.ID)
			}
			return out, next, e
		},
		"tracking": func(tc tenant.Context, after *time.Time, afterID string) ([]string, string, error) {
			v, next, e := f.s.Tracking.ListTracking(ctx, tc, dispatchmodels.PageFilter{Limit: 2, After: after, AfterID: afterID})
			out := []string{}
			for _, x := range v {
				branch := otherBranch
				if x.RoutePlanID == routesByBranch[f.branch] {
					branch = f.branch
				}
				out = append(out, branch+"/"+x.CanonicalTripReference)
			}
			return out, next, e
		},
	}
	for name, list := range lists {
		list := list
		all := pageAll(t, name+" (organization)", func(after *time.Time, afterID string) ([]string, string, error) { return list(f.t, after, afterID) })
		if len(all) != 7 {
			t.Fatalf("%s: paged %d rows across the organization, want 7", name, len(all))
		}
		inScope := pageAll(t, name+" (branch scope)", func(after *time.Time, afterID string) ([]string, string, error) { return list(scoped, after, afterID) })
		if len(inScope) != 5 {
			t.Fatalf("%s: paged %d rows in branch scope, want 5: %v", name, len(inScope), inScope)
		}
		for _, row := range inScope {
			if row[:len(f.branch)+1] != f.branch+"/" {
				t.Fatalf("%s: out-of-scope row %s on a scoped page", name, row)
			}
		}
	}
}

// TestSection16ListRejectsCursorWithOffset pins the compatibility rule: a
// list keeps offset paging, but a request sending both cursor and offset is
// a validation error (422), and a malformed cursor is too.
func TestSection16ListRejectsCursorWithOffset(t *testing.T) {
	h := newConventionsHarness(t)
	token := cursor.EncodeTime(time.Now(), "id")
	for _, path := range []string{"/dispatch/plans", "/routes", "/tracking", "/geofences", "/geofence-events", "/alerts", "/rescue-cases"} {
		for _, query := range []string{"?cursor=" + token + "&offset=0", "?cursor=not-a-cursor"} {
			if status, _, raw := h.get(t, "/fleet-partner/v1"+path+query); status != 422 {
				t.Fatalf("GET %s%s = %d, want 422: %s", path, query, status, raw)
			}
		}
	}
}
