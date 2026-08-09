// Package safety implements fleet-side incident and dispute REFERENCES.
//
// # What this package is, and is not
//
// It is not an incident system. The support-dispute service owns complaints,
// safety incidents and disputes: their content, their lifecycle, who may read
// them and what they mean. This package stores opaque pointers to those cases
// plus the fleet-side associations — assignment, request, trip, provider,
// vehicle — that make a case findable from the organization's own records.
//
// # Boundaries
//
//   - No cross-service foreign key. The external identifier is text, is never
//     parsed, and is never used in a join against another service's table.
//   - No direct read of another service's database. The only way fleet learns
//     anything about a case is an authenticated private HTTP call.
//   - Verification is explicit. Every stored row states whether the
//     authoritative service confirmed the identifier. An UNVERIFIED reference
//     is a note for humans and authorizes nothing: not a stand-down, not a
//     withheld payment, not a suspension.
//   - Fail closed. A deployment that could validate but cannot reach the
//     authority refuses the write rather than downgrading it to UNVERIFIED,
//     because an outage must not become a way to record arbitrary references.
//
// # Layout
//
//	models        the reference and its append-only history
//	repositories  tenant-scoped storage; no unscoped read exists
//	usecases      validation, idempotency, conflict detection, audit, outbox
//	http          corporate routes behind organization scope and permissions
//
// See docs/platform-boundaries.md for the platform-wide source-of-truth rules
// this package implements on the fleet side.
package safety
