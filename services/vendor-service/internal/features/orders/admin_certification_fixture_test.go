package orders

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

// Own fixture: an isolated package must not borrow another package's orders.
func seedAdministrativeOrder(t *testing.T, pool *pgxpool.Pool) (parent, child, customer, business string) {
	t.Helper()
	ctx := context.Background()
	parent = seedETAParent(t, pool)
	child = "vo_" + uuid.NewString()
	business = "biz_" + uuid.NewString()
	if err := pool.QueryRow(ctx, `SELECT customer_id FROM commerce_parent_orders WHERE id=$1`, parent).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO commerce_vendor_orders(id,parent_order_id,customer_id,business_id,branch_id,domain,currency,quote_group_ref,product_subtotal_minor,max_product_subtotal_minor,delivery_responsibility,delivery_fee_beneficiary,customer_payable_minor,vendor_payable_minor,fulfilment_mode,status,inventory_state) VALUES($1,$2,$3,$4,$5,'FOOD','NGN',$1,0,0,'PLATFORM_LOGISTICS','PLATFORM_PROVIDER',0,0,'SINGLE','PAID','NOT_REQUIRED')`, child, parent, customer, business, "branch_"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return
}
func seedAdministrativeDispatchReference(t *testing.T, pool *pgxpool.Pool) (ref, parcel string) {
	t.Helper()
	ctx := context.Background()
	parent, child, customer, business := seedAdministrativeOrder(t, pool)
	now := time.Now().UTC()
	store := fulfilmentgroup.NewStore(pool, nil)
	group, err := store.EnsureGroup(ctx, parent, customer, now)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := store.EnsureUnit(ctx, group, fulfilmentgroup.UnitInput{VendorOrderID: child}, now)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := store.CreateParcel(ctx, business, unit.ID, fulfilmentgroup.ParcelInput{IdempotencyKey: "fixture-pack", WeightGrams: 1200}, now)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.ActivatePlan(ctx, group.ID, fulfilmentgroup.PlanInput{ExpectedVersion: 0, Reason: fulfilmentgroup.ReasonInitialPlan, DeliveryMode: fulfilmentgroup.ModePlatform, IdempotencyKey: "fixture-plan", Missions: []fulfilmentgroup.PlanMission{{SlotRef: "fixture-slot", Sequence: 1, Owner: fulfilmentgroup.OwnerPlatform, Authority: fulfilmentgroup.AuthorityDispatch, Currency: "NGN", ParcelIDs: []string{pack.ID}, Stops: []fulfilmentgroup.Stop{{Sequence: 1, Type: fulfilmentgroup.StopPickup, UnitID: unit.ID}, {Sequence: 2, Type: fulfilmentgroup.StopDropoff, Address: "Disposable fixture dropoff"}}}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	mission, _, err := store.EnsurePlatformMissionRef(ctx, group.ID, plan.ID, plan.Missions[0].ID, fulfilmentgroup.AuthorityDispatch, now)
	if err != nil {
		t.Fatal(err)
	}
	return mission.ID, pack.ParcelRef
}
