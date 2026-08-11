// Package inventory is the reserved feature boundary for vendor stock
// control: on-hand quantity, reservations against pending orders, and
// low-stock signals per branch. This is a vendor's own sellable-item stock,
// distinct in both schema and meaning from fleet-partner-service's
// warehouse/ inventory_item concept (3PL warehouse stock-keeping for managed
// organizations) — the two must never be confused or merged even though both
// use the word "inventory". Gated by VENDOR_INVENTORY_ENABLED. Not
// implemented in this architecture phase.
package inventory
