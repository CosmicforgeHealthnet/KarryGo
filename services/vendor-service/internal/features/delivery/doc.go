// Package delivery owns branch delivery policy and vendor-owned execution
// evidence, including PLATFORM, VENDOR and HYBRID selection alongside the
// platform Dispatch/Hauling authorities.
//
// The delivery-policy implementation keeps the following boundaries:
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
// Gated by VENDOR_OWN_DELIVERY_ENABLED. Vendor-owned execution state is
// represented here, while provider matching, custody proof and logistics
// execution remain with their owning authorities.
package delivery
