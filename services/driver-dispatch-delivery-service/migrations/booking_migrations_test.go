package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBookingMigrationFilesExist(t *testing.T) {
	for _, name := range []string{
		"000031_create_dispatch_bookings.up.sql", "000031_create_dispatch_bookings.down.sql",
		"000032_add_review_fields_to_ratings.up.sql", "000032_add_review_fields_to_ratings.down.sql",
		"000049_booking_serviceability_idempotency.up.sql", "000049_booking_serviceability_idempotency.down.sql",
	} {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("migration %s is missing: %v", name, err)
		}
	}
}

func TestBookingServiceabilityIdempotencyMigrationShape(t *testing.T) {
	raw, err := os.ReadFile("000049_booking_serviceability_idempotency.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"serviceability_quote_id", "dispatch_booking_serviceability_customer_fk",
		"REFERENCES dispatch_serviceability_quotes(id, customer_id)",
		"create_idempotency_key", "create_request_fingerprint",
		"dispatch_booking_create_idempotency", "customer_id, create_idempotency_key",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}

func TestBookingServiceabilityIdempotencyConstraint(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	customerID := uuid.NewString()
	quoteID := "quote_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_serviceability_quotes(
		id,customer_id,idempotency_key,request_fingerprint,
		pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,
		vehicle_class,decision,policy_version,package_declaration,package_admission,
		fare_estimate_kobo,currency,expires_at)
		VALUES($1,$2,'quote-key','quote-fingerprint',6.5,3.4,6.6,3.5,
		'MOTORCYCLE','SERVICEABLE','route-policy-v1','{}','{"allowed":true}',
		1000,'NGN',now()+interval '10 minutes')`, quoteID, customerID); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO dispatch_bookings(
		customer_id,status,pickup_address,pickup_lat,pickup_lng,
		dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,
		serviceability_quote_id,serviceability_quote_version,serviceability_quote_fingerprint,
		create_idempotency_key,create_request_fingerprint)
		VALUES($1,'searching','pickup',6.5,3.4,'dropoff',6.6,3.5,1000,'receiver','+2348000000000',
		$2,1,'quote-fingerprint','create-key',$3)`
	if _, err := pool.Exec(ctx, insert, customerID, quoteID, "request-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, insert, customerID, quoteID, "request-b"); err == nil {
		t.Fatal("expected customer-scoped create idempotency constraint to reject duplicate key")
	}
	otherCustomer := uuid.NewString()
	otherQuoteID := "quote_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_serviceability_quotes(
		id,customer_id,idempotency_key,request_fingerprint,
		pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,
		vehicle_class,decision,policy_version,package_declaration,package_admission,
		fare_estimate_kobo,currency,expires_at)
		VALUES($1,$2,'quote-key','quote-fingerprint',6.5,3.4,6.6,3.5,
		'MOTORCYCLE','SERVICEABLE','route-policy-v1','{}','{"allowed":true}',
		1000,'NGN',now()+interval '10 minutes')`, otherQuoteID, otherCustomer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, insert, otherCustomer, otherQuoteID, "request-c"); err != nil {
		t.Fatalf("same key in another customer scope must remain valid: %v", err)
	}
	if _, err := pool.Exec(ctx, insert, otherCustomer, quoteID, "request-d"); err == nil {
		t.Fatal("expected cross-customer serviceability quote binding to fail")
	}
}

func TestBookingServiceabilityIdempotencyMigrationRollbackReapply(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	down, err := os.ReadFile("000049_booking_serviceability_idempotency.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("000049_booking_serviceability_idempotency.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback booking serviceability migration: %v", err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply booking serviceability migration: %v", err)
	}
	var indexName string
	if err = pool.QueryRow(ctx, `SELECT 'dispatch_booking_create_idempotency'::regclass::text`).Scan(&indexName); err != nil {
		t.Fatalf("idempotency index missing after reapply: %v", err)
	}
}
