package assignment

import (
	"context"

	bookingfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BookingFinancialAdapter delegates release/refund to the established Booking
// -> Wallet path, then reads back the projection. Dispatch never writes ledger
// state and an unconfirmed Wallet outcome remains UNKNOWN for reconciliation.
type BookingFinancialAdapter struct {
	db       *pgxpool.Pool
	bookings *bookingfeature.Service
}

func NewBookingFinancialAdapter(db *pgxpool.Pool, bookings *bookingfeature.Service) *BookingFinancialAdapter {
	return &BookingFinancialAdapter{db: db, bookings: bookings}
}

func (a *BookingFinancialAdapter) ReleaseForNoProvider(ctx context.Context, bookingID, _ string) (string, error) {
	if a == nil || a.db == nil || a.bookings == nil {
		return "UNKNOWN", ErrConflict
	}
	if err := a.bookings.HandleNoProviderFound(ctx, bookingID); err != nil {
		return "UNKNOWN", err
	}
	var payment string
	if err := a.db.QueryRow(ctx, `SELECT payment_status FROM dispatch_bookings WHERE id=$1`, bookingID).Scan(&payment); err != nil {
		if err == pgx.ErrNoRows {
			return "UNKNOWN", ErrNotFound
		}
		return "UNKNOWN", err
	}
	if payment == "refunded" || payment == "unpaid" {
		return "ACKNOWLEDGED", nil
	}
	return "UNKNOWN", nil
}
