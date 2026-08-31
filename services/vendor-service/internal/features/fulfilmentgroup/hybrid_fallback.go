package fulfilmentgroup

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

type HybridFallbackState string

const (
	FallbackStarted             HybridFallbackState = "STARTED"
	FallbackPlanActivated       HybridFallbackState = "PLAN_ACTIVATED"
	FallbackFundingTransitioned HybridFallbackState = "FUNDING_TRANSITIONED"
	FallbackMissionLinked       HybridFallbackState = "MISSION_LINKED"
	FallbackCompleted           HybridFallbackState = "COMPLETED"
	FallbackFailed              HybridFallbackState = "FAILED"
)

type HybridFallback struct {
	ID, OperationRef, BusinessID, BranchID, VendorMissionID, GroupID string
	SourcePlanID                                                     string
	SourcePlanVersion                                                int64
	SourcePlanMissionID                                              string
	TargetPlanID                                                     string
	TargetPlanVersion                                                int64
	TargetPlanMissionID                                              string
	PlatformMissionRefID                                             string
	WalletTransitionRef                                              string
	ReasonCode                                                       PlanReason
	State                                                            HybridFallbackState
	FailureReason                                                    string
	Version                                                          int64
	CreatedAt, UpdatedAt                                             time.Time
	CompletedAt                                                      *time.Time
	RequestFingerprint                                               string
}

type HybridFallbackRequest struct {
	OperationRef    string     `json:"-"`
	ExpectedVersion int64      `json:"expected_version"`
	ReasonCode      PlanReason `json:"reason_code"`
}

type FallbackFundingRequest struct {
	OperationRef, VendorOrderID, ReasonCode string
	PlatformExecutionCostMinor              int64
}
type FallbackMissionRequest struct {
	OperationRef, GroupID string
	Plan                  Plan
	Mission               PlanMission
	VendorOrderIDs        []string
	FundingReferences     []string
}
type FallbackMissionResult struct{ PlatformMissionRefID, ExternalRef string }

// HybridFallbackAuthority is a narrow adapter onto the accepted Wallet and
// Dispatch authorities. It owns no ledger and no logistics state.
type HybridFallbackAuthority interface {
	QuotePlatformMission(context.Context, FallbackMissionRequest) (int64, error)
	TransitionFunding(context.Context, FallbackFundingRequest) (string, error)
	CreatePlatformMission(context.Context, FallbackMissionRequest) (FallbackMissionResult, error)
}

func (s *Service) TriggerHybridFallback(ctx context.Context, actor production.Actor, missionID string, request HybridFallbackRequest) (HybridFallback, error) {
	if err := s.guard(); err != nil {
		return HybridFallback{}, err
	}
	if s.fallback == nil {
		return HybridFallback{}, ErrDependencyMissing
	}
	if request.OperationRef == "" || actor.CommandRef != request.OperationRef || request.ExpectedVersion < 1 || !request.ReasonCode.Replan() {
		return HybridFallback{}, ErrInvalid
	}
	mission, err := s.store.GetVendorMission(ctx, actor.VendorID, missionID)
	if err != nil {
		return HybridFallback{}, err
	}
	if err = s.authorizeBranch(ctx, actor, PermMissionManage, mission.BranchID); err != nil {
		return HybridFallback{}, err
	}
	fallback, err := s.store.EnsureHybridFallback(ctx, actor.VendorID, actor.StaffID, missionID, request, s.now().UTC())
	if err != nil {
		return HybridFallback{}, err
	}
	if fallback.State == FallbackCompleted {
		return fallback, nil
	}

	plan, err := s.store.GetPlan(ctx, fallback.GroupID, fallback.SourcePlanID)
	if err != nil {
		return fallback, err
	}
	if fallback.TargetPlanID == "" {
		missions := append([]PlanMission(nil), plan.Missions...)
		var prospective PlanMission
		for i := range missions {
			if missions[i].ID == fallback.SourcePlanMissionID {
				prospective = missions[i]
				prospective.SlotRef = "vendor-fallback-" + shortSuffix(fallback.ID)
				prospective.Owner = OwnerPlatform
				prospective.Authority = AuthorityDispatch
				missions[i].ID = ""
				missions[i].PlanID = ""
				missions[i].SlotRef = prospective.SlotRef
				missions[i].Owner = prospective.Owner
				missions[i].Authority = prospective.Authority
			}
		}
		if prospective.SlotRef == "" {
			return fallback, ErrNotFound
		}
		vendorOrders, vendorErr := s.store.VendorOrdersForMission(ctx, fallback.SourcePlanMissionID)
		if vendorErr != nil {
			return fallback, vendorErr
		}
		platformCost, quoteErr := s.fallback.QuotePlatformMission(ctx, FallbackMissionRequest{OperationRef: request.OperationRef, GroupID: fallback.GroupID, Plan: plan, Mission: prospective, VendorOrderIDs: vendorOrders})
		if quoteErr != nil {
			return fallback, quoteErr
		}
		if platformCost < 0 {
			return fallback, ErrInvalid
		}
		plannedCost := int64(0)
		for i := range missions {
			if missions[i].SlotRef == prospective.SlotRef {
				missions[i].PlannedCostMinor = platformCost
			}
			if missions[i].PlannedCostMinor > math.MaxInt64-plannedCost {
				return fallback, ErrInvalid
			}
			plannedCost += missions[i].PlannedCostMinor
		}
		next, activateErr := s.store.ActivatePlan(ctx, fallback.GroupID, PlanInput{ExpectedVersion: fallback.SourcePlanVersion, Reason: request.ReasonCode, ReasonDetail: "vendor rider pre-custody fallback", PolicyVersion: plan.PolicyVersion, DeliveryMode: ModeHybrid, PlannedCostMinor: plannedCost, Currency: plan.Currency, CostSnapshot: plan.CostSnapshot, RoutingEvidence: plan.RoutingEvidence, IdempotencyKey: "vendor-fallback-plan-" + request.OperationRef, Missions: missions}, s.now().UTC())
		if activateErr != nil {
			return fallback, activateErr
		}
		var target PlanMission
		for _, candidate := range next.Missions {
			if candidate.SlotRef == "vendor-fallback-"+shortSuffix(fallback.ID) {
				target = candidate
				break
			}
		}
		fallback, err = s.store.MarkFallbackPlan(ctx, fallback.ID, next, target, s.now().UTC())
		if err != nil {
			return fallback, err
		}
		plan = next
	} else {
		plan, err = s.store.GetPlan(ctx, fallback.GroupID, fallback.TargetPlanID)
		if err != nil {
			return fallback, err
		}
	}
	var target PlanMission
	for _, candidate := range plan.Missions {
		if candidate.ID == fallback.TargetPlanMissionID {
			target = candidate
			break
		}
	}
	vendorOrders, err := s.store.VendorOrdersForMission(ctx, target.ID)
	if err != nil {
		return fallback, err
	}
	// A canonical VendorMission belongs to one VendorOrder delivery obligation.
	// Fail closed if corrupt planning evidence attempts to spread one Dispatch
	// mission price across several immutable Wallet allocations; a batch funding
	// authority would be required to apportion that safely.
	if len(vendorOrders) != 1 {
		return fallback, ErrConflict
	}
	var fundingRefs []string
	if fallback.WalletTransitionRef != "" {
		_ = json.Unmarshal([]byte(fallback.WalletTransitionRef), &fundingRefs)
	}
	if len(fundingRefs) == 0 {
		for _, vendorOrderID := range vendorOrders {
			ref, fundErr := s.fallback.TransitionFunding(ctx, FallbackFundingRequest{OperationRef: request.OperationRef + ":" + vendorOrderID, VendorOrderID: vendorOrderID, ReasonCode: string(request.ReasonCode), PlatformExecutionCostMinor: target.PlannedCostMinor})
			if fundErr != nil {
				return fallback, fundErr
			}
			fundingRefs = append(fundingRefs, ref)
		}
		raw, _ := json.Marshal(fundingRefs)
		fallback, err = s.store.MarkFallbackFunding(ctx, fallback.ID, string(raw), s.now().UTC())
		if err != nil {
			return fallback, err
		}
	}
	if fallback.PlatformMissionRefID == "" {
		result, createErr := s.fallback.CreatePlatformMission(ctx, FallbackMissionRequest{OperationRef: request.OperationRef, GroupID: fallback.GroupID, Plan: plan, Mission: target, VendorOrderIDs: vendorOrders, FundingReferences: fundingRefs})
		if createErr != nil {
			return fallback, createErr
		}
		fallback, err = s.store.CompleteHybridFallback(ctx, fallback.ID, result, s.now().UTC())
		if err != nil {
			return fallback, err
		}
	}
	return fallback, nil
}

func (s *Service) GetHybridFallback(ctx context.Context, actor production.Actor, operationRef string) (HybridFallback, error) {
	if err := s.guard(); err != nil {
		return HybridFallback{}, err
	}
	fallback, err := s.store.GetHybridFallback(ctx, strings.TrimSpace(operationRef))
	if err != nil {
		return fallback, err
	}
	if fallback.BusinessID != actor.VendorID {
		return HybridFallback{}, ErrNotFound
	}
	if err = s.authorizeBranch(ctx, actor, PermMissionManage, fallback.BranchID); err != nil {
		return HybridFallback{}, err
	}
	return fallback, nil
}
