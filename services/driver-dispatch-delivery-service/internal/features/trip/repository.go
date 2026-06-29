package trip

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
	CreateTripFromAcceptedRequest(ctx context.Context, input CreateTripInput) (*Trip, error)
	GetTripByID(ctx context.Context, tripID string) (*Trip, error)
	GetTripByBookingID(ctx context.Context, bookingID string) (*Trip, error)
	ListProviderTrips(ctx context.Context, providerID string, options ListTripsOptions) ([]Trip, int, error)
	GetProviderActiveTrip(ctx context.Context, providerID string) (*Trip, error)
	GetProviderTripByID(ctx context.Context, tripID, providerID string) (*Trip, error)
	GetAssignedTripForProvider(ctx context.Context, providerID string) (*Trip, error)
	TransitionTripStatus(ctx context.Context, input TransitionTripInput) (bool, error)
	InsertStateLog(ctx context.Context, input StateLogInput) error
	ListTripStateLog(ctx context.Context, tripID string) ([]TripStateLog, error)
	CreateDeliveryProof(ctx context.Context, input CreateProofInput) (*DeliveryProof, error)
	GetDeliveryProofByTripID(ctx context.Context, tripID string) (*DeliveryProof, error)
	CreateCancellation(ctx context.Context, input CreateCancellationInput) (*Cancellation, error)
	// Phase 7F–7H state-change methods.
	MarkArrived(ctx context.Context, tripID, providerID string, fromStatus TripStatus, now time.Time) (*Trip, error)
	MarkTripStarted(ctx context.Context, tripID, providerID string, now time.Time) (*Trip, error)
	SubmitProofTx(ctx context.Context, input SubmitProofDBInput) (*DeliveryProof, error)
	// Phase 7J–7K state-change methods.
	CompleteTripTx(ctx context.Context, input CompleteTripInput) (*Trip, *DeliveryProof, error)
	CountProviderCancellationsLast30Days(ctx context.Context, providerID string) (int, error)
	CancelTripTx(ctx context.Context, input CancelTripInput) (*Trip, *Cancellation, error)
	// Phase 7L — customer-initiated cancellation (no provider_id scoping).
	CancelTripByCustomerTx(ctx context.Context, input CustomerCancelTripInput) (*Trip, *Cancellation, error)
	InsertCustomerRating(ctx context.Context, tripID, providerID, customerID string, score int, comment *string) (CustomerRating, bool, error)
	GetCustomerRatingByTripID(ctx context.Context, tripID string) (CustomerRating, bool, error)
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateTripFromAcceptedRequest(ctx context.Context, input CreateTripInput) (*Trip, error) {
	row := r.db.QueryRow(ctx, `
		WITH source AS (
			SELECT COALESCE(NULLIF($3, '')::uuid, NULLIF(booking_payload->>'customer_id', '')::uuid) AS customer_id
			FROM request_broadcasts
			WHERE booking_id = $1
		),
		inserted AS (
			INSERT INTO trips (
				booking_id, provider_id, customer_id, status,
				pickup_address, pickup_lat, pickup_lng,
				dropoff_address, dropoff_lat, dropoff_lng,
				distance_km, fare_amount, currency, receiver_name, receiver_phone,
				package_desc, package_weight, package_type, package_size, is_fragile, service_tier
			)
			SELECT $1, $2, source.customer_id, 'assigned',
				$4, $5, $6, $7, $8, $9, $10, $11, COALESCE(NULLIF($12, ''), 'NGN'),
				$13, $14, NULLIF($15, ''), NULLIF($16::numeric, 0),
				NULLIF($17, ''), NULLIF($18, ''), $19, COALESCE(NULLIF($20, ''), 'standard')
			FROM source
			WHERE source.customer_id IS NOT NULL
			ON CONFLICT (booking_id) DO NOTHING
			RETURNING *
		),
		initial_log AS (
			INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_by, notes)
			SELECT id, 'none', 'assigned', 'system', 'created_from_request_accepted'
			FROM inserted
		)
		SELECT `+tripColumns+` FROM inserted
		UNION ALL
		SELECT `+tripColumns+` FROM trips WHERE booking_id = $1
		LIMIT 1
	`, input.BookingID, input.ProviderID, input.CustomerID,
		input.PickupAddress, input.PickupLat, input.PickupLng,
		input.DropoffAddress, input.DropoffLat, input.DropoffLng,
		input.DistanceKM, input.FareAmount, input.Currency,
		input.ReceiverName, input.ReceiverPhone, input.PackageDesc, input.PackageWeight,
		input.PackageType, input.PackageSize, input.IsFragile, input.ServiceTier)
	trip, err := scanTrip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.Internal("Trip could not be created because booking customer data is unavailable.", err)
	}
	return trip, err
}

func (r *PostgresRepository) GetTripByID(ctx context.Context, tripID string) (*Trip, error) {
	return getTrip(r.db.QueryRow(ctx, `SELECT `+tripColumns+` FROM trips WHERE id=$1`, tripID))
}

func (r *PostgresRepository) GetTripByBookingID(ctx context.Context, bookingID string) (*Trip, error) {
	return getTrip(r.db.QueryRow(ctx, `SELECT `+tripColumns+` FROM trips WHERE booking_id=$1`, bookingID))
}

func (r *PostgresRepository) ListProviderTrips(ctx context.Context, providerID string, options ListTripsOptions) ([]Trip, int, error) {
	if options.Limit <= 0 || options.Limit > 50 {
		options.Limit = 20
	}
	countQuery := `SELECT count(*) FROM trips WHERE provider_id=$1`
	query := `SELECT ` + tripColumns + ` FROM trips WHERE provider_id=$1`
	args := []any{providerID}
	countArgs := []any{providerID}
	if options.Status != "" {
		countQuery += ` AND status=$2`
		query += ` AND status=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`
		args = append(args, options.Status, options.Limit, options.Offset)
		countArgs = append(countArgs, options.Status)
	} else {
		query += ` ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = append(args, options.Limit, options.Offset)
	}
	var total int
	if err := r.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	trips := []Trip{}
	for rows.Next() {
		item, err := scanTrip(rows)
		if err != nil {
			return nil, 0, err
		}
		trips = append(trips, *item)
	}
	return trips, total, rows.Err()
}

func (r *PostgresRepository) GetProviderActiveTrip(ctx context.Context, providerID string) (*Trip, error) {
	return getTrip(r.db.QueryRow(ctx, `
		SELECT `+tripColumns+`
		FROM trips
		WHERE provider_id=$1 AND status NOT IN ('completed','cancelled','failed')
		ORDER BY created_at DESC
		LIMIT 1
	`, providerID))
}

func (r *PostgresRepository) GetProviderTripByID(ctx context.Context, tripID, providerID string) (*Trip, error) {
	return getTrip(r.db.QueryRow(ctx, `SELECT `+tripColumns+` FROM trips WHERE id=$1 AND provider_id=$2`, tripID, providerID))
}

func (r *PostgresRepository) GetAssignedTripForProvider(ctx context.Context, providerID string) (*Trip, error) {
	return getTrip(r.db.QueryRow(ctx, `
		SELECT `+tripColumns+`
		FROM trips
		WHERE provider_id=$1 AND status='assigned'
		ORDER BY created_at DESC
		LIMIT 1
	`, providerID))
}

func (r *PostgresRepository) TransitionTripStatus(ctx context.Context, input TransitionTripInput) (bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx, `
		UPDATE trips
		SET status=$3, updated_at=$4
		WHERE id=$1 AND status=$2
	`, input.TripID, input.FromStatus, input.ToStatus, input.ChangedAt)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''))
	`, input.TripID, input.FromStatus, input.ToStatus, input.ChangedAt, input.ChangedBy, input.Notes); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *PostgresRepository) InsertStateLog(ctx context.Context, input StateLogInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_by, notes)
		VALUES ($1,$2,$3,$4,NULLIF($5,''))
	`, input.TripID, input.FromStatus, input.ToStatus, input.ChangedBy, input.Notes)
	return err
}

func (r *PostgresRepository) ListTripStateLog(ctx context.Context, tripID string) ([]TripStateLog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, trip_id::text, from_status, to_status, changed_at, changed_by, notes
		FROM trip_state_log
		WHERE trip_id=$1
		ORDER BY changed_at ASC, id ASC
	`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []TripStateLog{}
	for rows.Next() {
		var item TripStateLog
		var toStatus, changedBy string
		var notes pgtype.Text
		if err := rows.Scan(&item.ID, &item.TripID, &item.FromStatus, &toStatus, &item.ChangedAt, &changedBy, &notes); err != nil {
			return nil, err
		}
		item.ToStatus = TripStatus(toStatus)
		item.ChangedBy = CancelledBy(changedBy)
		if notes.Valid {
			item.Notes = &notes.String
		}
		logs = append(logs, item)
	}
	return logs, rows.Err()
}

func (r *PostgresRepository) CreateDeliveryProof(ctx context.Context, input CreateProofInput) (*DeliveryProof, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO delivery_proofs (trip_id, photo_ref, signature_ref, receiver_name, receiver_phone)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id::text, trip_id::text, photo_ref, signature_ref, receiver_name,
			receiver_phone, submitted_at, verified, verified_at
	`, input.TripID, input.PhotoRef, input.SignatureRef, input.ReceiverName, input.ReceiverPhone)
	proof, err := scanProof(row)
	if isUniqueViolation(err) {
		return nil, apperrors.Conflict("Proof has already been submitted for this trip.", err)
	}
	return proof, err
}

func (r *PostgresRepository) GetDeliveryProofByTripID(ctx context.Context, tripID string) (*DeliveryProof, error) {
	return getProof(r.db.QueryRow(ctx, `
		SELECT id::text, trip_id::text, photo_ref, signature_ref, receiver_name,
			receiver_phone, submitted_at, verified, verified_at
		FROM delivery_proofs WHERE trip_id=$1
	`, tripID))
}

func (r *PostgresRepository) CreateCancellation(ctx context.Context, input CreateCancellationInput) (*Cancellation, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO cancellations (trip_id, cancelled_by, reason_code, reason_text)
		VALUES ($1,$2,$3,NULLIF($4,''))
		RETURNING id::text, trip_id::text, cancelled_by, reason_code, reason_text,
			penalty_applied, cancelled_at
	`, input.TripID, input.CancelledBy, input.ReasonCode, input.ReasonText)
	cancellation, err := scanCancellation(row)
	if isUniqueViolation(err) {
		return nil, apperrors.Conflict("This trip already has a cancellation record.", err)
	}
	return cancellation, err
}

// CompleteTripTx verifies proof, completes the trip, and inserts a state log — all in one transaction (Phase 7J).
// Returns (nil, nil, nil) when the trip was not in proof_submitted state (race condition); caller returns 409.
func (r *PostgresRepository) CompleteTripTx(ctx context.Context, input CompleteTripInput) (*Trip, *DeliveryProof, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Mark proof as verified.
	if _, err := tx.Exec(ctx, `
		UPDATE delivery_proofs SET verified=true, verified_at=$2 WHERE trip_id=$1
	`, input.TripID, input.Now); err != nil {
		return nil, nil, err
	}
	// Update trip to completed.
	row := tx.QueryRow(ctx, `
		UPDATE trips SET status='completed', completed_at=$3, updated_at=$3
		WHERE id=$1 AND provider_id=$2 AND status='proof_submitted'
		RETURNING `+tripColumns,
		input.TripID, input.ProviderID, input.Now)
	trip, err := scanTrip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	// Insert state log.
	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,'proof_submitted','completed',$2,'provider','provider_completed_delivery')
	`, input.TripID, input.Now); err != nil {
		return nil, nil, err
	}
	// Fetch updated proof.
	proofRow := tx.QueryRow(ctx, `
		SELECT id::text, trip_id::text, photo_ref, signature_ref, receiver_name,
			receiver_phone, submitted_at, verified, verified_at
		FROM delivery_proofs WHERE trip_id=$1
	`, input.TripID)
	proof, err := scanProof(proofRow)
	if err != nil {
		return nil, nil, err
	}
	return trip, proof, tx.Commit(ctx)
}

// CountProviderCancellationsLast30Days returns the number of provider-initiated cancellations
// in the past 30 days for the given provider (Phase 7K).
func (r *PostgresRepository) CountProviderCancellationsLast30Days(ctx context.Context, providerID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM cancellations c
		JOIN trips t ON t.id = c.trip_id
		WHERE t.provider_id = $1
		AND c.cancelled_by = 'provider'
		AND c.cancelled_at >= now() - INTERVAL '30 days'
	`, providerID).Scan(&count)
	return count, err
}

// CancelTripTx cancels the trip, inserts the cancellation record, and inserts a state log — all
// in one transaction (Phase 7K). Returns (nil, nil, nil) when the trip status prevented cancellation.
func (r *PostgresRepository) CancelTripTx(ctx context.Context, input CancelTripInput) (*Trip, *Cancellation, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `
		UPDATE trips SET status='cancelled', cancelled_at=$3, updated_at=$3
		WHERE id=$1 AND provider_id=$2 AND status IN ('assigned','en_route_pickup','arrived_pickup','in_progress')
		RETURNING `+tripColumns,
		input.TripID, input.ProviderID, input.Now)
	trip, err := scanTrip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,$2,'cancelled',$3,'provider',NULLIF($4,''))
	`, input.TripID, string(input.FromStatus), input.Now, input.ReasonCode); err != nil {
		return nil, nil, err
	}
	cancelRow := tx.QueryRow(ctx, `
		INSERT INTO cancellations (trip_id, cancelled_by, reason_code, reason_text, penalty_applied)
		VALUES ($1,'provider',$2,NULLIF($3,''),$4)
		RETURNING id::text, trip_id::text, cancelled_by, reason_code, reason_text, penalty_applied, cancelled_at
	`, input.TripID, input.ReasonCode, input.ReasonText, input.PenaltyApplied)
	cancellation, err := scanCancellation(cancelRow)
	if isUniqueViolation(err) {
		return nil, nil, apperrors.Conflict("This trip has already been cancelled.", err)
	}
	if err != nil {
		return nil, nil, err
	}
	return trip, cancellation, tx.Commit(ctx)
}

// CancelTripByCustomerTx cancels a trip initiated by the customer (no provider_id scoping).
// Only assigned/en_route_pickup/arrived_pickup trips are eligible (Phase 7L).
// Returns (nil, nil, nil) when the trip was not in a cancellable state (race condition or ineligible).
func (r *PostgresRepository) CancelTripByCustomerTx(ctx context.Context, input CustomerCancelTripInput) (*Trip, *Cancellation, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `
		UPDATE trips SET status='cancelled', cancelled_at=$2, updated_at=$2
		WHERE id=$1 AND status IN ('assigned','en_route_pickup','arrived_pickup')
		RETURNING `+tripColumns,
		input.TripID, input.Now)
	trip, err := scanTrip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,$2,'cancelled',$3,'customer','customer_cancelled')
	`, input.TripID, string(input.FromStatus), input.Now); err != nil {
		return nil, nil, err
	}
	cancelRow := tx.QueryRow(ctx, `
		INSERT INTO cancellations (trip_id, cancelled_by, reason_code, reason_text, penalty_applied)
		VALUES ($1,'customer','customer_cancelled',NULLIF($2,''),false)
		RETURNING id::text, trip_id::text, cancelled_by, reason_code, reason_text, penalty_applied, cancelled_at
	`, input.TripID, input.ReasonText)
	cancellation, err := scanCancellation(cancelRow)
	if isUniqueViolation(err) {
		return nil, nil, apperrors.Conflict("This trip has already been cancelled.", err)
	}
	if err != nil {
		return nil, nil, err
	}
	return trip, cancellation, tx.Commit(ctx)
}

// MarkArrived transitions the trip to arrived_pickup and sets arrived_at in one DB transaction (Phase 7F).
// Returns nil (*Trip, nil) when no rows matched (race condition); caller returns 409.
func (r *PostgresRepository) MarkArrived(ctx context.Context, tripID, providerID string, fromStatus TripStatus, now time.Time) (*Trip, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `
		UPDATE trips SET status='arrived_pickup', arrived_at=$3, updated_at=$3
		WHERE id=$1 AND provider_id=$2 AND status IN ('assigned','en_route_pickup')
		RETURNING `+tripColumns,
		tripID, providerID, now)
	trip, err := scanTrip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,$2,'arrived_pickup',$3,'provider','provider_arrived_pickup')
	`, tripID, string(fromStatus), now); err != nil {
		return nil, err
	}
	return trip, tx.Commit(ctx)
}

// MarkTripStarted transitions the trip to in_progress and sets started_at in one DB transaction (Phase 7G).
// Returns nil (*Trip, nil) when no rows matched (race condition); caller returns 409.
func (r *PostgresRepository) MarkTripStarted(ctx context.Context, tripID, providerID string, now time.Time) (*Trip, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `
		UPDATE trips SET status='in_progress', started_at=$3, updated_at=$3
		WHERE id=$1 AND provider_id=$2 AND status='arrived_pickup'
		RETURNING `+tripColumns,
		tripID, providerID, now)
	trip, err := scanTrip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,'arrived_pickup','in_progress',$2,'provider','provider_started_delivery')
	`, tripID, now); err != nil {
		return nil, err
	}
	return trip, tx.Commit(ctx)
}

// SubmitProofTx inserts delivery_proofs, updates trip status, and inserts state log atomically (Phase 7H).
func (r *PostgresRepository) SubmitProofTx(ctx context.Context, input SubmitProofDBInput) (*DeliveryProof, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	proofRow := tx.QueryRow(ctx, `
		INSERT INTO delivery_proofs (trip_id, photo_ref, signature_ref, receiver_name, receiver_phone)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id::text, trip_id::text, photo_ref, signature_ref, receiver_name,
			receiver_phone, submitted_at, verified, verified_at
	`, input.TripID, input.PhotoRef, input.SignatureRef, input.ReceiverName, input.ReceiverPhone)
	proof, err := scanProof(proofRow)
	if isUniqueViolation(err) {
		return nil, apperrors.Conflict("Proof has already been submitted for this trip.", err)
	}
	if err != nil {
		return nil, err
	}

	tag, err := tx.Exec(ctx, `
		UPDATE trips SET status='proof_submitted', updated_at=$3
		WHERE id=$1 AND provider_id=$2 AND status='in_progress'
	`, input.TripID, input.ProviderID, input.Now)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, apperrors.Conflict("Trip is not in the expected state for proof submission.", nil)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,'in_progress','proof_submitted',$2,'provider','proof_submitted')
	`, input.TripID, input.Now); err != nil {
		return nil, err
	}

	return proof, tx.Commit(ctx)
}

const tripColumns = `
	id::text, booking_id::text, provider_id::text, customer_id::text, status,
	pickup_address, pickup_lat::float8, pickup_lng::float8,
	dropoff_address, dropoff_lat::float8, dropoff_lng::float8,
	distance_km::float8, fare_amount, currency, receiver_name, receiver_phone,
	package_desc, package_weight::float8, package_type, package_size, is_fragile, service_tier,
	started_at, arrived_at, completed_at,
	cancelled_at, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func getTrip(row rowScanner) (*Trip, error) {
	trip, err := scanTrip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return trip, err
}

func scanTrip(row rowScanner) (*Trip, error) {
	var result Trip
	var status string
	var packageDesc, packageType, packageSize, serviceTier pgtype.Text
	var packageWeight pgtype.Float8
	var startedAt, arrivedAt, completedAt, cancelledAt pgtype.Timestamptz
	if err := row.Scan(
		&result.ID, &result.BookingID, &result.ProviderID, &result.CustomerID, &status,
		&result.PickupAddress, &result.PickupLat, &result.PickupLng,
		&result.DropoffAddress, &result.DropoffLat, &result.DropoffLng,
		&result.DistanceKM, &result.FareAmount, &result.Currency, &result.ReceiverName,
		&result.ReceiverPhone, &packageDesc, &packageWeight, &packageType, &packageSize, &result.IsFragile, &serviceTier,
		&startedAt, &arrivedAt, &completedAt, &cancelledAt, &result.CreatedAt, &result.UpdatedAt,
	); err != nil {
		return nil, err
	}
	result.Status = TripStatus(status)
	if packageDesc.Valid {
		result.PackageDesc = &packageDesc.String
	}
	if packageWeight.Valid {
		result.PackageWeight = &packageWeight.Float64
	}
	if packageType.Valid {
		result.PackageType = &packageType.String
	}
	if packageSize.Valid {
		result.PackageSize = &packageSize.String
	}
	if serviceTier.Valid && serviceTier.String != "" {
		result.ServiceTier = serviceTier.String
	} else {
		result.ServiceTier = "standard"
	}
	setTripTime(&result.StartedAt, startedAt)
	setTripTime(&result.ArrivedAt, arrivedAt)
	setTripTime(&result.CompletedAt, completedAt)
	setTripTime(&result.CancelledAt, cancelledAt)
	return &result, nil
}

func getProof(row rowScanner) (*DeliveryProof, error) {
	proof, err := scanProof(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return proof, err
}

func scanProof(row rowScanner) (*DeliveryProof, error) {
	var proof DeliveryProof
	var verifiedAt pgtype.Timestamptz
	if err := row.Scan(&proof.ID, &proof.TripID, &proof.PhotoRef, &proof.SignatureRef,
		&proof.ReceiverName, &proof.ReceiverPhone, &proof.SubmittedAt, &proof.Verified, &verifiedAt); err != nil {
		return nil, err
	}
	if verifiedAt.Valid {
		proof.VerifiedAt = &verifiedAt.Time
	}
	return &proof, nil
}

func scanCancellation(row rowScanner) (*Cancellation, error) {
	var cancellation Cancellation
	var cancelledBy string
	var reasonText pgtype.Text
	if err := row.Scan(&cancellation.ID, &cancellation.TripID, &cancelledBy,
		&cancellation.ReasonCode, &reasonText, &cancellation.PenaltyApplied,
		&cancellation.CancelledAt); err != nil {
		return nil, err
	}
	cancellation.CancelledBy = CancelledBy(cancelledBy)
	if reasonText.Valid {
		cancellation.ReasonText = &reasonText.String
	}
	return &cancellation, nil
}

func setTripTime(target **time.Time, value pgtype.Timestamptz) {
	if value.Valid {
		*target = &value.Time
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *PostgresRepository) InsertCustomerRating(ctx context.Context, tripID, providerID, customerID string, score int, comment *string) (CustomerRating, bool, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO customer_ratings_by_provider (trip_id, provider_id, customer_id, score, comment)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (trip_id) DO NOTHING
		RETURNING id::text, trip_id::text, provider_id::text, customer_id::text, score, comment, created_at
	`, tripID, providerID, customerID, score, comment)

	rating, err := scanCustomerRating(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomerRating{}, false, nil
	}
	if err != nil {
		return CustomerRating{}, false, err
	}
	return rating, true, nil
}

func (r *PostgresRepository) GetCustomerRatingByTripID(ctx context.Context, tripID string) (CustomerRating, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, trip_id::text, provider_id::text, customer_id::text, score, comment, created_at
		FROM customer_ratings_by_provider
		WHERE trip_id = $1
	`, tripID)
	rating, err := scanCustomerRating(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomerRating{}, false, nil
	}
	if err != nil {
		return CustomerRating{}, false, err
	}
	return rating, true, nil
}

func scanCustomerRating(row interface{ Scan(dest ...any) error }) (CustomerRating, error) {
	var r CustomerRating
	var comment pgtype.Text
	err := row.Scan(&r.ID, &r.TripID, &r.ProviderID, &r.CustomerID, &r.Score, &comment, &r.CreatedAt)
	if err != nil {
		return CustomerRating{}, err
	}
	if comment.Valid {
		r.Comment = &comment.String
	}
	return r, nil
}
