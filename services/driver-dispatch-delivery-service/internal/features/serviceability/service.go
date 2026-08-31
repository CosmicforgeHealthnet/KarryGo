package serviceability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/dispatchpricing"
)

var (
	ErrUnavailable         = errors.New("serviceability unavailable")
	ErrFingerprintConflict = errors.New("idempotency key reused with different request")
)

const PolicyVersion = "dispatch_route_serviceability_v1"

type Store interface {
	CreateOrReplay(context.Context, Quote) (Quote, error)
}

type Options struct {
	Store        Store
	Routes       RoutingAdapter
	Zones        ZoneEvaluator
	Packages     PackagePolicyEvaluator
	TTL          time.Duration
	MaxRouteKM   float64
	DegradedMode bool
	Now          func() time.Time
}

type Service struct {
	store      Store
	routes     RoutingAdapter
	zones      ZoneEvaluator
	packages   PackagePolicyEvaluator
	ttl        time.Duration
	maxRouteKM float64
	degraded   bool
	now        func() time.Time
}

func New(options Options) *Service {
	if options.TTL <= 0 {
		options.TTL = 15 * time.Minute
	}
	if options.MaxRouteKM <= 0 {
		options.MaxRouteKM = 60
	}
	if options.Packages == nil {
		options.Packages = DefaultPackagePolicy{}
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: options.Store, routes: options.Routes, zones: options.Zones, packages: options.Packages, ttl: options.TTL, maxRouteKM: options.MaxRouteKM, degraded: options.DegradedMode, now: options.Now}
}

func fingerprint(request Request) (string, error) {
	request.SourceRef = strings.TrimSpace(request.SourceRef)
	request.VehicleClass = strings.ToUpper(strings.TrimSpace(request.VehicleClass))
	request.ServiceTier = strings.ToLower(strings.TrimSpace(request.ServiceTier))
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) Evaluate(ctx context.Context, customerID, idempotencyKey string, request Request) (Quote, error) {
	customerID, idempotencyKey = strings.TrimSpace(customerID), strings.TrimSpace(idempotencyKey)
	if customerID == "" || idempotencyKey == "" || request.Pickup.Validate() != nil || request.Dropoff.Validate() != nil || request.Package.Validate() != nil {
		return Quote{}, errors.New("invalid serviceability request")
	}
	request.VehicleClass = strings.ToUpper(strings.TrimSpace(request.VehicleClass))
	request.ServiceTier = dispatchpricing.NormalizeTier(request.ServiceTier)
	if request.SourceVersion <= 0 {
		request.SourceVersion = 1
	}
	fp, err := fingerprint(request)
	if err != nil {
		return Quote{}, ErrUnavailable
	}
	now := s.now().UTC()
	quote := Quote{ID: "dsq_" + strings.ReplaceAll(uuid.NewString(), "-", ""), CustomerID: customerID, IdempotencyKey: idempotencyKey, RequestFingerprint: fp, SourceType: "CUSTOMER", SourceRef: strings.TrimSpace(request.SourceRef), SourceVersion: request.SourceVersion, Pickup: request.Pickup, Dropoff: request.Dropoff, VehicleClass: request.VehicleClass, PolicyVersion: PolicyVersion, Package: request.Package, ExpiresAt: now.Add(s.ttl), CreatedAt: now, Version: 1}
	quote.Admission = s.packages.Evaluate(request.Package, now)

	if request.VehicleClass != "MOTORCYCLE" {
		quote.Decision, quote.ReasonCodes = DecisionOutsideServiceArea, []string{ReasonVehicleUnsupported}
		return s.persist(ctx, quote)
	}
	if s.zones == nil {
		quote.Decision, quote.ReasonCodes = DecisionOperationalReview, []string{ReasonPolicyReviewRequired}
		return s.persist(ctx, quote)
	}
	pickupInside, dropoffInside, restricted, available := s.zones.Evaluate(request.Pickup, request.Dropoff)
	if !available {
		quote.Decision, quote.ReasonCodes = DecisionOperationalReview, []string{ReasonPolicyReviewRequired}
		return s.persist(ctx, quote)
	}
	if restricted {
		quote.Decision, quote.ReasonCodes = DecisionOperationalReview, []string{ReasonRestrictedArea}
		return s.persist(ctx, quote)
	}
	if !pickupInside {
		quote.Decision, quote.ReasonCodes = DecisionOutsideServiceArea, []string{ReasonPickupOutsideZone}
		return s.persist(ctx, quote)
	}
	if !dropoffInside {
		quote.Decision, quote.ReasonCodes = DecisionOutsideServiceArea, []string{ReasonDropoffOutsideZone}
		return s.persist(ctx, quote)
	}
	if quote.Admission.Result == AdmissionRefusedProhibited || quote.Admission.Result == AdmissionRefusedUnsafe {
		quote.Decision, quote.ReasonCodes = DecisionOperationalReview, []string{ReasonPackageRefused}
		return s.persist(ctx, quote)
	}
	if quote.Admission.Result == AdmissionOperationalReview || quote.Admission.Result == AdmissionRequiresRepack || quote.Admission.Result == AdmissionRequiresReprice {
		quote.Decision, quote.ReasonCodes = DecisionOperationalReview, []string{ReasonPackageReview}
		return s.persist(ctx, quote)
	}

	if s.routes == nil {
		err = ErrUnavailable
	} else {
		quote.Route, err = s.routes.Route(ctx, request.Pickup, request.Dropoff, request.VehicleClass)
	}
	if err != nil {
		if !s.degraded {
			quote.Decision, quote.ReasonCodes = DecisionOperationalReview, []string{ReasonRouteUnavailable}
			return s.persist(ctx, quote)
		}
		quote.Route = degradedRoute(request.Pickup, request.Dropoff)
		quote.Decision, quote.ReasonCodes = DecisionServiceableWithConditions, []string{ReasonDegradedRouting}
	} else {
		quote.Decision, quote.ReasonCodes = DecisionServiceable, []string{ReasonServiceable}
	}
	if float64(quote.Route.DistanceMeters)/1000 > s.maxRouteKM {
		quote.Decision, quote.ReasonCodes = DecisionOutsideServiceArea, []string{ReasonRouteTooLong}
		quote.FareEstimateKobo = 0
		return s.persist(ctx, quote)
	}
	if quote.Admission.Result == AdmissionAcceptedWithConditions && quote.Decision == DecisionServiceable {
		quote.Decision, quote.ReasonCodes = DecisionServiceableWithConditions, append(quote.ReasonCodes, ReasonPackageReview)
	}
	distanceKM := float64(quote.Route.DistanceMeters) / 1000
	quote.FareEstimateKobo = dispatchpricing.FareKobo(distanceKM, request.Package.SizeCategory, request.ServiceTier)
	quote.Currency = "NGN"
	quote.PricingInputs = map[string]any{"distance_meters": quote.Route.DistanceMeters, "package_size": dispatchpricing.NormalizeSize(request.Package.SizeCategory), "service_tier": request.ServiceTier, "pricing_authority": "shared/go/dispatchpricing"}
	return s.persist(ctx, quote)
}

func (s *Service) persist(ctx context.Context, quote Quote) (Quote, error) {
	if s.store == nil {
		return Quote{}, ErrUnavailable
	}
	stored, err := s.store.CreateOrReplay(ctx, quote)
	if err == nil {
		log.Printf("dispatch serviceability evaluated decision=%s route_source=%s degraded=%t package_admission=%s", stored.Decision, stored.Route.Source, stored.Route.Degraded, stored.Admission.Result)
	}
	return stored, err
}
