package delivery

import "time"

// Booking status values (mirrors partner_bookings.status CHECK constraint).
const (
	StatusPending     = "pending"
	StatusDispatching = "dispatching"
	StatusAccepted    = "accepted"
	StatusInProgress  = "in_progress"
	StatusCompleted   = "completed"
	StatusCancelled   = "cancelled"
	StatusFailed      = "failed"
	StatusNoProvider  = "no_provider"
)

// Payment methods accepted by the Partner API.
const (
	PaymentMethodWallet   = "wallet"
	PaymentMethodPaystack = "paystack"
)

// Service tiers.
const (
	ServiceTierStandard = "standard"
	ServiceTierExpress  = "express"
)

const currencyNGN = "NGN"

var validPackageTypes = map[string]struct{}{
	"documents": {}, "food": {}, "electronics": {}, "clothing": {}, "medical": {}, "other": {},
}

var validPackageSizes = map[string]struct{}{
	"small": {}, "medium": {}, "large": {},
}

// cancellableStatuses lists the statuses from which a partner may cancel.
var cancellableStatuses = map[string]struct{}{
	StatusPending:     {},
	StatusDispatching: {},
	StatusAccepted:    {},
}

func canCancel(status string) bool {
	_, ok := cancellableStatuses[status]
	return ok
}

// PartnerBooking mirrors a partner_bookings row.
//
// PickupContactName, PickupContactPhone, DistanceKM, and PackageDescription are
// pointers because they were added by migration 003 as nullable columns (rows
// created before that migration have no value for them).
type PartnerBooking struct {
	ID                  string
	BookingRef          string
	PartnerID           string
	ExternalRef         *string
	PickupAddress       string
	PickupContactName   *string
	PickupContactPhone  *string
	PickupLat           float64
	PickupLng           float64
	DestAddress         string
	DestLat             float64
	DestLng             float64
	ReceiverName        string
	ReceiverPhone       string
	PackageType         *string
	PackageSize         *string
	PackageDescription  *string
	IsFragile           bool
	ServiceTier         string
	DistanceKM          *float64
	FareAmountKobo      int64
	Currency            string
	Status              string
	PaymentIntentID     *string
	DispatchBookingID   *string
	IdempotencyKey      string
	EventPublishedAt    *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ListFilter is a partner booking list query.
type ListFilter struct {
	Status string // optional; validated before use
	Page   int
	Limit  int
	Offset int
}

// --- request DTOs -----------------------------------------------------------

// QuoteRequest is the POST /quotes body.
type QuoteRequest struct {
	PickupLat   *float64 `json:"pickup_lat"`
	PickupLng   *float64 `json:"pickup_lng"`
	DestLat     *float64 `json:"dest_lat"`
	DestLng     *float64 `json:"dest_lng"`
	PackageSize string   `json:"package_size"`
	ServiceTier string   `json:"service_tier"`
}

// DeliveryRequest is the POST /deliveries body.
//
// PickupContactName, PickupContactPhone, and PackageDescription are new,
// optional fields (added alongside migration 003). DistanceKM is NOT a request
// field - it is always server-computed from pickup/dest coordinates, the same
// way fare already is, so a partner cannot submit a fake distance.
type DeliveryRequest struct {
	PickupAddress      string   `json:"pickup_address"`
	PickupContactName  string   `json:"pickup_contact_name"`
	PickupContactPhone string   `json:"pickup_contact_phone"`
	PickupLat          *float64 `json:"pickup_lat"`
	PickupLng          *float64 `json:"pickup_lng"`
	DestAddress        string   `json:"dest_address"`
	DestLat            *float64 `json:"dest_lat"`
	DestLng            *float64 `json:"dest_lng"`
	ReceiverName       string   `json:"receiver_name"`
	ReceiverPhone      string   `json:"receiver_phone"`
	PackageType        string   `json:"package_type"`
	PackageSize        string   `json:"package_size"`
	PackageDescription string   `json:"package_description"`
	IsFragile          bool     `json:"is_fragile"`
	ServiceTier        string   `json:"service_tier"`
	PaymentMethod      string   `json:"payment_method"`
	CustomerID         string   `json:"customer_id"`
	CustomerEmail      string   `json:"customer_email"`
	IdempotencyKey     string   `json:"idempotency_key"`
	ExternalRef        string   `json:"external_ref"`
}

// --- response DTOs ----------------------------------------------------------

// Money is the standard fare representation.
type Money struct {
	Kobo    int64  `json:"kobo"`
	Display string `json:"display"`
}

// QuoteResponse is the POST /quotes response data.
type QuoteResponse struct {
	DistanceKM   float64 `json:"distance_km"`
	EstimatedETA string  `json:"estimated_eta"`
	Fare         Money   `json:"fare"`
	ServiceTier  string  `json:"service_tier"`
	ExpiresAt    string  `json:"expires_at"`
}

// DeliverySummary is the compact booking shape used by create/list. Left
// unchanged in this pass (contact/distance/package-description detail live on
// DeliveryDetail only, matching what the list screen actually shows).
type DeliverySummary struct {
	ID            string  `json:"id"`
	BookingRef    string  `json:"booking_ref"`
	Status        string  `json:"status"`
	ServiceTier   string  `json:"service_tier"`
	FareAmount    Money   `json:"fare_amount"`
	PickupAddress string  `json:"pickup_address"`
	DestAddress   string  `json:"dest_address"`
	ReceiverName  string  `json:"receiver_name"`
	ExternalRef   *string `json:"external_ref"`
	CreatedAt     string  `json:"created_at"`
}

// TimelineEntry is one status transition in a delivery timeline.
type TimelineEntry struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

// RiderInfo is rider detail (null until dispatch lifecycle sync exists).
// Vehicle is intentionally not added here yet - it needs a cross-service
// decision (live call to driver-dispatch-delivery-service vs. an event-carried
// snapshot) before it can be implemented; see conversation notes.
type RiderInfo struct {
	Name   *string  `json:"name"`
	Rating *float64 `json:"rating"`
	Phone  *string  `json:"phone"`
}

// DeliveryDetail is the GET /deliveries/:id response data.
//
// New fields vs. the original: PickupContactName, PickupContactPhone,
// ReceiverPhone (data already existed on PartnerBooking, just was never put on
// this response DTO before), DistanceKM, PackageDescription.
type DeliveryDetail struct {
	ID                 string          `json:"id"`
	BookingRef         string          `json:"booking_ref"`
	Status             string          `json:"status"`
	ServiceTier        string          `json:"service_tier"`
	FareAmount         Money           `json:"fare_amount"`
	PickupAddress      string          `json:"pickup_address"`
	PickupContactName  *string         `json:"pickup_contact_name"`
	PickupContactPhone *string         `json:"pickup_contact_phone"`
	DestAddress        string          `json:"dest_address"`
	ReceiverName       string          `json:"receiver_name"`
	ReceiverPhone      string          `json:"receiver_phone"`
	DistanceKM         *float64        `json:"distance_km"`
	PackageDescription *string         `json:"package_description"`
	Rider              RiderInfo       `json:"rider"`
	TrackingURL        *string         `json:"tracking_url"`
	Timeline           []TimelineEntry `json:"timeline"`
	CreatedAt          string          `json:"created_at"`
}

// ListResponse is the GET /deliveries response data.
type ListResponse struct {
	Deliveries []DeliverySummary `json:"deliveries"`
	Meta       ListMeta          `json:"meta"`
}

// ListMeta is pagination metadata.
type ListMeta struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
}

// CancelResponse is the DELETE /deliveries/:id response data.
type CancelResponse struct {
	ID         string `json:"id"`
	BookingRef string `json:"booking_ref"`
	Status     string `json:"status"`
	Cancelled  bool   `json:"cancelled"`
}

const rfc3339 = time.RFC3339

func summaryOf(b *PartnerBooking) DeliverySummary {
	return DeliverySummary{
		ID:            b.ID,
		BookingRef:    b.BookingRef,
		Status:        b.Status,
		ServiceTier:   b.ServiceTier,
		FareAmount:    Money{Kobo: b.FareAmountKobo, Display: formatNaira(b.FareAmountKobo)},
		PickupAddress: b.PickupAddress,
		DestAddress:   b.DestAddress,
		ReceiverName:  b.ReceiverName,
		ExternalRef:   b.ExternalRef,
		CreatedAt:     b.CreatedAt.UTC().Format(rfc3339),
	}
}

func detailOf(b *PartnerBooking) DeliveryDetail {
	// Timeline is derived from partner_bookings timestamps only; rider/tracking
	// stay null until dispatch lifecycle sync is implemented. No faked data.
	timeline := []TimelineEntry{{Status: StatusPending, At: b.CreatedAt.UTC().Format(rfc3339)}}
	if b.EventPublishedAt != nil {
		timeline = append(timeline, TimelineEntry{
			Status: StatusDispatching,
			At:     b.EventPublishedAt.UTC().Format(rfc3339),
		})
	}
	return DeliveryDetail{
		ID:                 b.ID,
		BookingRef:         b.BookingRef,
		Status:             b.Status,
		ServiceTier:        b.ServiceTier,
		FareAmount:         Money{Kobo: b.FareAmountKobo, Display: formatNaira(b.FareAmountKobo)},
		PickupAddress:      b.PickupAddress,
		PickupContactName:  b.PickupContactName,
		PickupContactPhone: b.PickupContactPhone,
		DestAddress:        b.DestAddress,
		ReceiverName:       b.ReceiverName,
		ReceiverPhone:      b.ReceiverPhone,
		DistanceKM:         b.DistanceKM,
		PackageDescription: b.PackageDescription,
		Rider:              RiderInfo{},
		TrackingURL:        nil,
		Timeline:           timeline,
		CreatedAt:          b.CreatedAt.UTC().Format(rfc3339),
	}
}