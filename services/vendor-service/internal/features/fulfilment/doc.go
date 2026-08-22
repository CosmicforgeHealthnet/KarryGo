// Package fulfilment owns commerce fulfilment planning: turning validated
// Section 17B vendor/branch groups into an authoritative candidate fulfilment
// plan.
//
// Section 17C builds this in slices. What is implemented here so far:
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
//   - TAXI IS NOT USED. driver-taxi-service has no production goods-delivery
//     capability, so VehicleCar is defined but never selected and no taxi
//     route is ever called. See vehicle.go for how that is enforced rather
//     than merely intended.
package fulfilment
