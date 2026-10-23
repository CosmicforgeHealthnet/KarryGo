package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

// These use the actual Food/Shop services, migrations, stock and event writers.
// Authorization/approval/signed transport is tested separately through the
// actual owner command boundary, never replaced by these native scope fixtures.
func TestVendorSecureAdminListingModerationPreservesStockAndPrice(t *testing.T) {
	e := newSection17BEnv(t, 15*time.Minute)
	ctx := context.Background()
	foodItem := e.seedFoodItem("Section 11 prepared food", 2500)
	actor := food.Actor{StaffID: e.staffA, VendorID: e.businessA, CommandRef: "test-food-moderation"}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var originalPrice, version int64
	if err = tx.QueryRow(ctx, `SELECT base_price_minor,version FROM food_items WHERE id=$1`, foodItem.ItemID).Scan(&originalPrice, &version); err != nil {
		t.Fatal(err)
	}
	_, prior, next, err := e.foodService.AdministrativeListingTx(ctx, tx, actor, food.Scope{BranchIDs: []string{e.foodBranch}}, foodItem.ItemID, false, version)
	if err != nil || prior != "ACTIVE" || next != "PAUSED" {
		t.Fatal("food takedown", prior, next, err)
	}
	if _, _, _, err = e.foodService.AdministrativeListingTx(ctx, tx, actor, food.Scope{BranchIDs: []string{e.foodBranch}}, foodItem.ItemID, true, version); err == nil {
		t.Fatal("stale moderation allowed")
	}
	if _, _, _, err = e.foodService.AdministrativeListingTx(ctx, tx, actor, food.Scope{BranchIDs: []string{e.foodBranch}}, foodItem.ItemID, true, version+1); err != nil {
		t.Fatal("food restore", err)
	}
	var price int64
	if err = tx.QueryRow(ctx, `SELECT base_price_minor FROM food_items WHERE id=$1`, foodItem.ItemID).Scan(&price); err != nil || price != originalPrice {
		t.Fatal("moderation repriced food", price, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	item := e.seedShopItem(s17bStockOptions{Business: e.businessA, Staff: e.staffA, Branch: e.branchA1, Name: "Section11 Grocery", Quantity: 15, PriceMinor: 700})
	tx, err = e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var onHand, reserved int64
	if err = tx.QueryRow(ctx, `SELECT on_hand_quantity,reserved_quantity FROM shop_inventory_balances WHERE business_id=$1 AND branch_id=$2 AND stock_basis_id=$3`, e.businessA, e.branchA1, item.InventoryItemID).Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT version FROM shop_branch_listings WHERE id=$1`, item.ListingID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	shopActor := shop.Actor{StaffID: e.staffA, VendorID: e.businessA, CommandRef: "test-shop-moderation"}
	scope := shop.Scope{BranchIDs: []string{e.branchA1}}
	if _, _, state, err := e.shopService.AdministrativeListingTx(ctx, tx, shopActor, scope, item.ListingID, false, version); err != nil || state != "INACTIVE" {
		t.Fatal("shop takedown", state, err)
	}
	if _, _, _, err = e.shopService.AdministrativeListingTx(ctx, tx, shopActor, shop.Scope{BranchIDs: []string{e.branchB1}}, item.ListingID, true, version+1); err == nil {
		t.Fatal("foreign branch restoration allowed")
	}
	if _, _, state, err := e.shopService.AdministrativeListingTx(ctx, tx, shopActor, scope, item.ListingID, true, version+1); err != nil || state != "ACTIVE" {
		t.Fatal("shop restore", state, err)
	}
	var afterHand, afterReserved int64
	if err = tx.QueryRow(ctx, `SELECT on_hand_quantity,reserved_quantity FROM shop_inventory_balances WHERE business_id=$1 AND branch_id=$2 AND stock_basis_id=$3`, e.businessA, e.branchA1, item.InventoryItemID).Scan(&afterHand, &afterReserved); err != nil || afterHand != onHand || afterReserved != reserved {
		t.Fatal("moderation changed stock", afterHand, afterReserved, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestVendorSecureAdminProductionRecoveryNeverCompletesOrInventsCapacity(t *testing.T) {
	e := newSection17BEnv(t, 15*time.Minute)
	ctx := context.Background()
	id := "section11-production-" + e.suffix
	promise := time.Now().UTC().Add(3 * time.Hour)
	if _, err := e.pool.Exec(ctx, `INSERT INTO production_jobs(id,business_id,branch_id,capability,source_type,source_ref,title,status,planned_quantity,promised_ready_at) VALUES($1,$2,$3,'RESTAURANT','EXTERNAL_ORDER',$1,'Section 11 test job','IN_PROGRESS',20,$4)`, id, e.businessA, e.foodBranch, promise); err != nil {
		t.Fatal(err)
	}
	service := production.NewService(production.Options{DB: e.pool, Flags: production.Flags{ProductionEnabled: true, WritesEnabled: true}, Events: events.NewDomainPublisher(events.NewOutboxRepository(e.pool))})
	actor := production.Actor{StaffID: e.staffA, VendorID: e.businessA, CommandRef: "test-production-recovery"}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, _, state, err := service.AdministrativeInterruptionTx(ctx, tx, actor, production.Scope{BranchIDs: []string{e.foodBranch}}, id, false, "EQUIPMENT_FAILURE", 1); err != nil || state != "BLOCKED" {
		t.Fatal("native block", state, err)
	}
	if _, _, state, err := service.AdministrativeInterruptionTx(ctx, tx, actor, production.Scope{BranchIDs: []string{e.foodBranch}}, id, true, "EQUIPMENT_REMEDIATED", 2); err != nil || state != "IN_PROGRESS" {
		t.Fatal("native resume", state, err)
	}
	start, end := time.Now().UTC().Add(time.Hour), time.Now().UTC().Add(2*time.Hour)
	if err = service.ReportDelayInTx(ctx, tx, actor, id, production.DelayInput{ReasonCode: "EQUIPMENT_FAILURE", NewWindowStart: &start, NewWindowEnd: &end, ExpectedVersion: 3, IdempotencyKey: "section11-delay-" + e.suffix}); err != nil {
		t.Fatal("native delay", err)
	}
	var state string
	var produced, waste int
	var unchangedPromise time.Time
	if err = tx.QueryRow(ctx, `SELECT status,produced_quantity,waste_quantity,promised_ready_at FROM production_jobs WHERE id=$1`, id).Scan(&state, &produced, &waste, &unchangedPromise); err != nil || state != "IN_PROGRESS" || produced != 0 || waste != 0 || promise.Sub(unchangedPromise) > time.Microsecond || unchangedPromise.Sub(promise) > time.Microsecond {
		t.Fatal("fake production outcome or promise", state, produced, waste, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// A failed enclosing receipt/audit rolls all native effects back together.
	tx, err = e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = service.AdministrativeInterruptionTx(ctx, tx, actor, production.Scope{BranchIDs: []string{e.foodBranch}}, id, false, "TEST_ROLLBACK", 4); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil && !errorsIsClosedTx(err) {
		t.Fatal(err)
	}
	if err = e.pool.QueryRow(ctx, `SELECT status FROM production_jobs WHERE id=$1`, id).Scan(&state); err != nil || state != "IN_PROGRESS" {
		t.Fatal("native rollback failed", state, err)
	}
}
func errorsIsClosedTx(err error) bool { return err == pgx.ErrTxClosed }
