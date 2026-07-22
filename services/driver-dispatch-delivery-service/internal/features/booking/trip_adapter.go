package booking

import (
	"context"
	"time"

	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
)

// TripCancelAdapter adapts the trip feature's customer-cancel handler to the
// TripControl interface. It reuses the exact logic the booking.dispatch.cancelled
// subscriber runs, but synchronously, so the caller can verify the outcome.
type TripCancelAdapter struct {
	Trips *tripfeature.Service
}

func (a TripCancelAdapter) CancelForCustomer(ctx context.Context, bookingID, reason string) error {
	if a.Trips == nil {
		return nil
	}
	now := time.Now().UTC()
	return a.Trips.HandleBookingDispatchCancelled(ctx, tripfeature.BookingDispatchCancelledEvent{
		Event:       tripfeature.TopicBookingDispatchCancelled,
		BookingID:   bookingID,
		Reason:      reason,
		CancelledAt: now,
		OccurredAt:  now,
	})
}
