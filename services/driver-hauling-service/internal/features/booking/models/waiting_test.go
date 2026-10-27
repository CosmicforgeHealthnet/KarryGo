package bookingmodels

import (
	"testing"
	"time"
)

func TestChargeableBlocks(t *testing.T) {
	p := WaitingPolicy{FreeMinutes: 60, BlockMinutes: 30, BlockFeeKobo: 200000}
	cases := []struct {
		waited time.Duration
		blocks int64
	}{
		{0, 0},
		{59 * time.Minute, 0},
		{60 * time.Minute, 0},             // exactly the free time is free
		{60*time.Minute + time.Second, 1}, // a started block counts in full
		{90 * time.Minute, 1},
		{90*time.Minute + time.Second, 2},
		{3 * time.Hour, 4},
	}
	for _, c := range cases {
		if got := p.ChargeableBlocks(c.waited); got != c.blocks {
			t.Errorf("waited %s: %d blocks, want %d", c.waited, got, c.blocks)
		}
	}
}

func TestWaitingViewIsLiveAndFinalFeeIgnoresOpenStops(t *testing.T) {
	arrived := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	left := arrived.Add(75 * time.Minute)
	dropoff := arrived.Add(3 * time.Hour)
	w := BookingWaiting{
		BookingID:        "b1",
		Policy:           WaitingPolicy{FreeMinutes: 60, BlockMinutes: 30, BlockFeeKobo: 200000},
		PickupArrivedAt:  &arrived,
		PickupLeftAt:     &left,
		DropoffArrivedAt: &dropoff,
	}

	// Pickup: 75 min = 1 block. Drop-off still running 100 min in = 2 blocks.
	view := w.View(dropoff.Add(100 * time.Minute))
	if view.Stops[0].FeeKobo != 200000 || view.Stops[0].Running {
		t.Fatalf("pickup %+v", view.Stops[0])
	}
	if view.Stops[1].FeeKobo != 400000 || !view.Stops[1].Running || view.TotalFeeKobo != 600000 {
		t.Fatalf("dropoff %+v total %d", view.Stops[1], view.TotalFeeKobo)
	}
	if view.Stops[1].FreeUntil == nil || !view.Stops[1].FreeUntil.Equal(dropoff.Add(time.Hour)) {
		t.Fatalf("free until %v", view.Stops[1].FreeUntil)
	}
	// The open drop-off stop is not charged in the final fee.
	if fee := w.FinalFeeKobo(); fee != 200000 {
		t.Fatalf("final fee %d, want only the closed pickup stop's 200000", fee)
	}
	closed := dropoff.Add(100 * time.Minute)
	w.DropoffLeftAt = &closed
	if fee := w.FinalFeeKobo(); fee != 600000 {
		t.Fatalf("final fee %d, want 600000", fee)
	}
}

func TestWaitingPolicyValidation(t *testing.T) {
	if err := (WaitingPolicy{FreeMinutes: 60, BlockMinutes: 30, BlockFeeKobo: 200000}).Validate(); err != nil {
		t.Fatalf("valid policy refused: %v", err)
	}
	if err := (WaitingPolicy{FreeMinutes: -1, BlockMinutes: 0, BlockFeeKobo: -5}).Validate(); err == nil {
		t.Fatal("an invalid policy was accepted")
	}
}
