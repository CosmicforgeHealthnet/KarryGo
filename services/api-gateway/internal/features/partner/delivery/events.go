package delivery

import (
	"encoding/json"
	"time"
)

// Dispatch event topics. These strings and payload shapes are duplicated here
// (not imported) from driver-dispatch-delivery-service because cross-service
// internal packages must not be imported. They MUST stay in sync with
// services/driver-dispatch-delivery-service/internal/features/request/model.go.
const (
	TopicBookingDispatchCreated   = "booking.dispatch.created"
	TopicBookingDispatchCancelled = "booking.dispatch.cancelled"
)

// bookingDispatchCreatedEvent mirrors request.BookingDispatchCreatedEvent.
type bookingDispatchCreatedEvent struct {
	CorrelationID  string    `json:"correlation_id,omitempty"`
	BookingID      string    `json:"booking_id"`
	CustomerID     string    `json:"customer_id"`
	PickupLat      float64   `json:"pickup_lat"`
	PickupLng      float64   `json:"pickup_lng"`
	DropoffLat     float64   `json:"dropoff_lat"`
	DropoffLng     float64   `json:"dropoff_lng"`
	PickupAddress  string    `json:"pickup_address"`
	DropoffAddress string    `json:"dropoff_address"`
	FareAmount     int64     `json:"fare_amount"`
	ServiceType    string    `json:"service_type"`
	PaymentMethod  string    `json:"payment_method"`
	Currency       string    `json:"currency"`
	ReceiverName   string    `json:"receiver_name"`
	ReceiverPhone  string    `json:"receiver_phone"`
	PackageType    string    `json:"package_type"`
	PackageSize    string    `json:"package_size"`
	IsFragile      bool      `json:"is_fragile"`
	ServiceTier    string    `json:"service_tier"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// bookingDispatchCancelledEvent mirrors request.BookingDispatchCancelledEvent.
type bookingDispatchCancelledEvent struct {
	Event       string    `json:"event"`
	BookingID   string    `json:"booking_id"`
	Reason      string    `json:"reason"`
	CancelledAt time.Time `json:"cancelled_at"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// buildDispatchCreatedPayload builds the booking.dispatch.created JSON. The
// event's booking_id is the dispatch booking id (Option A source reference),
// and customer_id is the end-customer id from the request.
func buildDispatchCreatedPayload(b *PartnerBooking, dispatchBookingID, customerID, paymentMethod string, now time.Time) ([]byte, error) {
	ev := bookingDispatchCreatedEvent{
		BookingID:      dispatchBookingID,
		CustomerID:     customerID,
		PickupLat:      b.PickupLat,
		PickupLng:      b.PickupLng,
		DropoffLat:     b.DestLat,
		DropoffLng:     b.DestLng,
		PickupAddress:  b.PickupAddress,
		DropoffAddress: b.DestAddress,
		FareAmount:     b.FareAmountKobo,
		ServiceType:    "dispatch",
		PaymentMethod:  paymentMethod,
		Currency:       currencyNGN,
		ReceiverName:   b.ReceiverName,
		ReceiverPhone:  b.ReceiverPhone,
		PackageType:    deref(b.PackageType),
		PackageSize:    deref(b.PackageSize),
		IsFragile:      b.IsFragile,
		ServiceTier:    b.ServiceTier,
		OccurredAt:     now.UTC(),
	}
	return json.Marshal(ev)
}

func buildDispatchCancelledPayload(dispatchBookingID, reason string, now time.Time) ([]byte, error) {
	ev := bookingDispatchCancelledEvent{
		Event:       TopicBookingDispatchCancelled,
		BookingID:   dispatchBookingID,
		Reason:      reason,
		CancelledAt: now.UTC(),
		OccurredAt:  now.UTC(),
	}
	return json.Marshal(ev)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
