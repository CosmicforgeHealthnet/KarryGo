# Sections 6–10: source audit and implementation gates


## 22 October 2026 — direct staging engineering continuation

PR #15 is merged. Current work starts from staging `f16fa7ed`; historical
unmerged-branch paragraphs below are superseded by this dated reconciliation.
No historical Dispatch branch is merged or cherry-picked. Migrations 070–072
and the later replay/authorization fences remain intact.

Analytics now has all ten fixed native scoped GET routes, their fixed typed SDK
methods and private mTLS Bridge paths. New per-report unit selectors are explicit:
`ANALYTICS_ADMIN_SCOPED_<REPORT_WITH_UNDERSCORES>_UNIT_FIELD`; none has a default.
Each report needs its own signed permission/source contract; configuring another
report does not authorize this one. Readiness reuses its existing native reducer.
Mission outcomes, recovery-event counts, mismatches, safety-separated declines,
individual demand observations and provider-quality components use independently
signed retained native facts. Every bounded subset remains PARTIAL_COVERAGE and
fully suppressed; it does not prove producer completeness. Recovery counts are
not mean resolution time; provider components are not a score. Supply fails
closed without a native individual roster. Demand forecast fails closed without
complete authorized history and an authoritative model contract. These two deny
paths and the other component transports are not a claim of completed reporting
semantics or complete source exports. No aggregate totals become privacy members.

Real PostgreSQL/native ingest, independent owner context import, exact component
counts, cross-tenant denial, audit-failure withholding, typed signed HTTP and
revocation tests cover the new source adapters. Every additional Bridge path has
real mTLS, five-scope, delegation, policy and kill-gate regression tests. Existing
mission-success counts/rate and all existing tests remain unchanged in behavior.
Source/Bridge schemas and capability entries are additive. All production flags
remain off; no signing keys, grants, suppression values or Media lifetimes added.


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


## Current continuation: scoped transport and historical fit reads

The current resource dictionary is 26 kinds / 52 fixed list/detail paths.
Customer review intake/readback, Taxi recovery, Dispatch minimal handover state,
Hauling restriction and historical fit-decision reads are implemented and tested.
The first native scoped Analytics counts transport plus a rate projection,
producer binding preparation and independently signed context publication are
implemented; complete source membership and nine other reducers/routes remain
engineering gaps. All current native Analytics values remain withheld.
Native Media case enforcement, signed private HTTP and typed SDK are tested,
with live case adapters, canonical object membership and HQ facade still missing.
The A–G report and focused read-model documents give the current evidence.
All new HQ production access remains default disabled / BLOCKED_POLICY.
