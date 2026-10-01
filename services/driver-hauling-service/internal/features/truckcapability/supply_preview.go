package truckcapability

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/services/hauling-service/internal/observability"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// NearbyProviders is the slice of Hauling's availability index the preview
// needs.
//
// It deliberately returns only what is required to count and to estimate an
// ETA. A preview that received the full provider status would be one careless
// field away from leaking an identity, and this interface makes that leak
// impossible rather than merely unlikely.
type NearbyProviders interface {
	NearbyProviders(ctx context.Context, center geo.Point, radiusKm float64, limit int) ([]NearbyProvider, error)
}

// NearbyProvider is one anonymous candidate.
//
// ProviderID is present because the preview must load that provider's trucks
// to answer honestly, and it never leaves this package: the response type has
// no field it could be written to.
type NearbyProvider struct {
	ProviderID string
	DistanceKm float64
}

// PreviewOptions configure the preview.
type PreviewOptions struct {
	Service  *Service
	Nearby   NearbyProviders
	RadiusKm float64
	Enabled  bool
	Now      func() time.Time
	// AverageSpeedKmh converts distance into a pickup ETA. It is a crude
	// constant rather than a routing call on purpose: a preview is advisory,
	// runs on a planning path that may fire many times per order, and must not
	// put a routing provider in the critical path of Vendor's planner.
	AverageSpeedKmh float64
}

// Previewer answers read-only supply questions.
//
// Everything it can do is a read. It holds no repository write method, no
// availability mutator and no booking creator, so the contract's "creates
// nothing" guarantee is enforced by what it was handed rather than by
// discipline at the call sites.
type Previewer struct {
	service         *Service
	nearby          NearbyProviders
	radiusKm        float64
	enabled         bool
	now             func() time.Time
	averageSpeedKmh float64

	// loadFits is the capability read, held as a field so a test can supply a
	// stub without a database. It defaults to the service's own method; the
	// seam exists to make "the preview only ever reads" a testable claim
	// rather than a stated one.
	loadFits func(context.Context, []string, cargofit.Requirement) (map[string][]cargofit.TruckCapability, error)
}

// NewPreviewer builds the preview.
func NewPreviewer(options PreviewOptions) *Previewer {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	radius := options.RadiusKm
	if radius <= 0 {
		radius = 25
	}
	speed := options.AverageSpeedKmh
	if speed <= 0 {
		// Lagos arterial traffic for a laden truck. Deliberately pessimistic:
		// an ETA a planner can beat is far less damaging than one it cannot.
		speed = 22
	}
	previewer := &Previewer{
		service: options.Service, nearby: options.Nearby, radiusKm: radius,
		enabled: options.Enabled, now: now, averageSpeedKmh: speed,
	}
	if options.Service != nil {
		previewer.loadFits = options.Service.LoadProviderTruckFits
	}
	return previewer
}

// capabilityVersionForPreview pins what the answer was computed against.
//
// It is the commerce capability document's version, so a caller comparing a
// preview against a later serviceability read can tell whether the declared
// capability moved underneath it.
const capabilityVersionForPreview = 2

// PreviewSupply answers whether Hauling could probably run this, right now.
//
// It creates nothing. No booking, no mission, no offer, no reservation, no
// lock, no payment state, and no change to any provider's availability. The
// response names no provider and no truck.
func (p *Previewer) PreviewSupply(c *gin.Context, request supplypreview.Request) (supplypreview.Response, error) {
	started := p.now()
	response := supplypreview.Response{
		Authority:         "HAULING",
		CheckedAt:         started.UTC(),
		CapabilityVersion: capabilityVersionForPreview,
	}

	if p == nil || !p.enabled || p.service == nil || p.nearby == nil || p.loadFits == nil {
		// A deployment with the preview switched off says so rather than
		// pretending it has no supply: UNKNOWN is a retryable answer, "no
		// trucks" is not.
		response.SupplyStatus = supplypreview.StatusUnknown
		response.ReasonCode = supplypreview.ReasonDependencyUnavailable
		observability.SupplyPreview(string(response.SupplyStatus), response.ReasonCode, 0)
		return response, nil
	}

	ctx := c.Request.Context()
	response.PolicyVersion = p.service.CapabilityPolicyVersion(ctx)

	requirement := requirementFromPreview(request)
	requirement.ProductCode, requirement.CertificationRequired = p.service.ProductForRequirement(requirement)
	response.ProductCode = requirement.ProductCode

	if !validCoordinate(request.PickupLatitude, request.PickupLongitude) {
		response.SupplyStatus = supplypreview.StatusUnknown
		response.ReasonCode = supplypreview.ReasonNotServiceable
		p.observe(response, started)
		return response, nil
	}

	candidates, err := p.nearby.NearbyProviders(ctx,
		geo.Point{Lat: request.PickupLatitude, Lng: request.PickupLongitude},
		p.radiusKm, previewCandidateLimit)
	if err != nil {
		// The availability index did not answer. That is a dependency
		// problem, never "this city has no trucks" -- a planner that reads the
		// second stops routing to Hauling entirely.
		response.SupplyStatus = supplypreview.StatusUnknown
		response.ReasonCode = supplypreview.ReasonDependencyUnavailable
		p.observe(response, started)
		return response, nil
	}
	if len(candidates) == 0 {
		response.SupplyStatus = supplypreview.StatusUnknown
		response.ReasonCode = supplypreview.ReasonSupplyUnavailable
		p.observe(response, started)
		return response, nil
	}

	providerIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		providerIDs = append(providerIDs, candidate.ProviderID)
	}

	byProvider, err := p.loadFits(ctx, providerIDs, requirement)
	if err != nil {
		response.SupplyStatus = supplypreview.StatusUnknown
		response.ReasonCode = supplypreview.ReasonDependencyUnavailable
		p.observe(response, started)
		return response, nil
	}

	eligible := 0
	nearestEligibleKm := -1.0
	commonestRefusal := ""
	for _, candidate := range candidates {
		trucks := byProvider[candidate.ProviderID]
		if len(trucks) == 0 {
			continue
		}
		fits := false
		for _, truck := range trucks {
			decision := cargofit.Evaluate(cargofit.ResolutionFound, truck, requirement)
			if decision.Eligible {
				fits = true
				break
			}
			if commonestRefusal == "" {
				commonestRefusal = decision.PrimaryReason()
			}
		}
		if !fits {
			continue
		}
		eligible++
		if nearestEligibleKm < 0 || candidate.DistanceKm < nearestEligibleKm {
			nearestEligibleKm = candidate.DistanceKm
		}
	}

	switch {
	case eligible == 0:
		response.Serviceable = false
		response.SupplyStatus = supplypreview.StatusUnknown
		// Candidates existed and none fitted: that is a different problem from
		// having no candidates, and a planner can act on the difference by
		// adjusting the load rather than the city.
		response.ReasonCode = supplypreview.ReasonNoEligibleProvider
		if commonestRefusal != "" && commonestRefusal != capabilityfit.ReasonEligible {
			response.ReasonCode = commonestRefusal
		}
	default:
		response.Serviceable = true
		response.ReasonCode = supplypreview.ReasonServiceable
		response.SupplyStatus = supplypreview.StatusAvailable
		if eligible < limitedSupplyThreshold {
			// Thin supply is worth saying out loud: a planner that treats one
			// eligible truck as a guarantee will be surprised when it is taken
			// between the preview and the commit.
			response.SupplyStatus = supplypreview.StatusLimited
		}
		if nearestEligibleKm >= 0 {
			response.EstimatedPickupETASeconds = int64(nearestEligibleKm / p.averageSpeedKmh * 3600)
		}
	}

	p.observe(response, started)
	return response, nil
}

// previewCandidateLimit bounds the index read. A preview that scanned the
// whole fleet would be slower than the booking it is meant to precede.
const previewCandidateLimit = 50

// limitedSupplyThreshold is where AVAILABLE becomes LIMITED.
const limitedSupplyThreshold = 3

func (p *Previewer) observe(response supplypreview.Response, started time.Time) {
	observability.SupplyPreview(string(response.SupplyStatus), response.ReasonCode,
		float64(p.now().Sub(started).Milliseconds()))
}

// requirementFromPreview converts the shared preview request into Hauling's
// own cargo requirement, so the preview and the matcher evaluate identically.
//
// Using a second, preview-only fit rule is how a planner ends up being told
// "yes" by one code path and "no" by another for the same load.
func requirementFromPreview(request supplypreview.Request) cargofit.Requirement {
	handling := supplypreview.NormalizeHandling(request.Handling)
	requirement := cargofit.Requirement{
		WeightGrams:  request.WeightGrams,
		VolumeMl:     request.VolumeMl,
		LengthMm:     request.LengthMm,
		WidthMm:      request.WidthMm,
		HeightMm:     request.HeightMm,
		PackageCount: request.PackageCount,
		VehicleClass: request.VehicleClass,
		GoodsClasses: []string{"GENERAL"},
	}
	for _, code := range handling {
		switch code {
		case "FRAGILE":
			requirement.Fragile = true
			requirement.UprightRequired = true
		case "UPRIGHT":
			requirement.UprightRequired = true
		case "CHILLED":
			requirement.TemperatureClass = "CHILLED"
		case "FROZEN":
			requirement.TemperatureClass = "FROZEN"
		case "WEATHER_PROTECTED":
			requirement.WeatherProtectionRequired = true
		case "ENCLOSED":
			requirement.EnclosureRequired = true
		case "TAIL_LIFT":
			requirement.TailLiftRequired = true
		case "CRANE":
			requirement.CraneRequired = true
		case "SPILL_LIQUID":
			requirement.GoodsClasses = []string{"LIQUID_BULK"}
		}
	}
	return requirement
}

// validCoordinate rejects null island and out-of-range values, matching the
// guard booking creation already applies.
func validCoordinate(lat, lng float64) bool {
	if lat == 0 && lng == 0 {
		return false
	}
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}
