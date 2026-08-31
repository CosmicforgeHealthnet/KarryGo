// Package bookingearnings computes the addendum §19 earnings-insights
// report: gross trip earnings, online/active-trip time, utilisation, and
// average pickup ETA/trip duration, for one provider over one time window.
//
// This is a REPORT, not a ledger -- see Insights.IsLedger's own doc comment.
// The truck-provider app already learned the hard way that showing a
// booking-derived figure next to a real wallet balance without labelling
// which is which confuses drivers into thinking they disagree; this
// package's own shape exists so that mistake cannot repeat here.
//
// Acceptance/rejection rate (an addendum §19 field) is deliberately NOT
// computed: it requires the per-offer audit trail (taxi_match_candidates,
// migration 002) that phase B2's matcher was scoped to persist but never
// actually wired up -- a disclosed, pre-existing gap, not something this
// phase silently invents data for or silently expands scope to backfill.
package bookingearnings

import "time"

// Insights is the full report for one provider over [From, Until]. JSON
// tags matter: this struct is returned directly from GET
// /provider/earnings/insights -- found live serializing as bare Go field
// names (TripsCompleted, GrossEarningsKobo, ...) before these tags were
// added, the exact same class of bug as bookingreviews.Review's own
// (missing tags are not caught by any unit test that only inspects the Go
// struct in memory; only a live/marshaled response surfaces it).
type Insights struct {
	From  time.Time `json:"from"`
	Until time.Time `json:"until"`

	TripsCompleted           int   `json:"trips_completed"`
	GrossEarningsKobo        int64 `json:"gross_earnings_kobo"`
	CancelledByCustomerCount int   `json:"cancelled_by_customer_count"`
	CancelledByProviderCount int   `json:"cancelled_by_provider_count"`

	// AveragePickupEtaSeconds is nil when no booking in the window has both
	// MatchedAt and ArrivedAt (e.g. every booking was cancelled before the
	// driver arrived).
	AveragePickupEtaSeconds *float64 `json:"average_pickup_eta_seconds,omitempty"`
	// AverageTripDurationMinutes is nil when no booking in the window has
	// both StartedAt and CompletedAt.
	AverageTripDurationMinutes *float64 `json:"average_trip_duration_minutes,omitempty"`

	OnlineTimeSeconds     float64 `json:"online_time_seconds"`
	ActiveTripTimeSeconds float64 `json:"active_trip_time_seconds"`
	// UtilizationPercent and EarningsPerOnlineHourKobo are both nil when
	// OnlineTimeSeconds is 0 -- there is no meaningful rate to report for a
	// provider who was never online in this window, and 0/0 is not "0%".
	UtilizationPercent        *float64 `json:"utilization_percent,omitempty"`
	EarningsPerOnlineHourKobo *float64 `json:"earnings_per_online_hour_kobo,omitempty"`

	// IsLedger is ALWAYS false. Every field above is derived from booking
	// history and operational-state history, never from payment-wallet-
	// service's real ledger -- see the package doc comment.
	IsLedger bool `json:"is_ledger"`
}

// BookingFacts is the minimal slice of a taxi_bookings row this package
// needs. Narrower than bookingmodels.Booking on purpose: this package must
// not import the booking package (bookingusecases already imports THIS
// package; the reverse would cycle), and every field here already has an
// obvious, direct mapping the caller performs.
type BookingFacts struct {
	Status        string
	FareFinalKobo *int64
	CancelledBy   *string
	MatchedAt     *time.Time
	ArrivedAt     *time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
}

const (
	statusCompleted = "completed"
	statusCancelled = "cancelled"
	stateOffline    = "OFFLINE"
	stateOnTrip     = "ON_TRIP"
)

// StateChange is one provider_operational_state_history row's relevant
// fields -- named independently of availabilityrepositories.HistoryEntry
// (which has the identical shape) so this package stays free of that
// import; the caller converts.
type StateChange struct {
	ToState   string
	ChangedAt time.Time
}

// Compute is a pure aggregator: no DB, no clock read -- 'now' plays no part
// here, the caller resolves From/Until before calling in (matching the
// hauling service's own ComputeEarnings precedent for this exact "pure
// function over pre-fetched rows" shape). Fully unit-testable without
// Postgres.
//
// asOfState is the provider's operational state at exactly 'from' (the
// to_state of the most recent transition at or before it, or "" if none --
// treated as OFFLINE: a provider with no prior history has never gone
// online). changes must be sorted ascending by ChangedAt and contain only
// rows in (from, until] -- the exact contract of
// PostgresOperationalStateRepository.StateAsOf/ListHistory.
func Compute(bookings []BookingFacts, asOfState string, changes []StateChange, from, until time.Time) Insights {
	ins := Insights{From: from, Until: until, IsLedger: false}

	var pickupEtaTotalSeconds float64
	var pickupEtaCount int
	var tripDurationTotalMinutes float64
	var tripDurationCount int

	for _, b := range bookings {
		switch b.Status {
		case statusCompleted:
			ins.TripsCompleted++
			if b.FareFinalKobo != nil {
				ins.GrossEarningsKobo += *b.FareFinalKobo
			}
			if b.StartedAt != nil && b.CompletedAt != nil {
				tripDurationTotalMinutes += b.CompletedAt.Sub(*b.StartedAt).Minutes()
				tripDurationCount++
			}
		case statusCancelled:
			if b.CancelledBy != nil {
				switch *b.CancelledBy {
				case "customer":
					ins.CancelledByCustomerCount++
				case "provider":
					ins.CancelledByProviderCount++
				}
			}
		}
		// A ride cancelled after the driver arrived still reported a real
		// pickup ETA -- counted regardless of the booking's final status.
		if b.MatchedAt != nil && b.ArrivedAt != nil {
			pickupEtaTotalSeconds += b.ArrivedAt.Sub(*b.MatchedAt).Seconds()
			pickupEtaCount++
		}
	}
	if pickupEtaCount > 0 {
		avg := pickupEtaTotalSeconds / float64(pickupEtaCount)
		ins.AveragePickupEtaSeconds = &avg
	}
	if tripDurationCount > 0 {
		avg := tripDurationTotalMinutes / float64(tripDurationCount)
		ins.AverageTripDurationMinutes = &avg
	}

	ins.OnlineTimeSeconds, ins.ActiveTripTimeSeconds = walkTimeline(asOfState, changes, from, until)

	if ins.OnlineTimeSeconds > 0 {
		util := ins.ActiveTripTimeSeconds / ins.OnlineTimeSeconds * 100
		ins.UtilizationPercent = &util
		perHour := float64(ins.GrossEarningsKobo) / (ins.OnlineTimeSeconds / 3600)
		ins.EarningsPerOnlineHourKobo = &perHour
	}

	return ins
}

// walkTimeline reconstructs online/active-trip seconds from a piecewise
// state timeline: asOfState holds from 'from' until the first change (if
// any), each change's ToState holds until the next change (or 'until' for
// the last one). A gap-free walk, not a per-row lookup -- the history table
// only records the INSTANT a state changed, not how long it lasted.
func walkTimeline(asOfState string, changes []StateChange, from, until time.Time) (onlineSeconds, activeTripSeconds float64) {
	state := asOfState
	if state == "" {
		state = stateOffline
	}
	cursor := from

	accumulate := func(to time.Time) {
		if !to.After(cursor) {
			return
		}
		d := to.Sub(cursor).Seconds()
		if state != stateOffline {
			onlineSeconds += d
		}
		if state == stateOnTrip {
			activeTripSeconds += d
		}
	}

	for _, c := range changes {
		accumulate(c.ChangedAt)
		state = c.ToState
		cursor = c.ChangedAt
	}
	accumulate(until)

	return onlineSeconds, activeTripSeconds
}
