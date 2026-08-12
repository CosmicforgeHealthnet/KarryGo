// Package purchasing is the reserved feature boundary for supplier purchase
// orders and goods receiving: a vendor business restocking its own
// inventory/ from its suppliers, distinct from a customer's commerce order.
// Depends on suppliers being modeled (see PLATFORM_SERVICE_CHANGE_REGISTER.md
// if a supplier ever needs to be a real account rather than a plain vendor
// record). Gated by VENDOR_PURCHASING_ENABLED. Not implemented in this
// architecture phase.
package purchasing
