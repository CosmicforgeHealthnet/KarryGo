package managedexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/dbtx"
)

// Executor is internal/dbtx's shared Executor, referenced by alias.
//
// It MUST be the same named type the booking and availability repositories
// use, not an identically-shaped local interface: Go checks exact type
// identity of method parameters, so a local copy would stop
// PostgresBookingRepository.MarkMatched from satisfying BookingBinder and
// break the single-transaction guarantee this package depends on. See
// internal/dbtx's own package doc.
type Executor = dbtx.Executor

const reservationColumns = `
	id::text, booking_id::text, source_request_id, fleet_assignment_id,
	provider_id, organization_id, membership_id, acceptance_source, status,
	expected_source_version, source_version, version, idempotency_key,
	request_fingerprint, correlation_id, execution_booking_id::text,
	expires_at, released_at, created_at, updated_at`

// Repository persists execution reservations and the corporate binding on the
// booking itself.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the reservation repository.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// BeginTx starts a transaction for the multi-statement confirm path.
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.BeginTx(ctx, pgx.TxOptions{})
}

// Fingerprint is the stable hash of a reservation/confirm request. Replaying
// the same idempotency key with the same fingerprint returns the original
// answer; replaying it with a different one is a conflict, never a silent
// second reservation.
func Fingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func scanReservation(row pgx.Row) (Reservation, error) {
	var res Reservation
	err := row.Scan(
		&res.ID, &res.BookingID, &res.SourceRequestID, &res.FleetAssignmentID,
		&res.ProviderID, &res.OrganizationID, &res.MembershipID, &res.AcceptanceSource, &res.Status,
		&res.ExpectedSourceVersion, &res.SourceVersion, &res.Version, &res.IdempotencyKey,
		&res.RequestFingerprint, &res.CorrelationID, &res.ExecutionBookingID,
		&res.ExpiresAt, &res.ReleasedAt, &res.CreatedAt, &res.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrNotFound
	}
	return res, err
}

// GetByIdempotencyKey resolves a replay to its original reservation.
func (r *Repository) GetByIdempotencyKey(ctx context.Context, key string) (Reservation, error) {
	return scanReservation(r.db.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM taxi_fleet_execution_reservations WHERE idempotency_key = $1`, key))
}

// GetLiveByBooking returns the booking's live reservation, if any.
func (r *Repository) GetLiveByBooking(ctx context.Context, bookingID string) (Reservation, error) {
	return scanReservation(r.db.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM taxi_fleet_execution_reservations
		  WHERE booking_id = $1::uuid AND status IN ('RESERVED','CONFIRMED')`, bookingID))
}

// GetByID returns one reservation.
func (r *Repository) GetByID(ctx context.Context, id string) (Reservation, error) {
	return scanReservation(r.db.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM taxi_fleet_execution_reservations WHERE id = $1::uuid`, id))
}

// Insert creates a reservation.
//
// The unique indexes do the deciding, not a read-then-write: two concurrent
// Fleet reservations for the same booking race into the same partial unique
// index and exactly one wins. The loser gets a duplicate-key error, which the
// service turns into ALREADY_CLAIMED rather than a retry.
func (r *Repository) Insert(ctx context.Context, res Reservation) (Reservation, error) {
	expires := res.ExpiresAt
	return scanReservation(r.db.QueryRow(ctx, `
		INSERT INTO taxi_fleet_execution_reservations
			(booking_id, source_request_id, fleet_assignment_id, provider_id,
			 organization_id, membership_id, acceptance_source, status,
			 expected_source_version, source_version, idempotency_key,
			 request_fingerprint, correlation_id, expires_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+reservationColumns,
		res.BookingID, res.SourceRequestID, res.FleetAssignmentID, res.ProviderID,
		res.OrganizationID, res.MembershipID, res.AcceptanceSource, StatusReserved,
		res.ExpectedSourceVersion, res.SourceVersion, res.IdempotencyKey,
		res.RequestFingerprint, res.CorrelationID, expires))
}

// Confirm marks a reservation confirmed and binds it to its execution.
//
// Guarded compare-and-set on both status and version: a reservation that was
// released or already confirmed by a concurrent call affects zero rows, which
// is a conflict rather than a silent overwrite.
func (r *Repository) Confirm(ctx context.Context, exec Executor, reservationID, bookingID, providerID string, version int64) (Reservation, error) {
	if exec == nil {
		exec = r.db
	}
	return scanReservation(exec.QueryRow(ctx, `
		UPDATE taxi_fleet_execution_reservations
		   SET status = $1, execution_booking_id = $2::uuid, provider_id = $3,
		       version = version + 1, updated_at = now()
		 WHERE id = $4::uuid AND status = $5 AND version = $6
		RETURNING `+reservationColumns,
		StatusConfirmed, bookingID, providerID, reservationID, StatusReserved, version))
}

// Release ends a reservation with a terminal status.
func (r *Repository) Release(ctx context.Context, reservationID, status string) (Reservation, error) {
	return scanReservation(r.db.QueryRow(ctx, `
		UPDATE taxi_fleet_execution_reservations
		   SET status = $1, released_at = now(), version = version + 1, updated_at = now()
		 WHERE id = $2::uuid AND status IN ('RESERVED','CONFIRMED')
		RETURNING `+reservationColumns, status, reservationID))
}

// ReleaseLiveByBooking releases whatever live reservation a booking holds.
func (r *Repository) ReleaseLiveByBooking(ctx context.Context, bookingID, status string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE taxi_fleet_execution_reservations
		   SET status = $1, released_at = now(), version = version + 1, updated_at = now()
		 WHERE booking_id = $2::uuid AND status IN ('RESERVED','CONFIRMED')`, status, bookingID)
	return err
}

// BindCorporateExecution attaches the corporate identity to the booking and
// moves it into the normal taxi matching lifecycle.
//
// This is TAX-014 in one statement: there is no corporate_taxi_trips table and
// no parallel ladder. The booking keeps its own id, its own status column and
// every existing guard; only metadata is added.
//
// The WHERE clause is the TAX-014A guard: a booking that already carries a
// DIFFERENT Fleet assignment is never rebound, so one taxi execution can never
// be bound to two conflicting assignments.
func (r *Repository) BindCorporateExecution(ctx context.Context, exec Executor, in ConfirmInput, bookingID string) (int64, error) {
	if exec == nil {
		exec = r.db
	}
	var sourceVersion int64
	err := exec.QueryRow(ctx, `
		UPDATE taxi_bookings
		   SET is_corporate_managed = TRUE,
		       organization_id = $1,
		       membership_id = $2,
		       fleet_assignment_id = $3,
		       fleet_assignment_version = $4,
		       source_context = $5,
		       settlement_mode = $6,
		       fleet_correlation_id = NULLIF($7, ''),
		       provider_id = $8::uuid,
		       vehicle_id = COALESCE(NULLIF($9,'')::uuid, vehicle_id),
		       source_version = source_version + 1,
		       updated_at = now()
		 WHERE id = $10::uuid
		   AND (fleet_assignment_id IS NULL OR fleet_assignment_id = $3)
		 RETURNING source_version`,
		in.OrganizationID, in.OrganizationMembershipID, in.FleetAssignmentID,
		in.FleetAssignmentVersion, sourceContextFor(in.AcceptanceSource),
		"ORGANIZATION_MANAGED", in.CorrelationID, in.ExecutingProviderID,
		in.VehicleID, bookingID).Scan(&sourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: booking is bound to a different fleet assignment", ErrIdempotencyConflict)
	}
	return sourceVersion, err
}

// sourceContextFor maps Fleet's acceptance source onto Taxi's own recorded
// acquisition context.
func sourceContextFor(acceptanceSource string) string {
	switch acceptanceSource {
	case "PROVIDER_DIRECT", "PROVIDER":
		return "FLEET_OFFER"
	case "DISPATCHER", "ORGANIZATION":
		return "FLEET_ASSIGNMENT"
	default:
		return "FLEET_PRIVATE_EXECUTION"
	}
}

// ResolveBookingByRequest maps Fleet's source request id onto the taxi booking
// it refers to. Taxi's marketplace request id IS the booking id: a taxi
// booking is the unit of work Fleet offers, so there is no second identifier
// to invent or keep in step.
func (r *Repository) ResolveBookingByRequest(ctx context.Context, sourceRequestID string) (BookingSnapshot, error) {
	var snap BookingSnapshot
	err := r.db.QueryRow(ctx, `
		SELECT id::text, COALESCE(provider_id::text,''), status,
		       COALESCE(organization_id,''), COALESCE(fleet_assignment_id,''),
		       source_version, is_corporate_managed,
		       COALESCE(fare_final_kobo, fare_estimate_kobo, 0),
		       completed_at, cancelled_at
		  FROM taxi_bookings WHERE id = $1::uuid`, sourceRequestID).
		Scan(&snap.BookingID, &snap.ProviderID, &snap.Status,
			&snap.OrganizationID, &snap.FleetAssignmentID,
			&snap.SourceVersion, &snap.IsCorporateManaged,
			&snap.FareKobo, &snap.CompletedAt, &snap.CancelledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookingSnapshot{}, ErrNotFound
	}
	return snap, err
}

// BookingSnapshot is the narrow booking projection the Fleet-facing surface
// needs. Deliberately not the full Booking aggregate: nothing on this surface
// has any business reading a passenger's ride notes or contact details.
type BookingSnapshot struct {
	BookingID          string
	ProviderID         string
	Status             string
	OrganizationID     string
	FleetAssignmentID  string
	SourceVersion      int64
	IsCorporateManaged bool
	FareKobo           int64
	CompletedAt        *time.Time
	CancelledAt        *time.Time
}
