package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	warehousehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/http"
	warehousemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	warehouserepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/repositories"
	warehouseusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// section51Router mounts the real warehouse handler with every Section 21/51
// capability enabled. The tenant comes from test headers; permissions are
// resolved from the seeded member's real role grants.
func section51Router(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := warehouseusecases.NewService(warehouseusecases.Options{
		Pool: pool, Repository: warehouserepos.NewRepository(pool), Events: fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(pool)), Restrictions: s51NoRestrictions{},
		Flags: warehousemodels.Flags{Warehouses: true, Inventory: true, Fulfilment: true, Dock: true, Yard: true, Terminal: true, Returns: true, Workers: true},
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: c.GetHeader("X-Test-Member"),
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: c.GetHeader("X-Test-Member"),
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	warehousehttp.NewHandler(service, authorizer, idempotency.NewStore(pool, 0)).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	return engine
}

// s51NoRestrictions is the compliance restriction gate with no restriction
// recorded; production wires the real compliance feature.
type s51NoRestrictions struct{}

func (s51NoRestrictions) Restricted(context.Context, string, string, string) (bool, error) {
	return false, nil
}

type s51 struct {
	t      *testing.T
	router http.Handler
	org    string
	n      int
}

func (h *s51) call(member, method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	h.n++
	headers := map[string]string{}
	if method != http.MethodGet {
		headers["Idempotency-Key"] = fmt.Sprintf("s51-%s-%d", member, h.n)
	}
	return warehouseIDORCall(h.t, h.router, method, path, h.org, member, body, headers)
}

func (h *s51) must(member, method, path string, body any, want int) map[string]any {
	h.t.Helper()
	code, out := h.call(member, method, path, body)
	if code != want {
		h.t.Fatalf("%s %s = %d %v, want %d", method, path, code, out, want)
	}
	return out
}

func ver(v map[string]any) int64 {
	f, _ := v["version"].(float64)
	return int64(f)
}

func stockAt(t *testing.T, pool *pgxpool.Pool, location, sku string) (qty, reserved float64, status string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `SELECT COALESCE(sum(quantity_projection),0)::float8,COALESCE(sum(reserved_quantity),0)::float8,COALESCE(max(status),'') FROM fleet_partner_inventory_items WHERE location_id=$1 AND sku=$2`, location, sku).Scan(&qty, &reserved, &status)
	if err != nil {
		t.Fatal(err)
	}
	return
}

// TestSection51WarehouseJourney walks goods receipt -> discrepancy -> put-away
// -> reservation -> pick wave -> packing, and proves the safety rules the
// specification names: wrong-SKU and wrong-bin scans refused, stock only for
// accepted quantity, no double allocation, task leases exclusive and
// recoverable, packed never exceeding picked, gate events in yard order and
// tenant isolation (WH-001..WH-004).
func TestSection51WarehouseJourney(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Section 51 Warehouse Co", "RC-S51-"+ids.New("r"))
	other := createOrganization(t, pool, "Section 51 Other Co", "RC-S51O-"+ids.New("r"))
	router := section51Router(t, pool)
	owner := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "s51-owner")
	workerB := seedApprovalMember(t, pool, org.ID, "rol_system_operations", "s51-worker-b")
	outsider := seedApprovalMember(t, pool, other.ID, "rol_system_owner", "s51-outsider")
	branch := seedBranch(t, pool, org.ID, "Section 51 Branch")
	h := &s51{t: t, router: router, org: org.ID}

	wh := h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouses", map[string]any{"branchId": branch, "code": "WH-S51", "name": "S51 DC", "addressReference": "addr-s51", "timezone": "Africa/Lagos"}, http.StatusCreated)
	warehouseID := wh["id"].(string)
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_warehouses SET status='ACTIVE' WHERE id=$1`, warehouseID); err != nil {
		t.Fatal(err)
	}
	for _, l := range [][3]string{{"loc_s51_recv", "RECV", "STAGING"}, {"loc_s51_bin", "BIN-A", "BIN"}, {"loc_s51_quar", "QUAR", "QUARANTINE"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_storage_locations(id,organization_id,warehouse_id,code,name,location_type)VALUES($1||$4,$2,$3,$5,$5,$6)`, l[0], org.ID, warehouseID, org.ID[len(org.ID)-6:], l[1], l[2]); err != nil {
			t.Fatal(err)
		}
	}
	sfx := org.ID[len(org.ID)-6:]
	recv, bin, quar := "loc_s51_recv"+sfx, "loc_s51_bin"+sfx, "loc_s51_quar"+sfx

	// --- Receipt with two lines -------------------------------------------------
	receipt := h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts", map[string]any{
		"warehouseId": warehouseID, "receiptNumber": "GRN-" + sfx, "sourceType": "SUPPLIER", "sourceReference": "PO-1",
		"receivingLocationId": recv, "quarantineLocationId": quar,
		"lines": []map[string]any{
			{"sku": "SKU-1", "ownerType": "ORGANIZATION", "ownerReference": org.ID, "unit": "EACH", "expectedQuantity": 10, "putawayLocationId": bin},
			{"sku": "SKU-2", "ownerType": "ORGANIZATION", "ownerReference": org.ID, "unit": "EACH", "expectedQuantity": 5},
		}}, http.StatusCreated)
	receiptID := receipt["id"].(string)
	lines := receipt["lines"].([]any)
	line1, line2 := lines[0].(map[string]any)["id"].(string), lines[1].(map[string]any)["id"].(string)
	if receipt["status"] != "EXPECTED" || len(lines) != 2 {
		t.Fatalf("receipt = %v", receipt)
	}
	// Cross-tenant read is a safe 404.
	if code, _ := warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/warehouse-receipts/"+receiptID, other.ID, outsider, nil, nil); code != http.StatusNotFound {
		t.Fatalf("cross-tenant receipt read = %d, want 404", code)
	}
	// Receiving before arrival is refused.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/receive", map[string]any{"expectedVersion": ver(receipt), "lines": []map[string]any{{"lineId": line1, "scannedSku": "SKU-1", "receivedQuantity": 10}}}, http.StatusConflict)
	receipt = h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/arrive", map[string]any{"expectedVersion": ver(receipt)}, http.StatusOK)
	// Wrong SKU scan is refused and records nothing.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/receive", map[string]any{"expectedVersion": ver(receipt), "lines": []map[string]any{{"lineId": line1, "scannedSku": "SKU-2", "receivedQuantity": 10}}}, http.StatusUnprocessableEntity)
	// Stale version is refused.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/receive", map[string]any{"expectedVersion": ver(receipt) - 1, "lines": []map[string]any{{"lineId": line1, "scannedSku": "SKU-1", "receivedQuantity": 10}}}, http.StatusConflict)
	receipt = h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/receive", map[string]any{"expectedVersion": ver(receipt), "lines": []map[string]any{
		{"lineId": line1, "scannedSku": "SKU-1", "receivedQuantity": 10},
		{"lineId": line2, "scannedSku": "SKU-2", "receivedQuantity": 5, "damagedQuantity": 1, "quarantinedQuantity": 1},
	}}, http.StatusOK)
	if receipt["status"] != "EXCEPTION" {
		t.Fatalf("receipt with damage should be EXCEPTION: %v", receipt)
	}
	// WH-001: stock only for the accepted quantity; damaged/quarantined held apart.
	if q, _, st := stockAt(t, pool, recv, "SKU-2"); q != 3 || st != "AVAILABLE" {
		t.Fatalf("SKU-2 receiving stock = %v %s, want 3 AVAILABLE", q, st)
	}
	if q, _, st := stockAt(t, pool, quar, "SKU-2"); q != 2 || st != "QUARANTINED" {
		t.Fatalf("SKU-2 quarantine stock = %v %s, want 2 QUARANTINED", q, st)
	}
	// Put-away cannot start while a discrepancy is open.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/complete", map[string]any{"expectedVersion": ver(receipt)}, http.StatusConflict)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/resolve-discrepancy", map[string]any{"expectedVersion": ver(receipt), "lineId": line2, "resolution": "SUPPLIER_CLAIM"}, http.StatusUnprocessableEntity)
	receipt = h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/resolve-discrepancy", map[string]any{"expectedVersion": ver(receipt), "lineId": line2, "resolution": "SUPPLIER_CLAIM", "note": "1 crushed carton, 1 wet carton"}, http.StatusOK)
	if receipt["status"] != "RECEIVED" {
		t.Fatalf("resolved receipt = %v", receipt["status"])
	}
	receipt = h.must(owner, http.MethodPost, "/fleet-partner/v1/warehouse-receipts/"+receiptID+"/complete", map[string]any{"expectedVersion": ver(receipt)}, http.StatusOK)
	if receipt["status"] != "PUTAWAY_IN_PROGRESS" {
		t.Fatalf("receipt after complete = %v", receipt["status"])
	}

	// --- Leased put-away -------------------------------------------------------
	var task1, task2 string
	if err := pool.QueryRow(ctx, `SELECT id FROM fleet_partner_putaway_tasks WHERE receipt_line_id=$1`, line1).Scan(&task1); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM fleet_partner_putaway_tasks WHERE receipt_line_id=$1`, line2).Scan(&task2); err != nil {
		t.Fatal(err)
	}
	h.must(owner, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task1+"/claim", map[string]any{"clientClaimId": "claim-a"}, http.StatusOK)
	// Same worker, same claim id: idempotent.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task1+"/claim", map[string]any{"clientClaimId": "claim-a"}, http.StatusOK)
	// WH-002: a second worker cannot take a live lease.
	h.must(workerB, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task1+"/claim", map[string]any{"clientClaimId": "claim-b"}, http.StatusConflict)
	h.must(workerB, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task1+"/complete", map[string]any{"toLocationId": bin, "scannedLocationCode": "BIN-A", "scannedSku": "SKU-1", "quantity": 10}, http.StatusConflict)
	// Wrong-bin scan refused.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task1+"/complete", map[string]any{"toLocationId": bin, "scannedLocationCode": "BIN-Z", "scannedSku": "SKU-1", "quantity": 10}, http.StatusUnprocessableEntity)
	// Quarantine is never a put-away destination.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task1+"/complete", map[string]any{"toLocationId": quar, "scannedLocationCode": "QUAR", "scannedSku": "SKU-1", "quantity": 10}, http.StatusUnprocessableEntity)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task1+"/complete", map[string]any{"toLocationId": bin, "scannedLocationCode": "BIN-A", "scannedSku": "SKU-1", "quantity": 10}, http.StatusOK)
	if q, _, _ := stockAt(t, pool, recv, "SKU-1"); q != 0 {
		t.Fatalf("receiving SKU-1 after put-away = %v, want 0", q)
	}
	if q, _, _ := stockAt(t, pool, bin, "SKU-1"); q != 10 {
		t.Fatalf("bin SKU-1 after put-away = %v, want 10", q)
	}
	// Lease recovery: an expired lease can be taken over by another worker.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task2+"/claim", map[string]any{"clientClaimId": "claim-c"}, http.StatusOK)
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_putaway_tasks SET lease_expires_at=now()-interval '1 minute' WHERE id=$1`, task2); err != nil {
		t.Fatal(err)
	}
	// The expired owner can no longer complete.
	h.must(owner, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task2+"/complete", map[string]any{"toLocationId": bin, "scannedLocationCode": "BIN-A", "scannedSku": "SKU-2", "quantity": 3}, http.StatusConflict)
	h.must(workerB, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task2+"/claim", map[string]any{"clientClaimId": "claim-d"}, http.StatusOK)
	h.must(workerB, http.MethodPost, "/fleet-partner/v1/putaway-tasks/"+task2+"/complete", map[string]any{"toLocationId": bin, "scannedLocationCode": "BIN-A", "scannedSku": "SKU-2", "quantity": 3}, http.StatusOK)
	receipt = h.must(owner, http.MethodGet, "/fleet-partner/v1/warehouse-receipts/"+receiptID, nil, http.StatusOK)
	if receipt["status"] != "COMPLETED" {
		t.Fatalf("receipt after all put-away = %v", receipt["status"])
	}
	// Movements are immutable.
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_inventory_movements SET quantity=1 WHERE correlation_id=$1`, "putaway:"+task1+":in"); err == nil {
		t.Fatal("inventory movement was mutable")
	}

	// --- WH-003: no double allocation under concurrent release ------------------
	var binItem string
	if err := pool.QueryRow(ctx, `SELECT id FROM fleet_partner_inventory_items WHERE location_id=$1 AND sku='SKU-1'`, bin).Scan(&binItem); err != nil {
		t.Fatal(err)
	}
	orders := make([]map[string]any, 2)
	for i := range orders {
		orders[i] = h.must(owner, http.MethodPost, "/fleet-partner/v1/fulfilment-orders", map[string]any{"warehouseId": warehouseID, "customerReference": fmt.Sprintf("cust-%d", i), "lines": []map[string]any{{"inventoryItemId": binItem, "requestedQuantity": 6}}}, http.StatusCreated)
	}
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range orders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = warehouseIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/fulfilment-orders/"+orders[i]["id"].(string)+"/release", org.ID, owner, map[string]any{"expectedVersion": ver(orders[i])}, map[string]string{"Idempotency-Key": fmt.Sprintf("s51-release-%d", i)})
		}(i)
	}
	wg.Wait()
	won := -1
	for i, c := range codes {
		if c == http.StatusOK {
			if won >= 0 {
				t.Fatalf("both releases reserved the same stock: %v", codes)
			}
			won = i
		} else if c != http.StatusConflict {
			t.Fatalf("release %d = %d", i, c)
		}
	}
	if won < 0 {
		t.Fatalf("no release succeeded: %v", codes)
	}
	if _, reserved, _ := stockAt(t, pool, bin, "SKU-1"); reserved != 6 {
		t.Fatalf("reserved = %v, want 6", reserved)
	}
	orderID := orders[won]["id"].(string)

	// --- Pick wave --------------------------------------------------------------
	h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-waves", map[string]any{"warehouseId": warehouseID, "waveNumber": "W-" + sfx, "fulfilmentOrderIds": []string{orders[1-won]["id"].(string)}}, http.StatusConflict)
	wave := h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-waves", map[string]any{"warehouseId": warehouseID, "waveNumber": "W-" + sfx, "fulfilmentOrderIds": []string{orderID}}, http.StatusCreated)
	tasks := wave["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("wave tasks = %v", tasks)
	}
	pick := tasks[0].(map[string]any)["id"].(string)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-tasks/"+pick+"/complete", map[string]any{"scannedLocationCode": "BIN-A", "scannedSku": "SKU-1", "pickedQuantity": 6}, http.StatusConflict)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-tasks/"+pick+"/claim", map[string]any{"clientClaimId": "pick-1"}, http.StatusOK)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-tasks/"+pick+"/complete", map[string]any{"scannedLocationCode": "RECV", "scannedSku": "SKU-1", "pickedQuantity": 6}, http.StatusUnprocessableEntity)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-tasks/"+pick+"/complete", map[string]any{"scannedLocationCode": "BIN-A", "scannedSku": "SKU-1", "pickedQuantity": 7}, http.StatusUnprocessableEntity)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-tasks/"+pick+"/complete", map[string]any{"scannedLocationCode": "BIN-A", "scannedSku": "SKU-1", "pickedQuantity": 6}, http.StatusOK)
	order := h.must(owner, http.MethodGet, "/fleet-partner/v1/fulfilment-orders/"+orderID, nil, http.StatusOK)
	if order["status"] != "PICKED" {
		t.Fatalf("order after pick = %v", order["status"])
	}
	wave = h.must(owner, http.MethodGet, "/fleet-partner/v1/pick-waves/"+wave["id"].(string), nil, http.StatusOK)
	if wave["status"] != "COMPLETED" {
		t.Fatalf("wave after pick = %v", wave["status"])
	}

	// --- Packing ----------------------------------------------------------------
	h.must(owner, http.MethodPost, "/fleet-partner/v1/packing-units", map[string]any{"fulfilmentOrderId": orderID, "packingCode": "PK-" + sfx + "-X", "contents": []map[string]any{{"inventoryItemId": binItem, "quantity": 7}}}, http.StatusConflict)
	unit := h.must(owner, http.MethodPost, "/fleet-partner/v1/packing-units", map[string]any{"fulfilmentOrderId": orderID, "packingCode": "PK-" + sfx, "sealNumber": "SEAL-9", "contents": []map[string]any{{"inventoryItemId": binItem, "quantity": 6}}}, http.StatusCreated)
	if unit["status"] != "SEALED" {
		t.Fatalf("packing unit = %v", unit)
	}
	h.must(owner, http.MethodPost, "/fleet-partner/v1/packing-units", map[string]any{"fulfilmentOrderId": orderID, "packingCode": "PK-" + sfx + "-Y", "contents": []map[string]any{{"inventoryItemId": binItem, "quantity": 1}}}, http.StatusConflict)

	// --- Short pick recovery -----------------------------------------------------
	// A wave order is picked only by its tasks: the order-level pick command
	// must not record the same stock a second time, before or after a short
	// pick. Once no task is open the order is cancellable, which returns the
	// picked units to stock and ends its reservations.
	_, reservedBefore, _ := stockAt(t, pool, bin, "SKU-1")
	short := h.must(owner, http.MethodPost, "/fleet-partner/v1/fulfilment-orders", map[string]any{"warehouseId": warehouseID, "customerReference": "cust-short", "lines": []map[string]any{{"inventoryItemId": binItem, "requestedQuantity": 3}}}, http.StatusCreated)
	shortID := short["id"].(string)
	short = h.must(owner, http.MethodPost, "/fleet-partner/v1/fulfilment-orders/"+shortID+"/release", map[string]any{"expectedVersion": ver(short)}, http.StatusOK)
	shortWave := h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-waves", map[string]any{"warehouseId": warehouseID, "waveNumber": "W-SHORT-" + sfx, "fulfilmentOrderIds": []string{shortID}}, http.StatusCreated)
	shortTask := shortWave["tasks"].([]any)[0].(map[string]any)["id"].(string)
	short = h.must(owner, http.MethodGet, "/fleet-partner/v1/fulfilment-orders/"+shortID, nil, http.StatusOK)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/fulfilment-orders/"+shortID+"/pick", map[string]any{"expectedVersion": ver(short)}, http.StatusConflict)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-tasks/"+shortTask+"/claim", map[string]any{"clientClaimId": "pick-short"}, http.StatusOK)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/fulfilment-orders/"+shortID+"/cancel", map[string]any{"expectedVersion": ver(short)}, http.StatusConflict)
	h.must(owner, http.MethodPost, "/fleet-partner/v1/pick-tasks/"+shortTask+"/complete", map[string]any{"scannedLocationCode": "BIN-A", "scannedSku": "SKU-1", "pickedQuantity": 1}, http.StatusOK)
	short = h.must(owner, http.MethodGet, "/fleet-partner/v1/fulfilment-orders/"+shortID, nil, http.StatusOK)
	if short["status"] != "PICKING" {
		t.Fatalf("order after short pick = %v, want PICKING", short["status"])
	}
	if _, reserved, _ := stockAt(t, pool, bin, "SKU-1"); reserved != reservedBefore+1 {
		t.Fatalf("reserved after short pick = %v, want %v (only the picked unit stays reserved)", reserved, reservedBefore+1)
	}
	var pickMovements int
	countPicks := func() int {
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_inventory_movements WHERE fulfilment_order_id=$1 AND movement_type='PICK'`, shortID).Scan(&pickMovements); err != nil {
			t.Fatal(err)
		}
		return pickMovements
	}
	if n := countPicks(); n != 1 {
		t.Fatalf("PICK movements after short pick = %d, want 1", n)
	}
	h.must(owner, http.MethodPost, "/fleet-partner/v1/fulfilment-orders/"+shortID+"/pick", map[string]any{"expectedVersion": ver(short)}, http.StatusConflict)
	if n := countPicks(); n != 1 {
		t.Fatalf("order-level pick recorded stock a second time: %d PICK movements", n)
	}
	short = h.must(owner, http.MethodPost, "/fleet-partner/v1/fulfilment-orders/"+shortID+"/cancel", map[string]any{"expectedVersion": ver(short)}, http.StatusOK)
	if short["status"] != "CANCELLED" {
		t.Fatalf("short-picked order after cancel = %v", short["status"])
	}
	if _, reserved, _ := stockAt(t, pool, bin, "SKU-1"); reserved != reservedBefore {
		t.Fatalf("reserved after cancel = %v, want %v", reserved, reservedBefore)
	}
	var released, liveReservations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_inventory_movements WHERE fulfilment_order_id=$1 AND movement_type='RELEASE' AND reason_code='PICK_CANCELLED' AND quantity=1`, shortID).Scan(&released); err != nil || released != 1 {
		t.Fatalf("RELEASE movement for cancelled pick = %d (%v), want 1", released, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_inventory_reservations WHERE fulfilment_order_id=$1 AND status<>'RELEASED'`, shortID).Scan(&liveReservations); err != nil || liveReservations != 0 {
		t.Fatalf("reservations still held by the cancelled order = %d (%v)", liveReservations, err)
	}

	// --- Gate events in yard order ---------------------------------------------
	visit := h.must(owner, http.MethodPost, "/fleet-partner/v1/yard-visits", map[string]any{"warehouseId": warehouseID, "providerReference": "carrier-1", "vehicleOrTruckReference": "TRK-1"}, http.StatusCreated)
	visitPath := "/fleet-partner/v1/yard-visits/" + visit["id"].(string) + "/gate-events"
	h.must(owner, http.MethodPost, visitPath, map[string]any{"eventType": "EXIT", "gateCode": "G1"}, http.StatusConflict)
	h.must(owner, http.MethodPost, visitPath, map[string]any{"eventType": "ENTRY", "gateCode": "G1", "evidenceFileIds": []string{"file_photo"}}, http.StatusCreated)
	h.must(owner, http.MethodPost, visitPath, map[string]any{"eventType": "ENTRY", "gateCode": "G1"}, http.StatusConflict)
	h.must(owner, http.MethodPost, visitPath, map[string]any{"eventType": "SEAL_CHECK", "gateCode": "G1", "notes": "seal intact"}, http.StatusCreated)
	h.must(owner, http.MethodPost, visitPath, map[string]any{"eventType": "EXIT", "gateCode": "G2"}, http.StatusCreated)
	h.must(owner, http.MethodPost, visitPath, map[string]any{"eventType": "PARKING", "gateCode": "G2"}, http.StatusConflict)
	code, list := warehouseIDORCall(t, router, http.MethodGet, visitPath, org.ID, owner, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("list gate events = %d %v", code, list)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_gate_events WHERE yard_visit_id=$1`, visit["id"]).Scan(&n); err != nil || n != 3 {
		t.Fatalf("gate events = %d %v, want 3", n, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM fleet_partner_gate_events WHERE yard_visit_id=$1`, visit["id"]); err == nil {
		t.Fatal("gate events were deletable")
	}
	// Every state change wrote an outbox event.
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type IN('warehouse_receipt.completed.v1','putaway_task.completed.v1','pick_task.completed.v1','packing_unit.created.v1','yard.gate_event_recorded.v1')`, org.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events < 6 {
		t.Fatalf("outbox events = %d, want at least 6", events)
	}
}
