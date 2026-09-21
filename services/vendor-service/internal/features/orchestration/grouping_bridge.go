package orchestration

// The bridge between what the orchestration stores and what the planner needs.
//
// The planner is a pure function over candidates and a policy. This file is
// what turns a persisted orchestration into that input and the resulting plan
// back into rows and evidence, and it is deliberately the only place that
// knows both shapes.

import (
	"context"
	"strconv"

	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
)

// GroupAssignment is the planner's decision about one pickup.
type GroupAssignment struct {
	GroupIndex  int
	SplitReason string
}

// candidatesFrom projects the stored pickups onto planner candidates, filling
// each one's predicted ready window from live readiness.
//
// Readiness is read rather than stored on the pickup because it moves: the
// snapshot on the pickup describes the goods, which do not change, while when
// the kitchen will be done changes every few minutes and re-planning on stale
// times is how a group is built around a vendor who is already late.
//
// A pickup whose readiness cannot be read still becomes a candidate, with the
// orchestration's own desired window. Dropping it would silently plan a trip
// that collects fewer orders than the orchestration holds, which is a worse
// failure than planning conservatively around an unknown.
func (s *Service) candidatesFrom(ctx context.Context, current Orchestration,
	pickups []Pickup) []GroupCandidate {
	candidates := make([]GroupCandidate, 0, len(pickups))
	for _, pickup := range pickups {
		candidate := GroupCandidate{
			VendorOrderID:       pickup.VendorOrderID,
			BusinessID:          pickup.BusinessID,
			BranchID:            pickup.BranchID,
			PredictedReadyStart: current.Requirement.DesiredPickupStart,
			PredictedReadyEnd:   current.Requirement.DesiredPickupEnd,
			Deadline:            current.Requirement.DeliveryDeadline,
			Latitude:            pickup.PickupLatitude,
			Longitude:           pickup.PickupLongitude,
			WeightGrams:         pickup.WeightGrams,
			VolumeMl:            pickup.VolumeMl,
			PackageCount:        pickup.PackageCount,
			Handling:            pickup.Handling,
			TemperatureBand:     pickup.TemperatureBand,
			SecurePickup:        pickup.SecurePickup || current.Requirement.SecurePickup,
		}
		if s.readiness != nil {
			if state, err := s.readiness.Read(ctx, pickup.VendorOrderID); err == nil {
				if state.PredictedReadyStart != nil {
					candidate.PredictedReadyStart = state.PredictedReadyStart.UTC()
				}
				if state.PredictedReadyEnd != nil {
					candidate.PredictedReadyEnd = state.PredictedReadyEnd.UTC()
				}
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// policyFor builds the grouping envelope from the selected authority's own
// declared capability.
//
// An authority that could not be asked keeps the conservative default rather
// than an optimistic one: the default allows a single pickup per trip, so an
// unreachable capability service costs efficiency and never correctness.
func (s *Service) policyFor(ctx context.Context, authority logistics.Authority) GroupingPolicy {
	policy := DefaultGroupingPolicy()
	if s.gate == nil || authority == "" {
		return policy
	}
	capability, err := s.gate.Capability(ctx, authority)
	if err != nil {
		return policy
	}
	return policy.FromCapability(capability)
}

// assignmentsFrom turns a plan into one assignment per vendor order.
//
// Every candidate the planner saw gets an entry, including the ones it split
// out, because "this pickup was considered and rejected from group 0 for this
// reason" is the answer an operator needs. A split candidate still gets a group
// of its own -- it is being collected, just not with the others.
func assignmentsFrom(plan Plan) map[string]GroupAssignment {
	assignments := map[string]GroupAssignment{}
	reasons := map[string]string{}
	for _, split := range plan.Splits {
		reasons[split.Candidate.VendorOrderID] = string(split.Reason)
	}
	for index, group := range plan.Groups {
		for _, member := range group.Members {
			assignments[member.VendorOrderID] = GroupAssignment{
				GroupIndex: index, SplitReason: reasons[member.VendorOrderID],
			}
		}
	}
	return assignments
}

// planGroupEvents turns a plan into append-only evidence.
//
// One event per group records what will travel together; one per split records
// what would not and why. Recording only the groups would leave the more
// interesting half unexplained: nobody asks why two orders shared a rider, and
// everybody asks why they did not.
func planGroupEvents(current Orchestration, plan Plan) []DecisionEvent {
	events := make([]DecisionEvent, 0, len(plan.Groups)+len(plan.Splits))
	for index, group := range plan.Groups {
		events = append(events, DecisionEvent{
			OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
			EventType:          EventGroupPlanned,
			CandidateAuthority: current.SelectedAuthority,
			CandidateProduct:   current.SelectedProductCode,
			OutcomeClass:       OutcomeServiceable,
			ReasonCode:         groupReason(index, len(group.Members)),
		})
	}
	for _, split := range plan.Splits {
		events = append(events, DecisionEvent{
			OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
			EventType:          EventGroupSplit,
			CandidateAuthority: current.SelectedAuthority,
			CandidateProduct:   current.SelectedProductCode,
			ReasonCode:         string(split.Reason),
		})
	}
	return events
}

// planGroups runs the multi-vendor planner and persists its decision.
//
// It runs after an authority is selected and before the commit window opens,
// because the policy it needs is the selected authority's own declared limits
// -- planning first and then discovering the authority carries one pickup would
// mean re-planning with custody arrangements already described to the vendors.
//
// It is idempotent in the way that matters: re-running it on an unchanged
// orchestration recomputes the same grouping from the same snapshot, and the
// assignment write is an update rather than an insert.
func (s *Service) planGroups(ctx context.Context, current Orchestration) error {
	if !s.jitEnabled {
		return nil
	}
	pickups, err := s.repo.Pickups(ctx, current.ID)
	if err != nil {
		return err
	}
	if len(pickups) == 0 {
		return nil
	}
	plan := GroupCandidates(s.candidatesFrom(ctx, current, pickups),
		s.policyFor(ctx, current.SelectedAuthority))

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, event := range planGroupEvents(current, plan) {
		if err = s.repo.RecordDecision(ctx, tx, event); err != nil {
			return err
		}
	}
	if err = s.repo.AssignGroups(ctx, tx, current.ID, assignmentsFrom(plan)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// groupReason names a planned group by its size, so the evidence reads as a
// statement rather than a bare index.
func groupReason(index, members int) string {
	if members > 1 {
		return "GROUP_" + strconv.Itoa(index) + "_SHARED_" + strconv.Itoa(members)
	}
	return "GROUP_" + strconv.Itoa(index) + "_SOLO"
}
