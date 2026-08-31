package inventory

import "strings"

const (
	EventInventoryReservationHeld = "vendor.inventory.reservation.held"

	EventInventoryReservationConfirmed = "vendor.inventory.reservation.confirmed"

	EventInventoryReservationCommitted = "vendor.inventory.reservation.committed"

	EventInventoryReservationReleased = "vendor.inventory.reservation.released"

	EventInventoryReservationExpired = "vendor.inventory.reservation.expired"

	EventInventoryReceiptPosted = "vendor.inventory.receipt.posted"

	EventInventoryAdjusted = "vendor.inventory.adjusted"

	EventInventoryQuarantined = "vendor.inventory.quarantined"

	EventInventoryQuarantineReleased = "vendor.inventory.quarantine.released"

	EventInventoryTransferDispatched = "vendor.inventory.transfer.dispatched"

	EventInventoryTransferReceived = "vendor.inventory.transfer.received"

	EventInventoryStocktakePosted = "vendor.inventory.stocktake.posted"

	EventInventoryLowStock = "vendor.inventory.low_stock"

	EventInventoryLotExpired = "vendor.inventory.lot.expired"
)

// Section 14G already owns authoritative out-of-stock / restored transitions.
// Reuse those event names rather than emitting competing aliases.
const (
	EventInventoryOutOfStock = EventInventoryItemOutOfStock

	EventInventoryRestocked = EventInventoryItemAvailable
)

func canonicalInventoryEventType(
	action string,
	eventType string,
) string {
	current :=
		strings.TrimSpace(
			eventType,
		)

	key :=
		strings.ToLower(
			strings.TrimSpace(
				action + " " + current,
			),
		)

	containsAny :=
		func(
			values ...string,
		) bool {
			for _, value := range values {

				if strings.Contains(
					key,
					value,
				) {
					return true
				}
			}

			return false
		}

	isReservation :=
		strings.Contains(
			key,
			"reservation",
		)

	isQuarantine :=
		strings.Contains(
			key,
			"quarantine",
		)

	isTransfer :=
		strings.Contains(
			key,
			"transfer",
		)

	isStocktake :=
		strings.Contains(
			key,
			"stocktake",
		)

	isLot :=
		strings.Contains(
			key,
			"lot",
		)

	isReceipt :=
		strings.Contains(
			key,
			"receipt",
		)

	switch {
	case isReservation &&
		containsAny(
			"held",
			"created",
		):

		return EventInventoryReservationHeld

	case isReservation &&
		containsAny(
			"confirm",
		):

		return EventInventoryReservationConfirmed

	case isReservation &&
		containsAny(
			"commit",
		):

		return EventInventoryReservationCommitted

	case isReservation &&
		containsAny(
			"expir",
		):

		return EventInventoryReservationExpired

	case isReservation &&
		containsAny(
			"release",
			"cancel",
		):

		return EventInventoryReservationReleased

	case isQuarantine &&
		containsAny(
			"release",
		):

		return EventInventoryQuarantineReleased

	case isQuarantine:
		return EventInventoryQuarantined

	case isTransfer &&
		containsAny(
			"dispatch",
		):

		return EventInventoryTransferDispatched

	case isTransfer &&
		containsAny(
			"receiv",
		):

		return EventInventoryTransferReceived

	case isStocktake &&
		containsAny(
			"post",
			"correct",
		):

		return EventInventoryStocktakePosted

	case isLot &&
		containsAny(
			"expir",
		):

		return EventInventoryLotExpired

	case isReceipt &&
		containsAny(
			"post",
			"receiv",
		):

		return EventInventoryReceiptPosted

	case containsAny(
		"adjust",
	):
		return EventInventoryAdjusted

	default:
		return current
	}
}
