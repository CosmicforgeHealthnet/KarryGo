# Section 19: compliance, safety and proof

## Ownership and routes

Fleet owns organization-scoped requirements, evaluations, restrictions, safety reports, incidents, corrective actions, proof policies and immutable proof records. Tenant routes are under `/fleet-partner/v1`; the exact paths and permission/flag metadata are in `openapi/section19-compliance-safety-proof.openapi.json`.

Dispatch and Hauling remain canonical execution authorities. Fleet propagates restrictions to `/api/v1/internal/restrictions` and `/api/v1/hauling/internal/restrictions` using HMAC v2, caller allowlisting, timestamp and durable nonce replay protection. Each service supports apply, operation read-back, clear and clear read-back. Fleet records every operation and immutable attempt. An ambiguous mutation is read back before it is retried. Local clearance remains `CLEAR_PENDING` until every required authority confirms it.

## State and safety rules

Requirements move through `DRAFT`, `ACTIVE`, `INACTIVE`, and `ARCHIVED`. Evaluation is server-side and preserves source version; missing or gapped qualification, training, inspection, or document evidence fails closed. Restrictions are separate from warnings and remain enforceable even if creation flags are later disabled.

Safety reports do not automatically punish a provider. Anonymous API views suppress reporter references; protected investigation data remains permissioned. Incident evidence is a validated Media File Service reference—Fleet stores no bytes. Corrective-action completion and independent verification are separate transitions.

Proof policies are versioned and publication creates immutable snapshots. Proof records bind to a snapshot, use opaque Media references, hash sensitive submitted values with `FLEET_PROOF_HMAC_SECRET`, and exclude exact coordinates and hashes from ordinary responses. Proof cannot establish live tracking or mutate canonical trip status. Custody stays in the Section 15 custody authority.

## Controls and operations

Writes require the semantic permissions shown in OpenAPI, active tenant/membership scope, `Idempotency-Key`, and expected version on state-sensitive commands. Flags default false: `COMPLIANCE_ENABLED`, `SAFETY_REPORTS_ENABLED`, `INCIDENTS_ENABLED`, `PROOF_POLICIES_ENABLED`, `PROOF_RECORDS_ENABLED`, `RESTRICTION_PROPAGATION_ENABLED`, and `SECTION19_WORKERS_ENABLED`. The global Fleet kill switch stops integration writes/workers without disabling restriction reads.

Workers use the shared PostgreSQL queue with `SKIP LOCKED`, owner leases, heartbeat, expiry recovery, bounded retry, immutable attempts, dead-letter and permissioned replay. Roll out migrations first, configure both authority credentials and the proof HMAC secret, then enable reads/writes and workers progressively. Roll back flags first; paired down migrations are intended only for controlled deployment rollback after workers are stopped.
