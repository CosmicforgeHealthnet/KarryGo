package main

// Section 17E integration tests.
//
// These run against real PostgreSQL because the thing under test IS the
// durable orchestration: the exactly-once creation constraint, the multi-vendor
// atomicity, the per-vendor isolation and the aggregate-state re-derivation are
// all database behaviour, and an in-memory fake would prove none of them.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
)

type section17EFixture struct {
	env     *s17cEnv
	service *orders.Service
	quotes  orders.QuoteAuthority

	customerID string
	quoteID    string
	finger     string

	staffA string
	staffB string
}

// newSection17EFixture builds an approved Section 17D quote and the real
// Section 17E service on top of it, wired to the real Section 5 permission
// evaluator rather than a stand-in.
func newSection17EFixture(t *testing.T, multiVendor bool) section17EFixture {
	return buildSection17EFixture(t, multiVendor, false)
}

func newSection19VariableFixture(t *testing.T, multiVendor bool) section17EFixture {
	return buildSection17EFixtureWithVariable(t, multiVendor, false, true)
}

// buildSection17EFixture optionally re-plans vendor A's mission as vendor-owned
// delivery BEFORE the quote is created, so the quote freezes that authority as
// its own evidence rather than the order reading it from a live plan row.
func buildSection17EFixture(t *testing.T, multiVendor, vendorOwnDeliveryForA bool) section17EFixture {
	return buildSection17EFixtureWithVariable(t, multiVendor, vendorOwnDeliveryForA, false)
}

func buildSection17EFixtureWithVariable(t *testing.T, multiVendor, vendorOwnDeliveryForA, variable bool) section17EFixture {
	return buildSection17EFixtureWithVariableAndMode(t, multiVendor, vendorOwnDeliveryForA, variable, "")
}

func buildSection17EFixtureWithVariableAndMode(
	t *testing.T, multiVendor, vendorOwnDeliveryForA, variable bool, initialMode string,
) section17EFixture {
	t.Helper()

	env := newSection17CEnv(t)
	var quoteFixture section17DSource
	if variable && multiVendor {
		quoteFixture = seedSection19VariableMultiSource(t, env, nil)
	} else {
		quoteFixture = seedSection17DSource(t, env, variable, multiVendor, nil)
	}
	ctx := context.Background()
	if initialMode != "" {
		// The plan is the authority for the mode frozen into the quote. Set the
		// fixture's intended mode before quote creation; immutable quote evidence
		// is never updated after insertion.
		env.exec(ctx, `UPDATE commerce_fulfilment_plans SET mode=$2 WHERE id=$1`, quoteFixture.planID, initialMode)
	}
	// Vendor B in the multi-vendor fixture uses a platform delivery subsidy.
	// Supply the same durable approval evidence production activation requires,
	// before the quote freezes the plan.
	env.exec(ctx, `UPDATE commerce_fulfilment_plans
		SET delivery_policy_snapshot=jsonb_build_object(
			'hybrid_platform_approval_ref',$2::text)
		WHERE id=$1`, quoteFixture.planID, "platform_approval_"+env.suffix)

	if vendorOwnDeliveryForA {
		// Vendor-owned delivery is priced by the vendor's own policy, so the
		// plan schema requires it to carry no platform authority quote.
		env.exec(ctx, `
			UPDATE commerce_fulfilment_missions
			   SET authority='VENDOR_OWN_DELIVERY',service='VENDOR_OWN_DELIVERY',
			       fee_beneficiary='VENDOR',vehicle_class='',
			       authority_quote_id='',authority_quote_reference='',
			       authority_quote_revision=0,authority_quote_expires_at=NULL
			 WHERE plan_id=$1 AND pickup_business_id=$2`,
			quoteFixture.planID, env.businessA)
		env.exec(ctx, `
			UPDATE commerce_fulfilment_plan_groups
			   SET authority='VENDOR_OWN_DELIVERY'
			 WHERE plan_id=$1 AND business_id=$2`,
			quoteFixture.planID, env.businessA)
	}

	quote, err := quoteFixture.service.Create(
		ctx, quoteFixture.customerID, quoteFixture.sessionID, quoteFixture.planID,
	)
	if err != nil {
		t.Fatalf("create quote: %v", err)
	}
	approved, err := quoteFixture.service.Approve(
		ctx, quoteFixture.customerID, quote.ID, quote.QuoteFingerprint,
	)
	if err != nil {
		t.Fatalf("approve quote: %v", err)
	}
	if approved.State != checkoutquote.StateApproved {
		t.Fatalf("quote state=%s", approved.State)
	}

	fixture := section17EFixture{
		env:        env,
		quotes:     newSection17EQuoteAuthority(env.pool),
		customerID: quoteFixture.customerID,
		quoteID:    quote.ID,
		finger:     quote.QuoteFingerprint,
		staffA:     "s17c_staff_a_" + env.suffix,
		staffB:     "s17c_staff_b_" + env.suffix,
	}

	// Real IAM: the branch-manager role already carries vendor.orders.read and
	// vendor.orders.manage from the Section 5 seed, so Section 17E needs no new
	// permission and no new role mapping.
	for _, grant := range []struct{ staff, branch string }{
		{fixture.staffA, env.branchA},
		{fixture.staffB, env.branchB},
	} {
		env.exec(ctx, `
			INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id)
			VALUES($1,$2,'role_branch_manager',$3)`,
			"s17e_assign_"+grant.staff, grant.staff, grant.branch)
	}

	evaluator := staffusecases.NewAuthorizationEvaluator(
		staffrepositories.NewAssignmentRepository(env.pool),
	)
	service, err := orders.NewService(orders.Options{
		Store:   orders.NewStore(env.pool),
		Quotes:  fixture.quotes,
		Scopes:  section17EScopeAdapter{evaluator: evaluator},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("build order service: %v", err)
	}
	fixture.service = service
	return fixture
}

func (f section17EFixture) actor(staffID, businessID string) orders.Actor {
	return orders.Actor{StaffID: staffID, VendorID: businessID, SessionID: "session_" + staffID}
}

func (f section17EFixture) vendorOrderFor(
	t *testing.T, parent orders.ParentOrder, businessID string,
) orders.VendorOrder {
	t.Helper()
	for _, vendorOrder := range parent.VendorOrders {
		if vendorOrder.BusinessID == businessID {
			return vendorOrder
		}
	}
	t.Fatalf("no vendor order for %s", businessID)
	return orders.VendorOrder{}
}

// -----------------------------------------------------------------------------
// Creation
// -----------------------------------------------------------------------------

func TestSection17EOrders_FoodParentOrderHasOneVendorOrder(t *testing.T) {
	fixture := newSection17EFixture(t, false)
	ctx := context.Background()

	parent, created, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if !created {
		t.Fatal("first creation reported as a retry")
	}
	if len(parent.VendorOrders) != 1 || parent.VendorOrderCount != 1 {
		t.Fatalf("vendor orders=%d count=%d", len(parent.VendorOrders), parent.VendorOrderCount)
	}
	if parent.State != orders.ParentStateCreated {
		t.Fatalf("state=%s", parent.State)
	}
	// The approved figure is copied, never recomputed.
	if parent.CustomerApprovedTotalMinor != 1200 {
		t.Fatalf("approved total=%d, want 1200", parent.CustomerApprovedTotalMinor)
	}
	if parent.QuoteFingerprint != fixture.finger {
		t.Fatal("parent order does not bind the approved fingerprint")
	}

	vendorOrder := parent.VendorOrders[0]
	if vendorOrder.CustomerPayableMinor+parent.PlatformFeeTaxMinor !=
		parent.CustomerApprovedTotalMinor {
		t.Fatalf("allocation does not reconcile: vendor=%d platform=%d total=%d",
			vendorOrder.CustomerPayableMinor, parent.PlatformFeeTaxMinor,
			parent.CustomerApprovedTotalMinor)
	}
	if len(vendorOrder.Lines) != 1 {
		t.Fatalf("item snapshots=%d, want 1", len(vendorOrder.Lines))
	}
	if vendorOrder.Lines[0].OnlinePriceDisclosure != "VENDOR_ONLINE_PRICE" ||
		vendorOrder.Lines[0].StorePriceParityClaimed {
		t.Fatal("order line lost Section 17D's online-price disclosure")
	}
	if vendorOrder.ReservationRef == "" {
		t.Fatal("vendor order carries no Inventory reservation reference")
	}
}

func TestSection17EOrders_MultiVendorShopOrderIsolatesEveryAllocation(t *testing.T) {
	fixture := newSection17EFixture(t, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if len(parent.VendorOrders) != 2 {
		t.Fatalf("vendor orders=%d, want 2", len(parent.VendorOrders))
	}

	first := fixture.vendorOrderFor(t, parent, fixture.env.businessA)
	second := fixture.vendorOrderFor(t, parent, fixture.env.businessB)

	// Each vendor order carries its own product money, its own stock
	// reservation and its own mission. None of them is a gross figure.
	if first.ProductSubtotalMinor != 1000 || second.ProductSubtotalMinor != 2000 {
		t.Fatalf("product allocation a=%d b=%d",
			first.ProductSubtotalMinor, second.ProductSubtotalMinor)
	}
	if first.ReservationRef == second.ReservationRef {
		t.Fatal("two vendors share one Inventory reservation")
	}
	if first.LogisticsMissionRef == second.LogisticsMissionRef {
		t.Fatal("two vendors share one logistics mission")
	}
	if first.BranchID == second.BranchID {
		t.Fatal("two vendors share one branch")
	}

	// The whole purchase reconciles exactly, with nothing unattributed.
	var customer int64
	for _, vendorOrder := range parent.VendorOrders {
		customer += vendorOrder.CustomerPayableMinor
		for _, allocation := range vendorOrder.Allocations {
			if allocation.BeneficiaryType == orders.BeneficiaryTypeVendor &&
				allocation.BeneficiaryRef != vendorOrder.BusinessID {
				t.Errorf("%s holds an allocation for %s",
					vendorOrder.BusinessID, allocation.BeneficiaryRef)
			}
		}
	}
	if customer+parent.PlatformFeeTaxMinor != parent.CustomerApprovedTotalMinor {
		t.Fatalf("multi-vendor allocation does not reconcile: %d + %d != %d",
			customer, parent.PlatformFeeTaxMinor, parent.CustomerApprovedTotalMinor)
	}

	// Vendor A's delivery is vendor-subsidised and vendor B's is
	// platform-subsidised. Each subsidy must stay on the vendor order that
	// earned it.
	if first.DeliveryVendorFundedMinor != 300 || first.DeliveryPlatformFundedMinor != 0 {
		t.Errorf("vendor A delivery funding vendor=%d platform=%d",
			first.DeliveryVendorFundedMinor, first.DeliveryPlatformFundedMinor)
	}
	if second.DeliveryVendorFundedMinor != 0 || second.DeliveryPlatformFundedMinor != 200 {
		t.Errorf("vendor B delivery funding vendor=%d platform=%d",
			second.DeliveryVendorFundedMinor, second.DeliveryPlatformFundedMinor)
	}
}

func TestSection17EOrders_CreationIsIdempotentPerApprovedQuote(t *testing.T) {
	fixture := newSection17EFixture(t, true)
	ctx := context.Background()

	first, created, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}
	second, created, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if created {
		t.Fatal("a retry reported a second creation")
	}
	if second.ID != first.ID {
		t.Fatalf("retry produced a different order: %s vs %s", first.ID, second.ID)
	}

	// A retry may not duplicate an order, a vendor order, a line or an
	// allocation.
	counts := map[string]int{}
	count := func(table, query string, arg string) {
		t.Helper()
		var value int
		if err = fixture.env.pool.QueryRow(ctx, query, arg).Scan(&value); err != nil {
			t.Fatalf("%s count: %v", table, err)
		}
		counts[table] = value
	}
	count("parent", `SELECT count(*) FROM commerce_parent_orders WHERE quote_id=$1`, fixture.quoteID)
	count("vendor", `SELECT count(*) FROM commerce_vendor_orders WHERE parent_order_id=$1`, first.ID)
	count("lines", `SELECT count(*) FROM commerce_vendor_order_lines WHERE parent_order_id=$1`, first.ID)
	count("allocations", `SELECT count(*) FROM commerce_order_allocations WHERE parent_order_id=$1`, first.ID)
	if counts["parent"] != 1 || counts["vendor"] != 2 || counts["lines"] != 2 {
		t.Fatalf("retry duplicated rows: %v", counts)
	}
	if counts["allocations"] == 0 {
		t.Fatal("no allocation was recorded")
	}
}

func TestSection17EOrders_ConcurrentCreationConvergesOnOneOrder(t *testing.T) {
	fixture := newSection17EFixture(t, true)

	const attempts = 12
	ids := make(chan string, attempts)
	errs := make(chan error, attempts)

	var wait sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			order, _, err := fixture.service.CreateFromQuote(
				context.Background(), fixture.customerID, fixture.quoteID, fixture.finger,
			)
			if err != nil {
				errs <- err
				return
			}
			ids <- order.ID
		}()
	}
	wait.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		t.Errorf("concurrent create: %v", err)
	}
	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		} else if id != expected {
			t.Errorf("concurrent creation produced %q and %q", expected, id)
		}
	}

	var rows int
	if err := fixture.env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM commerce_parent_orders WHERE quote_id=$1`,
		fixture.quoteID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("parent order rows=%d err=%v", rows, err)
	}
}

func TestSection17EOrders_WrongFingerprintCannotOrder(t *testing.T) {
	fixture := newSection17EFixture(t, false)

	_, _, err := fixture.service.CreateFromQuote(
		context.Background(), fixture.customerID, fixture.quoteID,
		strings.Repeat("a", 64),
	)
	if !errors.Is(err, orders.ErrFingerprintMismatch) {
		t.Fatalf("err=%v, want ErrFingerprintMismatch", err)
	}
}

func TestSection17EOrders_UnapprovedQuoteCannotOrder(t *testing.T) {
	env := newSection17CEnv(t)
	quoteFixture := seedSection17DSource(t, env, false, false, nil)
	ctx := context.Background()

	quote, err := quoteFixture.service.Create(
		ctx, quoteFixture.customerID, quoteFixture.sessionID, quoteFixture.planID,
	)
	if err != nil {
		t.Fatal(err)
	}

	evaluator := staffusecases.NewAuthorizationEvaluator(
		staffrepositories.NewAssignmentRepository(env.pool),
	)
	service, err := orders.NewService(orders.Options{
		Store:   orders.NewStore(env.pool),
		Quotes:  newSection17EQuoteAuthority(env.pool),
		Scopes:  section17EScopeAdapter{evaluator: evaluator},
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err = service.CreateFromQuote(
		ctx, quoteFixture.customerID, quote.ID, quote.QuoteFingerprint,
	); !errors.Is(err, orders.ErrQuoteNotOrderable) {
		t.Fatalf("an unapproved quote produced an order: %v", err)
	}
}

// Approval and order creation are separate commitments at separate moments, so
// creation re-verifies rather than trusting the earlier approval.
func TestSection17EOrders_ReleasedStockBlocksOrderCreation(t *testing.T) {
	fixture := newSection17EFixture(t, true)
	ctx := context.Background()

	fixture.env.exec(ctx, `
		UPDATE commerce_checkout_groups SET reservation_status='RELEASED'
		 WHERE business_id=$1`, fixture.env.businessB)

	if _, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); !errors.Is(err, orders.ErrQuoteNotOrderable) {
		t.Fatalf("released stock still produced an order: %v", err)
	}

	// Nothing partial may be left behind.
	var rows int
	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_parent_orders WHERE quote_id=$1`,
		fixture.quoteID).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("a failed creation left %d parent orders (err=%v)", rows, err)
	}
}

func TestSection17EOrders_RevalidatedCheckoutBlocksOrderCreation(t *testing.T) {
	fixture := newSection17EFixture(t, false)
	ctx := context.Background()

	// The customer revalidated checkout after approving. The old agreement is
	// still readable history, but it can no longer become an order.
	fixture.env.exec(ctx, `
		UPDATE commerce_checkout_sessions SET validation_revision=validation_revision+1
		 WHERE customer_id=$1`, fixture.customerID)

	if _, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); !errors.Is(err, orders.ErrQuoteNotOrderable) {
		t.Fatalf("stale checkout evidence still produced an order: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Immutability
// -----------------------------------------------------------------------------

func TestSection17EOrders_SnapshotsAndAllocationsAreImmutable(t *testing.T) {
	fixture := newSection17EFixture(t, false)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, statement := range map[string]string{
		"line update": `UPDATE commerce_vendor_order_lines SET unit_price_minor=1
		                 WHERE parent_order_id=$1`,
		"line delete": `DELETE FROM commerce_vendor_order_lines WHERE parent_order_id=$1`,
		"allocation update": `UPDATE commerce_order_allocations SET amount_minor=1
		                       WHERE parent_order_id=$1`,
		"allocation delete": `DELETE FROM commerce_order_allocations WHERE parent_order_id=$1`,
		"event update": `UPDATE commerce_order_events SET action='commerce.order.created'
		                  WHERE parent_order_id=$1`,
		"event delete": `DELETE FROM commerce_order_events WHERE parent_order_id=$1`,
	} {
		if _, err = fixture.env.pool.Exec(ctx, statement, parent.ID); err == nil {
			t.Errorf("database allowed %s on immutable evidence", name)
		}
	}
}

func TestSection17EOrders_HistoricalSnapshotSurvivesCatalogueChange(t *testing.T) {
	fixture := newSection17EFixture(t, false)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	before := parent.VendorOrders[0].Lines[0]

	// The live catalogue and checkout evidence move on.
	fixture.env.exec(ctx, `
		UPDATE commerce_checkout_lines
		   SET unit_price_minor=9999,quoted_total_minor=9999,
		       max_authorized_total_minor=9999,price_reference='online-price-v9',
		       display_name='Renamed product'
		 WHERE customer_id=$1`, fixture.customerID)

	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	after := reread.VendorOrders[0].Lines[0]

	if after.UnitPriceMinor != before.UnitPriceMinor ||
		after.QuotedTotalMinor != before.QuotedTotalMinor ||
		after.PriceReference != before.PriceReference ||
		after.ItemName != before.ItemName {
		t.Fatalf("historical receipt was recomputed:\nbefore=%+v\nafter=%+v", before, after)
	}
	if reread.CustomerApprovedTotalMinor != parent.CustomerApprovedTotalMinor {
		t.Fatal("approved total was recomputed from the live catalogue")
	}
}

// -----------------------------------------------------------------------------
// Fulfilment isolation and aggregate state
// -----------------------------------------------------------------------------

func TestSection17EOrders_VendorOrdersRunIndependentlyAndParentAggregates(t *testing.T) {
	// Vendor A performs its own delivery, so it can carry an allocation all
	// the way to COMPLETED. Vendor B's delivery stays platform-executed.
	fixture := buildSection17EFixture(t, true, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.vendorOrderFor(t, parent, fixture.env.businessA)
	second := fixture.vendorOrderFor(t, parent, fixture.env.businessB)

	actorA := fixture.actor(fixture.staffA, fixture.env.businessA)
	actorB := fixture.actor(fixture.staffB, fixture.env.businessB)

	// Vendor A starts preparing. Vendor B is untouched, and the purchase moves
	// to IN_PROGRESS rather than claiming either extreme.
	moved, aggregate, err := fixture.service.Advance(
		ctx, actorA, first.ID, orders.TransitionAccept, "", first.Version,
	)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if moved.Status != orders.VendorStatusPreparing {
		t.Fatalf("vendor A status=%s", moved.Status)
	}
	if aggregate.State != orders.ParentStateInProgress {
		t.Fatalf("parent state=%s, want IN_PROGRESS", aggregate.State)
	}

	untouched, err := fixture.service.GetForVendor(ctx, actorB, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Status != orders.VendorStatusCreated || untouched.Version != second.Version {
		t.Fatalf("vendor B was disturbed: status=%s version=%d",
			untouched.Status, untouched.Version)
	}

	// Vendor B cancels. Vendor A keeps running, and only vendor B's refund is
	// prepared.
	cancelled, aggregate, err := fixture.service.Advance(
		ctx, actorB, second.ID, orders.TransitionCancel, "OUT_OF_STOCK", second.Version,
	)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.RefundAllocatedMinor != second.CustomerPayableMinor {
		t.Fatalf("refund prepared=%d, want %d",
			cancelled.RefundAllocatedMinor, second.CustomerPayableMinor)
	}
	if aggregate.State != orders.ParentStateInProgress {
		t.Fatalf("a running sibling was hidden: parent state=%s", aggregate.State)
	}
	if aggregate.RefundPreparedMinor != second.CustomerPayableMinor {
		t.Fatalf("parent refund prepared=%d, want %d",
			aggregate.RefundPreparedMinor, second.CustomerPayableMinor)
	}

	stillRunning, err := fixture.service.GetForVendor(ctx, actorA, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillRunning.Status != orders.VendorStatusPreparing {
		t.Fatalf("vendor A was dragged into vendor B's cancellation: %s", stillRunning.Status)
	}
	if stillRunning.RefundAllocatedMinor != 0 {
		t.Fatalf("vendor A had a refund prepared for vendor B's cancellation: %d",
			stillRunning.RefundAllocatedMinor)
	}

	// Vendor A runs to completion. The purchase is now finished, but it did not
	// all end the same way, so the aggregate says so.
	current := stillRunning
	for _, step := range []orders.Transition{
		orders.TransitionReady, orders.TransitionHandover,
		orders.TransitionDeliver, orders.TransitionComplete,
	} {
		current, aggregate, err = fixture.service.Advance(
			ctx, actorA, current.ID, step, "", current.Version,
		)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}
	if current.Status != orders.VendorStatusCompleted {
		t.Fatalf("vendor A status=%s", current.Status)
	}
	if current.SettlementAllocatedMinor != first.VendorPayableMinor {
		t.Fatalf("settlement prepared=%d, want %d",
			current.SettlementAllocatedMinor, first.VendorPayableMinor)
	}
	if aggregate.State != orders.ParentStatePartiallyCompleted {
		t.Fatalf("mixed outcome hidden as %s", aggregate.State)
	}
	if aggregate.SettlementPreparedMinor != first.VendorPayableMinor ||
		aggregate.RefundPreparedMinor != second.CustomerPayableMinor {
		t.Fatalf("parent prepared amounts settlement=%d refund=%d",
			aggregate.SettlementPreparedMinor, aggregate.RefundPreparedMinor)
	}
	if aggregate.TerminalAt == nil {
		t.Fatal("a terminal purchase has no terminal timestamp")
	}
}

func TestSection17EOrders_PlatformDeliveryCannotBeConfirmedByTheVendor(t *testing.T) {
	fixture := newSection17EFixture(t, false)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	vendorOrder := parent.VendorOrders[0]
	if vendorOrder.DeliveryResponsibility != orders.DeliveryPlatform {
		t.Fatalf("fixture responsibility=%s", vendorOrder.DeliveryResponsibility)
	}

	actor := fixture.actor(fixture.staffA, fixture.env.businessA)
	current := vendorOrder
	for _, step := range []orders.Transition{
		orders.TransitionAccept, orders.TransitionReady,
	} {
		current, _, err = fixture.service.Advance(ctx, actor, current.ID, step, "", current.Version)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}

	if _, _, err = fixture.service.Advance(
		ctx, actor, current.ID, orders.TransitionHandover, "", current.Version,
	); !errors.Is(err, orders.ErrForbidden) {
		t.Fatalf("a vendor handed over a platform-executed order: %v", err)
	}

	if _, _, err = fixture.service.Advance(
		ctx, actor, current.ID, orders.TransitionDeliver, "", current.Version,
	); !errors.Is(err, orders.ErrOrderConflict) {
		t.Fatalf("a vendor delivered before platform handover: %v", err)
	}
}

func TestSection17EOrders_WholeCancellationRefundsTheOrderLevelCharge(t *testing.T) {
	fixture := newSection17EFixture(t, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}

	var aggregate orders.ParentOrder
	for _, vendorOrder := range parent.VendorOrders {
		staff := fixture.staffA
		if vendorOrder.BusinessID == fixture.env.businessB {
			staff = fixture.staffB
		}
		_, aggregate, err = fixture.service.Advance(
			ctx, fixture.actor(staff, vendorOrder.BusinessID), vendorOrder.ID,
			orders.TransitionCancel, "VENDOR_CLOSED", vendorOrder.Version,
		)
		if err != nil {
			t.Fatalf("cancel %s: %v", vendorOrder.BusinessID, err)
		}
	}

	if aggregate.State != orders.ParentStateCancelled {
		t.Fatalf("state=%s, want CANCELLED", aggregate.State)
	}
	// Everything the customer paid, including the order-level charge, is now
	// refundable.
	if aggregate.RefundPreparedMinor != aggregate.CustomerApprovedTotalMinor {
		t.Fatalf("refund prepared=%d, want the full approved total %d",
			aggregate.RefundPreparedMinor, aggregate.CustomerApprovedTotalMinor)
	}
	if aggregate.SettlementPreparedMinor != 0 {
		t.Fatalf("a wholly cancelled purchase prepared %d for settlement",
			aggregate.SettlementPreparedMinor)
	}
}

// -----------------------------------------------------------------------------
// Concurrency and security
// -----------------------------------------------------------------------------

func TestSection17EOrders_StaleVersionCannotOverwriteATransition(t *testing.T) {
	fixture := newSection17EFixture(t, false)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	vendorOrder := parent.VendorOrders[0]
	actor := fixture.actor(fixture.staffA, fixture.env.businessA)

	if _, _, err = fixture.service.Advance(
		ctx, actor, vendorOrder.ID, orders.TransitionAccept, "", vendorOrder.Version,
	); err != nil {
		t.Fatal(err)
	}
	// The same command replayed against the version the caller first read.
	if _, _, err = fixture.service.Advance(
		ctx, actor, vendorOrder.ID, orders.TransitionAccept, "", vendorOrder.Version,
	); !errors.Is(err, orders.ErrVersionConflict) {
		t.Fatalf("a stale version was accepted: %v", err)
	}
}

func TestSection17EOrders_ConcurrentSiblingTransitionsAgreeOnOneAggregate(t *testing.T) {
	fixture := newSection17EFixture(t, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errs := make(chan error, len(parent.VendorOrders))
	for _, vendorOrder := range parent.VendorOrders {
		staff := fixture.staffA
		if vendorOrder.BusinessID == fixture.env.businessB {
			staff = fixture.staffB
		}
		actor := fixture.actor(staff, vendorOrder.BusinessID)
		target := vendorOrder

		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, _, advanceErr := fixture.service.Advance(
				context.Background(), actor, target.ID,
				orders.TransitionAccept, "", target.Version,
			); advanceErr != nil {
				errs <- advanceErr
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent sibling transition: %v", err)
	}

	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.State != orders.ParentStateInProgress {
		t.Fatalf("aggregate after concurrent transitions=%s", reread.State)
	}
	for _, vendorOrder := range reread.VendorOrders {
		if vendorOrder.Status != orders.VendorStatusPreparing {
			t.Errorf("%s status=%s", vendorOrder.BusinessID, vendorOrder.Status)
		}
	}
}

func TestSection17EOrders_TenantIsolationAndIDOR(t *testing.T) {
	fixture := newSection17EFixture(t, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.vendorOrderFor(t, parent, fixture.env.businessA)
	second := fixture.vendorOrderFor(t, parent, fixture.env.businessB)

	// A different customer cannot read the purchase.
	if _, err = fixture.service.GetForCustomer(
		ctx, "another_customer", parent.ID,
	); !errors.Is(err, orders.ErrOrderNotFound) {
		t.Fatalf("customer isolation failed: %v", err)
	}

	// Vendor B cannot read or move vendor A's allocation, even holding a valid
	// session and a real vendor order id.
	actorB := fixture.actor(fixture.staffB, fixture.env.businessB)
	if _, err = fixture.service.GetForVendor(
		ctx, actorB, first.ID,
	); !errors.Is(err, orders.ErrOrderNotFound) {
		t.Fatalf("vendor B read vendor A's allocation: %v", err)
	}
	if _, _, err = fixture.service.Advance(
		ctx, actorB, first.ID, orders.TransitionAccept, "", first.Version,
	); !errors.Is(err, orders.ErrOrderNotFound) {
		t.Fatalf("vendor B moved vendor A's allocation: %v", err)
	}

	// A staff member who holds no order permission at all is refused.
	stranger := fixture.actor("s17e_unknown_staff_"+fixture.env.suffix, fixture.env.businessB)
	if _, err = fixture.service.GetForVendor(
		ctx, stranger, second.ID,
	); !errors.Is(err, orders.ErrForbidden) {
		t.Fatalf("an ungranted staff member read an order: %v", err)
	}

	// Vendor B's queue contains only vendor B's allocations.
	queue, err := fixture.service.ListForVendor(ctx, actorB, orders.VendorQueueFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].ID != second.ID {
		t.Fatalf("vendor B queue leaked: %+v", queue)
	}

	// A branch outside the actor's scope is refused rather than answered with
	// an empty page, which would still confirm the branch exists.
	if _, err = fixture.service.ListForVendor(ctx, actorB, orders.VendorQueueFilter{
		BranchID: fixture.env.branchA,
	}); !errors.Is(err, orders.ErrForbidden) {
		t.Fatalf("a foreign branch filter was answered: %v", err)
	}
}

func TestSection17EOrders_CustomerCancelsOneVendorOnly(t *testing.T) {
	fixture := newSection17EFixture(t, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	target := fixture.vendorOrderFor(t, parent, fixture.env.businessA)
	other := fixture.vendorOrderFor(t, parent, fixture.env.businessB)

	// Another customer cannot cancel it.
	if _, _, err = fixture.service.CancelByCustomer(
		ctx, "another_customer", target.ID, "CHANGED_MIND", target.Version,
	); !errors.Is(err, orders.ErrOrderNotFound) {
		t.Fatalf("a foreign customer cancelled an order: %v", err)
	}

	cancelled, aggregate, err := fixture.service.CancelByCustomer(
		ctx, fixture.customerID, target.ID, "CHANGED_MIND", target.Version,
	)
	if err != nil {
		t.Fatalf("customer cancel: %v", err)
	}
	if cancelled.CancelledBy != string(orders.CancelByCustomer) {
		t.Fatalf("cancelled_by=%q", cancelled.CancelledBy)
	}
	if aggregate.State != orders.ParentStateInProgress {
		t.Fatalf("state=%s, want IN_PROGRESS with a live sibling", aggregate.State)
	}

	untouched, err := fixture.service.GetForVendor(
		ctx, fixture.actor(fixture.staffB, fixture.env.businessB), other.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Status != orders.VendorStatusCreated {
		t.Fatalf("the sibling allocation was disturbed: %s", untouched.Status)
	}
}

func TestSection17EOrders_CustomerCannotCancelAfterHandover(t *testing.T) {
	fixture := buildSection17EFixture(t, false, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	current := parent.VendorOrders[0]
	actor := fixture.actor(fixture.staffA, fixture.env.businessA)

	for _, step := range []orders.Transition{
		orders.TransitionAccept, orders.TransitionReady, orders.TransitionHandover,
	} {
		current, _, err = fixture.service.Advance(ctx, actor, current.ID, step, "", current.Version)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}

	// The goods have left the vendor. Withdrawing is no longer a cancellation.
	if _, _, err = fixture.service.CancelByCustomer(
		ctx, fixture.customerID, current.ID, "CHANGED_MIND", current.Version,
	); !errors.Is(err, orders.ErrOrderConflict) {
		t.Fatalf("a customer cancelled goods already in transit: %v", err)
	}
}

// The customer withdrawal window is re-checked inside the transaction, so a
// vendor handing the goods over concurrently cannot be overtaken by a
// cancellation that read an older status.
func TestSection17EOrders_CustomerCancelCannotOvertakeAConcurrentHandover(t *testing.T) {
	fixture := buildSection17EFixture(t, false, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	current := parent.VendorOrders[0]
	actor := fixture.actor(fixture.staffA, fixture.env.businessA)

	// The customer reads the order while it is still cancellable.
	stale := current

	for _, step := range []orders.Transition{
		orders.TransitionAccept, orders.TransitionReady, orders.TransitionHandover,
	} {
		current, _, err = fixture.service.Advance(ctx, actor, current.ID, step, "", current.Version)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}

	// The cancellation now arrives carrying the version it read. It must be
	// refused on the version guard, never applied to the handed-over order.
	if _, _, err = fixture.service.CancelByCustomer(
		ctx, fixture.customerID, stale.ID, "CHANGED_MIND", stale.Version,
	); err == nil {
		t.Fatal("a stale customer cancellation was applied after handover")
	}

	reread, err := fixture.service.GetForVendor(ctx, actor, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Status != orders.VendorStatusHandedOver {
		t.Fatalf("status=%s, want HANDED_OVER untouched", reread.Status)
	}
	if reread.RefundAllocatedMinor != 0 {
		t.Fatalf("a refund was prepared for a handed-over order: %d",
			reread.RefundAllocatedMinor)
	}
}

// -----------------------------------------------------------------------------
// Evidence trail
// -----------------------------------------------------------------------------

func TestSection17EOrders_LifecycleIsRecordedAsAppendOnlyEvidence(t *testing.T) {
	fixture := newSection17EFixture(t, false)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	vendorOrder := parent.VendorOrders[0]
	actor := fixture.actor(fixture.staffA, fixture.env.businessA)

	if _, _, err = fixture.service.Advance(
		ctx, actor, vendorOrder.ID, orders.TransitionAccept, "", vendorOrder.Version,
	); err != nil {
		t.Fatal(err)
	}

	rows, err := fixture.env.pool.Query(ctx, `
		SELECT action,actor_type,from_status,to_status
		  FROM commerce_order_events WHERE parent_order_id=$1
		 ORDER BY occurred_at,id`, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var action, actorType, from, to string
		if err = rows.Scan(&action, &actorType, &from, &to); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		orders.ActionOrderCreated,
		orders.ActionVendorOrderCreated,
		orders.ActionAccepted,
		orders.ActionOrderStateChanged,
	} {
		found := false
		for _, action := range actions {
			if action == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("lifecycle evidence is missing %s (have %v)", want, actions)
		}
	}
}

// Section 17E does not finish payment execution. No order it creates may carry
// a payment operation reference, and no settlement or refund may be executed.
func TestSection17EOrders_NoPaymentIsCreatedOrExecuted(t *testing.T) {
	fixture := buildSection17EFixture(t, true, true)
	ctx := context.Background()

	parent, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	if parent.PaymentOperationRef != "" {
		t.Fatalf("Section 17E fabricated a payment reference: %q", parent.PaymentOperationRef)
	}

	// Completing a vendor order prepares a settlement figure. It must not have
	// moved anything: the allocation register is unchanged by the transition.
	first := fixture.vendorOrderFor(t, parent, fixture.env.businessA)
	actor := fixture.actor(fixture.staffA, fixture.env.businessA)

	var before int
	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_allocations WHERE parent_order_id=$1`,
		parent.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}

	current := first
	for _, step := range []orders.Transition{
		orders.TransitionAccept, orders.TransitionReady, orders.TransitionHandover,
	} {
		current, _, err = fixture.service.Advance(ctx, actor, current.ID, step, "", current.Version)
		if err != nil {
			t.Fatal(err)
		}
	}

	var after int
	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_allocations WHERE parent_order_id=$1`,
		parent.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("a lifecycle transition rewrote the allocation register: %d -> %d",
			before, after)
	}

	var stillEmpty string
	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT payment_operation_ref FROM commerce_parent_orders WHERE id=$1`,
		parent.ID).Scan(&stillEmpty); err != nil {
		t.Fatal(err)
	}
	if stillEmpty != "" {
		t.Fatalf("payment reference appeared during the lifecycle: %q", stillEmpty)
	}
}
