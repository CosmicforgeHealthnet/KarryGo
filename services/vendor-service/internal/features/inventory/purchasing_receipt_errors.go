package inventory

import "errors"

// These errors belong specifically to the Section 15C Purchasing ->
// Inventory transaction seam. They avoid pretending that unrelated Inventory
// workflows such as stocktakes, transfers or reservations own these states.
var (
	ErrPurchasingReceiptInvalid = errors.New(
		"inventory purchasing receipt input is invalid",
	)

	ErrPurchasingReceiptStateConflict = errors.New(
		"inventory purchasing receipt state conflict",
	)
)
