// Package production is the shared production and capacity platform behind
// Section 10 (Cloud Kitchen) and Section 11 (Pastry/Bakery).
//
// ONE engine, two first-class capabilities. CLOUD_KITCHEN and PASTRY_BAKERY
// each own their pools, their workflows and their vocabularies, but both
// reserve capacity through the same engine. Two engines would mean two answers
// to "is there room", and the wrong one would still be issuing confirmations.
//
// What a capacity reservation is: a branch has committed production capacity
// in a window. What it is NOT: an order, a sale, a payment, a stock movement or
// a delivery. Vendor Service owns none of those, and nothing in this package
// pretends otherwise.
//
// Overbooking is prevented by locking the affected capacity pool ROWS in
// ascending id order inside the reserving transaction, then recomputing usage
// under that lock. The ordering is what makes multi-pool reservations
// deadlock-safe: two callers requesting the same pools in opposite orders still
// take the locks in the same order. A HELD reservation stops consuming capacity
// the moment its hold expires, decided from the authoritative clock at read
// time rather than by a cleanup worker having run — correctness that depends on
// a worker being punctual is not correctness.
package production
