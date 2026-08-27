package delivery

import "errors"

// ExecutionState is the delivery operational lifecycle. It is intentionally
// separate from VendorOrder status and therefore cannot settle or cancel an
// order by itself.
type ExecutionState string

const (
	ExecutionPlanned            ExecutionState = "PLANNED"
	ExecutionAssigned           ExecutionState = "ASSIGNED"
	ExecutionReadyForPickup     ExecutionState = "READY_FOR_PICKUP"
	ExecutionCustodyTransferred ExecutionState = "CUSTODY_TRANSFERRED"
	ExecutionOutForDelivery     ExecutionState = "OUT_FOR_DELIVERY"
	ExecutionDelivered          ExecutionState = "DELIVERED"
	ExecutionFailed             ExecutionState = "FAILED"
	ExecutionReturnRequired     ExecutionState = "RETURN_REQUIRED"
	ExecutionReturned           ExecutionState = "RETURNED"
)

var ErrInvalidExecutionTransition = errors.New("delivery: invalid execution transition")

// Transition validates vendor-owned execution changes. Custody is explicit;
// a UI state cannot fabricate pickup or handover evidence.
func Transition(from, to ExecutionState) error {
	if from == to {
		return nil
	}
	switch from {
	case ExecutionPlanned:
		if to == ExecutionAssigned || to == ExecutionFailed {
			return nil
		}
	case ExecutionAssigned:
		if to == ExecutionReadyForPickup || to == ExecutionFailed {
			return nil
		}
	case ExecutionReadyForPickup:
		if to == ExecutionCustodyTransferred || to == ExecutionFailed || to == ExecutionReturnRequired {
			return nil
		}
	case ExecutionCustodyTransferred:
		if to == ExecutionOutForDelivery || to == ExecutionReturnRequired {
			return nil
		}
	case ExecutionOutForDelivery:
		if to == ExecutionDelivered || to == ExecutionFailed || to == ExecutionReturnRequired {
			return nil
		}
	case ExecutionReturnRequired:
		if to == ExecutionReturned {
			return nil
		}
	}
	return ErrInvalidExecutionTransition
}
