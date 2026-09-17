package bookinghttp

import (
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingusecases "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
)

type stopRequest struct {
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

type createBookingRequest struct {
	VehicleClass            string        `json:"vehicle_class"`
	BookingMode             string        `json:"booking_mode"`
	PassengerCount          int           `json:"passenger_count"`
	RideNotes               string        `json:"ride_notes"`
	PaymentMethod           string        `json:"payment_method"`
	ReservedDurationMinutes int           `json:"reserved_duration_minutes"`
	Stops                   []stopRequest `json:"stops"`
	// ScheduledAt is phase B9: omit (or send the zero value) for an
	// immediate booking. RFC3339, e.g. "2026-09-01T14:30:00Z".
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`

	// Parcel mode. ride_purpose omitted or "passenger" is a normal ride; the
	// package fields are only valid alongside "parcel".
	RidePurpose   string `json:"ride_purpose"`
	ReceiverName  string `json:"receiver_name"`
	ReceiverPhone string `json:"receiver_phone"`
	PackageDesc   string `json:"package_desc"`
	PackageType   string `json:"package_type"`
	PackageSize   string `json:"package_size"`
	IsFragile     bool   `json:"is_fragile"`

	// Section 7. service_product_code is optional (the vehicle class maps to
	// its legacy product). requirements are hard gates; preferences are
	// informational and never restrict matching.
	ServiceProductCode string                 `json:"service_product_code"`
	Requirements       *requirementsRequest   `json:"requirements,omitempty"`
	Preferences        *ridePreferenceRequest `json:"preferences,omitempty"`
}

type requirementsRequest struct {
	WheelchairRequired *bool `json:"wheelchair_required"`
	ChildSeatRequired  *bool `json:"child_seat_required"`
	PetTravel          *bool `json:"pet_travel"`
}

type ridePreferenceRequest struct {
	QuietRide        *bool  `json:"quiet_ride"`
	PreferredChannel string `json:"preferred_channel"`
	Luggage          string `json:"luggage"`
}

func (r createBookingRequest) toInput(customerID string) bookingusecases.CreateBookingInput {
	stops := make([]bookingusecases.StopInput, len(r.Stops))
	for i, s := range r.Stops {
		stops[i] = bookingusecases.StopInput{Address: s.Address, Lat: s.Lat, Lng: s.Lng}
	}
	return bookingusecases.CreateBookingInput{
		CustomerID:              customerID,
		VehicleClass:            r.VehicleClass,
		BookingMode:             r.BookingMode,
		PassengerCount:          r.PassengerCount,
		RideNotes:               r.RideNotes,
		PaymentMethod:           r.PaymentMethod,
		ReservedDurationMinutes: r.ReservedDurationMinutes,
		Stops:                   stops,
		ScheduledAt:             r.ScheduledAt,
		RidePurpose:             r.RidePurpose,
		ReceiverName:            r.ReceiverName,
		ReceiverPhone:           r.ReceiverPhone,
		PackageDesc:             r.PackageDesc,
		PackageType:             r.PackageType,
		PackageSize:             r.PackageSize,
		IsFragile:               r.IsFragile,
		ServiceProductCode:      r.ServiceProductCode,
		HardRequirements:        r.hardRequirements(),
		Preferences:             r.preferences(),
	}
}

func (r createBookingRequest) hardRequirements() capability.HardRequirementInput {
	if r.Requirements == nil {
		return capability.HardRequirementInput{}
	}
	return capability.HardRequirementInput{WheelchairRequired: r.Requirements.WheelchairRequired,
		ChildSeatRequired: r.Requirements.ChildSeatRequired, PetTravel: r.Requirements.PetTravel}
}

func (r createBookingRequest) preferences() capability.RidePreferenceInput {
	if r.Preferences == nil {
		return capability.RidePreferenceInput{}
	}
	return capability.RidePreferenceInput{QuietRide: r.Preferences.QuietRide,
		PreferredChannel: r.Preferences.PreferredChannel, Luggage: r.Preferences.Luggage}
}

type estimateFareRequest struct {
	PickupLat  float64 `json:"pickup_lat"`
	PickupLng  float64 `json:"pickup_lng"`
	DropoffLat float64 `json:"dropoff_lat"`
	DropoffLng float64 `json:"dropoff_lng"`
}

func (r estimateFareRequest) toInput() bookingusecases.EstimateFareInput {
	return bookingusecases.EstimateFareInput{
		PickupLat: r.PickupLat, PickupLng: r.PickupLng,
		DropoffLat: r.DropoffLat, DropoffLng: r.DropoffLng,
	}
}

type bookingListResponse struct {
	Bookings []bookingmodels.PublicBooking `json:"bookings"`
}

type fareEstimateResponse struct {
	Fares []bookingusecases.FareTile `json:"fares"`
}

type completeTripRequest struct {
	ActualDistanceKm  float64 `json:"actual_distance_km"`
	ActualDurationMin int     `json:"actual_duration_min"`
}

func (r completeTripRequest) toInput(bookingID, providerID string) bookingusecases.CompleteBookingInput {
	return bookingusecases.CompleteBookingInput{
		BookingID: bookingID, ProviderID: providerID,
		ActualDistanceKm: r.ActualDistanceKm, ActualDurationMin: r.ActualDurationMin,
	}
}

type cancelBookingRequest struct {
	Reason string `json:"reason"`
}

type verifyPickupRequest struct {
	Code string `json:"code"`
}

// verifyDeliveryRequest is the driver's drop-off submission for a parcel.
type verifyDeliveryRequest struct {
	Code string `json:"code"`
}

// locationPointRequest is one GPS fix in a POST /provider/location/events
// batch. RecordedAt is RFC3339 (encoding/json's native time.Time format) --
// when the driver device actually captured the fix, which may lag well
// behind when this request reaches the server (a queued-while-offline
// point flushed later).
type locationPointRequest struct {
	BookingID     string    `json:"booking_id"`
	Sequence      int64     `json:"sequence"`
	Lat           float64   `json:"lat"`
	Lng           float64   `json:"lng"`
	Accuracy      float64   `json:"accuracy"`
	Speed         float64   `json:"speed"`
	Heading       float64   `json:"heading"`
	RecordedAt    time.Time `json:"recorded_at"`
	Source        string    `json:"source"`
	ClientEventID string    `json:"client_event_id"`
}

type ingestLocationPointsRequest struct {
	Points []locationPointRequest `json:"points"`
}

func (r ingestLocationPointsRequest) toInputs() []bookingusecases.LocationPointInput {
	inputs := make([]bookingusecases.LocationPointInput, len(r.Points))
	for i, p := range r.Points {
		recordedAt := p.RecordedAt
		if recordedAt.IsZero() {
			// A device that omits recorded_at gets its fix timestamped at
			// ingest time rather than being treated as decades-stale
			// (the zero time.Time is year 1) -- see FlagStale in
			// bookingtelemetry.Classify, which would otherwise flag every
			// such point.
			recordedAt = time.Now()
		}
		inputs[i] = bookingusecases.LocationPointInput{
			BookingID: p.BookingID, Sequence: p.Sequence,
			Lat: p.Lat, Lng: p.Lng,
			Accuracy: p.Accuracy, Speed: p.Speed, Heading: p.Heading,
			RecordedAt: recordedAt, Source: p.Source, ClientEventID: p.ClientEventID,
		}
	}
	return inputs
}

type locationPointResultResponse struct {
	Results []bookingusecases.LocationPointResult `json:"results"`
}

// submitReviewRequest is shared by both POST .../review endpoints -- the
// authenticated caller's role (decided by which route was hit, customer or
// provider) determines the direction, not a field in this body.
type submitReviewRequest struct {
	Rating           int      `json:"rating"`
	ReviewText       string   `json:"review_text"`
	Tags             []string `json:"tags"`
	RecommendsDriver *bool    `json:"recommends_driver"`
}

func (r submitReviewRequest) toInput() bookingusecases.SubmitReviewInput {
	return bookingusecases.SubmitReviewInput{
		Rating: r.Rating, ReviewText: r.ReviewText, Tags: r.Tags, RecommendsDriver: r.RecommendsDriver,
	}
}
