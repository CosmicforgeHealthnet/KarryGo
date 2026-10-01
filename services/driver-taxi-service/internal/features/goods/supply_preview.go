package goods

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// Taxi Goods' answer to the read-only planning question.
//
// Vendor's just-in-time orchestration asks this before it has committed to
// anything. Without it the only way to learn whether supply exists would be to
// create a mission and watch, which is the premature-dispatch problem the whole
// design exists to avoid.
//
// Everything here is advisory. It creates no mission, no booking, no offer and
// no reservation; it alters no provider's availability; and it names nobody.
// The last point is not politeness: returning a provider identity would make
// the preview a de-facto assignment, and choosing a provider is not Vendor's to
// do.

// Previewer answers supply questions for Taxi Goods.
type Previewer struct {
	service *Service
	enabled bool
}

// NewPreviewer builds the previewer.
func NewPreviewer(service *Service, enabled bool) *Previewer {
	return &Previewer{service: service, enabled: enabled}
}

// limitedSupplyThreshold is the count below which supply is reported as thin.
//
// Two is chosen because Taxi Goods is a secondary use of a passenger fleet: a
// single eligible driver is one accepted passenger ride away from none, and a
// planner told AVAILABLE on that basis would commit and then fail to match.
const limitedSupplyThreshold = 2

// PreviewSupply answers whether Taxi Goods probably could, right now.
//
// It returns Unknown with no error whenever it cannot tell. An error would push
// every caller into writing its own fallback, and the first one to get it wrong
// would report a whole market as unserved because one lookup timed out.
func (p *Previewer) PreviewSupply(c *gin.Context, request supplypreview.Request) (supplypreview.Response, error) {
	now := time.Now().UTC()
	response := supplypreview.Response{
		Authority: "TAXI", CheckedAt: now, SupplyStatus: supplypreview.StatusUnknown,
	}
	if p == nil || !p.enabled || p.service == nil {
		response.ReasonCode = supplypreview.ReasonDependencyUnavailable
		return response, nil
	}
	service := p.service
	if service.markets == nil || service.candidates == nil || service.repo == nil {
		response.ReasonCode = supplypreview.ReasonDependencyUnavailable
		return response, nil
	}

	pickup := geo.Point{Lat: request.PickupLatitude, Lng: request.PickupLongitude}
	zone, err := service.markets.ResolveZone(c, pickup)
	if err != nil {
		// Outside every operating zone. This is a real answer about the
		// location, not a failure to answer, so it is NOT_SERVICEABLE rather
		// than UNKNOWN.
		response.ReasonCode = supplypreview.ReasonNotServiceable
		response.SupplyStatus = supplypreview.StatusUnknown
		return response, nil
	}
	if request.DropoffLatitude != 0 || request.DropoffLongitude != 0 {
		dropoff := geo.Point{Lat: request.DropoffLatitude, Lng: request.DropoffLongitude}
		dropZone, dropErr := service.markets.ResolveZone(c, dropoff)
		if dropErr != nil || dropZone.MarketID != zone.MarketID {
			// Taxi Goods does not run between markets, and saying so is a fact
			// about the route rather than about supply.
			response.ReasonCode = supplypreview.ReasonNotServiceable
			return response, nil
		}
	}

	// Taxi Goods carries one pickup. Reporting supply for a multi-pickup
	// request would answer a question this authority cannot execute.
	if request.PickupCount > 1 {
		response.ReasonCode = "MULTI_PICKUP_NOT_ALLOWED"
		return response, nil
	}

	ids, err := service.nearbyIDs(c, zone.MarketID, pickup)
	if err != nil {
		response.ReasonCode = supplypreview.ReasonDependencyUnavailable
		return response, nil
	}
	if len(ids) == 0 {
		// The market is served and nobody is near. That is a supply answer, and
		// a planner may legitimately try again in a minute.
		response.ReasonCode = supplypreview.ReasonSupplyUnavailable
		return response, nil
	}

	eligible, err := service.repo.EligibleCandidates(c, ids, loadFromPreview(request))
	if err != nil {
		response.ReasonCode = supplypreview.ReasonDependencyUnavailable
		return response, nil
	}
	if len(eligible) == 0 {
		// There were drivers; none of them fit. Distinct from
		// SUPPLY_CURRENTLY_UNAVAILABLE, because waiting will not help if the
		// load simply does not go in a car.
		response.ReasonCode = supplypreview.ReasonNoEligibleProvider
		return response, nil
	}

	response.Serviceable = true
	response.ReasonCode = supplypreview.ReasonServiceable
	response.SupplyStatus = supplypreview.StatusAvailable
	if len(eligible) < limitedSupplyThreshold {
		response.SupplyStatus = supplypreview.StatusLimited
	}
	response.ProductCode = strings.TrimSpace(request.ProductCode)
	return response, nil
}

// loadFromPreview projects the preview request onto the load shape the
// eligibility query already understands.
//
// It reuses the quote contract's Load rather than introducing a second shape,
// so the preview and the quote ask the candidate store exactly the same
// question. Two shapes would eventually answer differently, and the preview
// would promise supply the quote then refused.
func loadFromPreview(request supplypreview.Request) commercequote.Load {
	load := commercequote.Load{
		PackageCount:       request.PackageCount,
		TotalWeightGrams:   request.WeightGrams,
		TotalVolumeMl:      request.VolumeMl,
		LongestDimensionMm: int(request.LongestDimensionMm()),
	}
	for _, code := range supplypreview.NormalizeHandling(request.Handling) {
		load.Capabilities = append(load.Capabilities, code)
	}
	return load
}
