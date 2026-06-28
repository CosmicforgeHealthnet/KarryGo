package bookingmodels

import (
	"testing"
	"time"
)

func kobo(v int64) *int64 { return &v }

func tPtr(t time.Time) *time.Time { return &t }

func TestComputeEarnings_BalancesAndToday(t *testing.T) {
	now := time.Date(2026, 6, 22, 15, 0, 0, 0, time.UTC)
	earlierToday := now.Add(-3 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	bookings := []Booking{
		// Completed today → available + today.
		{
			ID: "b1", Status: StatusCompleted, ReceiverName: "Anthonia Dipson",
			DropoffAddress: "23rd avenue Ikeja", FareFinalKobo: kobo(2_400_000),
			CompletedAt: tPtr(earlierToday), CreatedAt: earlierToday,
		},
		// Completed yesterday → available only (not today).
		{
			ID: "b2", Status: StatusCompleted, FareFinalKobo: kobo(1_000_000),
			CompletedAt: tPtr(yesterday), CreatedAt: yesterday,
		},
		// In progress → pending only, falls back to estimate fare.
		{
			ID: "b3", Status: StatusEnRouteDelivery, FareEstimateKobo: kobo(500_000),
			AcceptedAt: tPtr(earlierToday), CreatedAt: earlierToday,
		},
		// Awaiting acceptance → ignored entirely (never worked).
		{
			ID: "b4", Status: StatusAwaitingAcceptance, FareEstimateKobo: kobo(999_999),
			CreatedAt: now,
		},
		// Cancelled → ignored.
		{
			ID: "b5", Status: StatusCancelled, FareFinalKobo: kobo(777_777),
			CreatedAt: yesterday,
		},
	}

	got := ComputeEarnings(bookings, now)

	if got.AvailableBalanceKobo != 3_400_000 {
		t.Errorf("available balance = %d, want 3400000", got.AvailableBalanceKobo)
	}
	if got.PendingBalanceKobo != 500_000 {
		t.Errorf("pending balance = %d, want 500000", got.PendingBalanceKobo)
	}
	if got.TodayEarningsKobo != 2_400_000 {
		t.Errorf("today earnings = %d, want 2400000", got.TodayEarningsKobo)
	}
	if got.TripsCompletedToday != 1 {
		t.Errorf("trips completed today = %d, want 1", got.TripsCompletedToday)
	}
	// Total earnings = lifetime completed (b1 + b2); in-progress excluded.
	if got.TotalEarningsKobo != 3_400_000 {
		t.Errorf("total earnings = %d, want 3400000", got.TotalEarningsKobo)
	}
	if got.SummaryYear != 2026 {
		t.Errorf("summary year = %d, want 2026", got.SummaryYear)
	}
	if len(got.MonthlyEarningsKobo) != 12 {
		t.Fatalf("monthly series length = %d, want 12", len(got.MonthlyEarningsKobo))
	}
	// b1 (today) + b2 (yesterday) both fall in June 2026 → index 5.
	if got.MonthlyEarningsKobo[5] != 3_400_000 {
		t.Errorf("June earnings = %d, want 3400000", got.MonthlyEarningsKobo[5])
	}
	if got.MonthlyEarningsKobo[0] != 0 {
		t.Errorf("January earnings = %d, want 0", got.MonthlyEarningsKobo[0])
	}
	// Only b1, b2, b3 produce transactions.
	if len(got.Transactions) != 3 {
		t.Fatalf("transactions = %d, want 3", len(got.Transactions))
	}
}

func TestComputeEarnings_TransactionFields(t *testing.T) {
	now := time.Date(2026, 6, 22, 15, 0, 0, 0, time.UTC)
	bookings := []Booking{
		{
			ID: "b1", Status: StatusCompleted, ReceiverName: "Anthonia Dipson",
			DropoffAddress: "23rd avenue Ikeja, Lagos", FareFinalKobo: kobo(2_400_000),
			CompletedAt: tPtr(now), CreatedAt: now,
		},
		// No receiver name + no dropoff → title/subtitle fall back.
		{
			ID: "b2", Status: StatusAccepted, PickupAddress: "Apapa Wharf",
			FareEstimateKobo: kobo(800_000), AcceptedAt: tPtr(now.Add(-time.Hour)), CreatedAt: now.Add(-time.Hour),
		},
	}

	got := ComputeEarnings(bookings, now)

	// Sorted most-recent first → b1 (now) before b2 (now-1h).
	first := got.Transactions[0]
	if first.BookingID != "b1" || first.Title != "Anthonia Dipson" {
		t.Errorf("first txn = %+v", first)
	}
	if first.Status != TxnStatusCompleted || first.Kind != TxnKindCredit || !first.IsTrip {
		t.Errorf("first txn status/kind = %+v", first)
	}
	if first.Subtitle != "23rd avenue Ikeja, Lagos" {
		t.Errorf("first subtitle = %q", first.Subtitle)
	}

	second := got.Transactions[1]
	if second.Title != "Haulage Trip" {
		t.Errorf("fallback title = %q, want 'Haulage Trip'", second.Title)
	}
	if second.Subtitle != "Apapa Wharf" {
		t.Errorf("fallback subtitle = %q, want 'Apapa Wharf'", second.Subtitle)
	}
	if second.Status != TxnStatusPending {
		t.Errorf("in-progress status = %q, want pending", second.Status)
	}
}

func TestComputeEarnings_Empty(t *testing.T) {
	got := ComputeEarnings(nil, time.Now())
	if got.AvailableBalanceKobo != 0 || got.PendingBalanceKobo != 0 {
		t.Errorf("empty earnings should be zero, got %+v", got)
	}
	if got.Transactions == nil {
		t.Error("transactions should be a non-nil empty slice for clean JSON")
	}
}
