package integration_test

import (
	"context"
	"net/http"
	"testing"
)

// Creating a fulfilment order persists its lines. The repository's insert
// returns the stored order without lines; the use case used to overwrite the
// request with it and then loop over an empty slice, so every order was
// saved with no lines at all.
func TestFulfilmentOrderPersistsItsLines(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Fulfilment Lines Co", "RC-FUL-LINES-1")
	router := warehouseIDORRouter(t, pool)
	owner := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "ful-lines-owner")
	branch := seedBranch(t, pool, org.ID, "Fulfilment Lines Branch")

	_, wh := warehouseIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/warehouses", org.ID, owner,
		map[string]any{"branchId": branch, "code": "WH-LINES", "name": "Lines DC", "addressReference": "addr-lines", "timezone": "Africa/Lagos"},
		map[string]string{"Idempotency-Key": "wh-lines"})
	warehouseID, _ := wh["id"].(string)
	if warehouseID == "" {
		t.Fatalf("warehouse create carries no id: %v", wh)
	}
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_warehouses SET status='ACTIVE' WHERE id=$1`, warehouseID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_storage_locations(id,organization_id,warehouse_id,code,name,location_type)
		VALUES('loc_lines',$1,$2,'BIN-1','Bin 1','BIN')`, org.ID, warehouseID); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"inv_lines_a", "inv_lines_b"} {
		if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_inventory_items(id,organization_id,warehouse_id,location_id,owner_type,owner_reference,sku,unit,status,quantity_projection)
			VALUES($1,$2,$3,'loc_lines','ORGANIZATION',$2,$1,'EACH','AVAILABLE',50)`, item, org.ID, warehouseID); err != nil {
			t.Fatal(err)
		}
	}

	status, created := warehouseIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/fulfilment-orders", org.ID, owner,
		map[string]any{"warehouseId": warehouseID, "customerReference": "cust-lines-1", "lines": []map[string]any{
			{"inventoryItemId": "inv_lines_a", "requestedQuantity": 3},
			{"inventoryItemId": "inv_lines_b", "requestedQuantity": 5},
		}}, map[string]string{"Idempotency-Key": "ful-lines-1"})
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create fulfilment = %d %v", status, created)
	}
	orderID, _ := created["id"].(string)
	if lines, _ := created["lines"].([]any); orderID == "" || len(lines) != 2 {
		t.Fatalf("created order = %v, want 2 lines", created)
	}
	var stored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_fulfilment_lines WHERE fulfilment_order_id=$1`, orderID).Scan(&stored); err != nil || stored != 2 {
		t.Fatalf("stored lines = %d (%v), want 2", stored, err)
	}
	status, got := warehouseIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/fulfilment-orders/"+orderID, org.ID, owner, nil, nil)
	if lines, _ := got["lines"].([]any); status != http.StatusOK || len(lines) != 2 {
		t.Fatalf("read back = %d %v, want 2 lines", status, got)
	}
}
