// Package dispatch is the reserved integration boundary for
// driver-dispatch-delivery-service (motorcycle/goods delivery execution
// authority; see CLAUDE.md).
//
// Vendor Service never assigns dispatch riders or mutates dispatch trip
// state directly (see docs/architecture/01_SERVICE_BOUNDARIES.md, "Logistics
// Authority"). It determines delivery requirements and requests execution
// from dispatch's own authority.
//
// Planned contract (not implemented in this architecture phase), modeled on
// the existing customer-facing booking contract
// (driver-dispatch-delivery-service's /api/v1/customer/dispatch routes) and
// the api-gateway Partner API's booking contract, which already delegates
// fare calculation to shared/go/dispatchpricing — vendor-service's own quote
// calls must use the same shared pricing package, never a re-derived fare
// formula:
//   - Quote/serviceability: is a dispatch rider available for this
//     pickup/dropoff pair, and at what fare (shared/go/dispatchpricing).
//   - Mission create: hand off a fulfilment unit for execution once a vendor
//     order is ready, carrying vendor-service's own opaque mission reference.
//   - Read-back: poll or receive lifecycle events (matched, accepted,
//     en_route_pickup, arrived, picked_up, delivered) — reuses dispatch's
//     existing booking-status model, not a new one.
//   - Cancellation: cancel before pickup; after-pickup cancellation is
//     refused by dispatch today (see driver-dispatch-delivery-service's
//     cancel-after-pickup guard) and vendor-service must respect the same
//     refusal rather than retry around it.
//   - Provider eligibility: none asserted by vendor-service — dispatch's own
//     marketplace decides which rider is offered a mission, including
//     company-affiliated (fleet-partner-managed) riders behaving as ordinary
//     marketplace participants (see 01_SERVICE_BOUNDARIES.md, "Fleet /
//     Logistics-Company Providers").
//   - Pickup/delivery custody confirmation: see internal/integrations/handover
//     — PENDING_EXTERNAL_DEPENDENCY, not present in the platform today.
//   - Proof/evidence integration: dispatch's existing SubmitProof/
//     ConfirmRecipient evidence capture (photo, signature, receiver-phone
//     match) is available today and vendor-service reuses it as-is; it is not
//     the same thing as the handover credential above.
//   - Unknown-outcome reconciliation: an internal call whose HTTP outcome is
//     ambiguous (timeout, 5xx after the mission may have been created) must
//     be resolved by an authoritative read-back against dispatch's own
//     mission-reference lookup before vendor-service retries or reports
//     failure — never by assuming success or failure from the transport error
//     alone.
package dispatch
