# Source-owned administrative resource contexts v1

Engineering implementation; production governance remains **DRAFT — PENDING OWNER APPROVAL**. These contracts add no production grant, owner signature, threshold or enabled feature flag. They are not a completion claim for Sections 6–10.

The existing source services embed these adapters; Admin never connects to their databases. The Bridge uses fixed owner origins, dedicated HMAC v2 credentials and bounded typed responses. All new owner and Bridge read flags default to false. Fleet routes remain behind its native enrolled service-credential, nonce and rate-limit middleware as well as the new checks.

## Enforcement and owner remediation

Every disclosure requires delegated staff product/permission/role, exact market/zone/organization/tenant and purpose, a valid independently signed governance policy, and a current source-owner authorization context. Both sides pin the policy revision. A policy cannot create a source record or replace source facts. Missing, expired, revoked, stale or contradictory bindings deny access. Source-owner context signatures use a separate key allowlist and the signing domain `karryam-owner-authorization-context-v1\n`; they cannot be substituted with governance signatures or Bridge HMAC credentials.

An authorization context allocates reviewed administrative access. It does not rewrite customer ownership, corporate planning, Fleet tenancy, settlement organization, execution state or geography. Nonempty canonical source dimensions must agree. For records without source geography, no coordinates or service region are treated as permission. An owner must review the proposed allocation and provenance before signing it; until then it is BLOCKED_POLICY.

The independent `owner-authorization-context-publisher` workload can GET `<owner-admin-base>/authorization-contexts/bindings/<kind>/<reference>?purpose=<review-case>` to obtain current known dimensions, source contract and binding fingerprint. This fixed source query accepts no proposed scope, records immutable preparation evidence before returning, and labels the response `DRAFT_PENDING_OWNER_APPROVAL`. The HQ read principal cannot use it. It gives the owner a preparation workflow without sharing database access with HQ.

The owner reviews and separately signs a bounded `admincontext.Approval` containing the resource identity, fingerprint, source contract, exact proposed scope, capability, provenance/approval references, effective/expiry times and increasing revision. The publisher POSTs the signed envelope to `<owner-admin-base>/authorization-contexts`. The source revalidates the live record before importing. This endpoint cannot sign or self-approve. Production builds reject TEST_ONLY approvals. Revocation is a higher signed revision with state REVOKED. History and preparation evidence are append-only; paired down migrations refuse to erase evidence.

List candidates come from the source-owned context index and are individually reauthorized. Coverage is always `CONTEXT_SCOPED_PARTIAL`, including when no business record is missing from a page. Cursors are encrypted, expire after ten minutes, and bind staff, role, purpose, all scope dimensions, policy revision, resource definition and time window. Detail reads do not apply the list creation-time filter. Current detail paths accept one opaque path segment; references containing slashes require an additional transport contract before detail lookup can be supported.

Output excludes names, contact details, coordinates, free text, document URLs, credentials, OTP/QR material and money. Unknown standing/consent/version fields are null, never a default success. Actual trusted-contact sharing switches are not represented as legal consent. Content ETags do not authorize lifecycle mutations. Native version counters are exposed only where explicitly mapped, not guessed from a future column named version. Timeline reads are bounded at 200 events and mark truncation.

## Fixed read dictionary and initial approval matrix

All rows below are **BLOCKED_POLICY** for production, pending source-owner allocation, governance role/purpose/scope approval, privacy review and credential enrollment. Privacy classification: minimized operational metadata; trusted-contact switches and custody references remain restricted. Responsible owner is the service named in the first column; governance and privacy owners must be assigned by the organization, not invented here.

| Responsible source owner | Kind | Actual canonical binding/source facts | Outstanding dependency |
|---|---|---|---|
| Customer | customers | customers.id + creation epoch; onboarding/status | Owner-reviewed geographic and tenant allocation; verification/legal consent source remains unknown |
| Customer | consents | emergency_contacts.id/customer_id; active/trip-sharing/emergency-alert switches and version | Parent-customer allocation; contact fields excluded; legal consent approval unresolved |
| Customer | booking-references | movement projection authority:source_ref, customer_id, source_version | Projection is not execution authority; customer/source relationship review |
| Taxi | providers | taxi_providers identity/creation; separate operational-state availability | Owner allocation; global eligibility is unknown |
| Taxi | vehicles | vehicles.provider_id; seats/status | Parent-provider allocation; unsupported compliance fields remain unknown |
| Taxi | independent-rides | actual markets.code and operating_zones.code with matching market foreign key; no corporate organization | Independent allocation; corporate rows excluded |
| Dispatch | riders | providers identity/creation; provider_availability and verification_steps vehicle/licence fields | Owner allocation; global eligibility is unknown |
| Dispatch | motorcycles | bikes.provider_id, verification status | Parent-provider allocation; capacity/compliance unavailable |
| Dispatch | missions | trips.organization_id frozen planning tenant; execution_source/mode; Fleet assignment/response references | Approved geographic allocation; no settlement fallback or inferred bike reference |
| Dispatch | custody-missions | custody_mission_state.tenant_ref must equal custody_missions.tenant_ref; native state version | Authority/custody review; no challenge secrets |
| Hauling | providers | truck_providers identity/creation; hauling_provider_availability.state | Owner allocation; unsupported verification/eligibility unknown |
| Hauling | trucks | trucks.provider_id/capacity_kg/status | Parent-provider allocation; unsupported compliance unknown |
| Hauling | missions | corporate_organization_id or winner_organization_id only when unambiguous; cargo weight and Fleet links | Contradictory planning owners deny; settlement_organization_id is never an authority fallback |
| Fleet | shipments | shipment.organization_id is canonical Fleet tenant; order/branch links; native version | Explicit market/zone allocation, enrolled read credential |
| Fleet | private-assignments | assignment.organization_id and work-origin/order/shipment links; native version | Explicit market/zone allocation and owner access review |
| Fleet | execution-links | execution operation organization_id, assignment/order/shipment links; native version | Settlement identity remains distinct; explicit market/zone allocation |

Source paths are `/internal/{customer,taxi,dispatch,hauling}/v1/admin` and `/internal/fleet-partner/v1/admin`. Read paths append `/resources/<kind>` and optional `/<reference>`. Bridge paths are `/internal/karryam-admin/v1/<domain>/resources/<kind>` and optional `/<reference>`, where Fleet's domain is `fleet`.

## Configuration and validation

For each compiled owner prefix CUSTOMER, TAXI, DISPATCH, HAULING or FLEET_PARTNER:

- Source: `<PREFIX>_ADMIN_RESOURCE_READ_ENABLED`, `_ADMIN_RESOURCE_SECRET_FILE`, `_ADMIN_CONTEXT_KEYS_FILE`, `_ADMIN_ACCESS_POLICY_FILE`, `_ADMIN_ACCESS_POLICY_KEYS_FILE`.
- Independent publisher: `<PREFIX>_ADMIN_CONTEXT_PUBLISH_ENABLED`, `_ADMIN_CONTEXT_PUBLISH_SECRET_FILE`. Context public keys are independently provisioned; preparation cannot approve a mapping.
- Bridge: `ADMIN_<PREFIX>_RESOURCE_READ_ENABLED`, `ADMIN_<PREFIX>_ORIGIN`, `ADMIN_<PREFIX>_RESOURCE_SECRET_FILE`; governance policy uses the existing ADMIN_ACCESS_POLICY_FILE/KEYS_FILE.

Secrets are base64url files of at least 32 bytes. No production values are supplied. Origins are fixed at startup; proxies and redirects are disabled. Activation requires normal owner migrations and immutable policy revision pins. A requested flag without credentials fails startup; a missing policy or source approval cannot disclose data.

Local PostgreSQL tests execute all five actual migration sets, provider-source projections, all dictionary lookup shapes, independent source signatures, scoped cursors, revocation, source-link changes, cross-replica nonces, audit failure and immutability. A dedicated Hauling test proves even a correctly signed approval cannot override the planning tenant or use settlement identity. Wire/delegation tests cover all sixteen kinds. The real Taxi main binary test prepares/publishes a context and reads through PostgreSQL plus actual Bridge mTLS, then revokes it. These checks do not certify every requested operational field or privileged command.

Remaining engineering includes full Dispatch multistop/custody/incident views, the remaining Fleet decision receipts, legitimate source command adapters, all ten scoped Analytics cohorts/façade and case-bound Media grants. Existing source APIs and draft policies are not evidence that those capabilities are complete.

Source reference hardening: Customer movement projections require the current authoritative customer record. Custody context fingerprints bind the mission anchor authority and custody mode as well as both matching tenant references. Changing any of those source links denies an existing context; a prior context must be prepared/reviewed again against the current binding.

Dispatch custody detail now additionally exposes the authoritative stop sequence, versioned manifest identities, package count/status metadata, current/historical custodian holdings and transfer inventory. Legacy frozen-manifest identities are distinguished from multi-manifest records. The view excludes challenges, credentials, contacts, coordinates, free text, raw evidence and earnings. Each collection is bounded at 100 with explicit truncation; list responses never expand inventory. These stored counters are not inferred eligibility decisions or verified measurements.

This larger disclosure requires **dispatch.admin-custody.v2**, independently approved in both governance policy and source-owner context. Existing v1 approvals fail closed; no production approval is supplied. The parent tenant/authority/custody mode is revalidated, and every child is selected through its actual mission foreign key. A real HTTP/PostgreSQL test covers a second tenant, hidden challenge material, canonical manifest versions, transfers, and bounded/truncated inventory; CI validates its actual serialized response against the pinned independent schema.

Hauling exposes the same bounded custody inventory from its own canonical
`custodyops` tables, under `hauling.custody.read` and the separate
`hauling.admin-custody.v2` source contract. Hauling tenant identifiers remain
source-owned; this does not equate them with a Fleet organization. The native
anchor/state tenant equality and signed binding checks apply before any child
inventory is loaded. Both implementations have actual-migration HTTP tests for
multi-manifest isolation, legacy frozen manifests and old-approval rejection.
There are now 17 source resource kinds and 34 Bridge resource read paths.
Both v2 custody disclosures remain BLOCKED_POLICY and disabled by default.

Dispatch additionally exposes `incidents`, `delivery-recoveries` and `returns`
with separate read capabilities. These follow their actual parent Trip's
`organization_id` as Fleet's canonical planning tenant. A recovery's booking,
customer and delivery-attempt Trip must agree; returns also validate their
recovery's parent links. A historical contradictory row is excluded even with
a signed context. Binding fingerprints include parent identity and native links,
so changing a parent invalidates previous authorization. Captured Trip versions
are not workflow mutation counters: `source_version` remains null. No notes,
route geometry/snapshots, wallet references or challenge/evidence data is exposed.
The dictionary now has 20 resource kinds / 40 private Bridge resource read paths.
These capabilities remain disabled and BLOCKED_POLICY; they execute no recovery.

Hauling truck reads now require `hauling.admin-truck-capability.v2`: physical
payload/body/opening dimensions, equipment/protection flags and native capability
revision counters come exclusively from recorded truck columns. Missing values
remain null, explicit false remains false, and no type/catalogue inference or
eligibility/compliance approval is made. Capability counters are partial evidence,
not the full truck mutation version; `source_version` remains null. Inspection
and certification references/bytes, plate numbers, temperatures and goods-policy
arrays are omitted from this minimized projection. Source approval must cover
the v2 disclosure before any production read.

Hauling `recoveries` uses `hauling.recovery.read`, the actual booking's consistent
planning tenant and exact commerce-mission ownership. Ambiguous or foreign
mission links deny access. Settlement identity is never a tenant substitute.
Case `version` is the native optimistic-concurrency revision; update time comes
from the latest native audit timestamp, with stored lifecycle milestones as the
legacy fallback. Private audit payloads, actor identities, reason free text,
evidence and transfer credentials are omitted. Reads execute no transition and
cannot reassign cargo. There are now 21 kinds / 42 Bridge resource read paths.
All of these new disclosures remain BLOCKED_POLICY and disabled by default.
