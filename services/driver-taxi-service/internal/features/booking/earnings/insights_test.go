package bookingearnings

import (
	"testing"
	"time"
)

func ts(minute int) time.Time {
	return time.Date(2026, 1, 1, 0, minute, 0, 0, time.UTC)
}

func TestCompute_IsLedgerAlwaysFalse(t *testing.T) {
	got := Compute(nil, "", nil, ts(0), ts(60))
	if got.IsLedger {
		t.Error("IsLedger = true, want unconditionally false -- this is a report, not a ledger")
	}
}

func TestCompute_CountsCompletedTripsAndGrossEarnings(t *testing.T) {
	fareA, fareB := int64(150000), int64(200000)
	bookings := []BookingFacts{
		{Status: statusCompleted, FareFinalKobo: &fareA},
		{Status: statusCompleted, FareFinalKobo: &fareB},
		{Status: statusCancelled},
	}
	got := Compute(bookings, "", nil, ts(0), ts(60))
	if got.TripsCompleted != 2 {
		t.Errorf("TripsCompleted = %d, want 2", got.TripsCompleted)
	}
	if got.GrossEarningsKobo != 350000 {
		t.Errorf("GrossEarningsKobo = %d, want 350000", got.GrossEarningsKobo)
	}
}

func TestCompute_SplitsCancellationsByWho(t *testing.T) {
	customer, provider := "customer", "provider"
	bookings := []BookingFacts{
		{Status: statusCancelled, CancelledBy: &customer},
		{Status: statusCancelled, CancelledBy: &customer},
		{Status: statusCancelled, CancelledBy: &provider},
	}
	got := Compute(bookings, "", nil, ts(0), ts(60))
	if got.CancelledByCustomerCount != 2 || got.CancelledByProviderCount != 1 {
		t.Errorf("customer=%d provider=%d, want 2/1", got.CancelledByCustomerCount, got.CancelledByProviderCount)
	}
}

func TestCompute_AveragePickupEtaNilWithNoData(t *testing.T) {
	got := Compute([]BookingFacts{{Status: statusCancelled}}, "", nil, ts(0), ts(60))
	if got.AveragePickupEtaSeconds != nil {
		t.Errorf("AveragePickupEtaSeconds = %v, want nil with no matched+arrived pair", got.AveragePickupEtaSeconds)
	}
}

func TestCompute_AveragePickupEtaFromMatchedToArrived(t *testing.T) {
	matched1, arrived1 := ts(0), ts(5)   // 5 min = 300s
	matched2, arrived2 := ts(10), ts(13) // 3 min = 180s
	bookings := []BookingFacts{
		{Status: statusCompleted, MatchedAt: &matched1, ArrivedAt: &arrived1},
		{Status: statusCancelled, MatchedAt: &matched2, ArrivedAt: &arrived2}, // counts even though cancelled -- a real ETA was reported
	}
	got := Compute(bookings, "", nil, ts(0), ts(60))
	if got.AveragePickupEtaSeconds == nil {
		t.Fatal("AveragePickupEtaSeconds is nil, want a value")
	}
	want := (300.0 + 180.0) / 2
	if *got.AveragePickupEtaSeconds != want {
		t.Errorf("AveragePickupEtaSeconds = %v, want %v", *got.AveragePickupEtaSeconds, want)
	}
}

func TestCompute_AverageTripDurationOnlyFromCompleted(t *testing.T) {
	started, completed := ts(0), ts(20) // 20 min
	bookings := []BookingFacts{
		{Status: statusCompleted, StartedAt: &started, CompletedAt: &completed},
	}
	got := Compute(bookings, "", nil, ts(0), ts(60))
	if got.AverageTripDurationMinutes == nil || *got.AverageTripDurationMinutes != 20 {
		t.Errorf("AverageTripDurationMinutes = %v, want 20", got.AverageTripDurationMinutes)
	}
}

func TestCompute_UtilizationAndPerHourNilWhenNeverOnline(t *testing.T) {
	got := Compute(nil, "OFFLINE", nil, ts(0), ts(60))
	if got.UtilizationPercent != nil {
		t.Errorf("UtilizationPercent = %v, want nil when OnlineTimeSeconds is 0", got.UtilizationPercent)
	}
	if got.EarningsPerOnlineHourKobo != nil {
		t.Errorf("EarningsPerOnlineHourKobo = %v, want nil when OnlineTimeSeconds is 0", got.EarningsPerOnlineHourKobo)
	}
}

func TestCompute_TimelineAsOfStateHoldsUntilFirstChange(t *testing.T) {
	// Provider was already AVAILABLE when the window opened at ts(0); no
	// transitions happen until ts(30), when they go OFFLINE.
	changes := []StateChange{{ToState: "OFFLINE", ChangedAt: ts(30)}}
	got := Compute(nil, "AVAILABLE", changes, ts(0), ts(60))
	if got.OnlineTimeSeconds != 30*60 {
		t.Errorf("OnlineTimeSeconds = %v, want %v (30 minutes AVAILABLE before going OFFLINE)", got.OnlineTimeSeconds, 30*60)
	}
}

func TestCompute_TimelineActiveTripOnlyDuringOnTrip(t *testing.T) {
	// AVAILABLE from window start, ON_TRIP for 15 min (ts10..ts25), then
	// AVAILABLE again for the rest of the window (until ts60).
	changes := []StateChange{
		{ToState: "ON_TRIP", ChangedAt: ts(10)},
		{ToState: "AVAILABLE", ChangedAt: ts(25)},
	}
	got := Compute(nil, "AVAILABLE", changes, ts(0), ts(60))
	if got.OnlineTimeSeconds != 60*60 {
		t.Errorf("OnlineTimeSeconds = %v, want %v (never went offline in this window)", got.OnlineTimeSeconds, 60*60)
	}
	if got.ActiveTripTimeSeconds != 15*60 {
		t.Errorf("ActiveTripTimeSeconds = %v, want %v (ts10 to ts25)", got.ActiveTripTimeSeconds, 15*60)
	}
}

func TestCompute_NoHistoryBeforeWindowTreatedAsOffline(t *testing.T) {
	// asOfState = "" (StateAsOf found nothing before 'from') -- must NOT be
	// treated as "online the whole time", it means the provider has no
	// history yet, i.e. offline.
	got := Compute(nil, "", nil, ts(0), ts(60))
	if got.OnlineTimeSeconds != 0 {
		t.Errorf("OnlineTimeSeconds = %v, want 0 with no prior history", got.OnlineTimeSeconds)
	}
}

func TestCompute_UtilizationAndPerHourComputedWhenOnline(t *testing.T) {
	fare := int64(500000) // 5000 NGN
	changes := []StateChange{
		{ToState: "ON_TRIP", ChangedAt: ts(0)},
		{ToState: "OFFLINE", ChangedAt: ts(30)}, // 30 min online, all of it ON_TRIP
	}
	got := Compute([]BookingFacts{{Status: statusCompleted, FareFinalKobo: &fare}}, "AVAILABLE", changes, ts(0), ts(60))
	if got.UtilizationPercent == nil || *got.UtilizationPercent != 100 {
		t.Errorf("UtilizationPercent = %v, want 100 (online time was entirely ON_TRIP)", got.UtilizationPercent)
	}
	if got.EarningsPerOnlineHourKobo == nil {
		t.Fatal("EarningsPerOnlineHourKobo is nil, want a value")
	}
	// 30 minutes online = 0.5h; 500000 kobo / 0.5h = 1,000,000 kobo/h.
	want := 1000000.0
	if *got.EarningsPerOnlineHourKobo != want {
		t.Errorf("EarningsPerOnlineHourKobo = %v, want %v", *got.EarningsPerOnlineHourKobo, want)
	}
}

func TestCompute_ChangesAfterUntilAreIgnoredByCaller(t *testing.T) {
	// This test documents the CONTRACT (changes must already be filtered to
	// (from, until]), not a defensive check inside Compute -- passing a
	// change past 'until' produces a nonsensical negative final segment,
	// which is a caller bug, not something this pure function guards
	// against (see the repository's own ListHistory doc comment for the
	// contract it upholds).
	changes := []StateChange{{ToState: "OFFLINE", ChangedAt: ts(0)}}
	got := Compute(nil, "AVAILABLE", changes, ts(0), ts(60))
	if got.OnlineTimeSeconds != 0 {
		t.Errorf("OnlineTimeSeconds = %v, want 0 (transitioned to OFFLINE at the exact window start)", got.OnlineTimeSeconds)
	}
}
