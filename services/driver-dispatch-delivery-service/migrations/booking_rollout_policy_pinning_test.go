package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBookingRolloutPolicyPinningMigrationShape(t *testing.T) {
	raw, err := os.ReadFile("000054_booking_rollout_policy_pinning.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"rollout_protection_mode", "rollout_policy_version", "handover_required",
		"pickup_evidence_required", "delivery_evidence_required", "provider_transfer_required",
		"dispatch_booking_rollout_requirements_consistent", "dispatch_booking_rollout_policy_guard",
		"booking rollout policy snapshot is immutable", "UPDATE dispatch_bookings b SET",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("rollout pinning migration missing %q", required)
		}
	}
}

func TestBookingRolloutPolicyIsDatabaseImmutable(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	bookingID, customerID := uuid.NewString(), uuid.NewString()
	_, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(
		id,customer_id,status,pickup_address,pickup_lat,pickup_lng,
		dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,
		rollout_protection_mode,rollout_policy_version,handover_required,pickup_evidence_required,delivery_evidence_required)
		VALUES($1,$2,'searching','pickup',6.5,3.4,'dropoff',6.6,3.5,1000,'receiver','+2348000000000',
		'PROTECTED','dispatch_rollout_v1',true,true,true)`, bookingID, customerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_bookings SET handover_required=false WHERE id=$1`, bookingID); err == nil {
		t.Fatal("protected handover requirement was downgraded")
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_bookings SET status='accepted' WHERE id=$1`, bookingID); err != nil {
		t.Fatalf("ordinary Booking lifecycle update was blocked: %v", err)
	}
}

func TestBookingRolloutPolicyMigrationRollbackReapply(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	down, err := os.ReadFile("000054_booking_rollout_policy_pinning.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("000054_booking_rollout_policy_pinning.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback rollout pinning: %v", err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply rollout pinning: %v", err)
	}
}
