// Package hauling is the reserved integration boundary for
// driver-hauling-service (van/truck delivery execution authority; owns both
// Van and Truck per CLAUDE.md — Vendor Service must never create a separate
// Van service or route van missions anywhere else).
//
// Same non-authority boundary as internal/integrations/dispatch: vendor-service
// never assigns truck/van providers or mutates hauling booking state
// directly.
//
// Planned contract (not implemented in this architecture phase), modeled on
// driver-hauling-service's existing customer booking contract
// (/api/v1/hauling/customer/bookings):
//   - Quote/serviceability: fare estimate via the existing formula (base +
//     per-km + weight surcharge + helper fee), truck-type/capacity
//     eligibility check before dispatch (see driver-hauling-service's
//     TruckLookup matcher gate).
//   - Mission create: hand off a larger-cargo fulfilment unit (bulk/B2B
//     commerce order, wholesale delivery) once ready.
//   - Read-back: reuse hauling's existing booking status machine
//     (pending_match -> ... -> delivered -> completed).
//   - Cancellation: reuse hauling's existing cancel-after-pickup refusal
//     (CancelByCustomer's guard against picked_up/en_route_delivery)
//     rather than re-deriving cancellation rules.
//   - Provider eligibility: hauling's own marketplace/matcher decides,
//     including fleet-partner-managed truck providers as ordinary
//     marketplace participants.
//   - Pickup/delivery custody confirmation: see internal/integrations/handover
//     — PENDING_EXTERNAL_DEPENDENCY.
//   - Proof/evidence integration: hauling's existing ConfirmDelivery
//     (proof_photo_url/signature_url, both optional/best-effort today) is
//     reused as-is.
//   - Unknown-outcome reconciliation: same authoritative-read-back rule as
//     internal/integrations/dispatch.
package hauling
