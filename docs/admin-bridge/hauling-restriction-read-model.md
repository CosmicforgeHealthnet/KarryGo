# Hauling restriction record reads

Status: tested, default disabled, production BLOCKED_POLICY.
Permission: `hauling.restrictions.read`.
Source contract: `hauling.admin-authority-restriction.v1`.

Native list/detail use `/internal/hauling/v1/admin/resources/restrictions` and
`/{id}`. The Bridge uses `/internal/karryam-admin/v1/hauling/resources/restrictions`
and detail. Existing separate owner/Bridge resource flags and credentials apply.
A current independently signed source context plus dedicated signed governance
rule are required; broad prior resource approvals cannot enable this view.

Only PROVIDER and ASSET subjects are supported. A provider must exist and have an
ACTIVE native organization/provider link matching the restriction's organization.
A truck must exist, have an ACTIVE organization/truck link, and its actual current
provider must have the same ACTIVE organization/provider membership. Membership
identity and link revisions are in the authorization fingerprint. Revocation,
replacement or reparenting invalidates the old allocation. Organization/tenant
come from this canonical native relationship; market/zone still require an
independent owner allocation. No settlement, GPS or caller tenant is authority.
Unresolved historical subjects, ORGANIZATION/SERVICE_CAPABILITY/SHIPMENT subjects
and arbitrary text identifiers remain unavailable under this source contract.

Output is stored APPLIED/CLEARED state, subject type, bounded reason code, opaque
subject reference, native restriction revision, original-source revision and
native timestamps. Source references, operation fingerprints, profile/contact/
plate/inspection material and history bodies are excluded. A native Apply/Clear
changes the observed row revision without changing an otherwise stable ownership
binding. This view grants no command authority and is not a complete global
eligibility or restriction evaluation.

Native Hauling booking/availability paths sometimes evaluate provider/asset
restrictions across organizations. A scoped HQ command must not silently apply
or clear a broader hold. Apply/clear therefore remain unsupported in this HQ
adapter; introducing them needs reviewed owner command/approval/receipt and
scope semantics. This increment changes no native enforcement behavior.

Validation uses every actual Hauling migration, native restriction Apply/Clear,
real signed owner HTTP, both subject kinds, unmapped/external subject denial,
foreign tenant/organization, revoked provider/truck links and replaced membership.
An independent schema validates the actual source response. Initial roles,
purposes, geographic allocation, privacy/retention and source-contract approval
remain BLOCKED_POLICY; no production grants or mappings are provided.
