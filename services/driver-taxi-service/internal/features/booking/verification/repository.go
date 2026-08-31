// Package verification persists the addendum §9 pickup-verification
// handshake: a short-lived code, generated when the driver arrives,
// displayed to the passenger, and submitted by the driver to unlock
// starting the trip. The cryptographic primitives (generate/hash/verify,
// expiry, attempt/resend bookkeeping decisions) all come from
// shared/go/handover -- this package only adds the Postgres persistence
// that package deliberately leaves to each caller.
//
// One direction only, per this build's own decision (confirmed with the
// user before building): the PASSENGER displays the code, the DRIVER
// verifies it. A taxi ride carries no package, so unlike hauling/dispatch's
// mirrored two-stage handover, there is no separate drop-off credential.
package verification

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/dbtx"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

// Record is the persisted shape of one booking's pickup verification.
type Record struct {
	BookingID string
	handover.CodeRecord
}

// Repository is the persistence boundary for pickup verification records.
type Repository interface {
	// Upsert creates the record on first generation (driver arrival) or
	// replaces it on resend. exec may be nil (uses the repository's own
	// pool) or a pgx.Tx shared with another write -- see dbtx.Executor.
	Upsert(ctx context.Context, exec dbtx.Executor, rec Record) error
	// Get returns the current record, or apperrors.NotFound if the driver
	// has not arrived yet (no record generated).
	Get(ctx context.Context, bookingID string) (Record, error)
	// IncrementAttempts is a bare atomic increment -- no read-modify-write
	// race, since concurrent UPDATEs to the same row serialize naturally.
	IncrementAttempts(ctx context.Context, bookingID string) error
	MarkVerified(ctx context.Context, exec dbtx.Executor, bookingID string, at time.Time) error
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const recordColumns = `booking_id::text, pin_hash, plaintext_pin, expires_at, attempts, resend_count, last_sent_at, verified_at`

func (r *PostgresRepository) Upsert(ctx context.Context, exec dbtx.Executor, rec Record) error {
	if exec == nil {
		exec = r.db
	}
	_, err := exec.Exec(ctx, `
		INSERT INTO taxi_pickup_verifications (booking_id, pin_hash, plaintext_pin, expires_at, attempts, resend_count, last_sent_at, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (booking_id) DO UPDATE
		   SET pin_hash = $2, plaintext_pin = $3, expires_at = $4,
		       attempts = $5, resend_count = $6, last_sent_at = $7, verified_at = $8
	`, rec.BookingID, rec.Hash, rec.Code, rec.ExpiresAt, rec.Attempts, rec.ResendCount, rec.LastSentAt, rec.VerifiedAt)
	return err
}

func (r *PostgresRepository) Get(ctx context.Context, bookingID string) (Record, error) {
	row := r.db.QueryRow(ctx, `SELECT `+recordColumns+` FROM taxi_pickup_verifications WHERE booking_id = $1`, bookingID)
	var rec Record
	err := row.Scan(&rec.BookingID, &rec.Hash, &rec.Code, &rec.ExpiresAt, &rec.Attempts, &rec.ResendCount, &rec.LastSentAt, &rec.VerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, apperrors.NotFound("Your driver has not arrived yet.", err)
	}
	return rec, err
}

func (r *PostgresRepository) IncrementAttempts(ctx context.Context, bookingID string) error {
	_, err := r.db.Exec(ctx, `UPDATE taxi_pickup_verifications SET attempts = attempts + 1 WHERE booking_id = $1`, bookingID)
	return err
}

func (r *PostgresRepository) MarkVerified(ctx context.Context, exec dbtx.Executor, bookingID string, at time.Time) error {
	if exec == nil {
		exec = r.db
	}
	_, err := exec.Exec(ctx, `UPDATE taxi_pickup_verifications SET verified_at = $1 WHERE booking_id = $2`, at, bookingID)
	return err
}
