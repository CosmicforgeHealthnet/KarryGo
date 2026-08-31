package orders

import (
	"context"
	"strings"
)

// ExecutePlatformFallback reuses the normal order mission, projection and
// logistics authorities for a new immutable plan slot. It creates no second
// checkout, delivery client or order identity.
func (s *Service) ExecutePlatformFallback(ctx context.Context, parentOrderID string, plan MissionPlan, fundingReference string) (MissionResult, error) {
	if s == nil || !s.enabled || s.store == nil || s.missions == nil || s.fulfilmentProjection == nil {
		return MissionResult{}, ErrDependencyMissing
	}
	order, err := s.store.GetParentByID(ctx, strings.TrimSpace(parentOrderID))
	if err != nil {
		return MissionResult{}, err
	}
	mission, _, err := s.store.EnsureMission(ctx, order, plan, s.now().UTC())
	if err != nil {
		return MissionResult{}, err
	}
	if mission.Status == MissionAccepted {
		return MissionResult{ExternalRef: mission.ExternalRef, Status: mission.Status, RefusalCode: mission.RefusalCode, AcceptedFeeMinor: plan.AgreedFeeMinor}, nil
	}
	if mission.Status == MissionRefused {
		return MissionResult{Status: mission.Status, RefusalCode: mission.RefusalCode}, nil
	}
	if !s.missions.Available(plan.Service) {
		return MissionResult{}, ErrDependencyMissing
	}
	var execution *MissionExecution
	if projection, ok := s.fulfilmentProjection.(MissionExecutionProjection); ok {
		snapshot, snapshotErr := projection.MissionExecution(ctx, order, plan)
		if snapshotErr != nil {
			return MissionResult{}, snapshotErr
		}
		execution = &snapshot
	}
	result, callErr := s.missions.CreateMission(ctx, MissionRequest{MissionRef: mission.MissionRef, ParentOrderID: order.ID, VendorOrderIDs: plan.VendorOrderIDs, CustomerID: order.CustomerID, Service: plan.Service, VehicleClass: plan.VehicleClass, Currency: order.Currency, AgreedFeeMinor: plan.AgreedFeeMinor, FundingReference: fundingReference, PlanMissionRef: plan.PlanMissionRef, PickupBusiness: plan.PickupBusiness, PickupBranch: plan.PickupBranch, DeliveryAddress: order.Address, Receiver: order.Receiver, WeightGrams: plan.WeightGrams, PackageCount: plan.PackageCount, Execution: execution})
	if callErr != nil {
		_ = s.store.RecordMissionFailure(ctx, mission.ID, redactAuthorityError(callErr), s.now().UTC())
		return MissionResult{}, callErr
	}
	if err = s.store.RecordMissionResult(ctx, mission.ID, result, s.now().UTC()); err != nil {
		return MissionResult{}, err
	}
	if err = s.fulfilmentProjection.LinkMission(ctx, order, plan, result); err != nil {
		return MissionResult{}, err
	}
	return result, nil
}

func (s *Service) ParentOrderForFulfilment(ctx context.Context, parentOrderID string) (ParentOrder, error) {
	if s == nil || s.store == nil {
		return ParentOrder{}, ErrDependencyMissing
	}
	return s.store.GetParentByID(ctx, strings.TrimSpace(parentOrderID))
}

func (s *Service) QuotePlatformFallback(ctx context.Context, request MissionRequest) (MissionQuote, error) {
	if s == nil || s.missions == nil {
		return MissionQuote{}, ErrDependencyMissing
	}
	authority, ok := s.missions.(MissionQuoteAuthority)
	if !ok {
		return MissionQuote{}, ErrDependencyMissing
	}
	return authority.QuoteMission(ctx, request)
}
