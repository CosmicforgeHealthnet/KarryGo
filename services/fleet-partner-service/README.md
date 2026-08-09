# fleet-partner-service

Corporate logistics control plane for KarryGo organizations (logistics
companies). This service owns organization tenancy — organizations, branches,
departments, settings, memberships-to-come, corporate read models, audit,
outbox/inbox and durable jobs — and integrates with the existing KarryGo
microservices **without creating competing sources of truth**.

Current state: the corporate logistics foundation, organization onboarding,
staff IAM, managed-provider integration, marketplace execution foundations,
company wallets and provider sub-wallets, Section 14 CRM, and the local
Section 15 private-operations aggregates and Section 16 dispatch planning,
routes, control-tower/tracking projections, geofences and alerts are
implemented. Production writes, workers and external providers remain disabled
until their explicit feature flags and authority configurations are enabled.

## What this service owns

Organizations and their profile/settings, branches, departments, organization
staff references, corporate read models, audit records, outbox/inbox records,
durable jobs.

Section 13: organization wallet references, provider sub-wallet links,
allocation instructions, withdrawal policy.

Section 14: CRM customers, contacts and locations, sales opportunities,
quotations and their immutable issued versions, contracts and their immutable
activated versions, service level agreements and their append-only observations,
and customer credit **policy**.

Section 15: service catalogue, private orders and stops, shipments and cargo,
packages, scans and custody, manifests and immutable close snapshots,
recurring templates and occurrences, asynchronous bulk imports, private
provider assignments, and a durable private-execution operation ledger.

Section 16: dispatch plans and board, explainable assignment candidates, route
plans and stops, provider-neutral optimization jobs, Section 15 assignment
publication, control-tower/tracking projections, geofences and operational
alerts. Dispatch and Hauling still own every canonical execution and provider
location event.

## What it never owns

Canonical customer identity, provider identity, trip lifecycle, wallet
ledgers, settlements, verification decisions, platform support truth,
platform analytics truth, KarryGo internal-admin accounts, Partner API
credentials.

It also owns **no accounts receivable**: no debt calculation, no invoice ageing,
no derived available credit. A CRM customer is a commercial relationship record,
never a marketplace identity. Document bytes belong to media-file-service and
delivery belongs to notification-service.

See [docs/platform-boundaries.md](docs/platform-boundaries.md).

## Layout

```
cmd/               HTTP binary (main.go) and worker binary (worker/main.go)
internal/app       Router assembly (middleware chain, route classes, readiness)
internal/config    Validated configuration (fails fast)
internal/database  Dedicated PostgreSQL pool, tx helper, migration runner
internal/middleware Auth boundaries, tenant resolution, correlation IDs, limits
internal/observability Metrics registry + structured logging
internal/platform  Reusable conventions: ids, money, respond, cursor, filters,
                   concurrency, idempotency, audit, tenant, sourceref
internal/platformclients Typed clients for the 11 platform services
internal/events    Event envelope, transactional outbox, consumer inbox
internal/jobs      Durable jobs with leases, heartbeats, recovery
internal/features  Feature boundaries (organizations implemented at
                   foundation level; 28 reserved packages documented)
migrations/        Paired up/down SQL with a tracking table
docs/              Architecture, boundaries, conventions, runbooks
```

## Authentication (passwordless)

```text
KarryGo corporate authentication has no user password.
Email and phone are required.
OTP is delivered to email only.
Managed-provider authentication remains in the owning provider service.
No temporary provider password is sent.
```

There is no password signup, login, hash, reset or temporary credential
anywhere — enforced on every build by `internal/architecture_auth_test.go`.
See [docs/passwordless-authentication.md](docs/passwordless-authentication.md)
and [docs/provider-authentication-boundary.md](docs/provider-authentication-boundary.md).

## Routes

| Class | Path | Auth |
| --- | --- | --- |
| Corporate authentication | `/fleet-partner/v1/auth/{start,verify,resend,refresh}` | none (passwordless OTP flow) |
| Corporate web API | `/fleet-partner/v1/*` | Bearer access token (organization users) |
| Customer secure link | `/fleet-partner/public/v1/quotations/*` | hashed, expiring, single-quotation token; rate limited |
| Internal service API | `/internal/fleet-partner/v1/*` | HMAC service signature |
| Health (liveness only) | `/health` | none |
| Readiness | `/ready` | none |
| Metrics | `/metrics` | Bearer metrics token |

### Section 14, Section 15 and Section 16 OpenAPI contracts

The Section 14 CRM API has a machine-readable OpenAPI 3.1 contract at
[docs/openapi/section14-crm.openapi.json](docs/openapi/section14-crm.openapi.json).
It covers Section 14 only. Section 15 has a separate OpenAPI 3.0 contract at
[docs/openapi/section15-private-operations.openapi.json](docs/openapi/section15-private-operations.openapi.json),
covering 40 paths and 54 operations, including the HMAC-authenticated provider
decision surface. Section 13 is documented separately.
Section 16 has an independent OpenAPI 3.1 contract at
[docs/openapi/section16-dispatch-routes-control.openapi.json](docs/openapi/section16-dispatch-routes-control.openapi.json),
covering 30 paths and 38 operations.

Validate JSON structure, references, path parameters, bidirectional route
parity, permissions, feature flags, idempotency, expected versions, security
classes and data minimization with:

```bash
go test -count=1 -v ./services/fleet-partner-service/internal/features/crm/http -run OpenAPI
go test -count=1 -v ./services/fleet-partner-service/internal/section15contract
go test -count=1 -v ./services/fleet-partner-service/internal/section16contract
```

The test mounts the production registrars and compares Gin's registered route
list with the shared route manifest and the specification in both directions.
To render locally without adding a production documentation route or committing
viewer assets, open the JSON in an OpenAPI 3.1-compatible editor, or run a
temporary viewer such as `npx @redocly/cli preview-docs
services/fleet-partner-service/docs/openapi/section14-crm.openapi.json`.

## Run locally

```bash
cp .env.example .env   # then edit values
go run ./cmd           # HTTP API (in-process workers by default)
go run ./cmd/worker    # dedicated worker process (optional)
```

See [docs/local-development.md](docs/local-development.md) and
[docs/configuration.md](docs/configuration.md).

## Database ownership

Every KarryGo service owns its own database. This service persists **only**
to the `fleet-partner-service` database via the single
`FLEET_PARTNER_DATABASE_URL` setting — no second business-database URL is
accepted, and an architecture test enforces it. `shared/go` supplies generic
helpers only; a shared helper is never a path to another service's database.
Data owned elsewhere arrives through authenticated APIs, durable events and
reconciliation, and is stored as opaque external IDs with source metadata —
never as a database foreign key.

## Tests

```bash
go test ./...                                  # unit + HTTP + architecture tests
```

The PostgreSQL integration suite is mandatory and needs a disposable
database that belongs only to this service:

```bash
docker run -d --name karrygo-fleet-partner-test-db -e POSTGRES_USER=fleet_partner_test -e POSTGRES_PASSWORD=CHOOSE_A_LOCAL_PASSWORD -e POSTGRES_DB=karrygo_fleet_partner_test -p 5446:5432 postgres:16-alpine
```

```bash
FLEET_PARTNER_TEST_DATABASE_URL='postgres://fleet_partner_test:CHOOSE_A_LOCAL_PASSWORD@localhost:5446/karrygo_fleet_partner_test?sslmode=disable' go test -count=1 ./internal/integration/...
```

The suite applies real migrations and rejects any database whose name is not
test-specific. Run the whole CI gate — including the integration suite,
which fails rather than skips — with:

```bash
bash services/fleet-partner-service/scripts/ci.sh
```

See [docs/local-development.md](docs/local-development.md) for cleanup and
smoke checks.

## Documents

- [docs/foundation-audit.md](docs/foundation-audit.md) — pre-implementation repository audit
- [docs/architecture.md](docs/architecture.md) — runtime components and dependency rules
- [docs/platform-boundaries.md](docs/platform-boundaries.md) — authority map, source-of-truth rules
- [docs/api-conventions.md](docs/api-conventions.md) — envelopes, IDs, Money, pagination, idempotency, concurrency
- [docs/multi-tenancy.md](docs/multi-tenancy.md) — tenant context and isolation rules
- [docs/events-and-jobs.md](docs/events-and-jobs.md) — outbox, inbox, durable jobs, leases
- [docs/configuration.md](docs/configuration.md) — every setting, defaults, spec-name mapping
- [docs/local-development.md](docs/local-development.md) — run, migrate, test
- [docs/rollback.md](docs/rollback.md) — migration and deployment rollback

Sections 8–10:

- [docs/auth-onboarding-iam-provider-audit.md](docs/auth-onboarding-iam-provider-audit.md) — pre-implementation audit
- [docs/passwordless-authentication.md](docs/passwordless-authentication.md) — OTP, sessions, step-up
- [docs/organization-onboarding.md](docs/organization-onboarding.md) — stages, documents, submission, activation
- [docs/organization-lifecycle.md](docs/organization-lifecycle.md) — state machine and closure
- [docs/staff-iam.md](docs/staff-iam.md) — invitations, roles, permissions
- [docs/approval-system.md](docs/approval-system.md) — policies, quorum, fingerprinting
- [docs/managed-provider-provisioning.md](docs/managed-provider-provisioning.md) — resolve-before-provision, consent
- [docs/provider-authentication-boundary.md](docs/provider-authentication-boundary.md) — who owns provider identity
- [docs/service-contracts.md](docs/service-contracts.md) — cross-service contract status
- [docs/security.md](docs/security.md) — secrets, authorization, audit
- Runbooks: [auth delivery](docs/runbooks/auth-delivery-failure.md),
  [provider reconciliation](docs/runbooks/provider-provisioning-reconciliation.md),
  [activation](docs/runbooks/organization-activation.md),
  [closure](docs/runbooks/organization-closure.md)

Section 13 — company wallets:

- [docs/company-wallets.md](docs/company-wallets.md) — ownership, link states, sub-wallets
- [docs/provider-allocations.md](docs/provider-allocations.md)
- [docs/provider-withdrawals.md](docs/provider-withdrawals.md)

Section 14 — company CRM, sales, quotations, contracts, SLAs and credit policy:

- [docs/company-crm.md](docs/company-crm.md) — customers, state machines, risk, duplicate detection
- [docs/customer-contacts-and-locations.md](docs/customer-contacts-and-locations.md) — normalization, consent, masking
- [docs/sales-opportunities.md](docs/sales-opportunities.md) — stage matrix, why winning creates nothing
- [docs/quotations.md](docs/quotations.md) — manual pricing, approval, issue, immutable versions, acceptance
- [docs/quotation-customer-access.md](docs/quotation-customer-access.md) — the secure customer link
- [docs/contracts-and-slas.md](docs/contracts-and-slas.md) — activation, immutability, SLA observations
- [docs/customer-credit-policy.md](docs/customer-credit-policy.md) — policy, holds, what Fleet does not calculate
- [docs/crm-events.md](docs/crm-events.md) — event catalogue and data minimization
- [docs/crm-workers.md](docs/crm-workers.md) — expiry, renewal, SLA, notification delivery
- Runbooks: [quotation delivery](docs/runbooks/quotation-delivery-failure.md),
  [acceptance conflict](docs/runbooks/quotation-acceptance-conflict.md),
  [contract activation](docs/runbooks/contract-activation-failure.md),
  [contract expiry](docs/runbooks/contract-expiry.md),
  [SLA unknown outcome](docs/runbooks/sla-unknown-outcome.md),
  [credit hold](docs/runbooks/customer-credit-hold.md),
  [dead-letter replay](docs/runbooks/crm-dead-letter-replay.md)

Every Section 14 write and worker **defaults off**. Enabling one is a deliberate
act; `FLEET_CRM_KILL_SWITCH` overrides all of them.

Section 15 quotation acceptance still creates a durable conversion intent; the
normal order conversion use case consumes that intent exactly once. The service
catalogue, private orders, shipments/cargo, packages/scans/custody, manifests,
recurring templates, bulk imports, and provider assignments are present and
feature-gated. All new writes and workers default off. Taxi is hard-disabled.
See [docs/section15-private-operations.md](docs/section15-private-operations.md).

Section 16 is described operationally in
[docs/section16-dispatch-routes-control.md](docs/section16-dispatch-routes-control.md).
All new writes and workers default off. Optimization requires a configured
provider-neutral authority, tracking accepts only HMAC-authenticated Dispatch or
Hauling events, and Taxi/passenger route publication remains hard-disabled.
