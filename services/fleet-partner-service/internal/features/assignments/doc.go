// Package assignments names the Section 11 assignment boundary. Its production
// implementation is split by work origin so each keeps one transaction
// boundary: marketplace company work lives in features/marketplace
// (offers, organization acceptance, provider assignments, accept/decline,
// expiry and reassignment) and private company work in
// features/privateoperations. Canonical trips stay owned by Dispatch, Taxi
// and Hauling.
package assignments
