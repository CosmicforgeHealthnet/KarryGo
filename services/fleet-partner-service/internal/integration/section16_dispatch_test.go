package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	dispatch "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type candidateFactsStub struct {
	facts []dispatch.CandidateFact
	err   error
}

func (s candidateFactsStub) Facts(context.Context, string, string, string, string) ([]dispatch.CandidateFact, error) {
	return s.facts, s.err
}

type assignmentTxStub struct{}

func (assignmentTxStub) CreateInTransaction(ctx context.Context, tx pgx.Tx, t tenant.Context, in privateusecases.CreateInput) (privatemodels.Assignment, error) {
	id := ids.New("pas")
	deadline := time.Now().Add(time.Hour)
	_, e := tx.Exec(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,response_deadline,created_by) VALUES($1,$2,$3,'ROUTE_STOP',$4,$5,$6,$7,$8,'PENDING_PROVIDER_RESPONSE',$9,$10)`, id, t.OrganizationID, in.BranchID, in.WorkID, in.ProviderMembershipID, in.ProviderID, in.VehicleOrTruckReference, in.ExecutionService, deadline, t.ActorID)
	return privatemodels.Assignment{ID: id, OrganizationID: t.OrganizationID, BranchID: in.BranchID, WorkOrigin: "ROUTE_STOP", RouteStopID: in.WorkID, ProviderMembershipID: in.ProviderMembershipID, ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference, ExecutionService: in.ExecutionService, Status: privatemodels.StatusPending, ResponseDeadline: deadline, Version: 1}, e
}

type optimizationStub struct{ submitCalls, readCalls int }

func (s *optimizationStub) Submit(ctx context.Context, input dispatch.OptimizationInput) (string, bool, error) {
	s.submitCalls++
	return "provider-job-16", true, nil
}
func (s *optimizationStub) ReadBack(ctx context.Context, ref string) (dispatch.OptimizationResult, bool, error) {
	s.readCalls++
	return dispatch.OptimizationResult{Sequence: []string{"stop"}, EstimatedDistance: 12.5, EstimatedDurationSeconds: 900, ProviderReference: "contract-fake", ProviderJobReference: ref, ResultVersion: 1}, true, nil
}

type section16Fixture struct {
	pool        *pgxpool.Pool
	s           *dispatch.Service
	t           tenant.Context
	org, branch string
	now         time.Time
}

func newSection16Fixture(t *testing.T) *section16Fixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, "Section 16 Logistics", "SEC16-"+ids.New("reg"))
	branchID := ids.New("brn")
	if _, e := pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone) VALUES($1,$2,'SEC16','Section 16','Europe/Rome')`, branchID, org.ID); e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	fact := dispatch.CandidateFact{ProviderID: "provider-16", ProviderMembershipID: "membership-16", VehicleReference: "vehicle-16", ExecutionService: "DISPATCH", MembershipActive: true, ProviderActive: true, CapabilityMatch: true, Available: true, VehicleEligible: true, CapacityOK: true, BranchAllowed: true, CargoAllowed: true, SafetyAllowed: true, ReservationFree: true, ActiveWorkFree: true, TrackingFreshness: dispatch.Fresh, SourceVersions: map[string]int64{"provider": 1, "vehicle": 1}}
	svc := dispatch.NewService(dispatch.Options{Pool: pool, PrivateAssignments: assignmentTxStub{}, Candidates: candidateFactsStub{facts: []dispatch.CandidateFact{fact}}, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Flags: dispatch.Flags{DispatchPlanning: true, RoutePlanning: true, RoutePublication: true, ControlTower: true, TrackingProjections: true, Geofences: true, OperationalAlerts: true, Workers: true, TrackingSourceConfigured: true}, Now: func() time.Time { return now }})
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: "user-planner", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "user-planner", CorrelationID: "section16-test"}
	return &section16Fixture{pool, svc, scope, org.ID, branchID, now}
}
func (f *section16Fixture) plan(t *testing.T) dispatch.DispatchPlan {
	t.Helper()
	v, e := f.s.CreatePlan(context.Background(), f.t, dispatch.CreatePlanInput{BranchID: f.branch, PlanDate: f.now, WorkWindowStart: f.now, WorkWindowEnd: f.now.Add(8 * time.Hour), Timezone: "Europe/Rome", IdempotencyKey: "plan-1"})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func (f *section16Fixture) route(t *testing.T, plan string) dispatch.RoutePlan {
	t.Helper()
	v, e := f.s.CreateRoute(context.Background(), f.t, dispatch.CreateRouteInput{BranchID: f.branch, DispatchPlanID: plan, RouteReference: "R-" + ids.New("ref"), RouteType: "DELIVERY", ProviderMembershipID: "membership-16", ProviderID: "provider-16", VehicleOrTruckReference: "vehicle-16", ExecutionService: "DISPATCH", ConstraintSnapshot: json.RawMessage(`{"capacity":100}`), IdempotencyKey: "route-" + ids.New("key")})
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestSection16DispatchPlansTenantBranchStateAndConcurrency(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	plan := f.plan(t)
	if plan.Status != dispatch.PlanDraft || plan.Version != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	replay, e := f.s.CreatePlan(ctx, f.t, dispatch.CreatePlanInput{BranchID: f.branch, PlanDate: f.now, WorkWindowStart: f.now, WorkWindowEnd: f.now.Add(8 * time.Hour), Timezone: "Europe/Rome", IdempotencyKey: "plan-1"})
	if e != nil || replay.ID != plan.ID {
		t.Fatalf("idempotency replay=%+v err=%v", replay, e)
	}
	denied := f.t
	denied.PermittedBranchIDs = []string{"brn_other"}
	if _, e = f.s.GetPlan(ctx, denied, plan.ID); e == nil {
		t.Fatal("branch IDOR was not denied")
	}
	if _, e = f.s.ApprovePlan(ctx, f.t, plan.ID, plan.Version, "approve-self"); e == nil {
		t.Fatal("planner approved own plan")
	}
	approver := f.t
	approver.OrganizationUserID = "user-approver"
	approver.ActorID = "user-approver"
	var wg sync.WaitGroup
	wg.Add(2)
	success, conflict := 0, 0
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		go func(n int) {
			defer wg.Done()
			_, x := f.s.ApprovePlan(ctx, approver, plan.ID, plan.Version, fmt.Sprintf("approve-%d", n))
			mu.Lock()
			defer mu.Unlock()
			if x == nil {
				success++
			} else {
				conflict++
			}
		}(i)
	}
	wg.Wait()
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent approval success=%d conflict=%d", success, conflict)
	}
	approved, e := f.s.GetPlan(ctx, approver, plan.ID)
	if e != nil || approved.Status != dispatch.PlanApproved {
		t.Fatalf("approved=%+v err=%v", approved, e)
	}
	published, e := f.s.PublishPlan(ctx, approver, approved.ID, approved.Version, "publish-plan")
	if e != nil || published.Status != dispatch.PlanPublished {
		t.Fatalf("published=%+v err=%v", published, e)
	}
	var history, audit, outbox int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_dispatch_route_history WHERE organization_id=$1 AND aggregate_id=$2`, f.org, plan.ID).Scan(&history)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2`, f.org, plan.ID).Scan(&audit)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND aggregate_id=$2`, f.org, plan.ID).Scan(&outbox)
	if history < 3 || audit < 3 || outbox < 3 {
		t.Fatalf("history=%d audit=%d outbox=%d", history, audit, outbox)
	}
}

func TestSection16RoutesStopsPublicationAndReplan(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	plan := f.plan(t)
	route := f.route(t, plan.ID)
	types := []string{"PICKUP", "PICKUP", "DELIVERY", "WAREHOUSE_TRANSFER", "RETURN"}
	for i, kind := range types {
		start := f.now.Add(time.Duration(i) * time.Hour)
		if _, e := f.s.AddStop(ctx, f.t, route.ID, dispatch.RouteStop{StopType: kind, OrderID: "order-1", ShipmentID: "shipment-1", SourceOrderStopID: fmt.Sprintf("ost-%d", i), LocationReference: fmt.Sprintf("loc-%d", i), TimeWindowStart: &start, TimeWindowEnd: ptrTime(start.Add(time.Hour)), ServiceDurationSeconds: 300}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.s.AddStop(ctx, f.t, route.ID, dispatch.RouteStop{StopType: "PASSENGER_PICKUP"}); e == nil {
		t.Fatal("passenger stop did not fail closed")
	}
	stops, e := f.s.ListStops(ctx, f.t, route.ID)
	if e != nil || len(stops) != 5 {
		t.Fatalf("stops=%d err=%v", len(stops), e)
	}
	for i, st := range stops {
		if st.Sequence != i+1 {
			t.Fatalf("non-contiguous stops=%+v", stops)
		}
	}
	if e = f.s.DeleteStop(ctx, f.t, route.ID, stops[1].ID); e != nil {
		t.Fatal(e)
	}
	stops, _ = f.s.ListStops(ctx, f.t, route.ID)
	for i, st := range stops {
		if st.Sequence != i+1 {
			t.Fatalf("sequence after delete=%+v", stops)
		}
	}
	route, e = f.s.ApproveRoute(ctx, f.t, route.ID, route.Version)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	success := 0
	mu := sync.Mutex{}
	for i := 0; i < 2; i++ {
		go func(n int) {
			defer wg.Done()
			_, _, x := f.s.PublishRoute(ctx, f.t, route.ID, dispatch.PublishRouteInput{ExpectedVersion: route.Version, IdempotencyKey: fmt.Sprintf("publish-%d", n)})
			if x == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("concurrent publication successes=%d", success)
	}
	published, e := f.s.GetRoute(ctx, f.t, route.ID)
	if e != nil || published.Status != dispatch.RoutePublished {
		t.Fatalf("published=%+v err=%v", published, e)
	}
	var assignments, executions, snapshots int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_private_assignments WHERE organization_id=$1 AND route_stop_id IN(SELECT id FROM fleet_partner_route_stops WHERE route_plan_id=$2)`, f.org, route.ID).Scan(&assignments)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_private_execution_operations WHERE organization_id=$1`, f.org).Scan(&executions)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_route_publication_snapshots WHERE route_plan_id=$1`, route.ID).Scan(&snapshots)
	if assignments != len(stops) || executions != 0 || snapshots != 1 {
		t.Fatalf("assignments=%d executions=%d snapshots=%d", assignments, executions, snapshots)
	}
	if _, e = f.s.EditRoute(ctx, f.t, route.ID, dispatch.EditRouteInput{ExpectedVersion: published.Version, ProviderID: "other"}); e == nil {
		t.Fatal("published route was directly edited")
	}
	_, _ = f.pool.Exec(ctx, `UPDATE fleet_partner_route_stops SET status='COMPLETED' WHERE id=$1`, stops[0].ID)
	replacement, e := f.s.Replan(ctx, f.t, route.ID, published.Version, "replan-1", "vehicle failure")
	if e != nil {
		t.Fatal(e)
	}
	replacementStops, _ := f.s.ListStops(ctx, f.t, replacement.ID)
	if replacement.ReplacementForRouteID != route.ID || len(replacementStops) != len(stops)-1 {
		t.Fatalf("replacement=%+v stops=%d", replacement, len(replacementStops))
	}
}
func ptrTime(v time.Time) *time.Time { return &v }

func TestSection16TrackingGapConflictFreshnessGeofenceAndAlerts(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	lat, lon := 0.001, 0.001
	base := dispatch.TrackingEvent{EventID: "evt-1", OrganizationID: f.org, SourceService: "driver-dispatch-delivery-service", CanonicalTripReference: "trip-16", ProviderID: "provider-16", VehicleOrTruckReference: "vehicle-16", OperationalStatus: "IN_PROGRESS", Latitude: &lat, Longitude: &lon, SourceTimestamp: f.now, SourceVersion: 1}
	p, d, e := f.s.IngestTracking(ctx, base)
	if e != nil || d != "APPLIED" || p.FreshnessStatus != dispatch.Fresh {
		t.Fatalf("projection=%+v disposition=%s err=%v", p, d, e)
	}
	_, d, e = f.s.IngestTracking(ctx, base)
	if e != nil || d != "DUPLICATE" {
		t.Fatalf("duplicate disposition=%s err=%v", d, e)
	}
	gap := base
	gap.EventID = "evt-3"
	gap.SourceVersion = 3
	gap.SourceTimestamp = f.now.Add(time.Minute)
	_, d, e = f.s.IngestTracking(ctx, gap)
	if e != nil || d != "GAP_DETECTED" {
		t.Fatalf("gap disposition=%s err=%v", d, e)
	}
	lower := base
	lower.EventID = "evt-2"
	lower.SourceVersion = 2
	_, d, e = f.s.IngestTracking(ctx, lower)
	if e != nil || d != "LOWER_VERSION_IGNORED" {
		t.Fatalf("lower disposition=%s err=%v", d, e)
	}
	conflict := gap
	conflict.EventID = "evt-3-conflict"
	conflict.OperationalStatus = "FAILED"
	_, d, e = f.s.IngestTracking(ctx, conflict)
	if e != nil || d != "CONFLICT" {
		t.Fatalf("conflict disposition=%s err=%v", d, e)
	}
	projected, e := f.s.GetTracking(ctx, f.t, "trip-16")
	if e != nil || projected.GapStatus != "GAP_DETECTED" || projected.OperationalStatus != "IN_PROGRESS" {
		t.Fatalf("unsafe overwrite projection=%+v err=%v", projected, e)
	}
	sequential := gap
	sequential.EventID = "evt-4"
	sequential.SourceVersion = 4
	sequential.SourceTimestamp = f.now.Add(2 * time.Minute)
	projected, d, e = f.s.IngestTracking(ctx, sequential)
	if e != nil || d != "GAP_DETECTED" || projected.GapStatus != "GAP_DETECTED" {
		t.Fatalf("an ordinary later event cleared a gap: projection=%+v disposition=%s err=%v", projected, d, e)
	}
	reconciled := sequential
	reconciled.EventID = "evt-5-reconciled"
	reconciled.SourceVersion = 5
	reconciled.ReconcilesThroughVersion = 5
	reconciled.SourceTimestamp = f.now.Add(3 * time.Minute)
	projected, d, e = f.s.IngestTracking(ctx, reconciled)
	if e != nil || d != "RECONCILED" || projected.GapStatus != "CURRENT" || projected.LastReconciledAt == nil {
		t.Fatalf("authoritative reconciliation failed: projection=%+v disposition=%s err=%v", projected, d, e)
	}
	reconcileWorker := &jobs.Worker{Queue: jobs.NewQueue(f.pool), Metrics: observability.NewMetrics(), Owner: "section16-reconcile-worker", PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, HeartbeatInterval: 100 * time.Millisecond, BatchSize: 10}
	f.s.RegisterWorkers(reconcileWorker)
	reconcileCtx, stopReconcile := context.WithCancel(ctx)
	reconcileDone := make(chan struct{})
	go func() { reconcileWorker.Run(reconcileCtx); close(reconcileDone) }()
	waitForSection16Count(t, f.pool, `SELECT count(*) FROM fleet_partner_jobs WHERE organization_id=$1 AND job_type=$2 AND status='SUCCEEDED'`, []any{f.org, dispatch.JobTrackingReconcile}, 1)
	stopReconcile()
	<-reconcileDone
	geo, e := f.s.CreateGeofence(ctx, f.t, dispatch.CreateGeofenceInput{BranchID: f.branch, Name: "Depot", GeometryType: "CIRCLE", Geometry: dispatch.Geometry{Center: &dispatch.Point{Latitude: 0, Longitude: 0}, RadiusMeters: 1000}, EventRules: []string{"ENTER", "EXIT"}, IdempotencyKey: "geo-1"})
	if e != nil {
		t.Fatal(e)
	}
	geo, e = f.s.SetGeofenceActive(ctx, f.t, geo.ID, "ACTIVE", geo.Version)
	if e != nil {
		t.Fatal(e)
	}
	alerts, e := f.s.ListAlerts(ctx, f.t, dispatch.PageFilter{})
	if e != nil || len(alerts) != 1 || alerts[0].AlertType != "TRACKING_GAP" {
		t.Fatalf("alerts=%+v err=%v", alerts, e)
	}
	ack, e := f.s.AcknowledgeAlert(ctx, f.t, alerts[0].ID, alerts[0].Version)
	if e != nil || ack.OwnerUserID != f.t.OrganizationUserID {
		t.Fatalf("ack=%+v err=%v", ack, e)
	}
	if _, e = f.s.ResolveAlert(ctx, f.t, ack.ID, ack.Version, "", ""); e == nil {
		t.Fatal("resolution without action/reason succeeded")
	}
	resolved, e := f.s.ResolveAlert(ctx, f.t, ack.ID, ack.Version, "RECONCILED", "authoritative read-back restored continuity")
	if e != nil || resolved.Status != dispatch.AlertResolved {
		t.Fatalf("resolved=%+v err=%v", resolved, e)
	}
	var alertCount int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND alert_type='TRACKING_GAP'`, f.org).Scan(&alertCount)
	if alertCount != 1 {
		t.Fatalf("alert storm count=%d", alertCount)
	}
	var conflicts int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_tracking_projection_conflicts WHERE organization_id=$1`, f.org).Scan(&conflicts)
	if conflicts != 1 {
		t.Fatalf("conflicts=%d", conflicts)
	}
}

func TestSection16BoardIsolationFiltersAndNoCommercialLeak(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	plan := f.plan(t)
	route := f.route(t, plan.ID)
	board, e := f.s.Board(ctx, f.t, dispatch.PageFilter{BranchID: f.branch, Service: "DISPATCH"})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(board)
	for _, forbidden := range []string{"fare", "commission", "margin", "settlementAmount", "customerPrice"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("commercial field leaked: %s", forbidden)
		}
	}
	denied := f.t
	denied.PermittedBranchIDs = []string{"other"}
	if _, e = f.s.Board(ctx, denied, dispatch.PageFilter{BranchID: f.branch}); e == nil {
		t.Fatal("board branch isolation failed")
	}
	if len(board.ActiveRoutes) != 0 || route.Status != dispatch.RouteDraft {
		t.Fatalf("draft route treated active: %+v", board.ActiveRoutes)
	}
	if board.CapacitySummary == nil || board.ProviderAvailability == nil || board.VehicleAvailability == nil || board.ProjectionGaps == nil {
		t.Fatalf("board authority summaries are not explicit: %+v", board)
	}
}

func TestSection16OptimizationDurableUnknownOutcomeReadBack(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	plan := f.plan(t)
	route := f.route(t, plan.ID)
	if _, e := f.s.AddStop(ctx, f.t, route.ID, dispatch.RouteStop{StopType: "PICKUP", LocationReference: "depot"}); e != nil {
		t.Fatal(e)
	}
	optimizer := &optimizationStub{}
	svc := dispatch.NewService(dispatch.Options{Pool: f.pool, Candidates: candidateFactsStub{}, Optimizer: optimizer, Flags: dispatch.Flags{RoutePlanning: true, RouteOptimization: true, Workers: true, OptimizationProviderConfigured: true}, Now: func() time.Time { return f.now }})
	request, e := svc.Optimize(ctx, f.t, route.ID, route.Version, "optimize-1")
	if e != nil || request.Status != "PENDING" {
		t.Fatalf("request=%+v err=%v", request, e)
	}
	var jobsCount int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_jobs WHERE organization_id=$1 AND job_type=$2`, f.org, dispatch.JobOptimizationSubmit).Scan(&jobsCount); e != nil || jobsCount != 1 {
		t.Fatalf("jobs=%d err=%v", jobsCount, e)
	}
	worker := &jobs.Worker{Queue: jobs.NewQueue(f.pool), Metrics: observability.NewMetrics(), Owner: "section16-worker", PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, HeartbeatInterval: 100 * time.Millisecond, BatchSize: 5}
	svc.RegisterWorkers(worker)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { worker.Run(workerCtx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, x := dispatch.NewRepository(f.pool).GetOptimization(ctx, nil, f.org, request.ID)
		if x == nil && current.Status == "SUCCEEDED" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	current, e := dispatch.NewRepository(f.pool).GetOptimization(ctx, nil, f.org, request.ID)
	if e != nil || current.Status != "SUCCEEDED" || optimizer.submitCalls != 1 || optimizer.readCalls < 1 {
		t.Fatalf("current=%+v submit=%d read=%d err=%v", current, optimizer.submitCalls, optimizer.readCalls, e)
	}
	optimized, e := svc.GetRoute(ctx, f.t, route.ID)
	if e != nil || optimized.Status != dispatch.RouteOptimized || optimized.PlannedDistance < 12 {
		t.Fatalf("route=%+v err=%v", optimized, e)
	}
}

func TestSection16GeofenceWorkerTransitionsDedupesAndProtectsStaleTracking(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	plan := f.plan(t)
	route := f.route(t, plan.ID)
	geo, e := f.s.CreateGeofence(ctx, f.t, dispatch.CreateGeofenceInput{BranchID: f.branch, Name: "Route corridor", GeometryType: "CIRCLE", Geometry: dispatch.Geometry{Center: &dispatch.Point{Latitude: 0, Longitude: 0}, RadiusMeters: 1000}, EventRules: []string{"ENTER", "EXIT", "DWELL", "ROUTE_DEVIATION"}, IdempotencyKey: "geo-worker"})
	if e != nil {
		t.Fatal(e)
	}
	geo, e = f.s.SetGeofenceActive(ctx, f.t, geo.ID, "ACTIVE", geo.Version)
	if e != nil {
		t.Fatal(e)
	}
	worker := &jobs.Worker{Queue: jobs.NewQueue(f.pool), Metrics: observability.NewMetrics(), Owner: "section16-geofence-worker", PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, HeartbeatInterval: 100 * time.Millisecond, BatchSize: 10}
	f.s.RegisterWorkers(worker)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { worker.Run(workerCtx); close(done) }()
	defer func() { cancel(); <-done }()

	lat, lon := 0.001, 0.001
	event := dispatch.TrackingEvent{EventID: "geo-event-1", OrganizationID: f.org, SourceService: "driver-dispatch-delivery-service", CanonicalTripReference: "geo-trip", RoutePlanID: route.ID, OperationalStatus: "IN_PROGRESS", Latitude: &lat, Longitude: &lon, SourceTimestamp: f.now, SourceVersion: 1}
	if _, _, e = f.s.IngestTracking(ctx, event); e != nil {
		t.Fatal(e)
	}
	waitForSection16Count(t, f.pool, `SELECT count(*) FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2 AND event_rule='ENTER'`, []any{f.org, geo.ID}, 1)

	event.EventID = "geo-event-2"
	event.SourceVersion = 2
	event.SourceTimestamp = f.now.Add(6 * time.Minute)
	if _, _, e = f.s.IngestTracking(ctx, event); e != nil {
		t.Fatal(e)
	}
	waitForSection16Count(t, f.pool, `SELECT count(*) FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2 AND event_rule='DWELL'`, []any{f.org, geo.ID}, 1)

	outLat, outLon := 1.0, 1.0
	event.EventID = "geo-event-3"
	event.SourceVersion = 3
	event.SourceTimestamp = f.now.Add(7 * time.Minute)
	event.Latitude, event.Longitude = &outLat, &outLon
	if _, _, e = f.s.IngestTracking(ctx, event); e != nil {
		t.Fatal(e)
	}
	waitForSection16Count(t, f.pool, `SELECT count(*) FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2 AND event_rule IN('EXIT','ROUTE_DEVIATION')`, []any{f.org, geo.ID}, 2)

	var enterCount, alertCount, beforeStale int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2 AND event_rule='ENTER'`, f.org, geo.ID).Scan(&enterCount)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND source_id=$2 AND alert_type IN('GEOFENCE_ENTRY','GEOFENCE_EXIT','ROUTE_DEVIATION')`, f.org, geo.ID).Scan(&alertCount)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2`, f.org, geo.ID).Scan(&beforeStale)
	if enterCount != 1 || alertCount != 3 {
		t.Fatalf("transition dedupe/alerts enter=%d alerts=%d", enterCount, alertCount)
	}
	event.EventID = "geo-event-4-stale"
	event.SourceVersion = 4
	event.SourceTimestamp = f.now.Add(-time.Hour)
	event.Latitude, event.Longitude = &lat, &lon
	if _, _, e = f.s.IngestTracking(ctx, event); e != nil {
		t.Fatal(e)
	}
	time.Sleep(150 * time.Millisecond)
	var afterStale int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2`, f.org, geo.ID).Scan(&afterStale)
	if afterStale != beforeStale {
		t.Fatalf("stale tracking created a high-confidence geofence event: before=%d after=%d", beforeStale, afterStale)
	}
}

func TestSection16MaintenanceProjectsRouteCompletionAndDeclineAlert(t *testing.T) {
	f := newSection16Fixture(t)
	ctx := context.Background()
	plan := f.plan(t)
	publishOne := func(key string) (dispatch.RoutePlan, dispatch.RouteStop, string) {
		route := f.route(t, plan.ID)
		stop, e := f.s.AddStop(ctx, f.t, route.ID, dispatch.RouteStop{StopType: "DELIVERY", LocationReference: "customer-" + key})
		if e != nil {
			t.Fatal(e)
		}
		route, e = f.s.ApproveRoute(ctx, f.t, route.ID, route.Version)
		if e != nil {
			t.Fatal(e)
		}
		route, assignmentIDs, e := f.s.PublishRoute(ctx, f.t, route.ID, dispatch.PublishRouteInput{ExpectedVersion: route.Version, IdempotencyKey: "publish-" + key})
		if e != nil || len(assignmentIDs) != 1 {
			t.Fatalf("publish route=%+v assignments=%v err=%v", route, assignmentIDs, e)
		}
		return route, stop, assignmentIDs[0]
	}
	completedRoute, completedStop, completedAssignment := publishOne("complete")
	declinedRoute, _, declinedAssignment := publishOne("decline")
	if _, e := f.pool.Exec(ctx, `UPDATE fleet_partner_private_assignments SET status='COMPLETED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, f.org, completedAssignment); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(ctx, `UPDATE fleet_partner_private_assignments SET status='DECLINED',version=version+1,declined_at=now(),updated_at=now() WHERE organization_id=$1 AND id=$2`, f.org, declinedAssignment); e != nil {
		t.Fatal(e)
	}
	if e := f.s.RunMaintenance(ctx); e != nil {
		t.Fatal(e)
	}
	worker := &jobs.Worker{Queue: jobs.NewQueue(f.pool), Metrics: observability.NewMetrics(), Owner: "section16-progress-worker", PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, HeartbeatInterval: 100 * time.Millisecond, BatchSize: 10}
	f.s.RegisterWorkers(worker)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { worker.Run(workerCtx); close(done) }()
	defer func() { cancel(); <-done }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		completed, _ := f.s.GetRoute(ctx, f.t, completedRoute.ID)
		declined, _ := f.s.GetRoute(ctx, f.t, declinedRoute.ID)
		if completed.Status == dispatch.RouteCompleted && declined.Status == dispatch.RouteFailed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	completed, e := f.s.GetRoute(ctx, f.t, completedRoute.ID)
	if e != nil || completed.Status != dispatch.RouteCompleted {
		t.Fatalf("completed route did not converge: route=%+v err=%v", completed, e)
	}
	stopProjection, e := f.s.ListStops(ctx, f.t, completedRoute.ID)
	if e != nil || len(stopProjection) != 1 || stopProjection[0].ID != completedStop.ID || stopProjection[0].Status != dispatch.StopCompleted {
		t.Fatalf("completed stop did not converge: stops=%+v err=%v", stopProjection, e)
	}
	declined, e := f.s.GetRoute(ctx, f.t, declinedRoute.ID)
	if e != nil || declined.Status != dispatch.RouteFailed {
		t.Fatalf("declined route did not become actionable: route=%+v err=%v", declined, e)
	}
	var alerts, escalationJobs int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND assignment_id=$2 AND alert_type='PROVIDER_DECLINED'`, f.org, declinedAssignment).Scan(&alerts)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_jobs j JOIN fleet_partner_operational_alerts a ON j.organization_id=a.organization_id AND j.dedupe_key='alert-sla:'||a.id WHERE a.organization_id=$1 AND a.assignment_id=$2`, f.org, declinedAssignment).Scan(&escalationJobs)
	if alerts != 1 || escalationJobs != 1 {
		t.Fatalf("decline alert/SLA job alerts=%d escalationJobs=%d", alerts, escalationJobs)
	}
}

func waitForSection16Count(t *testing.T, pool *pgxpool.Pool, query string, args []any, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var got int
		if e := pool.QueryRow(context.Background(), query, args...).Scan(&got); e == nil && got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	var got int
	e := pool.QueryRow(context.Background(), query, args...).Scan(&got)
	t.Fatalf("timed out waiting for count=%d got=%d err=%v", want, got, e)
}
