package bookingmodels

import (
	"math"
	"time"
)

// Booking statuses
const (
	StatusPendingMatch       = "pending_match"
	StatusAwaitingAcceptance = "awaiting_acceptance"
	StatusAccepted           = "accepted"
	StatusEnRoutePickup      = "en_route_pickup"
	StatusArrivedAtPickup    = "arrived_at_pickup"
	StatusPickedUp           = "picked_up"
	StatusEnRouteDelivery    = "en_route_delivery"
	StatusDelivered          = "delivered"
	StatusCompleted          = "completed"
	StatusCancelled          = "cancelled"
	StatusUnmatched          = "unmatched"
)

// Cargo types
const (
	CargoTypeFurniture    = "furniture"
	CargoTypeEquipment    = "equipment"
	CargoTypeConstruction = "construction"
	CargoTypeFood         = "food"
	CargoTypeGeneral      = "general"
)

var ValidCargoTypes = map[string]bool{
	CargoTypeFurniture:    true,
	CargoTypeEquipment:    true,
	CargoTypeConstruction: true,
	CargoTypeFood:         true,
	CargoTypeGeneral:      true,
}

// Actor types for events
const (
	ActorCustomer = "customer"
	ActorProvider = "provider"
	ActorSystem   = "system"
)

type Booking struct {
	ID         string
	CustomerID string
	ProviderID *string
	TruckID    *string

	PickupAddress string
	PickupLat     float64
	PickupLng     float64

	DropoffAddress string
	DropoffLat     float64
	DropoffLng     float64

	CargoType          string
	PreferredTruckType string
	CargoWeightKg      int
	CargoDescription   string
	RequiresHelpers    bool
	HelperCount        int

	WeightCategory string
	ReceiverName   string
	ReceiverPhone  string
	PackageContent string
	PackageSize    string
	IsFragile      bool

	DistanceKm       *float64
	FareEstimateKobo *int64
	FareFinalKobo    *int64

	PaymentIntentID *string

	Status       string
	CancelReason *string
	CancelledBy  *string

	MatchedAt   *time.Time
	AcceptedAt  *time.Time
	PickedUpAt  *time.Time
	DeliveredAt *time.Time
	CompletedAt *time.Time
	CancelledAt *time.Time

	ScheduledAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

type PublicBooking struct {
	ID         string  `json:"id"`
	CustomerID string  `json:"customer_id"`
	ProviderID *string `json:"provider_id,omitempty"`
	TruckID    *string `json:"truck_id,omitempty"`

	PickupAddress string  `json:"pickup_address"`
	PickupLat     float64 `json:"pickup_lat"`
	PickupLng     float64 `json:"pickup_lng"`

	DropoffAddress string  `json:"dropoff_address"`
	DropoffLat     float64 `json:"dropoff_lat"`
	DropoffLng     float64 `json:"dropoff_lng"`

	CargoType          string `json:"cargo_type"`
	PreferredTruckType string `json:"preferred_truck_type,omitempty"`
	CargoWeightKg      int    `json:"cargo_weight_kg"`
	CargoDescription   string `json:"cargo_description"`
	RequiresHelpers    bool   `json:"requires_helpers"`
	HelperCount        int    `json:"helper_count"`

	WeightCategory string `json:"weight_category,omitempty"`
	ReceiverName   string `json:"receiver_name,omitempty"`
	ReceiverPhone  string `json:"receiver_phone,omitempty"`
	PackageContent string `json:"package_content,omitempty"`
	PackageSize    string `json:"package_size,omitempty"`
	IsFragile      bool   `json:"is_fragile"`

	DistanceKm       *float64 `json:"distance_km,omitempty"`
	FareEstimateKobo *int64   `json:"fare_estimate_kobo,omitempty"`
	FareFinalKobo    *int64   `json:"fare_final_kobo,omitempty"`

	Status      string  `json:"status"`
	CancelReason *string `json:"cancel_reason,omitempty"`

	MatchedAt   *time.Time `json:"matched_at,omitempty"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	PickedUpAt  *time.Time `json:"picked_up_at,omitempty"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`

	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (b Booking) Public() PublicBooking {
	return PublicBooking{
		ID:               b.ID,
		CustomerID:       b.CustomerID,
		ProviderID:       b.ProviderID,
		TruckID:          b.TruckID,
		PickupAddress:    b.PickupAddress,
		PickupLat:        b.PickupLat,
		PickupLng:        b.PickupLng,
		DropoffAddress:   b.DropoffAddress,
		DropoffLat:       b.DropoffLat,
		DropoffLng:       b.DropoffLng,
		CargoType:          b.CargoType,
		PreferredTruckType: b.PreferredTruckType,
		CargoWeightKg:      b.CargoWeightKg,
		CargoDescription:   b.CargoDescription,
		RequiresHelpers:    b.RequiresHelpers,
		HelperCount:        b.HelperCount,
		WeightCategory:    b.WeightCategory,
		ReceiverName:      b.ReceiverName,
		ReceiverPhone:     b.ReceiverPhone,
		PackageContent:    b.PackageContent,
		PackageSize:       b.PackageSize,
		IsFragile:         b.IsFragile,
		DistanceKm:       b.DistanceKm,
		FareEstimateKobo: b.FareEstimateKobo,
		FareFinalKobo:    b.FareFinalKobo,
		Status:           b.Status,
		CancelReason:     b.CancelReason,
		MatchedAt:        b.MatchedAt,
		AcceptedAt:       b.AcceptedAt,
		PickedUpAt:       b.PickedUpAt,
		DeliveredAt:      b.DeliveredAt,
		CompletedAt:      b.CompletedAt,
		CancelledAt:      b.CancelledAt,
		ScheduledAt:      b.ScheduledAt,
		CreatedAt:        b.CreatedAt,
	}
}

// FareEstimate calculates v1 fare
type FareEstimate struct {
	DistanceKm       float64 `json:"distance_km"`
	FareEstimateKobo int64   `json:"fare_estimate_kobo"`
	BreakdownKobo    FareBreakdown `json:"breakdown"`
}

type FareBreakdown struct {
	BaseFareKobo      int64 `json:"base_fare_kobo"`
	PerKmFareKobo     int64 `json:"per_km_fare_kobo"`
	WeightSurcharge   int64 `json:"weight_surcharge_kobo"`
	HelperFeeKobo     int64 `json:"helper_fee_kobo"`
}

type BookingEvent struct {
	ID        string
	BookingID string
	EventType string
	ActorType string
	ActorID   string
	Metadata  []byte
}

type BookingReview struct {
	ID               string
	BookingID        string
	CustomerID       string
	ProviderID       string
	Rating           int
	ReviewText       string
	RecommendsDriver *bool
	CreatedAt        time.Time
}

type PublicBookingReview struct {
	ID               string    `json:"id"`
	BookingID        string    `json:"booking_id"`
	Rating           int       `json:"rating"`
	ReviewText       string    `json:"review_text"`
	RecommendsDriver *bool     `json:"recommends_driver,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func (r BookingReview) Public() PublicBookingReview {
	return PublicBookingReview{
		ID:               r.ID,
		BookingID:        r.BookingID,
		Rating:           r.Rating,
		ReviewText:       r.ReviewText,
		RecommendsDriver: r.RecommendsDriver,
		CreatedAt:        r.CreatedAt,
	}
}

func CalculateFare(distanceKm float64, weightKg, helperCount int) FareEstimate {
	const (
		baseFare   int64 = 500_000  // ₦5,000 in kobo
		perKmRate  int64 = 25_000   // ₦250/km in kobo
		helperFee  int64 = 200_000  // ₦2,000/helper in kobo
	)

	perKm := int64(math.Round(distanceKm * float64(perKmRate)))
	base := baseFare

	var weightSurcharge int64
	if weightKg > 500 {
		weightSurcharge = (base + perKm) / 10 // 10% surcharge
	}

	helpers := int64(helperCount) * helperFee
	total := base + perKm + weightSurcharge + helpers

	return FareEstimate{
		DistanceKm:       distanceKm,
		FareEstimateKobo: total,
		BreakdownKobo: FareBreakdown{
			BaseFareKobo:    base,
			PerKmFareKobo:   perKm,
			WeightSurcharge: weightSurcharge,
			HelperFeeKobo:   helpers,
		},
	}
}
