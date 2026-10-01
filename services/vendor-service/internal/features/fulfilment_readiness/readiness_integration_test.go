package fulfilment_readiness

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

// Readiness against a real PostgreSQL.
//
// Two of Section 11's guarantees are enforced by the database and cannot be
// observed any other way: first_actual_ready_at is write-once, and the version
// guard decides which of two staff members editing the same order wins.

func readinessTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres readiness tests")
	}
	if !strings.Contains(strings.ToLower(dsn), "test") {
		t.Fatal("refusing a database whose name does not identify it as a test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

// seedOrderGraph creates the minimum commerce evidence a readiness row needs.
//
// None of these columns matter to Section 11: readiness reacts to orders, it
// does not create them. They are here because the existing schema demands them,
// and a fixture that skipped them would be testing a shape the database does
// not actually hold.
func seedOrderGraph(t *testing.T, pool *pgxpool.Pool) (vendorOrderID, parentOrderID string) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()
	customerID := "cust_" + suffix
	cartID := "cart_" + suffix
	checkoutID := "checkout_" + suffix
	planID := "plan_" + suffix
	quoteID := "quote_" + suffix
	parentOrderID = "po_" + suffix
	vendorOrderID = "vo_" + suffix
	fingerprint := strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
	now := time.Now().UTC()
	expires := now.Add(time.Hour)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO commerce_carts(id,customer_id,domain,status,version,expires_at,created_at,updated_at)
		  VALUES($1,$2,'FOOD','CONVERTED',1,$3,$4,$4)`, []any{cartID, customerID, expires, now}},
		{`INSERT INTO commerce_checkout_sessions(
			id,customer_id,cart_id,cart_version,domain,status,validation_revision,address_ref,
			address_snapshotted_at,currency,quoted_items_total_minor,max_authorized_items_total_minor,
			reservation_ttl_seconds,expires_at,validated_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'FOOD','VALIDATED',1,'readiness-fixture',$4,'NGN',0,0,900,$5,$4,$4,$4)`,
			[]any{checkoutID, customerID, cartID, now, expires}},
		{`INSERT INTO commerce_fulfilment_plans(
			id,checkout_session_id,customer_id,checkout_validation_revision,status,mode,plan_revision,
			currency,delivery_gross_minor,delivery_customer_minor,delivery_vendor_funded_minor,
			delivery_platform_funded_minor,expires_at,version,planned_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',0,0,0,0,$4,1,$5,$5,$5)`,
			[]any{planID, checkoutID, customerID, expires, now}},
		{`INSERT INTO commerce_quotes(
			id,customer_id,checkout_session_id,checkout_validation_revision,cart_id,cart_version,
			fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,quote_version,
			material_fingerprint,quote_fingerprint,currency,selected_fulfilment_mode,address_snapshot,
			expires_at,product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,
			fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,subsidy_total_minor,
			final_payable_minor,quoted_total_minor,max_authorized_total_minor,funding_breakdown,
			price_disclosure_snapshot,created_at)
		  VALUES($1,$2,$3,1,$4,1,$5,1,1,1,$6,$6,'NGN','SINGLE','{}'::jsonb,$7,
			0,0,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,$8)`,
			[]any{quoteID, customerID, checkoutID, cartID, planID, fingerprint, expires, now}},
		{`INSERT INTO commerce_parent_orders(
			id,customer_id,quote_id,quote_version,quote_fingerprint,checkout_session_id,
			checkout_validation_revision,cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,
			fulfilment_plan_version,domain,currency,selected_fulfilment_mode,address_snapshot,
			customer_approved_total_minor,max_authorized_total_minor,product_subtotal_minor,
			delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,
			subsidy_total_minor,funding_breakdown,price_disclosure_snapshot,state,vendor_order_count,
			created_at,updated_at)
		  VALUES($1,$2,$3,1,$4,$5,1,$6,1,$7,1,1,'FOOD','NGN','SINGLE','{}'::jsonb,
			0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,'CREATED',1,$8,$8)`,
			[]any{parentOrderID, customerID, quoteID, fingerprint, checkoutID, cartID, planID, now}},
		{`INSERT INTO commerce_vendor_orders(
			id,parent_order_id,customer_id,business_id,branch_id,domain,currency,quote_group_ref,
			product_subtotal_minor,max_product_subtotal_minor,delivery_responsibility,
			delivery_fee_beneficiary,customer_payable_minor,vendor_payable_minor,fulfilment_mode,
			effective_delivery_responsibility,effective_delivery_fee_beneficiary,
			effective_logistics_mission_ref,reservation_required,inventory_state)
		  VALUES($1,$2,$3,'biz-1','br-1','FOOD','NGN',$4,0,0,'PLATFORM_LOGISTICS','PLATFORM_PROVIDER',
			0,0,'SINGLE','PLATFORM_LOGISTICS','PLATFORM_PROVIDER','',false,'NOT_REQUIRED')`,
			[]any{vendorOrderID, parentOrderID, customerID, "qg_" + suffix}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed order graph: %v", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return vendorOrderID, parentOrderID
}

func newReadinessService(pool *pgxpool.Pool, now time.Time) *Service {
	return NewService(Options{
		Repository: NewRepository(pool), Enabled: true,
		Now: func() time.Time { return now },
	})
}

func staffActor() Actor {
	return Actor{Kind: SourceVendorStaff, Ref: "staff-1",
		BusinessID: "biz-1", PermittedBranches: []string{"br-1"}}
}

func startOrder(t *testing.T, service *Service, vendorOrderID, parentOrderID string) Readiness {
	t.Helper()
	started, err := service.Start(context.Background(), StartRequest{
		VendorOrderID: vendorOrderID, ParentOrderID: parentOrderID,
		BusinessID: "biz-1", BranchID: "br-1",
		VendorType: VendorRestaurant, Actor: staffActor(),
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return started
}

// TestFirstActualReadyAtIsWriteOnce is mandatory test 11.
//
// A vendor who declares READY and then corrects it has still declared it. A
// dispute about a rider who arrived to nothing depends on that record surviving
// the correction, which is why the database refuses to change it rather than
// leaving it to whichever statement runs next.
func TestFirstActualReadyAtIsWriteOnce(t *testing.T) {
	pool := readinessTestPool(t)
	now := time.Now().UTC()
	service := newReadinessService(pool, now)
	vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
	ctx := context.Background()

	preparing := startOrder(t, service, vendorOrderID, parentOrderID)

	ready, err := service.Advance(ctx, AdvanceRequest{
		VendorOrderID: vendorOrderID, ExpectedVersion: preparing.Version,
		Move: Move{To: StateReady}, Actor: staffActor(),
	})
	if err != nil {
		t.Fatalf("declare ready: %v", err)
	}
	if ready.FirstActualReadyAt == nil {
		t.Fatal("declaring READY recorded no first-ready evidence")
	}
	firstReady := *ready.FirstActualReadyAt

	// The correction: ten more minutes after all.
	corrected, err := service.Advance(ctx, AdvanceRequest{
		VendorOrderID: vendorOrderID, ExpectedVersion: ready.Version,
		Move:  Move{To: StateDelayed, ReasonCode: "ITEM_MISSING"},
		Actor: staffActor(),
	})
	if err != nil {
		t.Fatalf("correct a ready order: %v", err)
	}
	if corrected.State != StateDelayed {
		t.Fatalf("state = %s, want DELAYED", corrected.State)
	}
	if corrected.FirstActualReadyAt == nil || !corrected.FirstActualReadyAt.Equal(firstReady) {
		t.Fatalf("first-ready evidence changed on correction: %v -> %v",
			firstReady, corrected.FirstActualReadyAt)
	}

	// And the database itself refuses, so a statement written later cannot
	// erase it either.
	if _, err = pool.Exec(ctx,
		`UPDATE commerce_vendor_order_readiness SET first_actual_ready_at=now() + interval '1 hour'
		  WHERE vendor_order_id=$1`, vendorOrderID); err == nil {
		t.Fatal("the database allowed first_actual_ready_at to be rewritten")
	}

	// The history records both the declaration and the correction.
	history, err := service.History(ctx, vendorOrderID)
	if err != nil {
		t.Fatal(err)
	}
	sawReady, sawCorrection := false, false
	for _, entry := range history {
		if entry.ToState == StateReady {
			sawReady = true
		}
		if entry.ToState == StateDelayed && entry.ReasonCode == "ITEM_MISSING" {
			sawCorrection = true
		}
	}
	if !sawReady || !sawCorrection {
		t.Fatalf("history is missing the declaration or the correction: %+v", history)
	}
}

// TestBranchAndTenantIsolation is mandatory test 12.
//
// The member of another business's staff has no standing here, however senior
// they are at their own, and staff scoped to one branch have none at another.
func TestBranchAndTenantIsolation(t *testing.T) {
	pool := readinessTestPool(t)
	now := time.Now().UTC()
	service := newReadinessService(pool, now)
	vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
	ctx := context.Background()

	current := startOrder(t, service, vendorOrderID, parentOrderID)

	strangers := map[string]Actor{
		"another business": {Kind: SourceVendorStaff, Ref: "rival",
			BusinessID: "biz-2", PermittedBranches: []string{"br-1"}},
		"another branch": {Kind: SourceVendorStaff, Ref: "other-branch",
			BusinessID: "biz-1", PermittedBranches: []string{"br-9"}},
		"a customer": {Kind: Source("CUSTOMER"), Ref: "cust-1", BusinessID: "biz-1"},
		"a provider": {Kind: Source("PROVIDER"), Ref: "rider-1", BusinessID: "biz-1"},
	}
	for name, actor := range strangers {
		_, err := service.Advance(ctx, AdvanceRequest{
			VendorOrderID: vendorOrderID, ExpectedVersion: current.Version,
			Move: Move{To: StateReady}, Actor: actor,
		})
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("%s moved readiness: %v", name, err)
		}
	}

	// The order has not moved.
	after, err := service.Read(ctx, vendorOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != current.State || after.Version != current.Version {
		t.Fatalf("readiness moved despite every attempt being refused: %s v%d",
			after.State, after.Version)
	}
}

// TestStaleVersionLosesToAConcurrentEdit is the concurrency guard.
//
// Two staff members with the same order open on two screens is the normal case,
// not an edge one. The second save must be told the ground moved rather than
// silently overwriting a decision it never saw.
func TestStaleVersionLosesToAConcurrentEdit(t *testing.T) {
	pool := readinessTestPool(t)
	now := time.Now().UTC()
	service := newReadinessService(pool, now)
	vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
	ctx := context.Background()

	current := startOrder(t, service, vendorOrderID, parentOrderID)
	staleVersion := current.Version

	if _, err := service.Advance(ctx, AdvanceRequest{
		VendorOrderID: vendorOrderID, ExpectedVersion: staleVersion,
		Move: Move{To: StateReadySoon}, Actor: staffActor(),
	}); err != nil {
		t.Fatalf("first edit: %v", err)
	}

	_, err := service.Advance(ctx, AdvanceRequest{
		VendorOrderID: vendorOrderID, ExpectedVersion: staleVersion,
		Move:  Move{To: StateBlocked, ReasonCode: "EQUIPMENT_FAILURE"},
		Actor: staffActor(),
	})
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("the stale edit = %v, want ErrVersion", err)
	}
}

// TestSystemTransitionsRecordTheirOrigin is mandatory test 15 end to end.
//
// An automatic state change must be traceable to the Vendor-owned production
// event that caused it, or nobody can audit why an order moved.
func TestSystemTransitionsRecordTheirOrigin(t *testing.T) {
	pool := readinessTestPool(t)
	now := time.Now().UTC()
	service := newReadinessService(pool, now)
	vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
	ctx := context.Background()

	current := startOrder(t, service, vendorOrderID, parentOrderID)

	system := Actor{Kind: SourceSystem, Ref: "production-adapter",
		BusinessID: "biz-1", OriginEventRef: "prod_evt_42"}
	updated, err := service.Advance(ctx, AdvanceRequest{
		VendorOrderID: vendorOrderID, ExpectedVersion: current.Version,
		Move: Move{To: StateReadySoon, Substate: SubPacking}, Actor: system,
	})
	if err != nil {
		t.Fatalf("system transition: %v", err)
	}
	if updated.Source != SourceSystem {
		t.Fatalf("source = %s, want SYSTEM", updated.Source)
	}

	history, err := service.History(ctx, vendorOrderID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range history {
		if entry.Source == SourceSystem && entry.OriginEventRef == "prod_evt_42" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no history entry names the origin event: %+v", history)
	}
}

// TestPredictionIsRecordedAndMovesWithTheOrder proves the predictor's output
// actually reaches the row the planner reads.
func TestPredictionIsRecordedAndMovesWithTheOrder(t *testing.T) {
	pool := readinessTestPool(t)
	now := time.Now().UTC()
	service := newReadinessService(pool, now)
	vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
	ctx := context.Background()

	preparing := startOrder(t, service, vendorOrderID, parentOrderID)
	if preparing.PredictedReadyStart == nil || preparing.PredictedReadyEnd == nil {
		t.Fatal("starting preparation recorded no predicted window")
	}
	if !preparing.PredictedReadyEnd.After(*preparing.PredictedReadyStart) {
		t.Fatal("the predicted window is not a window")
	}
	if preparing.ConfidenceBand == "" {
		t.Fatal("no confidence band was recorded")
	}

	// A vendor estimate sharpens it and is recorded alongside.
	estimate := now.Add(12 * time.Minute)
	updated, err := service.Advance(ctx, AdvanceRequest{
		VendorOrderID: vendorOrderID, ExpectedVersion: preparing.Version,
		Move:  Move{To: StatePreparing, Substate: SubCooking, EstimatedAt: &estimate},
		Actor: staffActor(),
	})
	if err != nil {
		t.Fatalf("supply an estimate: %v", err)
	}
	if updated.VendorEstimatedReadyAt == nil {
		t.Fatal("the vendor estimate was not recorded")
	}
	if updated.ConfidenceBand != ConfidenceHigh {
		t.Fatalf("confidence = %s, want HIGH once the vendor has given an estimate",
			updated.ConfidenceBand)
	}
}
