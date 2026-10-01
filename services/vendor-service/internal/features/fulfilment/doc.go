// Package fulfilment owns commerce fulfilment planning: turning validated
// Section 17B vendor/branch groups into an authoritative candidate fulfilment
// plan.
//
// Section 17C and the delivery-policy extension build this in slices. The
// package now also records smart-hybrid owner/fallback evidence, preserves
// customer delivery-price guarantees, and guards post-custody takeover.
//
//   - requirements.go — aggregation of ACTUAL logistics metadata per vendor
//     group (weight, volume, dimensions, package count, handling and
//     temperature requirements, urgency), from the catalogue authorities that
//     own it. Nothing is guessed: an unstated weight is reported as unstated,
//     not as zero.
//   - vehicle.go — deterministic vehicle-class selection from those
//     requirements, and the mapping from a class to the logistics authority
//     that serves it.
//
// Boundaries that hold for every later slice:
//
//   - Vendor Service never assigns a platform provider, never writes to a
//     driver service's providers/bookings/trips tables and never invents a
//     competing matching algorithm. It determines requirements and requests
//     execution.
//   - Vendor Service never prices a platform delivery. The fee comes from the
//     owning logistics authority's own quote. The single exception is
//     Vendor Own Delivery (internal/features/delivery), where the vendor both
//     performs and prices the delivery.
//   - CAR is executed only through driver-taxi-service's distinct Taxi Goods
//     quote/mission contract. The Vendor and Taxi feature flags default false;
//     no passenger booking endpoint is used for Commerce.
package fulfilment
