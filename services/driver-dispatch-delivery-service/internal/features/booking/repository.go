package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

type Repository interface {
	Insert(ctx context.Context, booking Booking) (Booking, error)
	GetWithDerivedState(ctx context.Context, id string) (DerivedState, bool, error)
	ListWithDerivedState(ctx context.Context, customerID string, options ListBookingsOptions) ([]DerivedState, error)
	SetPayment(ctx context.Context, id string, paymentStatus string, paymentRef string) error
	SetAccepted(ctx context.Context, id string, providerID string) (bool, error)
	SaveDropoffRoute(ctx context.Context, id, polyline string, distanceM, durationS int64) error
	SavePickupRoute(ctx context.Context, id, polyline string, distanceM, durationS int64) error
	SetUnmatched(ctx context.Context, id string, at time.Time) (bool, error)
	SetCompleted(ctx context.Context, id string, completedAt time.Time, paymentStatus string) (bool, error)
	SetTerminal(ctx context.Context, id string, status string, paymentStatus string, reason string, at time.Time) (bool, error)
	InsertReview(ctx context.Context, review Review) (Review, error)
	GetReviewByBookingID(ctx context.Context, bookingID string) (Review, bool, error)
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const bookingColumns = `
	b.id::text, b.customer_id::text, b.status,
	b.pickup_address, b.pickup_lat::float8, b.pickup_lng::float8,
	b.dropoff_address, b.dropoff_lat::float8, b.dropoff_lng::float8,
	b.distance_km::float8, b.fare_amount, b.currency,
	b.payment_method, b.payment_status, b.payment_ref,
	b.receiver_name, b.receiver_phone,
	b.package_desc, b.package_weight::float8, b.package_type, b.package_size, b.is_fragile,
	b.service_tier, COALESCE(b.serviceability_quote_id, ''), COALESCE(b.serviceability_quote_version, 0),
      b.rollout_protection_mode, b.rollout_policy_version, b.handover_required,
      b.pickup_evidence_required, b.delivery_evidence_required, b.provider_transfer_required,
      b.provider_id::text,
      b.route_pickup_polyline, b.route_pickup_distance_m, b.route_pickup_duration_s,
      b.route_dropoff_polyline, b.route_dropoff_distance_m, b.route_dropoff_duration_s,
      b.cancel_reason,
	b.cancelled_at, b.completed_at, b.created_at, b.updated_at`

const derivedStateSelect = `
	SELECT ` + bookingColumns + `,
		COALESCE(rb.status, ''),
		COALESCE(t.id::text, ''),
		COALESCE(t.status, ''),
		COALESCE(t.provider_id::text, ''),
		COALESCE(t.delivery_code, ''),
		t.delivery_code_expires_at,
		COALESCE(t.delivery_code_attempts, 0)
	FROM dispatch_bookings b
	LEFT JOIN request_broadcasts rb ON rb.booking_id = b.id
	LEFT JOIN trips t ON t.booking_id = b.id`

func (r *PostgresRepository) Insert(ctx context.Context, booking Booking) (Booking, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO dispatch_bookings (
			customer_id, status,
			pickup_address, pickup_lat, pickup_lng,
			dropoff_address, dropoff_lat, dropoff_lng,
			distance_km, fare_amount, currency,
			payment_method, payment_status,
			receiver_name, receiver_phone, receiver_email, receiver_customer_id,
			package_desc, package_weight, package_type, package_size, is_fragile,
			service_tier, rollout_protection_mode, rollout_policy_version,
			handover_required, pickup_evidence_required, delivery_evidence_required, provider_transfer_required
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,NULLIF($17,'')::uuid,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
		RETURNING id::text, created_at, updated_at
	`,
		booking.CustomerID, booking.Status,
		booking.PickupAddress, booking.PickupLat, booking.PickupLng,
		booking.DropoffAddress, booking.DropoffLat, booking.DropoffLng,
		booking.DistanceKM, booking.FareAmount, booking.Currency,
		booking.PaymentMethod, booking.PaymentStatus,
		booking.ReceiverName, booking.ReceiverPhone, booking.ReceiverEmail, booking.ReceiverCustomerID,
		booking.PackageDesc, booking.PackageWeight, booking.PackageType, booking.PackageSize, booking.IsFragile,
		booking.ServiceTier, booking.RolloutProtectionMode, booking.RolloutPolicyVersion,
		booking.HandoverRequired, booking.PickupEvidenceRequired, booking.DeliveryEvidenceRequired, booking.ProviderTransferRequired,
	)
	if err := row.Scan(&booking.ID, &booking.CreatedAt, &booking.UpdatedAt); err != nil {
		return Booking{}, err
	}
	return booking, nil
}

// InsertIdempotent persists one customer-scoped create operation. A retry with
// the same fingerprint reads the original booking; a changed payload fails
// closed. The booking row itself is the durable operation record, so a timeout
// after INSERT can safely resume the idempotent Wallet/broadcast steps.
// CreatedWithIdempotencyKey reports whether the customer already created a
// booking with this Idempotency-Key.
func (r *PostgresRepository) CreatedWithIdempotencyKey(ctx context.Context, customerID, key string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM dispatch_bookings WHERE customer_id=$1 AND create_idempotency_key=$2)`, customerID, key).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) InsertIdempotent(ctx context.Context, booking Booking, key, fingerprint, quoteID string, quoteVersion int64, quoteFingerprint string) (Booking, bool, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO dispatch_bookings (
			customer_id,status,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,distance_km,fare_amount,currency,
			payment_method,payment_status,receiver_name,receiver_phone,package_desc,
			package_weight,package_type,package_size,is_fragile,service_tier,
			serviceability_quote_id,serviceability_quote_version,serviceability_quote_fingerprint,
			create_idempotency_key,create_request_fingerprint,
			rollout_protection_mode,rollout_policy_version,handover_required,
			pickup_evidence_required,delivery_evidence_required,provider_transfer_required,
			receiver_email,receiver_customer_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,
			NULLIF($22,''),NULLIF($23,0),$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,NULLIF($34,'')::uuid)
		ON CONFLICT (customer_id,create_idempotency_key)
		WHERE create_idempotency_key IS NOT NULL DO NOTHING
		RETURNING id::text,created_at,updated_at`,
		booking.CustomerID, booking.Status, booking.PickupAddress, booking.PickupLat, booking.PickupLng,
		booking.DropoffAddress, booking.DropoffLat, booking.DropoffLng, booking.DistanceKM, booking.FareAmount,
		booking.Currency, booking.PaymentMethod, booking.PaymentStatus, booking.ReceiverName, booking.ReceiverPhone,
		booking.PackageDesc, booking.PackageWeight, booking.PackageType, booking.PackageSize, booking.IsFragile,
		booking.ServiceTier, quoteID, quoteVersion, quoteFingerprint, key, fingerprint,
		booking.RolloutProtectionMode, booking.RolloutPolicyVersion, booking.HandoverRequired,
		booking.PickupEvidenceRequired, booking.DeliveryEvidenceRequired, booking.ProviderTransferRequired,
		booking.ReceiverEmail, booking.ReceiverCustomerID)
	if err := row.Scan(&booking.ID, &booking.CreatedAt, &booking.UpdatedAt); err == nil {
		return booking, false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, false, err
	}

	var storedFingerprint, bookingID string
	if err := r.db.QueryRow(ctx, `SELECT id::text,create_request_fingerprint FROM dispatch_bookings WHERE customer_id=$1 AND create_idempotency_key=$2`, booking.CustomerID, key).Scan(&bookingID, &storedFingerprint); err != nil {
		return Booking{}, false, err
	}
	if storedFingerprint != fingerprint {
		return Booking{}, false, apperrors.Conflict("Idempotency-Key was already used for different booking details.", nil)
	}
	state, found, err := r.GetWithDerivedState(ctx, bookingID)
	if err != nil {
		return Booking{}, false, err
	}
	if !found {
		return Booking{}, false, apperrors.NotFound("Delivery booking not found.", nil)
	}
	return state.Booking, true, nil
}

func (r *PostgresRepository) GetWithDerivedState(ctx context.Context, id string) (DerivedState, bool, error) {
	row := r.db.QueryRow(ctx, derivedStateSelect+` WHERE b.id = $1`, id)
	state, err := scanDerivedState(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DerivedState{}, false, nil
		}
		return DerivedState{}, false, err
	}
	return state, true, nil
}

func (r *PostgresRepository) CommerceMissionScope(ctx context.Context, bookingID string) (string, []string, bool, error) {
	var parentID string
	var vendorIDs []string
	err := r.db.QueryRow(ctx, `SELECT parent_order_ref,vendor_order_refs FROM commerce_missions WHERE booking_id=$1 AND status='accepted'`, bookingID).Scan(&parentID, &vendorIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, false, nil
	}
	return parentID, vendorIDs, err == nil, err
}

func (r *PostgresRepository) ListWithDerivedState(ctx context.Context, customerID string, options ListBookingsOptions) ([]DerivedState, error) {
	limit := options.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.Query(ctx, derivedStateSelect+`
		WHERE b.customer_id = $1
		ORDER BY b.created_at DESC
		LIMIT $2 OFFSET $3`, customerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := []DerivedState{}
	for rows.Next() {
		state, err := scanDerivedState(rows)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (r *PostgresRepository) SetPayment(ctx context.Context, id string, paymentStatus string, paymentRef string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dispatch_bookings
		SET payment_status = $2, payment_ref = $3, updated_at = now()
		WHERE id = $1
	`, id, paymentStatus, paymentRef)
	return err
}

// SetAccepted moves a searching booking to accepted and records the provider.
// Guarded so a late accept event cannot overwrite a terminal booking.
func (r *PostgresRepository) SetAccepted(ctx context.Context, id string, providerID string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_bookings
		SET status = $2, provider_id = $3, updated_at = now()
		WHERE id = $1 AND status = $4
	`, id, StatusAccepted, providerID, StatusSearching)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SaveDropoffRoute stores the road route from pickup to drop-off, fetched once
// at booking creation when both endpoints are already known.
func (r *PostgresRepository) SaveDropoffRoute(ctx context.Context, id, polyline string, distanceM, durationS int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dispatch_bookings
		SET route_dropoff_polyline = $2, route_dropoff_distance_m = $3, route_dropoff_duration_s = $4,
		    route_fetched_at = now(), updated_at = now()
		WHERE id = $1
	`, id, polyline, distanceM, durationS)
	return err
}

// SavePickupRoute stores the road route from the rider to the pickup. It can
// only be fetched once a rider has accepted, since it starts at their position
// at that moment.
func (r *PostgresRepository) SavePickupRoute(ctx context.Context, id, polyline string, distanceM, durationS int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dispatch_bookings
		SET route_pickup_polyline = $2, route_pickup_distance_m = $3, route_pickup_duration_s = $4,
		    route_fetched_at = now(), updated_at = now()
		WHERE id = $1
	`, id, polyline, distanceM, durationS)
	return err
}

func (r *PostgresRepository) SetCompleted(ctx context.Context, id string, completedAt time.Time, paymentStatus string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_bookings
		SET status = $2, completed_at = $3, payment_status = $4, updated_at = now()
		WHERE id = $1 AND status NOT IN ($5, $6)
	`, id, StatusCompleted, completedAt, paymentStatus, StatusCompleted, StatusCancelled)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SetUnmatched marks a searching booking unmatched (no rider found). Strict
// compare-and-set from 'searching' so a late no_provider_found event can never
// clobber a booking a rider already accepted.
func (r *PostgresRepository) SetUnmatched(ctx context.Context, id string, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_bookings
		SET status = $2, cancel_reason = $3, cancelled_at = $4, updated_at = now()
		WHERE id = $1 AND status = $5
	`, id, StatusUnmatched, "no_provider_found", at, StatusSearching)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SetTerminal marks a booking cancelled or unmatched. Guarded against
// double-terminal transitions and against clobbering a completed booking.
func (r *PostgresRepository) SetTerminal(ctx context.Context, id string, status string, paymentStatus string, reason string, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_bookings
		SET status = $2, payment_status = $3, cancel_reason = $4, cancelled_at = $5, updated_at = now()
		WHERE id = $1 AND status NOT IN ($6, $7, $8)
	`, id, status, paymentStatus, reason, at, StatusCompleted, StatusCancelled, StatusUnmatched)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) InsertReview(ctx context.Context, review Review) (Review, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Review{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `
		INSERT INTO ratings (provider_id, booking_id, rated_by_customer_id, score, comment, recommends_provider)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		RETURNING id::text, created_at
	`, review.ProviderID, review.BookingID, review.CustomerID, review.Rating, review.ReviewText, review.RecommendsProvider)
	if err := row.Scan(&review.ID, &review.CreatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Review{}, apperrors.Conflict("This delivery has already been reviewed.", nil)
		}
		return Review{}, err
	}

	// Keep the rider's public avg_rating in sync, exactly like the profile
	// feature's own rating insert does (profile/repository.go).
	if _, err := tx.Exec(ctx, `
		UPDATE providers
		SET avg_rating = COALESCE((
				SELECT ROUND(AVG(score)::numeric, 2)
				FROM ratings
				WHERE provider_id = $1
			), 0.00),
			updated_at = now()
		WHERE id = $1
	`, review.ProviderID); err != nil {
		return Review{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Review{}, err
	}
	return review, nil
}

func (r *PostgresRepository) GetReviewByBookingID(ctx context.Context, bookingID string) (Review, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, booking_id::text, provider_id::text, rated_by_customer_id::text,
			score, COALESCE(comment, ''), recommends_provider, created_at
		FROM ratings
		WHERE booking_id = $1
	`, bookingID)
	var review Review
	if err := row.Scan(
		&review.ID, &review.BookingID, &review.ProviderID, &review.CustomerID,
		&review.Rating, &review.ReviewText, &review.RecommendsProvider, &review.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Review{}, false, nil
		}
		return Review{}, false, err
	}
	return review, true, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDerivedState(row rowScanner) (DerivedState, error) {
	var state DerivedState
	var providerID pgtype.Text
	var packageWeight pgtype.Float8
	var cancelledAt, completedAt pgtype.Timestamptz
	var deliveryCodeExpiresAt pgtype.Timestamptz
	err := row.Scan(
		&state.Booking.ID, &state.Booking.CustomerID, &state.Booking.Status,
		&state.Booking.PickupAddress, &state.Booking.PickupLat, &state.Booking.PickupLng,
		&state.Booking.DropoffAddress, &state.Booking.DropoffLat, &state.Booking.DropoffLng,
		&state.Booking.DistanceKM, &state.Booking.FareAmount, &state.Booking.Currency,
		&state.Booking.PaymentMethod, &state.Booking.PaymentStatus, &state.Booking.PaymentRef,
		&state.Booking.ReceiverName, &state.Booking.ReceiverPhone,
		&state.Booking.PackageDesc, &packageWeight, &state.Booking.PackageType, &state.Booking.PackageSize, &state.Booking.IsFragile,
		&state.Booking.ServiceTier, &state.Booking.ServiceabilityQuoteRef, &state.Booking.ServiceabilityQuoteVersion,
		&state.Booking.RolloutProtectionMode, &state.Booking.RolloutPolicyVersion, &state.Booking.HandoverRequired,
		&state.Booking.PickupEvidenceRequired, &state.Booking.DeliveryEvidenceRequired, &state.Booking.ProviderTransferRequired,
		&providerID,
		&state.Booking.RoutePickupPolyline, &state.Booking.RoutePickupDistanceM, &state.Booking.RoutePickupDurationS,
		&state.Booking.RouteDropoffPolyline, &state.Booking.RouteDropoffDistanceM, &state.Booking.RouteDropoffDurationS,
		&state.Booking.CancelReason,
		&cancelledAt, &completedAt, &state.Booking.CreatedAt, &state.Booking.UpdatedAt,
		&state.BroadcastStatus, &state.TripID, &state.TripStatus, &state.TripProviderID,
		&state.DeliveryCode, &deliveryCodeExpiresAt, &state.DeliveryCodeAttempts,
	)
	if err != nil {
		return DerivedState{}, err
	}
	if deliveryCodeExpiresAt.Valid {
		expires := deliveryCodeExpiresAt.Time
		state.DeliveryCodeExpiresAt = &expires
	}
	if providerID.Valid {
		state.Booking.ProviderID = providerID.String
	}
	if packageWeight.Valid {
		weight := packageWeight.Float64
		state.Booking.PackageWeight = &weight
	}
	if cancelledAt.Valid {
		at := cancelledAt.Time
		state.Booking.CancelledAt = &at
	}
	if completedAt.Valid {
		at := completedAt.Time
		state.Booking.CompletedAt = &at
	}
	return state, nil
}
