package main

import (
	"cosmicforge/logistics/services/vendor-service/internal/features/adminresources"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/shared/go/adminowner"
	"testing"
	"time"
)

func TestVendorSecureAdminStockReadsUseCanonicalExpiryAwareAvailability(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)
	ctx := env.ctx()
	item := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Canonical admin stock", Quantity: 10, PriceMinor: 100})
	expires := time.Now().UTC().Add(2 * time.Second)
	actor := inventory.Actor{BusinessID: env.businessA, StaffID: env.staffA, ActorRef: env.staffA}
	_, err := env.stock.CreateReservation(ctx, actor, mustExactBranchScope(env.businessA, env.branchA1), inventory.CreateReservationInput{BranchID: env.branchA1, SourceType: inventory.ReservationSourceCheckout, SourceRef: "section11-expiry-" + env.suffix, ExpiresAt: &expires, Lines: []inventory.ReservationLineInput{{InventoryItemID: item.InventoryItemID, Quantity: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err = env.pool.QueryRow(ctx, `SELECT id FROM inventory_balances WHERE business_id=$1 AND inventory_item_id=$2`, env.businessA, item.InventoryItemID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	read := func(want int64) string {
		t.Helper()
		tx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		source := adminresources.Source()
		r, err := source.Read(ctx, tx, "stock-balances", id, true)
		if err != nil {
			t.Fatal(err)
		}
		if err = adminowner.ValidateResource(r, adminowner.VendorProfile().Definitions[r.Kind], true); err != nil {
			t.Fatal(err)
		}
		counters := map[string]*int64{}
		for _, c := range r.Counters {
			counters[c.Name] = c.Value
		}
		if counters["available_quantity"] == nil || *counters["available_quantity"] != want {
			t.Fatal("wrong canonical availability", counters["available_quantity"], want)
		}
		b, err := source.Binding(ctx, tx, r.Kind, id)
		if err != nil {
			t.Fatal(err)
		}
		if b.Canonical.Organization != "" || b.Canonical.Tenant != "" {
			t.Fatal("invented inventory tenant")
		}
		return b.Fingerprint
	}
	before := read(0)
	time.Sleep(time.Until(expires.Add(50 * time.Millisecond)))
	after := read(10)
	if before != after {
		t.Fatal("expiry changed native ownership binding")
	}
}

func TestVendorSecureAdminEligibilityUsesNativeVerificationReducer(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := t.Context()
	var id string
	if err := e.f.env.pool.QueryRow(ctx, `SELECT id FROM vendor_branch_capabilities WHERE business_id=$1 AND branch_id=$2 AND status='ACTIVE' LIMIT 1`, e.f.env.businessA, e.f.env.branchA).Scan(&id); err != nil {
		t.Fatal(err)
	}
	read := func(want bool) {
		t.Helper()
		tx, err := e.f.env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		r, err := adminresources.Source().Read(ctx, tx, "branch-capabilities", id, true)
		if err != nil {
			t.Fatal(err)
		}
		if err = adminowner.ValidateResource(r, adminowner.VendorProfile().Definitions[r.Kind], true); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range r.Dimensions {
			if d.Name == "operationally_eligible" {
				found = true
				if d.Flag == nil || *d.Flag != want {
					t.Fatal("wrong native eligibility", d)
				}
			}
		}
		if !found {
			t.Fatal("eligibility missing")
		}
	}
	read(true)
	e.f.env.exec(ctx, `INSERT INTO vendor_onboarding_profiles(business_id,status)VALUES($1,'IN_PROGRESS')ON CONFLICT(business_id)DO UPDATE SET status='IN_PROGRESS'`, e.f.env.businessA)
	read(false)
}
