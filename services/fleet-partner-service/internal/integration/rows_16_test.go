package integration_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	dispatch "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Section 16 requirement rows that previously had no PostgreSQL test:
// unassigned work, the candidate service path, plan list/patch/cancel,
// route create/list/get/stop-edit/cancel, the tracking list, geofence
// list/get/patch, GET /alerts/:id and the Section 16 job replay.

// rows16Facts is the candidate-facts seam (the eligibility authority): it
// returns one eligible provider unless the source id starts with "none-".
type rows16Facts struct{}

func (rows16Facts) Facts(_ context.Context, _, _, _, sourceID string) ([]dispatch.CandidateFact, error) {
	if strings.HasPrefix(sourceID, "none-") {
		return nil, nil
	}
	return []dispatch.CandidateFact{{ProviderID: "provider-r16", ProviderMembershipID: "membership-r16", VehicleReference: "vehicle-r16",
		ExecutionService: "DISPATCH", MembershipActive: true, ProviderActive: true, CapabilityMatch: true, Available: true,
		VehicleEligible: true, CapacityOK: true, BranchAllowed: true, CargoAllowed: true, SafetyAllowed: true,
		ReservationFree: true, ActiveWorkFree: true, TrackingFreshness: dispatch.Fresh,
		SourceVersions: map[string]int64{"provider": 3, "vehicle": 2}}}, nil
}

type rows16 struct {
	rowsOrgs
	svc     *dispatch.Service
	router  http.Handler
	branchA string
	branchX string
	branchB string
	now     time.Time
}

func newRows16(t *testing.T) *rows16 {
	t.Helper()
	orgs := newRowsOrgs(t, "Rows 16", "")
	now := time.Now().UTC().Truncate(time.Second)
	svc := dispatch.NewService(dispatch.Options{Pool: orgs.pool, Candidates: rows16Facts{},
		Events: events.NewDomainPublisher(events.NewOutboxRepository(orgs.pool)),
		Flags: dispatch.Flags{DispatchPlanning: true, RoutePlanning: true, RoutePublication: true, ControlTower: true,
			TrackingProjections: true, Geofences: true, OperationalAlerts: true, Workers: true, TrackingSourceConfigured: true},
		Now: func() time.Time { return now }})
	f := &rows16{rowsOrgs: orgs, svc: svc, now: now}
	f.NoPermission = rowsActor{Org: orgs.A.Org, Member: seedApprovalMember(t, orgs.pool, orgs.A.Org, "", "no-role")}
	f.branchA = seedBranch(t, orgs.pool, orgs.A.Org, "Rows16 Main")
	f.branchX = seedBranch(t, orgs.pool, orgs.A.Org, "Rows16 Other")
	f.branchB = seedBranch(t, orgs.pool, orgs.B.Org, "Rows16 B")
	f.router = rowsRouter(t, orgs.pool, func(g *gin.RouterGroup, a *accesscontrol.Authorizer, s *idempotency.Store) {
		dispatch.NewHandler(svc, a, s).RegisterTenantRoutes(g)
	})
	return f
}

func (f *rows16) confined(branch string) rowsActor {
	return rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: branch}
}

func (f *rows16) plan(t *testing.T, actor rowsActor, branch string, day time.Time) map[string]any {
	t.Helper()
	return rowsExpect(t, rowsDo(t, f.router, actor, "POST", "/fleet-partner/v1/dispatch/plans", map[string]any{"branchId": branch,
		"planDate": day, "workWindowStart": day, "workWindowEnd": day.Add(8 * time.Hour), "timezone": "Africa/Lagos"}, nil), 201, "create plan").Object(t)
}

func (f *rows16) route(t *testing.T, actor rowsActor, branch, planID, service string) rowsResponse {
	t.Helper()
	return rowsDo(t, f.router, actor, "POST", "/fleet-partner/v1/routes", map[string]any{"branchId": branch, "dispatchPlanId": planID,
		"routeReference": "R-" + ids.New("r"), "routeType": "DELIVERY", "providerMembershipId": "membership-r16", "providerId": "provider-r16",
		"vehicleOrTruckReference": "vehicle-r16", "executionService": service, "constraintSnapshot": map[string]any{"capacity": 100}}, nil)
}

// TestRow16_2_UnassignedWork covers FLEET-16.2: GET /dispatch/unassigned.
func TestRow16_2_UnassignedWork(t *testing.T) {
	f := newRows16(t)
	ctx := context.Background()
	confirmed := ws4Order(t, f.pool, f.A.Org, f.branchA)
	routed := ws4Order(t, f.pool, f.A.Org, f.branchA)
	draft := ws4Order(t, f.pool, f.A.Org, f.branchA)
	elsewhere := ws4Order(t, f.pool, f.A.Org, f.branchX)
	foreign := ws4Order(t, f.pool, f.B.Org, f.branchB)
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_private_orders SET status='CONFIRMED' WHERE id = ANY($1)`, []string{confirmed, routed, elsewhere, foreign}); err != nil {
		t.Fatal(err)
	}
	// An order already on a live route is not unassigned.
	route := rowsExpect(t, f.route(t, f.A, f.branchA, "", "DISPATCH"), 201, "route").Object(t)
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", "/fleet-partner/v1/routes/"+route["id"].(string)+"/stops", map[string]any{"stopType": "DELIVERY", "orderId": routed, "locationReference": "loc-1"}, nil), 201, "stop")

	items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/unassigned", nil, nil), 200, "unassigned").Items(t)
	got := rowsIDs(items, "id")
	if !got[confirmed] || !got[elsewhere] || got[routed] || got[draft] || got[foreign] {
		t.Fatalf("unassigned work: %v", got)
	}
	for _, item := range items {
		if item["id"] == confirmed && (item["sourceType"] != "ORDER" || item["actionReason"] != "CONFIRMED_WITHOUT_ROUTE") {
			t.Fatalf("unassigned item: %v", item)
		}
	}
	if c := rowsIDs(rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/unassigned?branchId="+f.branchX, nil, nil), 200, "branch filter").Items(t), "id"); len(c) != 1 || !c[elsewhere] {
		t.Fatalf("branch filter: %v", c)
	}
	if c := rowsIDs(rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchA), "GET", "/fleet-partner/v1/dispatch/unassigned", nil, nil), 200, "confined").Items(t), "id"); c[elsewhere] || !c[confirmed] {
		t.Fatalf("confined unassigned: %v", c)
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/dispatch/unassigned", nil, nil), 403, "no permission")
}

// TestRow16_3_ExplainableCandidates covers FLEET-16.3: GET /dispatch/candidates
// through the service path (scoring is unit-tested separately).
func TestRow16_3_ExplainableCandidates(t *testing.T) {
	f := newRows16(t)
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/candidates?branchId="+f.branchA, nil, nil), 400, "missing source")
	rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchX), "GET", "/fleet-partner/v1/dispatch/candidates?orderId=ord-1&branchId="+f.branchA, nil, nil), 404, "branch outside scope")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/dispatch/candidates?orderId=ord-1&branchId="+f.branchA, nil, nil), 403, "no permission")

	items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/candidates?orderId=ord-16&branchId="+f.branchA, nil, nil), 200, "candidates").Items(t)
	if len(items) != 1 {
		t.Fatalf("candidates: %v", items)
	}
	c := items[0]
	if c["sourceType"] != "ORDER" || c["sourceId"] != "ord-16" || c["providerId"] != "provider-r16" || c["branchId"] != f.branchA || c["dataFreshness"] != dispatch.Fresh {
		t.Fatalf("candidate: %v", c)
	}
	// Explainable: the score is broken down and the inputs are versioned.
	if components, _ := c["scoreComponents"].([]any); len(components) == 0 {
		t.Fatalf("candidate has no score breakdown: %v", c)
	}
	if versions, _ := c["sourceVersions"].(map[string]any); versions["provider"] != float64(3) {
		t.Fatalf("candidate source versions: %v", c["sourceVersions"])
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_assignment_candidates WHERE organization_id=$1 AND id=$2`, f.A.Org, c["id"]); n != 1 {
		t.Fatalf("candidate not persisted: %d", n)
	}
	if rowsOutboxCount(t, f.pool, "ord-16", "assignment_candidates.generated.v1") != 1 {
		t.Fatal("candidates generated event missing")
	}
	// No eligible provider: an empty list plus an operational alert.
	none := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/candidates?shipmentId=none-shp&branchId="+f.branchA, nil, nil), 200, "no candidates").Items(t)
	if len(none) != 0 {
		t.Fatalf("expected no candidates: %v", none)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND alert_type='NO_ELIGIBLE_PROVIDER' AND source_id='none-shp'`, f.A.Org); n != 1 {
		t.Fatalf("no-eligible-provider alerts = %d", n)
	}
}

// TestRow16_4b_DispatchPlanListPatchCancel covers FLEET-16.4b:
// GET /dispatch/plans, PATCH /dispatch/plans/:id, POST .../cancel.
func TestRow16_4b_DispatchPlanListPatchCancel(t *testing.T) {
	f := newRows16(t)
	day := f.now.Truncate(24 * time.Hour)
	first := f.plan(t, f.A, f.branchA, day)
	second := f.plan(t, f.A, f.branchX, day.Add(24*time.Hour))
	foreign := f.plan(t, f.B, f.branchB, day)

	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/plans", nil, nil), 200, "list").Items(t)
	if len(list) != 2 || list[0]["id"] != second["id"] || list[1]["id"] != first["id"] {
		t.Fatalf("plans newest date first: %v", rowsIDs(list, "id"))
	}
	if rowsIDs(list, "id")[foreign["id"].(string)] {
		t.Fatal("foreign plan listed")
	}
	if c := rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchA), "GET", "/fleet-partner/v1/dispatch/plans", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["id"] != first["id"] {
		t.Fatalf("confined plans: %v", rowsIDs(c, "id"))
	}
	rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchA), "GET", "/fleet-partner/v1/dispatch/plans?branchId="+f.branchX, nil, nil), 404, "branch filter outside scope")
	if paged := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/plans?limit=1&offset=1", nil, nil), 200, "paged").Items(t); len(paged) != 1 || paged[0]["id"] != first["id"] {
		t.Fatalf("paging: %v", rowsIDs(paged, "id"))
	}
	// A negative page window is a validation error, never a server error.
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/plans?offset=-1", nil, nil), 400, "negative offset")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/dispatch/plans", nil, nil), 403, "no permission")

	path := "/fleet-partner/v1/dispatch/plans/" + first["id"].(string)
	window := map[string]any{"timezone": "Europe/London", "workWindowStart": day.Add(time.Hour), "workWindowEnd": day.Add(9 * time.Hour)}
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, window, nil), 400, "patch without version")
	stale := map[string]any{"timezone": "Europe/London", "workWindowStart": day.Add(time.Hour), "workWindowEnd": day.Add(9 * time.Hour), "expectedVersion": 7}
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, stale, nil), 409, "patch stale")
	window["expectedVersion"] = 1
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", path, window, nil), 404, "cross-tenant patch")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, window, nil), 200, "patch").Object(t)
	if patched["timezone"] != "Europe/London" || patched["version"] != float64(2) {
		t.Fatalf("patched plan: %v", patched)
	}
	if rowsOutboxCount(t, f.pool, first["id"].(string), "dispatch_plan.updated.v1") != 1 {
		t.Fatal("dispatch_plan.updated event missing")
	}

	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path+"/cancel", map[string]any{}, nil), 400, "cancel without version")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", path+"/cancel", map[string]any{"expectedVersion": 2}, nil), 404, "cross-tenant cancel")
	cancelled := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path+"/cancel", map[string]any{"expectedVersion": 2}, nil), 200, "cancel").Object(t)
	if cancelled["status"] != dispatch.PlanCancelled {
		t.Fatalf("cancelled plan: %v", cancelled)
	}
	// A cancelled plan is no longer editable or approvable.
	window["expectedVersion"] = 3
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, window, nil), 409, "edit cancelled plan")
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: seedApprovalMember(t, f.pool, f.A.Org, "rol_system_owner", "approver")}, "POST", path+"/approve", map[string]any{"expectedVersion": 3}, nil), 409, "approve cancelled plan")
	if rowsOutboxCount(t, f.pool, first["id"].(string), "dispatch_plan.cancelled.v1") != 1 {
		t.Fatal("dispatch_plan.cancelled event missing")
	}
}

// TestRow16_5b_RouteCreateListGetStopEditCancel covers FLEET-16.5b:
// POST/GET /routes, GET /routes/:id, PATCH /routes/:id/stops/:stopId and
// POST /routes/:id/cancel.
func TestRow16_5b_RouteCreateListGetStopEditCancel(t *testing.T) {
	f := newRows16(t)
	plan := f.plan(t, f.A, f.branchA, f.now)
	foreignPlan := f.plan(t, f.B, f.branchB, f.now)

	rowsExpect(t, f.route(t, f.A, f.branchA, "", "TAXI"), 403, "passenger route")
	rowsExpect(t, f.route(t, f.A, f.branchA, "", "BIKE"), 400, "unknown execution service")
	rowsExpect(t, f.route(t, f.A, f.branchA, foreignPlan["id"].(string), "DISPATCH"), 404, "another organization's plan")
	rowsExpect(t, f.route(t, f.A, f.branchX, plan["id"].(string), "DISPATCH"), 404, "plan from another branch")
	rowsExpect(t, f.route(t, f.confined(f.branchX), f.branchA, "", "DISPATCH"), 404, "branch outside scope")
	rowsExpect(t, f.route(t, f.NoPermission, f.branchA, "", "DISPATCH"), 403, "no permission")
	route := rowsExpect(t, f.route(t, f.A, f.branchA, plan["id"].(string), "dispatch"), 201, "create").Object(t)
	routeID := route["id"].(string)
	if route["status"] != dispatch.RouteDraft || route["executionService"] != "DISPATCH" || route["dispatchPlanId"] != plan["id"] {
		t.Fatalf("route: %v", route)
	}
	hauling := rowsExpect(t, f.route(t, f.A, f.branchX, "", "HAULING"), 201, "hauling route").Object(t)
	if rowsOutboxCount(t, f.pool, routeID, "route_plan.created.v1") != 1 {
		t.Fatal("route_plan.created event missing")
	}

	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/routes", nil, nil), 200, "list").Items(t)
	if ids := rowsIDs(list, "id"); len(list) != 2 || !ids[routeID] || !ids[hauling["id"].(string)] {
		t.Fatalf("routes: %v", ids)
	}
	if svc := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/routes?service=hauling", nil, nil), 200, "service filter").Items(t); len(svc) != 1 || svc[0]["id"] != hauling["id"] {
		t.Fatalf("service filter: %v", svc)
	}
	if c := rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchA), "GET", "/fleet-partner/v1/routes", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["id"] != routeID {
		t.Fatalf("confined routes: %v", rowsIDs(c, "id"))
	}
	if b := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/routes", nil, nil), 200, "other org").Items(t); len(b) != 0 {
		t.Fatalf("other org routes: %v", b)
	}
	path := "/fleet-partner/v1/routes/" + routeID
	if got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get").Object(t); got["routeReference"] != route["routeReference"] {
		t.Fatalf("get route: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchX), "GET", path, nil, nil), 404, "cross-branch get")

	stop := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path+"/stops", map[string]any{"stopType": "PICKUP", "orderId": "ord-1", "locationReference": "dock-1", "serviceDurationSeconds": 300}, nil), 201, "add stop").Object(t)
	stopPath := path + "/stops/" + stop["id"].(string)
	edit := map[string]any{"stopType": "PICKUP", "orderId": "ord-1", "locationReference": "dock-2", "serviceDurationSeconds": 600}
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", stopPath, edit, nil), 400, "edit stop without version")
	edit["version"] = 9
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", stopPath, edit, nil), 409, "edit stop stale")
	edit["version"] = 1
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", stopPath, edit, nil), 404, "cross-tenant stop edit")
	editedStop := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", stopPath, edit, nil), 200, "edit stop").Object(t)
	if editedStop["locationReference"] != "dock-2" || editedStop["serviceDurationSeconds"] != float64(600) || editedStop["version"] != float64(2) {
		t.Fatalf("edited stop: %v", editedStop)
	}
	if rowsOutboxCount(t, f.pool, stop["id"].(string), "route_stop.updated.v1") != 1 {
		t.Fatal("route_stop.updated event missing")
	}

	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path+"/cancel", map[string]any{}, nil), 400, "cancel without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path+"/cancel", map[string]any{"expectedVersion": 9}, nil), 409, "cancel stale")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", path+"/cancel", map[string]any{"expectedVersion": 1}, nil), 404, "cross-tenant cancel")
	current := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "reread").Object(t)
	cancelled := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path+"/cancel", map[string]any{"expectedVersion": current["version"]}, nil), 200, "cancel").Object(t)
	if cancelled["status"] != dispatch.RouteCancelled {
		t.Fatalf("cancelled route: %v", cancelled)
	}
	// A cancelled route's stops are frozen and it cannot be approved.
	edit["version"] = 2
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", stopPath, edit, nil), 409, "edit stop on cancelled route")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path+"/approve", map[string]any{"expectedVersion": cancelled["version"]}, nil), 409, "approve cancelled route")
	if rowsOutboxCount(t, f.pool, routeID, "route_plan.cancelled.v1") != 1 {
		t.Fatal("route_plan.cancelled event missing")
	}
}

// TestRow16_10b_TrackingList covers the untested part of FLEET-16.10:
// GET /tracking.
func TestRow16_10b_TrackingList(t *testing.T) {
	f := newRows16(t)
	ctx := context.Background()
	routeA := rowsExpect(t, f.route(t, f.A, f.branchA, "", "DISPATCH"), 201, "route A").Object(t)
	routeX := rowsExpect(t, f.route(t, f.A, f.branchX, "", "DISPATCH"), 201, "route X").Object(t)
	lat, lon := 6.45, 3.39
	ingest := func(org, trip, route, provider string, at time.Time) {
		t.Helper()
		if _, d, err := f.svc.IngestTracking(ctx, dispatch.TrackingEvent{EventID: ids.New("evt"), OrganizationID: org, SourceService: "driver-dispatch-delivery-service",
			CanonicalTripReference: trip, RoutePlanID: route, ProviderID: provider, VehicleOrTruckReference: "veh-" + provider, OperationalStatus: "IN_PROGRESS",
			Latitude: &lat, Longitude: &lon, SourceTimestamp: at, SourceVersion: 1}); err != nil || d != "APPLIED" {
			t.Fatalf("ingest %s: %s %v", trip, d, err)
		}
	}
	ingest(f.A.Org, "trip-a", routeA["id"].(string), "prv-1", f.now)
	ingest(f.A.Org, "trip-x", routeX["id"].(string), "prv-2", f.now.Add(-30*time.Minute))
	ingest(f.B.Org, "trip-b", "", "prv-1", f.now)

	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/tracking", nil, nil), 200, "list").Items(t)
	trips := rowsIDs(list, "canonicalTripReference")
	if len(list) != 2 || !trips["trip-a"] || !trips["trip-x"] || trips["trip-b"] {
		t.Fatalf("tracking list: %v", trips)
	}
	for _, p := range list {
		// Freshness is recomputed on read: an old position is never "live".
		if p["canonicalTripReference"] == "trip-x" && p["freshnessStatus"] == dispatch.Fresh {
			t.Fatalf("30-minute-old position reported fresh: %v", p)
		}
		if p["canonicalTripReference"] == "trip-a" && p["freshnessStatus"] != dispatch.Fresh {
			t.Fatalf("current position not fresh: %v", p)
		}
	}
	if byProvider := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/tracking?providerId=prv-2", nil, nil), 200, "provider filter").Items(t); len(byProvider) != 1 || byProvider[0]["canonicalTripReference"] != "trip-x" {
		t.Fatalf("provider filter: %v", byProvider)
	}
	if c := rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchA), "GET", "/fleet-partner/v1/tracking", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["canonicalTripReference"] != "trip-a" {
		t.Fatalf("confined tracking: %v", c)
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/tracking", nil, nil), 403, "no permission")
}

// TestRow16_11b_GeofenceListGetPatch covers FLEET-16.11b:
// GET /geofences, GET/PATCH /geofences/:id.
func TestRow16_11b_GeofenceListGetPatch(t *testing.T) {
	f := newRows16(t)
	circle := map[string]any{"center": map[string]any{"latitude": 6.45, "longitude": 3.39}, "radiusMeters": 500}
	mk := func(actor rowsActor, branch, name string) map[string]any {
		return rowsExpect(t, rowsDo(t, f.router, actor, "POST", "/fleet-partner/v1/geofences", map[string]any{"BranchID": branch, "Name": name,
			"GeometryType": "CIRCLE", "Geometry": circle, "EventRules": []string{"ENTER"}}, nil), 201, "create geofence").Object(t)
	}
	depot := mk(f.A, f.branchA, "Depot")
	port := mk(f.A, f.branchX, "Port")
	foreign := mk(f.B, f.branchB, "Foreign")

	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/geofences", nil, nil), 200, "list").Items(t)
	if ids := rowsIDs(list, "id"); len(list) != 2 || !ids[depot["id"].(string)] || !ids[port["id"].(string)] || ids[foreign["id"].(string)] {
		t.Fatalf("geofences: %v", ids)
	}
	if byBranch := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/geofences?branchId="+f.branchX, nil, nil), 200, "branch filter").Items(t); len(byBranch) != 1 || byBranch[0]["id"] != port["id"] {
		t.Fatalf("branch filter: %v", byBranch)
	}
	if c := rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchA), "GET", "/fleet-partner/v1/geofences", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["id"] != depot["id"] {
		t.Fatalf("confined geofences: %v", rowsIDs(c, "id"))
	}
	// A negative page window is refused before it can reach PostgreSQL.
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/geofences?offset=-1", nil, nil), 400, "negative offset")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/geofences", nil, nil), 403, "no permission")

	path := "/fleet-partner/v1/geofences/" + depot["id"].(string)
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get").Object(t)
	if got["name"] != "Depot" || got["activeStatus"] != "INACTIVE" {
		t.Fatalf("geofence: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchX), "GET", path, nil, nil), 404, "cross-branch get")

	polygon := map[string]any{"points": []map[string]any{{"latitude": 6.4, "longitude": 3.3}, {"latitude": 6.5, "longitude": 3.3}, {"latitude": 6.5, "longitude": 3.4}, {"latitude": 6.4, "longitude": 3.3}}}
	patch := map[string]any{"Name": "Depot (extended)", "GeometryType": "POLYGON", "Geometry": polygon, "EventRules": []string{"enter", "exit"}}
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, patch, nil), 400, "patch without version")
	patch["expectedVersion"] = 5
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, patch, nil), 409, "patch stale")
	patch["expectedVersion"] = 1
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", path, patch, nil), 409, "cross-tenant patch never touches the row")
	rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchX), "PATCH", path, patch, nil), 404, "cross-branch patch")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"GeometryType": "CIRCLE", "Geometry": map[string]any{"radiusMeters": 10}, "expectedVersion": 1}, nil), 400, "invalid geometry")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, patch, nil), 200, "patch").Object(t)
	rules, _ := patched["eventRules"].([]any)
	if patched["name"] != "Depot (extended)" || patched["geometryType"] != "POLYGON" || patched["version"] != float64(2) || len(rules) != 2 || rules[0] != "ENTER" {
		t.Fatalf("patched geofence: %v", patched)
	}
	if rowsOutboxCount(t, f.pool, depot["id"].(string), "geofence.updated.v1") != 1 {
		t.Fatal("geofence.updated event missing")
	}
}

// TestRow16_13_GetAlertAndJobReplay covers the untested part of FLEET-16.13:
// GET /alerts/:id and POST /alerts/jobs/:jobId/replay.
func TestRow16_13_GetAlertAndJobReplay(t *testing.T) {
	f := newRows16(t)
	ctx := context.Background()
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/dispatch/candidates?orderId=none-ord&branchId="+f.branchA, nil, nil), 200, "trigger alert")
	var alertID string
	if err := f.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND source_id='none-ord'`, f.A.Org).Scan(&alertID); err != nil {
		t.Fatal(err)
	}
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/alerts/"+alertID, nil, nil), 200, "get alert").Object(t)
	if got["alertType"] != "NO_ELIGIBLE_PROVIDER" || got["severity"] != "HIGH" || got["status"] == "" || got["branchId"] != f.branchA {
		t.Fatalf("alert: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/alerts/"+alertID, nil, nil), 404, "cross-tenant alert")
	rowsExpect(t, rowsDo(t, f.router, f.confined(f.branchX), "GET", "/fleet-partner/v1/alerts/"+alertID, nil, nil), 404, "cross-branch alert")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/alerts/"+alertID, nil, nil), 403, "no permission")

	queue := jobs.NewQueue(f.pool)
	deadLetter := func(org, jobType string) string {
		t.Helper()
		id, err := queue.Enqueue(ctx, f.pool, jobs.EnqueueRequest{OrganizationID: org, JobType: jobType, Payload: map[string]string{"organizationId": org}, MaxAttempts: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.pool.Exec(ctx, `UPDATE fleet_partner_jobs SET status=$1,dead_lettered_at=now() WHERE id=$2`, jobs.StatusDeadLettered, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	job := deadLetter(f.A.Org, dispatch.JobGeofenceEvaluate)
	foreignJob := deadLetter(f.B.Org, dispatch.JobGeofenceEvaluate)
	otherFeatureJob := deadLetter(f.A.Org, "section19.restriction_reconcile")
	replay := func(id string) string { return "/fleet-partner/v1/alerts/jobs/" + id + "/replay" }

	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "POST", replay(job), map[string]any{}, nil), 403, "replay without permission")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", replay(foreignJob), map[string]any{}, nil), 404, "replay another organization's job")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", replay("job_missing"), map[string]any{}, nil), 404, "replay unknown job")
	// The Section 16 replay permission does not reach other features' work.
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", replay(otherFeatureJob), map[string]any{}, nil), 404, "replay a non-Section-16 job")
	accepted := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", replay(job), map[string]any{}, nil), 202, "replay").Object(t)
	if accepted["jobId"] != job || accepted["status"] != "PENDING" {
		t.Fatalf("replay response: %v", accepted)
	}
	replayed, err := queue.Get(ctx, f.A.Org, job)
	if err != nil || replayed.Status != jobs.StatusPending {
		t.Fatalf("job after replay = %+v %v", replayed, err)
	}
	if rowsAuditCount(t, f.pool, f.A.Org, job) != 1 {
		t.Fatal("a job replay must be audited")
	}
	// Only dead-lettered work can be replayed.
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", replay(job), map[string]any{}, nil), 409, "replay a pending job")
	if untouched, _ := queue.Get(ctx, f.B.Org, foreignJob); untouched.Status != jobs.StatusDeadLettered {
		t.Fatalf("foreign job changed: %s", untouched.Status)
	}
}
