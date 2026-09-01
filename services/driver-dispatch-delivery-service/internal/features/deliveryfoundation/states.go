package deliveryfoundation

import "fmt"

type GroupState string

const (
	GroupDraft              GroupState = "DRAFT"
	GroupServiceability     GroupState = "SERVICEABILITY_CHECK"
	GroupQuoted             GroupState = "QUOTED"
	GroupPaymentPending     GroupState = "PAYMENT_PENDING"
	GroupReadyForDispatch   GroupState = "READY_FOR_DISPATCH"
	GroupActive             GroupState = "ACTIVE"
	GroupPartiallyDelivered GroupState = "PARTIALLY_DELIVERED"
	GroupDelivered          GroupState = "DELIVERED"
	GroupSettlementPending  GroupState = "SETTLEMENT_PENDING"
	GroupCompleted          GroupState = "COMPLETED"
	GroupCancelled          GroupState = "CANCELLED"
	GroupFailed             GroupState = "FAILED"
	GroupReturning          GroupState = "RETURNING"
	GroupReturned           GroupState = "RETURNED"
	GroupUnderReview        GroupState = "UNDER_REVIEW"
)

var groupTransitions = map[GroupState]map[GroupState]bool{
	GroupDraft:              set(GroupServiceability, GroupCancelled),
	GroupServiceability:     set(GroupQuoted, GroupFailed, GroupUnderReview, GroupCancelled),
	GroupQuoted:             set(GroupPaymentPending, GroupCancelled, GroupServiceability),
	GroupPaymentPending:     set(GroupReadyForDispatch, GroupFailed, GroupCancelled),
	GroupReadyForDispatch:   set(GroupActive, GroupCancelled, GroupFailed, GroupUnderReview),
	GroupActive:             set(GroupPartiallyDelivered, GroupDelivered, GroupFailed, GroupReturning, GroupUnderReview),
	GroupPartiallyDelivered: set(GroupDelivered, GroupFailed, GroupReturning, GroupUnderReview),
	GroupDelivered:          set(GroupSettlementPending, GroupUnderReview),
	GroupSettlementPending:  set(GroupCompleted, GroupUnderReview),
	GroupReturning:          set(GroupReturned, GroupFailed, GroupUnderReview),
	GroupUnderReview:        set(GroupActive, GroupReturning, GroupFailed, GroupCancelled),
}

func (s GroupState) Valid() bool { _, ok := groupTransitions[s]; return ok || groupTerminal(s) }
func (s GroupState) CanTransition(to GroupState) bool {
	return s.Valid() && to.Valid() && groupTransitions[s][to]
}

func groupTerminal(s GroupState) bool {
	switch s {
	case GroupCompleted, GroupCancelled, GroupFailed, GroupReturned:
		return true
	default:
		return false
	}
}

// ValidateCompletion preserves the aggregate invariant: settlement acknowledgement
// and all required execution outcomes are prerequisites for COMPLETED.
func ValidateCompletion(from GroupState, allRequiredOutcomesTerminal, financialCompletionKnown bool) error {
	if from != GroupSettlementPending || !allRequiredOutcomesTerminal || !financialCompletionKnown {
		return fmt.Errorf("delivery group completion prerequisites are not satisfied")
	}
	return nil
}

type MissionState string

const (
	MissionPlanned              MissionState = "PLANNED"
	MissionQueued               MissionState = "QUEUED"
	MissionOffering             MissionState = "OFFERING"
	MissionAssigned             MissionState = "ASSIGNED"
	MissionEnRoutePickup        MissionState = "EN_ROUTE_PICKUP"
	MissionAtPickup             MissionState = "AT_PICKUP"
	MissionPickupVerification   MissionState = "PICKUP_VERIFICATION"
	MissionPickedUp             MissionState = "PICKED_UP"
	MissionInTransit            MissionState = "IN_TRANSIT"
	MissionAtDelivery           MissionState = "AT_DELIVERY"
	MissionDeliveryVerification MissionState = "DELIVERY_VERIFICATION"
	MissionDelivered            MissionState = "DELIVERED"
	MissionCompleted            MissionState = "COMPLETED"
	MissionFailed               MissionState = "FAILED"
	MissionReassigning          MissionState = "REASSIGNING"
	MissionReturning            MissionState = "RETURNING"
	MissionReturned             MissionState = "RETURNED"
	MissionHubHold              MissionState = "HUB_HOLD"
	MissionCancelled            MissionState = "CANCELLED"
)

var missionTransitions = map[MissionState]map[MissionState]bool{
	MissionPlanned:              missionSet(MissionQueued, MissionCancelled, MissionFailed),
	MissionQueued:               missionSet(MissionOffering, MissionCancelled, MissionFailed),
	MissionOffering:             missionSet(MissionAssigned, MissionQueued, MissionCancelled, MissionFailed),
	MissionAssigned:             missionSet(MissionEnRoutePickup, MissionReassigning, MissionCancelled, MissionFailed),
	MissionEnRoutePickup:        missionSet(MissionAtPickup, MissionReassigning, MissionCancelled, MissionFailed),
	MissionAtPickup:             missionSet(MissionPickupVerification, MissionReassigning, MissionCancelled, MissionFailed),
	MissionPickupVerification:   missionSet(MissionPickedUp, MissionReassigning, MissionCancelled, MissionFailed),
	MissionPickedUp:             missionSet(MissionInTransit, MissionReturning, MissionReassigning, MissionFailed),
	MissionInTransit:            missionSet(MissionAtDelivery, MissionReturning, MissionReassigning, MissionHubHold, MissionFailed),
	MissionAtDelivery:           missionSet(MissionDeliveryVerification, MissionReturning, MissionFailed),
	MissionDeliveryVerification: missionSet(MissionDelivered, MissionReturning, MissionFailed),
	MissionDelivered:            missionSet(MissionCompleted),
	MissionReassigning:          missionSet(MissionAssigned, MissionReturning, MissionFailed),
	MissionReturning:            missionSet(MissionReturned, MissionHubHold, MissionFailed),
	MissionHubHold:              missionSet(MissionInTransit, MissionReturning, MissionFailed),
}

func (s MissionState) Valid() bool { _, ok := missionTransitions[s]; return ok || missionTerminal(s) }
func (s MissionState) CanTransition(to MissionState) bool {
	return s.Valid() && to.Valid() && missionTransitions[s][to]
}
func missionTerminal(s MissionState) bool {
	switch s {
	case MissionCompleted, MissionFailed, MissionReturned, MissionCancelled:
		return true
	default:
		return false
	}
}

type ParcelState string

const (
	ParcelDeclared         ParcelState = "DECLARED"
	ParcelReadyForPickup   ParcelState = "READY_FOR_PICKUP"
	ParcelPickupVerified   ParcelState = "PICKUP_VERIFIED"
	ParcelInCustody        ParcelState = "IN_CUSTODY"
	ParcelInTransit        ParcelState = "IN_TRANSIT"
	ParcelDeliveryVerified ParcelState = "DELIVERY_VERIFIED"
	ParcelDelivered        ParcelState = "DELIVERED"
	ParcelDiscrepancy      ParcelState = "DISCREPANCY"
	ParcelRefused          ParcelState = "REFUSED"
	ParcelDamaged          ParcelState = "DAMAGED"
	ParcelMissing          ParcelState = "MISSING"
	ParcelReturning        ParcelState = "RETURNING"
	ParcelReturned         ParcelState = "RETURNED"
	ParcelHubHold          ParcelState = "HUB_HOLD"
	ParcelCancelled        ParcelState = "CANCELLED"
)

var parcelTransitions = map[ParcelState]map[ParcelState]bool{
	ParcelDeclared:         parcelSet(ParcelReadyForPickup, ParcelDiscrepancy, ParcelRefused, ParcelCancelled),
	ParcelReadyForPickup:   parcelSet(ParcelPickupVerified, ParcelDiscrepancy, ParcelRefused, ParcelCancelled),
	ParcelPickupVerified:   parcelSet(ParcelInCustody, ParcelDiscrepancy, ParcelDamaged, ParcelMissing),
	ParcelInCustody:        parcelSet(ParcelInTransit, ParcelDamaged, ParcelMissing, ParcelReturning, ParcelHubHold),
	ParcelInTransit:        parcelSet(ParcelDeliveryVerified, ParcelDamaged, ParcelMissing, ParcelReturning, ParcelHubHold),
	ParcelDeliveryVerified: parcelSet(ParcelDelivered, ParcelDiscrepancy, ParcelReturning),
	ParcelDiscrepancy:      parcelSet(ParcelReadyForPickup, ParcelRefused, ParcelReturning, ParcelCancelled),
	ParcelDamaged:          parcelSet(ParcelReturning, ParcelRefused),
	ParcelMissing:          parcelSet(ParcelInCustody, ParcelReturning),
	ParcelReturning:        parcelSet(ParcelReturned, ParcelHubHold),
	ParcelHubHold:          parcelSet(ParcelInTransit, ParcelReturning),
}

func (s ParcelState) Valid() bool { _, ok := parcelTransitions[s]; return ok || parcelTerminal(s) }
func (s ParcelState) CanTransition(to ParcelState) bool {
	return s.Valid() && to.Valid() && parcelTransitions[s][to]
}
func parcelTerminal(s ParcelState) bool {
	switch s {
	case ParcelDelivered, ParcelRefused, ParcelReturned, ParcelCancelled:
		return true
	default:
		return false
	}
}

type HandoverChallengeState string

const (
	ChallengeActive      HandoverChallengeState = "ACTIVE"
	ChallengeConsumed    HandoverChallengeState = "CONSUMED"
	ChallengeInvalidated HandoverChallengeState = "INVALIDATED"
	ChallengeExpired     HandoverChallengeState = "EXPIRED"
	ChallengeLocked      HandoverChallengeState = "LOCKED"
)

func (s HandoverChallengeState) Valid() bool {
	switch s {
	case ChallengeActive, ChallengeConsumed, ChallengeInvalidated, ChallengeExpired, ChallengeLocked:
		return true
	default:
		return false
	}
}
func (s HandoverChallengeState) CanTransition(to HandoverChallengeState) bool {
	return s == ChallengeActive && to.Valid() && to != ChallengeActive
}

func set(values ...GroupState) map[GroupState]bool {
	result := make(map[GroupState]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
func missionSet(values ...MissionState) map[MissionState]bool {
	result := make(map[MissionState]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
func parcelSet(values ...ParcelState) map[ParcelState]bool {
	result := make(map[ParcelState]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
