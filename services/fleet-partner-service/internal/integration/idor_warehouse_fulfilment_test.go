package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 28 gap-1 IDOR domain 2/5: warehouse and fulfilment.
//
// GetWarehouse enforces organization scope AND t.BranchPermitted directly
// (Warehouse carries a real branch_id); GetFulfilment enforces organization
// scope on the order row itself and THEN re-resolves the order's declared
// warehouse through GetWarehouse -- so a fulfilment order can only be read
// by someone who could also read its parent warehouse, exactly the same
// nested-parent-through-re-validation pattern proven for maintenance work
// orders in idor_assets_maintenance_test.go.

func warehouseIDORRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := warehouse.NewService(warehouse.Options{
		Pool: pool, Flags: warehouse.Flags{Warehouses: true, Fulfilment: true, Inventory: true},
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	store := idempotency.NewStore(pool, 0)

	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		var permitted []string
		if restrict := c.GetHeader("X-Test-Permitted-Branch"); restrict != "" {
			permitted = []string{restrict}
		}
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: c.GetHeader("X-Test-Member"),
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: c.GetHeader("X-Test-Member"),
			PermittedBranchIDs: permitted,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	warehouse.NewHandler(service, authorizer, store).RegisterTenantRoutes(group)
	return engine
}

func warehouseIDORCall(t *testing.T, router http.Handler, method, path, orgID, memberID string,
	body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Org", orgID)
	request.Header.Set("X-Test-Member", memberID)
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	envelope := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	if data, ok := envelope["data"].(map[string]any); ok {
		return recorder.Code, data
	}
	return recorder.Code, envelope
}

// TestWarehouse_ObjectAuthorization proves the object-authorization matrix
// for /warehouses: same-org access, cross-org denial, cross-branch denial,
// missing-permission denial -- all safe 404/403, never a leaked view.
func TestWarehouse_ObjectAuthorization(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "Warehouse IDOR Co A", "RC-IDOR-WH-1")
	orgB := createOrganization(t, pool, "Warehouse IDOR Co B", "RC-IDOR-WH-2")
	router := warehouseIDORRouter(t, pool)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	// rol_system_crm holds neither warehouses.* nor fulfilment.* (migrations
	// 000046/000047 grant those to owner/operations only).
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org-owner")

	branchA := seedBranch(t, pool, orgA.ID, "Warehouse Branch A")
	branchOther := seedBranch(t, pool, orgA.ID, "Warehouse Branch Other")

	status, body := warehouseIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/warehouses", orgA.ID, noPermission,
		map[string]any{"BranchID": branchA, "Code": "WH-1", "Name": "Lagos DC", "AddressReference": "addr-1", "Timezone": "Africa/Lagos"},
		map[string]string{"Idempotency-Key": "no-perm-create"})
	if status != http.StatusForbidden {
		t.Fatalf("create without warehouses.manage status = %d body=%v, want 403", status, body)
	}

	status, created := warehouseIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/warehouses", orgA.ID, owner,
		map[string]any{"BranchID": branchA, "Code": "WH-100", "Name": "Lagos DC", "AddressReference": "addr-100", "Timezone": "Africa/Lagos"},
		map[string]string{"Idempotency-Key": "create-1"})
	if status != http.StatusCreated {
		t.Fatalf("create status = %d body=%v, want 201", status, created)
	}
	warehouseID, _ := created["ID"].(string)
	if warehouseID == "" {
		t.Fatalf("create response carries no ID: %v", created)
	}

	status, got := warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/warehouses/"+warehouseID, orgA.ID, owner, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org get status = %d body=%v, want 200", status, got)
	}

	status, got = warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/warehouses/"+warehouseID, orgB.ID, crossOrgOwner, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}

	status, got = warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/warehouses/"+warehouseID, orgA.ID, owner, nil,
		map[string]string{"X-Test-Permitted-Branch": branchOther})
	if status != http.StatusNotFound {
		t.Fatalf("cross-branch get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}
}

// TestFulfilment_ObjectAuthorization proves the object-authorization matrix
// for /fulfilment-orders, including the nested-parent case: GetFulfilment
// re-resolves the order's warehouse_id through GetWarehouse on every read,
// so an order whose warehouse belongs to a different organization (forced
// here by direct SQL, since normal creation validates this) is refused --
// the parent relationship is genuinely re-checked, not merely trusted from
// the foreign key having been valid at insert time.
func TestFulfilment_ObjectAuthorization(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "Fulfilment IDOR Co A", "RC-IDOR-FUL-1")
	orgB := createOrganization(t, pool, "Fulfilment IDOR Co B", "RC-IDOR-FUL-2")
	router := warehouseIDORRouter(t, pool)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org-owner")

	branchA := seedBranch(t, pool, orgA.ID, "Fulfilment Branch A")
	branchB := seedBranch(t, pool, orgB.ID, "Fulfilment Branch B")

	_, whA := warehouseIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/warehouses", orgA.ID, owner,
		map[string]any{"BranchID": branchA, "Code": "WH-200", "Name": "Org A DC", "AddressReference": "addr-200", "Timezone": "Africa/Lagos"},
		map[string]string{"Idempotency-Key": "wh-a"})
	warehouseAID, _ := whA["ID"].(string)
	if warehouseAID == "" {
		t.Fatalf("orgA warehouse create carries no ID: %v", whA)
	}
	_, whB := warehouseIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/warehouses", orgB.ID, crossOrgOwner,
		map[string]any{"BranchID": branchB, "Code": "WH-201", "Name": "Org B DC", "AddressReference": "addr-201", "Timezone": "Africa/Lagos"},
		map[string]string{"Idempotency-Key": "wh-b"})
	warehouseBID, _ := whB["ID"].(string)
	if warehouseBID == "" {
		t.Fatalf("orgB warehouse create carries no ID: %v", whB)
	}

	// Fulfilment orders are seeded directly: CreateFulfilment additionally
	// requires a matching real inventory item per line, which is business
	// logic this suite is not exercising -- only the read-time
	// authorization of an order that already exists.
	fulfilmentID := "ful_idor_test_1"
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO fleet_partner_fulfilment_orders(id,organization_id,warehouse_id,customer_reference) VALUES($1,$2,$3,'cust-ref-1')`,
		fulfilmentID, orgA.ID, warehouseAID); err != nil {
		t.Fatalf("seed fulfilment order: %v", err)
	}

	// Missing permission -> 403.
	status, body := warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/fulfilment-orders/"+fulfilmentID, orgA.ID, noPermission, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("get without fulfilment.read status = %d body=%v, want 403", status, body)
	}

	// Same-org read.
	status, got := warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/fulfilment-orders/"+fulfilmentID, orgA.ID, owner, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org get status = %d body=%v, want 200", status, got)
	}

	// Cross-org: orgB cannot reach orgA's fulfilment order id.
	status, got = warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/fulfilment-orders/"+fulfilmentID, orgB.ID, crossOrgOwner, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404", status, got)
	}

	// Nested parent mismatch: flip the order's warehouse_id to orgB's
	// warehouse. The order row is still organization_id=orgA so an orgA
	// caller reaches the row, but GetFulfilment's own re-resolution of the
	// warehouse through GetWarehouse(t, warehouseId) must refuse once that
	// id no longer resolves inside orgA.
	if _, err := pool.Exec(context.Background(),
		`UPDATE fleet_partner_fulfilment_orders SET warehouse_id=$1 WHERE id=$2`, warehouseBID, fulfilmentID); err != nil {
		t.Fatalf("force parent mismatch: %v", err)
	}
	status, got = warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/fulfilment-orders/"+fulfilmentID, orgA.ID, owner, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("fulfilment order with cross-org parent warehouse status = %d body=%v, want 404 (parent re-validated, not trusted)", status, got)
	}
}
