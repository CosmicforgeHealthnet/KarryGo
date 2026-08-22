package orders

// Fulfilment isolation and aggregate state.
//
// Each VendorOrder runs its own preparation, ready, handover, delivery, cancel
// and refund state. Nothing here forces every VendorOrder of a purchase into
// one shared lifecycle, and the ParentOrder's state is always derived from the
// children rather than written independently.

// Transition names one VendorOrder lifecycle command.
type Transition string

const (
	TransitionAccept         Transition = "ACCEPT"
	TransitionReady          Transition = "READY"
	TransitionHandover       Transition = "HANDOVER"
	TransitionDeliver        Transition = "DELIVER"
	TransitionComplete       Transition = "COMPLETE"
	TransitionCancel         Transition = "CANCEL"
	TransitionReject         Transition = "REJECT"
	TransitionPreparing      Transition = "START_PREPARING"
	TransitionPicking        Transition = "START_PICKING"
	TransitionPack           Transition = "PACK"
	TransitionReadyForPickup Transition = "READY_FOR_PICKUP"
	TransitionPickup         Transition = "PICKUP"
	TransitionDelivering     Transition = "DELIVERING"
	TransitionDelivered      Transition = "DELIVERED"
)

// AllTransitions is the ordered command set, used by the HTTP surface and the
// contract test so a new command cannot be added in only one of them.
var AllTransitions = []Transition{
	TransitionAccept,
	TransitionReady,
	TransitionHandover,
	TransitionDeliver,
	TransitionComplete,
	TransitionCancel,
	TransitionReject,
	TransitionPreparing,
	TransitionPicking,
	TransitionPack,
	TransitionReadyForPickup,
	TransitionPickup,
	TransitionDelivering,
	TransitionDelivered,
}

// Outcome is the result of applying a transition to one VendorOrder.
type Outcome struct {
	Status            VendorStatus
	VendorOwnDelivery VendorOwnDeliveryState

	// Terminal marks a status that can never change again.
	Terminal bool

	// RefundAllocatedMinor and SettlementAllocatedMinor are PREPARED, never
	// executed. Section 17E records what a later payment-capable section will
	// have to move; it moves nothing itself.
	RefundAllocatedMinor     int64
	SettlementAllocatedMinor int64

	Action string
}

// Lifecycle event actions.
const (
	ActionOrderCreated       = "commerce.order.created"
	ActionOrderStateChanged  = "commerce.order.state_changed"
	ActionVendorOrderCreated = "commerce.vendor_order.created"
	ActionAccepted           = "commerce.vendor_order.accepted"
	ActionPreparing          = "commerce.vendor_order.preparing"
	ActionPaid               = "commerce.vendor_order.paid"
	ActionSentToVendor       = "commerce.vendor_order.sent_to_vendor"
	ActionReady              = "commerce.vendor_order.ready"
	ActionHandedOver         = "commerce.vendor_order.handed_over"
	ActionPickedUp           = "commerce.vendor_order.picked_up"
	ActionDelivering         = "commerce.vendor_order.delivering"
	ActionDelivered          = "commerce.vendor_order.delivered"
	ActionCompleted          = "commerce.vendor_order.completed"
	ActionCancelled          = "commerce.vendor_order.cancelled"
	ActionRejected           = "commerce.vendor_order.rejected"
	ActionPicking            = "commerce.vendor_order.picking"
	ActionPacked             = "commerce.vendor_order.packed"
	ActionReadyForPickup     = "commerce.vendor_order.ready_for_pickup"
)

// ApplyTransition decides one VendorOrder's next state.
//
// It is a pure function of the order's own current state, so one vendor's
// progress can never depend on, or disturb, another vendor's.
func ApplyTransition(order VendorOrder, transition Transition, cancelledBy CancelActor) (Outcome, error) {
	if order.Status.Terminal() {
		return Outcome{}, ErrOrderConflict
	}

	switch transition {
	case TransitionAccept:
		if order.Status != VendorStatusCreated && order.Status != VendorStatusSentToVendor && order.Status != VendorStatusPaid {
			return Outcome{}, ErrOrderConflict
		}
		next := VendorStatusPreparing
		if order.Status == VendorStatusSentToVendor || order.Status == VendorStatusPaid {
			next = VendorStatusAccepted
		}
		return Outcome{
			Status:            next,
			VendorOwnDelivery: order.VendorOwnDeliveryState,
			Action:            ActionAccepted,
		}, nil

	case TransitionReject:
		if order.Status != VendorStatusCreated && order.Status != VendorStatusPaid &&
			order.Status != VendorStatusSentToVendor && order.Status != VendorStatusAccepted {
			return Outcome{}, ErrOrderConflict
		}
		return Outcome{Status: VendorStatusRejected, VendorOwnDelivery: order.VendorOwnDeliveryState,
			Terminal: true, RefundAllocatedMinor: order.CustomerPayableMinor, Action: ActionRejected}, nil

	case TransitionPreparing:
		if order.Status != VendorStatusAccepted && order.Status != VendorStatusCreated {
			return Outcome{}, ErrOrderConflict
		}
		return Outcome{Status: VendorStatusPreparing, VendorOwnDelivery: order.VendorOwnDeliveryState, Action: ActionPreparing}, nil

	case TransitionPicking:
		if order.Status != VendorStatusAccepted && order.Status != VendorStatusPreparing {
			return Outcome{}, ErrOrderConflict
		}
		return Outcome{Status: VendorStatusPicking, VendorOwnDelivery: order.VendorOwnDeliveryState, Action: ActionPicking}, nil

	case TransitionPack:
		if order.Status != VendorStatusPicking && order.Status != VendorStatusPreparing && order.Status != VendorStatusAccepted && order.Status != VendorStatusSubstitutionReq && order.Status != VendorStatusAwaitingCustomer {
			return Outcome{}, ErrOrderConflict
		}
		return Outcome{Status: VendorStatusPacking, VendorOwnDelivery: order.VendorOwnDeliveryState, Action: ActionPacked}, nil

	case TransitionReadyForPickup:
		if order.Status != VendorStatusPacking && order.Status != VendorStatusPreparing && order.Status != VendorStatusReady {
			return Outcome{}, ErrOrderConflict
		}
		return Outcome{Status: VendorStatusReadyForPickup, VendorOwnDelivery: order.VendorOwnDeliveryState, Action: ActionReadyForPickup}, nil

	case TransitionPickup:
		if order.Status != VendorStatusReadyForPickup && order.Status != VendorStatusReady && order.Status != VendorStatusHandedOver {
			return Outcome{}, ErrOrderConflict
		}
		if order.DeliveryResponsibility == DeliveryPlatform {
			return Outcome{}, ErrForbidden
		}
		next := order.VendorOwnDeliveryState
		if order.DeliveryResponsibility == DeliveryVendorOwn {
			next = VendorOwnDispatched
		}
		return Outcome{Status: VendorStatusPickedUp, VendorOwnDelivery: next, Action: ActionPickedUp}, nil

	case TransitionDelivering:
		if order.Status != VendorStatusPickedUp && order.Status != VendorStatusHandedOver {
			return Outcome{}, ErrOrderConflict
		}
		if order.DeliveryResponsibility == DeliveryPlatform {
			return Outcome{}, ErrForbidden
		}
		return Outcome{Status: VendorStatusDelivering, VendorOwnDelivery: VendorOwnDispatched, Action: ActionDelivering}, nil

	case TransitionDelivered:
		if order.Status != VendorStatusDelivering && order.Status != VendorStatusPickedUp && order.Status != VendorStatusHandedOver {
			return Outcome{}, ErrOrderConflict
		}
		if order.DeliveryResponsibility == DeliveryPlatform {
			return Outcome{}, ErrForbidden
		}
		return Outcome{Status: VendorStatusDelivered, VendorOwnDelivery: VendorOwnDelivered, Action: ActionDelivered}, nil

	case TransitionReady:
		if order.Status != VendorStatusPreparing {
			return Outcome{}, ErrOrderConflict
		}
		return Outcome{
			Status:            VendorStatusReady,
			VendorOwnDelivery: order.VendorOwnDeliveryState,
			Action:            ActionReady,
		}, nil

	case TransitionHandover:
		if order.Status != VendorStatusReady {
			return Outcome{}, ErrOrderConflict
		}
		// Platform logistics owns the physical handover. A vendor may only
		// mark handover when the vendor itself is the delivery executor.
		if order.DeliveryResponsibility == DeliveryPlatform {
			return Outcome{}, ErrForbidden
		}
		next := order.VendorOwnDeliveryState
		if order.DeliveryResponsibility == DeliveryVendorOwn {
			next = VendorOwnDispatched
		}
		return Outcome{
			Status:            VendorStatusHandedOver,
			VendorOwnDelivery: next,
			Action:            ActionHandedOver,
		}, nil

	case TransitionDeliver:
		if order.Status != VendorStatusHandedOver {
			return Outcome{}, ErrOrderConflict
		}
		// A platform-executed delivery is confirmed by the logistics authority
		// that performed it, never by the vendor who handed the goods over.
		// Section 17E has no mission execution, so it refuses rather than
		// letting a vendor assert a delivery it cannot observe.
		if order.DeliveryResponsibility == DeliveryPlatform {
			return Outcome{}, ErrForbidden
		}
		next := order.VendorOwnDeliveryState
		if order.DeliveryResponsibility == DeliveryVendorOwn {
			next = VendorOwnDelivered
		}
		return Outcome{
			Status:            VendorStatusDelivered,
			VendorOwnDelivery: next,
			Action:            ActionDelivered,
		}, nil

	case TransitionComplete:
		if order.Status != VendorStatusDelivered {
			return Outcome{}, ErrOrderConflict
		}
		return Outcome{
			Status:                   VendorStatusCompleted,
			VendorOwnDelivery:        order.VendorOwnDeliveryState,
			Terminal:                 true,
			SettlementAllocatedMinor: order.VendorPayableMinor,
			Action:                   ActionCompleted,
		}, nil

	case TransitionCancel:
		if cancelledBy != CancelByVendor && cancelledBy != CancelByCustomer &&
			cancelledBy != CancelByPlatform {
			return Outcome{}, ErrOrderConflict
		}
		// Cancellation is a pre-handover command. Once a pickup/delivery
		// authority has begun execution, a normal cancellation must not pretend
		// that stock never left the reservation.
		switch order.Status {
		case VendorStatusCreated, VendorStatusPendingPayment, VendorStatusPaid,
			VendorStatusSentToVendor, VendorStatusAccepted, VendorStatusPreparing,
			VendorStatusPicking, VendorStatusSubstitutionReq,
			VendorStatusAwaitingCustomer, VendorStatusPacking,
			VendorStatusReadyForPickup, VendorStatusReady:
		default:
			return Outcome{}, ErrOrderConflict
		}
		next := order.VendorOwnDeliveryState
		if order.DeliveryResponsibility == DeliveryVendorOwn {
			next = VendorOwnCancelled
		}
		return Outcome{
			Status:               VendorStatusCancelled,
			VendorOwnDelivery:    next,
			Terminal:             true,
			RefundAllocatedMinor: order.CustomerPayableMinor,
			Action:               ActionCancelled,
		}, nil
	}

	return Outcome{}, ErrOrderConflict
}

// CancellableByCustomer reports whether a customer may still withdraw from one
// vendor order. Once a vendor has handed the goods over, withdrawal is no
// longer a cancellation and belongs to a returns/dispute flow.
func CancellableByCustomer(status VendorStatus) bool {
	return status == VendorStatusCreated || status == VendorStatusPendingPayment ||
		status == VendorStatusPaid || status == VendorStatusSentToVendor ||
		status == VendorStatusAccepted || status == VendorStatusPreparing ||
		status == VendorStatusPicking || status == VendorStatusPacking ||
		status == VendorStatusReadyForPickup
}

// DeriveParentState computes the ParentOrder's aggregate state.
//
// A mixed outcome is never hidden: a purchase where some vendors completed and
// others cancelled reports PARTIALLY_COMPLETED, not COMPLETED and not
// CANCELLED.
func DeriveParentState(statuses []VendorStatus) ParentState {
	if len(statuses) == 0 {
		return ParentStateCreated
	}

	var completed, cancelled, created, refunded, partiallyRefunded, refundPending int
	for _, status := range statuses {
		switch status {
		case VendorStatusCompleted, VendorStatusDelivered:
			completed++
		case VendorStatusCancelled, VendorStatusRejected:
			cancelled++
		case VendorStatusRefunded:
			refunded++
		case VendorStatusPartiallyRefunded:
			partiallyRefunded++
		case VendorStatusRefundPending:
			refundPending++
		case VendorStatusCreated, VendorStatusPendingPayment:
			created++
		}
	}

	switch {
	case refundPending > 0:
		return ParentStateRefundPending
	case refunded == len(statuses):
		return ParentStateRefunded
	case refunded+partiallyRefunded == len(statuses) && partiallyRefunded > 0:
		return ParentStatePartiallyRefunded
	case completed == len(statuses):
		return ParentStateCompleted
	case cancelled == len(statuses):
		return ParentStateCancelled
	case completed+cancelled+refunded+partiallyRefunded == len(statuses):
		// Every vendor order is finished, but they did not all end the same
		// way. Reporting either extreme here would be a lie.
		return ParentStatePartiallyCompleted
	case created == len(statuses):
		return ParentStateCreated
	default:
		return ParentStateInProgress
	}
}

// TerminalParentState reports whether an aggregate state is final.
func TerminalParentState(state ParentState) bool {
	return state == ParentStateCompleted ||
		state == ParentStatePartiallyCompleted ||
		state == ParentStateCancelled || state == ParentStateRefunded ||
		state == ParentStatePartiallyRefunded
}

// DeriveParentPreparedAmounts totals the prepared refund and settlement
// allocations across a purchase.
//
// Parent-scope fee and tax become refundable only when the whole purchase is
// cancelled: while any vendor order still stands, the customer is still buying
// something and the order-level charge still applies.
func DeriveParentPreparedAmounts(
	orders []VendorOrder,
	platformFeeTaxMinor int64,
) (refund int64, settlement int64) {
	allCancelled := len(orders) > 0
	for _, order := range orders {
		refund += order.RefundAllocatedMinor
		settlement += order.SettlementAllocatedMinor
		if order.Status != VendorStatusCancelled {
			allCancelled = false
		}
	}
	if allCancelled {
		refund += platformFeeTaxMinor
	}
	return refund, settlement
}
