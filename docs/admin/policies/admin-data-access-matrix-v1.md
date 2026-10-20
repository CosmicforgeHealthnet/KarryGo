# Admin data access matrix v1

**DRAFT — PENDING OWNER APPROVAL**

All proposed disclosure rules below remain **BLOCKED_POLICY**. Existing approved
service-to-service contracts remain authoritative, but none grants new HQ
production access by itself. No production roles, purpose codes or scope tuples
are invented by this matrix.

| Capability | Canonical source fields / contract | Proposed access rule | Privacy classification | Responsible owner | Dependencies / outstanding approval |
|---|---|---|---|---|---|
| Network reports (all ten) | Analytics dictionary and source projection watermarks | Staff + approved tuple + owner-resolved cohort; disclosure journal/suppression | Aggregate, potentially identifying in small cells | Analytics + Privacy + affected source owner | Geographic/tenant lineage, distinct-cohort definitions, all suppression values and composition policy |
| Customer directory/detail | `customers.id/status/onboarding_status/updated_at` | Owner-resolved access context; no authorization from query tenant alone | Personal identity, minimized by field purpose | Customer + IAM + Privacy | Market/zone/tenant mapping contract and approved search/detail field list |
| Customer consent/verification/privacy workflows | Owner consent/preferences/verification records | Action-specific role/purpose, source ownership, version and approval | Sensitive personal/compliance | Customer + verification/consent owner + Privacy | Typed workflow contract and approved lifecycle/privacy rules |
| Customer owner review intake/readback | `customer_admin_review_state.customer_id/version/status`, actual `customers.id`, immutable `customer_admin_review_requests` | Independent owner root allocation + approved staff scope/purpose and request/read capability; native review revision, exact original identity and current scoped readback. No lifecycle/Trip/Wallet mutation | Minimized operational intake metadata | Customer + IAM + Privacy | BLOCKED_POLICY: request field/purpose/scope, work-item handling/closure/retention approval. Owner API, typed SDK and durable five-scope Bridge intake/readback implemented and tested; real HQ/target-environment certification pending |
| Corporate Taxi ride list/detail | `taxi_bookings` joined to `markets` and `operating_zones`; `taxi.corporate-rides.v1` | Exact approved tuple; signed staff tenant must match Fleet-origin execution tenant; both owner and bridge enforce | Minimized operational state | Taxi + Fleet + IAM/Privacy | Signed approval, exact roles/purposes/tuples and private owner connectivity |
| Taxi passenger/goods recovery reads | `taxi_recovery_cases` native case version, exact booking/goods parent; booking market/zone/organization; goods source/customer identity | Current owner allocation + staff + approved governance. Validate zone.market; no goods quote/GPS/vendor-derived scope | Minimized operational recovery metadata | Taxi + Fleet + IAM/Privacy | BLOCKED_POLICY: initial field/purpose/scope/capability approval; missing goods dimensions remain owner allocated. No intervention or Media authority |
| Taxi provider/vehicle eligibility/documents | Taxi provider/vehicle compliance dimensions | Owner-scoped provider/asset context; separate protected-document grants | Personal/compliance/evidence | Taxi + Media + Privacy | Provider-wide geographic/tenant authority; short-lived case grants |
| Dispatch rider/mission/timeline | Canonical provider/trip/assignment/custody records; `organization_id` distinct from `settlement_organization_id` | Owner execution facts + approved geography + correct Fleet tenant context | Operational, personal and custody | Dispatch + Fleet + Privacy | Geographic lineage and minimized owner snapshot contract |
| Dispatch handover/recovery | Securehandover, deliveryrecovery, custody and immutable outboxes | Current version/assignment/custody, supported verified recipient channel, exact command approval | Security credentials and protected evidence | Dispatch + Customer/Notification + Media | Approved recovery policy; authoritative decision receipts/reconciliation; no OTP/QR disclosure or custody bypass |
| Dispatch incident metadata | `dispatch_delivery_incidents.trip_id` → `trips.id/organization_id/customer_id/execution_source`; incident reason/state/strategy/recovery result and attempt count | Exact signed workflow context + approved staff tuple; parent Trip organization/tenant must match. No note, credentials, money or direct recipient identifiers | Minimized security/operations metadata | Dispatch + Fleet + Privacy | BLOCKED_POLICY: initial field/purpose/capability approval and native owner allocation |
| Dispatch delivery recovery metadata | `dispatch_delivery_recovery_actions` → same actual Trip, booking, customer and delivery attempt | Reject contradictory historical parent links; exact signed workflow context + approved tuple. Captured Trip versions never authorize writes | Minimized operational metadata | Dispatch + Customer + Privacy | BLOCKED_POLICY: read permission approval; intervention remains separate and unimplemented |
| Dispatch return metadata | `dispatch_return_workflows.original_trip_id/recovery_action_id` → same canonical recovery subject | Exact signed workflow context + approved tuple; omit route snapshots, parcel arrays, wallet and evidence fields | Minimized custody/operations metadata | Dispatch + Fleet + Privacy | BLOCKED_POLICY: read approval; native return/custody guards remain authoritative |
| Hauling provider/truck/cargo | Hauling provider/truck/mission and authority restriction records | Owner eligibility/suitability/custody plus approved tuple | Operational/compliance/cargo | Hauling + Fleet + Privacy | Geographic source facts, read contracts, cargo/recovery approval rules |
| Hauling physical truck capability | Native `trucks` payload/body/opening dimensions, protection/equipment flags and partial capability counters; `hauling.admin-truck-capability.v2` | Exact signed owner context + approved tuple + new v2 disclosure approval; unknown stays null, no inferred fit or eligibility | Minimized operational/compliance facts | Hauling + Fleet + Privacy | BLOCKED_POLICY: new source-contract field/purpose approval. Inspection/certification documents and cargo-policy details remain excluded |
| Hauling recovery metadata | `hauling_recovery_cases.booking_id/version`, actual `haulage_bookings` planning owner and `commerce_missions.booking_id/mission_ref` | Exact signed case context and approved tuple; reject contradictory planning/mission links; settlement never authorizes | Minimized custody/operations metadata | Hauling + Fleet + Privacy | BLOCKED_POLICY: field/purpose/capability approval; executable recovery remains separate |
| Fleet organization/private shipment links | `fleet_partner_organizations.id`, trusted `tenant.Context.OrganizationID`, actual branch and execution links | Existing canonical membership/tenant resolution intersected with HQ policy; preserve planning/execution distinction | Corporate-private | Fleet + IAM/Privacy | HQ geographic mapping and source-specific grants; body/query tenant cannot override trusted context |
| Protected documents/evidence | Media-owned subject/case grants | Separate permission, purpose, approval and short-lived grant; audit/revocation | Highly sensitive | Media + owning domain + Privacy | Owner grant API and evidence-denial tests |

The JSON companion records the same approval status for automation. Draft policy
envelopes contain empty rules and no production values. Runtime requires a
valid owner-signed `APPROVED` envelope pinned to governance keys, the correct
production environment, a live effective interval and a monotonic immutable
revision. `TEST_ONLY` approvals are accepted only by explicit test constructors
or the integration-test build; production builds cannot enable them via an env
flag. Feature flags cannot bypass policy or source-owner facts.

Approval evidence must identify the responsible owner(s), exact version/checksum,
canonical field contracts, privacy review and outstanding infrastructure gates.
No administrative operator can approve their own request by supplying these
fields in the request body.

Engineering continuation (2026-10-19): [source-owned resource context contracts](../../admin-bridge/owner-resource-contexts.md) provide signed owner remediation and deny-by-default typed reads. This draft supplies no context signature or production allocation. The sixteen-kind initial approval matrix remains **BLOCKED_POLICY**; Analytics thresholds/cohorts, Media grants and command approvals remain separate requirements.


`analytics.scoped.mission-success.read` is a separate counts-only framework
capability owned by Analytics + Privacy + source owners. It binds native event
version/hash and independent current producer context to delegated five-scope
access. All counts are withheld while coverage is partial. Initial unit, scope,
suppression/composition and freshness approvals remain **BLOCKED_POLICY**; no
production grants exist. Complete membership/publication and rate reduction are
engineering dependencies. The JSON matrix records the approval-ready entry.


`dispatch.handover.read` (Dispatch + Security/Privacy owner): stored handover status,
stage and notification state only, with parent Trip reference. Dedicated source
contract `dispatch.admin-handover-state.v1` is required; all grants remain empty
and **BLOCKED_POLICY**. Canonical Trip planning organization/tenant and exact
booking/mission/group/stop/frozen-manifest links must agree. Market/zone still
require independent owner allocation. No credential-validity, custody completion,
full revision, challenge/contact/evidence material or intervention is supplied.
Roles, purposes and retention require owner review; see JSON matrix dependencies.
