package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	dispatch "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privaterepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// rescueWorkStub resolves any route-stop work as ready in the given branch.
// Route-stop work has no Fleet-owned row to resolve beyond the assignment.
type rescueWorkStub struct{ branch string }

func (w rescueWorkStub) Resolve(context.Context, database.Querier, string, string, string) (privateusecases.WorkInfo, error) {
	return privateusecases.WorkInfo{BranchID: w.branch, Ready: true}, nil
}

type rescueFixture struct {
	pool        *pgxpool.Pool
	dispatch    *dispatch.Service
	private     *privateusecases.Service
	t           tenant.Context
	org, branch string
}

func newRescueFixture(t *testing.T) *rescueFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, "Rescue Logistics", "RESCUE-"+ids.New("reg"))
	branch := ids.New("brn")
	if _, e := pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone) VALUES($1,$2,$3,'Rescue','Africa/Lagos')`, branch, org.ID, "RSC-"+branch[len(branch)-6:]); e != nil {
		t.Fatal(e)
	}
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	private := privateusecases.NewService(privateusecases.Options{Pool: pool, Repository: privaterepos.NewRepository(pool), Work: rescueWorkStub{branch: branch}, Eligibility: privateEligibilityAlways{}, Events: publisher, Flags: privateusecases.Flags{Enabled: true, WritesEnabled: true, DispatchEnabled: true, HaulingEnabled: true}})
	svc := dispatch.NewService(dispatch.Options{Pool: pool, PrivateAssignments: private, Rescue: private, Candidates: candidateFactsStub{}, Events: publisher, Flags: dispatch.Flags{DispatchPlanning: true, ControlTower: true, TrackingProjections: true, Geofences: true, OperationalAlerts: true, Workers: true}})
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: "user-rescue", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "user-rescue", CorrelationID: "rescue-test"}
	return &rescueFixture{pool: pool, dispatch: svc, private: private, t: scope, org: org.ID, branch: branch}
}

func (f *rescueFixture) assignment(t *testing.T, status, membership, provider string) string {
	t.Helper()
	id := ids.New("pas")
	if _, e := f.pool.Exec(context.Background(), `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,response_deadline,created_by) VALUES($1,$2,$3,'ROUTE_STOP',$4,$5,$6,'vehicle-rescue','DISPATCH',$7,now()+interval '1 hour','test')`, id, f.org, f.branch, "stop-"+id, membership, provider, status); e != nil {
		t.Fatal(e)
	}
	return id
}

func (f *rescueFixture) respond(t *testing.T, assignmentID, provider, decision string) privatemodels.Assignment {
	t.Helper()
	in := privateusecases.RespondInput{ProviderID: provider, Decision: decision, IdempotencyKey: "respond-" + assignmentID, RequestFingerprint: "fp-" + assignmentID}
	if decision == "DECLINE" {
		in.ReasonCode = "FATIGUE_OR_REST_REQUIRED"
	}
	a, e := f.private.Respond(context.Background(), f.org, assignmentID, in)
	if e != nil {
		t.Fatalf("%s %s: %v", decision, assignmentID, e)
	}
	return a
}

func errStatus(e error) int {
	if e == nil {
		return 0
	}
	if errors.Is(e, dispatch.ErrConflict) {
		return http.StatusConflict
	}
	return apperrors.From(e).Status
}

func TestSection16RescueCaseReplacementRequiresProviderAcceptance(t *testing.T) {
	f := newRescueFixture(t)
	ctx := context.Background()
	original := f.assignment(t, "PENDING_PROVIDER_RESPONSE", "mem-a", "prov-a")

	// Validation.
	if _, e := f.dispatch.OpenRescueCase(ctx, f.t, dispatch.OpenRescueCaseInput{TriggerType: "BREAKDOWN", Severity: "HIGH", PrivateAssignmentID: original, IdempotencyKey: "k0"}); errStatus(e) != 400 {
		t.Fatalf("missing reason accepted: %v", e)
	}
	if _, e := f.dispatch.OpenRescueCase(ctx, f.t, dispatch.OpenRescueCaseInput{TriggerType: "MAGIC", Severity: "HIGH", Reason: "x", PrivateAssignmentID: original, IdempotencyKey: "k0"}); errStatus(e) != 400 {
		t.Fatalf("invalid trigger accepted: %v", e)
	}
	if _, e := f.dispatch.OpenRescueCase(ctx, f.t, dispatch.OpenRescueCaseInput{TriggerType: "BREAKDOWN", Severity: "HIGH", Reason: "x", PrivateAssignmentID: "pas_unknown", IdempotencyKey: "k0"}); errStatus(e) != 404 {
		t.Fatalf("unknown assignment status=%d %v", errStatus(e), e)
	}

	open := dispatch.OpenRescueCaseInput{TriggerType: "BREAKDOWN", Severity: "HIGH", Reason: "Truck broke down on the expressway", PrivateAssignmentID: original, IdempotencyKey: "rescue-1"}
	rc, e := f.dispatch.OpenRescueCase(ctx, f.t, open)
	if e != nil {
		t.Fatal(e)
	}
	if rc.Status != dispatch.RescueOpen || rc.BranchID != f.branch || rc.CurrentAssignmentID != original || rc.AlertID == "" || rc.Version != 1 {
		t.Fatalf("opened case %+v", rc)
	}
	replay, e := f.dispatch.OpenRescueCase(ctx, f.t, open)
	if e != nil || replay.ID != rc.ID {
		t.Fatalf("idempotent replay %+v %v", replay, e)
	}
	open.IdempotencyKey = "rescue-dup"
	if _, e = f.dispatch.OpenRescueCase(ctx, f.t, open); errStatus(e) != 409 {
		t.Fatalf("second open case for same work/trigger: %v", e)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND alert_type='RESCUE_REQUIRED' AND source_id=$2 AND status='OPEN'`, f.org, rc.ID); n != 1 {
		t.Fatalf("rescue alerts=%d", n)
	}

	// The original assignment still holds the work (awaiting its provider):
	// no replacement may be forced past it.
	repl := dispatch.RescueReplacementInput{ProviderMembershipID: "mem-b", ProviderID: "prov-b", VehicleOrTruckReference: "vehicle-b", ExpectedVersion: rc.Version}
	if _, e = f.dispatch.AssignRescueReplacement(ctx, f.t, rc.ID, repl); errStatus(e) != 409 {
		t.Fatalf("replacement while original active: %v", e)
	}
	f.respond(t, original, "prov-a", "DECLINE")
	stale := repl
	stale.ExpectedVersion = 99
	if _, e = f.dispatch.AssignRescueReplacement(ctx, f.t, rc.ID, stale); errStatus(e) != 409 {
		t.Fatalf("stale version accepted: %v", e)
	}
	same := repl
	same.ProviderMembershipID = "mem-a"
	if _, e = f.dispatch.AssignRescueReplacement(ctx, f.t, rc.ID, same); errStatus(e) != 400 {
		t.Fatalf("same provider re-offered: %v", e)
	}

	// Two dispatchers race to offer the work: exactly one offer is created.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, x := f.dispatch.AssignRescueReplacement(ctx, f.t, rc.ID, repl)
			results <- x
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for x := range results {
		if x == nil {
			ok++
		} else if errStatus(x) != 409 {
			t.Fatalf("racing offer unexpected error %v", x)
		}
	}
	if ok != 1 {
		t.Fatalf("concurrent offers succeeded=%d", ok)
	}
	rc, e = f.dispatch.GetRescueCase(ctx, f.t, rc.ID)
	if e != nil || rc.Status != dispatch.RescueReplacementPending || rc.ReplacementAttempts != 1 || rc.ReplacementAssignmentStatus != privatemodels.StatusPending {
		t.Fatalf("after offer %+v %v", rc, e)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_private_assignments WHERE organization_id=$1 AND provider_id='prov-b' AND status='PENDING_PROVIDER_RESPONSE'`, f.org); n != 1 {
		t.Fatalf("replacement assignments=%d", n)
	}
	if _, e = f.dispatch.ResolveRescueCase(ctx, f.t, rc.ID, dispatch.ResolveRescueCaseInput{ResolutionCode: "RECOVERED_ON_SITE", Notes: "n", ExpectedVersion: rc.Version}); errStatus(e) != 409 {
		t.Fatalf("resolve while replacement pending: %v", e)
	}

	// The replacement provider declines; the durable sweep reopens the case.
	f.respond(t, rc.ReplacementAssignmentID, "prov-b", "DECLINE")
	if e = f.dispatch.RunMaintenance(ctx); e != nil {
		t.Fatal(e)
	}
	rc, _ = f.dispatch.GetRescueCase(ctx, f.t, rc.ID)
	if rc.Status != dispatch.RescueOpen {
		t.Fatalf("declined replacement did not reopen case: %+v", rc)
	}

	// A second provider is offered the work and explicitly accepts it.
	repl = dispatch.RescueReplacementInput{ProviderMembershipID: "mem-c", ProviderID: "prov-c", ExpectedVersion: rc.Version}
	rc, e = f.dispatch.AssignRescueReplacement(ctx, f.t, rc.ID, repl)
	if e != nil || rc.ReplacementAttempts != 2 {
		t.Fatalf("second offer %+v %v", rc, e)
	}
	f.respond(t, rc.ReplacementAssignmentID, "prov-c", "ACCEPT")
	resolved, e := f.dispatch.ResolveRescueCase(ctx, f.t, rc.ID, dispatch.ResolveRescueCaseInput{ResolutionCode: "REPLACEMENT_COMPLETED", Notes: "Cargo transferred to replacement rider", ExpectedVersion: rc.Version})
	if e != nil || resolved.Status != dispatch.RescueResolved || resolved.ResolvedAt == nil {
		t.Fatalf("resolve %+v %v", resolved, e)
	}
	if _, e = f.dispatch.ResolveRescueCase(ctx, f.t, rc.ID, dispatch.ResolveRescueCaseInput{ResolutionCode: "NO_ACTION_REQUIRED", Notes: "n", ExpectedVersion: resolved.Version}); errStatus(e) != 409 {
		t.Fatalf("resolved twice: %v", e)
	}
	var alertStatus string
	_ = f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_operational_alerts WHERE id=$1`, rc.AlertID).Scan(&alertStatus)
	if alertStatus != "RESOLVED" {
		t.Fatalf("rescue alert status=%s", alertStatus)
	}
	for event, want := range map[string]int{"rescue_case.opened.v1": 1, "rescue_case.assigned.v1": 2, "rescue_case.replacement_declined.v1": 1, "rescue_case.replacement_accepted.v1": 1, "rescue_case.resolved.v1": 1} {
		if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND aggregate_id=$2 AND event_type=$3`, f.org, rc.ID, event); n != want {
			t.Errorf("%s events=%d want %d", event, n, want)
		}
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2`, f.org, rc.ID); n < 5 {
		t.Errorf("audit rows=%d", n)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_rescue_case_history WHERE rescue_case_id=$1`, rc.ID); n != 6 {
		t.Errorf("history rows=%d", n)
	}
	if _, e = f.pool.Exec(ctx, `UPDATE fleet_partner_rescue_case_history SET reason='x' WHERE rescue_case_id=$1`, rc.ID); e == nil {
		t.Fatal("rescue history is mutable")
	}

	// Tenant and branch isolation: other organizations and other branches see a
	// safe not-found.
	other := f.t
	other.OrganizationID = createOrganization(t, f.pool, "Rescue Other", "RESCUE-O-"+ids.New("reg")).ID
	if _, e = f.dispatch.GetRescueCase(ctx, other, rc.ID); errStatus(e) != 404 {
		t.Fatalf("cross-tenant read: %v", e)
	}
	if _, e = f.dispatch.ResolveRescueCase(ctx, other, rc.ID, dispatch.ResolveRescueCaseInput{ResolutionCode: "NO_ACTION_REQUIRED", Notes: "n", ExpectedVersion: resolved.Version}); errStatus(e) != 404 {
		t.Fatalf("cross-tenant write: %v", e)
	}
	restricted := f.t
	restricted.PermittedBranchIDs = []string{"brn_elsewhere"}
	if _, e = f.dispatch.GetRescueCase(ctx, restricted, rc.ID); errStatus(e) != 404 {
		t.Fatalf("cross-branch read: %v", e)
	}
	if list, _ := f.dispatch.ListRescueCases(ctx, restricted, dispatch.RescueFilter{}); len(list) != 0 {
		t.Fatalf("cross-branch list leaked %d", len(list))
	}
	if _, e = f.dispatch.OpenRescueCase(ctx, other, dispatch.OpenRescueCaseInput{TriggerType: "DELAY", Severity: "LOW", Reason: "x", PrivateAssignmentID: original, IdempotencyKey: "other"}); errStatus(e) != 404 {
		t.Fatalf("cross-tenant link accepted: %v", e)
	}
}

func TestSection16RescueCaseCancelAndTripOnlyCase(t *testing.T) {
	f := newRescueFixture(t)
	ctx := context.Background()
	if _, e := f.dispatch.OpenRescueCase(ctx, f.t, dispatch.OpenRescueCaseInput{TriggerType: "DELAY", Severity: "MEDIUM", Reason: "Stuck in traffic", CanonicalTripReference: "trip-rescue-1", IdempotencyKey: "trip-1"}); errStatus(e) != 400 {
		t.Fatalf("trip-only case without branch: %v", e)
	}
	rc, e := f.dispatch.OpenRescueCase(ctx, f.t, dispatch.OpenRescueCaseInput{BranchID: f.branch, TriggerType: "DELAY", Severity: "MEDIUM", Reason: "Stuck in traffic", CanonicalTripReference: "trip-rescue-1", IdempotencyKey: "trip-2"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.dispatch.AssignRescueReplacement(ctx, f.t, rc.ID, dispatch.RescueReplacementInput{ProviderMembershipID: "m", ProviderID: "p", ExpectedVersion: rc.Version}); errStatus(e) != 409 {
		t.Fatalf("replacement without assignment link: %v", e)
	}
	if _, e = f.dispatch.CancelRescueCase(ctx, f.t, rc.ID, dispatch.CancelRescueCaseInput{ExpectedVersion: rc.Version}); errStatus(e) != 400 {
		t.Fatalf("cancel without reason: %v", e)
	}
	cancelled, e := f.dispatch.CancelRescueCase(ctx, f.t, rc.ID, dispatch.CancelRescueCaseInput{Reason: "Traffic cleared", ExpectedVersion: rc.Version})
	if e != nil || cancelled.Status != dispatch.RescueCancelled {
		t.Fatalf("cancel %+v %v", cancelled, e)
	}
	var alertStatus string
	_ = f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_operational_alerts WHERE id=$1`, rc.AlertID).Scan(&alertStatus)
	if alertStatus != "DISMISSED" {
		t.Fatalf("alert after cancel=%s", alertStatus)
	}
	// A cancelled original releases the work: the replacement is a fresh
	// pending assignment for the same route stop, still awaiting acceptance.
	cancelledOriginal := f.assignment(t, "CANCELLED", "mem-d", "prov-d")
	cc, e := f.dispatch.OpenRescueCase(ctx, f.t, dispatch.OpenRescueCaseInput{TriggerType: "FAILED_DELIVERY", Severity: "LOW", Reason: "Recipient absent; redeliver", PrivateAssignmentID: cancelledOriginal, IdempotencyKey: "cancelled-orig"})
	if e != nil {
		t.Fatal(e)
	}
	cc, e = f.dispatch.AssignRescueReplacement(ctx, f.t, cc.ID, dispatch.RescueReplacementInput{ProviderMembershipID: "mem-e", ProviderID: "prov-e", ExpectedVersion: cc.Version})
	if e != nil || cc.Status != dispatch.RescueReplacementPending || cc.ReplacementAssignmentStatus != privatemodels.StatusPending {
		t.Fatalf("replacement after cancellation %+v %v", cc, e)
	}
	var stop string
	_ = f.pool.QueryRow(ctx, `SELECT route_stop_id FROM fleet_partner_private_assignments WHERE id=$1`, cc.ReplacementAssignmentID).Scan(&stop)
	if stop != "stop-"+cancelledOriginal {
		t.Fatalf("replacement work=%s", stop)
	}
	if _, e = f.dispatch.CancelRescueCase(ctx, f.t, cc.ID, dispatch.CancelRescueCaseInput{Reason: "x", ExpectedVersion: cc.Version}); errStatus(e) != 409 {
		t.Fatalf("cancel with pending replacement: %v", e)
	}
	// A closed case frees the subject for a new case.
	if _, e = f.dispatch.OpenRescueCase(ctx, f.t, dispatch.OpenRescueCaseInput{BranchID: f.branch, TriggerType: "DELAY", Severity: "MEDIUM", Reason: "Delayed again", CanonicalTripReference: "trip-rescue-1", IdempotencyKey: "trip-3"}); e != nil {
		t.Fatal(e)
	}
}

// rescueHTTPRouter mounts the real dispatch handler with the real authorizer
// and idempotency store behind a test-only tenant injector.
func rescueHTTPRouter(t *testing.T, pool *pgxpool.Pool, svc *dispatch.Service) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: c.GetHeader("X-Test-Member"), ActorType: tenant.ActorTypeOrganizationUser, ActorID: c.GetHeader("X-Test-Member")})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	dispatch.NewHandler(svc, accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0), idempotency.NewStore(pool, 0)).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	return engine
}

func rescueCall(t *testing.T, router http.Handler, method, path, org, member string, body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	if body == nil {
		raw = nil
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Org", org)
	req.Header.Set("X-Test-Member", member)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestSection16RescueAndGeofenceEventsHTTPBoundaries(t *testing.T) {
	f := newRescueFixture(t)
	router := rescueHTTPRouter(t, f.pool, f.dispatch)
	owner := seedApprovalMember(t, f.pool, f.org, "rol_system_owner", "rescue-owner")
	dispatcher := seedApprovalMember(t, f.pool, f.org, "rol_system_dispatcher", "rescue-dispatcher")
	noPermission := seedApprovalMember(t, f.pool, f.org, "rol_system_crm", "rescue-crm")
	assignment := f.assignment(t, "ACCEPTED", "mem-h", "prov-h")
	body := map[string]any{"triggerType": "PROVIDER_UNAVAILABLE", "severity": "HIGH", "reason": "Rider unreachable", "privateAssignmentId": assignment}

	if code, out := rescueCall(t, router, "POST", "/fleet-partner/v1/rescue-cases", f.org, noPermission, body, map[string]string{"Idempotency-Key": "h1"}); code != 403 {
		t.Fatalf("no permission open=%d %v", code, out)
	}
	if code, out := rescueCall(t, router, "POST", "/fleet-partner/v1/rescue-cases", f.org, owner, body, nil); code != 400 {
		t.Fatalf("missing idempotency key=%d %v", code, out)
	}
	code, out := rescueCall(t, router, "POST", "/fleet-partner/v1/rescue-cases", f.org, dispatcher, body, map[string]string{"Idempotency-Key": "h2"})
	if code != 201 {
		t.Fatalf("dispatcher open=%d %v", code, out)
	}
	data := out["data"].(map[string]any)
	id := data["id"].(string)
	if code, again := rescueCall(t, router, "POST", "/fleet-partner/v1/rescue-cases", f.org, dispatcher, body, map[string]string{"Idempotency-Key": "h2"}); code != 201 || again["data"].(map[string]any)["id"] != id {
		t.Fatalf("idempotent replay=%d %v", code, again)
	}
	if code, out := rescueCall(t, router, "GET", "/fleet-partner/v1/rescue-cases/"+id, f.org, noPermission, nil, nil); code != 403 {
		t.Fatalf("no permission read=%d %v", code, out)
	}
	if code, out := rescueCall(t, router, "GET", "/fleet-partner/v1/rescue-cases/"+id, f.org, owner, nil, nil); code != 200 {
		t.Fatalf("owner read=%d %v", code, out)
	}
	otherOrg := createOrganization(t, f.pool, "Rescue HTTP Other", "RESCUE-H-"+ids.New("reg"))
	otherOwner := seedApprovalMember(t, f.pool, otherOrg.ID, "rol_system_owner", "rescue-other-owner")
	if code, out := rescueCall(t, router, "GET", "/fleet-partner/v1/rescue-cases/"+id, otherOrg.ID, otherOwner, nil, nil); code != 404 {
		t.Fatalf("cross-tenant read=%d %v", code, out)
	}
	if code, out := rescueCall(t, router, "POST", "/fleet-partner/v1/rescue-cases/"+id+"/replacement", f.org, owner, map[string]any{"providerMembershipId": "mem-x", "providerId": "prov-x"}, map[string]string{"Idempotency-Key": "h3", "If-Match": "1"}); code != 409 {
		t.Fatalf("forced replacement while original active=%d %v", code, out)
	}
	if code, out := rescueCall(t, router, "GET", "/fleet-partner/v1/geofence-events", f.org, owner, nil, nil); code != 200 {
		t.Fatalf("geofence events read=%d %v", code, out)
	}
	if code, out := rescueCall(t, router, "GET", "/fleet-partner/v1/geofence-events", f.org, noPermission, nil, nil); code != 403 {
		t.Fatalf("geofence events without permission=%d %v", code, out)
	}
	if code, out := rescueCall(t, router, "GET", "/fleet-partner/v1/geofence-events?from=yesterday", f.org, owner, nil, nil); code != 400 {
		t.Fatalf("bad from=%d %v", code, out)
	}
}

// TestSection16GeofenceEventsReadScopedWithFreshness drives the production
// path end to end: an authoritative tracking event is ingested, the durable
// worker derives the geofence event, and the read surface returns it scoped
// to tenant and branch with read-time freshness.
func TestSection16GeofenceEventsReadScopedWithFreshness(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	clock := f.now
	var mu sync.Mutex
	now := func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	svc := dispatch.NewService(dispatch.Options{Pool: f.pool, Candidates: candidateFactsStub{}, Events: events.NewDomainPublisher(events.NewOutboxRepository(f.pool)), Flags: dispatch.Flags{ControlTower: true, TrackingProjections: true, Geofences: true, OperationalAlerts: true, Workers: true, TrackingSourceConfigured: true}, Now: now})
	geo, e := svc.CreateGeofence(ctx, f.t, dispatch.CreateGeofenceInput{BranchID: f.branch, Name: "Depot gate", GeometryType: "CIRCLE", Geometry: dispatch.Geometry{Center: &dispatch.Point{Latitude: 6.5, Longitude: 3.3}, RadiusMeters: 500}, EventRules: []string{"ENTER", "EXIT"}, IdempotencyKey: "geo-read"})
	if e != nil {
		t.Fatal(e)
	}
	if geo, e = svc.SetGeofenceActive(ctx, f.t, geo.ID, "ACTIVE", geo.Version); e != nil {
		t.Fatal(e)
	}
	worker := &jobs.Worker{Queue: jobs.NewQueue(f.pool), Metrics: observability.NewMetrics(), Owner: "section16-geofence-read", PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, HeartbeatInterval: 100 * time.Millisecond, BatchSize: 10}
	svc.RegisterWorkers(worker)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { worker.Run(workerCtx); close(done) }()
	defer func() { cancel(); <-done }()

	lat, lon := 6.5001, 3.3001
	trip := "trip-geo-read-" + ids.New("t")
	if _, _, e = svc.IngestTracking(ctx, dispatch.TrackingEvent{EventID: trip + "-1", OrganizationID: f.org, SourceService: "driver-dispatch-delivery-service", CanonicalTripReference: trip, OperationalStatus: "IN_PROGRESS", Latitude: &lat, Longitude: &lon, SourceTimestamp: f.now, SourceVersion: 1}); e != nil {
		t.Fatal(e)
	}
	waitForSection16Count(t, f.pool, `SELECT count(*) FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2 AND event_rule='ENTER'`, []any{f.org, geo.ID}, 1)

	list, e := svc.ListGeofenceEvents(ctx, f.t, dispatch.GeofenceEventFilter{GeofenceID: geo.ID})
	if e != nil || len(list) != 1 {
		t.Fatalf("events %+v %v", list, e)
	}
	if ev := list[0]; ev.EventRule != "ENTER" || ev.CanonicalTripReference != trip || ev.BranchID != f.branch || ev.GeofenceName != "Depot gate" || ev.DataFreshness != dispatch.Fresh || ev.SourceVersion != 1 {
		t.Fatalf("event %+v", ev)
	}
	// Filters.
	if l, _ := svc.ListGeofenceEvents(ctx, f.t, dispatch.GeofenceEventFilter{EventRule: "EXIT", GeofenceID: geo.ID}); len(l) != 0 {
		t.Fatalf("rule filter leaked %d", len(l))
	}
	future := f.now.Add(time.Hour)
	if l, _ := svc.ListGeofenceEvents(ctx, f.t, dispatch.GeofenceEventFilter{GeofenceID: geo.ID, From: &future}); len(l) != 0 {
		t.Fatalf("from filter leaked %d", len(l))
	}
	// Read-time freshness: the same event is reported STALE once its fix ages.
	mu.Lock()
	clock = f.now.Add(time.Hour)
	mu.Unlock()
	if l, _ := svc.ListGeofenceEvents(ctx, f.t, dispatch.GeofenceEventFilter{GeofenceID: geo.ID}); len(l) != 1 || l[0].DataFreshness != dispatch.Stale || l[0].AgeSeconds != 3600 {
		t.Fatalf("aged event %+v", l)
	}
	// Tenant and branch isolation.
	other := f.t
	other.OrganizationID = createOrganization(t, f.pool, "Geo Other", "GEO-O-"+ids.New("reg")).ID
	if l, _ := svc.ListGeofenceEvents(ctx, other, dispatch.GeofenceEventFilter{}); len(l) != 0 {
		t.Fatalf("cross-tenant events leaked %d", len(l))
	}
	restricted := f.t
	restricted.PermittedBranchIDs = []string{"brn_elsewhere"}
	if l, _ := svc.ListGeofenceEvents(ctx, restricted, dispatch.GeofenceEventFilter{}); len(l) != 0 {
		t.Fatalf("cross-branch events leaked %d", len(l))
	}
}

// TestSection16BoardNeverPresentsStaleLocationAsLive pins Section 16.10: the
// stored projection freshness is only as current as the last sweep, so the
// board must recompute it at read time.
func TestSection16BoardNeverPresentsStaleLocationAsLive(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	clock := f.now
	svc := dispatch.NewService(dispatch.Options{Pool: f.pool, Candidates: candidateFactsStub{}, Flags: dispatch.Flags{DispatchPlanning: true, ControlTower: true, TrackingProjections: true, Geofences: true, OperationalAlerts: true, TrackingSourceConfigured: true}, Now: func() time.Time { return clock }})
	lat, lon := 6.6, 3.4
	trip := "trip-board-fresh-" + ids.New("t")
	if _, _, e := svc.IngestTracking(ctx, dispatch.TrackingEvent{EventID: trip, OrganizationID: f.org, SourceService: "driver-dispatch-delivery-service", CanonicalTripReference: trip, OperationalStatus: "IN_PROGRESS", Latitude: &lat, Longitude: &lon, SourceTimestamp: f.now, SourceVersion: 1}); e != nil {
		t.Fatal(e)
	}
	var stored string
	_ = f.pool.QueryRow(ctx, `SELECT freshness_status FROM fleet_partner_tracking_projections WHERE canonical_trip_reference=$1`, trip).Scan(&stored)
	if stored != dispatch.Fresh {
		t.Fatalf("stored freshness=%s", stored)
	}
	clock = f.now.Add(time.Hour)
	board, e := svc.Board(ctx, f.t, dispatch.PageFilter{})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, trip2 := range board.ActiveTrips {
		if trip2.CanonicalTripReference == trip {
			found = true
			if trip2.FreshnessStatus != dispatch.Stale || trip2.SourceTimestamp.IsZero() {
				t.Fatalf("stale fix presented as %s", trip2.FreshnessStatus)
			}
		}
	}
	if !found || board.TrackingSummary[dispatch.Stale] < 1 || !board.ProjectionIncomplete {
		t.Fatalf("board found=%v summary=%v incomplete=%v", found, board.TrackingSummary, board.ProjectionIncomplete)
	}
	projection, e := svc.GetTracking(ctx, f.t, trip)
	if e != nil || projection.FreshnessStatus != dispatch.Stale {
		t.Fatalf("tracking read %+v %v", projection, e)
	}
}
