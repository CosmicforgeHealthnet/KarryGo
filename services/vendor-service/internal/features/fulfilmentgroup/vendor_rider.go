package fulfilmentgroup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

const PermRiderExecute = "vendor.delivery.execute_assigned"

type RiderAvailabilityState string

const (
	RiderOffline   RiderAvailabilityState = "OFFLINE"
	RiderAvailable RiderAvailabilityState = "AVAILABLE"
	RiderBusy      RiderAvailabilityState = "BUSY"
	RiderSuspended RiderAvailabilityState = "SUSPENDED"
)

func (s RiderAvailabilityState) ValidRequest() bool { return s == RiderOffline || s == RiderAvailable }

type RiderAvailability struct {
	StaffID                string                 `json:"staff_id"`
	BusinessID             string                 `json:"business_id"`
	BranchID               string                 `json:"branch_id"`
	CurrentVendorMissionID string                 `json:"current_vendor_mission_id,omitempty"`
	State                  RiderAvailabilityState `json:"state"`
	Version                int64                  `json:"version"`
	UpdatedAt              time.Time              `json:"updated_at"`
}

type VendorMissionStage string

const (
	StagePlanned             VendorMissionStage = "PLANNED"
	StageAssigned            VendorMissionStage = "ASSIGNED"
	StageAcknowledged        VendorMissionStage = "ACKNOWLEDGED"
	StageEnRoutePickup       VendorMissionStage = "EN_ROUTE_PICKUP"
	StageArrivedPickup       VendorMissionStage = "ARRIVED_PICKUP"
	StageInCustody           VendorMissionStage = "IN_CUSTODY"
	StageEnRouteReceiver     VendorMissionStage = "EN_ROUTE_RECEIVER"
	StageArrivedReceiver     VendorMissionStage = "ARRIVED_RECEIVER"
	StageCompleted           VendorMissionStage = "COMPLETED"
	StageReceiverUnavailable VendorMissionStage = "RECEIVER_UNAVAILABLE"
	StageReturning           VendorMissionStage = "RETURNING"
	StageReturned            VendorMissionStage = "RETURNED"
	StageFailed              VendorMissionStage = "FAILED"
)

type VendorMissionRuntime struct {
	Mission                   VendorMission      `json:"mission"`
	Stage                     VendorMissionStage `json:"stage"`
	RuntimeVersion            int64              `json:"runtime_version"`
	PickupArrivedAt           *time.Time         `json:"pickup_arrived_at,omitempty"`
	CustodyVerifiedAt         *time.Time         `json:"custody_verified_at,omitempty"`
	ReceiverArrivedAt         *time.Time         `json:"receiver_arrived_at,omitempty"`
	ReceiverUnavailableReason string             `json:"receiver_unavailable_reason,omitempty"`
}

type RiderAction string

const (
	ActionAcknowledge         RiderAction = "ACKNOWLEDGE"
	ActionStartPickup         RiderAction = "START_PICKUP"
	ActionArrivePickup        RiderAction = "ARRIVE_PICKUP"
	ActionCompletePickup      RiderAction = "COMPLETE_PICKUP"
	ActionStartReceiver       RiderAction = "START_RECEIVER"
	ActionArriveReceiver      RiderAction = "ARRIVE_RECEIVER"
	ActionCompleteDelivery    RiderAction = "COMPLETE_DELIVERY"
	ActionReceiverUnavailable RiderAction = "RECEIVER_UNAVAILABLE"
	ActionReturn              RiderAction = "RETURN"
)

type RiderActionRequest struct {
	Action          RiderAction `json:"action"`
	OperationRef    string      `json:"operation_ref"`
	ExpectedVersion int64       `json:"expected_version"`
	EvidenceRef     string      `json:"evidence_ref,omitempty"`
	ProofRef        string      `json:"proof_ref,omitempty"`
	ReasonCode      string      `json:"reason_code,omitempty"`
	ParcelRefs      []string    `json:"parcel_refs,omitempty"`
	verifiedRef     string
	verifiedProof   string
}

type HandoverVerification struct {
	OpaqueRef, EvidenceRef string
	VerifiedAt             time.Time
}

type VendorHandoverRequest struct {
	MissionID, BusinessID, BranchID, RiderStaffID, OperationRef string
	ParcelRefs                                                  []string
	EvidenceRef                                                 string
}

type VendorMissionOrderAuthority interface {
	ProjectVendorMissionAction(context.Context, string, string, []string, string) error
}

// VendorHandoverAuthority is intentionally opaque. Implementations own
// challenges, hashes and replay protection; Vendor receives no OTP/QR secret.
type VendorHandoverAuthority interface {
	VerifyPickup(context.Context, VendorHandoverRequest) (HandoverVerification, error)
	VerifyReceiver(context.Context, VendorHandoverRequest) (HandoverVerification, error)
	VerifyReturn(context.Context, VendorHandoverRequest) (HandoverVerification, error)
}

func secureRiderAction(action RiderAction) bool {
	return action == ActionCompletePickup || action == ActionCompleteDelivery || action == ActionReturn
}

func actionFingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Service) SetRiderAvailability(ctx context.Context, actor production.Actor, branchID string, state RiderAvailabilityState, expectedVersion int64) (RiderAvailability, error) {
	if err := s.guard(); err != nil {
		return RiderAvailability{}, err
	}
	if actor.StaffID == "" || actor.CommandRef == "" || !state.ValidRequest() {
		return RiderAvailability{}, ErrInvalid
	}
	if err := s.authorizeBranch(ctx, actor, PermRiderExecute, branchID); err != nil {
		return RiderAvailability{}, err
	}
	return s.store.SetRiderAvailability(ctx, actor.VendorID, actor.StaffID, branchID, state, expectedVersion, actor.CommandRef, s.now().UTC())
}

func (s *Service) RiderAvailability(ctx context.Context, actor production.Actor) (RiderAvailability, error) {
	if err := s.guard(); err != nil {
		return RiderAvailability{}, err
	}
	return s.store.GetRiderAvailability(ctx, actor.VendorID, actor.StaffID)
}

func (s *Service) CurrentRiderMission(ctx context.Context, actor production.Actor) (VendorMissionRuntime, error) {
	if err := s.guard(); err != nil {
		return VendorMissionRuntime{}, err
	}
	availability, err := s.store.GetRiderAvailability(ctx, actor.VendorID, actor.StaffID)
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	if strings.TrimSpace(availability.CurrentVendorMissionID) == "" {
		return VendorMissionRuntime{}, ErrNotFound
	}
	return s.riderMission(ctx, actor, availability.CurrentVendorMissionID)
}

func (s *Service) RiderMission(ctx context.Context, actor production.Actor, missionID string) (VendorMissionRuntime, error) {
	if err := s.guard(); err != nil {
		return VendorMissionRuntime{}, err
	}
	return s.riderMission(ctx, actor, missionID)
}

func (s *Service) ListRiderMissions(ctx context.Context, actor production.Actor, limit int) ([]VendorMissionRuntime, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	return s.store.ListRiderMissionRuntime(ctx, actor.VendorID, actor.StaffID, limit)
}

func (s *Service) riderMission(ctx context.Context, actor production.Actor, missionID string) (VendorMissionRuntime, error) {
	runtime, err := s.store.GetVendorMissionRuntime(ctx, actor.VendorID, missionID)
	if err != nil {
		return runtime, err
	}
	if runtime.Mission.AssignedStaffID != actor.StaffID {
		return VendorMissionRuntime{}, ErrNotFound
	}
	if err = s.authorizeBranch(ctx, actor, PermRiderExecute, runtime.Mission.BranchID); err != nil {
		return VendorMissionRuntime{}, err
	}
	return runtime, nil
}

func (s *Service) ExecuteRiderAction(ctx context.Context, actor production.Actor, missionID string, request RiderActionRequest) (VendorMissionRuntime, error) {
	if err := s.guard(); err != nil {
		return VendorMissionRuntime{}, err
	}
	if actor.CommandRef == "" || request.OperationRef == "" || actor.CommandRef != request.OperationRef {
		return VendorMissionRuntime{}, ErrInvalid
	}
	if request.ProofRef != "" || (!secureRiderAction(request.Action) && request.EvidenceRef != "") {
		// Proof is authority output, never client input. EvidenceRef is accepted
		// only as an opaque challenge/reference for a secure handover action.
		return VendorMissionRuntime{}, ErrInvalid
	}
	runtime, err := s.riderMission(ctx, actor, missionID)
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	if replay, found, replayErr := s.store.GetRiderActionResult(ctx, actor.VendorID, actor.StaffID, missionID, request); replayErr != nil {
		return VendorMissionRuntime{}, replayErr
	} else if found {
		return s.projectRiderAction(ctx, replay, request.Action)
	}
	var verified HandoverVerification
	if secureRiderAction(request.Action) {
		if s.handover == nil {
			return VendorMissionRuntime{}, ErrHandoverUnavailable
		}
		handoverRequest := VendorHandoverRequest{MissionID: missionID, BusinessID: runtime.Mission.BusinessID, BranchID: runtime.Mission.BranchID, RiderStaffID: actor.StaffID, OperationRef: request.OperationRef, ParcelRefs: request.ParcelRefs, EvidenceRef: request.EvidenceRef}
		switch request.Action {
		case ActionCompletePickup:
			verified, err = s.handover.VerifyPickup(ctx, handoverRequest)
		case ActionCompleteDelivery:
			verified, err = s.handover.VerifyReceiver(ctx, handoverRequest)
		case ActionReturn:
			verified, err = s.handover.VerifyReturn(ctx, handoverRequest)
		}
		if err != nil || verified.OpaqueRef == "" {
			if err != nil {
				return VendorMissionRuntime{}, err
			}
			return VendorMissionRuntime{}, ErrHandoverUnavailable
		}
		request.verifiedRef = verified.OpaqueRef
		request.verifiedProof = verified.EvidenceRef
	}
	result, err := s.store.ApplyRiderAction(ctx, actor.VendorID, actor.StaffID, missionID, request, s.now().UTC())
	if err != nil {
		return VendorMissionRuntime{}, err
	}
	return s.projectRiderAction(ctx, result, request.Action)
}

func (s *Service) projectRiderAction(ctx context.Context, runtime VendorMissionRuntime, action RiderAction) (VendorMissionRuntime, error) {
	if s.orders == nil || (action != ActionCompletePickup && action != ActionStartReceiver && action != ActionCompleteDelivery) {
		return runtime, nil
	}
	vendorOrders, err := s.store.VendorOrdersEligibleForRiderProjection(ctx, runtime.Mission.PlanMissionID, action)
	if err != nil {
		return runtime, err
	}
	if len(vendorOrders) == 0 {
		return runtime, nil
	}
	if err = s.orders.ProjectVendorMissionAction(ctx, runtime.Mission.BusinessID, runtime.Mission.AssignedStaffID, vendorOrders, string(action)); err != nil {
		return runtime, err
	}
	return runtime, nil
}
