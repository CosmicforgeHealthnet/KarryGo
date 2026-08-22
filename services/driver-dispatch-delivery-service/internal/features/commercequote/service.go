// Package commercequote answers authoritative delivery quotes for internal
// commerce callers (Vendor Service Section 17C fulfilment planning).
//
// It answers two questions in one call — "can dispatch serve this route with
// this load" and "what would it cost" — and it persists the answer before
// returning it, so the reference the caller keeps names a real row.
//
// Two boundaries this package must never cross:
//
//   - It creates NO booking, reserves no rider and mutates no trip. A quote is
//     an answer, not a commitment; mission creation is a separate contract.
//   - The fee is computed by shared/go/dispatchpricing, the same formula the
//     customer and partner surfaces use. There is no commerce-specific fare,
//     because a third fare formula is how two surfaces start disagreeing about
//     the same route.
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

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/dispatchpricing"
)

// Dispatch carries motorcycles and nothing else. A van or truck asked of this
// service is refused rather than silently quoted on a bike.
const serviceName = "DISPATCH"

// currencyNGN is the only currency this service prices in. A request in
// anything else is refused, not converted: no service in this platform holds
// exchange-rate authority.
const currencyNGN = "NGN"

// Dispatch riders carry an insulated box. That satisfies a hot-hold or a
// short-haul insulated need; it is NOT a cold chain and must never be offered
// as one.
var supportedCapabilities = map[string]bool{
	commercequote.CapabilityHotHold:         true,
	commercequote.CapabilityInsulated:       true,
	commercequote.CapabilityUprightCarry:    true,
	commercequote.CapabilityLeakContainment: true,
}

// Physical envelope of a motorcycle courier. These bound what this service is
// willing to promise; the dispatch marketplace has no per-vehicle capacity
// model to check against later, so refusing here is the only place an
// impossible load can be caught.
const (
	maxWeightGrams int64 = 25000
	maxVolumeMl    int64 = 80000
	maxDimensionMm       = 700
)

var (
	// ErrInvalidRequest is a malformed request.
	ErrInvalidRequest = errors.New("commercequote: invalid request")

	// ErrUnavailable means this service could not answer at all — the
	// availability store is down, or the quote could not be persisted. It is
	// deliberately NOT reported as "not serviceable": a caller told "no rider"
	// would stop asking, while a caller told "unavailable" knows to retry.
	ErrUnavailable = errors.New("commercequote: unable to answer")
)

// LiveAvailability is the rider-presence authority. Satisfied by the existing
// availability service.
type LiveAvailability interface {
	GetNearby(
		ctx context.Context,
		request availability.NearbyProvidersRequest,
	) ([]availability.NearbyProvider, error)
}

// Store persists issued quotes.
type Store interface {
	// Upsert stores a quote idempotently on (caller, reference).
	//
	// An identical fingerprint returns the stored answer unchanged; a changed
	// one supersedes it and increments the revision. Returning the stored row
	// rather than the candidate is what makes a retry return the same quote
	// instead of a second one.
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
	store    Store
	live     LiveAvailability
	radiusKM float64
	ttl      time.Duration
	now      func() time.Time
}

// Options configures the service.
type Options struct {
	Store    Store
	Live     LiveAvailability
	RadiusKM float64
	TTL      time.Duration
	Now      func() time.Time
}

// DefaultTTL is how long an issued quote stands. It is short on purpose: a
// quote is a promise about rider availability, and rider availability is the
// most perishable fact in this service.
const DefaultTTL = 15 * time.Minute

// New builds the quote service.
func New(options Options) *Service {
	ttl := options.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	radius := options.RadiusKM
	if radius <= 0 {
		radius = 10
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		store:    options.Store,
		live:     options.Live,
		radiusKM: radius,
		ttl:      ttl,
		now:      now,
	}
}

// fingerprint is the stable hash of everything that can change an answer.
//
// It deliberately excludes the correlation id and the address strings: neither
// can change a fee, so including them would make an identical re-ask look like
// a changed one and churn revisions for no reason.
func fingerprint(request commercequote.Request) string {
	payload := struct {
		Class    commercequote.VehicleClass `json:"class"`
		PLat     float64                    `json:"p_lat"`
		PLng     float64                    `json:"p_lng"`
		DLat     float64                    `json:"d_lat"`
		DLng     float64                    `json:"d_lng"`
		Load     commercequote.Load         `json:"load"`
		Urgency  commercequote.Urgency      `json:"urgency"`
		Currency string                     `json:"currency"`
	}{
		Class:    request.VehicleClass,
		PLat:     request.Pickup.Latitude,
		PLng:     request.Pickup.Longitude,
		DLat:     request.Dropoff.Latitude,
		DLng:     request.Dropoff.Longitude,
		Load:     request.Load,
		Urgency:  request.Urgency,
		Currency: strings.ToUpper(strings.TrimSpace(request.Currency)),
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		// json.Marshal of this fixed shape cannot fail. Falling back to the
		// reference alone would silently disable revision detection, so an
		// impossible error becomes an obviously-wrong fingerprint instead.
		return "unfingerprintable"
	}

	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:])
}

// packageSizeFor maps the caller's physical load onto the dispatch fare's own
// small/medium/large vocabulary.
//
// The mapping is here rather than at the caller because the vocabulary belongs
// to dispatch's fare model: Vendor Service states physics, this service
// decides what that physics costs.
func packageSizeFor(load commercequote.Load) string {
	switch {
	case load.TotalWeightGrams > 8000 ||
		load.TotalVolumeMl > 30000 ||
		load.LongestDimensionMm > 450:
		return dispatchpricing.SizeLarge

	case load.TotalWeightGrams > 3000 ||
		load.TotalVolumeMl > 10000 ||
		load.LongestDimensionMm > 250:
		return dispatchpricing.SizeMedium

	default:
		return dispatchpricing.SizeSmall
	}
}

func tierFor(urgency commercequote.Urgency) string {
	if urgency == commercequote.UrgencyImmediate {
		return dispatchpricing.TierExpress
	}

	return dispatchpricing.TierStandard
}

// refusal builds a stored refusal. It carries no fee and no currency, which is
// what the table's own CHECK constraint also insists on.
func (s *Service) refuse(
	request commercequote.Request,
	reason string,
	distanceKm float64,
	availableProviders int,
) Record {
	return Record{
		ID:                 "dq_" + uuid.NewString(),
		QuoteReference:     strings.TrimSpace(request.QuoteReference),
		Revision:           1,
		RequestFingerprint: fingerprint(request),
		VehicleClass:       string(commercequote.VehicleMotorcycle),
		PickupLatitude:     request.Pickup.Latitude,
		PickupLongitude:    request.Pickup.Longitude,
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
// say no riders" must be able to find the answer that was given, and a refusal
// that left no trace makes that impossible.
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
// The order of the checks matters: everything this service can decide on its
// own is decided before the availability store is consulted, so an impossible
// load is refused for being impossible rather than for a transient absence of
// riders.
func (s *Service) evaluate(
	ctx context.Context,
	request commercequote.Request,
) (Record, error) {
	if request.VehicleClass != commercequote.VehicleMotorcycle {
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

	if request.Load.LargeLoad ||
		request.Load.TotalWeightGrams > maxWeightGrams ||
		request.Load.TotalVolumeMl > maxVolumeMl ||
		request.Load.LongestDimensionMm > maxDimensionMm {
		return s.refuse(
			request, commercequote.ReasonLoadExceedsCapacity, 0, 0,
		), nil
	}

	distance := dispatchpricing.HaversineKm(
		request.Pickup.Latitude, request.Pickup.Longitude,
		request.Dropoff.Latitude, request.Dropoff.Longitude,
	)

	if math.IsNaN(distance) || math.IsInf(distance, 0) {
		return s.refuse(request, commercequote.ReasonInvalidRoute, 0, 0), nil
	}

	if s.live == nil {
		return Record{}, ErrUnavailable
	}

	providers, err := s.live.GetNearby(ctx, availability.NearbyProvidersRequest{
		Latitude:  request.Pickup.Latitude,
		Longitude: request.Pickup.Longitude,
		RadiusKM:  s.radiusKM,
		Limit:     availability.NearbyMaxLimit,
	})
	if err != nil {
		// An availability fault is a fault, not a refusal. Reporting "no
		// riders" here would tell a customer something this service does not
		// actually know.
		return Record{}, ErrUnavailable
	}

	if len(providers) == 0 {
		return s.refuse(
			request, commercequote.ReasonNoProviderAvailable, distance, 0,
		), nil
	}

	size := packageSizeFor(request.Load)
	tier := tierFor(request.Urgency)

	breakdown := dispatchpricing.Breakdown(distance, size, tier)

	return Record{
		ID:                 "dq_" + uuid.NewString(),
		QuoteReference:     strings.TrimSpace(request.QuoteReference),
		Revision:           1,
		RequestFingerprint: fingerprint(request),
		VehicleClass:       string(commercequote.VehicleMotorcycle),
		PickupLatitude:     request.Pickup.Latitude,
		PickupLongitude:    request.Pickup.Longitude,
		DropoffLatitude:    request.Dropoff.Latitude,
		DropoffLongitude:   request.Dropoff.Longitude,
		PackageCount:       request.Load.PackageCount,
		TotalWeightGrams:   request.Load.TotalWeightGrams,
		Serviceable:        true,
		ReasonCode:         commercequote.ReasonServiceable,
		DistanceKm:         dispatchpricing.RoundKM(distance),
		EstimatedMinutes:   dispatchpricing.ETAMinutes(distance),
		FeeMinor:           dispatchpricing.FareKobo(distance, size, tier),
		Currency:           currencyNGN,
		BaseMinor:          breakdown.BaseFareKobo,
		DistanceMinor:      breakdown.DistanceFareKobo,
		SurchargeMinor: breakdown.SizeSurchargeKobo +
			breakdown.ExpressSurchargeKobo,
		AvailableProviders: len(providers),
		ExpiresAt:          s.now().UTC().Add(s.ttl),
	}, nil
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

		// The endpoint handles this quote is bound to. A mission created later
		// must present them, which is what stops a quote for one route being
		// spent on another.
		quote.PickupRef = record.ID + ":pickup"
		quote.DropoffRef = record.ID + ":dropoff"
	}

	return quote
}
