package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// approvedCheckoutQuote drives a real cart through checkout, planning and
// quote approval, and stops there: no payment, no order. It is the approved
// commercial evidence Section 21 scheduled and RFQ flows bind to.
func approvedCheckoutQuote(t *testing.T, env *s17bEnv, customer string, current cart.Cart) (*checkoutquote.Service, checkoutquote.Quote) {
	t.Helper()
	ctx := context.Background()
	planner, err := fulfilment.NewPlanner(fulfilment.Options{Logistics: newSection17CLogisticsAuthority(env.pool), Branches: newSection17CBranchAuthority(env.pool),
		Dispatch: fixedQuoteAuthority{333}, Taxi: fixedQuoteAuthority{444}, Hauling: fixedQuoteAuthority{555},
		VendorOwn: section17CVendorDelivery{service: delivery.NewService(env.pool, lifecycleProduction(env), true)}})
	if err != nil {
		t.Fatal(err)
	}
	planning, err := fulfilment.NewService(fulfilment.ServiceOptions{Store: fulfilment.NewStore(env.pool), Planner: planner, Checkout: newSection17CCheckoutAuthority(env.pool)})
	if err != nil {
		t.Fatal(err)
	}
	session, err := env.checkout.StartCheckout(ctx, customer, current.ID, current.Version)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	plan, err := planning.Plan(ctx, customer, session.ID)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	quotes, err := checkoutquote.NewService(checkoutquote.Options{Store: checkoutquote.NewStore(env.pool, events.NewDomainPublisher(events.NewOutboxRepository(env.pool))),
		Promotions: checkoutquote.NoPromotions{}, QuoteTTL: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := quotes.Create(ctx, customer, session.ID, plan.ID)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if quote, err = quotes.Approve(ctx, customer, quote.ID, quote.QuoteFingerprint); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return quotes, quote
}

// E2E-07 (Customer V4 §30): the Cloud Kitchen/Pastry scheduled, bulk and
// custom RFQ path exists only behind its backend capability flag, and with
// the flag on it binds to real approved checkout evidence, idempotently and
// without leaking across customers or vendors.
func TestCustomerV4E2E07ScheduledAndRFQBehindCapabilityFlag(t *testing.T) {
	env := newSection17BEnv(t, 15*time.Minute)
	ctx := context.Background()
	item := env.seedFoodItem("Celebration Cake", 450000)
	env.pool.Exec(ctx, `INSERT INTO food_item_logistics_profiles(item_id,business_id,temperature_class,package_size_class,approx_weight_grams) VALUES($1,$2,'AMBIENT','MEDIUM',2500)`, //nolint:errcheck
		item.ItemID, env.businessA)
	env.pool.Exec(ctx, `INSERT INTO vendor_branch_service_areas(branch_id,business_id,mode,center_latitude,center_longitude,radius_meters)
		SELECT id,business_id,'RADIUS',6.5244,3.3792,15000 FROM vendor_branches WHERE id=$1 ON CONFLICT DO NOTHING`, env.foodBranch) //nolint:errcheck
	customer := env.customer("s21_e2e")
	other := env.customer("s21_other")
	quotes, quote := approvedCheckoutQuote(t, env, customer, env.seedFoodCart(customer, item, 2))
	now := time.Now().UTC()

	// Flag off: every write is refused before any state is touched.
	off := orders.NewSection21Service(orders.Section21Options{DB: env.pool, Checkout: quotes, Enabled: false})
	if _, _, err := off.AcceptScheduled(ctx, orders.ScheduledOrderInput{BusinessID: env.businessA, CustomerID: customer, QuoteID: quote.ID,
		BranchID: env.foodBranch, DesiredDeliveryAt: now.Add(48 * time.Hour), IdempotencyKey: "off"}); !errors.Is(err, orders.ErrSection21Disabled) {
		t.Fatalf("disabled scheduled accept = %v", err)
	}
	if _, err := off.CreateRFQ(ctx, orders.RFQInput{RequesterID: customer, Servings: 40}); !errors.Is(err, orders.ErrSection21Disabled) {
		t.Fatalf("disabled RFQ = %v", err)
	}

	s21 := orders.NewSection21Service(orders.Section21Options{DB: env.pool, Checkout: quotes, Production: lifecycleProduction(env),
		Events: events.NewOutboxRepository(env.pool), Enabled: true})

	// Scheduled/bulk: bound to the approved quote, idempotent per key, and a
	// reused key cannot be re-pointed at different evidence.
	scheduled := orders.ScheduledOrderInput{BusinessID: env.businessA, CustomerID: customer, QuoteID: quote.ID, BranchID: env.foodBranch,
		DesiredDeliveryAt: now.Add(48 * time.Hour), TentativeVehicleClass: "van", BulkRequirement: map[string]any{"servings": 40},
		IdempotencyKey: "sched-e2e-07"}
	first, created, err := s21.AcceptScheduled(ctx, scheduled)
	if err != nil || !created || first.SchedulingStatus != "ACCEPTED" || first.CapacityStatus != "NOT_REQUIRED" || first.TentativeVehicleClass != "VAN" {
		t.Fatalf("scheduled accept = %+v created=%t err=%v", first, created, err)
	}
	again, created, err := s21.AcceptScheduled(ctx, scheduled)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("replay must return the original: %+v created=%t err=%v", again, created, err)
	}
	stolen := scheduled
	stolen.CustomerID = other
	if _, _, err := s21.AcceptScheduled(ctx, stolen); !errors.Is(err, orders.ErrSection21Conflict) {
		t.Fatalf("a reused key for another customer = %v", err)
	}
	unapproved := scheduled
	unapproved.IdempotencyKey, unapproved.CustomerID = "sched-foreign", other
	if _, _, err := s21.AcceptScheduled(ctx, unapproved); !errors.Is(err, orders.ErrSection21CheckoutNeeded) {
		t.Fatalf("another customer's quote must not schedule: %v", err)
	}
	var accepted int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type='commerce.scheduled_order.accepted' AND aggregate_id=$1`, first.ID).Scan(&accepted); err != nil || accepted != 1 {
		t.Fatalf("scheduled acceptance events = %d (%v), want exactly 1", accepted, err)
	}

	// Custom RFQ: an explicit eligible pool only, idempotent quoting, and
	// acceptance bound to the approved checkout quote's exact amount.
	rfq, err := s21.CreateRFQ(ctx, orders.RFQInput{RequesterID: customer, Servings: 40, RequestedAt: now.Add(72 * time.Hour),
		DeadlineAt: now.Add(24 * time.Hour), ExpiresAt: now.Add(48 * time.Hour), Currency: quote.Currency,
		EligibilityScope: map[string]any{"vendor_ids": []string{env.businessA}}, MenuRequirements: map[string]any{"item": "custom cake"}})
	if err != nil || rfq.Status != "DRAFT" {
		t.Fatalf("create RFQ = %+v %v", rfq, err)
	}
	if err := s21.OpenRFQ(ctx, other, rfq.ID); !errors.Is(err, orders.ErrSection21InvalidState) {
		t.Fatalf("another customer opened the RFQ: %v", err)
	}
	if err := s21.OpenRFQ(ctx, customer, rfq.ID); err != nil {
		t.Fatal(err)
	}
	snapshot := map[string]any{"rfq_id": rfq.ID, "checkout_quote_id": quote.ID, "checkout_quote_fingerprint": quote.QuoteFingerprint,
		"vendor_id": env.businessA, "branch_id": env.foodBranch}
	if _, err := s21.SubmitRFQQuote(ctx, rfq.ID, env.businessB, env.branchB1, quote.FinalPayableMinor, quote.Currency, snapshot, orders.CapacityRequirement{}, now.Add(24*time.Hour)); !errors.Is(err, orders.ErrSection21Forbidden) {
		t.Fatalf("an ineligible vendor quoted: %v", err)
	}
	revision, err := s21.SubmitRFQQuote(ctx, rfq.ID, env.businessA, env.foodBranch, quote.FinalPayableMinor, quote.Currency, snapshot, orders.CapacityRequirement{}, now.Add(24*time.Hour), "rfq-quote-1")
	if err != nil || revision.Revision != 1 {
		t.Fatalf("vendor quote = %+v %v", revision, err)
	}
	if replay, err := s21.SubmitRFQQuote(ctx, rfq.ID, env.businessA, env.foodBranch, quote.FinalPayableMinor, quote.Currency, snapshot, orders.CapacityRequirement{}, now.Add(24*time.Hour), "rfq-quote-1"); err != nil || replay.ID != revision.ID {
		t.Fatalf("quote replay created a new revision: %+v %v", replay, err)
	}
	if _, err := s21.ListRFQQuotes(ctx, other, rfq.ID); !errors.Is(err, orders.ErrSection21NotFound) {
		t.Fatalf("another customer listed the RFQ quotes: %v", err)
	}
	if _, err := s21.AcceptRFQQuote(ctx, other, rfq.ID, revision.QuoteID, revision.Revision, quote.ID); !errors.Is(err, orders.ErrSection21NotFound) {
		t.Fatalf("another customer accepted the RFQ: %v", err)
	}
	acceptedRFQ, err := s21.AcceptRFQQuote(ctx, customer, rfq.ID, revision.QuoteID, revision.Revision, quote.ID)
	if err != nil || acceptedRFQ.Status != "ACCEPTED" || acceptedRFQ.CheckoutQuoteID != quote.ID {
		t.Fatalf("accept RFQ = %+v %v", acceptedRFQ, err)
	}
	if replay, err := s21.AcceptRFQQuote(ctx, customer, rfq.ID, revision.QuoteID, revision.Revision, quote.ID); err != nil || replay.Status != "ACCEPTED" {
		t.Fatalf("accept replay = %+v %v", replay, err)
	}
	var bindings int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM commerce_rfq_checkout_bindings WHERE rfq_id=$1 AND checkout_quote_id=$2 AND price_minor=$3`, rfq.ID, quote.ID, quote.FinalPayableMinor).Scan(&bindings); err != nil || bindings != 1 {
		t.Fatalf("checkout bindings = %d (%v), want exactly 1", bindings, err)
	}
}
