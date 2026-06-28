package bookingmodels

import (
	"sort"
	"time"
)

// Earnings is a read-only projection over the provider's own haulage bookings.
//
// This is trip-earnings *reporting* (how much the provider has earned from
// completed/in-progress trips), not a financial ledger. The authoritative wallet
// balance, ledger entries, and withdrawals live in payment-wallet-service. This
// projection only reads fare data the hauling service already owns on
// haulage_bookings, so it introduces no cross-service coupling.

// Transaction kinds.
const (
	TxnKindCredit     = "credit"     // money earned from a trip
	TxnKindWithdrawal = "withdrawal" // money withdrawn (tracked by payment-wallet-service)
)

// Transaction display statuses.
const (
	TxnStatusCompleted = "completed"
	TxnStatusPending   = "pending"
	TxnStatusFailed    = "failed"
)

// EarningsTransaction is a single row in the provider's recent-transactions list.
type EarningsTransaction struct {
	ID         string    `json:"id"`
	BookingID  string    `json:"booking_id,omitempty"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Subtitle   string    `json:"subtitle"`
	AmountKobo int64     `json:"amount_kobo"`
	Status     string    `json:"status"`
	IsTrip     bool      `json:"is_trip"`
	OccurredAt time.Time `json:"occurred_at"`
}

// ProviderEarnings is the payload backing the provider Earnings/Wallet screen.
type ProviderEarnings struct {
	// AvailableBalanceKobo is settled earnings from completed trips.
	AvailableBalanceKobo int64 `json:"available_balance_kobo"`
	// PendingBalanceKobo is earnings from in-progress trips (accepted through
	// delivered) that have not been settled to the provider yet.
	PendingBalanceKobo int64 `json:"pending_balance_kobo"`
	// TodayEarningsKobo is the sum of trips completed today.
	TodayEarningsKobo int64 `json:"today_earnings_kobo"`
	// TripsCompletedToday counts trips completed today.
	TripsCompletedToday int `json:"trips_completed_today"`
	// HoursOnline is not tracked historically yet; reported as 0 for now.
	HoursOnline int `json:"hours_online"`

	// TotalEarningsKobo is the lifetime sum of completed trips (backs the
	// Earning Summary screen's "Total Earnings").
	TotalEarningsKobo int64 `json:"total_earnings_kobo"`
	// SummaryYear is the year the monthly series covers.
	SummaryYear int `json:"summary_year"`
	// MonthlyEarningsKobo holds Jan..Dec (length 12) completed-trip earnings for
	// SummaryYear, backing the Earning Summary chart.
	MonthlyEarningsKobo []int64 `json:"monthly_earnings_kobo"`

	Transactions []EarningsTransaction `json:"transactions"`
}

// in-progress statuses where the provider has committed to the trip and the fare
// counts as pending (earned, not yet settled).
var earnedInProgressStatuses = map[string]bool{
	StatusAccepted:        true,
	StatusEnRoutePickup:   true,
	StatusArrivedAtPickup: true,
	StatusPickedUp:        true,
	StatusEnRouteDelivery: true,
	StatusDelivered:       true,
}

// bookingFareKobo resolves the fare to count for a booking: the final fare when
// set, otherwise the estimate.
func bookingFareKobo(b Booking) int64 {
	if b.FareFinalKobo != nil {
		return *b.FareFinalKobo
	}
	if b.FareEstimateKobo != nil {
		return *b.FareEstimateKobo
	}
	return 0
}

// ComputeEarnings aggregates a provider's bookings into the earnings projection.
// Pure: takes the bookings and the reference time so it can be unit-tested.
//
// Only bookings the provider actually worked count: completed trips contribute to
// the available balance, in-progress trips to the pending balance. Bookings that
// never reached the provider (pending_match, awaiting_acceptance) and dead ones
// (cancelled, unmatched) are ignored.
func ComputeEarnings(bookings []Booking, now time.Time) ProviderEarnings {
	out := ProviderEarnings{
		SummaryYear:         now.Year(),
		MonthlyEarningsKobo: make([]int64, 12),
		Transactions:        []EarningsTransaction{},
	}

	for _, b := range bookings {
		fare := bookingFareKobo(b)

		switch {
		case b.Status == StatusCompleted:
			out.AvailableBalanceKobo += fare
			out.TotalEarningsKobo += fare
			occurred := firstTime(b.CompletedAt, b.DeliveredAt, &b.UpdatedAt, &b.CreatedAt)
			if sameDay(occurred, now) {
				out.TodayEarningsKobo += fare
				out.TripsCompletedToday++
			}
			if occurred.Year() == now.Year() {
				out.MonthlyEarningsKobo[int(occurred.Month())-1] += fare
			}
			out.Transactions = append(out.Transactions, EarningsTransaction{
				ID:         b.ID,
				BookingID:  b.ID,
				Kind:       TxnKindCredit,
				Title:      transactionTitle(b),
				Subtitle:   transactionSubtitle(b),
				AmountKobo: fare,
				Status:     TxnStatusCompleted,
				IsTrip:     true,
				OccurredAt: occurred,
			})

		case earnedInProgressStatuses[b.Status]:
			out.PendingBalanceKobo += fare
			occurred := firstTime(b.AcceptedAt, &b.CreatedAt)
			out.Transactions = append(out.Transactions, EarningsTransaction{
				ID:         b.ID,
				BookingID:  b.ID,
				Kind:       TxnKindCredit,
				Title:      transactionTitle(b),
				Subtitle:   transactionSubtitle(b),
				AmountKobo: fare,
				Status:     TxnStatusPending,
				IsTrip:     true,
				OccurredAt: occurred,
			})
		}
	}

	// Most recent first.
	sort.SliceStable(out.Transactions, func(i, j int) bool {
		return out.Transactions[i].OccurredAt.After(out.Transactions[j].OccurredAt)
	})

	return out
}

func transactionTitle(b Booking) string {
	if name := b.ReceiverName; name != "" {
		return name
	}
	return "Haulage Trip"
}

func transactionSubtitle(b Booking) string {
	if b.DropoffAddress != "" {
		return b.DropoffAddress
	}
	return b.PickupAddress
}

// firstTime returns the first non-nil, non-zero time from the candidates.
func firstTime(candidates ...*time.Time) time.Time {
	for _, t := range candidates {
		if t != nil && !t.IsZero() {
			return *t
		}
	}
	return time.Time{}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
