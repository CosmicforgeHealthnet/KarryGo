// Package verification persists the addendum §9 pickup-verification
// handshake: a short-lived code, generated when the driver arrives,
// displayed to the passenger, and submitted by the driver to unlock
// starting the trip. The cryptographic primitives (generate/hash/verify,
// expiry, attempt/resend bookkeeping decisions) all come from
// shared/go/handover -- this package only adds the Postgres persistence
// that package deliberately leaves to each caller.
//
// Direction is the same for both steps and is the opposite of a package
// courier's: the PASSENGER (or, for a parcel, the sender at pickup and the
// receiver at drop-off) displays the code and the DRIVER verifies it. In a
// taxi the customer is what is being collected, so the driver is the party
// who must confirm they have the right person or package.
//
// Rows are keyed by (booking_id, step). A passenger ride only ever has a
// 'pickup' row; a parcel also gets a 'delivery' row once the receiver opens
// their code near drop-off. The table keeps its taxi_pickup_verifications
// name for continuity -- see migration 006.
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

// Record is the persisted shape of one booking's verification for ONE
// handover step.
type Record struct {
	BookingID string
	Step      handover.Step
	handover.CodeRecord
}

// Repository is the persistence boundary for pickup verification records.
type Repository interface {
	// Upsert creates the record on first generation (driver arrival) or
	// replaces it on resend. exec may be nil (uses the repository's own
	// pool) or a pgx.Tx shared with another write -- see dbtx.Executor.
	Upsert(ctx context.Context, exec dbtx.Executor, rec Record) error
	// Get returns the current record for one step, or apperrors.NotFound when
	// no code has been generated for it yet.
	Get(ctx context.Context, bookingID string, step handover.Step) (Record, error)
	// IncrementAttempts is a bare atomic increment -- no read-modify-write
	// race, since concurrent UPDATEs to the same row serialize naturally.
	IncrementAttempts(ctx context.Context, bookingID string, step handover.Step) error
	MarkVerified(ctx context.Context, exec dbtx.Executor, bookingID string, step handover.Step, at time.Time) error
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const recordColumns = `booking_id::text, step, pin_hash, plaintext_pin, expires_at, attempts, resend_count, last_sent_at, verified_at`

func (r *PostgresRepository) Upsert(ctx context.Context, exec dbtx.Executor, rec Record) error {
	// Existing passenger callers predate the explicit two-stage column; their
	// zero value is the original pickup verification operation.
	if rec.Step == "" {
		rec.Step = handover.StepPickup
	}
	if exec == nil {
		exec = r.db
	}
	_, err := exec.Exec(ctx, `
		INSERT INTO taxi_pickup_verifications (booking_id, step, pin_hash, plaintext_pin, expires_at, attempts, resend_count, last_sent_at, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (booking_id, step) DO UPDATE
		   SET pin_hash = $3, plaintext_pin = $4, expires_at = $5,
		       attempts = $6, resend_count = $7, last_sent_at = $8, verified_at = $9
	`, rec.BookingID, string(rec.Step), rec.Hash, rec.Code, rec.ExpiresAt, rec.Attempts, rec.ResendCount, rec.LastSentAt, rec.VerifiedAt)
	return err
}

func (r *PostgresRepository) Get(ctx context.Context, bookingID string, step handover.Step) (Record, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+recordColumns+` FROM taxi_pickup_verifications WHERE booking_id = $1 AND step = $2`,
		bookingID, string(step))
	var rec Record
	var scannedStep string
	err := row.Scan(&rec.BookingID, &scannedStep, &rec.Hash, &rec.Code, &rec.ExpiresAt, &rec.Attempts, &rec.ResendCount, &rec.LastSentAt, &rec.VerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, apperrors.NotFound(notFoundMessage(step), err)
	}
	rec.Step = handover.Step(scannedStep)
	return rec, err
}

// notFoundMessage keeps the absent-record wording truthful per step: "your
// driver has not arrived yet" is simply wrong at drop-off time.
func notFoundMessage(step handover.Step) string {
	if step == handover.StepDelivery {
		return "A delivery code has not been generated yet."
	}
	return "Your driver has not arrived yet."
}

func (r *PostgresRepository) IncrementAttempts(ctx context.Context, bookingID string, step handover.Step) error {
	_, err := r.db.Exec(ctx,
		`UPDATE taxi_pickup_verifications SET attempts = attempts + 1 WHERE booking_id = $1 AND step = $2`,
		bookingID, string(step))
	return err
}

func (r *PostgresRepository) MarkVerified(ctx context.Context, exec dbtx.Executor, bookingID string, step handover.Step, at time.Time) error {
	if exec == nil {
		exec = r.db
	}
	_, err := exec.Exec(ctx,
		`UPDATE taxi_pickup_verifications SET verified_at = $1 WHERE booking_id = $2 AND step = $3`,
		at, bookingID, string(step))
	return err
}
