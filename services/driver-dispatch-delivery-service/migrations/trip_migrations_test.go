package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTripMigrationFilesExist(t *testing.T) {
	for _, name := range []string{
		"000021_create_trips.up.sql", "000021_create_trips.down.sql",
		"000022_create_trip_state_log.up.sql", "000022_create_trip_state_log.down.sql",
		"000023_create_delivery_proofs.up.sql", "000023_create_delivery_proofs.down.sql",
		"000024_create_cancellations.up.sql", "000024_create_cancellations.down.sql",
	} {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("migration %s is missing: %v", name, err)
		}
	}
}

func TestTripMigrationsApplyValidateAndRollback(t *testing.T) {
	databaseURL := os.Getenv("DISPATCH_TRIP_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_TRIP_TEST_DATABASE_URL or TEST_DATABASE_URL to run trip migration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()

	schema := "trip_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") //nolint:errcheck
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema+", public"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"000004_create_providers.up.sql", "000021_create_trips.up.sql",
		"000022_create_trip_state_log.up.sql", "000023_create_delivery_proofs.up.sql",
		"000024_create_cancellations.up.sql",
	} {
		execMigrationFile(t, ctx, conn, name)
	}
	for _, index := range []string{
		"idx_trips_booking", "idx_trips_provider", "idx_trips_status", "idx_trips_provider_active",
		"idx_trips_created_at", "idx_trips_provider_created_at", "idx_state_log_trip",
		"idx_state_log_trip_changed_at", "idx_proof_trip", "idx_cancel_trip", "idx_cancelled_by",
		"idx_cancellations_cancelled_at",
	} {
		assertIndexExists(t, ctx, conn, schema, index)
	}

	providerID, customerID, bookingID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := conn.Exec(ctx, `INSERT INTO providers (id,phone) VALUES ($1,$2)`, providerID, "+2348000072001"); err != nil {
		t.Fatal(err)
	}
	var tripID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO trips (
			booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone
		) VALUES ($1,$2,$3,'Pickup',6.5,3.4,'Dropoff',6.6,3.5,150000,'Receiver','+2348011223344')
		RETURNING id::text
	`, bookingID, providerID, customerID).Scan(&tripID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO trips (
			booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone
		) VALUES ($1,$2,$3,'Pickup',6.5,3.4,'Dropoff',6.6,3.5,150000,'Receiver','+2348011223344')
	`, bookingID, providerID, customerID); err == nil {
		t.Fatal("duplicate trip booking_id accepted")
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO trips (
			booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone
		) VALUES ($1,$2,$3,'invalid','Pickup',6.5,3.4,'Dropoff',6.6,3.5,150000,'Receiver','+2348011223344')
	`, uuid.NewString(), providerID, customerID); err == nil {
		t.Fatal("invalid trip status accepted")
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO delivery_proofs (trip_id,photo_ref,signature_ref,receiver_name,receiver_phone)
		VALUES ($1,'local-private://photo','local-private://signature','Receiver','+2348011223344')
	`, tripID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO delivery_proofs (trip_id,photo_ref,signature_ref,receiver_name,receiver_phone)
		VALUES ($1,'local-private://photo','local-private://signature','Receiver','+2348011223344')
	`, tripID); err == nil {
		t.Fatal("duplicate delivery proof accepted")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO cancellations (trip_id,cancelled_by,reason_code) VALUES ($1,'invalid','other')`, tripID); err == nil {
		t.Fatal("invalid cancelled_by accepted")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO cancellations (trip_id,cancelled_by,reason_code) VALUES ($1,'provider','other')`, tripID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO cancellations (trip_id,cancelled_by,reason_code) VALUES ($1,'provider','other')`, tripID); err == nil {
		t.Fatal("duplicate cancellation accepted")
	}

	for _, name := range []string{
		"000024_create_cancellations.down.sql", "000023_create_delivery_proofs.down.sql",
		"000022_create_trip_state_log.down.sql", "000021_create_trips.down.sql",
	} {
		execMigrationFile(t, ctx, conn, name)
	}
	for _, table := range []string{"cancellations", "delivery_proofs", "trip_state_log", "trips"} {
		assertTableDropped(t, ctx, conn, schema, table)
	}
}
