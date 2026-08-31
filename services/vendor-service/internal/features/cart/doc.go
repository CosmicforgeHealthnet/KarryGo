// Package cart implements Section 17A customer cart state.
//
// Authority boundaries:
//
//   - Customer Service owns customer identity and saved-address truth.
//   - Food and Shop own catalogue, option and selling-price truth.
//   - Inventory owns stock and reservations.
//   - Logistics services own platform delivery serviceability, pricing and
//     execution.
//   - Wallet owns payment/escrow/refund/settlement.
//   - Cart owns only mutable customer cart state and cart-time snapshots.
//
// A cart snapshot is never final checkout authorization. Section 17B must
// revalidate authoritative catalogue, price, availability, Inventory and
// Vendor/Branch state before creating a quote.
//
// Section 17A deliberately contains no checkout, payment, logistics quote,
// mission, ParentOrder or VendorOrder implementation.
package cart
