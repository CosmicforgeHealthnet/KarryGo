package bookingusecases

import (
	"context"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/hauling-service/internal/observability"
	"cosmicforge/logistics/shared/go/logging"
)

// CapabilityAuthority is Hauling's own capability store, seen from the
// matcher.
//
// It is an interface rather than a concrete dependency for the usual reason
// (the booking usecase must not import the whole truckcapability feature) and
// one specific one: LoadProviderTrucks returning an error is what lets the
// matcher tell a store outage from an empty fleet. A concrete call that
// returned an empty map on failure would reproduce the exact fail-open bug
// this work exists to remove.
type CapabilityAuthority interface {
	// LoadProviderTrucks returns every active truck, with capability, for each
	// provider. A provider absent from the map has no active truck; an error
	// means the store could not answer at all.
	//
	// The requirement is passed in so the authority can project product
	// eligibility onto each truck before the fit engine sees it. That keeps
	// the certification gate inside the one Evaluate call rather than as a
	// second check every call site has to remember.
	LoadProviderTrucks(ctx context.Context, providerIDs []string, requirement cargofit.Requirement) (map[string][]cargofit.TruckCapability, error)

	// RecordDecisions persists hard-fit evidence. Failures are swallowed by
	// the caller: a booking must never fail to match because an operations
	// table was unavailable.
	RecordDecisions(ctx context.Context, decisions []CapabilityDecision) error

	// PolicyVersion is the locally-snapshotted policy matching was evaluated
	// against.
	PolicyVersion(ctx context.Context) string

	// ResolveProduct names the service product a requirement runs under and
	// whether this deployment demands a certificate for it. It is deployment
	// policy, not a property of the requirement, so the authority answers it.
	ResolveProduct(requirement cargofit.Requirement) (productCode string, certificationRequired bool)
}

// CapabilityDecision is one candidate's recorded outcome.
type CapabilityDecision struct {
	BookingID         string
	ProviderID        string
	TruckID           string
	Decision          string
	ReasonCodes       string
	Evidence          map[string]string
	DistanceM         int64
	CapabilityVersion int64
	PolicyVersion     string
	Shadow            bool
}

// Decision outcomes, mirroring the truckcapability vocabulary.
const (
	capabilityDecisionEligible = "ELIGIBLE"
	capabilityDecisionRejected = "REJECTED"
)

// CapabilityMatchingConfig carries the two Section 9.13 flags.
//
// The pair expresses three states, and the middle one is the point of the
// design:
//
//	Enabled=false                  legacy only; nothing is computed
//	Enabled=true,  FailClosed=false SHADOW: decide, record, do not enforce
//	Enabled=true,  FailClosed=true  ENFORCED: the decision selects
//
// Shadow exists because switching hard fit on blind would, on a fleet that has
// never been surveyed, refuse almost every candidate on CAPABILITY_UNKNOWN and
// look exactly like a total outage. Shadow makes that measurable first.
type CapabilityMatchingConfig struct {
	Enabled    bool
	FailClosed bool
}

// Shadow reports whether decisions are computed but not enforced.
func (c CapabilityMatchingConfig) Shadow() bool { return c.Enabled && !c.FailClosed }

// requirementForBooking normalizes a stored booking into the shared cargo
// requirement.
//
// Legacy direct bookings carry a weight in kilograms and nothing else, so most
// fields stay zero and therefore constrain nothing. DimensionsRequired is
// deliberately NOT set here: a customer app that has never collected
// dimensions must not have every one of its bookings refused.
func requirementForBooking(booking bookingmodels.Booking) cargofit.Requirement {
	requirement := cargofit.Requirement{
		WeightGrams:        cargofit.FromKilograms(booking.CargoWeightKg),
		PreferredTruckType: booking.PreferredTruckType,
		Fragile:            booking.IsFragile,
	}
	// A fragile load must stay the way it was loaded, so a body that
	// discharges by tilting is disqualified even though nobody ticked
	// "upright". This is the one inference the requirement makes, and it is
	// safe in the conservative direction: it can only narrow the candidate
	// set.
	if booking.IsFragile {
		requirement.UprightRequired = true
	}
	return requirement
}

// capabilityFilter applies hard fit to a proximity-ordered candidate list.
//
// It returns the survivors plus the evidence for every candidate considered.
// In shadow mode the caller keeps the legacy list and only stores the
// evidence, which is what makes the rollout measurable without changing a
// single assignment.
func (s *BookingService) capabilityFilter(
	ctx context.Context,
	booking bookingmodels.Booking,
	candidates []availabilityrepositories.ProviderStatus,
	distances map[string]float64,
) (eligible []availabilityrepositories.ProviderStatus, decisions []CapabilityDecision) {
	if s.capability == nil || len(candidates) == 0 {
		return candidates, nil
	}

	providerIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		providerIDs = append(providerIDs, candidate.ProviderID)
	}

	requirement := requirementForBooking(booking)
	requirement.ProductCode, requirement.CertificationRequired = s.capability.ResolveProduct(requirement)

	resolution := cargofit.ResolutionFound
	byProvider, err := s.capability.LoadProviderTrucks(ctx, providerIDs, requirement)
	if err != nil {
		// The store did not answer. Every candidate is therefore
		// DEPENDENCY_UNAVAILABLE -- not "too small", not "wrong truck". The
		// old code turned this into proximity-only matching, which is how an
		// outage silently dispatched an unverified vehicle.
		resolution = cargofit.ResolutionFailed
		byProvider = nil
		observability.TruckLookupFailure()
		logging.Error("hauling-capability", "truck capability lookup failed for booking %s: %v", booking.ID, err)
	}

	policyVersion := s.capability.PolicyVersion(ctx)
	shadow := s.capabilityMatching.Shadow()

	for _, candidate := range candidates {
		truck, decision := cargofit.EvaluateProvider(byProvider[candidate.ProviderID], resolution, requirement)

		outcome := capabilityDecisionRejected
		if decision.Eligible {
			outcome = capabilityDecisionEligible
			eligible = append(eligible, candidate)
		}

		observability.CargoFitOutcome(decision.PrimaryReason(), shadow)

		decisions = append(decisions, CapabilityDecision{
			BookingID:         booking.ID,
			ProviderID:        candidate.ProviderID,
			TruckID:           truck.TruckID,
			Decision:          outcome,
			ReasonCodes:       decision.ReasonString(),
			Evidence:          decision.Details,
			DistanceM:         int64(distances[candidate.ProviderID] * 1000),
			CapabilityVersion: truck.CapabilityVersion,
			PolicyVersion:     policyVersion,
			Shadow:            shadow,
		})
	}
	observability.EligibleCandidates(len(candidates), len(eligible), shadow)
	return eligible, decisions
}

// recordCapabilityDecisions persists evidence without ever failing a match.
func (s *BookingService) recordCapabilityDecisions(ctx context.Context, decisions []CapabilityDecision) {
	if s.capability == nil || len(decisions) == 0 {
		return
	}
	// A detached context: the matcher's own context is cancelled the moment a
	// provider accepts, and losing the evidence for the match that just
	// succeeded is precisely the case an operator most wants to read.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.capability.RecordDecisions(writeCtx, decisions); err != nil {
		logging.Error("hauling-capability", "record match decisions: %v", err)
	}
}

// capabilityShortfall summarizes why a candidate set emptied out, for logs.
func capabilityShortfall(decisions []CapabilityDecision) string {
	counts := map[string]int{}
	for _, decision := range decisions {
		if decision.Decision == capabilityDecisionRejected {
			for _, reason := range strings.Split(decision.ReasonCodes, ",") {
				if reason != "" {
					counts[reason]++
				}
			}
		}
	}
	if len(counts) == 0 {
		return ""
	}
	reasons := make([]string, 0, len(counts))
	for reason := range counts {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if counts[reasons[i]] != counts[reasons[j]] {
			return counts[reasons[i]] > counts[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, reason)
	}
	return strings.Join(parts, ",")
}
