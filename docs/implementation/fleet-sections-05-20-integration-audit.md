# Fleet Sections 5-20: integration and dependency report

Snapshot 2026-11-13, branch `staging`. Detailed cross-service evidence from
the earlier audit: `spec-v2-audit/B-events-integrations.md`. This report
records what this session changed and what each integration still needs.
No new service-to-service connection was introduced this session.

## 1. Integrations touched this session

| Integration | Owner | Change | Auth | Recovery |
|---|---|---|---|---|
| Managed provider removal / suspension -> owning provider service | Dispatch / Taxi / Hauling | None to the contract. `POST /providers/:id/remove` reuses `ChangeStatus`, which calls `RevokeOrganizationEligibility` on the owning service (best-effort, as before). | HMAC (existing provider contracts) | Membership history and audit are authoritative; provider services also read Fleet's internal membership summary. |
| Provider reactivation | Fleet + Wallet + provider service | Reactivation re-evaluates the existing activation gates: sub-wallet ACTIVE (Wallet projection), provider platform status ACTIVE (`ReadProvider` on the owning service), organization ACTIVE. | existing | A provider whose gates fail stays SUSPENDED and the error lists the failing gates. |
| Unsafe-vehicle declines -> maintenance/safety | Fleet (assets) | The existing reconciler now also reads marketplace assignment declines. | in-process worker | Inbox table makes the sweep idempotent; unmanaged vehicles are recorded so they cannot wedge it. Gated by the existing `SECTION18_WORKERS_ENABLED`, inspections and maintenance flags. |

## 2. Integrations unchanged and their status

| Integration | Status | What is needed |
|---|---|---|
| Fleet <-> Verification (company approval, ADR-006) | Built and deployed by the previous session (`e234978`, `d82599b`, pairing workflow `3b01b40`). | Live certification of an operator decision on staging was not re-run this session. |
| Fleet <-> Wallet (company wallet, sub-wallets, allocations, withdrawals, managed settlement) | Code complete per audit; finance posting and allocations deliberately off on staging. | `FLEET_PARTNER_PAYMENT_WALLET_SERVICE_SECRET` and the matching trust entry in payment-wallet-service on staging (handoff brief 4). Until then the wallet activation gate uses the staging-only waiver (refused in production by config validation). |
| Fleet -> Dispatch / Taxi provider provisioning | Only Hauling has a live contract (`app/wiring.go`). Dispatch and Taxi resolve to `UnconfiguredProvider` and fail closed. | Resolve-or-provision internal endpoints in driver-dispatch-delivery-service and driver-taxi-service, then wiring. Not attempted: it is cross-service work in services that have their own uncommitted workstreams. |
| Marketplace offers (Dispatch `platform.request.offered.v1` -> Fleet projection/offer) | Ingestion code exists behind `FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED` and the caller allowlist, both off by default; Hauling/Taxi send no offer events (audit B). | Producer wiring in the execution services and staging enablement. Not changed this session. |
| Fleet domain events -> other services | Events go to Fleet's outbox/Redis stream; no consumer in other services (audit B). | Consumers in the owning services. Not changed. |
| Media (documents, evidence) | Working (onboarding v2 closure). | none |
| Notification (staff invitations, OTP) | Working (auth v2 closure). | none |

## 3. Events emitted by new code

All through the existing transactional outbox (`events.NewDomainPublisher`),
each with organizationId and correlationId:

`branch.created.v1`, `branch.updated.v1`, `branch.archived.v1`,
`department.created.v1`, `department.updated.v1`, `department.archived.v1`,
`organization.settings_updated.v1`, `role.created.v1`, `role.updated.v1`,
`role.archived.v1`, `role_binding.created.v1` (existing name, reused),
`organization.provider.updated.v1`, `organization.provider.capabilities_changed.v1`,
`organization.provider.policy_changed.v1`, existing provider status events
for reactivate/remove, `shift_template.created.v1`, `shift_template.changed.v1`,
`shift.created.v1` (existing name, reused for generated shifts),
`training.assigned.v1`, `training.completed.v1`, `training.enrollment_cancelled.v1`.

No other service consumes these yet (same as every Fleet event, see 2).
