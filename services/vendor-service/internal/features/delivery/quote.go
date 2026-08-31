package delivery

// Vendor Own Delivery evaluation.
//
// Pure and deterministic: no database, no clock, no network. Everything it
// needs is in the request, which is what makes the whole rule set — reach,
// minimum order, weight limit, charge and funding attribution — testable
// without a vendor, a branch or a running service.
//
// This is the ONLY place in Vendor Service permitted to calculate a delivery
// charge, and it is permitted only because the vendor both performs and prices
// this delivery. A platform-executed delivery fee is never computed here; it
// comes from the owning logistics authority's own quote.

import (
	"math"
	"strings"

	"cosmicforge/logistics/services/vendor-service/internal/platform/geo"
)

// refuse builds a non-serviceable decision. It deliberately carries the
// distance it managed to compute (zero when it could not), so an operator
// debugging "why did this vendor not deliver" sees the same number the rule
// saw.
func refuse(reason string, distanceKm float64, policyVersion int64) Decision {
	return Decision{
		Serviceable:   false,
		ReasonCode:    reason,
		DistanceKm:    distanceKm,
		PolicyVersion: policyVersion,
	}
}

// effectiveRadiusMeters is the reach the branch actually has.
//
// vendor_branch_service_areas is the authority; the policy override may only
// narrow it. Taking the minimum is what makes that structural rather than a
// promise: a vendor writing a larger radius into its delivery policy changes
// nothing.
func effectiveRadiusMeters(area ServiceArea, override *int) int {
	radius := area.RadiusMeters

	if override != nil && *override > 0 && *override < radius {
		radius = *override
	}

	return radius
}

// Quote answers whether this vendor will deliver this group itself, and for
// how much.
func Quote(request QuoteRequest) Decision {
	policy := request.Policy

	// Two independent switches, refused with two distinct reasons. A vendor
	// asking "why is my own delivery not being offered" must be able to tell
	// "the platform has it switched off" from "you have it switched off".
	if !request.PlatformEnabled {
		return refuse(ReasonPlatformDisabled, 0, policy.Version)
	}

	if !policy.Enabled {
		return refuse(ReasonVendorDisabled, 0, policy.Version)
	}

	// Currency is compared, never converted. Vendor Service holds no exchange
	// rate authority, so a mismatch fails closed rather than guessing.
	if !strings.EqualFold(
		strings.TrimSpace(policy.Currency),
		strings.TrimSpace(request.Currency),
	) {
		return refuse(ReasonCurrencyMismatch, 0, policy.Version)
	}

	if !geo.ValidCoordinate(request.PickupLatitude, request.PickupLongitude) ||
		!geo.ValidCoordinate(request.DropoffLatitude, request.DropoffLongitude) {
		return refuse(ReasonInvalidCoordinate, 0, policy.Version)
	}

	// A branch that never configured a service area has not stated any reach.
	// That is a refusal, not an unlimited one.
	if !request.HasArea || request.Area.RadiusMeters <= 0 {
		return refuse(ReasonNoServiceArea, 0, policy.Version)
	}

	distanceKm := geo.HaversineKm(
		request.PickupLatitude, request.PickupLongitude,
		request.DropoffLatitude, request.DropoffLongitude,
	)

	// Reach is measured from the service-area centre, which Section 6 allows
	// to differ from the branch pin — a vendor may deliver around a market
	// rather than around its own doorway.
	if !geo.WithinRadiusMeters(
		request.Area.CenterLatitude, request.Area.CenterLongitude,
		request.DropoffLatitude, request.DropoffLongitude,
		effectiveRadiusMeters(request.Area, policy.MaxDeliveryRadiusMeters),
	) {
		return refuse(ReasonOutsideArea, distanceKm, policy.Version)
	}

	// The minimum order is a serviceability rule, not a surcharge: under it
	// this vendor does not deliver at all.
	if request.OrderValueMinor < policy.MinimumOrderMinor {
		return refuse(ReasonBelowMinimum, distanceKm, policy.Version)
	}

	if policy.MaxPackageWeightGrams != nil &&
		request.TotalWeightGrams > *policy.MaxPackageWeightGrams {
		return refuse(ReasonOverWeightLimit, distanceKm, policy.Version)
	}

	gross := policy.BaseFeeMinor +
		int64(math.Round(float64(policy.PerKmFeeMinor)*distanceKm))

	if gross < 0 {
		gross = 0
	}

	funding := Funding{
		GrossFeeMinor:    gross,
		CustomerFeeMinor: gross,
	}

	// Free delivery is a vendor-funded promotion, not an absence of cost. The
	// gross stays on the record and moves to the vendor's side of the split,
	// so settlement can still answer who paid for it.
	if policy.FreeDeliveryThresholdMinor != nil &&
		request.OrderValueMinor >= *policy.FreeDeliveryThresholdMinor {
		funding.CustomerFeeMinor = 0
		funding.VendorFundedMinor = gross
	}

	minutes := policy.EstimatedMinutesBase +
		int(math.Round(float64(policy.EstimatedMinutesPerKm)*distanceKm))

	if minutes < 0 {
		minutes = 0
	}

	return Decision{
		Serviceable:      true,
		ReasonCode:       ReasonServiceable,
		FeeBeneficiary:   BeneficiaryVendor,
		Currency:         strings.ToUpper(strings.TrimSpace(policy.Currency)),
		Funding:          funding,
		DistanceKm:       distanceKm,
		EstimatedMinutes: minutes,
		PolicyVersion:    policy.Version,
	}
}
