package privacy

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

func privacyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(context.Background(), pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func seedTerminalOrder(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()
	parentID, customerID := "privacy_po_"+suffix, "privacy_customer_"+suffix
	cartID, checkoutID, planID, quoteID := "privacy_cart_"+suffix, "privacy_checkout_"+suffix, "privacy_plan_"+suffix, "privacy_quote_"+suffix
	fingerprint := strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
	now, expires := time.Now().UTC(), time.Now().UTC().Add(time.Hour)
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
			id,customer_id,cart_id,cart_version,domain,status,validation_revision,address_ref,address_snapshotted_at,
			currency,quoted_items_total_minor,max_authorized_items_total_minor,reservation_ttl_seconds,expires_at,validated_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'FOOD','VALIDATED',1,'address-ref',$4,'NGN',0,0,900,$5,$4,$4,$4)`, []any{checkoutID, customerID, cartID, now, expires}},
		{`INSERT INTO commerce_fulfilment_plans(
			id,checkout_session_id,customer_id,checkout_validation_revision,status,mode,plan_revision,currency,
			delivery_gross_minor,delivery_customer_minor,delivery_vendor_funded_minor,delivery_platform_funded_minor,
			expires_at,version,planned_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',0,0,0,0,$4,1,$5,$5,$5)`, []any{planID, checkoutID, customerID, expires, now}},
		{`INSERT INTO commerce_quotes(
			id,customer_id,checkout_session_id,checkout_validation_revision,cart_id,cart_version,
			fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,quote_version,
			material_fingerprint,quote_fingerprint,currency,selected_fulfilment_mode,address_snapshot,expires_at,
			product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,
			discount_total_minor,subsidy_total_minor,final_payable_minor,quoted_total_minor,max_authorized_total_minor,
			funding_breakdown,price_disclosure_snapshot,created_at)
		  VALUES($1,$2,$3,1,$4,1,$5,1,1,1,$6,$6,'NGN','SINGLE','{}'::jsonb,$7,
			0,0,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,$8)`, []any{quoteID, customerID, checkoutID, cartID, planID, fingerprint, expires, now}},
		{`INSERT INTO commerce_parent_orders(
			id,customer_id,quote_id,quote_version,quote_fingerprint,checkout_session_id,checkout_validation_revision,
			cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,domain,currency,
			selected_fulfilment_mode,address_snapshot,has_receiver,receiver_snapshot,
			customer_approved_total_minor,max_authorized_total_minor,product_subtotal_minor,delivery_subtotal_minor,
			fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,subsidy_total_minor,funding_breakdown,
			price_disclosure_snapshot,state,vendor_order_count,created_at,updated_at,terminal_at)
		  VALUES($1,$2,$3,1,$4,$5,1,$6,1,$7,1,1,'FOOD','NGN','SINGLE',
			'{"address":"1 Secret Street","latitude":6.5,"longitude":3.4,"snapshotted_at":"2026-01-01T00:00:00Z"}'::jsonb,
			true,'{"receiver_name":"Private Person","receiver_phone":"+2348012345678"}'::jsonb,
			0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,'COMPLETED',1,$8,$8,$8)`, []any{parentID, customerID, quoteID, fingerprint, checkoutID, cartID, planID, now}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed privacy order: %v", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return parentID, customerID
}

func TestRetentionWorkerHonorsLegalHoldThenMinimizesOperationalPII(t *testing.T) {
	pool := privacyPool(t)
	ctx := context.Background()
	parentID, customerID := seedTerminalOrder(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE vendor_privacy_retention_jobs
		SET next_attempt_at=now()+interval '1 day'
		WHERE parent_order_id<>$1 AND status='PENDING'`, parentID); err != nil {
		t.Fatal(err)
	}
	holdID := "hold_" + uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO vendor_data_legal_holds
		(id,scope_type,scope_ref,reason_code,created_by_ref) VALUES($1,'CUSTOMER',$2,'DISPUTE','support')`, holdID, customerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vendor_privacy_retention_jobs SET next_attempt_at=now()-interval '1 second' WHERE parent_order_id=$1`, parentID); err != nil {
		t.Fatal(err)
	}
	worker := NewRetentionWorker(pool, 3)
	if count, err := worker.RunPass(ctx, 1); err != nil || count != 1 {
		t.Fatalf("held pass count=%d err=%v", count, err)
	}
	var outcome string
	if err := pool.QueryRow(ctx, `SELECT last_outcome FROM vendor_privacy_retention_jobs WHERE parent_order_id=$1`, parentID).Scan(&outcome); err != nil || outcome != "LEGAL_HOLD" {
		t.Fatalf("hold outcome=%q err=%v", outcome, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vendor_data_legal_holds SET released_at=now() WHERE id=$1`, holdID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vendor_privacy_retention_jobs SET next_attempt_at=now()-interval '1 second' WHERE parent_order_id=$1`, parentID); err != nil {
		t.Fatal(err)
	}
	if count, err := worker.RunPass(ctx, 1); err != nil || count != 1 {
		t.Fatalf("minimize pass count=%d err=%v", count, err)
	}
	var operationalAddress, operationalReceiver, evidenceAddress, evidenceReceiver, status string
	if err := pool.QueryRow(ctx, `SELECT p.operational_address_snapshot::text,p.operational_receiver_snapshot::text,
		e.address_snapshot::text,e.receiver_snapshot::text,j.status
		FROM commerce_parent_orders p
		JOIN commerce_order_restricted_evidence e ON e.parent_order_id=p.id
		JOIN vendor_privacy_retention_jobs j ON j.parent_order_id=p.id WHERE p.id=$1`, parentID).
		Scan(&operationalAddress, &operationalReceiver, &evidenceAddress, &evidenceReceiver, &status); err != nil {
		t.Fatal(err)
	}
	if status != "MINIMIZED" || !strings.Contains(operationalAddress, "[REDACTED]") || operationalReceiver != "{}" {
		t.Fatalf("status=%s operational address=%s receiver=%s", status, operationalAddress, operationalReceiver)
	}
	if !strings.Contains(evidenceAddress, "1 Secret Street") || !strings.Contains(evidenceReceiver, "+2348012345678") {
		t.Fatalf("restricted evidence was not preserved: %s %s", evidenceAddress, evidenceReceiver)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vendor_audit_log
		WHERE aggregate_id=$1 AND action='privacy.order_receiver_location_minimized'`, parentID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit count=%d err=%v", auditCount, err)
	}
}
