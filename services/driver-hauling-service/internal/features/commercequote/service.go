// Package commercequote answers authoritative delivery quotes for internal
// commerce callers (Vendor Service Section 17C fulfilment planning).
//
// It answers two questions in one call — "can hauling serve this route with
// this load" and "what would it cost" — and it persists the answer before
// returning it, so the reference the caller keeps names a real row.
//
// This service owns BOTH van and truck. A quote for either class is answered
// here and nowhere else.
//
// Two boundaries this package must never cross:
//
//   - It creates NO booking, matches no provider and mutates no trip. A quote
//     is an answer, not a commitment.
//   - The fee is this service's own fare model, the same one the customer
//     booking surface uses. There is no commerce-specific fare, because a
//     second fare formula is how two surfaces start disagreeing about the same
//     route.
package commercequote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/commercequote"
)

const (
	serviceName = "HAULING"

	// currencyNGN is the only currency this service prices in. A request in
	// anything else is refused, not converted: no service in this platform
	// holds exchange-rate authority.
	currencyNGN = "NGN"
)

// DefaultTTL is how long an issued quote stands. It is longer than dispatch's
// because a truck fleet's availability is less volatile than a bike courier's,
// and shorter than a checkout session so a plan can never outlive its quote.
const DefaultTTL = 20 * time.Minute

// Vans and trucks are enclosed vehicles with the space for handling equipment.
// COLD_CHAIN and FROZEN_CHAIN are supported only through a refrigerated truck,
// which is why they change the truck type rather than merely passing a check.
var supportedCapabilities = map[string]bool{
	commercequote.CapabilityHotHold:         true,
	commercequote.CapabilityInsulated:       true,
	commercequote.CapabilityColdChain:       true,
	commercequote.CapabilityFrozenChain:     true,
	commercequote.CapabilityUprightCarry:    true,
	commercequote.CapabilityFragileCarry:    true,
	commercequote.CapabilityLeakContainment: true,
}

var (
	// ErrInvalidRequest is a malformed request.
	ErrInvalidRequest = errors.New("commercequote: invalid request")

	// ErrUnavailable means this service could not answer at all. It is
	// deliberately NOT reported as "not serviceable": a caller told "no truck"
	// would stop asking, while a caller told "unavailable" knows to retry.
	ErrUnavailable = errors.New("commercequote: unable to answer")
)

// AvailabilityReader lists the providers currently online. Satisfied by the
// existing Redis availability store, which prunes stale entries as it reads —
// so a quote is never issued off an expired presence key.
type AvailabilityReader interface {
	GetOnlineProviders(
		ctx context.Context,
	) ([]availabilityrepositories.ProviderStatus, error)
}

// TruckInfo is the subset of a truck this package needs.
type TruckInfo struct {
	TruckType  string
	CapacityKg int
	Status     string
}

// TruckLookup resolves a truck by id. Satisfied by an adapter over the truck
// repository in cmd wiring, exactly as the matcher's own lookup is.
type TruckLookup interface {
	GetTruck(ctx context.Context, truckID string) (TruckInfo, error)
}

// Store persists issued quotes.
type Store interface {
	Upsert(ctx context.Context, record Record) (Record, error)
}

// Record is one persisted quote.
type Record struct {
	ID string

	CallerService  string
	QuoteReference string
	Revision       int64

	RequestFingerprint string

	VehicleClass string
	TruckType    string

	PickupLatitude   float64
	PickupLongitude  float64
	DropoffLatitude  float64
	DropoffLongitude float64

	PackageCount     int64
	TotalWeightGrams int64

	Serviceable bool
	ReasonCode  string

	DistanceKm       float64
	EstimatedMinutes int

	FeeMinor int64
	Currency string

	BaseMinor      int64
	DistanceMinor  int64
	SurchargeMinor int64

	AvailableProviders int

	ExpiresAt time.Time
	CreatedAt time.Time
}

// Service answers quotes.
type Service struct {
	store        Store
	availability AvailabilityReader
	trucks       TruckLookup

	maxRadiusKm float64
	ttl         time.Duration
	now         func() time.Time
}

// Options configures the service.
type Options struct {
	Store        Store
	Availability AvailabilityReader
	Trucks       TruckLookup
	MaxRadiusKm  float64
	TTL          time.Duration
	Now          func() time.Time
}

// New builds the quote service.
func New(options Options) *Service {
	ttl := options.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	radius := options.MaxRadiusKm
	if radius <= 0 {
		radius = 25
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		store:        options.Store,
		availability: options.Availability,
		trucks:       options.Trucks,
		maxRadiusKm:  radius,
		ttl:          ttl,
		now:          now,
	}
}

// truckTypesFor is the set of truck types that can serve a class.
//
// It mirrors the matcher's own vocabulary, because a quote naming a type the
// matcher would never select is a quote that cannot be honoured. Refrigerated
// is not in either list: it is chosen only when a cooling chain is actually
// required, so a plain load is never quoted on a fridge and charged for it.
func truckTypesFor(class commercequote.VehicleClass) []string {
	switch class {
	case commercequote.VehicleVan:
		return []string{"van", "pickup", "box"}

	case commercequote.VehicleTruck:
		return []string{
			"flatbed", "container", "tipper", "trailer",
			"lowbed", "dump", "box",
		}

	default:
		return nil
	}
}

func requiresCooling(capabilities []string) bool {
	for _, capability := range capabilities {
		switch strings.ToUpper(strings.TrimSpace(capability)) {
		case commercequote.CapabilityColdChain,
			commercequote.CapabilityFrozenChain:
			return true
		}
	}

	return false
}

// fingerprint is the stable hash of everything that can change an answer. It
// excludes the correlation id and the address strings: neither can change a
// fee, so including them would churn revisions for no reason.
func fingerprint(request commercequote.Request) string {
	payload := struct {
		Class    commercequote.VehicleClass `json:"class"`
		PLat     float64                    `json:"p_lat"`
		PLng     float64                    `json:"p_lng"`
		Stops    []commercequote.PickupStop `json:"stops,omitempty"`
		DLat     float64                    `json:"d_lat"`
		DLng     float64                    `json:"d_lng"`
		Load     commercequote.Load         `json:"load"`
		Urgency  commercequote.Urgency      `json:"urgency"`
		Currency string                     `json:"currency"`
	}{
		Class:    request.VehicleClass,
		PLat:     request.Pickup.Latitude,
		PLng:     request.Pickup.Longitude,
		Stops:    request.PickupStops,
		DLat:     request.Dropoff.Latitude,
		DLng:     request.Dropoff.Longitude,
		Load:     request.Load,
		Urgency:  request.Urgency,
		Currency: strings.ToUpper(strings.TrimSpace(request.Currency)),
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "unfingerprintable"
	}

	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:])
}

func (s *Service) refuse(
	request commercequote.Request,
	reason string,
	distanceKm float64,
	availableProviders int,
) Record {
	pickup := firstPickup(request)
	return Record{
		ID:                 "hq_" + uuid.NewString(),
		QuoteReference:     strings.TrimSpace(request.QuoteReference),
		Revision:           1,
		RequestFingerprint: fingerprint(request),
		VehicleClass:       string(request.VehicleClass),
		PickupLatitude:     pickup.Latitude,
		PickupLongitude:    pickup.Longitude,
		DropoffLatitude:    request.Dropoff.Latitude,
		DropoffLongitude:   request.Dropoff.Longitude,
		PackageCount:       request.Load.PackageCount,
		TotalWeightGrams:   request.Load.TotalWeightGrams,
		Serviceable:        false,
		ReasonCode:         reason,
		DistanceKm:         distanceKm,
		AvailableProviders: availableProviders,
		ExpiresAt:          s.now().UTC().Add(s.ttl),
	}
}

// Quote answers one request.
//
// Every refusal is still persisted. An operator asked "why did this checkout
// say no trucks" must be able to find the answer that was given.
func (s *Service) Quote(
	ctx context.Context,
	caller string,
	request commercequote.Request,
) (commercequote.Quote, error) {
	caller = strings.TrimSpace(caller)

	if caller == "" {
		return commercequote.Quote{}, ErrInvalidRequest
	}

	if _, ok := request.Validate(); !ok {
		return commercequote.Quote{}, ErrInvalidRequest
	}

	if s.store == nil {
		return commercequote.Quote{}, ErrUnavailable
	}

	record, err := s.evaluate(ctx, request)
	if err != nil {
		return commercequote.Quote{}, err
	}

	record.CallerService = caller

	stored, err := s.store.Upsert(ctx, record)
	if err != nil {
		return commercequote.Quote{}, ErrUnavailable
	}

	return toQuote(stored), nil
}

// evaluate produces the answer without persisting it.
//
// Everything this service can decide alone is decided before the availability
// store is consulted, so an unsupported class is refused for being unsupported
// rather than for a transient absence of trucks.
func (s *Service) evaluate(
	ctx context.Context,
	request commercequote.Request,
) (Record, error) {
	eligibleTypes := truckTypesFor(request.VehicleClass)

	if len(eligibleTypes) == 0 {
		// A motorcycle asked of hauling. Dispatch owns that class.
		return s.refuse(
			request, commercequote.ReasonVehicleClassUnsupported, 0, 0,
		), nil
	}

	if !strings.EqualFold(strings.TrimSpace(request.Currency), currencyNGN) {
		return s.refuse(
			request, commercequote.ReasonCurrencyUnsupported, 0, 0,
		), nil
	}

	for _, capability := range request.Load.Capabilities {
		if !supportedCapabilities[strings.ToUpper(strings.TrimSpace(capability))] {
			return s.refuse(
				request, commercequote.ReasonHandlingUnsupported, 0, 0,
			), nil
		}
	}

	// A cooling chain is served by a refrigerated truck and by nothing else,
	// so it replaces the class's ordinary type set rather than adding to it.
	cooling := requiresCooling(request.Load.Capabilities)

	if cooling {
		eligibleTypes = []string{"refrigerated"}
	}

	distance := routeDistanceKm(request)

	if math.IsNaN(distance) || math.IsInf(distance, 0) {
		return s.refuse(request, commercequote.ReasonInvalidRoute, 0, 0), nil
	}

	if s.availability == nil {
		return Record{}, ErrUnavailable
	}

	providers, err := s.availability.GetOnlineProviders(ctx)
	if err != nil {
		// An availability fault is a fault, not a refusal. Reporting "no
		// trucks" here would tell a customer something this service does not
		// actually know.
		return Record{}, ErrUnavailable
	}

	weightKg := int(request.Load.TotalWeightGrams / 1000)

	eligible, sawInRange := s.countEligible(
		ctx, providers, request, eligibleTypes, weightKg,
	)

	if eligible == 0 {
		// Two distinct refusals: nobody was near the pickup at all, versus
		// somebody was but no truck of theirs could carry this. The customer
		// sees a different message for each, and so does the operator.
		reason := commercequote.ReasonNoProviderAvailable

		if sawInRange {
			reason = commercequote.ReasonLoadExceedsCapacity
		}

		return s.refuse(request, reason, distance, 0), nil
	}

	fare := bookingmodels.CalculateFare(distance, weightKg, 0)

	truckType := eligibleTypes[0]

	pickup := firstPickup(request)

	return Record{
		ID:                 "hq_" + uuid.NewString(),
		QuoteReference:     strings.TrimSpace(request.QuoteReference),
		Revision:           1,
		RequestFingerprint: fingerprint(request),
		VehicleClass:       string(request.VehicleClass),
		TruckType:          truckType,
		PickupLatitude:     pickup.Latitude,
		PickupLongitude:    pickup.Longitude,
		DropoffLatitude:    request.Dropoff.Latitude,
		DropoffLongitude:   request.Dropoff.Longitude,
		PackageCount:       request.Load.PackageCount,
		TotalWeightGrams:   request.Load.TotalWeightGrams,
		Serviceable:        true,
		ReasonCode:         commercequote.ReasonServiceable,
		DistanceKm:         distance,
		EstimatedMinutes: estimatedMinutes(
			distance, request.Load.LoadingDurationSeconds,
		),
		FeeMinor:      fare.FareEstimateKobo,
		Currency:      currencyNGN,
		BaseMinor:     fare.BreakdownKobo.BaseFareKobo,
		DistanceMinor: fare.BreakdownKobo.PerKmFareKobo,
		SurchargeMinor: fare.BreakdownKobo.WeightSurcharge +
			fare.BreakdownKobo.HelperFeeKobo,
		AvailableProviders: eligible,
		ExpiresAt:          s.now().UTC().Add(s.ttl),
	}, nil
}

// countEligible applies the SAME filters the matcher applies, so a quote is
// never issued that the matcher would then refuse to fill.
//
// sawInRange reports whether any provider was near enough to be considered at
// all, which is what lets the caller distinguish "nobody here" from "nobody
// here with a big enough truck".
func (s *Service) countEligible(
	ctx context.Context,
	providers []availabilityrepositories.ProviderStatus,
	request commercequote.Request,
	eligibleTypes []string,
	weightKg int,
) (eligible int, sawInRange bool) {
	allowed := make(map[string]bool, len(eligibleTypes))

	for _, truckType := range eligibleTypes {
		allowed[truckType] = true
	}

	pickup := firstPickup(request)
	for _, provider := range providers {
		if haversineKm(
			pickup.Latitude, pickup.Longitude,
			provider.Lat, provider.Lng,
		) > s.maxRadiusKm {
			continue
		}

		sawInRange = true

		// No truck lookup wired, or no truck on the presence record: the
		// matcher degrades to proximity-only in exactly this case, and a quote
		// that were stricter than the matcher would refuse work the service
		// can actually do.
		if s.trucks == nil || provider.TruckID == "" {
			eligible++
			continue
		}

		truck, err := s.trucks.GetTruck(ctx, provider.TruckID)
		if err != nil {
			eligible++
			continue
		}

		if truck.CapacityKg > 0 && weightKg > truck.CapacityKg {
			continue
		}

		if !allowed[strings.ToLower(strings.TrimSpace(truck.TruckType))] {
			continue
		}

		eligible++
	}

	return eligible, sawInRange
}

func toQuote(record Record) commercequote.Quote {
	quote := commercequote.Quote{
		QuoteID:            record.ID,
		QuoteReference:     record.QuoteReference,
		Revision:           record.Revision,
		Service:            serviceName,
		Serviceable:        record.Serviceable,
		ReasonCode:         record.ReasonCode,
		DistanceKm:         record.DistanceKm,
		EstimatedMinutes:   record.EstimatedMinutes,
		FeeMinor:           record.FeeMinor,
		Currency:           record.Currency,
		AvailableProviders: record.AvailableProviders,
		ExpiresAt:          record.ExpiresAt,
		QuotedAt:           record.CreatedAt,
		Breakdown: commercequote.Breakdown{
			BaseMinor:      record.BaseMinor,
			DistanceMinor:  record.DistanceMinor,
			SurchargeMinor: record.SurchargeMinor,
		},
	}

	if record.Serviceable {
		quote.VehicleClass = commercequote.VehicleClass(record.VehicleClass)
		quote.PickupRef = record.ID + ":pickup"
		quote.DropoffRef = record.ID + ":dropoff"
	}

	return quote
}

func firstPickup(request commercequote.Request) commercequote.Point {
	if len(request.PickupStops) > 0 {
		return request.PickupStops[0].Location
	}
	return request.Pickup
}

func routeDistanceKm(request commercequote.Request) float64 {
	if len(request.PickupStops) == 0 {
		return haversineKm(
			request.Pickup.Latitude, request.Pickup.Longitude,
			request.Dropoff.Latitude, request.Dropoff.Longitude,
		)
	}

	previous := request.PickupStops[0].Location
	var total float64
	for index := 1; index < len(request.PickupStops); index++ {
		next := request.PickupStops[index].Location
		total += haversineKm(previous.Latitude, previous.Longitude, next.Latitude, next.Longitude)
		previous = next
	}
	total += haversineKm(previous.Latitude, previous.Longitude, request.Dropoff.Latitude, request.Dropoff.Longitude)
	return total
}

// haversineKm mirrors the matcher's own distance function (Earth radius
// 6371 km), so eligibility here and eligibility there agree.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0

	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }

	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// estimatedMinutes is a road-time estimate for a loaded van or truck.
//
// It is deliberately conservative and separate from dispatch's bike ETA: a
// truck through Lagos traffic is not a motorcycle, and reusing the bike figure
// would promise the customer a time this service cannot keep.
func estimatedMinutes(distanceKm float64, loadingDurationSeconds ...int64) int {
	const (
		averageSpeedKmh       = 14.0
		defaultLoadingMinutes = 25
		minimumMinutes        = 40
	)
	loadingMinutes := defaultLoadingMinutes
	if len(loadingDurationSeconds) > 0 && loadingDurationSeconds[0] > 0 {
		loadingMinutes = int(math.Ceil(float64(loadingDurationSeconds[0]) / 60))
	}

	minutes := int(math.Ceil(distanceKm/averageSpeedKmh*60)) + loadingMinutes

	if minutes < minimumMinutes {
		minutes = minimumMinutes
	}

	return minutes
}
