package bookingrepo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type BookingRepository interface {
	Create(ctx context.Context, b bookingmodels.Booking) (bookingmodels.Booking, error)
	GetByID(ctx context.Context, id string) (bookingmodels.Booking, error)
	GetByIDForCustomer(ctx context.Context, id, customerID string) (bookingmodels.Booking, error)
	ListByCustomer(ctx context.Context, customerID string, limit, offset int) ([]bookingmodels.Booking, error)
	ListByProvider(ctx context.Context, providerID string, limit, offset int) ([]bookingmodels.Booking, error)
	UpdateStatus(ctx context.Context, id, status string) (bookingmodels.Booking, error)
	AssignProvider(ctx context.Context, id, providerID, truckID string) (bookingmodels.Booking, error)
	MarkMatched(ctx context.Context, id, providerID, truckID string) (bookingmodels.Booking, error)
	MarkAccepted(ctx context.Context, id string) (bookingmodels.Booking, error)
	MarkPickedUp(ctx context.Context, id string) (bookingmodels.Booking, error)
	MarkDelivered(ctx context.Context, id string) (bookingmodels.Booking, error)
	MarkCompleted(ctx context.Context, id string) (bookingmodels.Booking, error)
	CancelByCustomer(ctx context.Context, id, customerID, reason string) (bookingmodels.Booking, error)
	CancelByProvider(ctx context.Context, id, providerID, reason string) (bookingmodels.Booking, error)
	ResetToMatching(ctx context.Context, id string) (bookingmodels.Booking, error)
	ListDeliveredForAutoComplete(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error)
	AddEvent(ctx context.Context, event bookingmodels.BookingEvent) error
	CreateReview(ctx context.Context, r bookingmodels.BookingReview) (bookingmodels.BookingReview, error)
	GetReviewByBooking(ctx context.Context, bookingID string) (bookingmodels.BookingReview, error)
}

type BookingEvent = bookingmodels.BookingEvent

type PostgresBookingRepository struct {
	db *pgxpool.Pool
}

func NewPostgresBookingRepository(db *pgxpool.Pool) *PostgresBookingRepository {
	return &PostgresBookingRepository{db: db}
}

func (r *PostgresBookingRepository) Create(ctx context.Context, b bookingmodels.Booking) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO haulage_bookings (
			id, customer_id, pickup_address, pickup_lat, pickup_lng,
			dropoff_address, dropoff_lat, dropoff_lng,
			cargo_type, preferred_truck_type, cargo_weight_kg, cargo_description,
			requires_helpers, helper_count,
			weight_category, receiver_name, receiver_phone, package_content, package_size, is_fragile,
			distance_km, fare_estimate_kobo, status, scheduled_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		RETURNING `+bookingSelectCols,
		b.ID, b.CustomerID,
		b.PickupAddress, b.PickupLat, b.PickupLng,
		b.DropoffAddress, b.DropoffLat, b.DropoffLng,
		b.CargoType, b.PreferredTruckType, b.CargoWeightKg, b.CargoDescription,
		b.RequiresHelpers, b.HelperCount,
		b.WeightCategory, b.ReceiverName, b.ReceiverPhone, b.PackageContent, b.PackageSize, b.IsFragile,
		b.DistanceKm, b.FareEstimateKobo, b.Status, b.ScheduledAt,
	)
	return scanBooking(row)
}

func (r *PostgresBookingRepository) GetByID(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `SELECT `+bookingSelectCols+` FROM haulage_bookings WHERE id = $1`, id)
	b, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, apperrors.NotFound("Booking could not be found.", err)
	}
	return b, err
}

func (r *PostgresBookingRepository) GetByIDForCustomer(ctx context.Context, id, customerID string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `SELECT `+bookingSelectCols+` FROM haulage_bookings WHERE id = $1 AND customer_id = $2`, id, customerID)
	b, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, apperrors.NotFound("Booking could not be found.", err)
	}
	return b, err
}

func (r *PostgresBookingRepository) ListByCustomer(ctx context.Context, customerID string, limit, offset int) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+bookingSelectCols+` FROM haulage_bookings WHERE customer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		customerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookings(rows)
}

func (r *PostgresBookingRepository) ListByProvider(ctx context.Context, providerID string, limit, offset int) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+bookingSelectCols+` FROM haulage_bookings WHERE provider_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		providerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookings(rows)
}

func (r *PostgresBookingRepository) UpdateStatus(ctx context.Context, id, status string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `UPDATE haulage_bookings SET status=$2, updated_at=now() WHERE id=$1 RETURNING `+bookingSelectCols, id, status)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) AssignProvider(ctx context.Context, id, providerID, truckID string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET provider_id=$2, truck_id=$3, status=$4, matched_at=now(), updated_at=now()
		WHERE id=$1 RETURNING `+bookingSelectCols,
		id, providerID, truckID, bookingmodels.StatusAwaitingAcceptance)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) MarkMatched(ctx context.Context, id, providerID, truckID string) (bookingmodels.Booking, error) {
	return r.AssignProvider(ctx, id, providerID, truckID)
}

func (r *PostgresBookingRepository) MarkAccepted(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, accepted_at=now(), updated_at=now()
		WHERE id=$1 AND status=$3 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusAccepted, bookingmodels.StatusAwaitingAcceptance)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) MarkPickedUp(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, picked_up_at=now(), updated_at=now()
		WHERE id=$1 AND status IN ($3,$4) RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusPickedUp, bookingmodels.StatusAccepted, bookingmodels.StatusEnRoutePickup)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) MarkDelivered(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, delivered_at=now(), updated_at=now()
		WHERE id=$1 AND status IN ($3,$4) RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusDelivered, bookingmodels.StatusPickedUp, bookingmodels.StatusEnRouteDelivery)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) MarkCompleted(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, completed_at=now(), updated_at=now()
		WHERE id=$1 AND status=$3 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusCompleted, bookingmodels.StatusDelivered)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) CancelByCustomer(ctx context.Context, id, customerID, reason string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$3, cancel_reason=$4, cancelled_by='customer', cancelled_at=now(), updated_at=now()
		WHERE id=$1 AND customer_id=$2
		  AND status NOT IN ($5,$6,$7)
		RETURNING `+bookingSelectCols,
		id, customerID, bookingmodels.StatusCancelled, reason,
		bookingmodels.StatusDelivered, bookingmodels.StatusCompleted, bookingmodels.StatusCancelled)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) CancelByProvider(ctx context.Context, id, providerID, reason string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$3, cancel_reason=$4, cancelled_by='provider', cancelled_at=now(), updated_at=now()
		WHERE id=$1 AND provider_id=$2
		  AND status IN ($5,$6,$7)
		RETURNING `+bookingSelectCols,
		id, providerID, bookingmodels.StatusCancelled, reason,
		bookingmodels.StatusAwaitingAcceptance, bookingmodels.StatusAccepted, bookingmodels.StatusEnRoutePickup)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) ResetToMatching(ctx context.Context, id string) (bookingmodels.Booking, error) {
	// Guard: never overwrite a terminal status (cancelled/completed/delivered).
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$2, provider_id=NULL, truck_id=NULL, matched_at=NULL, updated_at=now()
		WHERE id=$1 AND status NOT IN ($3,$4,$5) RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusPendingMatch,
		bookingmodels.StatusCancelled, bookingmodels.StatusCompleted, bookingmodels.StatusDelivered)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) ListDeliveredForAutoComplete(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+bookingSelectCols+` FROM haulage_bookings WHERE status = $1 AND delivered_at < $2`,
		bookingmodels.StatusDelivered, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookings(rows)
}

func (r *PostgresBookingRepository) AddEvent(ctx context.Context, event bookingmodels.BookingEvent) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO booking_events (id, booking_id, event_type, actor_type, actor_id, metadata)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, event.ID, event.BookingID, event.EventType, event.ActorType, event.ActorID, event.Metadata)
	return err
}

// ─── scan helpers ─────────────────────────────────────────────────────────────

const bookingSelectCols = `
	id::text, customer_id::text, provider_id::text, truck_id::text,
	pickup_address, pickup_lat, pickup_lng,
	dropoff_address, dropoff_lat, dropoff_lng,
	cargo_type, preferred_truck_type, cargo_weight_kg, cargo_description, requires_helpers, helper_count,
	weight_category, receiver_name, receiver_phone, package_content, package_size, is_fragile,
	distance_km, fare_estimate_kobo, fare_final_kobo,
	payment_intent_id, status, cancel_reason, cancelled_by,
	matched_at, accepted_at, picked_up_at, delivered_at, completed_at, cancelled_at,
	scheduled_at, created_at, updated_at
`

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanBooking(row scannable) (bookingmodels.Booking, error) {
	var b bookingmodels.Booking
	err := row.Scan(
		&b.ID, &b.CustomerID, &b.ProviderID, &b.TruckID,
		&b.PickupAddress, &b.PickupLat, &b.PickupLng,
		&b.DropoffAddress, &b.DropoffLat, &b.DropoffLng,
		&b.CargoType, &b.PreferredTruckType, &b.CargoWeightKg, &b.CargoDescription, &b.RequiresHelpers, &b.HelperCount,
		&b.WeightCategory, &b.ReceiverName, &b.ReceiverPhone, &b.PackageContent, &b.PackageSize, &b.IsFragile,
		&b.DistanceKm, &b.FareEstimateKobo, &b.FareFinalKobo,
		&b.PaymentIntentID, &b.Status, &b.CancelReason, &b.CancelledBy,
		&b.MatchedAt, &b.AcceptedAt, &b.PickedUpAt, &b.DeliveredAt, &b.CompletedAt, &b.CancelledAt,
		&b.ScheduledAt, &b.CreatedAt, &b.UpdatedAt,
	)
	return b, err
}

func scanBookingNotFound(row scannable) (bookingmodels.Booking, error) {
	b, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, apperrors.NotFound("Booking could not be found.", err)
	}
	return b, err
}

func scanBookings(rows pgx.Rows) ([]bookingmodels.Booking, error) {
	var result []bookingmodels.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// ─── Review repository ──────────────────────────────────────────────────────

func (r *PostgresBookingRepository) CreateReview(ctx context.Context, rev bookingmodels.BookingReview) (bookingmodels.BookingReview, error) {
	if rev.ID == "" {
		rev.ID = uuid.NewString()
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO booking_reviews (id, booking_id, customer_id, provider_id, rating, review_text, recommends_driver)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id::text, booking_id::text, customer_id::text, provider_id::text, rating, review_text, recommends_driver, created_at
	`, rev.ID, rev.BookingID, rev.CustomerID, rev.ProviderID, rev.Rating, rev.ReviewText, rev.RecommendsDriver)

	var out bookingmodels.BookingReview
	err := row.Scan(&out.ID, &out.BookingID, &out.CustomerID, &out.ProviderID, &out.Rating, &out.ReviewText, &out.RecommendsDriver, &out.CreatedAt)
	return out, err
}

func (r *PostgresBookingRepository) GetReviewByBooking(ctx context.Context, bookingID string) (bookingmodels.BookingReview, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, booking_id::text, customer_id::text, provider_id::text, rating, review_text, recommends_driver, created_at
		FROM booking_reviews WHERE booking_id = $1
	`, bookingID)

	var out bookingmodels.BookingReview
	err := row.Scan(&out.ID, &out.BookingID, &out.CustomerID, &out.ProviderID, &out.Rating, &out.ReviewText, &out.RecommendsDriver, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.BookingReview{}, apperrors.NotFound("Review not found.", err)
	}
	return out, err
}
