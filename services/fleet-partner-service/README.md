# fleet-partner-service

Corporate logistics control plane for KarryGo organizations (logistics
companies). This service owns organization tenancy — organizations, branches,
departments, settings, memberships-to-come, corporate read models, audit,
outbox/inbox and durable jobs — and integrates with the existing KarryGo
microservices **without creating competing sources of truth**.

Current state: **foundation only** (Sections 3–7 of the Corporate Logistics
Backend Implementation Specification). No onboarding, staff IAM, provider
provisioning, assignments, wallets, orders, warehouse or finance features are
implemented yet.

## What this service owns

Organizations and their profile/settings, branches, departments, organization
staff references (future), private company operations (future), corporate
read models, audit records, outbox/inbox records, durable jobs.

## What it never owns

Canonical customer identity, provider identity, trip lifecycle, wallet
ledgers, settlements, verification decisions, platform support truth,
platform analytics truth, KarryGo internal-admin accounts, Partner API
credentials. See [docs/platform-boundaries.md](docs/platform-boundaries.md).

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
| Internal service API | `/internal/fleet-partner/v1/*` | HMAC service signature |
| Health (liveness only) | `/health` | none |
| Readiness | `/ready` | none |
| Metrics | `/metrics` | Bearer metrics token |

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
