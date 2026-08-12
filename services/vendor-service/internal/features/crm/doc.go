// Package crm implements Section 8A: the CRM foundation, Customer 360,
// consent and privacy projections.
//
// # What this package is
//
// A CRM profile represents the commercial relationship between one vendor
// business and one customer subject. It is a Vendor-local projection, not a
// customer master. customer-service remains the authority for canonical
// identity, credentials, email, phone, addresses, identity merges, and
// (whenever such an authority exists) consent and privacy decisions. This
// package therefore:
//
//   - stores opaque, authority-owned subject references only — never canonical
//     email, phone, date of birth, address or credentials, and never a hash of
//     any of them used as a stand-in customer master;
//   - never creates a relationship from Vendor staff assertion. Profiles come
//     into existence only from an authenticated authoritative event, so a
//     production database legitimately holds zero CRM profiles until a real
//     commerce or identity authority is integrated;
//   - fails closed on marketing: an absent or unknown consent decision is
//     UNKNOWN, and UNKNOWN is never treated as ALLOW;
//   - lets privacy outrank consent: a redacted, restricted or
//     marketing-suppressed subject is ineligible regardless of any stored
//     ALLOW.
//
// # What this package deliberately does not do
//
// Section 8A stops at the foundation. Tags and segmentation (8B), interactions
// and tasks (8C), loyalty (8D), campaigns (8E/8F), corporate CRM (8G) and
// scoring/exports (8H) are not implemented here. Customer 360 reports those
// modules as unavailable rather than returning an empty structure that would
// read as "implemented and empty". Privacy processing is built around a
// handler registry (privacy.go) so those later domains can register their own
// redaction behaviour without the redaction engine being rewritten.
//
// # Authority dependencies, honestly stated
//
// At the time of writing no platform service publishes vendor commerce order
// events, customer consent changes, customer privacy instructions or customer
// identity links. The ingestion contract, projection algorithm, dedupe,
// source-version rules and rebuild path for all four are implemented and
// tested against fixtures, and the live integration stays dormant behind
// configuration. Nothing here fabricates a publisher.
package crm
