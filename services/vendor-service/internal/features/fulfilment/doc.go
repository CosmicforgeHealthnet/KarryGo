// Package fulfilment is the reserved feature boundary for commerce
// fulfilment planning: given one or more vendor orders from a single
// checkout, decide how many fulfilment groups/units/parcels are needed, which
// execution authority serves each (internal/integrations/dispatch|taxi|hauling,
// or internal/features/delivery for vendor-owned), and replanning when a
// mission fails or a vendor rejects an order. This is the orchestration layer
// described in docs/architecture/06_COMMERCE_FULFILMENT_MODEL.md — it
// requests execution and reacts to lifecycle events, it never assigns a
// platform provider or mutates a platform trip directly. Gated by
// COMMERCE_FULFILMENT_WORKERS_ENABLED for its background replanning worker.
// Not implemented in this architecture phase.
package fulfilment
