package commercequote

import (
	"time"

	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/logisticscapability"
)

// LogisticsCapabilityOptions carry the deployment facts that decide what
// Dispatch can actually do right now.
type LogisticsCapabilityOptions struct {
	// Enabled reports whether the commerce integration is switched on for
	// this deployment. A disabled authority declares itself unavailable
	// rather than pretending, which is what stops Vendor selecting it.
	Enabled bool
	// CorporateProviderSupported reports whether organization-managed (Fleet)
	// riders may execute Dispatch commerce work here.
	CorporateProviderSupported bool
	Now                        func() time.Time
}

// DeclaredLogisticsCapability is Dispatch's truthful statement in the shared
// cross-authority vocabulary (VND-006).
//
// It restates the SAME numbers the existing motorcycle capability already
// publishes, rather than inventing a second set: Capability() above and this
// share maxWeightGrams, maxVolumeMl, maxDimensionMm, maxPackageCount and
// dispatchHandling, so the two answers can never disagree about what a
// motorcycle can carry.
func DeclaredLogisticsCapability(options LogisticsCapabilityOptions) logisticscapability.Capability {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	capability := logisticscapability.Capability{
		Authority: logisticscapability.AuthorityDispatch,
		Version:   capabilityVersion,
		Available: options.Enabled,

		MultiPickupSupported: true,
		MaxPickups:           dispatchcommerce.MaxPickupStops,

		// One consolidated run delivers to one receiver.
		MultiStopSupported: false,
		MaxDropoffs:        1,

		SecurePickupSupported: true,

		// Dispatch's handover challenges are keyed per mission AND stop, so
		// every pickup it routes can carry its own credential.
		MaxSecurePickups: dispatchcommerce.MaxPickupStops,

		SecureDeliverySupported:    true,
		CustodyEvidenceSupported:   true,
		CorporateProviderSupported: options.CorporateProviderSupported,

		VehicleClasses:        []string{"MOTORCYCLE"},
		MaxWeightGrams:        maxWeightGrams,
		MaxVolumeMl:           maxVolumeMl,
		MaxLongestDimensionMm: maxDimensionMm,
		MaxPackageCount:       maxPackageCount,
		HandlingSupported:     append([]string(nil), dispatchHandling...),
		ObservedAt:            now().UTC(),
	}
	if !options.Enabled {
		capability.UnavailableReason = "DISPATCH_COMMERCE_INTEGRATION_DISABLED"
	}
	return capability
}

// LogisticsCapabilityProvider adapts the declaration to the shared server
// helper.
type LogisticsCapabilityProvider struct{ Options LogisticsCapabilityOptions }

// DeclaredCapability satisfies logisticscapability.Provider.
func (p LogisticsCapabilityProvider) DeclaredCapability() logisticscapability.Capability {
	return DeclaredLogisticsCapability(p.Options)
}
