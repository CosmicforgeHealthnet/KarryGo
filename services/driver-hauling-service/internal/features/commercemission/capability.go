package commercemission

import (
	"time"

	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/logisticscapability"
)

// MaxPickupStops is Hauling's declared consolidated-pickup bound.
//
// Hauling genuinely supports multi-pickup: commercequote routes across every
// PickupStop and commercemission persists them all, and Vendor's consolidated
// plans already send them. Declaring maxPickups=1 here would be untrue and
// would silently stop Vendor routing a legitimate consolidated haul.
//
// The bound is Hauling's own operational limit on one manifest, not a copy of
// another service's: Dispatch declares its own separately.
const MaxPickupStops = 10

// capabilityVersion increments when the declared capability below changes, so
// a consumer can tell a genuinely changed capability from a re-read.
//
// v2 declares MaxSecurePickups. Multi-pickup routing was and remains real;
// what changed is that the protected-pickup bound is now stated instead of
// being inferred from MaxPickups, which over-promised it.
const capabilityVersion = 2

// CapabilityOptions carry the deployment facts that decide what Hauling can
// actually do right now.
type CapabilityOptions struct {
	// Enabled reports whether the commerce-mission integration is switched on
	// for this deployment. A disabled authority declares itself unavailable
	// rather than pretending, which is what stops Vendor selecting it.
	Enabled bool
	// CorporateProviderSupported reports whether organization-managed (Fleet)
	// providers may execute Hauling commerce work here.
	CorporateProviderSupported bool
	Now                        func() time.Time
}

// DeclaredCapability is Hauling's truthful capability statement (HLG-031).
//
// Every value is a fact about this service, verified against its own code:
// multi-pickup because commercequote routes across stops; secure pickup and
// delivery because hauling_handover_challenges backs both; custody evidence
// because custody_manifests records the SENDER -> PROVIDER -> RECEIVER chain.
func DeclaredCapability(options CapabilityOptions) logisticscapability.Capability {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	capability := logisticscapability.Capability{
		Authority: logisticscapability.AuthorityHauling,
		Version:   capabilityVersion,
		Available: options.Enabled,

		MultiPickupSupported: true,
		MaxPickups:           MaxPickupStops,

		// One manifest is delivered to one receiver. Multi-DROPOFF is a
		// different capability from multi-pickup and Hauling does not have it.
		MultiStopSupported: false,
		MaxDropoffs:        1,

		SecurePickupSupported: true,

		// Hauling routes across up to MaxPickupStops, but its handover
		// challenge is keyed (booking_id, stage) with one ACTIVE row per
		// stage, so exactly one pickup of a mission can carry a credential.
		// Declaring the routing bound alone would let Vendor send a
		// protected multi-pickup mission whose second handover could never
		// be issued, let alone verified.
		MaxSecurePickups: 1,

		SecureDeliverySupported:    true,
		CustodyEvidenceSupported:   true,
		CorporateProviderSupported: options.CorporateProviderSupported,

		VehicleClasses: []string{
			string(commercequote.VehicleVan),
			string(commercequote.VehicleTruck),
		},
		HandlingSupported: []string{
			"FRAGILE", "UPRIGHT", "STACK_RESTRICTED", "SPILL_LIQUID",
			"CHILLED", "FROZEN", "HIGH_VALUE",
		},
		ObservedAt: now().UTC(),
	}
	if !options.Enabled {
		capability.UnavailableReason = "HAULING_COMMERCE_INTEGRATION_DISABLED"
	}
	return capability
}

// CapabilityProvider adapts the declaration to the shared server helper.
type CapabilityProvider struct{ Options CapabilityOptions }

// DeclaredCapability satisfies logisticscapability.Provider.
func (p CapabilityProvider) DeclaredCapability() logisticscapability.Capability {
	return DeclaredCapability(p.Options)
}
