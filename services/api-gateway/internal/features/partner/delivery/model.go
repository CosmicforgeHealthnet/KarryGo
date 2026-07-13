package delivery

import (
	"fmt"
	"time"
)

// Booking status values (mirrors partner_bookings.status CHECK constraint).
const (
	StatusPending     = "pending"
	StatusDispatching = "dispatching"
	StatusAccepted    = "accepted"
	StatusArrived     = "arrived"
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

// trackingURLTemplate is the provider live-location websocket URL exposed to
// partners once a rider is assigned (rider_id interpolated).
const trackingURLTemplate = "wss://api.karrygo.com/ws/provider/%s/location"

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
type PartnerBooking struct {
	ID                 string
	BookingRef         string
	PartnerID          string
	ExternalRef        *string
	PickupAddress      string
	PickupLat          float64
	PickupLng          float64
	PickupContactName  *string
	PickupContactPhone *string
	DropoffAddress     string
	DropoffLat         float64
	DropoffLng         float64
	ReceiverName       string
	ReceiverPhone      string
	PackageType        *string
	PackageSize        *string
	PackageDesc        *string
	PackageWeight      *float64
	IsFragile          bool
	DistanceKM         *float64
	ServiceTier        string
	FareAmountKobo     int64
	Currency           string
	Status             string
	PaymentIntentID    *string
	DispatchBookingID  *string
	RiderID            *string
	RiderName          *string
	RiderPhone         *string
	RiderRating        *float64
	VehicleColor       *string
	VehicleModel       *string
	PlateNumber        *string
	IdempotencyKey     string
	EventPublishedAt   *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// RiderSnapshot is the rider/vehicle info captured from the dispatch
// trip.started event/callback and stored on the partner booking.
type RiderSnapshot struct {
	RiderID      string
	RiderName    string
	RiderPhone   string
	RiderRating  *float64
	VehicleColor string
	VehicleModel string
	PlateNumber  string
}

// HasData reports whether the snapshot carries anything worth storing.
func (s RiderSnapshot) HasData() bool {
	return s.RiderID != "" || s.RiderName != "" || s.RiderPhone != "" ||
		s.RiderRating != nil || s.VehicleColor != "" || s.VehicleModel != "" || s.PlateNumber != ""
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
	DropoffLat  *float64 `json:"dropoff_lat"`
	DropoffLng  *float64 `json:"dropoff_lng"`
	PackageSize string   `json:"package_size"`
	ServiceTier string   `json:"service_tier"`
}

// DeliveryRequest is the POST /deliveries body.
type DeliveryRequest struct {
	PickupAddress      string   `json:"pickup_address"`
	PickupLat          *float64 `json:"pickup_lat"`
	PickupLng          *float64 `json:"pickup_lng"`
	PickupContactName  string   `json:"pickup_contact_name"`
	PickupContactPhone string   `json:"pickup_contact_phone"`
	DropoffAddress     string   `json:"dropoff_address"`
	DropoffLat         *float64 `json:"dropoff_lat"`
	DropoffLng         *float64 `json:"dropoff_lng"`
	ReceiverName       string   `json:"receiver_name"`
	ReceiverPhone      string   `json:"receiver_phone"`
	PackageType        string   `json:"package_type"`
	PackageSize        string   `json:"package_size"`
	PackageDesc        string   `json:"package_desc"`
	PackageWeight      *float64 `json:"package_weight"`
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

// DeliverySummary is the compact booking shape used by create/list.
type DeliverySummary struct {
	ID             string  `json:"id"`
	BookingRef     string  `json:"booking_ref"`
	Status         string  `json:"status"`
	ServiceTier    string  `json:"service_tier"`
	FareAmount     Money   `json:"fare_amount"`
	PickupAddress  string  `json:"pickup_address"`
	DropoffAddress string  `json:"dropoff_address"`
	ReceiverName   string  `json:"receiver_name"`
	ExternalRef    *string `json:"external_ref"`
	CreatedAt      string  `json:"created_at"`
}

// TimelineEntry is one status transition in a delivery timeline.
type TimelineEntry struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

// PickupInfo is the pickup location + contact block in the delivery detail.
type PickupInfo struct {
	Address      string  `json:"address"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	ContactName  *string `json:"contact_name"`
	ContactPhone *string `json:"contact_phone"`
}

// DropoffInfo is the dropoff location block in the delivery detail.
type DropoffInfo struct {
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

// ReceiverInfo is the receiver block in the delivery detail.
type ReceiverInfo struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// PackageInfo is the package block in the delivery detail.
type PackageInfo struct {
	Type        *string  `json:"type"`
	Size        *string  `json:"size"`
	Description *string  `json:"description"`
	WeightKg    *float64 `json:"weight_kg"`
	IsFragile   bool     `json:"is_fragile"`
}

// FareInfo is the fare block in the delivery detail.
type FareInfo struct {
	Kobo     int64  `json:"kobo"`
	Display  string `json:"display"`
	Currency string `json:"currency"`
}

// RiderInfo is the rider/vehicle block. The object always exists; fields are
// null until a rider is assigned (trip.started).
type RiderInfo struct {
	ID           *string  `json:"id"`
	Name         *string  `json:"name"`
	Phone        *string  `json:"phone"`
	Rating       *float64 `json:"rating"`
	VehicleColor *string  `json:"vehicle_color"`
	VehicleModel *string  `json:"vehicle_model"`
	PlateNumber  *string  `json:"plate_number"`
}

// DeliveryDetail is the GET /deliveries/:id response data.
type DeliveryDetail struct {
	ID          string          `json:"id"`
	BookingRef  string          `json:"booking_ref"`
	ExternalRef *string         `json:"external_ref"`
	Status      string          `json:"status"`
	ServiceTier string          `json:"service_tier"`
	DistanceKM  *float64        `json:"distance_km"`
	Pickup      PickupInfo      `json:"pickup"`
	Dropoff     DropoffInfo     `json:"dropoff"`
	Receiver    ReceiverInfo    `json:"receiver"`
	Package     PackageInfo     `json:"package"`
	Fare        FareInfo        `json:"fare"`
	Rider       RiderInfo       `json:"rider"`
	TrackingURL *string         `json:"tracking_url"`
	Timeline    []TimelineEntry `json:"timeline"`
	CreatedAt   string          `json:"created_at"`
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
		ID:             b.ID,
		BookingRef:     b.BookingRef,
		Status:         b.Status,
		ServiceTier:    b.ServiceTier,
		FareAmount:     Money{Kobo: b.FareAmountKobo, Display: formatNaira(b.FareAmountKobo)},
		PickupAddress:  b.PickupAddress,
		DropoffAddress: b.DropoffAddress,
		ReceiverName:   b.ReceiverName,
		ExternalRef:    b.ExternalRef,
		CreatedAt:      b.CreatedAt.UTC().Format(rfc3339),
	}
}

func detailOf(b *PartnerBooking) DeliveryDetail {
	currency := b.Currency
	if currency == "" {
		currency = currencyNGN
	}

	var trackingURL *string
	if b.RiderID != nil && *b.RiderID != "" {
		u := fmt.Sprintf(trackingURLTemplate, *b.RiderID)
		trackingURL = &u
	}

	return DeliveryDetail{
		ID:          b.ID,
		BookingRef:  b.BookingRef,
		ExternalRef: b.ExternalRef,
		Status:      b.Status,
		ServiceTier: b.ServiceTier,
		DistanceKM:  b.DistanceKM,
		Pickup: PickupInfo{
			Address:      b.PickupAddress,
			Lat:          b.PickupLat,
			Lng:          b.PickupLng,
			ContactName:  b.PickupContactName,
			ContactPhone: b.PickupContactPhone,
		},
		Dropoff: DropoffInfo{
			Address: b.DropoffAddress,
			Lat:     b.DropoffLat,
			Lng:     b.DropoffLng,
		},
		Receiver: ReceiverInfo{Name: b.ReceiverName, Phone: b.ReceiverPhone},
		Package: PackageInfo{
			Type:        b.PackageType,
			Size:        b.PackageSize,
			Description: b.PackageDesc,
			WeightKg:    b.PackageWeight,
			IsFragile:   b.IsFragile,
		},
		Fare: FareInfo{
			Kobo:     b.FareAmountKobo,
			Display:  formatNaira(b.FareAmountKobo),
			Currency: currency,
		},
		Rider: RiderInfo{
			ID:           b.RiderID,
			Name:         b.RiderName,
			Phone:        b.RiderPhone,
			Rating:       b.RiderRating,
			VehicleColor: b.VehicleColor,
			VehicleModel: b.VehicleModel,
			PlateNumber:  b.PlateNumber,
		},
		TrackingURL: trackingURL,
		Timeline:    timelineOf(b),
		CreatedAt:   b.CreatedAt.UTC().Format(rfc3339),
	}
}

// timelineOf derives a minimal timeline from partner_bookings timestamps until a
// dedicated status-log table exists: pending (created_at), dispatching
// (event_published_at when set), and the current status (updated_at) when it has
// moved beyond those two.
func timelineOf(b *PartnerBooking) []TimelineEntry {
	timeline := []TimelineEntry{{Status: StatusPending, At: b.CreatedAt.UTC().Format(rfc3339)}}
	if b.EventPublishedAt != nil {
		timeline = append(timeline, TimelineEntry{
			Status: StatusDispatching,
			At:     b.EventPublishedAt.UTC().Format(rfc3339),
		})
	}
	if b.Status != StatusPending && b.Status != StatusDispatching {
		timeline = append(timeline, TimelineEntry{
			Status: b.Status,
			At:     b.UpdatedAt.UTC().Format(rfc3339),
		})
	}
	return timeline
}
