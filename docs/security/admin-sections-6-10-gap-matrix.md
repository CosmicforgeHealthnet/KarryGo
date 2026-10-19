# Sections 6–10: source audit and implementation gates

Audit base: `69b03e5cd9efabbee718995813c4b49daac00265` (PR #13), stacked on
PR #12 `970c78b000ae50c5fb4191540eef6dd83ec2b921`. Neither dependency is merged;
`origin/staging` is `1f4ff09380f6e9bf74a9f7cc6dbfaab9e85efe95`.
These are engineering dependencies, not authorization for a live cutover.

This table records the **initial** audit. The implemented subset, source repairs,
actual test results and remaining release gates are documented in
`docs/admin-bridge/sections-6-10.md` and its companion runbook.

| Capability | Owner / source evidence | Initial state | Missing requirement / release gate |
|---|---|---|---|
| Private HQ security | Admin `internal/bridge`, `security`, `persistence`; PR12 | EXISTS | Re-run mTLS, delegation, replay, persistence and Gateway isolation tests |
| Partner/Gateway separation | PR13; `docs/migrations/partner-web-gateway-split.md` | PARTIAL | Dedicated source CI passed; sender inventory and live cutover are not certified |
| Ten network reports | Analytics `networkanalytics/http/handler.go`, migrations 003–004 | PARTIAL | Legacy aggregates lack authoritative market/tenant dimensions; no approved suppression/freshness policy supplied. Never expose them as scoped HQ reports |
| Customer directory/detail | Customer migration 001 and owner auth usecases | BLOCKED | Identity rows have no authoritative geographic/tenant authorization mapping; no approved sensitive-data policy |
| Customer restrictions/privacy actions | Customer owner workflows | MISSING | Versioned administrative contracts, source enforcement, approvals and authoritative read-back |
| Taxi corporate ride list/detail/history | Taxi migrations 001,012; `taxi_bookings`, `booking_events` | PARTIAL | Add fixed HMAC owner read and HQ adapter. Use actual market, zone and non-null organization; no invented public tenant |
| Taxi provider/vehicle directory | Taxi providers/vehicles and owner compliance usecases | PARTIAL | A ride's scope does not authorize the provider's entire profile. Owner-scoped contract required |
| Taxi interventions | Taxi operational usecases and `adminaudit` | PARTIAL | Existing operational commands do not satisfy delegated expected-version/approval/read-back contract; remain disabled |
| Dispatch rider/mission directory | Dispatch request/trip/commerce owner repositories | BLOCKED | Organization metadata exists for some trips, but authoritative geographic scopes are absent. No coordinate-based guessed market |
| Dispatch recipient binding | `securehandover/service.go`, repository and notification transport | PARTIAL | Account-bound receiver checks exist; email/SMS channel certification and real devices unavailable |
| Dispatch relay recovery | `marketplace/relay.go`, `relay_worker.go`, Fleet responder | IMPLEMENTED_AND_TESTED | Unknown mutations stay fenced; original-action immutable standard/private assignment and offer receipts reconcile using GET only. Historical unprovable intent stays fenced |
| Dispatch evidence/admin recovery | Securehandover, deliveryrecovery, shared custodyops and Media | PARTIAL | No scoped HQ evidence grant contract or approved recovery policy; no admin custody bypass |
| Hauling providers/cargo missions | Hauling migrations 001,008–020 and execution usecases | BLOCKED | Corporate links do not supply geographic authorization; no owner-scoped HQ snapshot contract |
| Fleet corporate planning/execution links | Fleet marketplace/private execution and tenant auth | PARTIAL | Preserve planning/execution distinction; add a separately authorized HQ geographic mapping/read contract |
| Domain command orchestration | Admin operation ledger/state model | PARTIAL | Durable rejection exists; executable typed owner commands and reconciliation are not implemented |

All new HQ reads require mTLS + HQ HMAC v2 + delegated staff permission, exact
market/zone/organization and signed purpose. Owner requests use only the dedicated
`admin-backoffice-service` principal. No browser credentials reach an owner.
Commands remain disabled until their owner contract and policy are implemented.

Required release evidence: real owner PostgreSQL migration/query tests, distributed
nonce tests, scoped HQ-to-owner integration, stale-version/concurrency/uncertain-write
tests, protected evidence tests, and CI. Live notification delivery, callback
cutover and real-device handover are `BLOCKED_INFRA`, not inferred from local tests.

## Engineering closure after the initial audit

The initial audit table above records the starting gaps. Source-owned adapters
now implement 20 kinds / 40 private resource read paths across Customer, Taxi,
Dispatch, Hauling and Fleet. Signed owner context remediation is implemented,
rechecks canonical facts, and denies historical/unmapped/contradictory rows.
All initial mapping values and production governance still require approval.
Dispatch and Hauling typed custody inventories require new v2 disclosure
contracts. Dispatch incident/recovery/return reads validate actual Trip parent
links and expose no command version where the owner has no complete counter.
Detailed implementation evidence is maintained in
`docs/admin-bridge/owner-resource-contexts.md` and
`docs/admin-bridge/provider-decision-readback.md`.

Remaining engineering: all ten scoped Analytics cohort adapters/facade; sensitive
Customer lifecycle/privacy/support details and commands; richer Taxi/Hauling
incident/remediation/suitability flows; executable HQ owner interventions; scoped
Media evidence grants. These are engineering gaps, distinct from BLOCKED_POLICY.
