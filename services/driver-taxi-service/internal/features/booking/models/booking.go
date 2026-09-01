// Package bookingmodels defines the ride booking aggregate: the booking
// itself, its ordered stops, and the status ladder every transition moves
// along. Booking creation is B1; the guarded transitions that move a booking
// along this ladder are B3, matching is B2, and so on -- this file defines
// the shape every later phase builds on.
package bookingmodels

import "time"

// Status is the ride lifecycle. Every transition between these is a guarded
// compare-and-set in SQL (invariant §4 in the build plan) -- read-then-write
// is never permitted, and a transition affecting zero rows is a conflict, not
// a silent success.
type Status string

const (
	StatusScheduled       Status = "scheduled" // pre-entry: not yet dispatched to matching (phase B9)
	StatusPendingMatch    Status = "pending_match"
	StatusAwaitingAccept  Status = "awaiting_acceptance"
	StatusAccepted        Status = "accepted"
	StatusEnRoutePickup   Status = "en_route_pickup"
	StatusArrivedAtPickup Status = "arrived_at_pickup"
	// StatusPickupVerificationPending is declared per the addendum §9.1
	// vocabulary but is NOT assigned to taxi_bookings.status by this build
	// (phase B5): the driver's real-world action of arriving and the
	// verification window opening are the same moment (there is no separate
	// button between them), so the booking's coarse status stays at
	// StatusArrivedAtPickup for the whole verification window. The finer
	// pending/verified sub-state is tracked by PickupVerificationStatus
	// below instead, which IS actively read/written. Kept here rather than
	// removed in case a future phase needs it as a real, distinct hop.
	StatusPickupVerificationPending Status = "pickup_verification_pending"
	StatusInProgress                Status = "in_progress"
	StatusCompleted                 Status = "completed"

	// Terminal states.
	StatusCancelled Status = "cancelled"
	StatusUnmatched Status = "unmatched"
)

// ActiveStatuses are the statuses the addendum §5.2 customer active-ride
// guard blocks a second booking against -- a customer "physically travelling
// on" an active ride, in the addendum's own words. Deliberately narrower than
// "every non-terminal status": StatusScheduled is excluded, because a ride
// booked for tomorrow does not conflict with booking a ride right now.
//
// Sponsored/family bookings (addendum §22, a distinct passenger reference
// exempting this guard) are out of scope for this build -- see the build
// plan's exclusion list -- so this guard is unconditional or none.
var ActiveStatuses = []Status{
	StatusPendingMatch, StatusAwaitingAccept, StatusAccepted,
	StatusEnRoutePickup, StatusArrivedAtPickup,
	StatusPickupVerificationPending, StatusInProgress,
}

// PickupVerificationStatus tracks the addendum §9 handshake independently of
// the booking's own Status, since "verification pending" and "ride status"
// are two different questions once phase B5 is built.
type PickupVerificationStatus string

const (
	PickupVerificationNotRequired PickupVerificationStatus = "not_required"
	PickupVerificationPending     PickupVerificationStatus = "pending"
	PickupVerificationVerified    PickupVerificationStatus = "verified"
)

// RidePurpose distinguishes a ride that carries a PASSENGER from one that
// carries a PACKAGE. It is deliberately NOT a third BookingMode value:
// BookingMode answers "standard point-to-point or hourly reservation", which
// is a different axis entirely, and folding the two together would make
// "hourly parcel" inexpressible by construction rather than by validation.
type RidePurpose string

const (
	RidePurposePassenger RidePurpose = "passenger"
	RidePurposeParcel    RidePurpose = "parcel"
)

// DeliveryVerificationStatus tracks the drop-off half of a parcel's handover,
// mirroring PickupVerificationStatus for the pickup half. The two are separate
// columns rather than one because they move independently: pickup is verified
// at the kerb before the trip starts, delivery at the destination before it
// completes.
//
// A passenger ride stays DeliveryVerificationNotRequired forever -- there is
// nothing to hand over at the end of a ride.
type DeliveryVerificationStatus string

const (
	DeliveryVerificationNotRequired DeliveryVerificationStatus = "not_required"
	DeliveryVerificationPending     DeliveryVerificationStatus = "pending"
	DeliveryVerificationVerified    DeliveryVerificationStatus = "verified"
)

// Payment status values (phase B4). Every booking is wallet-paid -- cash was
// removed so the platform actually collects the fare, matching hauling and
// dispatch. A booking stays PaymentUnpaid until a driver accepts (and stays
// there permanently when payments are not configured at all, i.e. local dev
// without TAXI_PAYMENT_URL). PaymentHeld means PayFromWallet succeeded (money
// is escrowed, not yet the driver's); PaymentPaid means CompleteJob settled
// it to the driver; PaymentRefunded means RequestRefund succeeded after a
// cancel that happened while money was held.
const (
	PaymentUnpaid   = "unpaid"
	PaymentHeld     = "held"
	PaymentPaid     = "paid"
	PaymentRefunded = "refunded"
)

// StopType is a position in the ordered route. A STANDARD booking always has
// exactly one PICKUP (sequence 1) and one DROPOFF (the last sequence);
// WAYPOINTs sit between them for HOURLY/multi-stop itineraries.
type StopType string

const (
	StopTypePickup   StopType = "PICKUP"
	StopTypeWaypoint StopType = "WAYPOINT"
	StopTypeDropoff  StopType = "DROPOFF"
)

type StopStatus string

const (
	StopStatusPending   StopStatus = "pending"
	StopStatusArrived   StopStatus = "arrived"
	StopStatusCompleted StopStatus = "completed"
	StopStatusSkipped   StopStatus = "skipped"
)

// Stop is one ordered location in a booking's route.
type Stop struct {
	ID          string
	BookingID   string
	Sequence    int
	Type        StopType
	Address     string
	Lat         float64
	Lng         float64
	Status      StopStatus
	ArrivedAt   *time.Time
	DepartedAt  *time.Time
	WaitSeconds *int
	Notes       string
}

// BookingMode distinguishes a standard point-to-point ride from an hourly/
// car-hire reservation (addendum §16). Only the data model exists in this
// build phase -- overtime/over-distance calculation is pricing-owned per
// addendum §16 and §37, and reservation activation logic is out of scope.
type BookingMode string

const (
	BookingModeStandard BookingMode = "standard"
	BookingModeHourly   BookingMode = "hourly"
)

// Booking is the ride aggregate. Fare fields are always a SNAPSHOT from
// PricingClient (internal/pricing) -- never computed or recomputed here.
type Booking struct {
	ID         string
	CustomerID string
	ProviderID *string
	VehicleID  *string

	MarketID        string
	OperatingZoneID string

	BookingMode             BookingMode
	VehicleClass            string
	PassengerCount          int
	RideNotes               string
	ReservedDurationMinutes *int

	FareEstimateKobo      *int64
	FareFinalKobo         *int64
	PricingQuoteReference string
	PricingVersion        string
	DistanceKm            *float64
	DurationMinutes       *int
	ActualDistanceKm      *float64

	PaymentMethod   string
	PaymentStatus   string
	PaymentIntentID *string

	Status                   Status
	PickupVerificationStatus PickupVerificationStatus

	// Parcel mode. Every package field is empty on a passenger ride, which
	// CreateBooking enforces rather than merely assuming.
	RidePurpose                RidePurpose
	ReceiverName               string
	ReceiverPhone              string
	PackageDesc                string
	PackageType                string
	PackageSize                string
	IsFragile                  bool
	DeliveryVerificationStatus DeliveryVerificationStatus

	ScheduledAt         *time.Time
	DispatchStartAt     *time.Time
	ReservationStatus   string
	ReservedProviderID  *string
	BackupDispatchAt    *time.Time
	ProviderConfirmedAt *time.Time
	CustomerReadyWindow *int
	// MatchingStartedAt is when active searching for a driver began -- set
	// at CreateBooking time for an immediate ride, at promotion time
	// (ScheduledDispatchWorker) for a scheduled one. The matcher's own
	// search-window deadline (matching.go) is anchored here, NEVER to
	// CreatedAt: a ride booked a day ahead of pickup has a CreatedAt far in
	// the past by the time it is actually promoted to matching, and reusing
	// it would leave the search deadline already expired the instant
	// searching begins.
	MatchingStartedAt *time.Time

	CancelReason *string
	CancelledBy  *string

	MatchedAt   *time.Time
	AcceptedAt  *time.Time
	ArrivedAt   *time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	CancelledAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Stops is populated by repository reads that fetch the full route; a
	// read that only needs the booking row itself may leave this nil.
	Stops []Stop
}

// PublicStop is the passenger/driver-facing view of a Stop.
type PublicStop struct {
	ID       string     `json:"id"`
	Sequence int        `json:"sequence"`
	Type     StopType   `json:"type"`
	Address  string     `json:"address"`
	Lat      float64    `json:"lat"`
	Lng      float64    `json:"lng"`
	Status   StopStatus `json:"status"`
}

func (s Stop) Public() PublicStop {
	return PublicStop{
		ID: s.ID, Sequence: s.Sequence, Type: s.Type,
		Address: s.Address, Lat: s.Lat, Lng: s.Lng, Status: s.Status,
	}
}

// PublicBooking is the wire shape returned to both the passenger and driver
// apps. Neither payment_intent_id nor any pickup-verification secret ever
// appears here -- those are handler-level exclusions, not just JSON tags,
// enforced once phase B5 exists.
type PublicBooking struct {
	ID         string  `json:"id"`
	CustomerID string  `json:"customer_id"`
	ProviderID *string `json:"provider_id,omitempty"`
	VehicleID  *string `json:"vehicle_id,omitempty"`

	MarketID        string `json:"market_id"`
	OperatingZoneID string `json:"operating_zone_id"`

	BookingMode             BookingMode `json:"booking_mode"`
	VehicleClass            string      `json:"vehicle_class"`
	PassengerCount          int         `json:"passenger_count"`
	RideNotes               string      `json:"ride_notes"`
	ReservedDurationMinutes *int        `json:"reserved_duration_minutes,omitempty"`

	FareEstimateKobo *int64   `json:"fare_estimate_kobo,omitempty"`
	FareFinalKobo    *int64   `json:"fare_final_kobo,omitempty"`
	DistanceKm       *float64 `json:"distance_km,omitempty"`
	DurationMinutes  *int     `json:"duration_minutes,omitempty"`

	PaymentMethod string `json:"payment_method"`
	PaymentStatus string `json:"payment_status"`

	Status                   Status                   `json:"status"`
	PickupVerificationStatus PickupVerificationStatus `json:"pickup_verification_status"`

	// The package fields carry omitempty so a passenger ride's wire shape is
	// byte-identical to what it was before parcel mode existed.
	RidePurpose                RidePurpose                `json:"ride_purpose"`
	ReceiverName               string                     `json:"receiver_name,omitempty"`
	ReceiverPhone              string                     `json:"receiver_phone,omitempty"`
	PackageDesc                string                     `json:"package_desc,omitempty"`
	PackageType                string                     `json:"package_type,omitempty"`
	PackageSize                string                     `json:"package_size,omitempty"`
	IsFragile                  bool                       `json:"is_fragile,omitempty"`
	DeliveryVerificationStatus DeliveryVerificationStatus `json:"delivery_verification_status"`

	// ScheduledAt/DispatchStartAt are the only B9 scheduling fields exposed
	// to clients. ReservationStatus/ReservedProviderID/BackupDispatchAt/
	// ProviderConfirmedAt stay internal: this build never sets
	// ReservationStatus away from its 'none' default (see the build plan's
	// own B9 notes on why "reserve a specific driver in advance" is out of
	// scope here), so exposing them would show a client permanently-inert
	// fields rather than real state.
	ScheduledAt     *time.Time `json:"scheduled_at,omitempty"`
	DispatchStartAt *time.Time `json:"dispatch_start_at,omitempty"`

	CancelReason *string `json:"cancel_reason,omitempty"`
	CancelledBy  *string `json:"cancelled_by,omitempty"`

	MatchedAt   *time.Time `json:"matched_at,omitempty"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	ArrivedAt   *time.Time `json:"arrived_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`

	Stops []PublicStop `json:"stops,omitempty"`
}

func (b Booking) Public() PublicBooking {
	pub := PublicBooking{
		ID: b.ID, CustomerID: b.CustomerID, ProviderID: b.ProviderID, VehicleID: b.VehicleID,
		MarketID: b.MarketID, OperatingZoneID: b.OperatingZoneID,
		BookingMode: b.BookingMode, VehicleClass: b.VehicleClass,
		PassengerCount: b.PassengerCount, RideNotes: b.RideNotes,
		ReservedDurationMinutes:  b.ReservedDurationMinutes,
		FareEstimateKobo:         b.FareEstimateKobo,
		FareFinalKobo:            b.FareFinalKobo,
		DistanceKm:               b.DistanceKm,
		DurationMinutes:          b.DurationMinutes,
		PaymentMethod:            b.PaymentMethod,
		PaymentStatus:            b.PaymentStatus,
		Status:                   b.Status,
		PickupVerificationStatus: b.PickupVerificationStatus,
		RidePurpose:              b.RidePurpose,
		ReceiverName:             b.ReceiverName,
		ReceiverPhone:            b.ReceiverPhone,
		PackageDesc:              b.PackageDesc,
		PackageType:              b.PackageType,
		PackageSize:              b.PackageSize,
		IsFragile:                b.IsFragile,

		DeliveryVerificationStatus: b.DeliveryVerificationStatus,
		ScheduledAt:                b.ScheduledAt,
		DispatchStartAt:            b.DispatchStartAt,
		CancelReason:               b.CancelReason,
		CancelledBy:                b.CancelledBy,
		MatchedAt:                  b.MatchedAt,
		AcceptedAt:                 b.AcceptedAt,
		ArrivedAt:                  b.ArrivedAt,
		StartedAt:                  b.StartedAt,
		CompletedAt:                b.CompletedAt,
		CancelledAt:                b.CancelledAt,
		CreatedAt:                  b.CreatedAt,
	}
	if b.Stops != nil {
		pub.Stops = make([]PublicStop, len(b.Stops))
		for i, s := range b.Stops {
			pub.Stops[i] = s.Public()
		}
	}
	return pub
}

// IsParcel reports whether this booking carries a package rather than a
// passenger. Treats the empty string as passenger so a row written before
// parcel mode existed reads correctly.
func (b Booking) IsParcel() bool { return b.RidePurpose == RidePurposeParcel }

// IsActive reports whether this booking counts against the customer
// active-ride guard.
func (b Booking) IsActive() bool {
	for _, s := range ActiveStatuses {
		if b.Status == s {
			return true
		}
	}
	return false
}

// IsTerminal reports whether no further transition is possible.
func (b Booking) IsTerminal() bool {
	return b.Status == StatusCompleted || b.Status == StatusCancelled || b.Status == StatusUnmatched
}

// BookingEvent is one row of a booking's audit trail (phase B10 route
// replay), written inside the SAME transaction as every guarded lifecycle
// transition -- see withTransition in usecases/lifecycle.go. ActorType is
// "customer", "provider", or "system" (the matcher/scheduler acting on
// nobody's behalf); Metadata is a small, event-specific JSON blob, never a
// second copy of the full booking row (that already exists at
// GET /bookings/:id -- an event records what CHANGED, not the whole state).
type BookingEvent struct {
	ID        string
	BookingID string
	EventType string
	ActorType string
	ActorID   string
	Metadata  []byte
	CreatedAt time.Time
}
