package booking

import (
	"context"
	"time"

	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	"cosmicforge/logistics/shared/go/handover"
)

// TripCancelAdapter adapts the trip feature's customer-cancel handler and the
// customer-facing side of handover verification to the TripControl
// interface. It reuses the exact logic the booking.dispatch.cancelled
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

func (a TripCancelAdapter) VerifyPickupCode(ctx context.Context, bookingID, code string) error {
	if a.Trips == nil {
		return nil
	}
	return a.Trips.VerifyPickupCodeForCustomer(ctx, bookingID, code)
}

func (a TripCancelAdapter) ResendDeliveryCode(ctx context.Context, bookingID string) (handover.CodeRecord, error) {
	if a.Trips == nil {
		return handover.CodeRecord{}, nil
	}
	return a.Trips.ResendDeliveryCodeForCustomer(ctx, bookingID)
}
