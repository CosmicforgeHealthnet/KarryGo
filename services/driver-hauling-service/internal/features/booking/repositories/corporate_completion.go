package bookingrepo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/hauling-service/internal/features/completedwork"
)

// IsOrganizationManaged reports whether a booking settles through Fleet
// (HLG-025).
//
// Read from the booking's OWN persisted execution mode, fixed at execution
// start, rather than from a live Fleet lookup at completion time -- see
// bookingusecases.ManagedRouteReader for why that distinction is the whole
// point of the Section 12 boundary.
//
// The organization and membership are checked too: a booking that claims
// managed execution but names neither is not a settlement authority, and must
// not be withheld from the legacy path on that claim alone.
func (r *PostgresBookingRepository) IsOrganizationManaged(ctx context.Context, bookingID string) (bool, error) {
	var managed bool
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(execution_mode,'') = $2
		   AND COALESCE(corporate_organization_id,'') <> ''
		   AND COALESCE(membership_id,'') <> ''
		  FROM haulage_bookings WHERE id = $1::uuid`,
		bookingID, completedwork.ExecutionModeManaged).Scan(&managed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return managed, err
}

// EnqueueCompletedWork writes the canonical Fleet completed-work delivery for
// an authoritative managed completion (HLG-017, HLG-018).
//
// It runs in its own transaction rather than the caller's because Hauling's
// completion transition already committed by the time settlement is routed.
// The ON CONFLICT identity in completedwork.Enqueue is what still makes this
// exactly-once: a repeated call for the same completion version writes one
// row, so a retried or replayed completion cannot produce two Fleet records.
func (r *PostgresBookingRepository) EnqueueCompletedWork(ctx context.Context, bookingID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	binding, err := completedwork.ReadBinding(ctx, tx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if !binding.Managed() {
		return nil
	}
	completedAt, err := completedAtFor(ctx, tx, bookingID)
	if err != nil {
		return err
	}
	if err := completedwork.Enqueue(ctx, tx, bookingID,
		completedwork.RequestFor(bookingID, binding, completedAt)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// completedAtFor reads the booking's authoritative completion timestamp.
//
// The stored completed_at is used rather than "now": a completion delivered
// after a retry must report when the job actually finished, not when the
// delivery happened to succeed.
func completedAtFor(ctx context.Context, tx pgx.Tx, bookingID string) (time.Time, error) {
	var completedAt *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT completed_at FROM haulage_bookings WHERE id = $1::uuid`, bookingID).
		Scan(&completedAt); err != nil {
		return time.Time{}, err
	}
	if completedAt == nil {
		return time.Now().UTC(), nil
	}
	return completedAt.UTC(), nil
}
