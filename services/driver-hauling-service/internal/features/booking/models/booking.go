package bookingmodels

import (
	"encoding/json"
	"math"
	"time"

	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/handover"
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

// Payment methods
const (
	PaymentMethodWallet = "wallet"
	PaymentMethodCard   = "card"
	PaymentMethodCash   = "cash"
)

var ValidPaymentMethods = map[string]bool{
	PaymentMethodWallet: true,
	PaymentMethodCard:   true,
	PaymentMethodCash:   true,
}

// Truck types are no longer listed here. internal/truckcatalog is the one
// source of truth for the taxonomy, shared with provider registration and the
// matcher; this file previously carried a second copy whose own comment asked
// the next author to keep it in sync by hand.

// Payment statuses (mirror the relevant payment-wallet intent states for the trip)
const (
	PaymentStatusUnpaid  = "unpaid"  // no charge yet (e.g. cash, or wallet before acceptance)
	PaymentStatusHeld    = "held"    // funds authorized/held against the booking
	PaymentStatusPaid    = "paid"    // settled to the provider
	PaymentStatusFailed  = "failed"  // charge attempt failed
	PaymentStatusUnknown = "unknown" // authoritative write outcome requires read-back
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

	PaymentMethod   string
	PaymentStatus   string
	PaymentIntentID *string

	Status       string
	CancelReason *string
	CancelledBy  *string

	ProofPhotoURL string
	SignatureURL  string

	// Route* holds the road geometry for the two legs the customer watches:
	// the driver approaching the pickup, then the cargo moving to the
	// drop-off. Empty when no routing API key is configured, in which case
	// consumers fall back to straight-line geometry.
	RoutePickupPolyline   string
	RoutePickupDistanceM  *int64
	RoutePickupDurationS  *int64
	RouteDropoffPolyline  string
	RouteDropoffDistanceM *int64
	RouteDropoffDurationS *int64
	RouteFetchedAt        *time.Time

	// PickupCode* is the driver's own pickup credential: generated when the
	// driver marks arrived-at-pickup, displayed on the driver's app, and
	// verified by the customer (PickupCodeAttempts counts the customer's
	// failed verification attempts).
	PickupCode            string
	PickupCodeHash        string
	PickupCodeExpiresAt   *time.Time
	PickupCodeAttempts    int
	PickupCodeResendCount int
	PickupCodeLastSentAt  *time.Time
	PickupCodeVerifiedAt  *time.Time

	// DeliveryCode* is the customer's delivery credential: generated when
	// the driver marks en-route-to-delivery, displayed on the customer's
	// app, and verified by the driver (DeliveryCodeAttempts counts the
	// driver's failed verification attempts).
	DeliveryCode            string
	DeliveryCodeHash        string
	DeliveryCodeExpiresAt   *time.Time
	DeliveryCodeAttempts    int
	DeliveryCodeResendCount int
	DeliveryCodeLastSentAt  *time.Time
	DeliveryCodeVerifiedAt  *time.Time

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

	PaymentMethod string `json:"payment_method,omitempty"`
	PaymentStatus string `json:"payment_status,omitempty"`

	Status       string  `json:"status"`
	CancelReason *string `json:"cancel_reason,omitempty"`

	ProofPhotoURL string `json:"proof_photo_url,omitempty"`
	SignatureURL  string `json:"signature_url,omitempty"`

	// Route geometry for the customer's map. The pickup leg is only present
	// once a driver has been assigned; both are absent when no routing API
	// key is configured and the app falls back to a straight line.
	RoutePickupPolyline   string `json:"route_pickup_polyline,omitempty"`
	RoutePickupDistanceM  *int64 `json:"route_pickup_distance_m,omitempty"`
	RoutePickupDurationS  *int64 `json:"route_pickup_duration_s,omitempty"`
	RouteDropoffPolyline  string `json:"route_dropoff_polyline,omitempty"`
	RouteDropoffDistanceM *int64 `json:"route_dropoff_distance_m,omitempty"`
	RouteDropoffDurationS *int64 `json:"route_dropoff_duration_s,omitempty"`

	MatchedAt   *time.Time `json:"matched_at,omitempty"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	PickedUpAt  *time.Time `json:"picked_up_at,omitempty"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`

	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`

	// Commerce is present only for a Hauling Commerce mission and contains
	// operational evidence needed by the assigned provider. It deliberately
	// carries no customer charge, Vendor margin, commission or settlement.
	Commerce *CommerceOperations `json:"commerce,omitempty"`
}

type CommerceParcel struct {
	ParcelRef       string                   `json:"parcel_ref"`
	PickupStopRef   string                   `json:"pickup_stop_ref,omitempty"`
	ManifestRef     string                   `json:"manifest_ref,omitempty"`
	ManifestVersion int64                    `json:"manifest_version,omitempty"`
	WeightGrams     int64                    `json:"weight_grams,omitempty"`
	LengthMm        int64                    `json:"length_mm,omitempty"`
	WidthMm         int64                    `json:"width_mm,omitempty"`
	HeightMm        int64                    `json:"height_mm,omitempty"`
	PackageCount    int64                    `json:"package_count,omitempty"`
	Handling        commercemission.Handling `json:"handling,omitempty"`
}

type CommerceOperations struct {
	SourceType             string                       `json:"source_type"`
	OperationRef           string                       `json:"operation_ref"`
	VehicleClass           string                       `json:"vehicle_class"`
	PickupStops            []commercemission.PickupStop `json:"pickup_stops"`
	Parcels                []CommerceParcel             `json:"parcels"`
	ManifestRef            string                       `json:"manifest_ref,omitempty"`
	ManifestVersion        int64                        `json:"manifest_version,omitempty"`
	Handling               commercemission.Handling     `json:"handling,omitempty"`
	PickupWindowStart      *time.Time                   `json:"pickup_window_start,omitempty"`
	PickupWindowEnd        *time.Time                   `json:"pickup_window_end,omitempty"`
	PickupTimezone         string                       `json:"pickup_timezone,omitempty"`
	LoadingDurationSeconds int64                        `json:"loading_duration_seconds,omitempty"`
}

type commerceParcelWire struct {
	ParcelRef       string                   `json:"parcel_ref"`
	PickupStopRef   string                   `json:"pickup_stop_ref,omitempty"`
	ManifestRef     string                   `json:"manifest_ref,omitempty"`
	ManifestVersion int64                    `json:"manifest_version,omitempty"`
	WeightGrams     int64                    `json:"weight_grams,omitempty"`
	LengthMm        int64                    `json:"length_mm,omitempty"`
	WidthMm         int64                    `json:"width_mm,omitempty"`
	HeightMm        int64                    `json:"height_mm,omitempty"`
	PackageCount    int64                    `json:"package_count,omitempty"`
	Handling        commercemission.Handling `json:"handling,omitempty"`
	Manifest        json.RawMessage          `json:"manifest,omitempty"`
}

// SanitizeCommerceParcels projects only provider-operational fields. The raw
// manifest may contain Vendor order detail and is intentionally discarded.
func SanitizeCommerceParcels(raw []byte) ([]CommerceParcel, error) {
	var source []commerceParcelWire
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &source); err != nil {
			return nil, err
		}
	}
	result := make([]CommerceParcel, 0, len(source))
	for _, parcel := range source {
		result = append(result, CommerceParcel{
			ParcelRef: parcel.ParcelRef, PickupStopRef: parcel.PickupStopRef,
			ManifestRef: parcel.ManifestRef, ManifestVersion: parcel.ManifestVersion,
			WeightGrams: parcel.WeightGrams, LengthMm: parcel.LengthMm,
			WidthMm: parcel.WidthMm, HeightMm: parcel.HeightMm,
			PackageCount: parcel.PackageCount, Handling: parcel.Handling,
		})
	}
	return result, nil
}

// PickupCodeRecord returns the driver's pickup credential state as a
// handover.CodeRecord for use with the shared verification helpers.
func (b Booking) PickupCodeRecord() handover.CodeRecord {
	rec := handover.CodeRecord{
		Code:        b.PickupCode,
		Hash:        b.PickupCodeHash,
		Attempts:    b.PickupCodeAttempts,
		ResendCount: b.PickupCodeResendCount,
		LastSentAt:  b.PickupCodeLastSentAt,
		VerifiedAt:  b.PickupCodeVerifiedAt,
	}
	if b.PickupCodeExpiresAt != nil {
		rec.ExpiresAt = *b.PickupCodeExpiresAt
	}
	return rec
}

// DeliveryCodeRecord returns the customer's delivery credential state as a
// handover.CodeRecord for use with the shared verification helpers.
func (b Booking) DeliveryCodeRecord() handover.CodeRecord {
	rec := handover.CodeRecord{
		Code:        b.DeliveryCode,
		Hash:        b.DeliveryCodeHash,
		Attempts:    b.DeliveryCodeAttempts,
		ResendCount: b.DeliveryCodeResendCount,
		LastSentAt:  b.DeliveryCodeLastSentAt,
		VerifiedAt:  b.DeliveryCodeVerifiedAt,
	}
	if b.DeliveryCodeExpiresAt != nil {
		rec.ExpiresAt = *b.DeliveryCodeExpiresAt
	}
	return rec
}

func (b Booking) Public() PublicBooking {
	return PublicBooking{
		ID:                    b.ID,
		CustomerID:            b.CustomerID,
		ProviderID:            b.ProviderID,
		TruckID:               b.TruckID,
		PickupAddress:         b.PickupAddress,
		PickupLat:             b.PickupLat,
		PickupLng:             b.PickupLng,
		DropoffAddress:        b.DropoffAddress,
		DropoffLat:            b.DropoffLat,
		DropoffLng:            b.DropoffLng,
		CargoType:             b.CargoType,
		PreferredTruckType:    b.PreferredTruckType,
		CargoWeightKg:         b.CargoWeightKg,
		CargoDescription:      b.CargoDescription,
		RequiresHelpers:       b.RequiresHelpers,
		HelperCount:           b.HelperCount,
		WeightCategory:        b.WeightCategory,
		ReceiverName:          b.ReceiverName,
		ReceiverPhone:         b.ReceiverPhone,
		PackageContent:        b.PackageContent,
		PackageSize:           b.PackageSize,
		IsFragile:             b.IsFragile,
		DistanceKm:            b.DistanceKm,
		FareEstimateKobo:      b.FareEstimateKobo,
		FareFinalKobo:         b.FareFinalKobo,
		PaymentMethod:         b.PaymentMethod,
		PaymentStatus:         b.PaymentStatus,
		Status:                b.Status,
		CancelReason:          b.CancelReason,
		ProofPhotoURL:         b.ProofPhotoURL,
		SignatureURL:          b.SignatureURL,
		RoutePickupPolyline:   b.RoutePickupPolyline,
		RoutePickupDistanceM:  b.RoutePickupDistanceM,
		RoutePickupDurationS:  b.RoutePickupDurationS,
		RouteDropoffPolyline:  b.RouteDropoffPolyline,
		RouteDropoffDistanceM: b.RouteDropoffDistanceM,
		RouteDropoffDurationS: b.RouteDropoffDurationS,
		MatchedAt:             b.MatchedAt,
		AcceptedAt:            b.AcceptedAt,
		PickedUpAt:            b.PickedUpAt,
		DeliveredAt:           b.DeliveredAt,
		CompletedAt:           b.CompletedAt,
		CancelledAt:           b.CancelledAt,
		ScheduledAt:           b.ScheduledAt,
		CreatedAt:             b.CreatedAt,
	}
}

// FareEstimate calculates v1 fare
type FareEstimate struct {
	DistanceKm       float64       `json:"distance_km"`
	FareEstimateKobo int64         `json:"fare_estimate_kobo"`
	BreakdownKobo    FareBreakdown `json:"breakdown"`
}

type FareBreakdown struct {
	BaseFareKobo    int64 `json:"base_fare_kobo"`
	PerKmFareKobo   int64 `json:"per_km_fare_kobo"`
	WeightSurcharge int64 `json:"weight_surcharge_kobo"`
	HelperFeeKobo   int64 `json:"helper_fee_kobo"`
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
		baseFare  int64 = 500_000 // ₦5,000 in kobo
		perKmRate int64 = 25_000  // ₦250/km in kobo
		helperFee int64 = 200_000 // ₦2,000/helper in kobo
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
