// Package delivery is the reserved feature boundary for vendor-owned
// delivery: a vendor business operating its own riders/fleet as an
// alternative to platform Dispatch/Taxi/Hauling execution for its orders.
// This is Vendor Service's own delivery-execution authority for
// vendor-owned missions only — it must never be confused with, or reused to
// implement, the platform Dispatch/Taxi/Hauling authorities, and a
// vendor-owned rider is not a marketplace provider in those services' sense.
// Gated by VENDOR_OWN_DELIVERY_ENABLED. Not implemented in this architecture
// phase.
package delivery
