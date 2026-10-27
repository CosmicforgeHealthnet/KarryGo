package bookingrepo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
)

func TestWaitingRepositoryIntegration(t *testing.T) {
	_, pool := secureHandoverTestRepository(t)
	bookingID, _, _ := seedSecureHandoverBooking(t, pool)
	repo := NewWaitingRepository(pool)
	ctx := context.Background()

	policy := bookingmodels.WaitingPolicy{
		ID: uuid.NewString(), FreeMinutes: 60, BlockMinutes: 30, BlockFeeKobo: 200000,
		CreatedBy: "ops@karryam.com", Reason: "integration test", CreatedAt: time.Now().UTC(),
	}
	if err := repo.InsertPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE hauling_waiting_policies SET block_fee_kobo = 1 WHERE id = $1`, policy.ID); err == nil {
		t.Fatal("a waiting-rate row was edited; rates must be append-only")
	}
	latest, ok, err := repo.LatestPolicy(ctx)
	if err != nil || !ok || latest.ID != policy.ID {
		t.Fatalf("LatestPolicy = %+v %v %v", latest, ok, err)
	}

	if _, err := repo.Get(ctx, bookingID); err != ErrNoWaiting {
		t.Fatalf("want ErrNoWaiting before arrival, got %v", err)
	}
	// Leaving a stop never reached records nothing.
	if err := repo.MarkLeft(ctx, bookingID, bookingmodels.WaitingStopPickup, time.Now()); err != nil {
		t.Fatal(err)
	}

	arrived := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	if err := repo.MarkArrived(ctx, bookingID, bookingmodels.WaitingStopPickup, arrived, policy); err != nil {
		t.Fatal(err)
	}
	// A second tap keeps the first arrival, and later rates do not replace the snapshot.
	later := bookingmodels.WaitingPolicy{FreeMinutes: 0, BlockMinutes: 1, BlockFeeKobo: 999}
	if err := repo.MarkArrived(ctx, bookingID, bookingmodels.WaitingStopPickup, arrived.Add(time.Hour), later); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkLeft(ctx, bookingID, bookingmodels.WaitingStopPickup, arrived.Add(95*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkLeft(ctx, bookingID, bookingmodels.WaitingStopPickup, arrived.Add(5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	w, err := repo.Get(ctx, bookingID)
	if err != nil {
		t.Fatal(err)
	}
	if !w.PickupArrivedAt.Equal(arrived) || !w.PickupLeftAt.Equal(arrived.Add(95*time.Minute)) {
		t.Fatalf("times changed: %v %v", w.PickupArrivedAt, w.PickupLeftAt)
	}
	if w.Policy.BlockFeeKobo != 200000 || w.Policy.ID != policy.ID {
		t.Fatalf("rate snapshot replaced: %+v", w.Policy)
	}
	if fee := w.FinalFeeKobo(); fee != 400000 {
		t.Fatalf("fee %d, want 400000", fee)
	}

	changed, err := repo.SettleFee(ctx, bookingID, 400000, bookingmodels.WaitingFeeOwed, "")
	if err != nil || !changed {
		t.Fatalf("SettleFee: %v %v", changed, err)
	}
	changed, err = repo.SettleFee(ctx, bookingID, 400000, bookingmodels.WaitingFeePaid, "ref")
	if err != nil || changed {
		t.Fatalf("a settled fee moved again: %v %v", changed, err)
	}
	if w, _ = repo.Get(ctx, bookingID); w.FeeStatus != bookingmodels.WaitingFeeOwed {
		t.Fatalf("status %s", w.FeeStatus)
	}
}
