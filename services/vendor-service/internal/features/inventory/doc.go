// Package inventory is the Vendor Service's generic inventory authority
// boundary.
//
// This inventory domain is distinct from fleet-partner-service warehouse
// inventory. Fleet/3PL inventory represents warehouse stock held or managed
// for organisations; Vendor Inventory represents a vendor business's own
// operational stock and sellability.
//
// Section 14A establishes the shared Inventory application/domain contracts.
// The already-certified Shop inventory implementation remains operational and
// unchanged until later Section 14 compatibility phases bridge it into this
// generic authority.
//
// Inventory is the future authority for:
//
//   - inventory item identity
//   - stock basis
//   - physical balances
//   - reservation balances
//   - quarantined quantity
//   - in-transit quantity
//   - lots and batches
//   - immutable stock movements
//   - stock reservations
//   - stocktakes
//   - branch transfers
//   - reorder rules
//   - stock alerts
//   - inventory revisions
//
// Inventory does not own:
//
//   - Shop product/variant/listing definitions
//   - Shop measurement vocabulary
//   - Food catalogue definitions
//   - supplier definitions
//   - purchasing or purchase orders
//   - ingredients or recipes
//   - production
//   - customer Orders or Checkout
//   - Wallet or Payment
//   - logistics execution
//
// External domain information is consumed through ports. In particular,
// inventory must never import Shop repository implementations.
//
// No generic stock persistence or generic Inventory HTTP routes are introduced
// in Section 14A.
package inventory
