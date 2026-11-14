package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func ws4SpecializedRouter(t *testing.T, pool *pgxpool.Pool, flags specialized.Flags) (http.Handler, *specialized.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := specialized.NewService(specialized.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Flags: flags})
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: c.GetHeader("X-Test-Member"),
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: c.GetHeader("X-Test-Member"), CorrelationID: ids.New("cor"),
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	specialized.NewHandler(service, accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0), idempotency.NewStore(pool, 0)).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	return engine, service
}

func ws4Order(t *testing.T, pool *pgxpool.Pool, org, branch string) string {
	t.Helper()
	ctx := context.Background()
	customer, item, order := ids.New("cus"), ids.New("svc"), ids.New("ord")
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_company_customers(id,organization_id,customer_type,legal_name,display_name,normalized_name,duplicate_fingerprint,status,default_currency,created_by)VALUES($1,$2,'BUSINESS','Rider Co','Rider Co',$1,$1,'ACTIVE','NGN','test')`, customer, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_service_catalog_items(id,organization_id,name,code,category,created_by)VALUES($1,$2,'Staff Shuttle',$1,'SAME_DAY_DELIVERY','test')`, item, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_private_orders(id,organization_id,branch_id,order_number,customer_id,source,service_catalog_item_id,timezone,created_by)VALUES($1,$2,$3,$1,$4,'MANUAL',$5,'Africa/Lagos','test')`, order, org, branch, customer, item); err != nil {
		t.Fatal(err)
	}
	return order
}

// TestWS4SpecializedDisabledModulesExposeNoRoutes covers spec 20.1 / 20.13:
// with the feature or a module switched off for the deployment every route
// is the same 404 -- for writes as well as reads, and before any permission
// check -- while an enabled module still enforces the permission.
func TestWS4SpecializedDisabledModulesExposeNoRoutes(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "WS4 Specialized Off", "RC-WS4-SPOFF").ID
	owner := seedApprovalMember(t, pool, org, "rol_system_owner", "owner")
	noPermission := seedApprovalMember(t, pool, org, "rol_system_crm", "crm")
	body := map[string]any{"reason": "rollout", "configuration": map[string]any{}}
	idem := func(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }

	off, _ := ws4SpecializedRouter(t, pool, specialized.Flags{Enabled: false, Modules: map[string]bool{specialized.Courier: true}})
	for _, call := range []struct{ method, path string }{
		{"GET", "/fleet-partner/v1/specialized/capabilities"},
		{"POST", "/fleet-partner/v1/specialized/capabilities/COURIER_LAST_MILE/enable"},
		{"GET", "/fleet-partner/v1/specialized/COURIER_LAST_MILE/records"},
		{"POST", "/fleet-partner/v1/specialized/COURIER_LAST_MILE/records"},
		{"GET", "/fleet-partner/v1/capability-schemas/COURIER_LAST_MILE"},
		{"GET", "/fleet-partner/v1/orders/ord_x/specialized-data"},
		{"POST", "/fleet-partner/v1/orders/ord_x/specialized-data"},
		{"POST", "/fleet-partner/v1/specialized/COLD_CHAIN/records/r/temperature-events"},
	} {
		for _, member := range []string{owner, noPermission} {
			if code, _ := assetsIDORCall(t, off, call.method, call.path, org, member, body, idem("off-"+call.path)); code != http.StatusNotFound {
				t.Errorf("feature off: %s %s as %s = %d", call.method, call.path, member, code)
			}
		}
	}

	on, _ := ws4SpecializedRouter(t, pool, specialized.Flags{Enabled: true, Modules: map[string]bool{specialized.Courier: true, specialized.Medical: false, specialized.Taxi: false}})
	for _, path := range []string{
		"/fleet-partner/v1/specialized/MEDICAL_SENSITIVE/records",
		"/fleet-partner/v1/specialized/TAXI_PASSENGER/records",
		"/fleet-partner/v1/capability-schemas/MEDICAL_SENSITIVE",
		"/fleet-partner/v1/specialized/COLD_CHAIN/records/r/temperature-events",
	} {
		method := "GET"
		if path[len(path)-1] == 's' && path[len(path)-7:] == "-events" {
			method = "POST"
		}
		if code, _ := assetsIDORCall(t, on, method, path, org, noPermission, body, idem("mod-"+path)); code != http.StatusNotFound {
			t.Errorf("module off: %s %s = %d", method, path, code)
		}
	}
	if code, _ := assetsIDORCall(t, on, "POST", "/fleet-partner/v1/specialized/capabilities/MEDICAL_SENSITIVE/enable", org, owner, body, idem("enable-medical")); code != http.StatusNotFound {
		t.Errorf("enabling a module switched off for the deployment = %d", code)
	}
	// An enabled module still checks the underlying permission.
	if code, _ := assetsIDORCall(t, on, "GET", "/fleet-partner/v1/specialized/COURIER_LAST_MILE/records", org, noPermission, nil, nil); code != http.StatusForbidden {
		t.Errorf("enabled module without permission = %d", code)
	}
	// Not enabled for this organization: schema and records stay hidden.
	if code, _ := assetsIDORCall(t, on, "GET", "/fleet-partner/v1/capability-schemas/COURIER_LAST_MILE", org, owner, nil, nil); code != http.StatusNotFound {
		t.Errorf("schema of a capability the organization has not enabled = %d", code)
	}
	if code, _ := assetsIDORCall(t, on, "POST", "/fleet-partner/v1/specialized/capabilities/COURIER_LAST_MILE/enable", org, owner, body, idem("enable-courier")); code != http.StatusOK {
		t.Fatalf("enable courier = %d", code)
	}
	code, schema := assetsIDORCall(t, on, "GET", "/fleet-partner/v1/capability-schemas/COURIER_LAST_MILE", org, owner, nil, nil)
	if code != http.StatusOK || schema["schemaId"] != "COURIER_LAST_MILE.v2" || len(schema["recordTypes"].([]any)) != 5 {
		t.Fatalf("schema = %d %v", code, schema)
	}
	code, listed := assetsIDORCall(t, on, "GET", "/fleet-partner/v1/specialized/capabilities", org, owner, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("list capabilities = %d %v", code, listed)
	}
}

// TestWS4OrderSpecializedDataAndPassengerRecords covers spec 20.3, 20.14 and
// 50.3: typed order extension data for passenger transport, closed-schema
// rejection, required asset and qualification references, backward-compatible
// schema versions, restricted visibility with access audit, tenant and branch
// isolation, idempotency and the specialized_order.validated event.
func TestWS4OrderSpecializedDataAndPassengerRecords(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	scope, branch := section1921Scope(t, pool)
	org := scope.OrganizationID
	owner := seedApprovalMember(t, pool, org, "rol_system_owner", "owner")
	order := ws4Order(t, pool, org, branch)
	flags := specialized.Flags{Enabled: true, Modules: map[string]bool{specialized.Taxi: true, specialized.Towing: true, specialized.Medical: true, specialized.Hauling: true}}
	router, service := ws4SpecializedRouter(t, pool, flags)
	scope.OrganizationUserID, scope.ActorID = owner, owner
	for _, code := range []string{specialized.Taxi, specialized.Towing, specialized.Medical} {
		if _, err := service.SetCapability(ctx, scope, code, "ENABLED", "pilot", 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	post := func(key string, body map[string]any) (int, map[string]any) {
		return assetsIDORCall(t, router, "POST", "/fleet-partner/v1/orders/"+order+"/specialized-data", org, owner, body, map[string]string{"Idempotency-Key": key})
	}
	manifest := map[string]any{"capabilityCode": "TAXI_PASSENGER", "recordType": "PASSENGER_MANIFEST", "payload": map[string]any{
		"passengers": []any{map[string]any{"passengerReference": "pax-1", "seatNumber": "1"}, map[string]any{"passengerReference": "pax-2"}}, "seatCapacity": 3}}
	code, created := post("manifest-1", manifest)
	if code != http.StatusCreated || created["schemaId"] != "TAXI_PASSENGER.v2" || created["restrictedVisibility"] != true || created["subjectType"] != "ORDER" || created["branchId"] != branch {
		t.Fatalf("manifest = %d %v", code, created)
	}
	if code, replay := post("manifest-1", manifest); code != http.StatusCreated || replay["id"] != created["id"] {
		t.Fatalf("idempotent replay = %d %v", code, replay)
	}

	over := map[string]any{"capabilityCode": "TAXI_PASSENGER", "recordType": "PASSENGER_MANIFEST", "payload": map[string]any{
		"passengers": []any{map[string]any{"passengerReference": "a"}, map[string]any{"passengerReference": "b"}}, "seatCapacity": 1}}
	if code, _ := post("manifest-over", over); code != http.StatusBadRequest {
		t.Fatalf("over capacity = %d", code)
	}
	pii := map[string]any{"capabilityCode": "TAXI_PASSENGER", "recordType": "NO_SHOW", "payload": map[string]any{"passengerReference": "pax-1", "waitedMinutes": 5, "recordedAt": "2026-11-13T09:00:00Z", "phone": "+2348000000000"}}
	if code, _ := post("no-show-pii", pii); code != http.StatusBadRequest {
		t.Fatalf("unsupported passenger field = %d", code)
	}
	noShow := map[string]any{"capabilityCode": "TAXI_PASSENGER", "recordType": "NO_SHOW", "payload": map[string]any{"passengerReference": "pax-1", "waitedMinutes": 5, "recordedAt": "2026-11-13T09:00:00Z"}}
	if code, _ := post("no-show", noShow); code != http.StatusCreated {
		t.Fatalf("no-show = %d", code)
	}
	if code, _ := post("haulage-not-enabled", map[string]any{"capabilityCode": "TRUCKING_HAULAGE", "recordType": "HAULAGE_PROFILE", "payload": map[string]any{"loadType": "FTL", "haulingAuthorityReference": "h"}}); code != http.StatusNotFound {
		t.Fatalf("capability not enabled for the organization = %d", code)
	}

	// Required asset: the towing asset must be the organization's.
	towing := func(asset string) map[string]any {
		return map[string]any{"capabilityCode": "TOWING_RECOVERY", "recordType": "TOWING_RECOVERY", "payload": map[string]any{"breakdownReference": "b", "disabledVehicleReference": "v", "incidentLocation": "km 12", "towingAssetReference": asset, "destination": "depot"}}
	}
	if code, _ := post("tow-missing", towing("ast_missing")); code != http.StatusBadRequest {
		t.Fatalf("unknown towing asset = %d", code)
	}
	if code, _ := post("tow-ok", towing(ws4Asset(t, pool, org, branch))); code != http.StatusCreated {
		t.Fatalf("towing with own asset = %d", code)
	}
	// Required qualification: must be an active qualification of the organization.
	qualification := ids.New("qal")
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_qualifications(id,organization_id,code,name)VALUES($1,$2,$1,'Cold chain medical handling')`, qualification, org); err != nil {
		t.Fatal(err)
	}
	medical := func(q string) map[string]any {
		return map[string]any{"capabilityCode": "MEDICAL_SENSITIVE", "recordType": "MEDICAL_HANDLING", "payload": map[string]any{"requiredQualificationIds": []any{q}, "handlingInstructions": "upright, 2-8C"}}
	}
	if code, _ := post("med-missing", medical("qal_missing")); code != http.StatusBadRequest {
		t.Fatalf("unknown qualification = %d", code)
	}
	if code, _ := post("med-ok", medical(qualification)); code != http.StatusCreated {
		t.Fatalf("medical with own qualification = %d", code)
	}

	// Backward-compatible extension version: a version 1 payload stays valid
	// when pinned to version 1 and keeps validating under it on update.
	legacy := map[string]any{"capabilityCode": "TAXI_PASSENGER", "recordType": "PASSENGER_MANIFEST", "schemaVersion": 1, "payload": map[string]any{"passengers": []any{"opaque-1"}, "seatCapacity": 2, "legacyNote": "x"}}
	code, legacyRecord := post("manifest-legacy", legacy)
	if code != http.StatusCreated || legacyRecord["schemaId"] != "TAXI_PASSENGER.v1" {
		t.Fatalf("legacy manifest = %d %v", code, legacyRecord)
	}
	updated, err := service.UpdateRecord(ctx, scope, specialized.Taxi, legacyRecord["id"].(string), 1, specialized.Record{Payload: json.RawMessage(`{"passengers":["opaque-1","opaque-2"],"seatCapacity":2,"legacyNote":"y"}`)}, "")
	if err != nil || updated.SchemaVersion != 1 {
		t.Fatalf("legacy update = %+v %v", updated, err)
	}
	unpinned := map[string]any{"capabilityCode": "TAXI_PASSENGER", "recordType": "PASSENGER_MANIFEST", "payload": legacy["payload"]}
	if code, _ := post("manifest-legacy-unpinned", unpinned); code != http.StatusBadRequest {
		t.Fatalf("legacy payload under the current version = %d", code)
	}
	if code, _ := post("manifest-v9", map[string]any{"capabilityCode": "TAXI_PASSENGER", "recordType": "PASSENGER_MANIFEST", "schemaVersion": 9, "payload": manifest["payload"]}); code != http.StatusBadRequest {
		t.Fatalf("unknown schema version = %d", code)
	}

	// Reads: all of the order's data, restricted reads audited.
	before := 0
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_sensitive_access_audit WHERE organization_id=$1`, org).Scan(&before); err != nil {
		t.Fatal(err)
	}
	code, listed := assetsIDORCall(t, router, "GET", "/fleet-partner/v1/orders/"+order+"/specialized-data", org, owner, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("list order data = %d %v", code, listed)
	}
	var after int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_sensitive_access_audit WHERE organization_id=$1`, org).Scan(&after); err != nil || after-before != 4 {
		t.Fatalf("sensitive access audit rows = %d %v", after-before, err)
	}
	records, err := service.ListOrderData(ctx, scope, order)
	if err != nil || len(records) != 5 {
		t.Fatalf("order data = %d %v", len(records), err)
	}
	// Record lists redact restricted payloads.
	list, err := service.ListRecords(ctx, scope, specialized.Taxi)
	if err != nil || len(list) == 0 || string(list[0].Payload) != `{"redacted":true}` {
		t.Fatalf("restricted list payload = %v %v", list, err)
	}

	// Tenant and branch isolation.
	otherScope, otherBranch := section1921Scope(t, pool)
	foreignOrder := ws4Order(t, pool, otherScope.OrganizationID, otherBranch)
	if code, _ := assetsIDORCall(t, router, "POST", "/fleet-partner/v1/orders/"+foreignOrder+"/specialized-data", org, owner, manifest, map[string]string{"Idempotency-Key": "foreign"}); code != http.StatusNotFound {
		t.Fatalf("cross-tenant order = %d", code)
	}
	if code, _ := assetsIDORCall(t, router, "GET", "/fleet-partner/v1/orders/"+foreignOrder+"/specialized-data", org, owner, nil, nil); code != http.StatusNotFound {
		t.Fatalf("cross-tenant order read = %d", code)
	}
	narrow := scope
	narrow.PermittedBranchIDs = []string{ws4Branch(t, pool, org)}
	if _, err = service.ListOrderData(ctx, narrow, order); appStatus(err) != 404 {
		t.Fatalf("other-branch order = %v", err)
	}
	if ws4CountOutbox(t, pool, org, "specialized_order.validated.v1") != 5 {
		t.Fatal("specialized_order.validated events missing")
	}
	// Disabling a capability hides its order data at once.
	capabilities, _ := service.ListCapabilities(ctx, scope)
	for _, c := range capabilities {
		if c.CapabilityCode == specialized.Taxi {
			if _, err = service.SetCapability(ctx, scope, specialized.Taxi, "DISABLED", "pilot ended", c.Version, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	if records, err = service.ListOrderData(ctx, scope, order); err != nil || len(records) != 2 {
		t.Fatalf("order data after disabling taxi = %d %v", len(records), err)
	}
}
