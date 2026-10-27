package bookingrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
)

// ErrNoWaiting means a booking has no waiting record yet (the driver has not
// arrived anywhere).
var ErrNoWaiting = errors.New("bookingrepo: no waiting record")

// WaitingRepository stores waiting-time rates and each booking's waiting.
type WaitingRepository struct{ db *pgxpool.Pool }

// NewWaitingRepository returns a repository over db.
func NewWaitingRepository(db *pgxpool.Pool) *WaitingRepository { return &WaitingRepository{db: db} }

// LatestPolicy returns the most recent rates an admin set, if any.
func (r *WaitingRepository) LatestPolicy(ctx context.Context) (bookingmodels.WaitingPolicy, bool, error) {
	var p bookingmodels.WaitingPolicy
	err := r.db.QueryRow(ctx, `
		SELECT id, free_minutes, block_minutes, block_fee_kobo, created_by, reason, created_at
		FROM hauling_waiting_policies ORDER BY created_at DESC LIMIT 1`).
		Scan(&p.ID, &p.FreeMinutes, &p.BlockMinutes, &p.BlockFeeKobo, &p.CreatedBy, &p.Reason, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.WaitingPolicy{}, false, nil
	}
	if err != nil {
		return bookingmodels.WaitingPolicy{}, false, fmt.Errorf("waiting: latest policy: %w", err)
	}
	return p, true, nil
}

// InsertPolicy records new rates. Rates are never edited.
func (r *WaitingRepository) InsertPolicy(ctx context.Context, p bookingmodels.WaitingPolicy) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO hauling_waiting_policies (id, free_minutes, block_minutes, block_fee_kobo, created_by, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.ID, p.FreeMinutes, p.BlockMinutes, p.BlockFeeKobo, p.CreatedBy, p.Reason, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("waiting: insert policy: %w", err)
	}
	return nil
}

// MarkArrived records the driver's first arrival at a stop. The booking's rates
// are copied from policy the first time the driver arrives anywhere; a repeated
// arrival keeps the first time, so a double tap cannot restart the clock.
func (r *WaitingRepository) MarkArrived(ctx context.Context, bookingID, stop string, at time.Time, policy bookingmodels.WaitingPolicy) error {
	column, err := arrivedColumn(stop)
	if err != nil {
		return err
	}
	var policyID any
	if policy.ID != "" {
		policyID = policy.ID
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO hauling_booking_waiting (booking_id, policy_id, free_minutes, block_minutes, block_fee_kobo, `+column+`)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (booking_id) DO UPDATE
		SET `+column+` = COALESCE(hauling_booking_waiting.`+column+`, EXCLUDED.`+column+`),
		    updated_at = now()`,
		bookingID, policyID, policy.FreeMinutes, policy.BlockMinutes, policy.BlockFeeKobo, at)
	if err != nil {
		return fmt.Errorf("waiting: mark arrived: %w", err)
	}
	return nil
}

// MarkLeft records when the driver left a stop they had arrived at. It keeps
// the first departure and never records one for a stop with no arrival.
func (r *WaitingRepository) MarkLeft(ctx context.Context, bookingID, stop string, at time.Time) error {
	arrived, err := arrivedColumn(stop)
	if err != nil {
		return err
	}
	left := map[string]string{bookingmodels.WaitingStopPickup: "pickup_left_at", bookingmodels.WaitingStopDropoff: "dropoff_left_at"}[stop]
	_, err = r.db.Exec(ctx, `
		UPDATE hauling_booking_waiting
		SET `+left+` = GREATEST($2, `+arrived+`), updated_at = now()
		WHERE booking_id = $1 AND `+arrived+` IS NOT NULL AND `+left+` IS NULL`, bookingID, at)
	if err != nil {
		return fmt.Errorf("waiting: mark left: %w", err)
	}
	return nil
}

// Get returns a booking's waiting record.
func (r *WaitingRepository) Get(ctx context.Context, bookingID string) (bookingmodels.BookingWaiting, error) {
	var w bookingmodels.BookingWaiting
	var policyID, reference *string
	err := r.db.QueryRow(ctx, `
		SELECT booking_id, policy_id, free_minutes, block_minutes, block_fee_kobo,
		       pickup_arrived_at, pickup_left_at, dropoff_arrived_at, dropoff_left_at,
		       fee_kobo, fee_status, fee_reference
		FROM hauling_booking_waiting WHERE booking_id = $1`, bookingID).
		Scan(&w.BookingID, &policyID, &w.Policy.FreeMinutes, &w.Policy.BlockMinutes, &w.Policy.BlockFeeKobo,
			&w.PickupArrivedAt, &w.PickupLeftAt, &w.DropoffArrivedAt, &w.DropoffLeftAt,
			&w.FeeKobo, &w.FeeStatus, &reference)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.BookingWaiting{}, ErrNoWaiting
	}
	if err != nil {
		return bookingmodels.BookingWaiting{}, fmt.Errorf("waiting: get: %w", err)
	}
	if policyID != nil {
		w.Policy.ID = *policyID
	}
	if reference != nil {
		w.FeeReference = *reference
	}
	return w, nil
}

// SettleFee records the fee's outcome, once: only a PENDING fee moves, so a
// retry cannot charge twice or overwrite a recorded outcome. It reports
// whether this call made the change.
func (r *WaitingRepository) SettleFee(ctx context.Context, bookingID string, feeKobo int64, status, reference string) (bool, error) {
	var ref any
	if reference != "" {
		ref = reference
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE hauling_booking_waiting
		SET fee_kobo = $2, fee_status = $3, fee_reference = $4, updated_at = now()
		WHERE booking_id = $1 AND fee_status = 'PENDING'`, bookingID, feeKobo, status, ref)
	if err != nil {
		return false, fmt.Errorf("waiting: settle fee: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func arrivedColumn(stop string) (string, error) {
	switch stop {
	case bookingmodels.WaitingStopPickup:
		return "pickup_arrived_at", nil
	case bookingmodels.WaitingStopDropoff:
		return "dropoff_arrived_at", nil
	}
	return "", fmt.Errorf("waiting: unknown stop %q", stop)
}
