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

func TestRequestMigrationFilesExist(t *testing.T) {
	for _, name := range []string{
		"000019_create_request_broadcasts.up.sql", "000019_create_request_broadcasts.down.sql",
		"000020_create_provider_request_inbox.up.sql", "000020_create_provider_request_inbox.down.sql",
	} {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("migration %s is missing: %v", name, err)
		}
	}
}

func TestRequestMigrationsApplyValidateAndRollback(t *testing.T) {
	databaseURL := os.Getenv("DISPATCH_REQUEST_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_REQUEST_TEST_DATABASE_URL or TEST_DATABASE_URL to run request migration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	schema := "request_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") //nolint:errcheck
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema+", public"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000004_create_providers.up.sql", "000019_create_request_broadcasts.up.sql", "000020_create_provider_request_inbox.up.sql"} {
		execMigrationFile(t, ctx, conn, name)
	}
	for _, idx := range []string{"idx_broadcasts_booking", "idx_broadcasts_status", "idx_broadcasts_expires", "idx_broadcasts_accepted_provider",
		"idx_inbox_provider_booking", "idx_inbox_broadcast", "idx_inbox_provider", "idx_inbox_status", "idx_inbox_provider_status_received", "idx_inbox_booking"} {
		assertIndexExists(t, ctx, conn, schema, idx)
	}
	providerID := uuid.NewString()
	if _, err := conn.Exec(ctx, `INSERT INTO providers (id,phone) VALUES ($1,$2)`, providerID, "+2348000060001"); err != nil {
		t.Fatal(err)
	}
	bookingID := uuid.NewString()
	var broadcastID string
	if err := conn.QueryRow(ctx, `INSERT INTO request_broadcasts (booking_id,broadcast_radius_km,expires_at,booking_payload) VALUES ($1,5,now()+interval '30 seconds','{}') RETURNING id::text`, bookingID).Scan(&broadcastID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO request_broadcasts (booking_id,broadcast_radius_km,expires_at,booking_payload) VALUES ($1,5,now(),'{}')`, bookingID); err == nil {
		t.Fatal("duplicate booking_id accepted")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO request_broadcasts (booking_id,broadcast_radius_km,expires_at,booking_payload,status) VALUES ($1,5,now(),'{}','bad')`, uuid.NewString()); err == nil {
		t.Fatal("invalid broadcast status accepted")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO request_broadcasts (booking_id,broadcast_radius_km,expires_at,booking_payload) VALUES ($1,5,now(),NULL)`, uuid.NewString()); err == nil {
		t.Fatal("NULL booking_payload accepted")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO provider_request_inbox (broadcast_id,booking_id,provider_id) VALUES ($1,$2,$3)`, broadcastID, bookingID, providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO provider_request_inbox (broadcast_id,booking_id,provider_id) VALUES ($1,$2,$3)`, broadcastID, bookingID, providerID); err == nil {
		t.Fatal("duplicate provider booking inbox accepted")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO provider_request_inbox (broadcast_id,booking_id,provider_id,status) VALUES ($1,$2,$3,'bad')`, broadcastID, uuid.NewString(), providerID); err == nil {
		t.Fatal("invalid inbox status accepted")
	}
	execMigrationFile(t, ctx, conn, "000020_create_provider_request_inbox.down.sql")
	execMigrationFile(t, ctx, conn, "000019_create_request_broadcasts.down.sql")
	assertTableDropped(t, ctx, conn, schema, "provider_request_inbox")
	assertTableDropped(t, ctx, conn, schema, "request_broadcasts")
}
