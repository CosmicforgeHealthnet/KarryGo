package bookingrepo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

type BookingRepository interface {
	Create(ctx context.Context, b bookingmodels.Booking) (bookingmodels.Booking, error)
	GetByID(ctx context.Context, id string) (bookingmodels.Booking, error)
	GetByIDForCustomer(ctx context.Context, id, customerID string) (bookingmodels.Booking, error)
	ListByCustomer(ctx context.Context, customerID string, limit, offset int) ([]bookingmodels.Booking, error)
	ListByProvider(ctx context.Context, providerID string, limit, offset int) ([]bookingmodels.Booking, error)
	UpdateStatus(ctx context.Context, id, status string) (bookingmodels.Booking, error)
	MarkMatched(ctx context.Context, id, providerID, truckID string) (bookingmodels.Booking, error)
	MarkAccepted(ctx context.Context, id, providerID string) (bookingmodels.Booking, error)
	MarkEnRoutePickup(ctx context.Context, id string) (bookingmodels.Booking, error)
	MarkArrivedAtPickup(ctx context.Context, id string, pickup handover.CodeRecord) (bookingmodels.Booking, error)
	MarkPickedUp(ctx context.Context, id string) (bookingmodels.Booking, error)
	MarkEnRouteDelivery(ctx context.Context, id string, delivery handover.CodeRecord) (bookingmodels.Booking, error)
	MarkDelivered(ctx context.Context, id, proofPhotoURL, signatureURL string) (bookingmodels.Booking, error)
	MarkCompleted(ctx context.Context, id string) (bookingmodels.Booking, error)
	IncrementPickupCodeAttempt(ctx context.Context, id string) error
	IncrementDeliveryCodeAttempt(ctx context.Context, id string) error
	MarkPickupVerified(ctx context.Context, id string) error
	ResendPickupCode(ctx context.Context, id string, rec handover.CodeRecord) error
	ResendDeliveryCode(ctx context.Context, id string, rec handover.CodeRecord) error
	MarkUnmatched(ctx context.Context, id string) (bookingmodels.Booking, error)
	CancelByCustomer(ctx context.Context, id, customerID, reason string) (bookingmodels.Booking, error)
	CancelByProvider(ctx context.Context, id, providerID, reason string) (bookingmodels.Booking, error)
	ResetToMatching(ctx context.Context, id, expectedProviderID string) (bookingmodels.Booking, error)
	SetPayment(ctx context.Context, id, paymentStatus, paymentIntentID string) (bookingmodels.Booking, error)
	SaveDropoffRoute(ctx context.Context, id, polyline string, distanceM, durationS int64) error
	SavePickupRoute(ctx context.Context, id, polyline string, distanceM, durationS int64) error
	ListDeliveredForAutoComplete(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error)
	ListStaleMatching(ctx context.Context, pendingCutoff, awaitingCutoff time.Time) ([]bookingmodels.Booking, error)
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

func (r *PostgresBookingRepository) CommerceMissionScope(ctx context.Context, bookingID string) (string, []string, bool, error) {
	var parentID string
	var vendorIDs []string
	err := r.db.QueryRow(ctx, `SELECT parent_order_ref,vendor_order_refs FROM commerce_missions WHERE booking_id=$1 AND status='accepted'`, bookingID).Scan(&parentID, &vendorIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, false, nil
	}
	return parentID, vendorIDs, err == nil, err
}

func (r *PostgresBookingRepository) CommerceProviderMetadata(ctx context.Context, bookingID string) (*bookingmodels.CommerceOperations, error) {
	var (
		metadata                   bookingmodels.CommerceOperations
		pickups, parcels, handling []byte
	)
	err := r.db.QueryRow(ctx, `
		SELECT operation_ref,vehicle_class,pickup_stops,parcels,manifest_ref,
		       manifest_version,handling_requirements,pickup_window_start,
		       pickup_window_end,pickup_timezone,loading_duration_seconds
		  FROM commerce_missions
		 WHERE booking_id=$1 AND status IN ('accepted','cancelled')`, bookingID).Scan(
		&metadata.OperationRef, &metadata.VehicleClass, &pickups, &parcels,
		&metadata.ManifestRef, &metadata.ManifestVersion, &handling,
		&metadata.PickupWindowStart, &metadata.PickupWindowEnd,
		&metadata.PickupTimezone, &metadata.LoadingDurationSeconds,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	metadata.SourceType = "VENDOR_COMMERCE"
	if err = json.Unmarshal(pickups, &metadata.PickupStops); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(handling, &metadata.Handling); err != nil {
		return nil, err
	}
	metadata.Parcels, err = bookingmodels.SanitizeCommerceParcels(parcels)
	if err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (r *PostgresBookingRepository) ListCompletedCommerceBookings(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `SELECT b.id::text FROM haulage_bookings b JOIN commerce_missions m ON m.booking_id=b.id AND m.status='accepted' WHERE b.status='completed' ORDER BY b.completed_at,b.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (r *PostgresBookingRepository) Create(ctx context.Context, b bookingmodels.Booking) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO haulage_bookings (
			id, customer_id, pickup_address, pickup_lat, pickup_lng,
			dropoff_address, dropoff_lat, dropoff_lng,
			cargo_type, preferred_truck_type, cargo_weight_kg, cargo_description,
			requires_helpers, helper_count,
			weight_category, receiver_name, receiver_phone, package_content, package_size, is_fragile,
			distance_km, fare_estimate_kobo, payment_method, payment_status, status, scheduled_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
		RETURNING `+bookingSelectCols,
		b.ID, b.CustomerID,
		b.PickupAddress, b.PickupLat, b.PickupLng,
		b.DropoffAddress, b.DropoffLat, b.DropoffLng,
		b.CargoType, b.PreferredTruckType, b.CargoWeightKg, b.CargoDescription,
		b.RequiresHelpers, b.HelperCount,
		b.WeightCategory, b.ReceiverName, b.ReceiverPhone, b.PackageContent, b.PackageSize, b.IsFragile,
		b.DistanceKm, b.FareEstimateKobo, b.PaymentMethod, b.PaymentStatus, b.Status, b.ScheduledAt,
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

// MarkMatched assigns a provider/truck and moves the booking to
// awaiting_acceptance, but only from pending_match. The status guard makes this a
// compare-and-set so a late ResetToMatching (from a previous provider's matching
// goroutine) cannot clobber a fresh match, and two concurrent matchers cannot both
// claim the same booking. Returns NotFound when the guard fails (already matched
// or terminal).
func (r *PostgresBookingRepository) MarkMatched(ctx context.Context, id, providerID, truckID string) (bookingmodels.Booking, error) {
	// A provider may be online without a specific truck selected (the app's
	// "Go Online" sends no truck_id), so truckID can be empty. truck_id is a uuid
	// column, and binding "" makes Postgres reject the whole UPDATE — which would
	// silently strand the booking in pending_match. Bind NULL instead; the truck
	// is assigned when the provider accepts.
	var truck *string
	if truckID != "" {
		truck = &truckID
	}
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET provider_id=$2, truck_id=$3, status=$4, matched_at=now(), updated_at=now()
		WHERE id=$1 AND status=$5 RETURNING `+bookingSelectCols,
		id, providerID, truck, bookingmodels.StatusAwaitingAcceptance, bookingmodels.StatusPendingMatch)
	return scanBookingNotFound(row)
}

// SetPayment records the payment method outcome for a booking: its payment status
// and (optionally) the payment-wallet intent id bound to it.
func (r *PostgresBookingRepository) SetPayment(ctx context.Context, id, paymentStatus, paymentIntentID string) (bookingmodels.Booking, error) {
	var intent *string
	if paymentIntentID != "" {
		intent = &paymentIntentID
	}
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET payment_status=$2,
		    payment_intent_id=COALESCE($3, payment_intent_id),
		    updated_at=now()
		WHERE id=$1 RETURNING `+bookingSelectCols,
		id, paymentStatus, intent)
	return scanBookingNotFound(row)
}

// MarkAccepted moves the booking to accepted, but only when it is still
// awaiting acceptance AND still assigned to this provider. The provider_id guard
// closes the race where the booking was reset and re-matched to another provider
// between the caller's ownership read and this update. Returns NotFound when the
// guard fails (expired, reassigned, or already accepted).
func (r *PostgresBookingRepository) MarkAccepted(ctx context.Context, id, providerID string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, accepted_at=now(), updated_at=now()
		WHERE id=$1 AND status=$3 AND provider_id=$4 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusAccepted, bookingmodels.StatusAwaitingAcceptance, providerID)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) MarkEnRoutePickup(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, updated_at=now()
		WHERE id=$1 AND status=$3 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusEnRoutePickup, bookingmodels.StatusAccepted)
	return scanBookingNotFound(row)
}

// MarkArrivedAtPickup moves the booking to arrived_at_pickup and stores a
// freshly generated pickup credential (the driver's own code, later verified
// by the customer), resetting its attempt/resend counters.
func (r *PostgresBookingRepository) MarkArrivedAtPickup(ctx context.Context, id string, pickup handover.CodeRecord) (bookingmodels.Booking, error) {
	if pickup.Version > 0 {
		tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return bookingmodels.Booking{}, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		row := tx.QueryRow(ctx, `
			UPDATE haulage_bookings
			SET status=$2,updated_at=now(),pickup_code='',pickup_code_hash='',
			    pickup_code_expires_at=NULL,pickup_code_attempts=0,
			    pickup_code_resend_count=0,pickup_code_last_sent_at=now(),pickup_code_verified_at=NULL
			WHERE id=$1 AND status=$3 AND provider_id=$4 RETURNING `+bookingSelectCols,
			id, bookingmodels.StatusArrivedAtPickup, bookingmodels.StatusEnRoutePickup, pickup.ProviderID)
		updated, err := scanBookingNotFound(row)
		if err != nil {
			return bookingmodels.Booking{}, err
		}
		if err = insertIssuedSecureChallenge(ctx, tx, pickup); err != nil {
			return bookingmodels.Booking{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return bookingmodels.Booking{}, err
		}
		return updated, nil
	}
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$2, updated_at=now(),
		    pickup_code=$4, pickup_code_hash=$5, pickup_code_expires_at=$6,
		    pickup_code_attempts=0, pickup_code_resend_count=0,
		    pickup_code_last_sent_at=now(), pickup_code_verified_at=NULL
		WHERE id=$1 AND status=$3 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusArrivedAtPickup, bookingmodels.StatusEnRoutePickup,
		pickup.Code, pickup.Hash, pickup.ExpiresAt)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) MarkPickedUp(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, picked_up_at=now(), updated_at=now()
		WHERE id=$1 AND status IN ($3,$4,$5) RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusPickedUp, bookingmodels.StatusAccepted,
		bookingmodels.StatusEnRoutePickup, bookingmodels.StatusArrivedAtPickup)
	return scanBookingNotFound(row)
}

// MarkEnRouteDelivery moves the booking to en_route_delivery and stores a
// freshly generated delivery credential (the customer's own code, later
// verified by the driver), resetting its attempt/resend counters.
func (r *PostgresBookingRepository) MarkEnRouteDelivery(ctx context.Context, id string, delivery handover.CodeRecord) (bookingmodels.Booking, error) {
	if delivery.Version > 0 {
		tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return bookingmodels.Booking{}, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		row := tx.QueryRow(ctx, `
			UPDATE haulage_bookings
			SET status=$2,updated_at=now(),delivery_code='',delivery_code_hash='',
			    delivery_code_expires_at=NULL,delivery_code_attempts=0,
			    delivery_code_resend_count=0,delivery_code_last_sent_at=now(),delivery_code_verified_at=NULL
			WHERE id=$1 AND status=$3 AND provider_id=$4 RETURNING `+bookingSelectCols,
			id, bookingmodels.StatusEnRouteDelivery, bookingmodels.StatusPickedUp, delivery.ProviderID)
		updated, err := scanBookingNotFound(row)
		if err != nil {
			return bookingmodels.Booking{}, err
		}
		if err = insertIssuedSecureChallenge(ctx, tx, delivery); err != nil {
			return bookingmodels.Booking{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return bookingmodels.Booking{}, err
		}
		return updated, nil
	}
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$2, updated_at=now(),
		    delivery_code=$4, delivery_code_hash=$5, delivery_code_expires_at=$6,
		    delivery_code_attempts=0, delivery_code_resend_count=0,
		    delivery_code_last_sent_at=now(), delivery_code_verified_at=NULL
		WHERE id=$1 AND status=$3 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusEnRouteDelivery, bookingmodels.StatusPickedUp,
		delivery.Code, delivery.Hash, delivery.ExpiresAt)
	return scanBookingNotFound(row)
}

// IncrementPickupCodeAttempt bumps the customer's failed-verification-attempt
// counter for the driver's pickup credential.
func (r *PostgresBookingRepository) IncrementPickupCodeAttempt(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE haulage_bookings SET pickup_code_attempts = pickup_code_attempts + 1, updated_at=now() WHERE id=$1`, id)
	return err
}

// IncrementDeliveryCodeAttempt bumps the driver's failed-verification-attempt
// counter for the customer's delivery credential.
func (r *PostgresBookingRepository) IncrementDeliveryCodeAttempt(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE haulage_bookings SET delivery_code_attempts = delivery_code_attempts + 1, updated_at=now() WHERE id=$1`, id)
	return err
}

// MarkPickupVerified records that the customer successfully verified the
// driver's pickup credential, unblocking the driver's confirm-pickup action.
func (r *PostgresBookingRepository) MarkPickupVerified(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE haulage_bookings SET pickup_code_verified_at = now(), updated_at=now() WHERE id=$1`, id)
	return err
}

// ResendPickupCode regenerates the driver's pickup credential (called by the
// provider when their displayed code expired or was lost).
func (r *PostgresBookingRepository) ResendPickupCode(ctx context.Context, id string, rec handover.CodeRecord) error {
	if rec.Version > 0 {
		tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err = supersedeAndInsertSecureChallenge(ctx, tx, rec); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE haulage_bookings SET pickup_code='',pickup_code_hash='',pickup_code_expires_at=NULL,pickup_code_attempts=0,pickup_code_resend_count=$2,pickup_code_last_sent_at=now(),pickup_code_verified_at=NULL,updated_at=now() WHERE id=$1 AND provider_id=$3`, id, rec.ResendCount, rec.ProviderID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	_, err := r.db.Exec(ctx, `
		UPDATE haulage_bookings
		SET pickup_code=$2, pickup_code_hash=$3, pickup_code_expires_at=$4,
		    pickup_code_attempts=0, pickup_code_resend_count=pickup_code_resend_count+1,
		    pickup_code_last_sent_at=now(), pickup_code_verified_at=NULL, updated_at=now()
		WHERE id=$1`,
		id, rec.Code, rec.Hash, rec.ExpiresAt)
	return err
}

// ResendDeliveryCode regenerates the customer's delivery credential (called
// by the customer when their displayed code expired or was lost).
func (r *PostgresBookingRepository) ResendDeliveryCode(ctx context.Context, id string, rec handover.CodeRecord) error {
	if rec.Version > 0 {
		tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err = supersedeAndInsertSecureChallenge(ctx, tx, rec); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE haulage_bookings SET delivery_code='',delivery_code_hash='',delivery_code_expires_at=NULL,delivery_code_attempts=0,delivery_code_resend_count=$2,delivery_code_last_sent_at=now(),delivery_code_verified_at=NULL,updated_at=now() WHERE id=$1 AND customer_id=$3`, id, rec.ResendCount, rec.OwnerID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	_, err := r.db.Exec(ctx, `
		UPDATE haulage_bookings
		SET delivery_code=$2, delivery_code_hash=$3, delivery_code_expires_at=$4,
		    delivery_code_attempts=0, delivery_code_resend_count=delivery_code_resend_count+1,
		    delivery_code_last_sent_at=now(), delivery_code_verified_at=NULL, updated_at=now()
		WHERE id=$1`,
		id, rec.Code, rec.Hash, rec.ExpiresAt)
	return err
}

// SaveDropoffRoute stores the road route from pickup to drop-off. It is
// fetched once at booking creation, when both endpoints are already known.
func (r *PostgresBookingRepository) SaveDropoffRoute(ctx context.Context, id, polyline string, distanceM, durationS int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE haulage_bookings
		SET route_dropoff_polyline=$2, route_dropoff_distance_m=$3, route_dropoff_duration_s=$4,
		    route_fetched_at=now(), updated_at=now()
		WHERE id=$1`,
		id, polyline, distanceM, durationS)
	return err
}

// SavePickupRoute stores the road route from the driver to the pickup. It can
// only be fetched once a driver has accepted, since it starts at their
// position at that moment.
func (r *PostgresBookingRepository) SavePickupRoute(ctx context.Context, id, polyline string, distanceM, durationS int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE haulage_bookings
		SET route_pickup_polyline=$2, route_pickup_distance_m=$3, route_pickup_duration_s=$4,
		    route_fetched_at=now(), updated_at=now()
		WHERE id=$1`,
		id, polyline, distanceM, durationS)
	return err
}

// MarkDelivered moves the booking to delivered, storing the proof-of-delivery
// photo and receiver-signature URLs captured by the provider (either may be
// empty when the provider skipped them or the upload failed).
func (r *PostgresBookingRepository) MarkDelivered(ctx context.Context, id, proofPhotoURL, signatureURL string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$2, delivered_at=now(), proof_photo_url=$5, signature_url=$6, updated_at=now()
		WHERE id=$1 AND status IN ($3,$4) RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusDelivered, bookingmodels.StatusPickedUp, bookingmodels.StatusEnRouteDelivery,
		proofPhotoURL, signatureURL)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) MarkCompleted(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, completed_at=now(), updated_at=now()
		WHERE id=$1 AND status=$3 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusCompleted, bookingmodels.StatusDelivered)
	return scanBookingNotFound(row)
}

// MarkUnmatched moves a booking to unmatched, but only from pending_match. The
// status guard makes this a compare-and-set — a matcher deadline firing
// concurrently with a provider's accept must never clobber the accepted (or
// cancelled) booking to unmatched.
func (r *PostgresBookingRepository) MarkUnmatched(ctx context.Context, id string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings SET status=$2, updated_at=now()
		WHERE id=$1 AND status=$3 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusUnmatched, bookingmodels.StatusPendingMatch)
	return scanBookingNotFound(row)
}

// CancelByCustomer cancels a booking on the customer's behalf. Cancellation is
// only allowed until the cargo is picked up — once the provider is hauling
// (picked_up / en_route_delivery) the customer must go through support instead.
func (r *PostgresBookingRepository) CancelByCustomer(ctx context.Context, id, customerID, reason string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$3, cancel_reason=$4, cancelled_by='customer', cancelled_at=now(), updated_at=now()
		WHERE id=$1 AND customer_id=$2
		  AND status NOT IN ($5,$6,$7,$8,$9)
		RETURNING `+bookingSelectCols,
		id, customerID, bookingmodels.StatusCancelled, reason,
		bookingmodels.StatusPickedUp, bookingmodels.StatusEnRouteDelivery,
		bookingmodels.StatusDelivered, bookingmodels.StatusCompleted, bookingmodels.StatusCancelled)
	return scanBookingNotFound(row)
}

func (r *PostgresBookingRepository) CancelByProvider(ctx context.Context, id, providerID, reason string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$3, cancel_reason=$4, cancelled_by='provider', cancelled_at=now(), updated_at=now()
		WHERE id=$1 AND provider_id=$2
		  AND status IN ($5,$6,$7,$8,$9)
		RETURNING `+bookingSelectCols,
		id, providerID, bookingmodels.StatusCancelled, reason,
		bookingmodels.StatusAwaitingAcceptance, bookingmodels.StatusAccepted,
		bookingmodels.StatusEnRoutePickup, bookingmodels.StatusArrivedAtPickup,
		bookingmodels.StatusEnRouteDelivery)
	return scanBookingNotFound(row)
}

// ResetToMatching clears the provider assignment and returns the booking to
// pending_match, but only while it is still awaiting acceptance by the expected
// provider. The compare-and-set closes the accept/timeout race: if the provider's
// accept lands first, this reset fails with NotFound instead of clobbering an
// accepted trip back to matching.
func (r *PostgresBookingRepository) ResetToMatching(ctx context.Context, id, expectedProviderID string) (bookingmodels.Booking, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE haulage_bookings
		SET status=$2, provider_id=NULL, truck_id=NULL, matched_at=NULL, updated_at=now()
		WHERE id=$1 AND status=$3 AND provider_id=$4 RETURNING `+bookingSelectCols,
		id, bookingmodels.StatusPendingMatch,
		bookingmodels.StatusAwaitingAcceptance, expectedProviderID)
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

// ListStaleMatching returns bookings whose matching may have been orphaned by a
// service restart: pending_match rows older than pendingCutoff (a fresh booking's
// matcher registers itself synchronously, so anything older with no live matcher
// is stranded) and awaiting_acceptance rows whose offer window expired before
// awaitingCutoff (a healthy matcher would already have reset them).
func (r *PostgresBookingRepository) ListStaleMatching(ctx context.Context, pendingCutoff, awaitingCutoff time.Time) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+bookingSelectCols+` FROM haulage_bookings
		WHERE (status = $1 AND created_at < $2)
		   OR (status = $3 AND matched_at IS NOT NULL AND matched_at < $4)`,
		bookingmodels.StatusPendingMatch, pendingCutoff,
		bookingmodels.StatusAwaitingAcceptance, awaitingCutoff)
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
	payment_method, payment_status, payment_intent_id, status, cancel_reason, cancelled_by,
	proof_photo_url, signature_url,
	route_pickup_polyline, route_pickup_distance_m, route_pickup_duration_s,
	route_dropoff_polyline, route_dropoff_distance_m, route_dropoff_duration_s, route_fetched_at,
	pickup_code, pickup_code_hash, pickup_code_expires_at, pickup_code_attempts,
	pickup_code_resend_count, pickup_code_last_sent_at, pickup_code_verified_at,
	delivery_code, delivery_code_hash, delivery_code_expires_at, delivery_code_attempts,
	delivery_code_resend_count, delivery_code_last_sent_at, delivery_code_verified_at,
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
		&b.PaymentMethod, &b.PaymentStatus, &b.PaymentIntentID, &b.Status, &b.CancelReason, &b.CancelledBy,
		&b.ProofPhotoURL, &b.SignatureURL,
		&b.RoutePickupPolyline, &b.RoutePickupDistanceM, &b.RoutePickupDurationS,
		&b.RouteDropoffPolyline, &b.RouteDropoffDistanceM, &b.RouteDropoffDurationS, &b.RouteFetchedAt,
		&b.PickupCode, &b.PickupCodeHash, &b.PickupCodeExpiresAt, &b.PickupCodeAttempts,
		&b.PickupCodeResendCount, &b.PickupCodeLastSentAt, &b.PickupCodeVerifiedAt,
		&b.DeliveryCode, &b.DeliveryCodeHash, &b.DeliveryCodeExpiresAt, &b.DeliveryCodeAttempts,
		&b.DeliveryCodeResendCount, &b.DeliveryCodeLastSentAt, &b.DeliveryCodeVerifiedAt,
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
