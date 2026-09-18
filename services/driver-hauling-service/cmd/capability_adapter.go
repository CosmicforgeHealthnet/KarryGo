package main

import (
	"context"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	"cosmicforge/logistics/services/hauling-service/internal/features/truckcapability"
)

// capabilityAuthority adapts the truck-capability feature to the booking
// matcher's interface.
//
// It lives in the composition root, next to the existing truckLookup adapter,
// so neither feature has to import the other. The translation is deliberately
// thin -- every real decision is in truckcapability or cargofit -- because an
// adapter that makes judgements is an adapter nobody tests.
type capabilityAuthority struct {
	service *truckcapability.Service
}

// LoadProviderTrucks passes the requirement through so eligibility is
// projected onto each truck before the fit engine sees it.
func (a capabilityAuthority) LoadProviderTrucks(
	ctx context.Context,
	providerIDs []string,
	requirement cargofit.Requirement,
) (map[string][]cargofit.TruckCapability, error) {
	return a.service.LoadProviderTruckFits(ctx, providerIDs, requirement)
}

// RecordDecisions converts the matcher's evidence into the feature's own type.
//
// The two structs are identical in shape and kept separate because Go's
// nominal typing is what stops the booking usecase from importing the whole
// capability feature; the conversion is the price of that boundary and is
// cheap enough to pay per match.
func (a capabilityAuthority) RecordDecisions(ctx context.Context, decisions []bookingusecases.CapabilityDecision) error {
	converted := make([]truckcapability.MatchDecision, 0, len(decisions))
	for _, decision := range decisions {
		converted = append(converted, truckcapability.MatchDecision{
			BookingID:         decision.BookingID,
			ProviderID:        decision.ProviderID,
			TruckID:           decision.TruckID,
			Decision:          decision.Decision,
			ReasonCodes:       decision.ReasonCodes,
			Evidence:          decision.Evidence,
			DistanceM:         decision.DistanceM,
			CapabilityVersion: decision.CapabilityVersion,
			PolicyVersion:     decision.PolicyVersion,
			Shadow:            decision.Shadow,
		})
	}
	return a.service.RecordMatchDecisions(ctx, converted)
}

// PolicyVersion returns the locally-snapshotted capability policy version.
func (a capabilityAuthority) PolicyVersion(ctx context.Context) string {
	return a.service.CapabilityPolicyVersion(ctx)
}

// ResolveProduct names the product and whether it demands a certificate here.
func (a capabilityAuthority) ResolveProduct(requirement cargofit.Requirement) (string, bool) {
	return a.service.ProductForRequirement(requirement)
}

// The compiler is the check that the adapter still matches the matcher's
// interface, so a signature change on either side fails the build rather than
// silently disabling capability matching at runtime.
var _ bookingusecases.CapabilityAuthority = capabilityAuthority{}
