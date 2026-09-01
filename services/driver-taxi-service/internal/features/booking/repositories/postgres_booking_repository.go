package bookingrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/dbtx"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Executor is an alias for dbtx.Executor, not a new type -- so a single
// pgx.Tx opened via BeginTx below can be passed to a transition on this
// repository AND to provider_availability's
// OperationalStateRepository.Transition in the same call, with both
// commiting or rolling back together (the addendum's requirement that a
// provider operational-state change caused by a booking transition commits
// in the SAME database transaction, never as a follow-up write). See
// internal/dbtx's own doc comment for why this must be a type alias, not
// two independently-declared interfaces.
type Executor = dbtx.Executor

// bookingColumns must stay in step with scanBooking. One constant so adding a
// column cannot leave a query out of step with the scanner -- the same
// discipline already established for taxi_providers in provider_auth/
// provider_profile.
const bookingColumns = `
	id::text, customer_id::text, provider_id::text, vehicle_id::text,
	market_id::text, operating_zone_id::text,
	booking_mode, vehicle_class, passenger_count, ride_notes, reserved_duration_minutes,
	fare_estimate_kobo, fare_final_kobo, pricing_quote_reference, pricing_version,
	distance_km, duration_minutes, actual_distance_km,
	payment_method, payment_status, payment_intent_id,
	status, pickup_verification_status,
	ride_purpose, receiver_name, receiver_phone,
	package_desc, package_type, package_size, is_fragile,
	delivery_verification_status,
	scheduled_at, dispatch_start_at, reservation_status, reserved_provider_id::text,
	backup_dispatch_at, provider_confirmed_at, customer_ready_window, matching_started_at,
	cancel_reason, cancelled_by,
	matched_at, accepted_at, arrived_at, started_at, completed_at, cancelled_at,
	created_at, updated_at`

const stopColumns = `id::text, booking_id::text, sequence, type, address, lat, lng,
	status, arrived_at, departed_at, wait_seconds, notes`

// BookingRepository is the persistence boundary for the ride aggregate.
type BookingRepository interface {
	// CreateBooking inserts the booking row and every stop row in one
	// transaction: a booking with no stops, or stops with no booking, must
	// never be observable by any other reader.
	CreateBooking(ctx context.Context, booking bookingmodels.Booking, stops []bookingmodels.Stop) (bookingmodels.Booking, error)
	GetByID(ctx context.Context, id string) (bookingmodels.Booking, error)
	GetByIDForCustomer(ctx context.Context, id, customerID string) (bookingmodels.Booking, error)
	GetByIDForProvider(ctx context.Context, id, providerID string) (bookingmodels.Booking, error)
	ListByCustomer(ctx context.Context, customerID string, limit int) ([]bookingmodels.Booking, error)
	// ListActiveByProvider is the poll target for a driver app with no push
	// channel wired yet: their current offer and/or committed ride.
	ListActiveByProvider(ctx context.Context, providerID string) ([]bookingmodels.Booking, error)
	// ListTerminalByProviderInRange backs earnings insights (phase B8):
	// every completed or cancelled booking ever assigned to this provider,
	// created within [from, until]. Bookings that never reached a provider
	// (pending_match/unmatched) are irrelevant to one provider's own report.
	ListTerminalByProviderInRange(ctx context.Context, providerID string, from, until time.Time) ([]bookingmodels.Booking, error)
	// HasActiveBooking backs the addendum §5.2 customer active-ride guard.
	HasActiveBooking(ctx context.Context, customerID string) (bool, error)

	// ListDueScheduled backs ScheduledDispatchWorker (phase B9): every
	// booking still 'scheduled' whose dispatch window has opened
	// (dispatch_start_at <= now), oldest first so a backlog after downtime
	// drains in booking order rather than arbitrarily.
	ListDueScheduled(ctx context.Context, now time.Time) ([]string, error)
	// PromoteScheduledToPendingMatch: scheduled -> pending_match, guarded on
	// status so a booking already promoted by a concurrent sweep (this
	// instance or another) is refused, not double-promoted. Also sets
	// matching_started_at to matchingStartedAt -- see that field's own doc
	// comment on why it must NOT be created_at for a ride booked well ahead
	// of its promotion.
	PromoteScheduledToPendingMatch(ctx context.Context, bookingID string, matchingStartedAt time.Time) (bookingmodels.Booking, error)

	// ListActive backs the phase B10 ops "live operations" view: every
	// non-terminal, non-scheduled booking, optionally scoped to one market.
	ListActive(ctx context.Context, marketID string) ([]bookingmodels.Booking, error)
	// ListUnmatched backs the ops "unmatched" triage view: every
	// status='unmatched' booking, optionally scoped to market/zone and to
	// only those created at or before olderThan (a zero olderThan means no
	// age filter).
	ListUnmatched(ctx context.Context, marketID, zoneID string, olderThan time.Time) ([]bookingmodels.Booking, error)
	// CountByStatus backs the phase B10 /metrics endpoint: one row per
	// status value actually present, bounded cardinality by construction
	// (Status has a fixed, small set of values) -- never grouped by
	// booking_id or any other high-cardinality column, per the addendum's
	// explicit metrics-cardinality warning.
	CountByStatus(ctx context.Context) (map[string]int64, error)

	// ListStaleMatching backs the phase B10 match-recovery worker: every
	// booking still pending_match/awaiting_acceptance whose updated_at is at
	// or before cutoff -- old enough that a genuinely still-running matcher
	// goroutine would have moved it on by now, meaning it more likely got
	// orphaned by a crash/restart mid-search.
	ListStaleMatching(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error)
	// ListStaleInProgress backs the auto-complete recovery worker: every
	// in_progress booking whose started_at is at or before cutoff -- a trip
	// that should have completed by now but never got a driver-confirmed
	// complete call.
	ListStaleInProgress(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error)

	// AdminCancel force-cancels a booking from ANY non-terminal status,
	// including in_progress -- the one cancellation path in this service
	// that reaches an active trip, reserved for an operator, never a
	// customer/driver action. cancelled_by is always "admin".
	AdminCancel(ctx context.Context, exec Executor, bookingID, reason string) (bookingmodels.Booking, error)

	// RecordEvent appends one audit row (phase B10 route replay). exec may
	// be nil (uses the repository's own pool) or a shared pgx.Tx -- called
	// from inside withTransition's own transaction so a booking's status,
	// its provider's operational state, its outbox event, and its audit
	// trail all commit or roll back together.
	RecordEvent(ctx context.Context, exec Executor, event bookingmodels.BookingEvent) error
	// ListEvents returns a booking's full audit trail, oldest first.
	ListEvents(ctx context.Context, bookingID string) ([]bookingmodels.BookingEvent, error)

	// BeginTx opens a transaction so a caller can run a booking transition
	// atomically with another repository's write in this same database --
	// see provider_availability's OperationalStateRepository.Transition,
	// whose executor parameter exists for exactly this purpose.
	BeginTx(ctx context.Context) (pgx.Tx, error)

	// MarkMatched: pending_match -> awaiting_acceptance. Guarded on status
	// only (no provider_id guard -- none is set yet at this point).
	// MarkMatched also persists paymentIntentID in the same statement (phase
	// B4) -- nil for cash bookings, the wallet payment intent's own ID for
	// wallet bookings (created fresh per offer, outside this transaction,
	// since it is an external HTTP call -- see bookingusecases' matching.go).
	MarkMatched(ctx context.Context, exec Executor, bookingID, providerID string, paymentIntentID *string) (bookingmodels.Booking, error)
	// MarkAccepted: awaiting_acceptance -> accepted. Guarded on BOTH status
	// AND provider_id: a driver accepting a booking they were never offered
	// (or that has since been re-offered to someone else) is refused.
	MarkAccepted(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error)
	// ResetToMatching is the single decision point on an offer timeout or an
	// explicit reject: awaiting_acceptance -> pending_match, clearing
	// provider_id/matched_at. Guarded on BOTH status AND provider_id --
	// zero rows affected means an accept already landed in the race window
	// (or the booking was cancelled), and the caller must stop matching
	// rather than re-dispatch a ride that is no longer pending.
	ResetToMatching(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error)
	// MarkUnmatched: pending_match -> unmatched. Guarded on status only, so a
	// match that lands concurrently with the search window expiring is never
	// clobbered.
	MarkUnmatched(ctx context.Context, exec Executor, bookingID, reason string) (bookingmodels.Booking, error)

	// Every transition below is a guarded compare-and-set: WHERE id=... AND
	// provider_id=... AND status=<expected prior status>. Zero rows affected
	// -- wrong provider, wrong status, or a since-superseded booking -- is
	// apperrors.Conflict, never a silent success and never a 500. exec may be
	// nil (defaults to the repository's own pool) or a pgx.Tx opened via
	// BeginTx.
	MarkEnRoutePickup(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error)
	// MarkArrivedAtPickup also flips pickup_verification_status to 'pending'
	// in the same statement (phase B5) -- see its own doc comment.
	MarkArrivedAtPickup(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error)
	// MarkPickupVerified: pickup_verification_status 'pending' -> 'verified'.
	// Guarded on provider_id AND status=arrived_at_pickup, so only the
	// assigned driver can verify, and a booking that has moved on (e.g.
	// cancelled) cannot be verified after the fact.
	MarkPickupVerified(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error)
	// MarkDeliveryVerified: delivery_verification_status 'pending' ->
	// 'verified', the parcel mirror of MarkPickupVerified. Guarded on
	// provider_id AND status=in_progress, so only the assigned driver can
	// verify and only while the package is actually in the car. Until this
	// succeeds, MarkCompleted's own guard refuses the trip.
	MarkDeliveryVerified(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error)
	// MarkInProgress is the "start trip" transition. Phase B5: gated on
	// pickup_verification_status = 'verified' in the guard itself -- see
	// its own doc comment.
	MarkInProgress(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error)
	MarkCompleted(ctx context.Context, exec Executor, bookingID, providerID string, actualDistanceKm float64, fareFinalKobo int64) (bookingmodels.Booking, error)
	CancelByCustomer(ctx context.Context, exec Executor, bookingID, customerID, reason string) (bookingmodels.Booking, error)
	CancelByProvider(ctx context.Context, exec Executor, bookingID, providerID, reason string) (bookingmodels.Booking, error)

	// SetPaymentStatus is a plain denormalized-field update (phase B4), not a
	// guarded state machine of its own: the REAL sequencing (unpaid -> held ->
	// paid, or -> refunded) is enforced by the wallet call that must succeed
	// before the caller ever invokes this, not by a DB guard here.
	SetPaymentStatus(ctx context.Context, exec Executor, bookingID, status string) error
}

type PostgresBookingRepository struct {
	db *pgxpool.Pool
}

func NewPostgresBookingRepository(db *pgxpool.Pool) *PostgresBookingRepository {
	return &PostgresBookingRepository{db: db}
}

func (r *PostgresBookingRepository) CreateBooking(ctx context.Context, booking bookingmodels.Booking, stops []bookingmodels.Stop) (bookingmodels.Booking, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once committed

	id := uuid.NewString()
	row := tx.QueryRow(ctx, `
		INSERT INTO taxi_bookings (
			id, customer_id, market_id, operating_zone_id,
			booking_mode, vehicle_class, passenger_count, ride_notes, reserved_duration_minutes,
			fare_estimate_kobo, pricing_quote_reference, pricing_version, distance_km, duration_minutes,
			payment_method, payment_status, status,
			scheduled_at, dispatch_start_at, matching_started_at,
			ride_purpose, receiver_name, receiver_phone,
			package_desc, package_type, package_size, is_fragile
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14,
			$15, $16, $17,
			$18, $19, $20,
			$21, $22, $23,
			$24, $25, $26, $27
		)
		RETURNING `+bookingColumns,
		id, booking.CustomerID, booking.MarketID, booking.OperatingZoneID,
		string(booking.BookingMode), booking.VehicleClass, booking.PassengerCount, booking.RideNotes, booking.ReservedDurationMinutes,
		booking.FareEstimateKobo, booking.PricingQuoteReference, booking.PricingVersion, booking.DistanceKm, booking.DurationMinutes,
		booking.PaymentMethod, booking.PaymentStatus, string(booking.Status),
		booking.ScheduledAt, booking.DispatchStartAt, booking.MatchingStartedAt,
		string(booking.RidePurpose), booking.ReceiverName, booking.ReceiverPhone,
		booking.PackageDesc, booking.PackageType, booking.PackageSize, booking.IsFragile,
	)
	created, err := scanBooking(row)
	if err != nil {
		return bookingmodels.Booking{}, err
	}

	created.Stops = make([]bookingmodels.Stop, 0, len(stops))
	for _, stop := range stops {
		stopRow := tx.QueryRow(ctx, `
			INSERT INTO taxi_booking_stops (id, booking_id, sequence, type, address, lat, lng)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING `+stopColumns,
			uuid.NewString(), created.ID, stop.Sequence, string(stop.Type), stop.Address, stop.Lat, stop.Lng,
		)
		s, err := scanStop(stopRow)
		if err != nil {
			return bookingmodels.Booking{}, err
		}
		created.Stops = append(created.Stops, s)
	}

	// Written inside the SAME transaction as the booking + stops above: a
	// crash between commit and delivery can never lose this event, because
	// there is no window where the booking exists without it.
	payload, err := json.Marshal(bookingCreatedPayload{
		BookingID: created.ID, CustomerID: created.CustomerID,
		MarketID: created.MarketID, OperatingZoneID: created.OperatingZoneID,
		VehicleClass: created.VehicleClass, Status: string(created.Status),
		FareEstimateKobo: created.FareEstimateKobo, CreatedAt: created.CreatedAt,
	})
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	if err := outbox.Write(ctx, tx, "booking", created.ID, outbox.EventBookingCreated, payload); err != nil {
		return bookingmodels.Booking{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return bookingmodels.Booking{}, err
	}
	return created, nil
}

// bookingCreatedPayload is the taxi.booking.created event body. Kept
// intentionally small (identifiers + the facts that gate downstream
// consumers) rather than a full booking dump -- a subscriber that needs more
// can always read the booking back by ID.
type bookingCreatedPayload struct {
	BookingID        string    `json:"booking_id"`
	CustomerID       string    `json:"customer_id"`
	MarketID         string    `json:"market_id"`
	OperatingZoneID  string    `json:"operating_zone_id"`
	VehicleClass     string    `json:"vehicle_class"`
	Status           string    `json:"status"`
	FareEstimateKobo *int64    `json:"fare_estimate_kobo,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func (r *PostgresBookingRepository) GetByID(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `SELECT `+bookingColumns+` FROM taxi_bookings WHERE id = $1`, id)
	b, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, apperrors.NotFound("Ride could not be found.", err)
	}
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	stops, err := r.stopsFor(ctx, b.ID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	b.Stops = stops
	return b, nil
}

func (r *PostgresBookingRepository) GetByIDForCustomer(ctx context.Context, id, customerID string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `SELECT `+bookingColumns+` FROM taxi_bookings WHERE id = $1 AND customer_id = $2`, id, customerID)
	b, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, apperrors.NotFound("Ride could not be found.", err)
	}
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	stops, err := r.stopsFor(ctx, b.ID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	b.Stops = stops
	return b, nil
}

func (r *PostgresBookingRepository) GetByIDForProvider(ctx context.Context, id, providerID string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `SELECT `+bookingColumns+` FROM taxi_bookings WHERE id = $1 AND provider_id = $2`, id, providerID)
	b, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, apperrors.NotFound("Ride could not be found.", err)
	}
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	stops, err := r.stopsFor(ctx, b.ID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	b.Stops = stops
	return b, nil
}

func (r *PostgresBookingRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.BeginTx(ctx, pgx.TxOptions{})
}

// scanTransitionResult wraps scanBooking for every guarded-CAS transition
// below: a zero-row RETURNING (wrong provider, wrong status, or a
// since-superseded booking) is a real Postgres idiom for "the guard refused
// this write", surfaced as apperrors.Conflict rather than a raw NotFound --
// the caller does not get to distinguish "does not exist" from "exists but
// is not in the right state for this action", which is deliberate: neither
// leaks more than the other party needs to know.
func scanTransitionResult(row scannable) (bookingmodels.Booking, error) {
	b, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", err)
	}
	return b, err
}

func (r *PostgresBookingRepository) MarkMatched(ctx context.Context, exec Executor, bookingID, providerID string, paymentIntentID *string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings SET provider_id = $1, status = $2, matched_at = now(), updated_at = now(), payment_intent_id = $3
		WHERE id = $4 AND status = $5
		RETURNING `+bookingColumns,
		providerID, string(bookingmodels.StatusAwaitingAccept), paymentIntentID, bookingID, string(bookingmodels.StatusPendingMatch),
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) MarkAccepted(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings SET status = $1, accepted_at = now(), updated_at = now()
		WHERE id = $2 AND status = $3 AND provider_id = $4
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusAccepted), bookingID, string(bookingmodels.StatusAwaitingAccept), providerID,
	)
	return scanTransitionResult(row)
}

// ResetToMatching's guard is the exact SQL pattern verified working in
// driver-hauling-service's booking_repository.go: guarded on BOTH status AND
// provider_id, so a late accept landing between the matcher's timeout firing
// and this call is never clobbered -- zero rows affected here means the
// caller must re-read the booking and stop matching, not retry the reset.
func (r *PostgresBookingRepository) ResetToMatching(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings SET status = $1, provider_id = NULL, matched_at = NULL, updated_at = now()
		WHERE id = $2 AND status = $3 AND provider_id = $4
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusPendingMatch), bookingID, string(bookingmodels.StatusAwaitingAccept), providerID,
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) MarkUnmatched(ctx context.Context, exec Executor, bookingID, reason string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	cancelledBy := "system"
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET status = $1, cancelled_at = now(), updated_at = now(),
		       cancel_reason = $2, cancelled_by = $3
		WHERE id = $4 AND status = $5
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusUnmatched), reason, cancelledBy, bookingID, string(bookingmodels.StatusPendingMatch),
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) MarkEnRoutePickup(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings SET status = $1, updated_at = now()
		WHERE id = $2 AND provider_id = $3 AND status = $4
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusEnRoutePickup), bookingID, providerID, string(bookingmodels.StatusAccepted),
	)
	return scanTransitionResult(row)
}

// MarkArrivedAtPickup also flips pickup_verification_status to 'pending' in
// the SAME statement: arriving is exactly the moment the verification
// window opens (there is no separate driver action between them -- see
// StatusPickupVerificationPending's own doc comment), and this is a
// deterministic fact about the transition itself, not dependent on the
// actual generated code (which the usecase layer creates separately, since
// it needs the handover secret).
func (r *PostgresBookingRepository) MarkArrivedAtPickup(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET status = $1, arrived_at = now(), updated_at = now(), pickup_verification_status = $2
		WHERE id = $3 AND provider_id = $4 AND status = $5
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusArrivedAtPickup), string(bookingmodels.PickupVerificationPending),
		bookingID, providerID, string(bookingmodels.StatusEnRoutePickup),
	)
	return scanTransitionResult(row)
}

// MarkInProgress ("start trip") now REQUIRES pickup_verification_status =
// 'verified' in the guard itself (phase B5 -- this un-stubs the B3 gate
// that was previously always-true). This is the real, enforced boundary;
// the usecase layer's own pre-check exists only to give a clearer error
// message, never as a substitute for this guard.
func (r *PostgresBookingRepository) MarkPickupVerified(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings SET pickup_verification_status = $1, updated_at = now()
		WHERE id = $2 AND provider_id = $3 AND status = $4 AND pickup_verification_status = $5
		RETURNING `+bookingColumns,
		string(bookingmodels.PickupVerificationVerified), bookingID, providerID,
		string(bookingmodels.StatusArrivedAtPickup), string(bookingmodels.PickupVerificationPending),
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) MarkDeliveryVerified(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings SET delivery_verification_status = $1, updated_at = now()
		WHERE id = $2 AND provider_id = $3 AND status = $4 AND delivery_verification_status = $5
		RETURNING `+bookingColumns,
		string(bookingmodels.DeliveryVerificationVerified), bookingID, providerID,
		string(bookingmodels.StatusInProgress), string(bookingmodels.DeliveryVerificationPending),
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) MarkInProgress(ctx context.Context, exec Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	// The CASE opens the delivery-verification window for a parcel in the SAME
	// guarded statement that starts the trip -- a second UPDATE could be lost
	// to a crash between the two, leaving a package in transit that could then
	// be completed without anyone confirming it reached the receiver.
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET status = $1, started_at = now(), updated_at = now(),
		       delivery_verification_status = CASE
		           WHEN ride_purpose = 'parcel' THEN 'pending'
		           ELSE delivery_verification_status
		       END
		WHERE id = $2 AND provider_id = $3 AND status = $4 AND pickup_verification_status = $5
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusInProgress), bookingID, providerID,
		string(bookingmodels.StatusArrivedAtPickup), string(bookingmodels.PickupVerificationVerified),
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) MarkCompleted(ctx context.Context, exec Executor, bookingID, providerID string, actualDistanceKm float64, fareFinalKobo int64) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET status = $1, completed_at = now(), updated_at = now(),
		       actual_distance_km = $2, fare_final_kobo = $3
		WHERE id = $4 AND provider_id = $5 AND status = $6
		  AND delivery_verification_status <> 'pending'
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusCompleted), actualDistanceKm, fareFinalKobo,
		bookingID, providerID, string(bookingmodels.StatusInProgress),
	)
	return scanTransitionResult(row)
}

// cancellableByCustomer mirrors bookingmodels.ActiveStatuses minus
// StatusInProgress: once a ride is genuinely underway, self-service
// cancellation stops being a simple status flip (fare/safety implications
// belong to a future dispute flow, not this endpoint) -- the same boundary
// the addendum draws for a driver mid-trip in CancelByProvider below.
var cancellableByCustomer = []string{
	string(bookingmodels.StatusPendingMatch), string(bookingmodels.StatusAwaitingAccept),
	string(bookingmodels.StatusAccepted), string(bookingmodels.StatusEnRoutePickup),
	string(bookingmodels.StatusArrivedAtPickup),
}

var cancellableByProvider = []string{
	string(bookingmodels.StatusAccepted), string(bookingmodels.StatusEnRoutePickup),
	string(bookingmodels.StatusArrivedAtPickup),
}

func (r *PostgresBookingRepository) CancelByCustomer(ctx context.Context, exec Executor, bookingID, customerID, reason string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	cancelledBy := "customer"
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET status = $1, cancelled_at = now(), updated_at = now(),
		       cancel_reason = $2, cancelled_by = $3
		WHERE id = $4 AND customer_id = $5 AND status = ANY($6)
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusCancelled), reason, cancelledBy,
		bookingID, customerID, cancellableByCustomer,
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) CancelByProvider(ctx context.Context, exec Executor, bookingID, providerID, reason string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	cancelledBy := "provider"
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET status = $1, cancelled_at = now(), updated_at = now(),
		       cancel_reason = $2, cancelled_by = $3
		WHERE id = $4 AND provider_id = $5 AND status = ANY($6)
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusCancelled), reason, cancelledBy,
		bookingID, providerID, cancellableByProvider,
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) SetPaymentStatus(ctx context.Context, exec Executor, bookingID, status string) error {
	if exec == nil {
		exec = r.db
	}
	_, err := exec.Exec(ctx, `UPDATE taxi_bookings SET payment_status = $1, updated_at = now() WHERE id = $2`, status, bookingID)
	return err
}

// activeProviderStatuses is what a driver app polls for: an offer awaiting
// their response, or a ride already committed and underway.
var activeProviderStatuses = []string{
	string(bookingmodels.StatusAwaitingAccept), string(bookingmodels.StatusAccepted),
	string(bookingmodels.StatusEnRoutePickup), string(bookingmodels.StatusArrivedAtPickup),
	string(bookingmodels.StatusInProgress),
}

// ListActiveByProvider is the poll target for a driver app with no push
// channel wired yet in this build: their current offer (if any) and their
// committed-but-not-yet-completed ride (if any). Newest first, so a fresh
// offer always sorts above an older active ride.
func (r *PostgresBookingRepository) ListActiveByProvider(ctx context.Context, providerID string) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+bookingColumns+`
		FROM taxi_bookings WHERE provider_id = $1 AND status = ANY($2)
		ORDER BY created_at DESC
	`, providerID, activeProviderStatuses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresBookingRepository) ListDueScheduled(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text FROM taxi_bookings
		WHERE status = $1 AND dispatch_start_at <= $2
		ORDER BY dispatch_start_at ASC
	`, string(bookingmodels.StatusScheduled), now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *PostgresBookingRepository) PromoteScheduledToPendingMatch(ctx context.Context, bookingID string, matchingStartedAt time.Time) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET status = $1, matching_started_at = $2, updated_at = now()
		 WHERE id = $3 AND status = $4
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusPendingMatch), matchingStartedAt, bookingID, string(bookingmodels.StatusScheduled),
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) ListActive(ctx context.Context, marketID string) ([]bookingmodels.Booking, error) {
	statuses := make([]string, len(bookingmodels.ActiveStatuses))
	for i, s := range bookingmodels.ActiveStatuses {
		statuses[i] = string(s)
	}
	query := `SELECT ` + bookingColumns + ` FROM taxi_bookings WHERE status = ANY($1)`
	args := []any{statuses}
	if marketID != "" {
		query += ` AND market_id = $2`
		args = append(args, marketID)
	}
	query += ` ORDER BY created_at ASC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresBookingRepository) ListUnmatched(ctx context.Context, marketID, zoneID string, olderThan time.Time) ([]bookingmodels.Booking, error) {
	query := `SELECT ` + bookingColumns + ` FROM taxi_bookings WHERE status = $1`
	args := []any{string(bookingmodels.StatusUnmatched)}
	if marketID != "" {
		args = append(args, marketID)
		query += fmt.Sprintf(` AND market_id = $%d`, len(args))
	}
	if zoneID != "" {
		args = append(args, zoneID)
		query += fmt.Sprintf(` AND operating_zone_id = $%d`, len(args))
	}
	if !olderThan.IsZero() {
		args = append(args, olderThan)
		query += fmt.Sprintf(` AND created_at <= $%d`, len(args))
	}
	query += ` ORDER BY created_at ASC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresBookingRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT status, count(*) FROM taxi_bookings GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}

func (r *PostgresBookingRepository) ListStaleMatching(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+bookingColumns+`
		FROM taxi_bookings
		WHERE status = ANY($1) AND updated_at <= $2
		ORDER BY updated_at ASC
	`, []string{string(bookingmodels.StatusPendingMatch), string(bookingmodels.StatusAwaitingAccept)}, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresBookingRepository) ListStaleInProgress(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+bookingColumns+`
		FROM taxi_bookings
		WHERE status = $1 AND started_at IS NOT NULL AND started_at <= $2
		ORDER BY started_at ASC
	`, string(bookingmodels.StatusInProgress), cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

var adminCancellableStatuses = []string{
	string(bookingmodels.StatusPendingMatch), string(bookingmodels.StatusScheduled),
	string(bookingmodels.StatusAwaitingAccept), string(bookingmodels.StatusAccepted),
	string(bookingmodels.StatusEnRoutePickup), string(bookingmodels.StatusArrivedAtPickup),
	string(bookingmodels.StatusInProgress),
}

func (r *PostgresBookingRepository) AdminCancel(ctx context.Context, exec Executor, bookingID, reason string) (bookingmodels.Booking, error) {
	if exec == nil {
		exec = r.db
	}
	cancelledBy := "admin"
	row := exec.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET status = $1, cancelled_at = now(), updated_at = now(),
		       cancel_reason = $2, cancelled_by = $3
		 WHERE id = $4 AND status = ANY($5)
		RETURNING `+bookingColumns,
		string(bookingmodels.StatusCancelled), reason, cancelledBy, bookingID, adminCancellableStatuses,
	)
	return scanTransitionResult(row)
}

func (r *PostgresBookingRepository) RecordEvent(ctx context.Context, exec Executor, event bookingmodels.BookingEvent) error {
	if exec == nil {
		exec = r.db
	}
	metadata := event.Metadata
	if metadata == nil {
		metadata = []byte("{}")
	}
	_, err := exec.Exec(ctx, `
		INSERT INTO booking_events (booking_id, event_type, actor_type, actor_id, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`, event.BookingID, event.EventType, event.ActorType, event.ActorID, metadata)
	return err
}

func (r *PostgresBookingRepository) ListEvents(ctx context.Context, bookingID string) ([]bookingmodels.BookingEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, booking_id::text, event_type, actor_type, actor_id, metadata, created_at
		FROM booking_events WHERE booking_id = $1 ORDER BY created_at ASC
	`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.BookingEvent
	for rows.Next() {
		var e bookingmodels.BookingEvent
		if err := rows.Scan(&e.ID, &e.BookingID, &e.EventType, &e.ActorType, &e.ActorID, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

var terminalProviderStatuses = []string{string(bookingmodels.StatusCompleted), string(bookingmodels.StatusCancelled)}

func (r *PostgresBookingRepository) ListTerminalByProviderInRange(ctx context.Context, providerID string, from, until time.Time) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+bookingColumns+`
		FROM taxi_bookings
		WHERE provider_id = $1 AND status = ANY($2) AND created_at BETWEEN $3 AND $4
		ORDER BY created_at DESC
	`, providerID, terminalProviderStatuses, from, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresBookingRepository) ListByCustomer(ctx context.Context, customerID string, limit int) ([]bookingmodels.Booking, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+bookingColumns+`
		FROM taxi_bookings WHERE customer_id = $1
		ORDER BY created_at DESC LIMIT $2
	`, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Stops are attached with ONE batched query for the whole page, never a
	// per-booking fetch: the passenger's trip history renders each ride's
	// pickup and destination, and taxi keeps those in a separate table
	// (unlike hauling/dispatch, which carry addresses as booking columns), so
	// returning bookings without stops leaves the history list with nothing
	// to show. This closes that gap without reintroducing the N+1 the
	// previous comment here rightly warned about.
	if err := r.attachStops(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachStops loads every stop for the given bookings in a single query and
// assigns them, preserving sequence order within each booking.
func (r *PostgresBookingRepository) attachStops(ctx context.Context, bookings []bookingmodels.Booking) error {
	if len(bookings) == 0 {
		return nil
	}
	ids := make([]string, len(bookings))
	for i, b := range bookings {
		ids[i] = b.ID
	}

	rows, err := r.db.Query(ctx, `
		SELECT `+stopColumns+`
		FROM taxi_booking_stops
		WHERE booking_id = ANY($1::uuid[])
		ORDER BY booking_id, sequence ASC
	`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	byBooking := make(map[string][]bookingmodels.Stop, len(bookings))
	for rows.Next() {
		s, err := scanStop(rows)
		if err != nil {
			return err
		}
		byBooking[s.BookingID] = append(byBooking[s.BookingID], s)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range bookings {
		bookings[i].Stops = byBooking[bookings[i].ID]
	}
	return nil
}

// HasActiveBooking is a single EXISTS query against bookingmodels.ActiveStatuses,
// not a fetch-then-check-in-Go -- the guard must see a consistent snapshot at
// the moment of the check, not one assembled from a separate round trip.
func (r *PostgresBookingRepository) HasActiveBooking(ctx context.Context, customerID string) (bool, error) {
	statuses := make([]string, len(bookingmodels.ActiveStatuses))
	for i, s := range bookingmodels.ActiveStatuses {
		statuses[i] = string(s)
	}
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM taxi_bookings WHERE customer_id = $1 AND status = ANY($2)
		)
	`, customerID, statuses).Scan(&exists)
	return exists, err
}

func (r *PostgresBookingRepository) stopsFor(ctx context.Context, bookingID string) ([]bookingmodels.Stop, error) {
	rows, err := r.db.Query(ctx, `SELECT `+stopColumns+` FROM taxi_booking_stops WHERE booking_id = $1 ORDER BY sequence ASC`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bookingmodels.Stop
	for rows.Next() {
		s, err := scanStop(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanBooking(row scannable) (bookingmodels.Booking, error) {
	var b bookingmodels.Booking
	var status, pickupVerificationStatus, bookingMode string
	var ridePurpose, deliveryVerificationStatus string
	err := row.Scan(
		&b.ID, &b.CustomerID, &b.ProviderID, &b.VehicleID,
		&b.MarketID, &b.OperatingZoneID,
		&bookingMode, &b.VehicleClass, &b.PassengerCount, &b.RideNotes, &b.ReservedDurationMinutes,
		&b.FareEstimateKobo, &b.FareFinalKobo, &b.PricingQuoteReference, &b.PricingVersion,
		&b.DistanceKm, &b.DurationMinutes, &b.ActualDistanceKm,
		&b.PaymentMethod, &b.PaymentStatus, &b.PaymentIntentID,
		&status, &pickupVerificationStatus,
		&ridePurpose, &b.ReceiverName, &b.ReceiverPhone,
		&b.PackageDesc, &b.PackageType, &b.PackageSize, &b.IsFragile,
		&deliveryVerificationStatus,
		&b.ScheduledAt, &b.DispatchStartAt, &b.ReservationStatus, &b.ReservedProviderID,
		&b.BackupDispatchAt, &b.ProviderConfirmedAt, &b.CustomerReadyWindow, &b.MatchingStartedAt,
		&b.CancelReason, &b.CancelledBy,
		&b.MatchedAt, &b.AcceptedAt, &b.ArrivedAt, &b.StartedAt, &b.CompletedAt, &b.CancelledAt,
		&b.CreatedAt, &b.UpdatedAt,
	)
	b.BookingMode = bookingmodels.BookingMode(bookingMode)
	b.Status = bookingmodels.Status(status)
	b.PickupVerificationStatus = bookingmodels.PickupVerificationStatus(pickupVerificationStatus)
	b.RidePurpose = bookingmodels.RidePurpose(ridePurpose)
	b.DeliveryVerificationStatus = bookingmodels.DeliveryVerificationStatus(deliveryVerificationStatus)
	return b, err
}

func scanStop(row scannable) (bookingmodels.Stop, error) {
	var s bookingmodels.Stop
	var stopType, status string
	err := row.Scan(
		&s.ID, &s.BookingID, &s.Sequence, &stopType, &s.Address, &s.Lat, &s.Lng,
		&status, &s.ArrivedAt, &s.DepartedAt, &s.WaitSeconds, &s.Notes,
	)
	s.Type = bookingmodels.StopType(stopType)
	s.Status = bookingmodels.StopStatus(status)
	return s, err
}
