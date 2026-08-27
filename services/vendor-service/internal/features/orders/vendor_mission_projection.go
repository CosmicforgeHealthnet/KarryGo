package orders

import (
	"context"
	"strings"
)

// ProjectVendorMissionAction advances the existing VendorOrder lifecycle from
// canonical VendorMission evidence. It is retry-safe: already-projected steps
// are read back, while a genuinely out-of-order commercial state fails closed.
func (s *Service) ProjectVendorMissionAction(ctx context.Context, businessID, staffID string, vendorOrderIDs []string, action string) error {
	if s == nil || !s.enabled || s.store == nil {
		return ErrDependencyMissing
	}
	var transition Transition
	switch action {
	case "COMPLETE_PICKUP":
		transition = TransitionPickup
	case "START_RECEIVER":
		transition = TransitionDelivering
	case "COMPLETE_DELIVERY":
		transition = TransitionDelivered
	default:
		return nil
	}
	for _, vendorOrderID := range vendorOrderIDs {
		current, err := s.store.GetVendorOrderForVendor(ctx, businessID, strings.TrimSpace(vendorOrderID))
		if err != nil {
			return err
		}
		if vendorMissionActionProjected(current, transition) {
			continue
		}
		moved, parent, moveErr := s.store.Transition(ctx, TransitionRequest{VendorOrderID: current.ID, ExpectedVersion: current.Version, Transition: transition, ActorType: "VENDOR_RIDER", ActorRef: staffID, BusinessID: businessID}, s.now().UTC())
		if moveErr != nil {
			return moveErr
		}
		if _, _, moveErr = s.afterTransition(ctx, moved, parent, current.Status); moveErr != nil {
			return moveErr
		}
	}
	return nil
}

func vendorMissionActionProjected(order VendorOrder, transition Transition) bool {
	switch transition {
	case TransitionPickup:
		return order.Status == VendorStatusPickedUp || order.Status == VendorStatusDelivering || order.Status == VendorStatusDelivered
	case TransitionDelivering:
		return order.Status == VendorStatusDelivering || order.Status == VendorStatusDelivered
	case TransitionDelivered:
		return order.Status == VendorStatusDelivered
	default:
		return false
	}
}
