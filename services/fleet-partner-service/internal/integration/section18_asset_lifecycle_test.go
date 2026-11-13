package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type assetLifecycleFixture struct {
	pool                *pgxpool.Pool
	svc                 *assets.Service
	t                   tenant.Context
	org, branch, member string
	now                 time.Time
}

func newAssetLifecycleFixture(t *testing.T) *assetLifecycleFixture {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Asset Lifecycle Co", "S1812-"+ids.New("reg"))
	branch, member := ids.New("brn"), ids.New("mem")
	if _, e := pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,$3,'Depot','Africa/Lagos')`, branch, org.ID, "S1812-"+branch[len(branch)-6:]); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-hauling-service','HAULING',$3,'ORGANIZATION_SPONSORED','ACTIVE')`, member, org.ID, "provider-"+member); e != nil {
		t.Fatal(e)
	}
	media := sectionMedia{files: map[string]platformclients.FileMetadata{
		"org-evidence": {FileID: "org-evidence", OwnerID: org.ID, UploadState: "COMPLETED", ScanState: "CLEAN"},
	}}
	now := time.Now().UTC().Truncate(time.Second)
	svc := assets.NewService(assets.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Media: media, Flags: assets.Flags{Assets: true, Assignments: true, Inspections: true, Maintenance: true, Breakdowns: true, Inventory: true, Fuel: true, Workers: true}, Now: func() time.Time { return now }})
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: "user-18", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "user-18", CorrelationID: "section18-12"}
	return &assetLifecycleFixture{pool: pool, svc: svc, t: scope, org: org.ID, branch: branch, member: member, now: now}
}

func (f *assetLifecycleFixture) asset(t *testing.T, code string) assets.Asset {
	t.Helper()
	a, e := f.svc.CreateAsset(context.Background(), f.t, assets.Asset{BranchID: f.branch, AssetCode: code, AssetType: "TRUCK", OwnershipType: "OWNED", Capacity: 1000, CapacityUnit: "KG"})
	if e != nil {
		t.Fatal(e)
	}
	return a
}

func section18Router(t *testing.T, pool *pgxpool.Pool, svc *assets.Service) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: c.GetHeader("X-Test-Member"), ActorType: tenant.ActorTypeOrganizationUser, ActorID: c.GetHeader("X-Test-Member")})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	assets.NewHandler(svc, accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0), idempotency.NewStore(pool, 0)).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	return engine
}

var section18Key int

func section18Call(t *testing.T, router http.Handler, method, path, org, member string, body any, version int64) (int, map[string]any) {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, "/fleet-partner/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Org", org)
	req.Header.Set("X-Test-Member", member)
	section18Key++
	req.Header.Set("Idempotency-Key", fmt.Sprintf("s1812-%d-%s", section18Key, ids.New("k")))
	if version > 0 {
		req.Header.Set("If-Match", fmt.Sprint(version))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if data, ok := out["data"].(map[string]any); ok {
		return rec.Code, data
	}
	return rec.Code, out
}

func num(v any) int64 {
	f, _ := v.(float64)
	return int64(f)
}

// TestSection18BlockedAssetCannotBeAssignedOrReleasedWithoutAuthorization
// drives the safety-critical rule through the real HTTP surface: a critical
// defect blocks the asset atomically; the blocked asset cannot be assigned,
// edited back to service, released by a user without the return-to-service
// permission, nor released through an unrelated work order. Only the
// authorized return to service of its own completed repair releases it.
func TestSection18BlockedAssetCannotBeAssignedOrReleasedWithoutAuthorization(t *testing.T) {
	f := newAssetLifecycleFixture(t)
	ctx := context.Background()
	router := section18Router(t, f.pool, f.svc)
	owner := seedApprovalMember(t, f.pool, f.org, "rol_system_owner", "s18-owner")
	dispatcher := seedApprovalMember(t, f.pool, f.org, "rol_system_dispatcher", "s18-dispatcher")
	asset := f.asset(t, "BLK-1")

	code, defect := section18Call(t, router, "POST", "/defects", f.org, owner, map[string]any{"assetId": asset.ID, "severity": "CRITICAL", "description": "Brake line leaking"}, 0)
	if code != 201 || defect["critical"] != true || defect["workOrderId"] == "" {
		t.Fatalf("report critical defect=%d %v", code, defect)
	}
	defectID, repairID := defect["id"].(string), defect["workOrderId"].(string)
	blocked, _ := f.svc.GetAsset(ctx, f.t, asset.ID)
	if blocked.OperationalStatus != "BLOCKED" {
		t.Fatalf("critical defect did not block asset: %s", blocked.OperationalStatus)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_asset_safety_blocks WHERE asset_id=$1 AND source_type='DEFECT' AND source_reference=$2 AND status='ACTIVE'`, asset.ID, defectID); n != 1 {
		t.Fatalf("safety blocks=%d", n)
	}

	// Blocked asset cannot be assigned, even by a user allowed to assign.
	if code, out := section18Call(t, router, "POST", "/assets/"+asset.ID+"/assignments", f.org, dispatcher, map[string]any{"ProviderMembershipID": f.member, "AssignmentType": "PRIMARY"}, 0); code != 409 {
		t.Fatalf("blocked asset assigned=%d %v", code, out)
	}
	// Nor edited back into service.
	if code, out := section18Call(t, router, "PATCH", "/assets/"+asset.ID, f.org, owner, map[string]any{"OperationalStatus": "AVAILABLE", "Capacity": -1, "Odometer": -1, "OperatingHours": -1}, blocked.Version); code != 200 {
		t.Fatalf("patch=%d %v", code, out)
	}
	if still, _ := f.svc.GetAsset(ctx, f.t, asset.ID); still.OperationalStatus != "BLOCKED" {
		t.Fatalf("PATCH released a blocked asset: %s", still.OperationalStatus)
	}
	// A critical defect cannot be deferred or resolved before its repair.
	d, _ := f.svc.GetDefect(ctx, f.t, defectID)
	if code, _ := section18Call(t, router, "POST", "/defects/"+defectID+"/defer", f.org, owner, map[string]any{"reason": "later", "deferredUntil": f.now.Add(48 * time.Hour)}, d.Version); code != 409 {
		t.Fatalf("critical defect deferred=%d", code)
	}
	if code, _ := section18Call(t, router, "POST", "/defects/"+defectID+"/resolve", f.org, owner, map[string]any{"notes": "fixed"}, d.Version); code != 409 {
		t.Fatalf("critical defect resolved before repair=%d", code)
	}

	// An unrelated, fully completed work order cannot release the asset.
	unrelated, e := f.svc.CreateWorkOrder(ctx, f.t, assets.WorkOrder{AssetID: asset.ID, SourceType: "MANUAL", SourceReference: "wash-" + asset.ID, Priority: "LOW", RequiredWork: "Wash"})
	if e != nil {
		t.Fatal(e)
	}
	complete := func(id string, version int64) assets.WorkOrder {
		t.Helper()
		wo, x := f.svc.UpdateWorkOrder(ctx, f.t, id, "APPROVE", assets.WorkAction{ExpectedVersion: version})
		if x != nil {
			t.Fatal(x)
		}
		wo, x = f.svc.UpdateWorkOrder(ctx, f.t, id, "COMPLETE", assets.WorkAction{ExpectedVersion: wo.Version, EvidenceMediaIDs: []string{"org-evidence"}})
		if x != nil {
			t.Fatal(x)
		}
		return wo
	}
	unrelated = complete(unrelated.ID, unrelated.Version)
	if code, out := section18Call(t, router, "POST", "/maintenance/work-orders/"+unrelated.ID+"/return-to-service", f.org, owner, map[string]any{"reason": "washed", "evidenceMediaIds": []string{"org-evidence"}}, unrelated.Version); code != 409 {
		t.Fatalf("unrelated work order released asset=%d %v", code, out)
	}

	repair, e := f.svc.GetWorkOrder(ctx, f.t, repairID)
	if e != nil {
		t.Fatal(e)
	}
	repair = complete(repair.ID, repair.Version)
	// Without maintenance.return_to_service the release is refused.
	if code, _ := section18Call(t, router, "POST", "/maintenance/work-orders/"+repair.ID+"/return-to-service", f.org, dispatcher, map[string]any{"reason": "repaired", "evidenceMediaIds": []string{"org-evidence"}}, repair.Version); code != 403 {
		t.Fatalf("return to service without permission=%d", code)
	}
	if still, _ := f.svc.GetAsset(ctx, f.t, asset.ID); still.OperationalStatus != "BLOCKED" {
		t.Fatalf("asset released early: %s", still.OperationalStatus)
	}
	if code, out := section18Call(t, router, "POST", "/maintenance/work-orders/"+repair.ID+"/return-to-service", f.org, owner, map[string]any{"reason": "repaired and tested", "evidenceMediaIds": []string{"org-evidence"}}, repair.Version); code != 200 {
		t.Fatalf("authorized return to service=%d %v", code, out)
	}
	released, _ := f.svc.GetAsset(ctx, f.t, asset.ID)
	resolved, _ := f.svc.GetDefect(ctx, f.t, defectID)
	if released.OperationalStatus != "AVAILABLE" || resolved.Status != assets.DefectResolved {
		t.Fatalf("after release asset=%s defect=%s", released.OperationalStatus, resolved.Status)
	}
	if code, out := section18Call(t, router, "POST", "/assets/"+asset.ID+"/assignments", f.org, dispatcher, map[string]any{"ProviderMembershipID": f.member, "AssignmentType": "PRIMARY"}, 0); code != 201 {
		t.Fatalf("assignment after release=%d %v", code, out)
	}
	// Return to service on an asset no longer blocked is refused, not a 500.
	if code, _ := section18Call(t, router, "POST", "/maintenance/work-orders/"+unrelated.ID+"/return-to-service", f.org, owner, map[string]any{"reason": "again", "evidenceMediaIds": []string{"org-evidence"}}, unrelated.Version); code != 409 {
		t.Fatalf("release of unblocked asset=%d", code)
	}
}

func TestSection18UnassignProviderNeverUnblocksAndRespectsActiveWork(t *testing.T) {
	f := newAssetLifecycleFixture(t)
	ctx := context.Background()
	asset := f.asset(t, "UNA-1")
	assignment, e := f.svc.Assign(ctx, f.t, asset.ID, assets.Assignment{ProviderMembershipID: f.member, AssignmentType: "PRIMARY", StartAt: f.now.Add(-time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	asset, _ = f.svc.GetAsset(ctx, f.t, asset.ID)
	if asset.OperationalStatus != "ASSIGNED" {
		t.Fatalf("assigned asset status=%s", asset.OperationalStatus)
	}
	if _, e = f.svc.UnassignProvider(ctx, f.t, asset.ID, assets.UnassignInput{ExpectedVersion: asset.Version}); appStatus(e) != 400 {
		t.Fatalf("unassign without reason: %v", e)
	}
	if _, e = f.svc.UnassignProvider(ctx, f.t, asset.ID, assets.UnassignInput{Reason: "shift ended", ExpectedVersion: asset.Version + 7}); appStatus(e) != 409 {
		t.Fatalf("stale version: %v", e)
	}
	work := ids.New("pas")
	if _, e = f.pool.Exec(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,response_deadline,created_by)VALUES($1,$2,$3,'ROUTE_STOP',$4,$5,$6,$7,'HAULING','IN_PROGRESS',now()+interval '1 hour','test')`, work, f.org, f.branch, "stop-"+work, f.member, "provider-"+f.member, asset.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.UnassignProvider(ctx, f.t, asset.ID, assets.UnassignInput{Reason: "shift ended", ExpectedVersion: asset.Version}); appStatus(e) != 409 {
		t.Fatalf("unassign with active work: %v", e)
	}
	if _, e = f.pool.Exec(ctx, `UPDATE fleet_partner_private_assignments SET status='COMPLETED' WHERE id=$1`, work); e != nil {
		t.Fatal(e)
	}
	// A safety block raised while assigned must survive the unassignment.
	if _, e = f.svc.ReportDefect(ctx, f.t, assets.ReportDefectInput{AssetID: asset.ID, Severity: "CRITICAL", Description: "Cracked windscreen"}); e != nil {
		t.Fatal(e)
	}
	asset, _ = f.svc.GetAsset(ctx, f.t, asset.ID)
	ended, e := f.svc.UnassignProvider(ctx, f.t, asset.ID, assets.UnassignInput{AssignmentID: assignment.ID, Reason: "shift ended", ExpectedVersion: asset.Version})
	if e != nil || ended.Status != "COMPLETED" || ended.EndAt == nil {
		t.Fatalf("unassign %+v %v", ended, e)
	}
	after, _ := f.svc.GetAsset(ctx, f.t, asset.ID)
	if after.OperationalStatus != "BLOCKED" {
		t.Fatalf("unassignment released a blocked asset: %s", after.OperationalStatus)
	}
	if _, e = f.svc.UnassignProvider(ctx, f.t, asset.ID, assets.UnassignInput{Reason: "again", ExpectedVersion: after.Version}); appStatus(e) != 404 {
		t.Fatalf("second unassign: %v", e)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1 AND event_type='asset.unassigned.v1'`, asset.ID); n != 1 {
		t.Fatalf("unassigned events=%d", n)
	}
	other := f.t
	other.OrganizationID = createOrganization(t, f.pool, "Unassign Other", "S1812-O-"+ids.New("reg")).ID
	if _, e = f.svc.UnassignProvider(ctx, other, asset.ID, assets.UnassignInput{Reason: "x", ExpectedVersion: after.Version}); appStatus(e) != 404 {
		t.Fatalf("cross-tenant unassign: %v", e)
	}
}

func TestSection18MeterReadingsMonotonicAnomalyAndMaintenanceDue(t *testing.T) {
	f := newAssetLifecycleFixture(t)
	ctx := context.Background()
	asset := f.asset(t, "MTR-1")
	dueAt := float64(1000)
	schedule, e := f.svc.SaveSchedule(ctx, f.t, "", 0, assets.Schedule{AssetID: asset.ID, TriggerType: "MILEAGE", NextDueOdometer: &dueAt})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "FUEL_GAUGE", Reading: 1}); appStatus(e) != 400 {
		t.Fatalf("invalid meter type: %v", e)
	}
	first, e := f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ODOMETER", Reading: 900, RecordedAt: f.now.Add(-10 * time.Hour)})
	if e != nil || len(first.DueScheduleIDs) != 0 {
		t.Fatalf("first reading %+v %v", first, e)
	}
	if _, e = f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ODOMETER", Reading: 899, RecordedAt: f.now.Add(-9 * time.Hour)}); appStatus(e) != 409 {
		t.Fatalf("decreasing reading accepted: %v", e)
	}
	if _, e = f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ODOMETER", Reading: 950, RecordedAt: f.now.Add(-11 * time.Hour)}); appStatus(e) != 409 {
		t.Fatalf("out-of-order reading accepted: %v", e)
	}
	second, e := f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ODOMETER", Reading: 1200, RecordedAt: f.now.Add(-8 * time.Hour)})
	if e != nil || second.Anomaly || len(second.DueScheduleIDs) != 1 || second.DueScheduleIDs[0] != schedule.ID || second.PreviousReading != 900 {
		t.Fatalf("second reading %+v %v", second, e)
	}
	moved, _ := f.svc.GetAsset(ctx, f.t, asset.ID)
	if moved.Odometer != 1200 {
		t.Fatalf("asset odometer=%v", moved.Odometer)
	}
	// An implausible jump is recorded and flagged for review, never rejected.
	jump, e := f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ODOMETER", Reading: 9000, RecordedAt: f.now.Add(-7 * time.Hour)})
	if e != nil || !jump.Anomaly || len(jump.AnomalyReasons) == 0 {
		t.Fatalf("anomalous reading %+v %v", jump, e)
	}
	hours, e := f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ENGINE_HOURS", Reading: 10, RecordedAt: f.now.Add(-4 * time.Hour)})
	if e != nil || hours.Anomaly {
		t.Fatalf("first hours %+v %v", hours, e)
	}
	hours, e = f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ENGINE_HOURS", Reading: 20, RecordedAt: f.now.Add(-2 * time.Hour)})
	if e != nil || !hours.Anomaly || hours.AnomalyReasons[0] != "ENGINE_HOURS_EXCEED_ELAPSED_TIME" {
		t.Fatalf("engine hours anomaly %+v %v", hours, e)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type='meter_reading.anomaly_detected.v1'`, f.org); n != 2 {
		t.Fatalf("anomaly events=%d", n)
	}
	list, e := f.svc.ListMeterReadings(ctx, f.t, asset.ID, "ODOMETER")
	if e != nil || len(list) != 3 {
		t.Fatalf("readings %d %v", len(list), e)
	}
	if _, e = f.pool.Exec(ctx, `UPDATE fleet_partner_meter_readings SET reading=1 WHERE asset_id=$1`, asset.ID); e == nil {
		t.Fatal("meter readings are mutable")
	}
	// The advanced meter feeds preventive maintenance: the worker opens the
	// schedule's work order.
	if e = f.svc.RunMaintenance(ctx, f.org); e != nil {
		t.Fatal(e)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_work_orders WHERE organization_id=$1 AND source_type='MAINTENANCE_SCHEDULE' AND source_reference=$2`, f.org, schedule.ID); n != 1 {
		t.Fatalf("maintenance work orders=%d", n)
	}
	history, e := f.svc.AssetHistory(ctx, f.t, asset.ID)
	if e != nil || len(history) < 5 {
		t.Fatalf("asset history %d %v", len(history), e)
	}
	// Concurrent readings serialize on the asset: exactly one of two equal
	// later readings can follow the other without regressing.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, x := f.svc.RecordMeterReading(ctx, f.t, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ODOMETER", Reading: float64(9100 - i*50), RecordedAt: f.now.Add(-time.Hour)})
			errs <- x
		}(i)
	}
	wg.Wait()
	close(errs)
	succeeded := 0
	for x := range errs {
		if x == nil {
			succeeded++
		} else if appStatus(x) != 409 {
			t.Fatalf("concurrent reading error %v", x)
		}
	}
	final, _ := f.svc.GetAsset(ctx, f.t, asset.ID)
	if succeeded == 0 || final.Odometer < 9050 {
		t.Fatalf("concurrent readings succeeded=%d odometer=%v", succeeded, final.Odometer)
	}
	other := f.t
	other.OrganizationID = createOrganization(t, f.pool, "Meter Other", "S1812-M-"+ids.New("reg")).ID
	if _, e = f.svc.RecordMeterReading(ctx, other, assets.MeterReadingInput{AssetID: asset.ID, MeterType: "ODOMETER", Reading: 99999}); appStatus(e) != 404 {
		t.Fatalf("cross-tenant reading: %v", e)
	}
}

func TestSection18DefectsAndInspectionDerivedDefects(t *testing.T) {
	f := newAssetLifecycleFixture(t)
	ctx := context.Background()
	asset := f.asset(t, "DEF-1")
	minor, e := f.svc.ReportDefect(ctx, f.t, assets.ReportDefectInput{AssetID: asset.ID, Severity: "LOW", Description: "Mirror scratched"})
	if e != nil || minor.Critical || minor.Status != assets.DefectOpen {
		t.Fatalf("minor defect %+v %v", minor, e)
	}
	if a, _ := f.svc.GetAsset(ctx, f.t, asset.ID); a.OperationalStatus != "AVAILABLE" {
		t.Fatalf("minor defect blocked asset: %s", a.OperationalStatus)
	}
	if _, e = f.svc.ReportDefect(ctx, f.t, assets.ReportDefectInput{AssetID: asset.ID, SourceType: "INSPECTION", Severity: "LOW", Description: "x"}); appStatus(e) != 400 {
		t.Fatalf("system source accepted from API: %v", e)
	}
	if _, e = f.svc.DefectTransition(ctx, f.t, minor.ID, "DEFER", assets.DefectAction{ExpectedVersion: minor.Version, Reason: "next service"}); appStatus(e) != 400 {
		t.Fatalf("deferral without date: %v", e)
	}
	until := f.now.Add(72 * time.Hour)
	deferred, e := f.svc.DefectTransition(ctx, f.t, minor.ID, "DEFER", assets.DefectAction{ExpectedVersion: minor.Version, Reason: "next service", DeferredUntil: &until})
	if e != nil || deferred.Status != assets.DefectDeferred {
		t.Fatalf("defer %+v %v", deferred, e)
	}
	if _, e = f.svc.DefectTransition(ctx, f.t, minor.ID, "START_REPAIR", assets.DefectAction{ExpectedVersion: deferred.Version}); appStatus(e) != 400 {
		t.Fatalf("repair without work order: %v", e)
	}
	wo, e := f.svc.CreateWorkOrder(ctx, f.t, assets.WorkOrder{AssetID: asset.ID, SourceType: "MANUAL", SourceReference: "mirror-" + asset.ID, Priority: "LOW", RequiredWork: "Polish mirror"})
	if e != nil {
		t.Fatal(e)
	}
	repairing, e := f.svc.DefectTransition(ctx, f.t, minor.ID, "START_REPAIR", assets.DefectAction{ExpectedVersion: deferred.Version, WorkOrderID: wo.ID})
	if e != nil || repairing.Status != assets.DefectRepairing || repairing.WorkOrderID != wo.ID {
		t.Fatalf("start repair %+v %v", repairing, e)
	}
	if _, e = f.svc.DefectTransition(ctx, f.t, minor.ID, "RESOLVE", assets.DefectAction{ExpectedVersion: deferred.Version, Notes: "done"}); appStatus(e) != 409 {
		t.Fatalf("stale resolve: %v", e)
	}
	done, e := f.svc.DefectTransition(ctx, f.t, minor.ID, "RESOLVE", assets.DefectAction{ExpectedVersion: repairing.Version, Notes: "Polished"})
	if e != nil || done.Status != assets.DefectResolved || done.ResolvedAt == nil {
		t.Fatalf("resolve %+v %v", done, e)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_asset_defect_history WHERE defect_id=$1`, minor.ID); n != 4 {
		t.Fatalf("defect history=%d", n)
	}

	// A critical inspection failure turns each failed item into a defect;
	// the critical item is linked to the repair work order.
	tpl, e := f.svc.SaveTemplate(ctx, f.t, "", 0, assets.Template{Name: "Daily", ChecklistItems: json.RawMessage(`[{"code":"brakes","required":true,"critical":true},{"code":"wipers","required":true}]`), CriticalityRules: json.RawMessage(`{"criticalFailure":"BLOCK_ASSET"}`)})
	if e != nil {
		t.Fatal(e)
	}
	ins, e := f.svc.CreateInspection(ctx, f.t, assets.Inspection{AssetID: asset.ID, TemplateID: tpl.ID, InspectorMembershipID: f.member})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.CompleteInspection(ctx, f.t, ins.ID, assets.CompleteInspectionInput{ExpectedVersion: ins.Version, Answers: []assets.Answer{{ItemCode: "brakes", Passed: false}, {ItemCode: "wipers", Passed: false}}, DefectSummary: json.RawMessage(`[]`)}); e != nil {
		t.Fatal(e)
	}
	derived, e := f.svc.ListDefects(ctx, f.t, assets.DefectFilter{AssetID: asset.ID, Status: "OPEN"})
	if e != nil || len(derived) != 2 {
		t.Fatalf("inspection defects %d %v", len(derived), e)
	}
	for _, d := range derived {
		if d.ItemCode == "brakes" && (!d.Critical || d.WorkOrderID == "") {
			t.Fatalf("critical inspection defect %+v", d)
		}
		if d.ItemCode == "wipers" && d.Critical {
			t.Fatalf("non-critical item marked critical %+v", d)
		}
	}
	restricted := f.t
	restricted.PermittedBranchIDs = []string{"brn_elsewhere"}
	if _, e = f.svc.GetDefect(ctx, restricted, minor.ID); appStatus(e) != 404 {
		t.Fatalf("cross-branch defect read: %v", e)
	}
	if l, _ := f.svc.ListDefects(ctx, restricted, assets.DefectFilter{}); len(l) != 0 {
		t.Fatalf("cross-branch defects leaked %d", len(l))
	}
}

func TestSection18TyresPositionsRotationAndTread(t *testing.T) {
	f := newAssetLifecycleFixture(t)
	ctx := context.Background()
	asset := f.asset(t, "TYR-1")
	depth := 8.0
	a, e := f.svc.RegisterTyre(ctx, f.t, assets.RegisterTyreInput{BranchID: f.branch, SerialNumber: "SN-A-" + asset.ID, Brand: "Michelin", Size: "295/80R22.5", TreadDepthMM: &depth})
	if e != nil || a.Status != "IN_STOCK" {
		t.Fatalf("register %+v %v", a, e)
	}
	if _, e = f.svc.RegisterTyre(ctx, f.t, assets.RegisterTyreInput{BranchID: f.branch, SerialNumber: "SN-A-" + asset.ID}); appStatus(e) != 409 {
		t.Fatalf("duplicate serial: %v", e)
	}
	b, e := f.svc.RegisterTyre(ctx, f.t, assets.RegisterTyreInput{BranchID: f.branch, SerialNumber: "SN-B-" + asset.ID})
	if e != nil {
		t.Fatal(e)
	}
	a, e = f.svc.TyreTransition(ctx, f.t, a.ID, "INSTALL", assets.TyreAction{ExpectedVersion: a.Version, AssetID: asset.ID, Position: "fl"})
	if e != nil || a.Status != "INSTALLED" || a.Position != "FL" || a.InstalledAt == nil {
		t.Fatalf("install %+v %v", a, e)
	}
	if _, e = f.svc.TyreTransition(ctx, f.t, b.ID, "INSTALL", assets.TyreAction{ExpectedVersion: b.Version, AssetID: asset.ID, Position: "FL"}); appStatus(e) != 409 {
		t.Fatalf("occupied position accepted: %v", e)
	}
	b, e = f.svc.TyreTransition(ctx, f.t, b.ID, "INSTALL", assets.TyreAction{ExpectedVersion: b.Version, AssetID: asset.ID, Position: "FR"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.TyreTransition(ctx, f.t, a.ID, "ROTATE", assets.TyreAction{ExpectedVersion: a.Version, ToPosition: "FR"}); appStatus(e) != 409 {
		t.Fatalf("rotation onto occupied position: %v", e)
	}
	a, e = f.svc.TyreTransition(ctx, f.t, a.ID, "ROTATE", assets.TyreAction{ExpectedVersion: a.Version, ToPosition: "RL"})
	if e != nil || a.Position != "RL" {
		t.Fatalf("rotate %+v %v", a, e)
	}
	low := 1.0
	a, e = f.svc.TyreTransition(ctx, f.t, a.ID, "TREAD", assets.TyreAction{ExpectedVersion: a.Version, TreadDepthMM: &low})
	if e != nil || a.LowTreadDefectID == "" || a.LastTreadDepthMM == nil || *a.LastTreadDepthMM != 1.0 {
		t.Fatalf("low tread %+v %v", a, e)
	}
	again, e := f.svc.TyreTransition(ctx, f.t, a.ID, "TREAD", assets.TyreAction{ExpectedVersion: a.Version, TreadDepthMM: &low})
	if e != nil || again.LowTreadDefectID != "" {
		t.Fatalf("second low reading duplicated defect %+v %v", again, e)
	}
	if st, _ := f.svc.GetAsset(ctx, f.t, asset.ID); st.OperationalStatus != "AVAILABLE" {
		t.Fatalf("low tread (HIGH, non-critical) blocked the asset: %s", st.OperationalStatus)
	}
	if _, e = f.svc.TyreTransition(ctx, f.t, a.ID, "REMOVE", assets.TyreAction{ExpectedVersion: again.Version}); appStatus(e) != 400 {
		t.Fatalf("remove without reason: %v", e)
	}
	scrapped, e := f.svc.TyreTransition(ctx, f.t, a.ID, "REMOVE", assets.TyreAction{ExpectedVersion: again.Version, Reason: "worn out", Scrap: true})
	if e != nil || scrapped.Status != "SCRAPPED" || scrapped.AssetID != "" || scrapped.Position != "" {
		t.Fatalf("scrap %+v %v", scrapped, e)
	}
	if _, e = f.svc.TyreTransition(ctx, f.t, a.ID, "TREAD", assets.TyreAction{ExpectedVersion: scrapped.Version, TreadDepthMM: &low}); appStatus(e) != 409 {
		t.Fatalf("tread on scrapped tyre: %v", e)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM fleet_partner_tyre_events WHERE tyre_id=$1`, a.ID); n != 6 {
		t.Fatalf("tyre events=%d", n)
	}
	if _, e = f.pool.Exec(ctx, `DELETE FROM fleet_partner_tyre_events WHERE tyre_id=$1`, a.ID); e == nil {
		t.Fatal("tyre events are mutable")
	}
	installed, _ := f.svc.ListTyres(ctx, f.t, asset.ID, "INSTALLED")
	if len(installed) != 1 || installed[0].ID != b.ID {
		t.Fatalf("installed tyres %+v", installed)
	}
	other := f.t
	other.OrganizationID = createOrganization(t, f.pool, "Tyre Other", "S1812-T-"+ids.New("reg")).ID
	if _, e = f.svc.GetTyre(ctx, other, b.ID); appStatus(e) != 404 {
		t.Fatalf("cross-tenant tyre: %v", e)
	}
}

func TestSection18AssetCostHistoryMinorUnitsAndIsolation(t *testing.T) {
	f := newAssetLifecycleFixture(t)
	ctx := context.Background()
	router := section18Router(t, f.pool, f.svc)
	owner := seedApprovalMember(t, f.pool, f.org, "rol_system_owner", "s18-cost-owner")
	dispatcher := seedApprovalMember(t, f.pool, f.org, "rol_system_dispatcher", "s18-cost-dispatcher")
	asset := f.asset(t, "CST-1")
	otherAsset := f.asset(t, "CST-2")
	wo, e := f.svc.CreateWorkOrder(ctx, f.t, assets.WorkOrder{AssetID: asset.ID, SourceType: "MANUAL", SourceReference: "svc-" + asset.ID, Priority: "MEDIUM", RequiredWork: "Service"})
	if e != nil {
		t.Fatal(e)
	}
	otherWO, e := f.svc.CreateWorkOrder(ctx, f.t, assets.WorkOrder{AssetID: otherAsset.ID, SourceType: "MANUAL", SourceReference: "svc-" + otherAsset.ID, Priority: "MEDIUM", RequiredWork: "Service"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.RecordCost(ctx, f.t, asset.ID, assets.CostInput{CostType: "MAINTENANCE", SourceType: "WORK_ORDER", SourceReference: wo.ID, AmountMinor: 4500000, Currency: "ngn"}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.RecordCost(ctx, f.t, asset.ID, assets.CostInput{CostType: "PARTS", AmountMinor: 120050, Currency: "NGN", Description: "Filter"}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.RecordCost(ctx, f.t, asset.ID, assets.CostInput{CostType: "MAINTENANCE", AmountMinor: 500000, Currency: "NGN"}); e != nil {
		t.Fatal(e)
	}
	for name, in := range map[string]assets.CostInput{
		"zero amount":            {CostType: "FUEL", AmountMinor: 0, Currency: "NGN"},
		"bad currency":           {CostType: "FUEL", AmountMinor: 10, Currency: "NAIRA"},
		"bad type":               {CostType: "SALARY", AmountMinor: 10, Currency: "NGN"},
		"sourced without ref":    {CostType: "MAINTENANCE", SourceType: "WORK_ORDER", AmountMinor: 10, Currency: "NGN"},
		"future incurred":        {CostType: "FUEL", AmountMinor: 10, Currency: "NGN", IncurredAt: f.now.Add(48 * time.Hour)},
		"other asset work":       {CostType: "MAINTENANCE", SourceType: "WORK_ORDER", SourceReference: otherWO.ID, AmountMinor: 10, Currency: "NGN"},
		"unknown fuel record":    {CostType: "FUEL", SourceType: "FUEL_RECORD", SourceReference: "fur_missing", AmountMinor: 10, Currency: "NGN"},
		"unknown stock movement": {CostType: "PARTS", SourceType: "STOCK_MOVEMENT", SourceReference: "stm_missing", AmountMinor: 10, Currency: "NGN"},
	} {
		if _, x := f.svc.RecordCost(ctx, f.t, asset.ID, in); x == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	history, e := f.svc.ListCosts(ctx, f.t, asset.ID)
	if e != nil || len(history.Entries) != 3 {
		t.Fatalf("cost history %+v %v", history, e)
	}
	totals := map[string]int64{}
	for _, v := range history.Totals {
		totals[v.CostType+"/"+v.Currency] = v.AmountMinor
	}
	if totals["MAINTENANCE/NGN"] != 5000000 || totals["PARTS/NGN"] != 120050 {
		t.Fatalf("totals %v", totals)
	}
	if _, e = f.pool.Exec(ctx, `UPDATE fleet_partner_asset_cost_entries SET amount_minor=1 WHERE asset_id=$1`, asset.ID); e == nil {
		t.Fatal("cost entries are mutable")
	}
	// Cost data is organization financial data: a dispatcher without
	// assets.costs.read is refused; other organizations get a safe 404.
	if code, _ := section18Call(t, router, "GET", "/assets/"+asset.ID+"/costs", f.org, dispatcher, nil, 0); code != 403 {
		t.Fatalf("dispatcher cost read=%d", code)
	}
	if code, out := section18Call(t, router, "GET", "/assets/"+asset.ID+"/costs", f.org, owner, nil, 0); code != 200 || len(out["entries"].([]any)) != 3 {
		t.Fatalf("owner cost read=%d %v", code, out)
	}
	if code, out := section18Call(t, router, "POST", "/assets/"+asset.ID+"/costs", f.org, owner, map[string]any{"costType": "TYRES", "amountMinor": 9900000, "currency": "NGN"}, 0); code != 201 || num(out["amountMinor"]) != 9900000 {
		t.Fatalf("owner cost record=%d %v", code, out)
	}
	otherOrg := createOrganization(t, f.pool, "Cost Other", "S1812-C-"+ids.New("reg"))
	otherOwner := seedApprovalMember(t, f.pool, otherOrg.ID, "rol_system_owner", "s18-cost-other")
	if code, _ := section18Call(t, router, "GET", "/assets/"+asset.ID+"/costs", otherOrg.ID, otherOwner, nil, 0); code != 404 {
		t.Fatalf("cross-tenant cost read=%d", code)
	}
	if code, out := section18Call(t, router, "GET", "/assets/"+asset.ID+"/history", f.org, owner, nil, 0); code != 200 {
		t.Fatalf("history read=%d %v", code, out)
	}
}
