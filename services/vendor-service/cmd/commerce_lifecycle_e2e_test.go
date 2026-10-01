package main

// Section 14 Food/Shop lifecycle E2E through the real production services:
// cart -> checkout (real sellability, stock) -> fulfilment plan (real Section
// 17C catalogue/branch authorities and requirement engine) -> immutable quote
// -> approval -> ParentOrder/VendorOrders -> Wallet hold -> activation and
// logistics mission -> vendor execution -> the executing authority's own
// pickup/transit/delivery events -> completion -> Wallet settlement -> Share.
//
// Only the external authorities are doubles: the logistics quote/mission
// services, the Wallet and the logistics read side of Share. Everything that
// Vendor Service owns runs as it does in production.

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/customershare"
	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/journeyshare"
)

// settlingWallet adds Wallet's per-leg settlement to fakeAuthority,
// idempotent per settled VendorOrder.
type settlingWallet struct {
	*fakeAuthority
	mu      sync.Mutex
	settled map[string]int
}

type lifecycleCommerceWallet struct {
	*commerceWallet
	settlement *settlingWallet
}

func (w lifecycleCommerceWallet) ReleaseCommerceAllocation(_ context.Context, request orders.CommerceReleaseRequest) (orders.CommerceFinancialResult, error) {
	w.commerceWallet.mu.Lock()
	defer w.commerceWallet.mu.Unlock()
	if result, found := w.operations[request.IdempotencyKey]; found {
		return result, nil
	}
	result := orders.CommerceFinancialResult{Reference: "settle_" + request.IdempotencyKey, Status: "APPLIED"}
	w.operations[request.IdempotencyKey] = result
	w.settlement.mu.Lock()
	w.settlement.settled[request.VendorOrderID]++
	w.settlement.mu.Unlock()
	return result, nil
}

func (w *settlingWallet) HoldLeg(ctx context.Context, request orders.HoldLegRequest) (orders.LegResult, error) {
	result, err := w.fakeAuthority.HoldLeg(ctx, request)
	result.SnapshotFingerprint = "allocation_" + request.SourceReference
	return result, err
}

func (w lifecycleCommerceWallet) CompleteLeg(ctx context.Context, payment, source string) (orders.LegResult, error) {
	return w.settlement.CompleteLeg(ctx, payment, source)
}

func (w *settlingWallet) CompleteLeg(_ context.Context, _, sourceReference string) (orders.LegResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.settled[sourceReference]++
	return orders.LegResult{Reference: "settle_" + sourceReference, Status: "completed"}, nil
}

// shareLogistics answers the executing authority's own customer-safe view.
type shareLogistics struct{ terminal bool }

// The external handover authority returns opaque verification evidence;
// Vendor still executes its real rider, custody and order projection paths.
type lifecycleHandover struct{}

func (lifecycleHandover) VerifyPickup(_ context.Context, r fulfilmentgroup.VendorHandoverRequest) (fulfilmentgroup.HandoverVerification, error) {
	return fulfilmentgroup.HandoverVerification{OpaqueRef: "handover_" + r.OperationRef, EvidenceRef: "proof_" + r.OperationRef, VerifiedAt: time.Now().UTC()}, nil
}
func (h lifecycleHandover) VerifyReceiver(ctx context.Context, r fulfilmentgroup.VendorHandoverRequest) (fulfilmentgroup.HandoverVerification, error) {
	return h.VerifyPickup(ctx, r)
}
func (h lifecycleHandover) VerifyReturn(ctx context.Context, r fulfilmentgroup.VendorHandoverRequest) (fulfilmentgroup.HandoverVerification, error) {
	return h.VerifyPickup(ctx, r)
}

func (s shareLogistics) Read(_ context.Context, authority, ref, owner string) (journeyshare.Projection, error) {
	if s.terminal {
		return journeyshare.Projection{Authority: authority, SourceRef: ref, CustomerID: owner, SourceVersion: 4, Terminal: true, PublicStatus: "COMPLETED", Location: journeyshare.Location{State: "UNAVAILABLE"}}, nil
	}
	return journeyshare.Projection{Authority: authority, SourceRef: ref, CustomerID: owner, SourceVersion: 3,
		Eligible: true, PublicStatus: "IN_PROGRESS", Location: journeyshare.Location{State: "UNAVAILABLE"}}, nil
}

type lifecycleRun struct {
	env      *s17bEnv
	service  *orders.Service
	wallet   *settlingWallet
	missions *physicalMissions
	customer string
	order    orders.ParentOrder
	groups   *fulfilmentgroup.Service
}

func lifecycleExec(t *testing.T, env *s17bEnv, sql string, args ...any) {
	t.Helper()
	if _, err := env.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func lifecycleProduction(env *s17bEnv) *production.Service {
	return production.NewService(production.Options{DB: env.pool, Scopes: productionScopeAdapter{evaluator: staffusecases.NewAuthorizationEvaluator(staffrepositories.NewAssignmentRepository(env.pool))}})
}

// runToPaidOrder drives a real cart through checkout, planning, quote and
// approval into a paid, activated order.
func runToPaidOrder(t *testing.T, env *s17bEnv, customer string, current cart.Cart, branches ...string) *lifecycleRun {
	t.Helper()
	ctx := context.Background()
	for _, branch := range branches {
		env.pool.Exec(ctx, `INSERT INTO vendor_branch_service_areas(branch_id,business_id,mode,center_latitude,center_longitude,radius_meters)
			SELECT id,business_id,'RADIUS',6.5244,3.3792,15000 FROM vendor_branches WHERE id=$1 ON CONFLICT DO NOTHING`, branch) //nolint:errcheck
		env.pool.Exec(ctx, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id) VALUES($1,$2,'role_branch_manager',$3) ON CONFLICT DO NOTHING`, //nolint:errcheck
			"e2e_assign_"+branch, env.staffA, branch)
	}
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
	// Use the mounted customer endpoints against the same catalogue and cart
	// that will be checked out. Preview must not acquire a stock reservation.
	cfg := config.Config{CustomerAccessTokenSecret: []byte("lifecycle-customer-secret")}
	cfg.Flags.CustomerFoodEnabled, cfg.Flags.CustomerShopEnabled = true, true
	cfg.Flags.CommerceFoodEnabled, cfg.Flags.CommerceShopEnabled = true, true
	cfg.Flags.CommerceFulfilmentPlanningEnabled = true
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	registerCustomerDiscoveryRoutes(router, cfg, env.foodService, env.shopService, section13{}, &discoveryTestLimiter{max: 100})
	registerCartDeliveryPreview(router, cfg, env.carts, newSection17BFoodAuthority(env.pool, env.foodService), newSection17BShopAuthority(env.pool, env.shopService, env.stock), planning, &discoveryTestLimiter{max: 100})
	token := discoveryToken(t, cfg.CustomerAccessTokenSecret, customer, "customer", "customer", "access")
	discoveryPath := "/api/v1/commerce/shop/branches/" + branches[0] + "/catalogue"
	if current.Domain == cart.DomainFood {
		discoveryPath = "/api/v1/commerce/food/branches/" + branches[0] + "/catalogue?capability=RESTAURANT"
	}
	for _, path := range []string{discoveryPath, "/api/v1/commerce/carts/" + current.ID + "/delivery-preview"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 200 {
			t.Fatalf("customer endpoint %s: %d %s", path, response.Code, response.Body)
		}
	}
	var reservations int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM commerce_checkout_sessions WHERE cart_id=$1`, current.ID).Scan(&reservations); err != nil || reservations != 0 {
		t.Fatalf("preview created a checkout/reservation: %d %v", reservations, err)
	}
	session, err := env.checkout.StartCheckout(ctx, customer, current.ID, current.Version)
	if err != nil || session.Status != checkout.StatusValidated {
		t.Fatalf("checkout: %v %+v", err, session)
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
	quotes.SetReplacementSourceAuthority(customer14ReplacementSource{db: env.pool, checkout: env.checkout, planning: planning})
	quote, err := quotes.Create(ctx, customer, session.ID, plan.ID)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if quote, err = quotes.Approve(ctx, customer, quote.ID, quote.QuoteFingerprint); err != nil {
		t.Fatalf("approve: %v", err)
	}

	store := orders.NewStore(env.pool)
	store.SetEventPublisher(events.NewDomainPublisher(events.NewOutboxRepository(env.pool)))
	store.SetCustomerOrderMovements(true)
	service, err := orders.NewService(orders.Options{Store: store, Quotes: newSection17EQuoteAuthority(env.pool),
		Scopes: section17EScopeAdapter{evaluator: staffusecases.NewAuthorizationEvaluator(staffrepositories.NewAssignmentRepository(env.pool))}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	wallet := &settlingWallet{fakeAuthority: newFakeAuthority(), settled: map[string]int{}}
	service.SetPaymentAuthority(orders.PaymentOptions{Authority: wallet})
	missions := newPhysicalMissions()
	service.SetFulfilmentAuthorities(newSection17FInventoryAuthority(env.stock), missions)
	groups := fulfilmentgroup.NewService(fulfilmentgroup.Options{
		Store:      fulfilmentgroup.NewStore(env.pool, events.NewDomainPublisher(events.NewOutboxRepository(env.pool))),
		Production: lifecycleProduction(env), Enabled: true, Orders: service, Handover: lifecycleHandover{}})
	service.SetFulfilmentProjection(newFulfilmentOperationsProjection(groups, env.pool))
	service.SetCustomerGroupingPublisher(&customerGroupingPublisher{db: env.pool, outbox: events.NewOutboxRepository(env.pool)})
	service.SetPhysicalLoadEvaluator(newPhysicalLoadEvaluator(env.pool, planner))
	service.SetCommercialChangeAuthority(section17FChangeQuoteAuthority{quotes: quotes})

	order, _, err := service.StartPayment(ctx, customer, quote.ID, quote.QuoteFingerprint)
	if err != nil {
		t.Fatalf("payment: %v", err)
	}
	// A retried payment start is the same order and the same holds.
	holds := wallet.callCount()
	if again, _, retryErr := service.StartPayment(ctx, customer, quote.ID, quote.QuoteFingerprint); retryErr != nil || again.ID != order.ID || wallet.callCount() != holds {
		t.Fatalf("payment retry duplicated work: %v order=%s holds %d->%d", retryErr, again.ID, holds, wallet.callCount())
	}
	if _, err = service.ActivateFulfilment(ctx, order.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if order, err = service.GetForCustomer(ctx, customer, order.ID); err != nil {
		t.Fatal(err)
	}
	for _, mission := range missions.requests {
		var shares int64
		for _, vo := range order.VendorOrders {
			if vo.LogisticsMissionRef == mission.PlanMissionRef {
				shares += vo.DeliveryGrossMinor
			}
		}
		if mission.AgreedFeeMinor != shares {
			t.Fatalf("mission fee=%d, apportioned shares sum=%d", mission.AgreedFeeMinor, shares)
		}
	}
	return &lifecycleRun{env: env, service: service, wallet: wallet, missions: missions, customer: customer, order: order, groups: groups}
}

func (r *lifecycleRun) assertGrouping(t *testing.T, refs ...string) {
	t.Helper()
	var raw []byte
	if err := r.env.pool.QueryRow(context.Background(), `SELECT payload FROM vendor_outbox WHERE aggregate_id=$1 AND event_type='commerce.fulfilment.customer_grouping' ORDER BY aggregate_version DESC LIMIT 1`, r.order.ID).Scan(&raw); err != nil {
		t.Fatalf("missing durable Activity grouping: %v", err)
	}
	var outer struct {
		Payload struct {
			Payload struct {
				CustomerID  string `json:"customer_id"`
				ProductCode string `json:"product_code"`
				Missions    []struct {
					Ref      string `json:"fulfilment_group_ref"`
					Sequence int    `json:"mission_sequence"`
					Count    int    `json:"mission_count"`
				} `json:"missions"`
			} `json:"payload"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		t.Fatal(err)
	}
	group := outer.Payload.Payload
	if group.CustomerID != r.customer || group.ProductCode != r.order.Domain || len(group.Missions) != len(refs) {
		t.Fatalf("incoherent grouping: %s", raw)
	}
	for i, mission := range group.Missions {
		if mission.Ref != refs[i] || mission.Sequence != i+1 || mission.Count != len(refs) {
			t.Fatalf("incorrect mission grouping: %s", raw)
		}
	}
}

func (r *lifecycleRun) actor() orders.Actor {
	return orders.Actor{StaffID: r.env.staffA, VendorID: r.env.businessA, SessionID: "session_" + r.env.staffA}
}

func (r *lifecycleRun) advance(t *testing.T, vendorOrderID string, transitions ...orders.Transition) orders.VendorOrder {
	t.Helper()
	var current orders.VendorOrder
	for _, transition := range transitions {
		order, err := orders.NewStore(r.env.pool).GetVendorOrderForCustomer(context.Background(), r.customer, vendorOrderID)
		if err != nil {
			t.Fatal(err)
		}
		if current, _, err = r.service.Advance(context.Background(), r.actor(), vendorOrderID, transition, "", order.Version); err != nil {
			t.Fatalf("%s from %s: %v", transition, order.Status, err)
		}
	}
	return current
}

func (r *lifecycleRun) mission(t *testing.T, vendorOrderID string) orders.OrderMission {
	t.Helper()
	missions, err := orders.NewStore(r.env.pool).ListMissions(context.Background(), r.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, mission := range missions {
		if mission.Status != orders.MissionAccepted {
			continue
		}
		for _, id := range mission.VendorOrderIDs {
			if id == vendorOrderID {
				return mission
			}
		}
	}
	t.Fatalf("no mission for %s", vendorOrderID)
	return orders.OrderMission{}
}

// deliver reports the executing authority's own lifecycle, keyed by the
// reference that authority actually holds: Dispatch's envelope carries the
// plan slot, Taxi Goods and Hauling carry the mission reference they were given.
func (r *lifecycleRun) deliver(t *testing.T, vendorOrderID string) {
	t.Helper()
	mission := r.mission(t, vendorOrderID)
	ref := mission.MissionRef
	if mission.Service == orders.LogisticsDispatch {
		ref = mission.PlanMissionRef
	}
	lifecycle := []string{"PICKED_UP", "DELIVERING", "DELIVERED"}
	if mission.Service == orders.LogisticsTaxi {
		lifecycle[1] = "IN_TRANSIT"
	}
	for version, eventType := range lifecycle {
		event := orders.LogisticsOrderEvent{EventID: mission.ID + ":" + vendorOrderID + ":" + eventType, MissionRef: ref, VendorOrderID: vendorOrderID,
			ParentOrderID: r.order.ID, Source: mission.Service, EventType: eventType, SourceVersion: int64(version + 1),
			OccurredAt: time.Now().UTC(), Payload: json.RawMessage(`{}`)}
		if _, _, err := r.service.ApplyLogisticsEvent(context.Background(), event); err != nil {
			t.Fatalf("%s %s: %v", mission.Service, eventType, err)
		}
		// At-least-once delivery: a duplicate is absorbed.
		if _, _, err := r.service.ApplyLogisticsEvent(context.Background(), event); err != nil {
			t.Fatalf("duplicate %s: %v", eventType, err)
		}
	}
	// An authority that is not executing this mission cannot move the order.
	stranger := orders.LogisticsOrderEvent{EventID: "stranger:" + mission.ID, MissionRef: "vfm_not_this_mission", VendorOrderID: vendorOrderID,
		ParentOrderID: r.order.ID, Source: mission.Service, EventType: "DELIVERED", SourceVersion: 9, OccurredAt: time.Now().UTC(), Payload: json.RawMessage(`{}`)}
	if _, _, err := r.service.ApplyLogisticsEvent(context.Background(), stranger); !errors.Is(err, orders.ErrForbidden) {
		t.Fatalf("unrelated mission reference was accepted: %v", err)
	}
}

// complete finishes a delivered VendorOrder and checks settlement and Share.
func (r *lifecycleRun) complete(t *testing.T, vendorOrderID string, service orders.LogisticsService) {
	t.Helper()
	ctx := context.Background()
	mission := r.mission(t, vendorOrderID)
	if mission.Service != service {
		t.Fatalf("mission executed by %s, want %s", mission.Service, service)
	}
	r.assertGrouping(t, mission.MissionRef)

	// Share, while the mission is live: the executing authority's safe view
	// only, never the order's money, items, handover credentials or ids.
	share := &customershare.Service{DB: r.env.pool, Enabled: true, Logistics: shareLogistics{},
		EnabledAuthorities: map[string]bool{"DISPATCH": true, "HAULING": true, "TAXI_GOODS": true, "VENDOR_OWN_DELIVERY": true}}
	projection, err := share.Read(ctx, mission.ID, r.customer)
	if err != nil {
		t.Fatalf("share read: %v", err)
	}
	raw, _ := json.Marshal(projection.Public([]string{"route", "provider", "location", "eta"}))
	for _, private := range []string{r.customer, r.order.ID, vendorOrderID, mission.MissionRef, "price", "payment", "refund", "otp", "qr", "fee"} {
		if strings.Contains(strings.ToLower(string(raw)), strings.ToLower(private)) {
			t.Fatalf("share leaks %q: %s", private, raw)
		}
	}
	if _, err = share.Read(ctx, mission.ID, "someone-else"); err == nil {
		t.Fatal("share readable by another customer")
	}

	r.deliver(t, vendorOrderID)
	share.Logistics = shareLogistics{terminal: true}
	if p, readErr := share.Read(ctx, mission.ID, r.customer); readErr != nil || !p.Terminal || p.Eligible || p.Location.Available {
		t.Fatalf("terminal platform Share: %+v %v", p, readErr)
	}
	done := r.advance(t, vendorOrderID, orders.TransitionComplete)
	if done.Status != orders.VendorStatusCompleted {
		t.Fatalf("status=%s", done.Status)
	}
	r.wallet.mu.Lock()
	settled := r.wallet.settled[vendorOrderID]
	r.wallet.mu.Unlock()
	if settled != 1 {
		t.Fatalf("settlements for %s = %d, want 1", vendorOrderID, settled)
	}
	var outboxEvents int
	if err = r.env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE aggregate_id=$1 AND event_type IN ('commerce.vendor_order.logistics_delivered','commerce.vendor_order.completed')`,
		vendorOrderID).Scan(&outboxEvents); err != nil || outboxEvents < 2 {
		t.Fatalf("activity events for %s = %d (%v), want delivered and completed", vendorOrderID, outboxEvents, err)
	}
}

func TestCommerceLifecycleE2E_Food(t *testing.T) {
	env := newSection17BEnv(t, 15*time.Minute)
	item := env.seedFoodItem("Jollof Rice", 320000)
	env.pool.Exec(context.Background(), `INSERT INTO food_item_logistics_profiles(item_id,business_id,temperature_class,package_size_class,approx_weight_grams) VALUES($1,$2,'HOT','SMALL',900)`, //nolint:errcheck
		item.ItemID, env.businessA)
	customer := env.customer("food_e2e")
	run := runToPaidOrder(t, env, customer, env.seedFoodCart(customer, item, 2), env.foodBranch)

	if run.order.Domain != "FOOD" || len(run.order.VendorOrders) != 1 {
		t.Fatalf("order domain=%s vendor orders=%d", run.order.Domain, len(run.order.VendorOrders))
	}
	vendorOrder := run.order.VendorOrders[0]
	if run.missions.distinctCreated("") != 1 {
		t.Fatalf("missions created=%d, want 1", run.missions.distinctCreated(""))
	}
	run.advance(t, vendorOrder.ID, orders.TransitionAccept, orders.TransitionPreparing, orders.TransitionReadyForPickup)
	run.complete(t, vendorOrder.ID, orders.LogisticsDispatch)

	final, err := run.service.GetForCustomer(context.Background(), customer, run.order.ID)
	if err != nil || final.State != orders.ParentStateCompleted {
		t.Fatalf("parent state=%s err=%v", final.State, err)
	}
	if final.CustomerApprovedTotalMinor != run.order.CustomerApprovedTotalMinor {
		t.Fatal("the approved total moved during fulfilment")
	}
}

// Shop, one executor per vehicle class: the load decides the authority.
func TestCommerceLifecycleE2E_ShopPerExecutor(t *testing.T) {
	for _, tc := range []struct {
		name    string
		weight  int64
		service orders.LogisticsService
	}{
		{"dispatch_motorcycle", 1_000, orders.LogisticsDispatch},
		{"taxi_goods_car", 30_000, orders.LogisticsTaxi},
		{"hauling_van", 300_000, orders.LogisticsHauling},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSection17BEnv(t, 15*time.Minute)
			item := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Goods", PriceMinor: 1000, Quantity: 100})
			env.pool.Exec(context.Background(), `UPDATE shop_variants SET net_weight_grams=$2,stackable=true WHERE id=$1`, item.VariantID, tc.weight) //nolint:errcheck
			customer := env.customer("shop_" + tc.name)
			run := runToPaidOrder(t, env, customer, env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1}), env.branchA1)
			vendorOrder := run.order.VendorOrders[0]

			run.advance(t, vendorOrder.ID, orders.TransitionAccept, orders.TransitionPicking)
			reread, _ := orders.NewStore(env.pool).GetVendorOrderForCustomer(context.Background(), customer, vendorOrder.ID)
			if _, err := run.service.ReportLineAvailability(context.Background(), run.actor(), vendorOrder.ID, reread.Lines[0].ID, 0, 1, 0, 0, orders.AvailabilityFullyAvailable); err != nil {
				t.Fatalf("picked: %v", err)
			}
			run.advance(t, vendorOrder.ID, orders.TransitionPack, orders.TransitionReadyForPickup)
			run.complete(t, vendorOrder.ID, tc.service)
			if run.missions.distinctCreated("") != 1 {
				t.Fatalf("missions created=%d, want exactly one", run.missions.distinctCreated(""))
			}
		})
	}
}

// Two vendors, one purchase: each VendorOrder completes on its own, and one
// vendor's cancellation refunds only that vendor's allocation.
func TestCommerceLifecycleE2E_ShopMultiVendorPartialCompletion(t *testing.T) {
	env := newSection17BEnv(t, 15*time.Minute)
	first := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "A goods", PriceMinor: 1000, Quantity: 100})
	second := env.seedShopItem(s17bStockOptions{Business: env.businessB, Staff: env.staffB, Branch: env.branchB1, Name: "B goods", PriceMinor: 2000, Quantity: 100})
	for _, variant := range []string{first.VariantID, second.VariantID} {
		env.pool.Exec(context.Background(), `UPDATE shop_variants SET net_weight_grams=1000,stackable=true WHERE id=$1`, variant) //nolint:errcheck
	}
	env.pool.Exec(context.Background(), `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id) VALUES($1,$2,'role_branch_manager',$3) ON CONFLICT DO NOTHING`, //nolint:errcheck
		"e2e_assign_b_"+env.suffix, env.staffB, env.branchB1)
	customer := env.customer("shop_multi")
	run := runToPaidOrder(t, env, customer, env.seedShopCart(customer, s17bCartLine{Item: first, Quantity: 1}, s17bCartLine{Item: second, Quantity: 1}), env.branchA1, env.branchB1)
	if len(run.order.VendorOrders) != 2 {
		t.Fatalf("vendor orders=%d, want 2", len(run.order.VendorOrders))
	}
	var orderA, orderB orders.VendorOrder
	for _, vendorOrder := range run.order.VendorOrders {
		if vendorOrder.BusinessID == env.businessA {
			orderA = vendorOrder
		} else {
			orderB = vendorOrder
		}
	}

	// Vendor B cancels before handover: only B's leg is refunded.
	actorB := orders.Actor{StaffID: env.staffB, VendorID: env.businessB, SessionID: "session_" + env.staffB}
	if _, _, err := run.service.Advance(context.Background(), actorB, orderB.ID, orders.TransitionCancel, "OUT_OF_STOCK", orderB.Version); err != nil {
		t.Fatalf("cancel B: %v", err)
	}
	for _, refund := range run.wallet.refundedFor() {
		if refund.SourceReference == orderA.ID {
			t.Fatal("vendor B's cancellation refunded vendor A")
		}
	}
	if len(run.wallet.refundedFor()) == 0 {
		t.Fatal("vendor B's cancellation refunded nothing")
	}
	refunds := len(run.wallet.refundedFor())
	_, _, _ = run.service.Advance(context.Background(), actorB, orderB.ID, orders.TransitionCancel, "OUT_OF_STOCK", orderB.Version)
	if len(run.wallet.refundedFor()) != refunds {
		t.Fatal("retried cancellation duplicated refund")
	}
	if available := env.availableStock(env.businessB, env.branchB1, second.InventoryItemID); available != 100 {
		t.Fatalf("cancelled vendor stock=%d", available)
	}

	// Vendor A completes normally.
	run.advance(t, orderA.ID, orders.TransitionAccept, orders.TransitionPicking)
	reread, _ := orders.NewStore(env.pool).GetVendorOrderForCustomer(context.Background(), customer, orderA.ID)
	if _, err := run.service.ReportLineAvailability(context.Background(), run.actor(), orderA.ID, reread.Lines[0].ID, 0, 1, 0, 0, orders.AvailabilityFullyAvailable); err != nil {
		t.Fatalf("picked: %v", err)
	}
	run.advance(t, orderA.ID, orders.TransitionPack, orders.TransitionReadyForPickup)
	run.complete(t, orderA.ID, orders.LogisticsDispatch)

	final, err := run.service.GetForCustomer(context.Background(), customer, run.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A mixed outcome is visible, never hidden behind "completed".
	if final.State == orders.ParentStateCompleted || final.State == orders.ParentStateCancelled {
		t.Fatalf("mixed outcome collapsed to %s", final.State)
	}
	r := run.wallet
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settled[orderB.ID] != 0 {
		t.Fatal("a cancelled vendor order was settled")
	}
}

func TestCommerceLifecycleE2E_ShopVendorOwnDelivery(t *testing.T) {
	env := newSection17BEnv(t, 15*time.Minute)
	ctx := context.Background()
	item := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Own delivery goods", PriceMinor: 1000, Quantity: 100})
	lifecycleExec(t, env, `UPDATE shop_variants SET net_weight_grams=1000,stackable=true WHERE id=$1`, item.VariantID)
	lifecycleExec(t, env, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id) VALUES($1,$2,'role_branch_manager',$3) ON CONFLICT DO NOTHING`, "own_manager_"+env.suffix, env.staffA, env.branchA1)
	policy := delivery.NewService(env.pool, lifecycleProduction(env), true)
	manager := production.Actor{StaffID: env.staffA, VendorID: env.businessA, SessionID: "session_" + env.staffA, CommandRef: "assign_" + env.suffix}
	if _, err := policy.UpsertPolicy(ctx, manager, env.branchA1, 0, delivery.PolicyInput{Enabled: true, Mode: delivery.ModeVendor, FeeRule: delivery.FeeFlat, Currency: "NGN", BaseFeeMinor: 250, OperatingTimezone: "Africa/Lagos", SupportedVehicleClasses: []string{"MOTORCYCLE"}, RiderAvailability: delivery.RiderAvailable, Reason: "Configured own rider delivery"}); err != nil {
		t.Fatal("own policy:", err)
	}
	customer := env.customer("own_delivery")
	run := runToPaidOrder(t, env, customer, env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1}), env.branchA1)
	vo := run.order.VendorOrders[0]
	if vo.DeliveryResponsibility != orders.DeliveryVendorOwn || run.missions.distinctCreated("") != 0 {
		t.Fatalf("own allocation created platform mission: %+v", vo)
	}
	run.advance(t, vo.ID, orders.TransitionAccept, orders.TransitionPicking)
	vo, _ = orders.NewStore(env.pool).GetVendorOrderForCustomer(ctx, customer, vo.ID)
	if _, err := run.service.ReportLineAvailability(ctx, run.actor(), vo.ID, vo.Lines[0].ID, 0, 1, 0, 0, orders.AvailabilityFullyAvailable); err != nil {
		t.Fatal(err)
	}
	run.advance(t, vo.ID, orders.TransitionPack, orders.TransitionReadyForPickup)
	owned, err := run.groups.ListVendorMissions(ctx, manager, 100)
	if err != nil {
		t.Fatal(err)
	}
	var mission fulfilmentgroup.VendorMission
	for _, m := range owned {
		if m.GroupID == run.order.ID {
			mission = m
		}
	}
	// Group IDs are canonical aggregate IDs, not commercial parent IDs.
	if mission.ID == "" {
		for _, m := range owned {
			var parent string
			if err = env.pool.QueryRow(ctx, `SELECT parent_order_id FROM commerce_fulfilment_groups WHERE id=$1`, m.GroupID).Scan(&parent); err != nil {
				t.Fatal(err)
			}
			if parent == run.order.ID {
				mission = m
				break
			}
		}
	}
	if mission.ID == "" {
		t.Fatal("no canonical own delivery mission")
	}
	riderID, userID := "rider_"+env.suffix, "rider_user_"+env.suffix
	lifecycleExec(t, env, `INSERT INTO vendor_users(id,email) VALUES($1,$2)`, userID, userID+"@example.test")
	lifecycleExec(t, env, `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`, riderID, env.businessA, userID)
	lifecycleExec(t, env, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id) VALUES($1,$2,'role_vendor_rider',$3)`, "rider_role_"+env.suffix, riderID, env.branchA1)
	rider := production.Actor{StaffID: riderID, VendorID: env.businessA, SessionID: "session_" + riderID, CommandRef: "available_" + env.suffix}
	if _, err = run.groups.SetRiderAvailability(ctx, rider, env.branchA1, fulfilmentgroup.RiderAvailable, 0); err != nil {
		t.Fatal(err)
	}
	if mission, err = run.groups.AssignRider(ctx, manager, mission.ID, riderID, mission.Version); err != nil {
		t.Fatal(err)
	}
	share := &customershare.Service{DB: env.pool, Enabled: true, EnabledAuthorities: map[string]bool{"VENDOR_OWN_DELIVERY": true}}
	if p, err := share.Read(ctx, mission.ID, customer); err != nil || !p.Eligible || p.Status != "ASSIGNED" || p.Location.Available {
		t.Fatalf("own live Share: %+v %v", p, err)
	}
	if _, err = share.Read(ctx, mission.ID, "someone_else"); err == nil {
		t.Fatal("own Share IDOR")
	}
	for _, action := range []fulfilmentgroup.RiderAction{fulfilmentgroup.ActionAcknowledge, fulfilmentgroup.ActionStartPickup, fulfilmentgroup.ActionArrivePickup, fulfilmentgroup.ActionCompletePickup, fulfilmentgroup.ActionStartReceiver, fulfilmentgroup.ActionArriveReceiver, fulfilmentgroup.ActionCompleteDelivery} {
		rider.CommandRef = mission.ID + ":" + string(action)
		request := fulfilmentgroup.RiderActionRequest{Action: action, OperationRef: rider.CommandRef, ExpectedVersion: mission.Version}
		if action == fulfilmentgroup.ActionCompletePickup || action == fulfilmentgroup.ActionCompleteDelivery {
			request.EvidenceRef = "external_evidence"
		}
		runtime, err := run.groups.ExecuteRiderAction(ctx, rider, mission.ID, request)
		if err != nil {
			t.Fatalf("own %s: %v", action, err)
		}
		if _, err = run.groups.ExecuteRiderAction(ctx, rider, mission.ID, request); err != nil {
			t.Fatalf("own retry %s: %v", action, err)
		}
		mission = runtime.Mission
	}
	done := run.advance(t, vo.ID, orders.TransitionComplete)
	if done.Status != orders.VendorStatusCompleted {
		t.Fatal(done.Status)
	}
	if p, err := share.Read(ctx, mission.ID, customer); err != nil || !p.Terminal || p.Eligible || p.Status != "COMPLETED" || p.Location.Available {
		t.Fatalf("own terminal Share: %+v %v", p, err)
	}
	run.assertGrouping(t, mission.ID)
}

func TestCommerceLifecycleE2E_ShopVariableMeasurementReplan(t *testing.T) {
	env := newSection17BEnv(t, 15*time.Minute)
	ctx := context.Background()
	item := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Measured goods", PriceMinor: 10000, Quantity: 100000, VariableWeight: true, OverfillBps: 10000})
	lifecycleExec(t, env, `UPDATE shop_variants SET net_weight_grams=18000,stackable=true WHERE id=$1`, item.VariantID)
	customer := env.customer("measured_replan")
	run := runToPaidOrder(t, env, customer, env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 2, ExpectedMeasureQuantity: 10000}), env.branchA1)
	vo := run.order.VendorOrders[0]
	old := run.mission(t, vo.ID)
	if old.Service != orders.LogisticsDispatch {
		t.Fatal("estimated load did not select Dispatch:", old)
	}
	run.advance(t, vo.ID, orders.TransitionAccept, orders.TransitionPicking)
	vo, _ = orders.NewStore(env.pool).GetVendorOrderForCustomer(ctx, customer, vo.ID)
	line := vo.Lines[0]
	measured, err := run.service.RecordActualQuantity(ctx, run.actor(), vo.ID, line.ID, 1, 30000, vo.MaxProductSubtotalMinor)
	if err != nil || measured.State != orders.FinancialActualRecorded {
		t.Fatalf("record measurement: %+v %v", measured, err)
	}
	finalized, err := run.service.FinalizeVariableMeasurement(ctx, run.actor(), vo.ID, line.ID, 2)
	if err != nil || finalized.State != orders.FinancialFinalized {
		t.Fatalf("finalize measurement: %+v %v", finalized, err)
	}
	if _, err = run.service.FinalizeVariableMeasurement(ctx, run.actor(), vo.ID, line.ID, 4); err != nil {
		t.Fatalf("measurement retry: %v", err)
	}
	if measured.EstimatedQuantity != 20000 || measured.FinalAmountMinor != 30000 {
		t.Fatalf("measured count/price basis: %+v", measured)
	}
	if _, err = run.service.ReportLineAvailability(ctx, run.actor(), vo.ID, line.ID, 0, 2, 0, 30000, orders.AvailabilityFullyAvailable); err != nil {
		t.Fatal(err)
	}
	run.advance(t, vo.ID, orders.TransitionPack, orders.TransitionReadyForPickup)
	newMission := run.mission(t, vo.ID)
	if newMission.Service != orders.LogisticsTaxi || newMission.VehicleClass != "CAR" || newMission.MissionRef == old.MissionRef {
		t.Fatalf("packed load was not replanned: %+v", newMission)
	}
	if len(run.missions.cancelled) != 1 || run.missions.cancelled[0] != old.MissionRef || run.missions.distinctCreated("") != 2 {
		t.Fatal("replan duplicated or failed to cancel:", run.missions)
	}
	stale := orders.LogisticsOrderEvent{EventID: "stale:" + old.ID, MissionRef: old.MissionRef, VendorOrderID: vo.ID, ParentOrderID: run.order.ID, Source: old.Service, EventType: "PICKED_UP", SourceVersion: 1, OccurredAt: time.Now().UTC(), Payload: json.RawMessage(`{}`)}
	if _, _, err = run.service.ApplyLogisticsEvent(ctx, stale); !errors.Is(err, orders.ErrForbidden) {
		t.Fatalf("replaced executor callback accepted: %v", err)
	}
	stale.EventID += ":slot"
	stale.MissionRef = old.PlanMissionRef
	if _, _, err = run.service.ApplyLogisticsEvent(ctx, stale); !errors.Is(err, orders.ErrForbidden) {
		t.Fatalf("old authority using current slot accepted: %v", err)
	}
	stillReady, err := orders.NewStore(env.pool).GetVendorOrderForCustomer(ctx, customer, vo.ID)
	if err != nil || stillReady.Status != orders.VendorStatusReadyForPickup {
		t.Fatalf("rejected callback mutated order: %+v %v", stillReady, err)
	}
	run.complete(t, vo.ID, orders.LogisticsTaxi)
	if available := env.availableStock(env.businessA, env.branchA1, item.InventoryItemID); available != 70000 {
		t.Fatalf("measured stock available=%d, want 70000", available)
	}
	if len(run.wallet.refundedFor()) != 1 || run.wallet.refundedFor()[0].AmountKobo != 10000 {
		t.Fatalf("measured cap refund: %+v", run.wallet.refundedFor())
	}
}

func TestCommerceLifecycleE2E_ShopSubstitutionThroughCompletion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		price   int64
		approve bool
		policy  string
	}{
		{"equal", 1000, true, "CONTACT_FIRST"}, {"cheaper_unknown_refund", 700, true, "CONTACT_FIRST"},
		{"costlier_real_requote", 1400, true, "CONTACT_FIRST"}, {"reject", 1000, false, "CONTACT_FIRST"}, {"exact_only", 1000, false, "EXACT_ONLY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSection17BEnv(t, 15*time.Minute)
			ctx := context.Background()
			original := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Original goods", PriceMinor: 1000, Quantity: 100})
			replacement := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Replacement goods", PriceMinor: tc.price, Quantity: 100})
			lifecycleExec(t, env, `UPDATE shop_variants SET net_weight_grams=1000,stackable=true WHERE id=$1`, original.VariantID)
			lifecycleExec(t, env, `UPDATE shop_variants SET net_weight_grams=30000,stackable=true WHERE id=$1`, replacement.VariantID)
			customer := env.customer("sub_chain_" + tc.name)
			run := runToPaidOrder(t, env, customer, env.seedShopCart(customer, s17bCartLine{Item: original, Quantity: 1, SubstitutionPolicy: tc.policy}), env.branchA1)
			vo := run.order.VendorOrders[0]
			run.advance(t, vo.ID, orders.TransitionAccept, orders.TransitionPicking)
			vo, _ = orders.NewStore(env.pool).GetVendorOrderForCustomer(ctx, customer, vo.ID)
			line := vo.Lines[0]
			if _, err := run.service.ReportLineAvailability(ctx, run.actor(), vo.ID, line.ID, 0, 0, 1, 0, orders.AvailabilityUnavailable); err != nil {
				t.Fatal(err)
			}
			wallet := newCommerceWallet(run.wallet.fakeAuthority, run.order)
			run.service.SetPaymentAuthority(orders.PaymentOptions{Authority: lifecycleCommerceWallet{wallet, run.wallet}})
			expires := time.Now().Add(10 * time.Minute)
			proposal, err := run.service.ProposeSubstitution(ctx, run.actor(), vo.ID, line.ID, orders.SubstitutionProposal{ProposedCatalogueRef: replacement.ListingID, Reason: "out of stock", ExpiresAt: &expires}, "sub-chain-"+vo.ID)
			if tc.policy == "EXACT_ONLY" {
				if !errors.Is(err, orders.ErrOrderConflict) {
					t.Fatalf("exact-only proposal: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal("propose:", err)
				}
				if tc.price < 1000 {
					wallet.loseNextResponse = true
				}
				_, err = run.service.RespondToSubstitution(ctx, customer, proposal.ID, tc.approve, proposal.Version)
				if tc.price > 1000 && tc.approve {
					var required *orders.SubstitutionChangeRequiredError
					if !errors.As(err, &required) {
						t.Fatalf("costlier quote approval: %v", err)
					}
					change, readErr := run.service.GetCommercialChange(ctx, customer, required.ChangeID)
					if readErr != nil {
						t.Fatal(readErr)
					}
					if _, err = run.service.ApproveCommercialChange(ctx, customer, change.ID, change.ReplacementQuote.Fingerprint); err != nil {
						t.Fatal("approve fresh quote:", err)
					}
				} else if tc.price < 1000 && tc.approve {
					if err == nil {
						t.Fatal("lost Wallet refund response reported success")
					}
				} else if err != nil {
					t.Fatal("respond:", err)
				}
				if tc.approve {
					for retry := 0; retry < 2; retry++ {
						if _, err = run.service.RespondToSubstitution(ctx, customer, proposal.ID, true, proposal.Version); err != nil {
							t.Fatal("substitution retry:", err)
						}
					}
				}
			}
			if !tc.approve {
				// The customer chose no replacement; the original is subsequently
				// found and the same purchase completes with its original goods.
				if _, err = run.service.ReportLineAvailability(ctx, run.actor(), vo.ID, line.ID, 1, 1, 0, 0, orders.AvailabilityFullyAvailable); err != nil {
					t.Fatal(err)
				}
			} else {
				// Picker confirms that the approved replacement was actually found.
				if _, err = run.service.ReportLineAvailability(ctx, run.actor(), vo.ID, line.ID, 1, 0, 1, 0, orders.AvailabilityFullyAvailable); err != nil {
					t.Fatal(err)
				}
			}
			run.advance(t, vo.ID, orders.TransitionPack, orders.TransitionReadyForPickup)
			executor := orders.LogisticsDispatch
			if tc.approve {
				executor = orders.LogisticsTaxi
			}
			run.complete(t, vo.ID, executor)
			originalStock := env.availableStock(env.businessA, env.branchA1, original.InventoryItemID)
			replacementStock := env.availableStock(env.businessA, env.branchA1, replacement.InventoryItemID)
			if tc.approve && (originalStock != 100 || replacementStock != 99) {
				t.Fatalf("replacement stock original=%d replacement=%d", originalStock, replacementStock)
			}
			if !tc.approve && (originalStock != 99 || replacementStock != 100) {
				t.Fatalf("rejected stock original=%d replacement=%d", originalStock, replacementStock)
			}
			if tc.price < 1000 && tc.approve && (wallet.refundCalls != 1 || wallet.refunded[vo.ID] != 300) {
				t.Fatalf("refund duplicated: %+v", wallet)
			}
		})
	}
}
