package bikecapability

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// NearbyProviders is the slice of Dispatch's availability index the preview
// needs.
//
// It returns only what is required to count candidates and estimate an ETA. A
// preview handed the full provider status would be one careless field away
// from leaking an identity; this interface makes that leak impossible rather
// than merely unlikely.
type NearbyProviders interface {
	NearbyForPreview(ctx context.Context, latitude, longitude, radiusKm float64, limit int) ([]NearbyProvider, error)
}

// NearbyProvider is one anonymous candidate.
//
// ProviderID is present because the preview must load that provider's bikes to
// answer honestly, and it never leaves this package: supplypreview.Response has
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
	// AverageSpeedKmh converts distance into a pickup ETA. A crude constant
	// rather than a routing call on purpose: a preview is advisory, may fire
	// many times per order during planning, and must not put a routing
	// provider in the critical path of Vendor's planner.
	AverageSpeedKmh float64
}

// Previewer answers read-only supply questions.
//
// Everything it holds is a read. There is no repository writer, no
// availability mutator, no booking or mission creator and no offer queue, so
// the contract's "creates nothing" guarantee rests on what it was handed
// rather than on discipline at the call sites.
type Previewer struct {
	service         *Service
	nearby          NearbyProviders
	radiusKm        float64
	enabled         bool
	now             func() time.Time
	averageSpeedKmh float64

	// loadFits is the capability read, held as a field so a test can supply a
	// stub without a database. The seam exists to make "the preview only ever
	// reads" a testable claim rather than a stated one.
	loadFits func(context.Context, []string, parcelfit.Requirement) (map[string][]parcelfit.Capability, error)
}

// NewPreviewer builds the preview.
func NewPreviewer(options PreviewOptions) *Previewer {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	radius := options.RadiusKm
	if radius <= 0 {
		radius = 10
	}
	speed := options.AverageSpeedKmh
	if speed <= 0 {
		// A motorcycle filtering through Lagos traffic. Deliberately
		// pessimistic: an ETA a planner can beat is far less damaging than one
		// it cannot.
		speed = 18
	}
	previewer := &Previewer{
		service: options.Service, nearby: options.Nearby, radiusKm: radius,
		enabled: options.Enabled, now: now, averageSpeedKmh: speed,
	}
	if options.Service != nil {
		previewer.loadFits = options.Service.LoadProviderBikeFits
	}
	return previewer
}

// capabilityVersionForPreview pins what the answer was computed against, so a
// caller comparing a preview with a later serviceability read can tell whether
// the declared capability moved underneath it.
const capabilityVersionForPreview = 1

// previewCandidateLimit bounds the index read. A preview that scanned the
// whole fleet would be slower than the mission it is meant to precede.
const previewCandidateLimit = 50

// limitedSupplyThreshold is where AVAILABLE becomes LIMITED.
const limitedSupplyThreshold = 3

// PreviewSupply answers whether Dispatch could probably run this, right now.
//
// It creates nothing: no mission, no booking, no offer, no reservation, no
// provider lock, no payment state, and no change to any rider's availability.
// The response names no provider and no bike.
func (p *Previewer) PreviewSupply(c *gin.Context, request supplypreview.Request) (supplypreview.Response, error) {
	started := p.now()
	response := supplypreview.Response{
		Authority:         "DISPATCH",
		CheckedAt:         started.UTC(),
		CapabilityVersion: capabilityVersionForPreview,
	}

	if p == nil || !p.enabled || p.service == nil || p.nearby == nil || p.loadFits == nil {
		// A deployment with the preview switched off says so rather than
		// pretending it has no supply: UNKNOWN is retryable, "no riders" is
		// not, and a planner that reads the second stops asking.
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

	candidates, err := p.nearby.NearbyForPreview(ctx,
		request.PickupLatitude, request.PickupLongitude, p.radiusKm, previewCandidateLimit)
	if err != nil {
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
	firstRefusal := ""
	for _, candidate := range candidates {
		bikes := byProvider[candidate.ProviderID]
		if len(bikes) == 0 {
			continue
		}
		fits := false
		for _, bike := range bikes {
			decision := parcelfit.Evaluate(parcelfit.ResolutionFound, bike, requirement)
			if decision.Eligible {
				fits = true
				break
			}
			if firstRefusal == "" {
				firstRefusal = decision.PrimaryReason()
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

	if eligible == 0 {
		response.Serviceable = false
		response.SupplyStatus = supplypreview.StatusUnknown
		// Candidates existed and none fitted: a different problem from having
		// no candidates, and one a planner can act on by adjusting the parcel
		// rather than the city.
		response.ReasonCode = supplypreview.ReasonNoEligibleProvider
		if firstRefusal != "" && firstRefusal != capabilityfit.ReasonEligible {
			response.ReasonCode = firstRefusal
		}
		p.observe(response, started)
		return response, nil
	}

	response.Serviceable = true
	response.ReasonCode = supplypreview.ReasonServiceable
	response.SupplyStatus = supplypreview.StatusAvailable
	if eligible < limitedSupplyThreshold {
		// Thin supply is worth saying out loud: a planner that treats one
		// eligible rider as a guarantee is surprised when they are taken
		// between the preview and the commit.
		response.SupplyStatus = supplypreview.StatusLimited
	}
	if nearestEligibleKm >= 0 {
		response.EstimatedPickupETASeconds = int64(nearestEligibleKm / p.averageSpeedKmh * 3600)
	}
	p.observe(response, started)
	return response, nil
}

func (p *Previewer) observe(response supplypreview.Response, started time.Time) {
	observability.SupplyPreview(string(response.SupplyStatus), response.ReasonCode,
		float64(p.now().Sub(started).Milliseconds()))
}

// requirementFromPreview converts the shared preview request into Dispatch's
// own parcel requirement, so the preview and assignment evaluate identically.
//
// A second, preview-only fit rule is how a planner gets told "yes" by one code
// path and "no" by another for the same parcel.
func requirementFromPreview(request supplypreview.Request) parcelfit.Requirement {
	requirement := parcelfit.Requirement{
		WeightGrams:    request.WeightGrams,
		VolumeMl:       request.VolumeMl,
		LengthMm:       request.LengthMm,
		WidthMm:        request.WidthMm,
		HeightMm:       request.HeightMm,
		PackageCount:   request.PackageCount,
		VehicleClass:   request.VehicleClass,
		SecureHandover: request.SecurePickup || request.SecureDelivery,
	}
	for _, code := range supplypreview.NormalizeHandling(request.Handling) {
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
		case "ENCLOSED", "HIGH_VALUE":
			// High-value goods need a box that can be closed and secured, not
			// merely one that keeps rain off.
			requirement.EnclosureRequired = true
		}
	}
	return requirement
}

// validCoordinate rejects null island and out-of-range values.
func validCoordinate(lat, lng float64) bool {
	if lat == 0 && lng == 0 {
		return false
	}
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}
