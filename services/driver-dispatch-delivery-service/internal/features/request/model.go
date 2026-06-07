package request

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

type BroadcastStatus string

const (
	BroadcastStatusBroadcasting    BroadcastStatus = "broadcasting"
	BroadcastStatusAccepted        BroadcastStatus = "accepted"
	BroadcastStatusExpired         BroadcastStatus = "expired"
	BroadcastStatusCancelled       BroadcastStatus = "cancelled"
	BroadcastStatusNoProviderFound BroadcastStatus = "no_provider_found"
)

type InboxStatus string

const (
	InboxStatusPending  InboxStatus = "pending"
	InboxStatusAccepted InboxStatus = "accepted"
	InboxStatusRejected InboxStatus = "rejected"
	InboxStatusExpired  InboxStatus = "expired"
)

const (
	TopicBookingDispatchCreated   = "booking.dispatch.created"
	TopicBookingDispatchCancelled = "booking.dispatch.cancelled"
	TopicRequestAccepted          = "request.accepted"
	TopicRequestRejected          = "request.rejected"
	TopicNoProviderFound          = "request.no_provider_found"

	AcceptLockTTL     = 10 * time.Second
	AcceptedMarkerTTL = 24 * time.Hour

	// Accept rate limit: 5 per 10 s per provider (Phase 6K).
	AcceptRateLimitMax    = 5
	AcceptRateLimitWindow = 10 * time.Second

	// Reject rate limit: 10 per 60 s per provider (Phase 6K).
	RejectRateLimitMax    = 10
	RejectRateLimitWindow = 60 * time.Second
)

func RequestLockKey(bookingID string) string {
	return "request:lock:" + bookingID
}

func RequestAcceptedKey(bookingID string) string {
	return "request:accepted:" + bookingID
}

func RequestBroadcastingKey(bookingID string) string {
	return "request:broadcasting:" + bookingID
}

func AcceptRateLimitKey(providerID string) string {
	return "request:ratelimit:accept:" + providerID
}

func RejectRateLimitKey(providerID string) string {
	return "request:ratelimit:reject:" + providerID
}

type RequestBroadcast struct {
	ID                   string          `json:"id"`
	BookingID            string          `json:"booking_id"`
	ServiceType          string          `json:"service_type"`
	BroadcastRadiusKM    float64         `json:"broadcast_radius_km"`
	AttemptNumber        int             `json:"attempt_number"`
	ProvidersNotified    int             `json:"providers_notified"`
	Status               BroadcastStatus `json:"status"`
	BroadcastAt          time.Time       `json:"broadcast_at"`
	ExpiresAt            time.Time       `json:"expires_at"`
	AcceptedByProviderID *string         `json:"accepted_by_provider_id,omitempty"`
	BookingPayload       json.RawMessage `json:"booking_payload"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type ProviderRequestInbox struct {
	ID             string          `json:"id"`
	BroadcastID    string          `json:"broadcast_id"`
	BookingID      string          `json:"booking_id"`
	ProviderID     string          `json:"provider_id"`
	Status         InboxStatus     `json:"status"`
	ReceivedAt     time.Time       `json:"received_at"`
	RespondedAt    *time.Time      `json:"responded_at,omitempty"`
	FCMSent        bool            `json:"fcm_sent"`
	FCMSentAt      *time.Time      `json:"fcm_sent_at,omitempty"`
	ExpiresAt      time.Time       `json:"expires_at"`
	BookingPayload json.RawMessage `json:"booking_payload"`
}

type CreateBroadcastInput struct {
	BookingID         string
	ServiceType       string
	RadiusKM          float64
	Attempt           int
	ProvidersNotified int
	BroadcastAt       time.Time
	ExpiresAt         time.Time
	BookingPayload    json.RawMessage
}

type ListInboxOptions struct {
	Status InboxStatus
	Limit  int
	Offset int
}

// BookingDispatchCancelledEvent is the incoming event payload for booking.dispatch.cancelled (Phase 6I).
type BookingDispatchCancelledEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	BookingID     string    `json:"booking_id"`
	Reason        string    `json:"reason"`
	CancelledAt   time.Time `json:"cancelled_at"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type BookingDispatchCreatedEvent struct {
	CorrelationID  string          `json:"correlation_id,omitempty"`
	BookingID      string          `json:"booking_id"`
	CustomerID     string          `json:"customer_id"`
	PickupLat      float64         `json:"pickup_lat"`
	PickupLng      float64         `json:"pickup_lng"`
	DropoffLat     float64         `json:"dropoff_lat"`
	DropoffLng     float64         `json:"dropoff_lng"`
	PickupAddress  string          `json:"pickup_address"`
	DropoffAddress string          `json:"dropoff_address"`
	FareAmount     int64           `json:"fare_amount"`
	ServiceType    string          `json:"service_type"`
	PaymentMethod  string          `json:"payment_method"`
	Currency       string          `json:"currency"`
	ReceiverName   string          `json:"receiver_name"`
	ReceiverPhone  string          `json:"receiver_phone"`
	PackageDesc    string          `json:"package_desc"`
	PackageWeight  float64         `json:"package_weight"`
	BookingPayload json.RawMessage `json:"booking_payload"`
	OccurredAt     time.Time       `json:"occurred_at"`
}

type NearbyProvider struct {
	ProviderID string  `json:"provider_id"`
	DistanceKM float64 `json:"distance_km"`
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
}

type NearbyResponse struct {
	Providers []NearbyProvider `json:"providers"`
	Count     int              `json:"count"`
	RadiusKM  float64          `json:"radius_km"`
}

type RequestPushPayload struct {
	InboxID        string `json:"inbox_id"`
	BroadcastID    string `json:"broadcast_id"`
	BookingID      string `json:"booking_id"`
	FareAmount     int64  `json:"fare_amount"`
	PickupAddress  string `json:"pickup_address"`
	DropoffAddress string `json:"dropoff_address"`
	PackageDesc    string `json:"package_desc"`
	ReceiverName   string `json:"receiver_name"`
	ExpiresIn      int    `json:"expires_in"`
	Type           string `json:"type"`
}

type ProviderRequestInboxItem struct {
	InboxID          string    `json:"inbox_id"`
	BroadcastID      string    `json:"broadcast_id"`
	BookingID        string    `json:"booking_id"`
	FareAmount       int64     `json:"fare_amount"`
	Currency         string    `json:"currency"`
	PickupAddress    string    `json:"pickup_address"`
	DropoffAddress   string    `json:"dropoff_address"`
	DistanceKM       *float64  `json:"distance_km,omitempty"`
	PackageDesc      string    `json:"package_desc"`
	ReceiverName     string    `json:"receiver_name"`
	RemainingSeconds int64     `json:"remaining_seconds"`
	ExpiresAt        time.Time `json:"expires_at"`
	ReceivedAt       time.Time `json:"received_at"`
}

func NewProviderRequestInboxItem(inbox ProviderRequestInbox, now time.Time) (ProviderRequestInboxItem, bool, error) {
	remaining := int64(math.Floor(inbox.ExpiresAt.Sub(now).Seconds()))
	if remaining < 0 {
		return ProviderRequestInboxItem{}, false, nil
	}
	var booking BookingDispatchCreatedEvent
	if err := json.Unmarshal(inbox.BookingPayload, &booking); err != nil {
		return ProviderRequestInboxItem{}, false, err
	}
	return ProviderRequestInboxItem{
		InboxID: inbox.ID, BroadcastID: inbox.BroadcastID, BookingID: inbox.BookingID,
		FareAmount: booking.FareAmount, Currency: booking.Currency, PickupAddress: booking.PickupAddress,
		DropoffAddress: booking.DropoffAddress, PackageDesc: booking.PackageDesc, ReceiverName: booking.ReceiverName,
		RemainingSeconds: remaining, ExpiresAt: inbox.ExpiresAt, ReceivedAt: inbox.ReceivedAt,
	}, true, nil
}

// RequestAcceptedEvent is published when a provider accepts a request.
// It contains all fields that trip-service needs to create a delivery trip (Phase 6J).
type RequestAcceptedEvent struct {
	Event          string  `json:"event"`
	CorrelationID  string  `json:"correlation_id,omitempty"`
	BookingID      string  `json:"booking_id"`
	BroadcastID    string  `json:"broadcast_id"`
	InboxID        string  `json:"inbox_id"`
	ProviderID     string  `json:"provider_id"`
	FareAmount     int64   `json:"fare_amount"`
	Currency       string  `json:"currency"`
	PickupLat      float64 `json:"pickup_lat"`
	PickupLng      float64 `json:"pickup_lng"`
	PickupAddress  string  `json:"pickup_address"`
	DropoffLat     float64 `json:"dropoff_lat"`
	DropoffLng     float64 `json:"dropoff_lng"`
	DropoffAddress string  `json:"dropoff_address"`
	// TODO: persist provider distance in provider_request_inbox during broadcast to populate this accurately.
	DistanceKm    float64   `json:"distance_km"`
	ReceiverName  string    `json:"receiver_name"`
	ReceiverPhone string    `json:"receiver_phone"`
	PackageDesc   string    `json:"package_desc"`
	PackageWeight float64   `json:"weight_kg"`
	AcceptedAt    time.Time `json:"accepted_at"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type RequestRejectedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	BookingID     string    `json:"booking_id"`
	BroadcastID   string    `json:"broadcast_id"`
	InboxID       string    `json:"inbox_id"`
	ProviderID    string    `json:"provider_id"`
	Reason        string    `json:"reason"`
	RejectedAt    time.Time `json:"rejected_at"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type NoProviderFoundEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	BookingID     string    `json:"booking_id"`
	BroadcastID   string    `json:"broadcast_id"`
	Attempts      int       `json:"attempts"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// RequestDetailResponse is returned by GET /api/v1/provider/requests/:id (Phase 6F).
type RequestDetailResponse struct {
	InboxID          string      `json:"inbox_id"`
	BroadcastID      string      `json:"broadcast_id"`
	BookingID        string      `json:"booking_id"`
	Status           InboxStatus `json:"status"`
	FareAmount       int64       `json:"fare_amount"`
	Currency         string      `json:"currency"`
	PickupAddress    string      `json:"pickup_address"`
	PickupLat        float64     `json:"pickup_lat"`
	PickupLng        float64     `json:"pickup_lng"`
	DropoffAddress   string      `json:"dropoff_address"`
	DropoffLat       float64     `json:"dropoff_lat"`
	DropoffLng       float64     `json:"dropoff_lng"`
	PackageDesc      string      `json:"package_desc"`
	PackageWeight    float64     `json:"package_weight,omitempty"`
	ReceiverName     string      `json:"receiver_name"`
	ReceiverPhone    string      `json:"receiver_phone"`
	RemainingSeconds int64       `json:"remaining_seconds"`
	ExpiresAt        time.Time   `json:"expires_at"`
	ReceivedAt       time.Time   `json:"received_at"`
}

// AcceptResponse is returned by POST /api/v1/provider/requests/:id/accept (Phase 6G).
type AcceptResponse struct {
	BookingID      string  `json:"booking_id"`
	BroadcastID    string  `json:"broadcast_id"`
	InboxID        string  `json:"inbox_id"`
	Message        string  `json:"message"`
	PickupAddress  string  `json:"pickup_address"`
	PickupLat      float64 `json:"pickup_lat"`
	PickupLng      float64 `json:"pickup_lng"`
	DropoffAddress string  `json:"dropoff_address"`
	DropoffLat     float64 `json:"dropoff_lat"`
	DropoffLng     float64 `json:"dropoff_lng"`
	ReceiverName   string  `json:"receiver_name"`
	ReceiverPhone  string  `json:"receiver_phone"`
	FareAmount     int64   `json:"fare_amount"`
	Currency       string  `json:"currency"`
}

// RejectRequest is the optional request body for POST /api/v1/provider/requests/:id/reject.
type RejectRequest struct {
	Reason string `json:"reason"`
}

// RejectResponse is returned on successful reject (Phase 6H).
type RejectResponse struct {
	Message string `json:"message"`
}

// ValidRejectReasons lists the allowed rejection reason values.
var ValidRejectReasons = map[string]struct{}{
	"too_far": {},
	"busy":    {},
	"other":   {},
}

type BroadcastReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (r BroadcastReason) Error() string {
	return fmt.Sprintf("%s: %s", r.Code, r.Message)
}
