package commercequote

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/dispatchpricing"
)

const (
	capabilityVersion      int64 = 1
	maxPackageCount        int64 = 40
	SupplyAvailable              = "AVAILABLE_NOW"
	SupplyQueued                 = "NO_PROVIDER_CURRENTLY_AVAILABLE"
	SupplyUnknown                = "AVAILABILITY_UNKNOWN"
	ReasonRouteUnavailable       = "ROUTE_SERVICE_UNAVAILABLE"
	ReasonReadyWindow            = "READY_WINDOW_UNSATISFIABLE"
	ReasonSLA                    = "PROMISED_WINDOW_UNSATISFIABLE"
)

var dispatchHandling = []string{
	"FRAGILE", "UPRIGHT", "SPILL_LIQUID", "HOT", "HIGH_VALUE",
}

type MultiStopRecord struct {
	ID                   string
	CallerService        string
	QuoteReference       string
	Revision             int64
	RequestFingerprint   string
	RequestSnapshot      []byte
	RouteSnapshot        []byte
	Serviceable          bool
	ReasonCode           string
	SupplyState          string
	Mode                 string
	ServiceClass         string
	CapabilityVersion    int64
	RouteProvider        string
	RouteEvidenceVersion string
	DistanceMeters       int64
	EstimatedSeconds     int64
	FeeMinor             int64
	Currency             string
	AvailableProviders   int
	ExpiresAt            time.Time
	CreatedAt            time.Time
}

type MultiStopStore interface {
	UpsertMultiStop(ctx context.Context, record MultiStopRecord) (MultiStopRecord, error)
	FindMultiStopByID(ctx context.Context, caller, quoteID string) (MultiStopRecord, bool, error)
}

type MultiStopService struct {
	store    MultiStopStore
	routes   RoutePlanner
	live     LiveAvailability
	radiusKM float64
	ttl      time.Duration
	now      func() time.Time
}

type MultiStopOptions struct {
	Store    MultiStopStore
	Routes   RoutePlanner
	Live     LiveAvailability
	RadiusKM float64
	TTL      time.Duration
	Now      func() time.Time
}

func NewMultiStop(options MultiStopOptions) *MultiStopService {
	if options.RadiusKM <= 0 {
		options.RadiusKM = 10
	}
	if options.TTL <= 0 {
		options.TTL = DefaultTTL
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &MultiStopService{store: options.Store, routes: options.Routes, live: options.Live, radiusKM: options.RadiusKM, ttl: options.TTL, now: options.Now}
}

func Capability(now time.Time) dispatchcommerce.MotorcycleCapability {
	return dispatchcommerce.MotorcycleCapability{
		Class: "MOTORCYCLE", Version: capabilityVersion,
		MaxWeightGrams: maxWeightGrams, MaxVolumeMl: maxVolumeMl,
		MaxDimensionMm: maxDimensionMm, MaxPackageCount: maxPackageCount,
		Handling: append([]string(nil), dispatchHandling...), MultiPickup: true,
		MaxPickupStops: dispatchcommerce.MaxPickupStops, ObservedAt: now.UTC(),
	}
}

func multiStopFingerprint(request dispatchcommerce.QuoteRequest) (string, error) {
	request.CorrelationID = ""
	return dispatchcommerce.Fingerprint(request)
}

func (s *MultiStopService) Quote(ctx context.Context, caller string, request dispatchcommerce.QuoteRequest) (dispatchcommerce.Quote, error) {
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return dispatchcommerce.Quote{}, ErrInvalidRequest
	}
	if reason, ok := request.Validate(); !ok {
		return dispatchcommerce.Quote{}, errors.New(reason)
	}
	if s == nil || s.store == nil {
		return dispatchcommerce.Quote{}, ErrUnavailable
	}
	fingerprint, err := multiStopFingerprint(request)
	if err != nil {
		return dispatchcommerce.Quote{}, ErrInvalidRequest
	}

	record := MultiStopRecord{
		ID: "dcq_" + strings.ReplaceAll(uuid.NewString(), "-", ""), CallerService: caller,
		QuoteReference: strings.TrimSpace(request.QuoteReference), Revision: 1,
		RequestFingerprint: fingerprint, Mode: string(request.Mode), ServiceClass: strings.TrimSpace(request.ServiceClass),
		CapabilityVersion: capabilityVersion, ExpiresAt: s.now().UTC().Add(s.ttl),
	}
	record.RequestSnapshot, _ = json.Marshal(request)

	if reason := validateOperationalLoad(request); reason != "" {
		record.ReasonCode = reason
		record.SupplyState = SupplyUnknown
		return s.persist(ctx, record)
	}

	points, physicalStops := routePoints(request)
	plan, err := s.routes.Plan(ctx, points, request.Mode)
	if err != nil {
		return dispatchcommerce.Quote{}, ErrRouteServiceUnavailable
	}
	sequence, ok := expandSequence(plan, physicalStops, request.PickupStops)
	if !ok {
		return dispatchcommerce.Quote{}, ErrRouteServiceUnavailable
	}
	if reason := validateWindows(request, sequence, plan.DurationSeconds, s.now().UTC()); reason != "" {
		record.ReasonCode = reason
		record.SupplyState = SupplyUnknown
		record.RouteProvider = plan.Provider
		record.RouteEvidenceVersion = plan.EvidenceVersion
		record.DistanceMeters = plan.DistanceMeters
		record.EstimatedSeconds = plan.DurationSeconds
		record.RouteSnapshot, _ = json.Marshal(sequence)
		return s.persist(ctx, record)
	}

	available := 0
	supply := SupplyUnknown
	if s.live != nil {
		providers, liveErr := s.live.GetNearby(ctx, availability.NearbyProvidersRequest{
			Latitude: request.PickupStops[0].Location.Latitude, Longitude: request.PickupStops[0].Location.Longitude,
			RadiusKM: s.radiusKM, Limit: availability.NearbyMaxLimit,
		})
		if liveErr == nil {
			available = len(providers)
			if available > 0 {
				supply = SupplyAvailable
			} else {
				supply = SupplyQueued
			}
		}
	}

	distanceKM := float64(plan.DistanceMeters) / 1000
	tier := dispatchpricing.TierStandard
	if request.Mode == dispatchcommerce.ModeFastest {
		tier = dispatchpricing.TierExpress
	}
	record.Serviceable = true
	record.ReasonCode = commerceReasonServiceable
	record.SupplyState = supply
	record.RouteProvider = plan.Provider
	record.RouteEvidenceVersion = plan.EvidenceVersion
	record.DistanceMeters = plan.DistanceMeters
	record.EstimatedSeconds = plan.DurationSeconds
	record.FeeMinor = dispatchpricing.FareKobo(distanceKM, multiStopPackageSize(request.Parcels), tier)
	record.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	record.AvailableProviders = available
	record.RouteSnapshot, _ = json.Marshal(sequence)
	return s.persist(ctx, record)
}

const commerceReasonServiceable = "SERVICEABLE"

func (s *MultiStopService) persist(ctx context.Context, record MultiStopRecord) (dispatchcommerce.Quote, error) {
	stored, err := s.store.UpsertMultiStop(ctx, record)
	if err != nil {
		return dispatchcommerce.Quote{}, ErrUnavailable
	}
	var sequence []dispatchcommerce.RouteStop
	_ = json.Unmarshal(stored.RouteSnapshot, &sequence)
	return dispatchcommerce.Quote{
		QuoteID: stored.ID, QuoteReference: stored.QuoteReference, Revision: stored.Revision,
		RequestFingerprint: stored.RequestFingerprint, Serviceable: stored.Serviceable,
		ReasonCode: stored.ReasonCode, SupplyState: stored.SupplyState,
		Mode: dispatchcommerce.Mode(stored.Mode), ServiceClass: stored.ServiceClass,
		VehicleClass: "MOTORCYCLE", CapabilityVersion: stored.CapabilityVersion,
		RouteProvider: stored.RouteProvider, RouteEvidenceVersion: stored.RouteEvidenceVersion,
		DistanceMeters: stored.DistanceMeters, EstimatedSeconds: stored.EstimatedSeconds,
		StopSequence: sequence, FeeMinor: stored.FeeMinor, Currency: stored.Currency,
		AvailableProviders: stored.AvailableProviders, ExpiresAt: stored.ExpiresAt, QuotedAt: stored.CreatedAt,
	}, nil
}

func validateOperationalLoad(request dispatchcommerce.QuoteRequest) string {
	capability := Capability(time.Time{})
	if request.Vehicle.CapabilityVersion != capability.Version {
		return "CAPABILITY_VERSION_STALE"
	}
	var weight, volume, packages int64
	for _, parcel := range request.Parcels {
		if parcel.WeightGrams > math.MaxInt64-weight || parcel.VolumeMl > math.MaxInt64-volume || parcel.PackageCount > math.MaxInt64-packages {
			return "LOAD_EXCEEDS_CAPACITY"
		}
		weight += parcel.WeightGrams
		volume += parcel.VolumeMl
		packages += parcel.PackageCount
		if parcel.LengthMm > capability.MaxDimensionMm || parcel.WidthMm > capability.MaxDimensionMm || parcel.HeightMm > capability.MaxDimensionMm {
			return "LOAD_EXCEEDS_CAPACITY"
		}
		if parcel.Handling.Chilled || parcel.Handling.Frozen {
			return "HANDLING_UNSUPPORTED"
		}
		if parcel.Handling.Hot && (parcel.Handling.Chilled || parcel.Handling.Frozen) {
			return "HANDLING_INCOMPATIBLE"
		}
	}
	if weight > capability.MaxWeightGrams || volume > capability.MaxVolumeMl || packages > capability.MaxPackageCount {
		return "LOAD_EXCEEDS_CAPACITY"
	}
	return ""
}

func routePoints(request dispatchcommerce.QuoteRequest) ([]RoutePoint, map[string][]string) {
	physical := map[string][]string{}
	location := map[string]dispatchcommerce.Point{}
	for _, stop := range request.PickupStops {
		key := strings.TrimSpace(stop.PhysicalStopRef)
		if key == "" {
			key = stop.StopRef
		}
		physical[key] = append(physical[key], stop.StopRef)
		location[key] = stop.Location
	}
	keys := make([]string, 0, len(physical))
	for key := range physical {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	points := make([]RoutePoint, 0, len(keys)+1)
	for _, key := range keys {
		point := location[key]
		points = append(points, RoutePoint{Ref: key, Latitude: point.Latitude, Longitude: point.Longitude})
	}
	points = append(points, RoutePoint{Ref: "__receiver__", Latitude: request.Receiver.Latitude, Longitude: request.Receiver.Longitude})
	return points, physical
}

func expandSequence(plan RoutePlan, physical map[string][]string, stops []dispatchcommerce.PickupStop) ([]dispatchcommerce.RouteStop, bool) {
	byRef := map[string]dispatchcommerce.PickupStop{}
	for _, stop := range stops {
		byRef[stop.StopRef] = stop
	}
	sequence := make([]dispatchcommerce.RouteStop, 0, len(stops)+1)
	n := 1
	for _, physicalRef := range plan.OrderedRefs {
		if physicalRef == "__receiver__" {
			sequence = append(sequence, dispatchcommerce.RouteStop{StopRef: physicalRef, PhysicalStopRef: physicalRef, Sequence: n, DistanceMeters: plan.DistanceMeters, DurationSeconds: plan.DurationSeconds})
			n++
			continue
		}
		logical := append([]string(nil), physical[physicalRef]...)
		sort.Strings(logical)
		if len(logical) == 0 {
			return nil, false
		}
		for _, stopRef := range logical {
			if _, ok := byRef[stopRef]; !ok {
				return nil, false
			}
			sequence = append(sequence, dispatchcommerce.RouteStop{StopRef: stopRef, PhysicalStopRef: physicalRef, Sequence: n})
			n++
		}
	}
	return sequence, len(sequence) == len(stops)+1
}

func validateWindows(request dispatchcommerce.QuoteRequest, sequence []dispatchcommerce.RouteStop, totalSeconds int64, now time.Time) string {
	if totalSeconds <= 0 {
		return ReasonRouteUnavailable
	}
	byRef := map[string]dispatchcommerce.PickupStop{}
	for _, stop := range request.PickupStops {
		byRef[stop.StopRef] = stop
	}
	pickups := int64(len(request.PickupStops))
	for _, item := range sequence {
		if item.StopRef == "__receiver__" {
			continue
		}
		stop := byRef[item.StopRef]
		arrival := now.Add(time.Duration(totalSeconds*int64(item.Sequence)/maxInt64(1, pickups+1)) * time.Second)
		if arrival.After(stop.ReadyWindow.To) {
			return ReasonReadyWindow
		}
	}
	if !request.PromisedBy.IsZero() && now.Add(time.Duration(totalSeconds)*time.Second).After(request.PromisedBy) {
		return ReasonSLA
	}
	return ""
}

func multiStopPackageSize(parcels []dispatchcommerce.Parcel) string {
	var weight, volume int64
	var dimension int64
	for _, parcel := range parcels {
		weight += parcel.WeightGrams
		volume += parcel.VolumeMl
		if parcel.LengthMm > dimension {
			dimension = parcel.LengthMm
		}
		if parcel.WidthMm > dimension {
			dimension = parcel.WidthMm
		}
		if parcel.HeightMm > dimension {
			dimension = parcel.HeightMm
		}
	}
	if weight > 8000 || volume > 30000 || dimension > 450 {
		return dispatchpricing.SizeLarge
	}
	if weight > 3000 || volume > 10000 || dimension > 250 {
		return dispatchpricing.SizeMedium
	}
	return dispatchpricing.SizeSmall
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
