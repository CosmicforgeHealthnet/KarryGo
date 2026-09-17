// Package bookingusecases owns booking creation, the read paths a
// passenger/driver client needs, and (lifecycle.go) the post-accept status
// ladder: en-route, arrived, start, complete, cancel.
package bookingusecases

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/adminaudit"
	"cosmicforge/logistics/services/taxi-service/internal/capability"
	bookingmatching "cosmicforge/logistics/services/taxi-service/internal/features/booking/matching"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	bookingreviews "cosmicforge/logistics/services/taxi-service/internal/features/booking/reviews"
	bookingtelemetry "cosmicforge/logistics/services/taxi-service/internal/features/booking/telemetry"
	bookingverification "cosmicforge/logistics/services/taxi-service/internal/features/booking/verification"
	marketmodels "cosmicforge/logistics/services/taxi-service/internal/features/markets/models"
	"cosmicforge/logistics/services/taxi-service/internal/movement"
	"cosmicforge/logistics/services/taxi-service/internal/pricing"
	"cosmicforge/logistics/services/taxi-service/internal/routing"
	"cosmicforge/logistics/services/taxi-service/internal/vehicleclass"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/phonenumber"
)

// MarketLookup resolves which operating zone a coordinate belongs to. Booking
// creation resolves it from the PICKUP stop specifically -- a multi-stop
// itinerary is priced and matched from where the passenger actually is, not
// wherever the route eventually ends up.
type MarketLookup interface {
	ResolveZone(ctx context.Context, point geo.Point) (marketmodels.Zone, error)
}

// MatchingConfig tunes the matcher loop (phase B2). Every duration/radius
// here is configuration, sourced from cmd/main.go's env-driven config --
// never a hard-coded constant -- so tuning the search behavior never needs a
// code change.
type MatchingConfig struct {
	// MatchTimeout is how long an offered driver has to accept before the
	// matcher requeues the booking to the next candidate.
	MatchTimeout time.Duration
	// SearchWindow is the total time (measured from booking.CreatedAt) the
	// matcher keeps searching before giving up and marking the booking
	// unmatched.
	SearchWindow time.Duration
	// BaseRadiusKm is the initial candidate search radius.
	BaseRadiusKm float64
	// ExpandedRadiusKm is used once a scan at BaseRadiusKm finds nothing --
	// a minimal, honest implementation of the addendum's "wave 2: expand
	// radius" behavior. There is no wave-3 "relax soft constraints" step in
	// this build: the addendum names the concept but this build has no
	// concrete soft constraint to relax yet, and a hollow struct field that
	// does nothing would be worse than not claiming the behavior at all.
	ExpandedRadiusKm float64
	// PollInterval is how often the matcher rescans for candidates when none
	// are found, and how often it checks an outstanding offer's status while
	// waiting for the driver to respond.
	PollInterval time.Duration
	// InstanceID identifies this process for the booking-level matcher
	// lease (internal/features/booking/matching) -- must be stable for the
	// life of the process, unique across horizontally-scaled instances.
	InstanceID string
}

// VerificationConfig tunes pickup verification (phase B5). HandoverSecret
// signs/verifies the code (shared/go/handover); RateWindow/MaxAttempts feed
// the Redis burst-rate limiter, layered on top of handover's own durable
// per-code attempt cap.
type VerificationConfig struct {
	HandoverSecret []byte
	RateWindow     time.Duration
	MaxAttempts    int
}

// SchedulingConfig tunes phase B9. LeadTime is how far ahead of a scheduled
// pickup ScheduledDispatchWorker promotes the ride to active matching --
// CreateBooking computes DispatchStartAt as ScheduledAt.Add(-LeadTime) at
// creation time, once, rather than the worker recomputing it every sweep.
type SchedulingConfig struct {
	LeadTime time.Duration
}

type BookingService struct {
	bookings         bookingrepositories.BookingRepository
	markets          MarketLookup
	pricing          pricing.PricingClient
	operationalState OperationalStateStore

	availability AvailabilityCandidateStore
	providers    ProviderComplianceLookup
	vehicles     VehicleFitLookup
	routes       routing.RoutingClient
	lease        *bookingmatching.MatcherLease
	matching     MatchingConfig

	pickupVerifications bookingverification.Repository
	verifyRateLimiter   *bookingverification.RateLimiter
	verification        VerificationConfig

	payments PaymentClient

	telemetry       bookingtelemetry.Repository
	positions       PositionUpdater
	telemetryConfig bookingtelemetry.Config

	reviews            bookingreviews.Repository
	reviewRevealWindow time.Duration
	operationalHistory OperationalHistoryLookup

	scheduling SchedulingConfig

	adminAudit adminaudit.Repository

	// managedSettlement and managedHandoff are the Section 12 boundary. See
	// managed_settlement.go: they decide whether a completed corporate ride
	// takes the legacy direct-to-provider wallet path or the Fleet managed
	// path, and that decision is never made by a live Fleet lookup at
	// completion time.
	managedSettlement ManagedSettlementConfig
	managedHandoff    ManagedSettlementHandoff
	// completedWorkEnabled gates writing the canonical Fleet completed-work
	// outbox row. Off by default (TAXI_FLEET_COMPLETED_WORK_ENABLED).
	completedWorkEnabled bool
	movement   *movement.Writer
	capability CapabilityGate
	riskStore  ScheduledRiskStore

	products        ProductPolicySource
	customerContext CustomerContextSource
}

// CapabilityGate runs Section 7 hard eligibility gates before scoring. It
// returns the provider ids allowed into ranking. In shadow mode it returns
// the input unchanged after recording evidence.
type CapabilityGate interface {
	FilterPassengerCandidates(ctx context.Context, booking bookingmodels.Booking, productCode string, snapshot []byte, providerIDs []string, instance string) ([]string, error)
}

type Options struct {
	Bookings         bookingrepositories.BookingRepository
	Markets          MarketLookup
	Pricing          pricing.PricingClient
	OperationalState OperationalStateStore

	// Availability/Providers/Vehicles/Routes/Lease are nil-able: a
	// BookingService built without them (e.g. in a unit test that never
	// exercises matching) still works for CreateBooking/EstimateFare/
	// lifecycle transitions. CreateBooking only spawns the matcher when
	// Lease is non-nil -- see spawnMatcher in matching.go.
	Availability AvailabilityCandidateStore
	Providers    ProviderComplianceLookup
	Vehicles     VehicleFitLookup
	Routes       routing.RoutingClient
	Lease        *bookingmatching.MatcherLease
	Matching     MatchingConfig

	// PickupVerifications/VerifyRateLimiter are nil-able the same way: a
	// service built without them can still do everything except
	// MarkArrivedAtPickup/GetPickupCode/ResendPickupCode/VerifyPickup (which
	// will return a clear internal error rather than panic -- see their own
	// nil checks in verification.go).
	PickupVerifications bookingverification.Repository
	VerifyRateLimiter   *bookingverification.RateLimiter
	Verification        VerificationConfig

	// Payments is nil-able the same way: without it, every wallet call is a
	// silent no-op (see payments.go's own per-method doc comments), so local
	// dev without TAXI_PAYMENT_URL/_SECRET still runs the full ride
	// lifecycle, just without ever actually charging anyone -- the same
	// "disabled integration is a safe no-op" contract this service already
	// documents for TAXI_NOTIFICATION_URL/TAXI_ROUTES_API_KEY.
	Payments PaymentClient

	// Telemetry/Positions are nil-able the same way: without Telemetry,
	// IngestLocationPoints returns apperrors.Unavailable rather than
	// panicking (see its own nil check); without Positions, a validated
	// point is still durably stored, it just never mirrors into the
	// live-tracking Redis state (see telemetry.go's own comment on that
	// call).
	Telemetry       bookingtelemetry.Repository
	Positions       PositionUpdater
	TelemetryConfig bookingtelemetry.Config

	// Reviews/ReviewRevealWindow/OperationalHistory are nil-able the same
	// way: without Reviews, every review method returns
	// apperrors.Unavailable; without OperationalHistory, earnings insights
	// does the same. ReviewRevealWindow of zero disables the "reveal after
	// a window elapses" half of the rule (a zero-value Duration comparison
	// -- see revealTheirs) -- text/tags only ever unlock once BOTH sides
	// have submitted, never on a timer.
	Reviews            bookingreviews.Repository
	ReviewRevealWindow time.Duration
	OperationalHistory OperationalHistoryLookup

	// Scheduling is nil-value-safe (a zero SchedulingConfig has LeadTime 0),
	// but a service wired for production should always set LeadTime from
	// TAXI_SCHEDULE_LEAD_MINUTES -- see CreateBooking's own validation for
	// what a zero lead time actually means (dispatch right at the scheduled
	// time, not before it).
	Scheduling SchedulingConfig

	// AdminAudit is nil-able the same way: without it, every admin action
	// (cancel/force-recover/pause-provider/release-hold) returns
	// apperrors.Unavailable rather than silently skipping the audit trail
	// an operator action must always leave.
	AdminAudit adminaudit.Repository

	// ManagedSettlement/ManagedHandoff/CompletedWorkEnabled configure the
	// Section 8 corporate completion path. All three are nil/zero-safe: a
	// service built without them behaves exactly as before, which is what
	// keeps every independent ride unchanged.
	ManagedSettlement    ManagedSettlementConfig
	ManagedHandoff       ManagedSettlementHandoff
	CompletedWorkEnabled bool
	// Movement publishes customer movement summaries through the outbox.
	// nil or disabled is a safe no-op.
	Movement *movement.Writer
	// CapabilityGate is nil when Section 7 passenger capability matching is
	// not wired; matching then behaves exactly as before.
	CapabilityGate CapabilityGate
	// RiskStore backs the scheduled-ride risk monitor. nil disables it.
	RiskStore ScheduledRiskStore
	// ProductPolicy freezes a service product and requirement snapshot onto
	// every new booking. nil keeps legacy class-only bookings.
	ProductPolicy ProductPolicySource
	// CustomerContext supplies saved customer mobility needs. nil skips the
	// handoff; explicit request requirements still apply.
	CustomerContext CustomerContextSource
}

func NewBookingService(opts Options) *BookingService {
	return &BookingService{
		bookings: opts.Bookings, markets: opts.Markets, pricing: opts.Pricing, operationalState: opts.OperationalState,
		availability: opts.Availability, providers: opts.Providers, vehicles: opts.Vehicles,
		routes: opts.Routes, lease: opts.Lease, matching: opts.Matching,
		pickupVerifications: opts.PickupVerifications, verifyRateLimiter: opts.VerifyRateLimiter, verification: opts.Verification,
		payments:  opts.Payments,
		telemetry: opts.Telemetry, positions: opts.Positions, telemetryConfig: opts.TelemetryConfig,
		reviews: opts.Reviews, reviewRevealWindow: opts.ReviewRevealWindow, operationalHistory: opts.OperationalHistory,
		scheduling:        opts.Scheduling,
		adminAudit:        opts.AdminAudit,
		managedSettlement: opts.ManagedSettlement, managedHandoff: opts.ManagedHandoff,
		completedWorkEnabled: opts.CompletedWorkEnabled,
		movement:   opts.Movement,
		capability: opts.CapabilityGate,
		riskStore:  opts.RiskStore,

		products:        opts.ProductPolicy,
		customerContext: opts.CustomerContext,
	}
}

// StopInput is one caller-supplied route point. Type is accepted but not
// trusted -- see normalizeStops.
type StopInput struct {
	Address string
	Lat     float64
	Lng     float64
}

type CreateBookingInput struct {
	CustomerID     string
	VehicleClass   string
	BookingMode    string
	PassengerCount int
	RideNotes      string
	PaymentMethod  string
	// ReservedDurationMinutes is hourly-mode only; ignored for standard rides.
	ReservedDurationMinutes int
	// RidePurpose is "passenger" (the default when empty) or "parcel". The
	// package fields below are required for a parcel and must be empty for a
	// passenger ride -- CreateBooking enforces both directions rather than
	// quietly ignoring stray values.
	RidePurpose   string
	ReceiverName  string
	ReceiverPhone string
	PackageDesc   string
	PackageType   string
	PackageSize   string
	IsFragile     bool
	// Stops must contain at least two points: pickup first, dropoff last.
	// Anything between is a waypoint. A STANDARD booking is expected to send
	// exactly two; HOURLY may send more.
	Stops []StopInput
	// ScheduledAt is phase B9: nil (the default) dispatches immediately, a
	// future timestamp defers matching until ScheduleLeadMinutes before it
	// (see ScheduledDispatchWorker). Must be strictly after now -- a
	// scheduled ride in the past is a client bug, not a same-as-immediate
	// booking.
	ScheduledAt *time.Time
	// ServiceProductCode selects a Section 7 product. Empty maps the vehicle
	// class to its legacy product.
	ServiceProductCode string
	// HardRequirements gate matching. A nil field is "not stated" and may be
	// filled from the customer's saved mobility needs.
	HardRequirements capability.HardRequirementInput
	// Preferences are informational and never gate matching.
	Preferences capability.RidePreferenceInput
}

// CreateBooking validates the request, enforces the addendum §5.2 one-active-
// ride guard, resolves the pickup's market/zone, prices the single requested
// class, and persists the booking with its stops in one transaction.
//
// New bookings created here always start at StatusPendingMatch --
// StatusScheduled handling belongs to phase B9, which is additive on top of
// this path, not a branch inside it.
// normalizeRidePurpose validates the passenger/parcel split and returns the
// purpose plus a normalized receiver phone. Both directions are enforced: a
// parcel must carry its package and receiver detail, and a passenger ride must
// carry none of it -- silently dropping stray package fields on a passenger
// ride would let a client believe it booked a delivery that nobody will treat
// as one.
func normalizeRidePurpose(input CreateBookingInput, mode bookingmodels.BookingMode, passengerCount int) (bookingmodels.RidePurpose, string, []apperrors.FieldViolation) {
	var violations []apperrors.FieldViolation

	purpose := bookingmodels.RidePurpose(strings.ToLower(strings.TrimSpace(input.RidePurpose)))
	if purpose == "" {
		purpose = bookingmodels.RidePurposePassenger
	}
	if purpose != bookingmodels.RidePurposePassenger && purpose != bookingmodels.RidePurposeParcel {
		return "", "", []apperrors.FieldViolation{
			{Field: "ride_purpose", Message: "Ride purpose must be 'passenger' or 'parcel'."},
		}
	}

	if purpose == bookingmodels.RidePurposePassenger {
		// Stray package detail on a passenger ride is a client bug.
		if strings.TrimSpace(input.PackageDesc) != "" ||
			strings.TrimSpace(input.PackageType) != "" ||
			strings.TrimSpace(input.PackageSize) != "" ||
			strings.TrimSpace(input.ReceiverName) != "" ||
			strings.TrimSpace(input.ReceiverPhone) != "" ||
			input.IsFragile {
			violations = append(violations, apperrors.FieldViolation{
				Field: "ride_purpose", Message: "Package details are only valid for a package delivery.",
			})
		}
		return purpose, "", violations
	}

	if strings.TrimSpace(input.PackageDesc) == "" {
		violations = append(violations, apperrors.FieldViolation{
			Field: "package_desc", Message: "Describe what is being sent.",
		})
	}
	if strings.TrimSpace(input.ReceiverName) == "" {
		violations = append(violations, apperrors.FieldViolation{
			Field: "receiver_name", Message: "Who is receiving the package?",
		})
	}

	// Normalized HERE, at creation, not at verification time: dispatch shipped
	// exactly that bug and it made bookings unfinishable, because the stored
	// raw string never matched the normalized one submitted at drop-off.
	receiver := ""
	rawPhone := strings.TrimSpace(input.ReceiverPhone)
	if rawPhone == "" {
		violations = append(violations, apperrors.FieldViolation{
			Field: "receiver_phone", Message: "A receiver phone number is required.",
		})
	} else {
		normalized, err := phonenumber.NormalizeNigerianPhoneNumber(rawPhone)
		if err != nil {
			violations = append(violations, apperrors.FieldViolation{
				Field: "receiver_phone", Message: "Enter a valid Nigerian phone number.",
			})
		} else {
			receiver = normalized
		}
	}

	// A package occupies the car; it does not travel with passengers, and an
	// hourly parcel reservation is not a product this build offers.
	if passengerCount > 1 {
		violations = append(violations, apperrors.FieldViolation{
			Field: "passenger_count", Message: "A package delivery cannot carry passengers.",
		})
	}
	if mode != bookingmodels.BookingModeStandard {
		violations = append(violations, apperrors.FieldViolation{
			Field: "booking_mode", Message: "A package delivery must be a standard booking.",
		})
	}

	return purpose, receiver, violations
}

func (s *BookingService) CreateBooking(ctx context.Context, input CreateBookingInput) (bookingmodels.Booking, error) {
	customerID := strings.TrimSpace(input.CustomerID)
	if customerID == "" {
		return bookingmodels.Booking{}, apperrors.Unauthorized("Authentication is required.", nil)
	}

	stops, err := normalizeStops(input.Stops)
	if err != nil {
		return bookingmodels.Booking{}, err
	}

	class := vehicleclass.Normalize(input.VehicleClass)
	spec, ok := vehicleclass.Get(class)
	if !ok {
		return bookingmodels.Booking{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "vehicle_class", Message: "Select a valid vehicle class."},
		})
	}

	mode := bookingmodels.BookingMode(strings.ToLower(strings.TrimSpace(input.BookingMode)))
	if mode == "" {
		mode = bookingmodels.BookingModeStandard
	}
	if mode != bookingmodels.BookingModeStandard && mode != bookingmodels.BookingModeHourly {
		return bookingmodels.Booking{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "booking_mode", Message: "Booking mode must be 'standard' or 'hourly'."},
		})
	}
	var reservedDuration *int
	if mode == bookingmodels.BookingModeHourly {
		if input.ReservedDurationMinutes <= 0 {
			return bookingmodels.Booking{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "reserved_duration_minutes", Message: "An hourly booking requires a reserved duration."},
			})
		}
		d := input.ReservedDurationMinutes
		reservedDuration = &d
	}

	passengerCount := input.PassengerCount
	if passengerCount <= 0 {
		passengerCount = 1
	}
	if passengerCount > spec.MaxPassengers {
		return bookingmodels.Booking{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "passenger_count", Message: "Too many passengers for this vehicle class."},
		})
	}

	paymentMethod := strings.ToLower(strings.TrimSpace(input.PaymentMethod))
	if paymentMethod == "" {
		paymentMethod = "wallet"
	}
	// Wallet is the only accepted method. A stale client still sending "cash"
	// gets an explicit 400 rather than a silent upgrade into a real wallet
	// charge the passenger never chose.
	if paymentMethod != "wallet" {
		return bookingmodels.Booking{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "payment_method", Message: "Rides are paid from your wallet."},
		})
	}

	purpose, receiver, violations := normalizeRidePurpose(input, mode, passengerCount)
	if len(violations) > 0 {
		return bookingmodels.Booking{}, apperrors.Validation("Check your details.", violations)
	}

	// Phase B9: a scheduled ride must be for a real future pickup -- in the
	// past (or "now", which is just an immediate booking spelled oddly) is
	// a client bug, not something to silently reinterpret.
	now := time.Now()
	if input.ScheduledAt != nil && !input.ScheduledAt.After(now) {
		return bookingmodels.Booking{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "scheduled_at", Message: "Scheduled pickup must be in the future."},
		})
	}

	// Addendum §5.2: one active ride per customer. No sponsored/booked_for
	// exemption -- Family/Sponsored Rides (addendum §22) is out of scope for
	// this build, so the guard is unconditional.
	active, err := s.bookings.HasActiveBooking(ctx, customerID)
	if err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("Ride could not be created.", err)
	}
	if active {
		return bookingmodels.Booking{}, apperrors.Conflict("You already have an active ride. Complete or cancel it before booking another.", nil)
	}

	pickup := stops[0]
	zone, err := s.resolveZone(ctx, pickup.Lat, pickup.Lng)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	if !zone.SupportsClass(class) {
		return bookingmodels.Booking{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "vehicle_class", Message: "This vehicle class is not available in your area."},
		})
	}

	routePoints := make([]geo.Point, len(stops))
	for i, st := range stops {
		routePoints[i] = geo.Point{Lat: st.Lat, Lng: st.Lng}
	}
	// A single-class request: booking creation already knows which class the
	// passenger picked, so there is no reason to price every class the zone
	// supports the way the estimate endpoint below does.
	quotes, err := s.pricing.Quote(ctx, pricing.QuoteRequest{
		MarketID: zone.MarketID, OperatingZoneID: zone.ID,
		Stops: routePoints, VehicleClasses: []string{class},
	})
	if err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("Fare could not be calculated. Try again.", err)
	}
	quote, ok := findQuote(quotes, class)
	if !ok {
		return bookingmodels.Booking{}, apperrors.Unavailable("Fare could not be calculated for this vehicle class.", nil)
	}

	fareKobo := quote.FareKobo
	distanceKm := quote.DistanceKm
	durationMin := quote.DurationMin

	booking := bookingmodels.Booking{
		CustomerID:              customerID,
		MarketID:                zone.MarketID,
		OperatingZoneID:         zone.ID,
		BookingMode:             mode,
		VehicleClass:            class,
		PassengerCount:          passengerCount,
		RideNotes:               strings.TrimSpace(input.RideNotes),
		ReservedDurationMinutes: reservedDuration,
		FareEstimateKobo:        &fareKobo,
		PricingQuoteReference:   quote.QuoteReference,
		PricingVersion:          quote.RateVersion,
		DistanceKm:              &distanceKm,
		DurationMinutes:         &durationMin,
		PaymentMethod:           paymentMethod,
		PaymentStatus:           "unpaid",

		RidePurpose: purpose,
		// DeliveryVerificationStatus starts 'not_required' for EVERY booking
		// (the column default). A parcel's window opens at trip start, inside
		// MarkInProgress's own guarded statement -- never here.
		DeliveryVerificationStatus: bookingmodels.DeliveryVerificationNotRequired,
	}
	if purpose == bookingmodels.RidePurposeParcel {
		booking.ReceiverName = strings.TrimSpace(input.ReceiverName)
		booking.ReceiverPhone = receiver
		booking.PackageDesc = strings.TrimSpace(input.PackageDesc)
		booking.PackageType = strings.TrimSpace(input.PackageType)
		booking.PackageSize = strings.TrimSpace(input.PackageSize)
		booking.IsFragile = input.IsFragile
	}
	if input.ScheduledAt != nil {
		dispatchStartAt := input.ScheduledAt.Add(-s.scheduling.LeadTime)
		booking.Status = bookingmodels.StatusScheduled
		booking.ScheduledAt = input.ScheduledAt
		booking.DispatchStartAt = &dispatchStartAt
		// MatchingStartedAt stays nil: it is set at PROMOTION time
		// (ScheduledDispatchWorker), not at creation, for exactly this
		// booking's status -- see that field's own doc comment.
	} else {
		booking.Status = bookingmodels.StatusPendingMatch
		booking.MatchingStartedAt = &now
	}

	if err := s.freezePassengerProduct(ctx, input, &booking, now); err != nil {
		return bookingmodels.Booking{}, err
	}

	bookingStops := make([]bookingmodels.Stop, len(stops))
	for i, st := range stops {
		bookingStops[i] = bookingmodels.Stop{
			Sequence: i + 1,
			Type:     st.stopType,
			Address:  st.Address,
			Lat:      st.Lat,
			Lng:      st.Lng,
			Status:   bookingmodels.StopStatusPending,
		}
	}

	created, err := s.bookings.CreateBooking(ctx, booking, bookingStops)
	if err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("Ride could not be created.", err)
	}
	if created.Status == bookingmodels.StatusPendingMatch {
		// A scheduled booking (StatusScheduled) is NOT dispatched here --
		// ScheduledDispatchWorker promotes it to pending_match and spawns
		// the matcher itself once its dispatch window opens.
		s.spawnMatcher(created.ID)
	}
	return created, nil
}

func (s *BookingService) GetByIDForCustomer(ctx context.Context, id, customerID string) (bookingmodels.Booking, error) {
	id = strings.TrimSpace(id)
	customerID = strings.TrimSpace(customerID)
	if id == "" {
		return bookingmodels.Booking{}, apperrors.NotFound("Ride could not be found.", nil)
	}
	return s.bookings.GetByIDForCustomer(ctx, id, customerID)
}

func (s *BookingService) ListByCustomer(ctx context.Context, customerID string, limit int) ([]bookingmodels.Booking, error) {
	return s.bookings.ListByCustomer(ctx, strings.TrimSpace(customerID), limit)
}

// FareTile is one vehicle class's upfront quote for the fare-tile screen.
type FareTile struct {
	VehicleClass   string  `json:"vehicle_class"`
	Label          string  `json:"label"`
	FareKobo       int64   `json:"fare_kobo"`
	DistanceKm     float64 `json:"distance_km"`
	DurationMin    int     `json:"duration_min"`
	QuoteReference string  `json:"quote_reference"`
}

type EstimateFareInput struct {
	PickupLat, PickupLng   float64
	DropoffLat, DropoffLng float64
}

// EstimateFare backs the public, unauthenticated fare-tile screen: one quote
// per class the resolved zone supports, nothing persisted.
//
// Parcel rides reuse these tiles unchanged: a package occupies a car for the
// same distance and time a passenger would. package_size is captured for the
// driver's information only and deliberately does NOT feed pricing in this
// build -- do not assume otherwise when adding a size surcharge later. This is the ONLY
// place PricingClient.Quote is called with more than one class -- booking
// creation above already knows which single class the passenger chose.
func (s *BookingService) EstimateFare(ctx context.Context, input EstimateFareInput) ([]FareTile, error) {
	pickup := geo.Point{Lat: input.PickupLat, Lng: input.PickupLng}
	dropoff := geo.Point{Lat: input.DropoffLat, Lng: input.DropoffLng}
	if !pickup.HasFix() || !dropoff.HasFix() {
		return nil, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "pickup", Message: "A valid pickup and drop-off location are required."},
		})
	}

	zone, err := s.resolveZone(ctx, pickup.Lat, pickup.Lng)
	if err != nil {
		return nil, err
	}
	if len(zone.SupportedClasses) == 0 {
		return []FareTile{}, nil
	}

	quotes, err := s.pricing.Quote(ctx, pricing.QuoteRequest{
		MarketID: zone.MarketID, OperatingZoneID: zone.ID,
		Stops:          []geo.Point{pickup, dropoff},
		VehicleClasses: zone.SupportedClasses,
	})
	if err != nil {
		return nil, apperrors.Unavailable("Fares could not be calculated right now.", err)
	}

	tiles := make([]FareTile, 0, len(quotes))
	for _, q := range quotes {
		label := q.VehicleClass
		if spec, ok := vehicleclass.Get(q.VehicleClass); ok {
			label = spec.Label
		}
		tiles = append(tiles, FareTile{
			VehicleClass:   q.VehicleClass,
			Label:          label,
			FareKobo:       q.FareKobo,
			DistanceKm:     q.DistanceKm,
			DurationMin:    q.DurationMin,
			QuoteReference: q.QuoteReference,
		})
	}
	return tiles, nil
}

// resolveZone translates a not-yet-mapped location into the same
// validation-error shape as every other rejected field on this request,
// rather than a raw NotFound -- "we do not serve this area" is a fact about
// what the passenger typed, not a server fault.
func (s *BookingService) resolveZone(ctx context.Context, lat, lng float64) (marketmodels.Zone, error) {
	zone, err := s.markets.ResolveZone(ctx, geo.Point{Lat: lat, Lng: lng})
	if err != nil {
		appErr := apperrors.From(err)
		if appErr != nil && appErr.Code == apperrors.CodeNotFound {
			return marketmodels.Zone{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "stops", Message: "We do not operate in this area yet."},
			})
		}
		return marketmodels.Zone{}, apperrors.Unavailable("Your location could not be resolved.", err)
	}
	return zone, nil
}

func findQuote(quotes []pricing.Quote, class string) (pricing.Quote, bool) {
	for _, q := range quotes {
		if q.VehicleClass == class {
			return q, true
		}
	}
	return pricing.Quote{}, false
}

type normalizedStop struct {
	stopType bookingmodels.StopType
	Address  string
	Lat, Lng float64
}

// normalizeStops derives each stop's type from its POSITION in the slice
// (first = PICKUP, last = DROPOFF, everything between = WAYPOINT) rather than
// trusting a client-supplied type. A client cannot mislabel a middle stop as
// PICKUP and misdirect where the matcher dispatches toward.
func normalizeStops(inputs []StopInput) ([]normalizedStop, error) {
	if len(inputs) < 2 {
		return nil, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "stops", Message: "At least a pickup and a drop-off are required."},
		})
	}
	out := make([]normalizedStop, len(inputs))
	for i, in := range inputs {
		point := geo.Point{Lat: in.Lat, Lng: in.Lng}
		if !point.HasFix() {
			return nil, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: fmt.Sprintf("stops[%d]", i), Message: "A valid location is required."},
			})
		}
		var stopType bookingmodels.StopType
		switch {
		case i == 0:
			stopType = bookingmodels.StopTypePickup
		case i == len(inputs)-1:
			stopType = bookingmodels.StopTypeDropoff
		default:
			stopType = bookingmodels.StopTypeWaypoint
		}
		out[i] = normalizedStop{stopType: stopType, Address: strings.TrimSpace(in.Address), Lat: in.Lat, Lng: in.Lng}
	}
	return out, nil
}
