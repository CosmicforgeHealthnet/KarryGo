# Market, zone and tenant mapping policy v1

**DRAFT — PENDING OWNER APPROVAL**

Policy status: **BLOCKED_POLICY**. This document grants no production access.
The governing A-to-Z specification is identified by the user, but its PDF was
not supplied. The pasted requirements and verified source contracts inform this
draft; missing policy values are not inferred from either.

## Canonical identifiers found in source

| Owner / record | Authoritative fields | Verified meaning | Unresolved mapping |
|---|---|---|---|
| Customer `customers` | `id`, `status`, `onboarding_status`, `updated_at` | Customer-owned global identity/profile | No market, zone, corporate organization or tenant authorization fields |
| Taxi `taxi_bookings` | `market_id`, `operating_zone_id`, `organization_id`, `is_corporate_managed`, `source_context` | Market and zone foreign keys; organization for actual corporate executions | Independent rides have no corporate tenant. Provider association does not authorize a whole provider profile |
| Taxi `markets`, `operating_zones` | `markets.id/code`; `operating_zones.id/code/market_id` | Taxi's market-code namespace; zone codes are unique within a market | No automatic equivalence to another owner's `market_id` or a Fleet branch |
| Dispatch `trips` | `organization_id`, `settlement_organization_id`, execution/provider/assignment references | Corporate planning and settlement identities are distinct owner facts | Missing authoritative market/zone relation; do not substitute GPS/geocoding or settlement organization |
| Hauling owner records | Organization links, corporate booking/mission references, authority restriction `organization_id` | Owner-validated corporate associations for supported workflows | No authoritative market/zone relation for a global provider/mission directory |
| Fleet `fleet_partner_organizations`, `fleet_partner_branches` | `organizations.id`; `branches.organization_id` | Organization-owned tenant and branch relation | Country code, timezone and coordinates do not define market/zone authorization |
| Fleet `tenant.Context` | `OrganizationID`, trusted actor, permitted branches | Tenant resolves from verified membership/internal claims; body/query/header organization cannot override it | HQ geographic mapping and source-specific administrative grants need review |
| Analytics supply/demand product facts | `market_id`, `zone`, `product_code`, `authority` | Producer-reported dimensions, sometimes empty; other aggregates omit these fields | Producer ID namespace and tenant provenance cannot be guessed or backfilled from query parameters |

Source references: owner migration files; Fleet
`internal/platform/tenant/tenant.go`; Taxi migrations 001/012; Dispatch migration
031; Hauling organization-link/authority migrations; Analytics migrations 003/004.

## Proposed enforcement rules

1. Effective access is the intersection of verified staff grants, an owner-signed
   approved policy, and facts resolved by the authoritative owner. Missing or
   ambiguous facts deny access. Approved policy cannot manufacture missing facts.
2. The policy grants explicit **tuples** of product/market/zone/organization/tenant.
   Independent lists are upper bounds, never permission to form arbitrary
   cross-market or cross-organization combinations. Wildcards and empty grants
   are prohibited by the evaluator.
3. Preserve canonical namespaces. Taxi reads use the joined `markets.code` and
   zone code within that market. Other owner IDs require an explicit source-owned
   mapping contract before they can be compared with these codes.
4. Fleet's canonical tenant is `tenant.Context.OrganizationID`. The implemented
   Taxi read uses this only for an actual Fleet-origin corporate execution with
   the frozen owner `organization_id`; an incoming tenant string cannot override
   it. No general rule equates tenant with organization across all services.
5. Do not infer Customer identity access from a ride, phone, GPS coordinate,
   arbitrary case reference, or user-supplied organization. A minimized owner
   context contract and approved purpose must establish any future relationship.
6. Do not authorize a provider's complete profile from one scoped booking. Do not
   equate planning, execution and settlement ownership. Validate each reference
   against the appropriate source records and existing Fleet membership boundary.
7. Historical mappings need explicit temporal validity, source version and
   provenance. Do not silently rewrite old facts or assign an unapproved default
   market/zone/public tenant. Unmapped coverage is explicit.
8. Signed policy revisions are immutable by checksum and monotonic in each
   consuming owner's database. Expired, altered, older, unapproved or test-only
   production approvals cannot enable disclosure. Staff/session revocation still
   uses the existing Admin boundary on every request.

## Approval and automation

Required reviewers: each affected domain owner, Fleet tenant-model owner,
Platform IAM/security owner and privacy owner. Their approved scope tuples,
canonical contract/field references, role/permission/purpose lists and effective
period must be represented in a signed policy envelope. Pinned governance keys
are separate from HQ staff assertion and service HMAC keys.

No production tuple or key is supplied here. The approval-ready capability matrix
is `admin-data-access-matrix-v1.md` and its JSON companion. Software enforcement
is automatic per request after initial governance approval; there is no UI/API
that turns a staff request into its own approval. Future source mapping ingestion
must use authentic, ordered owner records/contracts, not manual per-request
exceptions or an Admin-owned duplicate domain registry.

Engineering continuation (2026-10-19): [source-owned resource context contracts](../../admin-bridge/owner-resource-contexts.md) provide signed owner remediation and deny-by-default typed reads. This draft supplies no context signature or production allocation. The sixteen-kind initial approval matrix remains **BLOCKED_POLICY**; Analytics thresholds/cohorts, Media grants and command approvals remain separate requirements.
