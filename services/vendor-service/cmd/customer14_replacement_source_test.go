package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

func TestCustomer14ReplacementUsesFreshCheckoutAndLogisticsDB(t *testing.T) {
	for _, tc := range []struct {
		name               string
		price, weight, fee int64
		class              string
	}{
		{"equal", 1000, 1000, 333, "MOTORCYCLE"},
		{"cheaper", 800, 30000, 444, "CAR"},
		{"more_expensive", 1200, 300000, 555, "VAN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSection17BEnv(t, 15*time.Minute)
			ctx := context.Background()
			originalItem := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Original", PriceMinor: 1000, Quantity: 100})
			replacement := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Replacement", PriceMinor: tc.price, Quantity: 100})
			for variant, weight := range map[string]int64{originalItem.VariantID: 1000, replacement.VariantID: tc.weight} {
				if _, err := env.pool.Exec(ctx, `UPDATE shop_variants SET net_weight_grams=$2,stackable=true WHERE id=$1`, variant, weight); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := env.pool.Exec(ctx, `INSERT INTO vendor_branch_service_areas(branch_id,business_id,mode,center_latitude,center_longitude,radius_meters) VALUES($1,$2,'RADIUS',6.5244,3.3792,15000)`, env.branchA1, env.businessA); err != nil {
				t.Fatal(err)
			}
			customer := env.customer("replacement")
			current := env.seedShopCart(customer, s17bCartLine{Item: originalItem, Quantity: 1})
			source, err := env.checkout.StartCheckout(ctx, customer, current.ID, current.Version)
			if err != nil {
				t.Fatal(err)
			}
			if source.Status != checkout.StatusValidated {
				t.Fatalf("checkout refused: %+v", source)
			}
			planner, err := fulfilment.NewPlanner(fulfilment.Options{Logistics: newSection17CLogisticsAuthority(env.pool), Branches: newSection17CBranchAuthority(env.pool), Dispatch: fixedQuoteAuthority{333}, Taxi: fixedQuoteAuthority{444}, Hauling: fixedQuoteAuthority{555}})
			if err != nil {
				t.Fatal(err)
			}
			planning, err := fulfilment.NewService(fulfilment.ServiceOptions{Store: fulfilment.NewStore(env.pool), Planner: planner, Checkout: newSection17CCheckoutAuthority(env.pool)})
			if err != nil {
				t.Fatal(err)
			}
			originalPlan, err := planning.Plan(ctx, customer, source.ID)
			if err != nil {
				t.Fatal(err)
			}
			quotes, err := checkoutquote.NewService(checkoutquote.Options{Store: checkoutquote.NewStore(env.pool, events.NewDomainPublisher(events.NewOutboxRepository(env.pool))), QuoteTTL: 5 * time.Minute,
				ReplacementSource: customer14ReplacementSource{env.pool, env.checkout, planning}})
			if err != nil {
				t.Fatal(err)
			}
			original, err := quotes.Create(ctx, customer, source.ID, originalPlan.ID)
			if err != nil {
				t.Fatal(err)
			}
			original, err = quotes.Approve(ctx, customer, original.ID, original.QuoteFingerprint)
			if err != nil {
				t.Fatal(err)
			}
			create := func() (checkoutquote.Quote, error) {
				return quotes.CreateSubstitutionReplacementQuote(ctx, customer, source.ID, originalPlan.ID, current.Items[0].ID, replacement.ProductID, replacement.VariantID, replacement.ListingID, replacement.SellUnitID, tc.price)
			}
			if _, err = quotes.CreateSubstitutionReplacementQuote(ctx, customer, source.ID, originalPlan.ID, current.Items[0].ID, replacement.ProductID, replacement.VariantID, replacement.ListingID, replacement.SellUnitID, tc.price+1); !errors.Is(err, checkout.ErrSessionNotLive) {
				t.Fatalf("stale candidate price accepted: %v", err)
			}
			fresh, err := create()
			if err != nil {
				t.Fatal(err)
			}
			if fresh.CheckoutSessionID == source.ID || fresh.FulfilmentPlanID == originalPlan.ID || fresh.ID == original.ID {
				t.Fatal("replacement reused original evidence")
			}
			if fresh.ProductSubtotalMinor != tc.price || fresh.DeliverySubtotalMinor != tc.fee || fresh.Lines[0].VariantRef != replacement.VariantID {
				t.Fatalf("stale replacement commercial evidence: %+v", fresh)
			}
			if len(fresh.Missions) != 1 || fresh.Missions[0].VehicleClass != tc.class {
				t.Fatalf("physical escalation: %+v", fresh.Missions)
			}
			var wg sync.WaitGroup
			failures := make(chan error, 6)
			for i := 0; i < 6; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					duplicate, e := create()
					if e == nil && duplicate.ID != fresh.ID {
						e = errors.New("duplicate immutable replacement quote")
					}
					failures <- e
				}()
			}
			wg.Wait()
			close(failures)
			for e := range failures {
				if e != nil {
					t.Fatal(e)
				}
			}
			if _, err = quotes.Approve(ctx, customer, fresh.ID, fresh.QuoteFingerprint); err != nil {
				t.Fatal(err)
			}
			if _, err = newSection17EQuoteAuthority(env.pool).LoadApprovedQuote(ctx, customer, fresh.ID, time.Now()); !errors.Is(err, orders.ErrQuoteNotOrderable) {
				t.Fatalf("replacement evidence could create another purchase: %v", err)
			}
			unchanged, err := quotes.Get(ctx, customer, original.ID)
			if err != nil || unchanged.QuoteFingerprint != original.QuoteFingerprint || unchanged.ProductSubtotalMinor != 1000 {
				t.Fatalf("original quote mutated: %v %+v", err, unchanged)
			}
			var count int
			if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM inventory_reservations WHERE business_id=$1`, env.businessA).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("replacement reserved additional stock: %d", count)
			}
			// Invalidate original material without changing the replacement.
			if _, err = env.pool.Exec(ctx, `UPDATE commerce_checkout_lines SET catalogue_revision=catalogue_revision+1 WHERE session_id=$1`, source.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = quotes.Approve(ctx, customer, fresh.ID, fresh.QuoteFingerprint); !errors.Is(err, checkoutquote.ErrQuoteConflict) {
				t.Fatalf("stale source accepted: %v", err)
			}
			if _, err = env.pool.Exec(ctx, `UPDATE commerce_checkout_sessions SET expires_at=now()-interval '1 second' WHERE id=$1`, source.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = create(); !errors.Is(err, checkoutquote.ErrSourceNotQuotable) {
				t.Fatalf("expired source accepted: %v", err)
			}
		})
	}
}
