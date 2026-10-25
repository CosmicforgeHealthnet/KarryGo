# Secure Admin Section 12 — Fleet implementation evidence

**Software status: `SECTION_12_BACKEND_FUNCTIONALLY_COMPLETE`.** This status covers the Fleet-owned backend boundary on `staging`. It does not activate Headquarters access, deploy a service, enroll a workload, or create a production policy grant.

## Implemented owner behavior

- Fleet exposes fixed, source-backed list/detail contracts for organization standing and onboarding, branches and departments, minimized corporate staff and role bindings, managed-provider membership, assets and active assignments, verification/document expiry, contracts, quotations, approvals, minimized CRM contact posture, Wallet-owned transaction references, private and recurring orders, private manifests, route planning and optimization, operational alerts, incidents, SLA measurements, immutable audit history, shipments, private assignments, execution links, and investigations.
- Every row is bound to Fleet's canonical organization tenant. Market and zone still require a current, independently signed owner allocation; a request cannot supply a different tenant to cross an organization boundary.
- Company IAM records are visibility inputs only. They never enroll an HQ user, widen delegated Headquarters scope, or replace Fleet's native role/permission checks.
- Sensitive manifests use the separate `fleet.private_manifests.read` capability. Staff names, identity-provider identifiers, emails, phone numbers, street addresses, free-text incident/case material, audit reasons, actor identities, credentials, route snapshots, and raw document bytes are outside the allowlist.
- Existing source-owned investigation commands now cover Fleet organizations, assets, contracts, corporate incidents, route plans/optimization requests, and SLA measurements in addition to shipments and private execution. They create and transition an append-only Fleet investigation case; they do not mutate the investigated operational record.
- Investigation open/finding/close retains exact source scope, expected case version, idempotency, immutable command history, authoritative readback, separate approval for closure, and Media case-access termination when the case closes.
- No contract, route, assignment, shipment, Wallet, custody, verification, or approval state setter was added. Historical eligibility and route results remain observations, not authority for a new assignment or dispatch.

## Contracts and migration

The shared owner profile and Bridge capability registry include the new fixed resource kinds. Private owner OpenAPI, Bridge OpenAPI, capability matrix, schemas, fixtures, and checksums are regenerated deterministically. All new capabilities are discovery-visible but remain disabled unless both the owner read switch and Bridge read switch are enabled and an approved signed policy/source allocation exists.

Migration `000084_admin_investigation_subjects` extends only the existing Fleet investigation foreign-key dictionary and category constraints. Its down migration refuses to remove the extension while any expanded-subject case exists. No live migration was applied.

## Verification evidence

Disposable PostgreSQL tests compile every projection against all Fleet migrations and exercise authoritative organization binding, minimized disclosure, dedicated manifest permission, expanded investigation state transitions, cross-tenant denial, stale versions, concurrent commands, idempotent replay, approval separation, immutable history, signed HMAC/replay rejection, and Media case proof. Bridge tests cover fixed routing, policy/delegation intersection, HMAC/mTLS transport, deny-by-default behavior, and contract compatibility.

The full Fleet service gate, shared owner/command suites, contract validation, and repository acceptance workflows are recorded in the final Sections 12–15 closure report on the exact pushed staging SHA.

## External activation gates

Live HQ staff enrollment, signed market/zone/organization allocations, production policy revisions, HMAC/mTLS credentials, Media lifetime/disclosure approval, and high-risk approvers remain external governance inputs. Missing or ambiguous inputs fail closed. Fleet does not gain authority over Wallet, Verification, Dispatch, Taxi, Hauling, Media, or production deployment through this implementation.
