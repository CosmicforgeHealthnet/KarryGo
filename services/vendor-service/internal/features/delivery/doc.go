// Package delivery owns Vendor Own Delivery: a vendor business operating its
// own riders/fleet as an alternative to platform Dispatch/Hauling execution
// for its own orders.
//
// Section 17C (slice 1) implements the vendor-configured delivery policy and
// the vendor-authoritative charge evaluation built on it. What that means
// precisely:
//
//   - Vendor Own Delivery is INDEPENDENT of platform logistics. Choosing it
//     creates no Dispatch/Hauling mission and selects no platform rider. This
//     package never calls internal/integrations/dispatch|hauling|taxi and must
//     never be reused to implement any of them; a vendor-owned rider is not a
//     marketplace provider in those services' sense.
//   - The delivery charge is calculated from the vendor's own configured
//     policy. It is authoritative because the vendor owns both the execution
//     and the price, which is exactly why the same freedom must NOT be
//     extended to a platform-executed delivery, whose fee belongs to the
//     executing logistics authority.
//   - Who receives the delivery fee is derived, never configured. Per
//     docs/architecture/05_PAYMENT_ALLOCATION_MODEL.md, vendor-owned delivery
//     pays the vendor; platform delivery pays the executing provider and never
//     the vendor. See Decision.FeeBeneficiary.
//
// Gated by VENDOR_OWN_DELIVERY_ENABLED. Vendor-owned mission execution
// (rider assignment, lifecycle, proof) is not implemented here; 17C plans, it
// does not execute.
package delivery
