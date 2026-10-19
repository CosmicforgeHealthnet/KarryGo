# Karryam Admin Sections 6–10 — incremental implementation report

## A. Repository and dependencies

Work started on `codex/secure-admin-sections-2-5` at
`970c78b000ae50c5fb4191540eef6dd83ec2b921`, with a clean worktree. After fetching
and inspecting the two draft PRs, implementation branched from PR #13's tip
`69b03e5cd9efabbee718995813c4b49daac00265` onto
`codex/secure-admin-sections-6-10`. The stack is:

`staging` → [PR #12](https://github.com/CosmicforgeHealthnet/KarryGo/pull/12)
→ [PR #13](https://github.com/CosmicforgeHealthnet/KarryGo/pull/13) → this branch.

At the audit, both dependencies were OPEN/DRAFT and `origin/staging` remained
`1f4ff09380f6e9bf74a9f7cc6dbfaab9e85efe95`. PR12's dedicated Admin run
39118317754 passed. PR13's dedicated split run 39129188831 and Admin run
39129188891 passed. Repository-wide failures were recorded separately; neither
PR is merged or certified for live cutover. Final source SHA, PR identity and
actual CI outcomes are recorded in the implementation PR and delivery message.

No changes were made to Centralized Admin, production, deployed firewall rules,
live callback targets, or live feature flags. The user-supplied pasted brief is
the available specification; the named A-to-Z PDF was not supplied.

## B. Outcome by section

| Section | Actual result | Evidence | Status / outstanding |
|---|---|---|---|
| 6 Analytics | Ten fixed-path typed legacy clients and source dictionary; owner-side suppression service with durable disclosure journal, audit and reconstruction guard | Real PostgreSQL concurrency, frozen release, filter restriction, suppression and audit-failure tests | FRAMEWORK_TESTED / BLOCKED_POLICY. No production cohort source adapter or scoped HQ report exposure; legacy reports remain dormant |
| 7 Customer | Source-owned customer, consent and booking-reference reads; signed authorization-context remediation and immutable read audit | Actual Customer migrations, orphan-reference rejection, historical/unmapped and scope-isolation tests | INTEGRATION_TESTED / BLOCKED_POLICY. Sensitive support/privacy detail and executable lifecycle commands remain engineering work |
| 8 Taxi | Corporate rides plus source-owned provider, vehicle and independent-ride read models; signed context publication and binding preparation | Real PostgreSQL owner tests and actual Taxi process behind the mTLS Bridge; revoke/read isolation | INTEGRATION_TESTED / BLOCKED_POLICY. Incidents, executable interventions and Media grants remain engineering work |
| 9 Dispatch | Rider, motorcycle, mission and typed multi-stop/custody inventory reads; standard/private assignment and marketplace offer immutable Fleet decision readbacks | Actual owner migrations, scoped HTTP/schema tests, PostgreSQL/Redis uncertainty and concurrency tests | INTEGRATION_TESTED / BLOCKED_POLICY for HQ disclosure. Intervention commands, richer incident/return detail and Media grants remain engineering work |
| 10 Hauling/Fleet | Provider/truck/mission reads, recorded physical truck capacity/equipment, native recovery case metadata and typed custody inventory; Fleet shipment/private-assignment/execution-link reads; canonical planning tenant checks | Actual five-owner migrations, conflicting/settlement tenant denial, scoped custody HTTP/schema tests | INTEGRATION_TESTED / BLOCKED_POLICY. Protected inspections/cargo-fit detail and executable remediation remain engineering work |

The initial source-by-source assessment is in
`docs/security/admin-sections-6-10-gap-matrix.md`. This change is an incremental
closure, not completion of all five sections.

## C. Implemented APIs, permissions and field map

| Method / bridge path | Permission | Owner path | Verified behavior |
|---|---|---|---|
| GET `/internal/karryam-admin/v1/taxi/rides` | `taxi.rides.read` | GET `/internal/taxi/v1/admin/rides` | Bounded, scoped keyset page of corporate bookings |
| GET `/internal/karryam-admin/v1/taxi/rides/{id}` | `taxi.rides.read` | GET `/internal/taxi/v1/admin/rides/{id}` | Same source scope; latest 200 real booking events, explicit history truncation |

Both routes require the existing independently authenticated HQ workload and
staff assertion, plus an exact approved role, market, **zone**, organization and tenant.
There is no wildcard or inferred public organization. A signed purpose and
explicit start/end window (at most 31 days), limit (1–100) and optional cursor
are required. The window selects booking creation time; detail history is a
bounded latest-event slice. Cursors expire after ten minutes and are bound to
staff, purpose, scope, window, page size and the fixed owner contract. These are
technical query bounds, not financial/recovery/privacy approval thresholds.

`StaffAssertionZones` is additive to the original assertion schema. Metadata
clients can continue omitting `zone_scopes` and `tenant_scopes`; owner reads cannot. The exact roles
must be supplied by the authorization-policy owner; no production role is
implicitly approved. Discovery reports this adapter DISABLED when its startup
configuration, role grant or geographic delegation is absent.

The typed response exposes only ride ID, actual status, mode, source context,
creation/update timestamps, event ID/type/time and pagination/freshness metadata.
The SQL joins Taxi's own market and zone records, checks the zone belongs to the
booking's market, and requires the actual non-null corporate organization.
Unmapped independent rides are excluded with `PARTIAL_COVERAGE`. A repeatable
snapshot keeps booking and event reads consistent. No contacts, customer/provider
IDs, GPS, amounts, free-text notes, raw metadata, documents, OTPs or evidence URLs
are serialized. The bridge rejects unknown owner response fields and fails data
closed if either audit cannot persist.

`as_of` is the owner's transaction time and is preserved by the bridge. No event
history is fabricated. There is no new replicated projection: bounded owner
snapshots deliberately avoid an unsupported event-cursor/history claim.
`source_version` is null because Taxi's existing Fleet counter does not version
every lifecycle update; it must not be used for administrative writes.

### Command catalog

No new HQ command is executable. Existing `/operations` persists a durable
REJECTED record with `capability_blocked`; it does not execute an owner action.

| Command family | State | Gate before implementation/enablement |
|---|---|---|
| Customer restriction, remediation, merge/split, privacy fulfillment | DISABLED | Owner contract, scope provenance, approved policy, current version, idempotency, read-back |
| Taxi incident/restriction/reverification/recovery | DISABLED | Owner-supported action with reliable mutation version and approval policy |
| Dispatch retry/replan/reassignment/recovery/evidence | DISABLED | Geographic scope, current assignment/custody checks, receipts, reconciliation and Media case grants |
| Hauling/Fleet recovery/restriction/intervention | DISABLED | Geographic + tenant contract, cargo suitability/custody policy and owner receipts |

No generic status override, forced completion, custody override, GPS correction
or wallet mutation was introduced. Independent global write and domain rollout
flags now exist, all default false. They never grant permission or instantiate
an absent owner adapter: even all flags true leaves HQ commands durably rejected.
Disabling the existing bridge remote flag also leaves public traffic unaffected.

### Policy enforcement and pending governance

The three requested versioned policy documents and machine-readable matrix are
in `docs/admin/policies/`, all **DRAFT — PENDING OWNER APPROVAL**. Every unresolved
capability is explicitly `BLOCKED_POLICY`. No production thresholds, grants or
missing domain relationships are supplied. The companion draft JSON cannot
authorize a request.

The shared evaluator intersects verified staff permissions and all five scope
dimensions with an explicit owner-approved tuple, role and purpose. The owner
must independently resolve authoritative record facts. Taxi derives the tenant
only for Fleet-origin corporate execution from the actual organization link,
consistent with Fleet's tenant context; independent bookings remain unavailable.
An Ed25519 signature from independently pinned governance keys, approval
reference, owner list, effective/expiry dates and exact environment are required.
Both processes require identical policy fingerprints. Service-owned PostgreSQL
high-water marks reject policy rollback and same-version rewrite at policy pin.
Approval changes require coordinated process rollout; expiry is checked on reads.

Analytics supports configured minimum cohorts, small-cell and complementary
suppression, approved deterministic rounding, restricted filter combinations,
fixed windows, staff query budgets and immutable query auditing. A durable
cross-user/report release journal freezes identical queries and refuses different
queries over intersecting UTC-day partitions within the exact scope and privacy
domain. It never stores raw source cells in the release cache. These conservative
controls are not differential privacy or certification against every possible
cross-scope inference. A production cohort source and owner review are still
required. Test thresholds and grants exist only in ephemeral TEST_ONLY fixtures;
production builds cannot accept them. Nothing registers a production Analytics
disclosure route. Existing unscoped reports are not treated as scoped cohorts.

## D. Test evidence

Local environment: Go 1.26.9, PostgreSQL 18 and a real localhost Redis 8.8.0.
Tests use explicitly disposable test databases; fixtures create/drop isolated
schemas. No production credentials, data, notification recipients or callbacks
were used.

| Executed suite | Passing cases (includes subtests) | Failed | Skipped |
|---|---:|---:|---:|
| Admin + Taxi owner snapshot + policy + Analytics privacy + shared contract | 159 | 0 | 0 |
| Public Gateway + Partner Web + HMAC v2 | 300 | 0 | 0 |
| Dispatch marketplace + trip + securehandover | 411 | 0 | 0 |
| Selected real PostgreSQL secure recipient/custody/relay-upgrade scenarios | 11 | 0 | 0 |

The Admin suite starts the **actual Taxi main binary** and passes an HQ request
over a real verified mTLS connection to the bridge, then HMAC v2 to Taxi, then
real owner PostgreSQL queries. It checks both durable audit records. A separate
owner test exercises Taxi's migration runner twice, scoped tied-timestamp
pagination, cursor tampering/rebinding, eight-way nonce race, foreign scopes,
minimization and audit failure.

Dispatch tests cover duplicate/concurrent relay delivery claims, ambiguous
timeouts held without another POST, dead-letter replay refusal, expired leases,
failed durable confirmation remaining PENDING, legacy fence backfill, denied
rollback, current active-trip geometry and the real Redis allowance shared by
two independent clients. Existing PostgreSQL handover tests include unsupported
phone-only recipients, different receiver/sender accounts, receiver/contact
changes, OTP/QR consumption races, resend/verify races, evidence-gated completion
and immutable evidence. Test transports/fixtures are identified as such; they
do not certify live Notification or Media deployments.

Commands and gate scripts are in `.github/workflows/admin-owner-closure.yml` and
`.github/workflows/admin-bridge.yml`. JSON evidence is uploaded by CI; mandatory
gate scripts reject failed, skipped or missing named scenarios. The new workflow
runs on PostgreSQL 16 and Redis 7 with Go's race detector. Local Windows has no
race-enabled C toolchain or Docker, so those results must come from actual CI.

OpenAPI 3.1, eleven JSON Schemas, deterministic checksums, old-v1 compatibility,
actual owner wire response and deployment-source validation passed locally.
`go vet` passed for the changed service packages. Admin `govulncheck` found zero
reachable vulnerabilities and zero vulnerable imported packages; it reported
one uncalled required-module advisory. This is not a scan of every owner image.

The first broad Dispatch run exposed the canonical table-name defect and a
failed local Redis startup; those failures were corrected before the passing
runs above. Existing repository-wide baseline failures (module/static checks,
Media rollback, Customer secret gate and provider Flutter checks) are not waived
by these focused results. Final CI outcomes belong to the actual run/SHA listed
in the PR; local results do not prove staging or production certification.

## E. Security and reliability findings

* Existing mTLS workload pinning, independent staff verification, HMAC v2,
  principal revocation, replay claims, rate limits and immutable audit tests pass.
* Owner credentials are dedicated secret-file inputs. There are no redirects,
  proxy environment inheritance, arbitrary destination selectors or automatic
  transport retries in the Taxi client. HQ authentication headers never reach it.
* Public edge tests configure every public owner, verify normalized/encoded
  private paths never reach any owner, and include a successful public-route
  positive control. Actual deployed edge/network isolation still needs testing.
* SQL enforces market + zone + tenant at the owner. Cursor and assertion checks
  enforce the delegated reader. Both owner and bridge audit before disclosure.
* Dispatch previously re-sent UNKNOWN_OUTCOME rows. Migration 070 durably fences
  uncertain legacy work; workers and operator replay cannot clear that fence.
  The synchronous path now claims the row before HTTP so a worker and duplicate
  provider tap cannot both deliver it. Failed persistence cannot acknowledge a
  confirmed result. This repairs source behavior without an admin workaround.
* Uncertain decisions now stay unresolved until authoritative reconciliation;
  GET-only immutable Fleet receipts now reconcile standard/private assignments
  and marketplace accepts/declines against the exact original action identity.
  Missing or mismatched historical proof remains fenced; no POST is repeated.
  See `provider-decision-readback.md` for native evidence limitations.
* Recovery and evidence remain gated. No new path reads secret credentials,
  raw protected documents or custody evidence. There is no HQ handover bypass.

## F. Blockers and closure evidence

| Blocker | Impact / owner | Required action and evidence | Disabled? |
|---|---|---|---|
| BLOCKED_POLICY: missing scope provenance | Customer; Dispatch/Hauling missions and provider directories; Fleet geography | Owner-defined authoritative mapping and enforcement, including historical/unmapped rows; cross-market/tenant tests; initial approval of draft matrix | Yes, adapters implemented; no approved production authorization contexts |
| BLOCKED_POLICY: missing approved reporting policy | Analytics reports | Approve draft suppression values and privacy domains; implement owner-scoped cohorts and reconcile source tests for all ten reports | Yes; suppression framework tested |
| BLOCKED_POLICY: Taxi approved access tuples absent | Taxi corporate reads | Owner and governance review/sign exact canonical tuples, roles, purposes and key enrollment; target-environment verification | Yes; no production grants exist |
| No supported HQ executable command contracts | Customer, Taxi, Dispatch, Hauling/Fleet | Versioned owner mutation and receipt/read-back contracts, approvals, failure/timeout/reconciliation tests, per-family flags and write kill switch | Yes |
| Legacy decisions without immutable proof | Dispatch → Fleet | All supported native decision families have exact-action GET readbacks; historical rows without provable original intent and private evidence remain unresolved | Unprovable legacy work stays fenced |
| Protected admin evidence access absent | Media + owner services | Case/subject/permission scoped short-lived grants and denial/revocation tests | Yes |
| Recipient channel certification incomplete | Customer/Dispatch/Notification | Supported verified-channel contracts and real device/delivery evidence; wrong-recipient and network interruption tests | Unsupported channels fail closed |
| Private owner connectivity not provisioned | Platform infrastructure | Approved owner-only ingress/egress path and secret mounts; verify no Gateway or domain DB exposure | Taxi reads disabled by default |
| Live PR13 cutover not performed | Platform + callback producers | Inventory/redirection of actual senders, single worker ownership and real edge/process acceptance | No deployment/cutover performed |
| Full requested journey matrix unavailable | All five owners + real HQ | Complete adapters and all requested owner journeys, then real HQ/staging tests | No certification claimed |

## G. Verdict

**IMPLEMENTATION_INCOMPLETE.** Policy enforcement, Analytics suppression framework,
the five-owner resource adapters and Dispatch/Fleet receipt reconciliation have
real integration evidence. There are 21 resource kinds and 42 private Bridge read
paths. Production disclosure remains disabled pending owner approval. The ten
Analytics cohort adapters, executable HQ interventions, richer owner detail and
Media case grants remain engineering work; policy gaps do not complete them. This is not BACKEND_INTEGRATION_READY,
STAGING_CERTIFIED or END_TO_END_CERTIFIED. CI passing for this incremental change
would not change that overall verdict.

### Current custody and authority validation

The v2 Dispatch/Hauling custody detail contracts expose bounded canonical stops,
versioned manifests/packages, holdings and transfer items; legacy reads require
the exact frozen manifest version in native state. List responses contain only
metadata. Challenge data, raw parcel JSON, business references and evidence URLs
are excluded. Existing v1 approvals cannot authorize the larger disclosure.
See `owner-resource-contexts.md` and each owner's
`docs/openapi/admin-owner-resources.openapi.json`.

Local verification for this increment: 70 source authority scenarios across all
five actual migration suites and 203 Bridge/Taxi/privacy/policy scenarios passed
with zero skipped test cases. Actual custody HTTP output from both owners passed
independent schema validation. Windows vet passed; Linux race checks run in the
mandatory owner workflow. The preceding committed baseline passed Section 31
mandatory acceptance, owner safety, Bridge, Vendor and Gateway CI. Customer V4
passed all service test gates but exposed route audit defects, repaired in this
increment; fresh current-commit CI is still required.

Dispatch incident, delivery-recovery and return metadata now use three separate
read capabilities and exact native Trip parent validation. Conflicting booking,
customer or attempt links deny access, including dependent return workflows.
Protected notes, route snapshots, wallet references and recipient data are
omitted. These workflows have no complete mutation counter; source versions
remain unknown. `TestDispatchWorkflowReadsBindCanonicalTripAndRejectCrossLinks`
uses actual migrations, signed source contexts and canonical organization checks.

Hauling physical truck facts require `hauling.admin-truck-capability.v2`; old v1
approvals cannot authorize the larger view. Unknown/null remains distinct from
recorded false and zero revisions; no catalogue-derived fit or eligibility is
introduced. Hauling recovery reads preserve native case revisions and latest
audit timestamps while excluding private audit bodies, actor/evidence/transfer
and financial fields. Canonical planning ownership and exact commerce-mission
links are validated before resolving any signed authorization context.

All six acceptance workflows passed on committed baseline
`528b19c6cd91c5a3045c6b80ae3d0418c85ed8ca`, including Customer certification,
Section 31, owner race/PostgreSQL/Redis, Bridge/container, Vendor and
Partner/Gateway. Fresh CI is required for subsequent engineering increments.

Analytics now has a PostgreSQL-backed native event lineage validator and normal
migration 006 for producer-owned authorization context history. Tests exercise
the actual native ingest/projector and independent source-owner context import,
expiry/revocation, cross-owner/report/tenant rejection, unit deduplication,
payload-tamper detection and aggregate-bucket rejection. This is supporting
engineering: complete source membership/coverage proofs, ten report reducers
and the HQ facade remain unimplemented. No Analytics report is enabled.

Local validation after lineage support: 206 Bridge/Taxi/privacy/policy cases,
including 24 Analytics privacy/lineage scenarios, passed with no skipped tests.
The preceding five-owner suite remains 70 scenarios across actual migrations.
Native cohort field bindings are typed; provider/branch UUID encodings normalize
to their canonical identifiers, while source-entity identities retain their
native type. Hashes are internal deduplication identifiers, not anonymization.

The mission-success source adapter now reads a bounded approved subset directly
from canonical Analytics events and signed producer contexts, applies exact
native authority/product/time filters, and counts distinct canonical units per
cell. It feeds the existing disclosure service in real PostgreSQL tests. Six
native completion events for one provider produce a cohort of one. It is not
registered at runtime and always reports partial membership, so suppression
withholds every value. Complete membership exports/coverage and remaining
report adapters are still engineering requirements; no reporting readiness is
claimed from this partial source implementation.

After the first native source integration, 209 local Bridge/Taxi/privacy/policy
cases passed with zero skipped tests; Analytics contributes 27 privacy/lineage
and source scenarios. Changed-package vet passed. Native source counts remain
withheld because membership completeness is deliberately unproven.
