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
| Corporate Taxi ride list/detail | `taxi_bookings` joined to `markets` and `operating_zones`; `taxi.corporate-rides.v1` | Exact approved tuple; signed staff tenant must match Fleet-origin execution tenant; both owner and bridge enforce | Minimized operational state | Taxi + Fleet + IAM/Privacy | Signed approval, exact roles/purposes/tuples and private owner connectivity |
| Taxi provider/vehicle eligibility/documents | Taxi provider/vehicle compliance dimensions | Owner-scoped provider/asset context; separate protected-document grants | Personal/compliance/evidence | Taxi + Media + Privacy | Provider-wide geographic/tenant authority; short-lived case grants |
| Dispatch rider/mission/timeline | Canonical provider/trip/assignment/custody records; `organization_id` distinct from `settlement_organization_id` | Owner execution facts + approved geography + correct Fleet tenant context | Operational, personal and custody | Dispatch + Fleet + Privacy | Geographic lineage and minimized owner snapshot contract |
| Dispatch handover/recovery | Securehandover, deliveryrecovery, custody and immutable outboxes | Current version/assignment/custody, supported verified recipient channel, exact command approval | Security credentials and protected evidence | Dispatch + Customer/Notification + Media | Approved recovery policy; authoritative decision receipts/reconciliation; no OTP/QR disclosure or custody bypass |
| Hauling provider/truck/cargo | Hauling provider/truck/mission and authority restriction records | Owner eligibility/suitability/custody plus approved tuple | Operational/compliance/cargo | Hauling + Fleet + Privacy | Geographic source facts, read contracts, cargo/recovery approval rules |
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
