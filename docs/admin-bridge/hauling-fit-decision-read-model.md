# Hauling historical fit-decision read model

Status: implemented and tested software; **BLOCKED_POLICY**, default disabled.
Permission `hauling.fit_decisions.read`; separate source contract
`hauling.admin-fit-decision.v1`. This is R3 restricted operator candidate data.

Native list/detail: `/internal/hauling/v1/admin/resources/fit-decisions` and
`/{id}`. Bridge list/detail: `/internal/karryam-admin/v1/hauling/resources/fit-decisions`
and `/{id}`. Existing independent owner and Bridge resource flags apply.
No production roles, scope tuples or source approvals are supplied.

The source validates actual `hauling_match_decisions` booking/provider parents,
optional truck and its actual provider, and any captured commerce mission against
the same booking. Planning organization and winner organization must agree when
both are recorded. Native planning organization is also the canonical tenant;
settlement organization is excluded. Missing market/zone requires a current,
independently signed owner allocation. Staff delegation, source authorization and
approved governance must all agree. Reparenting invalidates an old allocation.

Responses expose the stored ELIGIBLE/REJECTED/SELECTED outcome, shadow flag,
captured policy revision, captured capability counter and opaque native references.
They exclude reasons, raw evidence, distance, addresses, contacts and document URLs.
A captured capability counter is not a complete row mutation version:
`source_version` stays null. An empty policy revision remains unknown. Historical
ELIGIBLE is not current eligibility, permission to assign, or a recalculated fit;
shadow observations remain observational. No fit or restriction command is added.

`TestHaulingFitDecisionReadsBindHistoricalCandidateParents` uses the native
`truckcapability.Repository.RecordDecisions`, real owner migrations, independently
signed TEST_ONLY contexts and native signed HTTP. It checks private-field exclusion,
wrong tenants/contracts, foreign truck/mission links, orphan records, contradictory
organizations and invalidation of existing authorization after reparenting.
The captured HTTP response is independently schema-validated in mandatory CI.

Hauling, Fleet/IAM and Security/Privacy must approve candidate disclosure, scopes,
roles, purpose and retention. These governance decisions remain **BLOCKED_POLICY**.
