// Package fulfilmentgroup owns the post-order operational fulfilment
// aggregate: the FulfilmentGroup rooted on one CommerceOrder, the
// FulfilmentUnits inside it, the Parcels that physically move, the versioned
// FulfilmentPlan history, the projections of platform logistics missions, and
// the VendorMissions this service genuinely owns.
//
// It is deliberately NOT internal/features/fulfilment. That package answers a
// pre-order question — "what would delivering this checkout cost, and can it
// be delivered at all" — and its Plan is bound to a checkout session. This
// package answers the post-order question: what is actually being fulfilled,
// in which packages, by whom, under which plan version. Merging them would
// give one type two lifecycles and make "which plan is in force" ambiguous at
// exactly the moment a customer asks.
//
// # Authority boundaries
//
// These are the lines that decide what this package is allowed to believe.
//
//   - VendorMission is CANONICAL here, and only for vendor-owned delivery.
//     The vendor performs the delivery, so the vendor's own service owns the
//     mission's truth. Its status vocabulary is the delivery feature's
//     existing ExecutionState, reused rather than re-invented.
//
//   - PlatformMissionRef is a PROJECTION and nothing more. Dispatch, Hauling
//     and Taxi each remain the authority for their own missions. This package
//     records an opaque external reference, the authority that owns it, and a
//     version-stamped status copy for vendor coordination. It never parses
//     meaning out of a mission identifier, never sets a platform mission's
//     status from a staff action, and never writes to a logistics database.
//
//   - The two are separate types and separate tables. Collapsing them into one
//     ambiguous mission entity would erase the only thing that answers "whose
//     word is final about this delivery".
//
//   - payment-wallet-service remains the financial authority. A plan version
//     stores a planned-cost SNAPSHOT so a later financial question can be
//     answered against the plan that was actually in force. That snapshot is
//     evidence, not a ledger, and nothing here creates a hold, a settlement or
//     a refund.
//
//   - The refund and delivery-failure policy of the orders feature is not
//     duplicated here. This package's job is to make the group, unit, parcel,
//     plan version and mission relationships traceable enough for that policy
//     to identify what it is deciding about.
//
// # Identity invariants
//
// Four identities must survive operational change, because every one of them
// is something a human is already looking at when the change happens:
//
//	CommerceOrder   -> one FulfilmentGroup, forever
//	VendorOrder     -> one FulfilmentUnit, forever
//	Parcel          -> the same parcel across replan, reassignment, mission
//	                   replacement, transfer and return
//	FulfilmentPlan  -> append-only versions; v1 stays readable after v3 is live
//
// A replan therefore creates the NEXT plan version and rewires mission slots
// under it. It never edits a historical version, never re-mints a parcel, and
// never creates a second group or unit.
//
// # Multi-pickup grouping
//
// One platform mission may carry several compatible vendor pickups to one
// receiver destination, expressed as several PICKUP stops and exactly one
// DROPOFF stop in a deterministic persisted sequence. Grouping is an
// OPTIMISATION, never a mandatory merge: see grouping.go, where a shared
// destination alone is explicitly not sufficient, and an authority that cannot
// execute multiple stops causes a split rather than a mission it would have to
// fail later.
//
// Taxi is modelled in the authority vocabulary but is not a currently active
// commerce fulfilment authority — driver-taxi-service has no commerce mission
// ingress. Selecting it is refused rather than attempted; see Authority.
package fulfilmentgroup
