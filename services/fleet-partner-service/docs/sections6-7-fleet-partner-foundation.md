# Sections 6-7 Fleet Partner Foundation

Staging baseline for this implementation: `fc379da72eb8ed740634fb962327b833a107df63`.

## Shared Internal Contract Foundation

Fleet continues to use the platform HMAC v2 service-auth convention from `shared/go/serviceauth`: service name, timestamp, nonce, signature version and body signature. New Fleet partner internal routes are mounted only on the existing `/internal/fleet-partner/v1` route class, inherit HMAC verification, and additionally check an explicit caller allowlist. Completed-work ingestion also verifies that the caller owns the declared `sourceService`.

`X-Request-ID` remains the correlation surface for synchronous HTTP. Fleet partner requests also carry `correlationId` in state-changing DTOs and persist it on completed-work records, audit entries and outbox events.

The shared app error model now includes `retryable`. Dependency/unavailable errors are retryable; validation, forbidden, stale-version and business-rule failures are terminal.

Money uses integer minor units: `amountMinor` plus `currency`. Percentages use basis points: `rateBps`, where `10000` is 100%.

Durable events use the existing Fleet outbox envelope. New event names:

- `fleet.organization.operating_mode.changed.v1`
- `fleet.compensation_plan.created.v1`
- `fleet.membership.compensation_assigned.v1`
- `fleet.completed_work.recorded.v1`
- `fleet.compensation.resolved.v1`
- `fleet.settlement_instruction.created.v1`

## Operating Mode

Organization settings now persist `operating_mode` with allowed values:

- `CENTRALIZED`
- `DELEGATED`
- `HYBRID`

Existing settings are deterministically backfilled to `CENTRALIZED`. Marketplace operating-mode reads are wired to the real organization settings repository rather than a no-op/default reader. Offers already retain `operating_mode`, so historical offer decisions remain interpretable after later mode changes.

Tenant APIs:

- `GET /fleet-partner/operating-mode`
- `PUT /fleet-partner/operating-mode`

## Compensation Foundation

The Fleet partner foundation introduces:

- `CompensationPlan`
- immutable `CompensationPlanVersion`
- versioned `CompensationPlanRule`
- effective-dated `MembershipCompensationAssignment`
- `CompletedWorkRecord`
- `CompensationResolution`
- immutable `ManagedWorkSettlementInstruction`
- `ProviderFinancialVisibilityPolicy`
- `WorkSession` for shift-backed compensation
- explicit approved compensation adjustments for allowance, overtime, bonus and deduction

Salary rules require `amountMinor`, `currency` and `cadence` (`DAILY`, `WEEKLY`, `MONTHLY`). Salary does not create a per-job provider share.

Commission rules require `rateBps` and an explicit basis. Initial basis values:

- `ELIGIBLE_JOB_GROSS`
- `ORGANIZATION_JOB_SHARE`
- `PROVIDER_ELIGIBLE_BASE`

Per-job and per-shift rules require explicit fixed money configuration. Rule applicability by service and role is executable in the resolver.

## Completed Work And Settlement

Internal endpoint:

- `POST /internal/fleet-partner/v1/completed-work`

The caller must be the authoritative service for the declared source:

- `driver-dispatch-delivery-service` -> `DISPATCH`
- `driver-taxi-service` -> `TAXI`
- `driver-hauling-service` -> `HAULING`

Fleet verifies organization, membership, provider binding, service eligibility, currency, source version and idempotency before recording. Wallet posting is not performed here. Settlement instructions freeze Fleet's compensation entitlement result for a later Wallet phase.

Internal provider context endpoint:

- `GET /internal/fleet-partner/v1/providers/:providerId/corporate-context?service=DISPATCH`

It returns an explicit independent context when no live membership exists. Suspended or non-active memberships are returned as managed with service eligibility false, so downstream managed operations fail closed.

## Feature Flags

- `FLEET_OPERATING_MODE_ENFORCEMENT_ENABLED` defaults `true`.
- `FLEET_COMPENSATION_RESOLVER_ENABLED` defaults `false`.
- `FLEET_COMPLETED_WORK_PROJECTION_ENABLED` defaults `false`.
- `FLEET_SETTLEMENT_INSTRUCTION_ENABLED` defaults `false`.
- `FLEET_TYPED_FINANCIAL_VISIBILITY_ENABLED` defaults `true`.

Security checks, caller/source ownership checks and membership verification are not disableable rollout flags.

## Reconciliation

Migration `000077` adds `fleet_partner_reconciliation_items` for repairable states:

- completed work received but compensation not resolved;
- compensation resolved but settlement instruction not created;
- duplicate source event;
- stale assignment reference;
- invalid membership binding;
- compensation plan missing;
- downstream settlement pending in a later phase.

Operators should use the internal/admin read surfaces and reconciliation rows rather than manual database edits.
