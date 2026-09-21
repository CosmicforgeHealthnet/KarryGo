package orders

// Just-in-time planning: the seam between the paid order and the orchestration.
//
// Everything about a commerce mission -- which authority, what fee, which plan,
// which parcels, which idempotency key -- is assembled in exactly one place,
// and that place is this feature. Just-in-time planning does not change what a
// mission IS. It changes WHEN the authority is asked.
//
// That distinction is the whole design. A second assembler would eventually
// disagree with this one about the approved fee or the plan binding, and two
// assemblers for one customer order is how two vehicles get dispatched. So the
// planner below hands the orchestration a REFERENCE to a mission slot this
// feature already made durable, and the orchestration hands it back at commit
// time -- it never carries the mission's contents.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrMissionNotPlanned means the reference names no mission slot this service
// owns. It is distinct from a transport failure: nothing should be retried.
var ErrMissionNotPlanned = errors.New("orders: no planned mission for this reference")

// PlannedMission is what the orchestration needs in order to plan a handover it
// will later ask this service to commit.
//
// It carries evidence and identity, never money decisions and never a provider.
// The fee is present only as the already-approved figure, so the orchestration
// can record what it is planning against; nothing in the planner may change it.
type PlannedMission struct {
	// MissionRef is the durable slot reference. It is this service's stable,
	// derived idempotency key and the only thing the orchestration needs to
	// give back at commit time.
	MissionRef     string
	PlanMissionRef string

	ParentOrderID  string
	CustomerID     string
	VendorOrderIDs []string

	// Service is the authority the frozen Section 17C plan already chose. With
	// automatic selection on, the orchestration may select a different one; the
	// committer then uses the orchestration's choice, because by then it is the
	// decision made from the authorities' own answers.
	Service      LogisticsService
	VehicleClass string

	// Execution is the immutable Section 17F evidence: plan binding, authority
	// quote binding, stops with coordinates and ready windows, and parcels.
	Execution MissionExecution

	// Delivery is the customer's destination, snapshotted at checkout.
	Delivery AddressSnapshot

	WeightGrams  int64
	PackageCount int64
}

// LogisticsPlanner hands a mission slot to the just-in-time orchestration
// instead of dispatching it now.
//
// It is a port rather than a direct dependency on the orchestration feature for
// the reason every other cross-feature edge here is: the orders feature must
// not learn another feature's persistence. The adapter is wired in the
// composition root.
type LogisticsPlanner interface {
	// PlanMission records that this slot will be committed later. It must be
	// idempotent: activation and recovery both run more than once, and a
	// planner that could not absorb a repeat would open a second orchestration
	// for one delivery.
	PlanMission(ctx context.Context, planned PlannedMission) error

	// CancelMissionPlan withdraws a slot that has not been committed. It
	// reports whether a plan was actually withdrawn, so the caller can tell
	// "nothing was planned" from "a committed mission cannot be withdrawn
	// here".
	CancelMissionPlan(ctx context.Context, missionRef, reasonCode string) (bool, error)
}

// SetLogisticsPlanner wires just-in-time planning.
//
// A nil planner is the legacy deployment and is entirely normal: activation
// then dispatches synchronously exactly as it did before. That is why this is a
// separate setter rather than a constructor argument -- the absence of a
// planner must be an ordinary configuration, not a half-built service.
func (s *Service) SetLogisticsPlanner(planner LogisticsPlanner) {
	s.logisticsPlanner = planner
}

// JITPlanningEnabled reports whether this deployment defers dispatch.
//
// It is deliberately a single predicate consulted at every branch, rather than
// a flag read in several places. One mission-creation owner per fulfilment is
// the rule; two places deciding who owns it is how both would.
func (s *Service) JITPlanningEnabled() bool {
	return s != nil && s.logisticsPlanner != nil
}

// plannedMissionFor projects one durable slot onto what the planner needs.
func plannedMissionFor(order ParentOrder, plan MissionPlan, mission OrderMission,
	execution MissionExecution) PlannedMission {
	return PlannedMission{
		MissionRef:     mission.MissionRef,
		PlanMissionRef: plan.PlanMissionRef,
		ParentOrderID:  order.ID,
		CustomerID:     order.CustomerID,
		VendorOrderIDs: plan.VendorOrderIDs,
		Service:        plan.Service,
		VehicleClass:   plan.VehicleClass,
		Execution:      execution,
		Delivery:       order.Address,
		WeightGrams:    plan.WeightGrams,
		PackageCount:   plan.PackageCount,
	}
}

// CommitOutcome is what the orchestration learns from one commit attempt.
//
// Definite is the field that matters. An indefinite outcome means the create
// may or may not have landed, and the only safe next step is to ask the
// authority -- never to send it again.
type CommitOutcome struct {
	Definite    bool
	Accepted    bool
	MissionRef  string
	ExternalRef string
	RefusalCode string
}

// CommitPlannedMission dispatches a slot the orchestration has decided is due.
//
// This is the SAME assembly and the SAME authority call the legacy path makes.
// The request is rebuilt here from durable state rather than carried through the
// orchestration, so a mission dispatched just-in-time is byte-for-byte the
// mission that would have been dispatched synchronously. Anything else would
// make the two paths diverge in exactly the place -- approved fee and plan
// binding -- where divergence is a financial defect.
//
// authority and productCode are the orchestration's decision. They override the
// frozen plan's own choice, because by commit time the orchestration has asked
// every authority and this is the answer that came back.
func (s *Service) CommitPlannedMission(
	ctx context.Context, missionRef string, authority LogisticsService, productCode string,
) (CommitOutcome, error) {
	if !s.enabled {
		return CommitOutcome{}, ErrDisabled
	}
	if !s.FulfilmentEnabled() {
		return CommitOutcome{}, ErrDependencyMissing
	}
	order, plan, mission, err := s.resolvePlannedMission(ctx, missionRef)
	if err != nil {
		return CommitOutcome{}, err
	}
	if mission.Status != MissionPending {
		// Already answered. Asking again would risk a second vehicle for goods
		// an authority already holds.
		return CommitOutcome{
			Definite: true, Accepted: mission.Status == MissionAccepted,
			MissionRef: mission.MissionRef, ExternalRef: mission.ExternalRef,
		}, nil
	}
	if strings.TrimSpace(string(authority)) != "" {
		plan.Service = authority
	}
	if strings.TrimSpace(productCode) != "" {
		plan.VehicleClass = productCode
	}
	if !s.missions.Available(plan.Service) {
		// An authority this deployment cannot call has said nothing. Leaving
		// the slot PENDING keeps it reconcilable instead of recording a
		// refusal it never made.
		return CommitOutcome{}, fmt.Errorf("%w: %s is not available", ErrDependencyMissing, plan.Service)
	}

	request, err := s.missionRequestFor(ctx, order, plan, mission)
	if err != nil {
		return CommitOutcome{}, err
	}

	result, missionErr := s.missions.CreateMission(ctx, request)
	if missionErr != nil {
		if classifyCommitFailure(missionErr) == commitAmbiguous {
			// The authority may have created it. The caller must read back,
			// and the slot stays PENDING so the read-back has something to
			// resolve against.
			return CommitOutcome{Definite: false, MissionRef: mission.MissionRef}, nil
		}
		if recordErr := s.store.RecordMissionFailure(
			ctx, mission.ID, redactAuthorityError(missionErr), s.now().UTC(),
		); recordErr != nil {
			return CommitOutcome{}, recordErr
		}
		return CommitOutcome{
			Definite: true, Accepted: false, MissionRef: mission.MissionRef,
			RefusalCode: redactAuthorityError(missionErr),
		}, nil
	}

	if err = s.recordCommitResult(ctx, order, plan, mission, result); err != nil {
		return CommitOutcome{}, err
	}
	return CommitOutcome{
		Definite: true, Accepted: result.Status == MissionAccepted,
		MissionRef: mission.MissionRef, ExternalRef: result.ExternalRef,
		RefusalCode: result.RefusalCode,
	}, nil
}

// ReadPlannedMission asks the authority whether a slot's mission exists.
//
// found=false must mean "definitely not there". An authority that could not be
// reached returns an error instead, because the difference between "absent" and
// "unknown" is the difference between a safe retry and a duplicate dispatch.
func (s *Service) ReadPlannedMission(
	ctx context.Context, missionRef string,
) (CommitOutcome, bool, error) {
	if !s.enabled {
		return CommitOutcome{}, false, ErrDisabled
	}
	order, plan, mission, err := s.resolvePlannedMission(ctx, missionRef)
	if err != nil {
		return CommitOutcome{}, false, err
	}
	if mission.Status != MissionPending {
		// This service already recorded the authority's answer. That IS the
		// read-back; going back out to the authority would be asking a
		// question already answered durably.
		return CommitOutcome{
			Definite: true, Accepted: mission.Status == MissionAccepted,
			MissionRef: mission.MissionRef, ExternalRef: mission.ExternalRef,
		}, true, nil
	}
	reader, ok := s.missions.(MissionReadAuthority)
	if !ok {
		return CommitOutcome{}, false, fmt.Errorf(
			"%w: this deployment cannot read a mission back", ErrDependencyMissing)
	}
	result, found, err := reader.ReadMission(ctx, plan.Service, mission.MissionRef)
	if err != nil {
		return CommitOutcome{}, false, err
	}
	if !found {
		return CommitOutcome{}, false, nil
	}
	if err = s.recordCommitResult(ctx, order, plan, mission, result); err != nil {
		return CommitOutcome{}, false, err
	}
	return CommitOutcome{
		Definite: true, Accepted: result.Status == MissionAccepted,
		MissionRef: mission.MissionRef, ExternalRef: result.ExternalRef,
		RefusalCode: result.RefusalCode,
	}, true, nil
}

// MissionReadAuthority is the authoritative read-back.
//
// It is an optional extension of MissionAuthority so a deployment or test
// double without it remains source compatible -- but just-in-time commit
// refuses to run without it, because committing without a way to ask what
// happened is precisely what turns one timeout into two vehicles.
type MissionReadAuthority interface {
	ReadMission(ctx context.Context, service LogisticsService, missionRef string) (
		result MissionResult, found bool, err error)
}

// recordCommitResult persists an authority's answer and projects it.
//
// It is shared by the create path and the read-back path on purpose: a mission
// adopted after a timeout must land in exactly the state it would have landed
// in had the create simply returned.
func (s *Service) recordCommitResult(ctx context.Context, order ParentOrder,
	plan MissionPlan, mission OrderMission, result MissionResult) error {
	if err := s.store.RecordMissionResult(ctx, mission.ID, result, s.now().UTC()); err != nil {
		return err
	}
	if s.fulfilmentProjection == nil {
		return nil
	}
	return s.fulfilmentProjection.LinkMission(ctx, order, plan, result)
}

// missionRequestFor rebuilds the authority request for one durable slot.
func (s *Service) missionRequestFor(ctx context.Context, order ParentOrder,
	plan MissionPlan, mission OrderMission) (MissionRequest, error) {
	var execution *MissionExecution
	if projection, ok := s.fulfilmentProjection.(MissionExecutionProjection); ok {
		snapshot, err := projection.MissionExecution(ctx, order, plan)
		if err != nil {
			return MissionRequest{}, err
		}
		execution = &snapshot
	}
	return MissionRequest{
		MissionRef:       mission.MissionRef,
		ParentOrderID:    order.ID,
		VendorOrderIDs:   plan.VendorOrderIDs,
		CustomerID:       order.CustomerID,
		Service:          plan.Service,
		VehicleClass:     plan.VehicleClass,
		Currency:         order.Currency,
		AgreedFeeMinor:   plan.AgreedFeeMinor,
		FundingReference: order.QuoteID,
		PlanMissionRef:   plan.PlanMissionRef,
		PickupBusiness:   plan.PickupBusiness,
		PickupBranch:     plan.PickupBranch,
		DeliveryAddress:  order.Address,
		Receiver:         order.Receiver,
		WeightGrams:      plan.WeightGrams,
		PackageCount:     plan.PackageCount,
		Execution:        execution,
	}, nil
}

// resolvePlannedMission loads a slot and the plan that produced it.
//
// The plan is re-derived from the order rather than stored on the orchestration
// so the committed mission reflects the order as it stands now, and so there is
// no second copy of the plan to drift from this one.
func (s *Service) resolvePlannedMission(ctx context.Context, missionRef string) (
	ParentOrder, MissionPlan, OrderMission, error) {
	reference := strings.TrimSpace(missionRef)
	if reference == "" {
		return ParentOrder{}, MissionPlan{}, OrderMission{}, ErrMissionNotPlanned
	}
	mission, err := s.store.GetMissionByRef(ctx, reference)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			return ParentOrder{}, MissionPlan{}, OrderMission{}, ErrMissionNotPlanned
		}
		return ParentOrder{}, MissionPlan{}, OrderMission{}, err
	}
	order, err := s.store.GetParentByID(ctx, mission.ParentOrderID)
	if err != nil {
		return ParentOrder{}, MissionPlan{}, OrderMission{}, err
	}
	plans, err := PlanMissions(order)
	if err != nil {
		return ParentOrder{}, MissionPlan{}, OrderMission{}, err
	}
	for index := range plans {
		if plans[index].PlanMissionRef != mission.PlanMissionRef {
			continue
		}
		plan := plans[index]
		if resolver, ok := s.fulfilmentProjection.(MissionPlanResolver); ok {
			plan, err = resolver.ResolveMissionPlan(ctx, order, plan)
			if err != nil {
				return ParentOrder{}, MissionPlan{}, OrderMission{}, err
			}
		}
		return order, plan, mission, nil
	}
	// The slot exists but its allocations no longer plan a platform delivery --
	// every vendor order in it was cancelled or rejected. Committing would
	// dispatch for goods nobody is sending.
	return ParentOrder{}, MissionPlan{}, OrderMission{}, ErrMissionNotPlanned
}

// commitFailureClass separates a refusal from an unanswered question.
type commitFailureClass int

const (
	commitRefused commitFailureClass = iota
	commitAmbiguous
)

// classifyCommitFailure decides whether a failed create may have landed.
//
// The default is AMBIGUOUS, and that asymmetry is deliberate. Treating an
// unrecognised failure as a refusal would let the slot be retried into a second
// vehicle; treating it as ambiguous costs one read-back. The cheap mistake is
// the one to make.
func classifyCommitFailure(err error) commitFailureClass {
	if err == nil {
		return commitRefused
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return commitAmbiguous
	}
	lowered := strings.ToLower(err.Error())
	// A refusal is something the authority said. Anything that reads like the
	// question never arrived, or the answer never came back, is ambiguous.
	for _, refusal := range []string{
		"refused", "rejected", "not serviceable", "unserviceable",
		"capability", "validation", "invalid", "forbidden", "unauthorized",
	} {
		if strings.Contains(lowered, refusal) {
			return commitRefused
		}
	}
	return commitAmbiguous
}

// PlanningDeadlineFor reports the promised delivery time for a slot.
//
// It exists so the composition root can build the orchestration requirement
// without reaching into the execution snapshot itself.
func PlanningDeadlineFor(execution MissionExecution) time.Time {
	return execution.PromisedBy
}
