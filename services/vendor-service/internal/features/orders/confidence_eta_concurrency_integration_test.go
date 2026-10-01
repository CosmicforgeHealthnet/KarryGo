package orders

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

func confidenceETATestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres confidence ETA races")
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

func seedETAParent(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	id := "po_" + uuid.NewString()
	customerID := "customer_" + uuid.NewString()
	suffix := uuid.NewString()
	cartID := "cart_eta_" + suffix
	checkoutID := "checkout_eta_" + suffix
	planID := "plan_eta_" + suffix
	quoteID := "quote_eta_" + suffix
	fingerprint := strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
	now := time.Now().UTC()
	expiresAt := now.Add(time.Hour)
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
		  VALUES($1,$2,'FOOD','CONVERTED',1,$3,$4,$4)`, []any{cartID, customerID, expiresAt, now}},
		{`INSERT INTO commerce_checkout_sessions(
			id,customer_id,cart_id,cart_version,domain,status,validation_revision,address_ref,address_snapshotted_at,
			currency,quoted_items_total_minor,max_authorized_items_total_minor,reservation_ttl_seconds,expires_at,validated_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'FOOD','VALIDATED',1,'eta-fixture-address',$4,'NGN',0,0,900,$5,$4,$4,$4)`, []any{checkoutID, customerID, cartID, now, expiresAt}},
		{`INSERT INTO commerce_fulfilment_plans(
			id,checkout_session_id,customer_id,checkout_validation_revision,status,mode,plan_revision,currency,
			delivery_gross_minor,delivery_customer_minor,delivery_vendor_funded_minor,delivery_platform_funded_minor,
			expires_at,version,planned_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',0,0,0,0,$4,1,$5,$5,$5)`, []any{planID, checkoutID, customerID, expiresAt, now}},
		{`INSERT INTO commerce_quotes(
			id,customer_id,checkout_session_id,checkout_validation_revision,cart_id,cart_version,
			fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,quote_version,
			material_fingerprint,quote_fingerprint,currency,selected_fulfilment_mode,address_snapshot,expires_at,
			product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,
			discount_total_minor,subsidy_total_minor,final_payable_minor,quoted_total_minor,max_authorized_total_minor,
			funding_breakdown,price_disclosure_snapshot,created_at)
		  VALUES($1,$2,$3,1,$4,1,$5,1,1,1,$6,$6,'NGN','SINGLE','{}'::jsonb,$7,
			0,0,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,$8)`, []any{quoteID, customerID, checkoutID, cartID, planID, fingerprint, expiresAt, now}},
		{`INSERT INTO commerce_parent_orders(
			id,customer_id,quote_id,quote_version,quote_fingerprint,checkout_session_id,checkout_validation_revision,
			cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,domain,currency,
			selected_fulfilment_mode,address_snapshot,customer_approved_total_minor,max_authorized_total_minor,
			product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,
			subsidy_total_minor,funding_breakdown,price_disclosure_snapshot,state,vendor_order_count,created_at,updated_at)
		  VALUES($1,$2,$3,1,$4,$5,1,$6,1,$7,1,1,'FOOD','NGN','SINGLE','{}'::jsonb,
			0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,'CREATED',1,$8,$8)`, []any{id, customerID, quoteID, fingerprint, checkoutID, cartID, planID, now}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed ETA commerce evidence: %v", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit ETA commerce evidence: %v", err)
	}
	return id
}

func databaseETAInput(parent, event string, version int64) ETAInput {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return ETAInput{ParentOrderID: parent, MissionSlotRef: "mission_1", PlanVersion: 1,
		SourceEventID: event, SourceAuthority: "DISPATCH", SourceVersion: version,
		SourceOccurredAt: now, Trigger: TriggerMissionCreated, Domain: "FOOD",
		RouteSeconds: 900, RouteProvider: "route", RouteEvidenceVersion: "route_1",
		RouteObservedAt: now, PickupStopCount: 1}
}

func TestConfidenceETAVersionRaceIntegration(t *testing.T) {
	pool := confidenceETATestPool(t)
	parent := seedETAParent(t, pool)
	store := NewStore(pool)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for version := int64(1); version <= 2; version++ {
		wg.Add(1)
		go func(version int64) {
			defer wg.Done()
			<-start
			tx, err := pool.BeginTx(context.Background(), pgx.TxOptions{})
			if err == nil {
				_, _, err = store.applyConfidenceETATx(context.Background(), tx, databaseETAInput(parent, "event_"+string(rune('0'+version)), version))
			}
			if err == nil {
				err = tx.Commit(context.Background())
			} else if tx != nil {
				_ = tx.Rollback(context.Background())
			}
			errs <- err
		}(version)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var versions, distinct int
	if err := pool.QueryRow(context.Background(), `SELECT count(*),count(DISTINCT eta_version) FROM commerce_delivery_eta_versions WHERE parent_order_id=$1`, parent).Scan(&versions, &distinct); err != nil {
		t.Fatal(err)
	}
	if versions < 1 || versions > 2 || versions != distinct {
		t.Fatalf("history versions=%d distinct=%d", versions, distinct)
	}
}

func TestConfidenceETAStalePlanAndOldProviderIntegration(t *testing.T) {
	pool := confidenceETATestPool(t)
	parent := seedETAParent(t, pool)
	store := NewStore(pool)
	apply := func(in ETAInput) (DeliveryETA, bool) {
		t.Helper()
		tx, err := pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background()) //nolint:errcheck
		eta, created, err := store.applyConfidenceETATx(context.Background(), tx, in)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
		return eta, created
	}
	current := databaseETAInput(parent, "event_plan_2", 2)
	current.PlanVersion, current.ProviderRef, current.ProviderAssigned = 2, "provider_1", true
	if _, created := apply(current); !created {
		t.Fatal("initial current ETA not created")
	}
	stale := databaseETAInput(parent, "event_plan_1", 3)
	stale.PlanVersion = 1
	if eta, created := apply(stale); created || eta.PlanVersion != 2 {
		t.Fatalf("stale plan moved current pointer: created=%t eta=%+v", created, eta)
	}
	oldProvider := databaseETAInput(parent, "event_old_provider", 4)
	oldProvider.PlanVersion, oldProvider.ProviderRef, oldProvider.ProviderAssigned = 2, "provider_2", true
	oldProvider.Trigger = TriggerTimingUpdated
	if eta, created := apply(oldProvider); created || eta.ETAVersion != 1 {
		t.Fatalf("old provider update was accepted: created=%t eta=%+v", created, eta)
	}
}
